package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/hearing"
	"github.com/Dardrai/airpods-gui/internal/argparse"
)

// profileInput is what apply can change, from flags or a file. A nil field stays as stored. The
// JSON names are those of the profile object that hearing profile show --json prints.
type profileInput struct {
	Left              *[]float64 `json:"left_db_hl"`
	Right             *[]float64 `json:"right_db_hl"`
	Amplification     *float64   `json:"amplification"`
	Balance           *float64   `json:"balance"`
	Tone              *float64   `json:"tone"`
	ConversationBoost *bool      `json:"conversation_boost"`
}

func (p profileInput) empty() bool {
	return p.Left == nil && p.Right == nil && p.Amplification == nil && p.Balance == nil && p.Tone == nil && p.ConversationBoost == nil
}

func readProfileFile(path string) (profileInput, error) {
	var input profileInput
	file, err := os.Open(path)
	if err != nil {
		return input, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields() // A misspelt field must not quietly leave a value as it is.
	if err := decoder.Decode(&input); err != nil {
		return input, fmt.Errorf("%s: %w", path, err)
	}
	if decoder.More() {
		return input, fmt.Errorf("%s: expected one JSON object", path)
	}
	return input, nil
}

func parseBands(text string) (*[]float64, error) {
	var values []float64
	for _, word := range strings.Split(text, ",") {
		value, err := strconv.ParseFloat(strings.TrimSpace(word), 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a number", word)
		}
		values = append(values, value)
	}
	return &values, nil
}

func checkRange(name string, value *float64, low, high float64) error {
	if value != nil && !(*value >= low && *value <= high) { // Also catches NaN.
		return fmt.Errorf("%s must be %g to %g", name, low, high)
	}
	return nil
}

func (p profileInput) check() error {
	for _, ear := range []struct {
		name  string
		bands *[]float64
	}{{"left", p.Left}, {"right", p.Right}} {
		if ear.bands == nil {
			continue
		}
		if len(*ear.bands) != hearing.Bands {
			return fmt.Errorf("the %s ear needs %d values, for %s Hz", ear.name, hearing.Bands, bandNames())
		}
		for _, value := range *ear.bands {
			if !(value >= 0 && value <= 90) {
				return fmt.Errorf("every %s ear value must be 0 to 90 dB HL", ear.name)
			}
		}
	}
	err := errors.Join(checkRange("amplification", p.Amplification, -1.5, 1.5), checkRange("balance", p.Balance, -3, 3), checkRange("tone", p.Tone, -1, 1))
	if err == nil && p.Amplification != nil && p.Balance != nil {
		err = checkGains(*p.Amplification, *p.Balance)
	}
	return err
}

// checkGains rejects a combination that puts either ear's stored gain outside -1.5 to 1.5:
// each ear is the amplification minus or plus half the balance, and nothing is clamped.
func checkGains(amplification, balance float64) error {
	left, right := amplification-balance/2, amplification+balance/2
	if left < -1.5 || left > 1.5 || right < -1.5 || right > 1.5 {
		return fmt.Errorf("amplification %g with balance %g puts the ears' gains at left %g, right %g; each must be -1.5 to 1.5", amplification, balance, left, right)
	}
	return nil
}

func bandNames() string {
	names := make([]string, len(hearing.BandHz))
	for i, hz := range hearing.BandHz {
		names[i] = strconv.Itoa(hz)
	}
	return strings.Join(names, ", ")
}

// profileFlags reads the file first, and the flags then replace what it says.
func profileFlags(ns argparse.Namespace) (profileInput, error) {
	var input profileInput
	if path, given := ns.String("file"); given {
		var err error
		if input, err = readProfileFile(path); err != nil {
			return input, err
		}
	}
	var problems []error
	for _, band := range []struct {
		dest   string
		target **[]float64
	}{{"left", &input.Left}, {"right", &input.Right}} {
		if text, given := ns.String(band.dest); given {
			values, err := parseBands(text)
			if err != nil {
				problems = append(problems, fmt.Errorf("--%s: %w", band.dest, err))
				continue
			}
			*band.target = values
		}
	}
	for _, option := range []struct {
		dest   string
		target **float64
	}{{"amplification", &input.Amplification}, {"balance", &input.Balance}, {"tone", &input.Tone}} {
		if text, given := ns.String(option.dest); given {
			value, err := strconv.ParseFloat(text, 64)
			if err != nil {
				problems = append(problems, fmt.Errorf("--%s: %q is not a number", option.dest, text))
				continue
			}
			*option.target = &value
		}
	}
	if err := errors.Join(problems...); err != nil {
		return input, err
	}
	if word, given := ns.String("boost"); given {
		value := word == "on"
		input.ConversationBoost = &value
	}
	if input.empty() {
		return input, errors.New("nothing to apply: give --file, or at least one of --left, --right, --amplification, --balance, --tone and --conversation-boost")
	}
	return input, input.check()
}

func curveOf(stored [hearing.Bands]float32, given *[]float64) ([]float64, bool) {
	current := make([]float64, hearing.Bands)
	for i, value := range stored {
		current[i] = float64(value)
	}
	if given == nil {
		return current, false
	}
	changed := false
	for i, value := range *given {
		if float32(value) != stored[i] {
			changed = true
		}
	}
	return *given, changed
}

// applyTo moves the draft only for values that differ from what the edit shows, because a
// combined slider that moves rewrites both ears' stored gains or tones.
func (p profileInput) applyTo(edit *hearing.ProfileEdit) bool {
	draft := edit.Draft
	left, leftChanged := curveOf(draft.LeftEQ, p.Left)
	right, rightChanged := curveOf(draft.RightEQ, p.Right)
	ok := true
	if leftChanged || rightChanged {
		ok = edit.EditCurve(left, right)
	}
	if p.Amplification != nil && float32(*p.Amplification) != edit.Amplification {
		ok = edit.EditAmplification(float32(*p.Amplification)) && ok
	}
	if p.Balance != nil && float32(*p.Balance) != edit.Balance {
		ok = edit.EditBalance(float32(*p.Balance)) && ok
	}
	if p.Tone != nil && float32(*p.Tone) != (draft.LeftTone+draft.RightTone)/2 {
		ok = edit.EditTone(float32(*p.Tone)) && ok
	}
	if p.ConversationBoost != nil && *p.ConversationBoost != (draft.LeftConversationBoost && draft.RightConversationBoost) {
		ok = edit.EditConversationBoost(*p.ConversationBoost) && ok
	}
	return ok
}

func number(value float32) string { return strconv.FormatFloat(float64(value), 'g', -1, 32) }

func onOffWord(on bool) string { return pick(on, "on", "off") }

// profileChanges words every difference between two profiles, one per line.
func profileChanges(before, after hearing.Settings) []string {
	var lines []string
	bands := func(name string, from, to [hearing.Bands]float32) {
		for i := range from {
			if from[i] != to[i] {
				lines = append(lines, fmt.Sprintf("%s %d Hz: %s -> %s dB HL", name, hearing.BandHz[i], number(from[i]), number(to[i])))
			}
		}
	}
	bands("Left", before.LeftEQ, after.LeftEQ)
	bands("Right", before.RightEQ, after.RightEQ)
	for _, row := range []struct {
		name     string
		from, to float32
	}{
		{"Left gain", before.LeftAmplification, after.LeftAmplification},
		{"Right gain", before.RightAmplification, after.RightAmplification},
		{"Left tone", before.LeftTone, after.LeftTone},
		{"Right tone", before.RightTone, after.RightTone},
	} {
		if row.from != row.to {
			lines = append(lines, fmt.Sprintf("%s: %s -> %s", row.name, number(row.from), number(row.to)))
		}
	}
	for _, row := range []struct {
		name     string
		from, to bool
	}{
		{"Left Conversation Boost", before.LeftConversationBoost, after.LeftConversationBoost},
		{"Right Conversation Boost", before.RightConversationBoost, after.RightConversationBoost},
	} {
		if row.from != row.to {
			lines = append(lines, fmt.Sprintf("%s: %s -> %s", row.name, onOffWord(row.from), onOffWord(row.to)))
		}
	}
	return lines
}

func profileApplyJob(ctx jobContext) (jobFunc, *argparse.Exit) {
	parser := ctx.cli.Parsers["hearing"].Parsers["profile"].Parsers["apply"]
	input, err := profileFlags(ctx.ns)
	if err != nil {
		return nil, parser.Error(err.Error())
	}
	yes := ctx.ns.Bool("yes")
	return func(client *dongle.Client) error {
		modes := new(hearing.Modes)
		profile, err := modes.ReadProfile(client)
		if err != nil {
			return err
		}
		if !profile.Editable {
			return &hearing.Refused{Reason: "This hearing profile can be read but not edited."}
		}
		edit := hearing.NewProfileEdit(*profile)
		if input.Amplification != nil || input.Balance != nil {
			// Only one of the two was given: the other is the stored one, so the check needs the read.
			amplification, balance := float64(edit.Amplification), float64(edit.Balance)
			if input.Amplification != nil {
				amplification = *input.Amplification
			}
			if input.Balance != nil {
				balance = *input.Balance
			}
			if err := checkGains(amplification, balance); err != nil {
				return &hearing.Refused{Reason: "Nothing was sent: " + err.Error() + "."}
			}
		}
		if !input.applyTo(edit) || !edit.CanApply() {
			return &hearing.Refused{Reason: "A complete, valid hearing profile is required."}
		}
		baseline, draft := edit.Baseline(), edit.Draft
		changes := profileChanges(baseline, draft)
		if len(changes) == 0 {
			fmt.Fprintln(ctx.stdout, "hearing profile: already stored; no write sent")
			return nil
		}
		if err := modes.PrepareProfile(client, baseline); err != nil {
			return err
		}
		fmt.Fprintln(ctx.stdout, "This will change:")
		for _, line := range changes {
			fmt.Fprintln(ctx.stdout, "  "+line)
		}
		fmt.Fprintln(ctx.stderr, hearingWarning)
		if !yes {
			if err := confirmHearing("Apply this hearing profile?", "applying a hearing profile"); err != nil {
				return err
			}
		}
		result, err := modes.ApplyProfile(client, baseline, draft)
		if err != nil {
			return hearingFailure(ctx, client, modes, err)
		}
		if !result.Written {
			fmt.Fprintln(ctx.stdout, "hearing profile: already stored; no write sent")
			return nil
		}
		fmt.Fprintln(ctx.stdout, "hearing profile: applied and read back")
		return nil
	}, nil
}

type jsonProfileValues struct {
	Left              []float32 `json:"left_db_hl"`
	Right             []float32 `json:"right_db_hl"`
	Amplification     float32   `json:"amplification"`
	Balance           float32   `json:"balance"`
	Tone              float32   `json:"tone"`
	ConversationBoost bool      `json:"conversation_boost"`
}

type jsonProfileStored struct {
	LeftAmplification          float32 `json:"left_amplification"`
	RightAmplification         float32 `json:"right_amplification"`
	LeftTone                   float32 `json:"left_tone"`
	RightTone                  float32 `json:"right_tone"`
	LeftConversationBoost      bool    `json:"left_conversation_boost"`
	RightConversationBoost     bool    `json:"right_conversation_boost"`
	LeftAmbientNoiseReduction  float32 `json:"left_ambient_noise_reduction"`
	RightAmbientNoiseReduction float32 `json:"right_ambient_noise_reduction"`
	OwnVoice                   float32 `json:"own_voice"`
}

type jsonProfile struct {
	Schema   int               `json:"schema"`
	Editable bool              `json:"editable"`
	BandsHz  []int             `json:"bands_hz"`
	Profile  jsonProfileValues `json:"profile"`
	Stored   jsonProfileStored `json:"stored"`
}

func joinNumbers(values [hearing.Bands]float32) string {
	words := make([]string, len(values))
	for i, value := range values {
		words[i] = number(value)
	}
	return strings.Join(words, " ")
}

func printProfile(stdout io.Writer, profile *hearing.Profile, edit *hearing.ProfileEdit) {
	s := profile.Settings
	fmt.Fprintln(stdout, "Editable: "+pick(profile.Editable, "yes", "no; it can be read but not written back"))
	fmt.Fprintf(stdout, "Bands: %s Hz\n", strings.Join(strings.Split(bandNames(), ", "), " "))
	fmt.Fprintln(stdout, "Left: "+joinNumbers(s.LeftEQ)+" dB HL")
	fmt.Fprintln(stdout, "Right: "+joinNumbers(s.RightEQ)+" dB HL")
	fmt.Fprintln(stdout, "Amplification: "+number(edit.Amplification))
	fmt.Fprintln(stdout, "Balance: "+number(edit.Balance))
	fmt.Fprintf(stdout, "Tone: left %s, right %s\n", number(s.LeftTone), number(s.RightTone))
	fmt.Fprintf(stdout, "Conversation Boost: left %s, right %s\n", onOffWord(s.LeftConversationBoost), onOffWord(s.RightConversationBoost))
	fmt.Fprintf(stdout, "Stored gain: left %s, right %s\n", number(s.LeftAmplification), number(s.RightAmplification))
	fmt.Fprintf(stdout, "Ambient noise reduction: left %s, right %s\n", number(s.LeftAmbientNoiseReduction), number(s.RightAmbientNoiseReduction))
	fmt.Fprintln(stdout, "Own voice: "+number(s.OwnVoice))
}

func profileJSON(profile *hearing.Profile, edit *hearing.ProfileEdit) jsonProfile {
	s := profile.Settings
	return jsonProfile{
		Schema: jsonSchema, Editable: profile.Editable, BandsHz: hearing.BandHz[:],
		Profile: jsonProfileValues{
			Left: s.LeftEQ[:], Right: s.RightEQ[:], Amplification: edit.Amplification, Balance: edit.Balance,
			Tone: (s.LeftTone + s.RightTone) / 2, ConversationBoost: s.LeftConversationBoost && s.RightConversationBoost,
		},
		Stored: jsonProfileStored{
			LeftAmplification: s.LeftAmplification, RightAmplification: s.RightAmplification,
			LeftTone: s.LeftTone, RightTone: s.RightTone,
			LeftConversationBoost: s.LeftConversationBoost, RightConversationBoost: s.RightConversationBoost,
			LeftAmbientNoiseReduction: s.LeftAmbientNoiseReduction, RightAmbientNoiseReduction: s.RightAmbientNoiseReduction,
			OwnVoice: s.OwnVoice,
		},
	}
}

func profileShowJob(ctx jobContext) (jobFunc, *argparse.Exit) {
	asJSON := ctx.ns.Bool("json")
	return func(client *dongle.Client) error {
		modes := new(hearing.Modes)
		profile, err := modes.ReadProfile(client)
		if err != nil {
			return err
		}
		edit := hearing.NewProfileEdit(*profile)
		if asJSON {
			return writeJSON(ctx.stdout, profileJSON(profile, edit))
		}
		printProfile(ctx.stdout, profile, edit)
		return nil
	}, nil
}
