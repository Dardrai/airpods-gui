package main

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/internal/argparse"
)

// audioCodec is one codec of audio set. The bitrates mirror dongle.AudioProfileRequest,
// which stays the authority; a test keeps the two in step.
type audioCodec struct {
	word     string
	value    byte
	name     string
	bitrates []uint32
}

var audioCodecs = []audioCodec{
	{word: "auto", value: 0, name: "automatic"},
	{word: "sbc", value: 1, name: "SBC"},
	{word: "aac", value: 2, name: "AAC", bitrates: []uint32{96000, 128000, 174000, 192000, 220000}},
	{word: "aac-eld", value: 3, name: "AAC-ELD", bitrates: []uint32{128000, 174000, 192000, 220000, 256000}},
}

func codecWords() []string {
	words := make([]string, len(audioCodecs))
	for i, codec := range audioCodecs {
		words[i] = codec.word
	}
	return words
}

// audioChange is what audio set was asked to change; nil leaves a value to the rule in apply.
type audioChange struct {
	codec    *byte
	variable *bool
	bitrate  *uint32
}

// parseAudioChange reads the flags. It refuses what no codec could accept.
func parseAudioChange(ns argparse.Namespace) (audioChange, error) {
	var change audioChange
	if word, given := ns.String("codec"); given {
		for _, codec := range audioCodecs {
			if codec.word == word {
				value := codec.value
				change.codec = &value
			}
		}
	}
	if word, given := ns.String("variable"); given {
		value := word == "on"
		change.variable = &value
	}
	if word, given := ns.String("bitrate"); given {
		value, err := parseBitrate(word)
		if err != nil {
			return change, err
		}
		change.bitrate = &value
	}
	if change.codec == nil && change.variable == nil && change.bitrate == nil {
		return change, errors.New("give at least one of --codec, --bitrate and --variable")
	}
	return change, nil
}

func parseBitrate(word string) (uint32, error) {
	if word == "default" {
		return 0, nil
	}
	value, err := strconv.ParseUint(word, 10, 32)
	if err == nil {
		for _, codec := range audioCodecs {
			if slices.Contains(codec.bitrates, uint32(value)) {
				return uint32(value), nil
			}
		}
	}
	return 0, fmt.Errorf("--bitrate: expected default or a bitrate of aac (%s) or aac-eld (%s)", bitrateWords(audioCodecs[2]), bitrateWords(audioCodecs[3]))
}

func bitrateWords(codec audioCodec) string {
	words := make([]string, len(codec.bitrates))
	for i, bitrate := range codec.bitrates {
		words[i] = strconv.FormatUint(uint64(bitrate), 10)
	}
	return strings.Join(words, ", ")
}

// apply fills in what was not given. A new codec starts from its defaults, because the
// old bitrate may not exist for it; otherwise the requested values stay.
func (c audioChange) apply(state dongle.AudioState) (codec byte, variable bool, bitrate uint32) {
	codec, variable, bitrate = state.RequestedCodec, state.Variable, state.RequestedBitrate
	if c.codec != nil {
		codec, variable, bitrate = *c.codec, false, 0
	}
	if c.variable != nil {
		variable = *c.variable
	}
	if c.bitrate != nil {
		bitrate = *c.bitrate
	}
	return codec, variable, bitrate
}

// checkProfile says what is wrong with a combination, in the terms of the flags.
func checkProfile(codec byte, variable bool, bitrate uint32) error {
	entry := audioCodecs[codec]
	switch {
	case variable && entry.word != "aac-eld":
		return fmt.Errorf("--variable on needs codec aac-eld, not %s", entry.word)
	case bitrate != 0 && entry.bitrates == nil:
		return fmt.Errorf("--bitrate needs codec aac or aac-eld, not %s", entry.word)
	case bitrate != 0 && !slices.Contains(entry.bitrates, bitrate):
		return fmt.Errorf("--bitrate: %s expects default, %s", entry.word, bitrateWords(entry))
	}
	return nil
}

// settledAudio classifies a failed profile or game mode write. A refusal did not apply, and
// neither did a request that was never sent; anything else may have, because the change can
// restart the link before the reply.
func settledAudio(name string, err error) error {
	if errors.Is(err, dongle.ErrNotSent) {
		return err
	}
	var unsupported *dongle.Unsupported
	if errors.As(err, &unsupported) {
		return err
	}
	var rejected *dongle.RequestRejected
	if errors.As(err, &rejected) && (rejected.Code == 1 || rejected.Code == 2 || rejected.Code == 4) {
		return err
	}
	return &dongle.Unconfirmed{
		Message: fmt.Sprintf("%s: not confirmed: %s. It may have applied; run airpods audio before retrying.", name, strings.TrimSuffix(err.Error(), ".")),
		Err:     err,
	}
}

// requireTyped refuses firmware without typed requests, before anything is sent to them.
func requireTyped(client *dongle.Client) error {
	status, err := client.Status()
	if err != nil {
		return err
	}
	if !status.Has(dongle.CapTypedRPC) {
		return errors.New("Firmware does not advertise typed settings support")
	}
	return nil
}

func runAudio(client *dongle.Client, change *audioChange, stdout io.Writer) error {
	if err := requireTyped(client); err != nil {
		return err
	}
	state, err := client.AudioState()
	if err != nil {
		return err
	}
	if change == nil {
		printAudio(stdout, state)
		return nil
	}
	codec, variable, bitrate := change.apply(state)
	if err := checkProfile(codec, variable, bitrate); err != nil {
		return err
	}
	if codec == state.RequestedCodec && variable == state.Variable && bitrate == state.RequestedBitrate {
		fmt.Fprintln(stdout, "audio set: already requested; no write sent")
		printAudio(stdout, state)
		return nil
	}
	state, err = client.SetAudioProfile(codec, variable, bitrate)
	if err != nil {
		return settledAudio("audio set", err)
	}
	fmt.Fprintln(stdout, "audio set: accepted by dongle")
	printAudio(stdout, state)
	return nil
}

func printAudio(stdout io.Writer, state dongle.AudioState) {
	activity := "idle"
	switch {
	case state.Pending:
		activity = "change pending"
	case state.Streaming:
		activity = "streaming"
	}
	delay := "not reported"
	if state.ReportedDelayTenthsMs != nil {
		delay = fmt.Sprintf("%.1f ms", float64(*state.ReportedDelayTenthsMs)/10)
	}
	fmt.Fprintln(stdout, "Codec: "+codecText(state.Codec))
	fmt.Fprintln(stdout, "Bitrate: "+bitrateText(state.Bitrate))
	fmt.Fprintln(stdout, "Requested codec: "+audioCodecs[state.RequestedCodec].name)
	fmt.Fprintln(stdout, "Requested bitrate: "+bitrateText(state.RequestedBitrate))
	fmt.Fprintln(stdout, "Variable bitrate: "+pick(state.Variable, "on", "off"))
	fmt.Fprintln(stdout, "Activity: "+activity)
	fmt.Fprintln(stdout, "Microphone: "+pick(state.MicrophoneActive, "active", "not active"))
	fmt.Fprintf(stdout, "Input: %d\n", state.Input)
	fmt.Fprintln(stdout, "Reported delay: "+delay)
	fmt.Fprintln(stdout, "Game mode: "+pick(state.GameModeSupported, dongle.GameModeLabel(state.GameModePhase), "not supported"))
}

func codecText(codec byte) string {
	if codec == 0 || int(codec) >= len(audioCodecs) {
		return "unknown"
	}
	return audioCodecs[codec].name
}

func bitrateText(bitrate uint32) string {
	if bitrate == 0 {
		return "default"
	}
	return fmt.Sprintf("%d kbps", bitrate/1000)
}

func runGame(client *dongle.Client, word string, stdout io.Writer) error {
	if err := requireTyped(client); err != nil {
		return err
	}
	state, err := client.AudioState()
	if err != nil {
		return err
	}
	if !state.GameModeSupported {
		return errors.New("Game mode is not supported by this dongle")
	}
	on := word == "on"
	if state.GameModeRequested == on {
		fmt.Fprintf(stdout, "game: %s (already requested, now %s; no write sent)\n", word, dongle.GameModeLabel(state.GameModePhase))
		return nil
	}
	state, err = client.SetGameMode(on)
	if err != nil {
		return settledAudio("game", err)
	}
	if state.GameModePhase == dongle.GameModeFailed || state.GameModePhase == dongle.GameModeUnsupported {
		return fmt.Errorf("game: the dongle reports %s", dongle.GameModeLabel(state.GameModePhase))
	}
	fmt.Fprintf(stdout, "game: %s (accepted by dongle, now %s)\n", word, dongle.GameModeLabel(state.GameModePhase))
	return nil
}

func runPerf(client *dongle.Client, stdout io.Writer) error {
	if err := requireTyped(client); err != nil {
		return err
	}
	perf, err := client.Performance()
	if err != nil {
		return err
	}
	line := func(label string, value *uint32, format func(uint32) string) {
		text := "not reported"
		if value != nil {
			text = format(*value)
		}
		fmt.Fprintln(stdout, label+": "+text)
	}
	plain := func(value uint32) string { return strconv.FormatUint(uint64(value), 10) }
	micros := func(value uint32) string { return fmt.Sprintf("%d us", value) }
	kilobytes := func(value uint32) string { return fmt.Sprintf("%.1f KiB", float64(value)/1024) }
	ofCapacity := func(value uint32) string {
		if perf.PCMCapacity == nil {
			return plain(value)
		}
		return fmt.Sprintf("%d of %d", value, *perf.PCMCapacity)
	}
	line("Uptime", perf.UptimeMs, func(value uint32) string {
		return (time.Duration(value) * time.Millisecond).Round(time.Second).String()
	})
	fmt.Fprintf(stdout, "Input: %d\n", perf.Input)
	fmt.Fprintln(stdout, "Codec: "+codecText(perf.Codec))
	fmt.Fprintf(stdout, "Flags: 0x%02x\n", perf.Flags)
	line("Buffer used", perf.PCMUsed, ofCapacity)
	line("Buffer high water", perf.PCMHighWater, ofCapacity)
	line("Underflow frames", perf.UnderflowFrames, plain)
	line("Dropped frames", perf.DroppedFrames, plain)
	line("Encoded units", perf.EncodedUnits, plain)
	line("Encode time", perf.EncodeUs, micros)
	line("Encode time max", perf.EncodeMaxUs, micros)
	line("Send failures", perf.SendFailures, plain)
	line("Recoveries", perf.Recoveries, plain)
	line("Heap capacity", perf.HeapCapacity, kilobytes)
	line("Heap allocated", perf.HeapAllocated, kilobytes)
	line("Heap remaining", perf.HeapRemaining, kilobytes)
	line("Heap arena", perf.HeapArena, kilobytes)
	return nil
}
