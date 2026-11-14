package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/hearing"
	"github.com/Dardrai/airpods-gui/internal/argparse"
)

const experimentalWarning = "Warning: experimental: this may not work through the dongle."

func experimentalJob(ctx jobContext) (jobFunc, *argparse.Exit) {
	if !ctx.ns.Bool("experimental") && !ctx.ns.Bool("experimental_leaf") {
		return nil, ctx.cli.Parsers["experimental"].Error("these commands need --experimental: they may not work through the dongle")
	}
	var job jobFunc
	var exit *argparse.Exit
	subcommand, _ := ctx.ns.String("subcommand")
	action, _ := ctx.ns.String("action")
	switch {
	case subcommand == "transparency" && action == "show":
		job = transparencyShow(ctx)
	case subcommand == "transparency":
		job, exit = transparencySet(ctx)
	case subcommand == "loud-sound-reduction" && action == "get":
		job = loudGet(ctx)
	case subcommand == "loud-sound-reduction":
		job, exit = loudSet(ctx)
	case subcommand == "eq" && action == "show":
		job = eqShow(ctx)
	case subcommand == "eq":
		job, exit = eqSet(ctx)
	default:
		job, exit = renameSend(ctx)
	}
	if exit != nil {
		return nil, exit
	}
	fmt.Fprintln(ctx.stderr, experimentalWarning)
	return job, nil
}

type jsonTransparency struct {
	Schema                     int     `json:"schema"`
	Enabled                    bool    `json:"enabled"`
	LeftAmplification          float32 `json:"left_amplification"`
	RightAmplification         float32 `json:"right_amplification"`
	LeftTone                   float32 `json:"left_tone"`
	RightTone                  float32 `json:"right_tone"`
	LeftConversationBoost      bool    `json:"left_conversation_boost"`
	RightConversationBoost     bool    `json:"right_conversation_boost"`
	LeftAmbientNoiseReduction  float32 `json:"left_ambient_noise_reduction"`
	RightAmbientNoiseReduction float32 `json:"right_ambient_noise_reduction"`
}

func printTransparency(ctx jobContext, s hearing.TransparencySettings) {
	fmt.Fprintln(ctx.stdout, "Custom Transparency: "+onOffWord(s.Enabled))
	fmt.Fprintf(ctx.stdout, "Amplification: left %s, right %s\n", number(s.LeftAmplification), number(s.RightAmplification))
	fmt.Fprintf(ctx.stdout, "Tone: left %s, right %s\n", number(s.LeftTone), number(s.RightTone))
	fmt.Fprintf(ctx.stdout, "Ambient noise reduction: left %s, right %s\n", number(s.LeftAmbientNoiseReduction), number(s.RightAmbientNoiseReduction))
	fmt.Fprintf(ctx.stdout, "Conversation Boost: left %s, right %s\n", onOffWord(s.LeftConversationBoost), onOffWord(s.RightConversationBoost))
}

func transparencyShow(ctx jobContext) jobFunc {
	asJSON := ctx.ns.Bool("json")
	return func(client *dongle.Client) error {
		settings, err := new(hearing.Modes).ExperimentalReadTransparency(client)
		if err != nil {
			return err
		}
		if asJSON {
			return writeJSON(ctx.stdout, jsonTransparency{
				jsonSchema, settings.Enabled, settings.LeftAmplification, settings.RightAmplification,
				settings.LeftTone, settings.RightTone, settings.LeftConversationBoost, settings.RightConversationBoost,
				settings.LeftAmbientNoiseReduction, settings.RightAmbientNoiseReduction,
			})
		}
		printTransparency(ctx, settings)
		return nil
	}
}

// transparencyChange is what transparency set was asked to change; nil stays as stored.
type transparencyChange struct {
	enabled, boost                 *bool
	amplification, tone, reduction *float32
	left, right                    bool
}

func (c transparencyChange) apply(s hearing.TransparencySettings) hearing.TransparencySettings {
	if c.enabled != nil {
		s.Enabled = *c.enabled
	}
	floats := func(value *float32, left, right *float32) {
		if value != nil && c.left {
			*left = *value
		}
		if value != nil && c.right {
			*right = *value
		}
	}
	floats(c.amplification, &s.LeftAmplification, &s.RightAmplification)
	floats(c.tone, &s.LeftTone, &s.RightTone)
	floats(c.reduction, &s.LeftAmbientNoiseReduction, &s.RightAmbientNoiseReduction)
	if c.boost != nil && c.left {
		s.LeftConversationBoost = *c.boost
	}
	if c.boost != nil && c.right {
		s.RightConversationBoost = *c.boost
	}
	return s
}

func parseTransparencyChange(ns argparse.Namespace) (transparencyChange, error) {
	ear, _ := ns.String("ear")
	change := transparencyChange{left: ear != "right", right: ear != "left"}
	onOff := func(dest string) *bool {
		word, given := ns.String(dest)
		if !given {
			return nil
		}
		value := word == "on"
		return &value
	}
	change.enabled, change.boost = onOff("enabled"), onOff("boost")
	var problems []error
	number := func(dest string, low, high float64) *float32 {
		text, given := ns.String(dest)
		if !given {
			return nil
		}
		value, err := strconv.ParseFloat(text, 64)
		if err != nil || !(value >= low && value <= high) {
			problems = append(problems, fmt.Errorf("--%s must be %g to %g", dest, low, high))
			return nil
		}
		narrow := float32(value)
		return &narrow
	}
	change.amplification = number("amplification", -1, 2)
	change.tone = number("tone", -1, 1)
	change.reduction = number("noise-reduction", 0, 1)
	if err := errors.Join(problems...); err != nil {
		return change, err
	}
	if change.enabled == nil && change.boost == nil && change.amplification == nil && change.tone == nil && change.reduction == nil {
		return change, errors.New("nothing to set: give at least one of --enabled, --amplification, --tone, --noise-reduction and --conversation-boost")
	}
	return change, nil
}

func transparencySet(ctx jobContext) (jobFunc, *argparse.Exit) {
	parser := ctx.cli.Parsers["experimental"].Parsers["transparency"].Parsers["set"]
	change, err := parseTransparencyChange(ctx.ns)
	if err != nil {
		return nil, parser.Error(err.Error())
	}
	return func(client *dongle.Client) error {
		modes := new(hearing.Modes)
		expected, err := modes.ExperimentalReadTransparency(client)
		if err != nil {
			return err
		}
		requested := change.apply(expected)
		if requested == expected {
			fmt.Fprintln(ctx.stdout, "transparency: already stored; no write sent")
			return nil
		}
		settings, err := modes.ExperimentalWriteTransparency(client, expected, requested)
		if err != nil {
			return err
		}
		fmt.Fprintln(ctx.stdout, "transparency: written and read back")
		printTransparency(ctx, settings)
		return nil
	}, nil
}

func loudGet(ctx jobContext) jobFunc {
	asJSON := ctx.ns.Bool("json")
	return func(client *dongle.Client) error {
		value, err := hearing.ExperimentalReadLoudSoundReduction(client)
		if err != nil {
			return err
		}
		if asJSON {
			return writeJSON(ctx.stdout, struct {
				Schema int    `json:"schema"`
				State  string `json:"state"`
			}{jsonSchema, onOffWord(value)})
		}
		fmt.Fprintln(ctx.stdout, "Loud Sound Reduction: "+onOffWord(value))
		return nil
	}
}

func loudSet(ctx jobContext) (jobFunc, *argparse.Exit) {
	word, _ := ctx.ns.String("state")
	return func(client *dongle.Client) error {
		expected, err := hearing.ExperimentalReadLoudSoundReduction(client)
		if err != nil {
			return err
		}
		if expected == (word == "on") {
			fmt.Fprintf(ctx.stdout, "loud-sound-reduction: %s (already reported; no write sent)\n", word)
			return nil
		}
		if _, err := hearing.ExperimentalSetLoudSoundReduction(client, expected, word == "on"); err != nil {
			return err
		}
		fmt.Fprintf(ctx.stdout, "loud-sound-reduction: %s (written and read back)\n", word)
		return nil
	}, nil
}

type jsonEQ struct {
	Schema  int  `json:"schema"`
	Enabled bool `json:"enabled"`
	Low     byte `json:"low"`
	Mid     byte `json:"mid"`
	High    byte `json:"high"`
}

func printEQ(ctx jobContext, e hearing.EQ) {
	fmt.Fprintln(ctx.stdout, "EQ: "+onOffWord(e.On))
	fmt.Fprintf(ctx.stdout, "Low: %d\nMid: %d\nHigh: %d\n", e.Low, e.Mid, e.High)
}

func eqShow(ctx jobContext) jobFunc {
	asJSON := ctx.ns.Bool("json")
	return func(client *dongle.Client) error {
		e, err := hearing.ExperimentalReadEQ(client)
		if err != nil {
			return err
		}
		if asJSON {
			return writeJSON(ctx.stdout, jsonEQ{jsonSchema, e.On, e.Low, e.Mid, e.High})
		}
		printEQ(ctx, e)
		return nil
	}
}

// eqChange is what eq set was asked to change; nil stays as the AirPods reported it.
type eqChange struct {
	on             *bool
	low, mid, high *byte
}

func (c eqChange) apply(e hearing.EQ) hearing.EQ {
	if c.on != nil {
		e.On = *c.on
	}
	for _, band := range []struct {
		value  *byte
		target *byte
	}{{c.low, &e.Low}, {c.mid, &e.Mid}, {c.high, &e.High}} {
		if band.value != nil {
			*band.target = *band.value
		}
	}
	return e
}

func parseEQChange(ns argparse.Namespace) (eqChange, error) {
	var change eqChange
	if word, given := ns.String("enabled"); given {
		on := word == "on"
		change.on = &on
	}
	var problems []error
	for _, band := range []struct {
		dest   string
		target **byte
	}{{"low", &change.low}, {"mid", &change.mid}, {"high", &change.high}} {
		value, given := ns[band.dest].(int)
		if !given {
			continue
		}
		if value < 0 || value > 100 {
			problems = append(problems, fmt.Errorf("--%s must be 0 to 100", band.dest))
			continue
		}
		narrow := byte(value)
		*band.target = &narrow
	}
	if err := errors.Join(problems...); err != nil {
		return change, err
	}
	if change.on == nil && change.low == nil && change.mid == nil && change.high == nil {
		return change, errors.New("nothing to set: give at least one of --enabled, --low, --mid and --high")
	}
	return change, nil
}

// eqSet is a read, modify and confirm: what is not given keeps the AirPods' reported value, and
// only a new EQ report with exactly the values sent confirms it.
func eqSet(ctx jobContext) (jobFunc, *argparse.Exit) {
	parser := ctx.cli.Parsers["experimental"].Parsers["eq"].Parsers["set"]
	change, err := parseEQChange(ctx.ns)
	if err != nil {
		return nil, parser.Error(err.Error())
	}
	return func(client *dongle.Client) error {
		expected, err := hearing.ExperimentalReadEQ(client)
		if err != nil {
			return err
		}
		requested := change.apply(expected)
		if requested == expected {
			fmt.Fprintln(ctx.stdout, "eq: already reported; no write sent")
			printEQ(ctx, expected)
			return nil
		}
		confirmed, err := hearing.ExperimentalSetEQ(client, expected, requested)
		if err != nil {
			return err
		}
		fmt.Fprintln(ctx.stdout, "eq: confirmed by the AirPods")
		printEQ(ctx, confirmed)
		return nil
	}, nil
}

// validRename is the check hearing.ExperimentalRename makes itself, here so that a bad name is a
// bad command line and the dongle is not opened. A test keeps the two in step.
func validRename(name string) bool {
	return utf8.ValidString(name) && strings.TrimSpace(name) != "" && len(name) <= hearing.MaxNameBytes && !strings.ContainsFunc(name, unicode.IsControl)
}

func renameSend(ctx jobContext) (jobFunc, *argparse.Exit) {
	name, _ := ctx.ns.String("name")
	if !validRename(name) {
		parser := ctx.cli.Parsers["experimental"].Parsers["rename"]
		return nil, parser.Error(fmt.Sprintf("Use a name of 1 to %d UTF-8 bytes with no control characters.", hearing.MaxNameBytes))
	}
	return func(client *dongle.Client) error {
		result, err := hearing.ExperimentalRename(client, name)
		if err != nil {
			return err
		}
		fmt.Fprintln(ctx.stdout, "rename: "+result.Status)
		fmt.Fprintln(ctx.stdout, result.Note)
		return nil
	}, nil
}
