// Command airpods controls AirPods through the dongle's USB HID interface.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Dardrai/airpods-gui/core/disconnect"
	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/argparse"
	"github.com/Dardrai/airpods-gui/internal/cliutil"
)

// version is set at build time with -ldflags -X main.version.
var version = "dev"

// environment is what a command needs from outside the process; tests inject simulators.
type environment struct {
	open      cliutil.OpenFunc
	enumerate func() ([]hid.Info, error)
	clock     dongle.Clock
	// interrupt, when set, gives commands that must clean up a stop channel.
	interrupt func() <-chan struct{}
}

func main() {
	if code, handled := installerPath(os.Args[1:]); handled {
		os.Exit(code)
	}
	env := simEnvironment(os.Getenv(cliutil.SimEnv), os.Stderr, environment{hid.Open, hid.Enumerate, dongle.RealClock, cliutil.NotifyStop})
	os.Exit(runEnv(os.Args[1:], os.Stdout, os.Stderr, env))
}

// runEnv also fails when stdout does: the printing in the commands does not check each line.
func runEnv(args []string, stdout, stderr io.Writer, env environment) int {
	out := &cliutil.LatchWriter{W: stdout}
	return out.Result(runCommand(args, out, stderr, env), stderr)
}

func runCommand(args []string, stdout, stderr io.Writer, env environment) int {
	cli := buildParser()
	ns, exit := cli.Parse(args)
	if exit != nil {
		return cliutil.Finish(exit, stdout, stderr)
	}
	if ns.Bool("debug") {
		env.open = wrapDebug(env.open, stderr, env.clock)
	}
	command, _ := ns.String("command")
	if ns.Bool("json") && !jsonAllowed(command, ns) {
		return cliutil.Finish(cli.Error("--json works only with status, settings, get, list, info and the read commands of hearing, hardware and experimental"), stdout, stderr)
	}
	if command == "help" {
		return runHelp(cli, ns, stdout, stderr)
	}
	if command == "diag" {
		return runDiag(cli.Parsers["diag"], ns, stdout, stderr, env)
	}
	if command == "completion" {
		shell, _ := ns.String("shell")
		writeCompletion(stdout, shell)
		return 0
	}
	if command == "list" {
		serial, _ := ns.String("serial")
		if ns.Bool("serials") {
			if ns.Bool("json") {
				return cliutil.Finish(cli.Parsers["list"].Error("--serials cannot be combined with --json"), stdout, stderr)
			}
			return runSerials(serial, stdout, stderr, env)
		}
		return runList(serial, ns.Bool("json"), stdout, stderr, env)
	}
	var encoded uint32
	if command == "set" {
		name, _ := ns.String("setting")
		value, _ := ns.String("value")
		setting, _ := dongle.LookupAnySetting(name)
		var err error
		if encoded, err = setting.Encode(value); err != nil {
			return cliutil.Finish(cli.Parsers["set"].Error(name+": expected "+err.Error()), stdout, stderr)
		}
	}
	var change *audioChange
	if subcommand, _ := ns.String("subcommand"); command == "audio" && subcommand == "set" {
		audioSet := cli.Parsers["audio"].Parsers["set"]
		parsed, err := parseAudioChange(ns)
		if err != nil {
			return cliutil.Finish(audioSet.Error(err.Error()), stdout, stderr)
		}
		if parsed.codec != nil {
			codec, variable, bitrate := parsed.apply(dongle.AudioState{})
			if err := checkProfile(codec, variable, bitrate); err != nil {
				return cliutil.Finish(audioSet.Error(err.Error()), stdout, stderr)
			}
		}
		change = &parsed
	}
	var job jobFunc
	if build, ok := jobCommands[command]; ok {
		var exit *argparse.Exit
		if job, exit = build(jobContext{cli, ns, env, stdout, stderr}); exit != nil {
			return cliutil.Finish(exit, stdout, stderr)
		}
	}
	var stop <-chan struct{}
	if command == "nearby" {
		if seconds := ns["seconds"].(int); seconds < 1 || seconds > nearbyMaxSeconds {
			return cliutil.Finish(cli.Parsers["nearby"].Error(fmt.Sprintf("--seconds must be 1..%d", nearbyMaxSeconds)), stdout, stderr)
		}
		if env.interrupt != nil {
			stop = env.interrupt()
		}
	}
	serial, _ := ns.String("serial")
	device, err := env.open(serial)
	if err != nil {
		return report(err, stderr)
	}
	defer device.Close()
	client := dongle.NewClient(device)
	client.SetClock(env.clock)
	var failure error
	switch {
	case job != nil:
		failure = job(client)
	case command == "status" && ns.Bool("details"):
		failure = runStatusDetails(client, device.Serial(), ns.Bool("json"), stdout)
	default:
		failure = executeBuiltin(client, command, ns, encoded, change, stop, stdout, stderr)
	}
	if err := failure; err != nil {
		if errors.Is(err, cliutil.ErrInterrupted) || cliutil.IsClosedPipe(err) {
			return cliutil.ExitInterrupted
		}
		if errors.Is(err, errUnreported) {
			return 1
		}
		if errors.Is(err, errUncertain) {
			return 3
		}
		return report(err, stderr)
	}
	return 0
}

func executeBuiltin(client *dongle.Client, command string, ns argparse.Namespace, encoded uint32, change *audioChange, stop <-chan struct{}, stdout, stderr io.Writer) error {
	var failure error
	switch command {
	case "audio":
		failure = runAudio(client, change, stdout)
	case "game":
		word, _ := ns.String("state")
		failure = runGame(client, word, stdout)
	case "nearby":
		failure = runNearby(client, ns["seconds"].(int), stdout, stderr, stop)
	case "perf":
		failure = runPerf(client, stdout)
	case "connect":
		failure = runConnect(client, stdout)
	case "reconnect-policy":
		word, given := ns.String("policy")
		failure = runReconnectPolicy(client, word, given, stdout, stderr)
	case "firmware":
		failure = runFirmware(client, stdout)
	default:
		failure = execute(client, command, ns, encoded, stdout)
	}
	return failure
}

func report(err error, stderr io.Writer) int {
	var unconfirmed *dongle.Unconfirmed
	if errors.As(err, &unconfirmed) {
		fmt.Fprintln(stderr, err)
		return 3
	}
	fmt.Fprintf(stderr, "Error: %v\n", err)
	return 1
}

func runHelp(cli *argparse.Parser, ns argparse.Namespace, stdout, stderr io.Writer) int {
	topic, given := ns.String("topic")
	if !given {
		io.WriteString(stdout, cli.HelpMsg)
		io.WriteString(stdout, "\nSettings:\n")
		for _, setting := range allSettings() {
			fmt.Fprintf(stdout, "  %s: %s\n", setting.Name, setting.ValueHelp())
		}
		io.WriteString(stdout, "\nExamples:\n  airpods get mode\n  airpods set mode transparency\n  airpods set ear-detection on\n")
		return 0
	}
	if setting, ok := dongle.LookupAnySetting(topic); ok {
		fmt.Fprintf(stdout, "%s: %s\n", topic, setting.ValueHelp())
		fmt.Fprintf(stdout, "Get: airpods get %s\nSet: airpods set %s VALUE\n", topic, topic)
		return 0
	}
	if sub, ok := cli.Parsers[topic]; ok {
		io.WriteString(stdout, sub.HelpMsg)
		return 0
	}
	return cliutil.Finish(cli.Error("unknown help topic: "+topic), stdout, stderr)
}

// errUnreported ends a command with exit 1 when what it had to say is already printed, as
// get does for a setting the device has not reported.
var errUnreported = errors.New("setting not reported")

func execute(client *dongle.Client, command string, ns argparse.Namespace, encoded uint32, stdout io.Writer) error {
	if control, ok := controlCommands[command]; ok {
		if err := control.call(client); err != nil {
			return err
		}
		fmt.Fprintln(stdout, control.line)
		return nil
	}
	if command == "disconnect" {
		return runDisconnect(client, stdout)
	}
	settings, err := client.SettingsClient()
	if err != nil {
		return err
	}
	switch command {
	case "status":
		status := settings.Status()
		if ns.Bool("json") {
			return writeJSON(stdout, statusJSON(status))
		}
		fmt.Fprintln(stdout, "AirPods: "+pick(status.Connected, "connected", "disconnected"))
		fmt.Fprintln(stdout, "Settings: "+pick(status.ControlsReady, "ready", "not ready"))
		fmt.Fprintln(stdout, "Codec: "+status.CodecName())
		fmt.Fprintln(stdout, "Battery left: "+dongle.BatteryText(status.BatteryLeft))
		fmt.Fprintln(stdout, "Battery right: "+dongle.BatteryText(status.BatteryRight))
		fmt.Fprintln(stdout, "Battery case: "+dongle.BatteryText(status.BatteryCase))
		return nil
	case "set":
		name, _ := ns.String("setting")
		line, err := setSetting(client, settings, name, encoded, stdout)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, line)
		return nil
	}
	values, err := settings.Read()
	if err != nil {
		return err
	}
	if ns.Bool("json") {
		return writeSettingsJSON(command, ns, values, stdout)
	}
	if command == "get" {
		name, _ := ns.String("setting")
		setting, _ := dongle.LookupAnySetting(name)
		if !printSetting(stdout, setting, values) {
			return errUnreported
		}
		return nil
	}
	for _, setting := range allSettings() {
		printSetting(stdout, setting, values)
	}
	return nil
}

// runDisconnect releases the AirPods through the routing handover, keeping them paired. The
// release is sent even when the AirPods are not connected, since it is also the hold that keeps
// the dongle from taking them back.
func runDisconnect(client *dongle.Client, stdout io.Writer) error {
	status, err := client.Status()
	if err != nil {
		return err
	}
	outcome, err := disconnect.Pick(client, status).Disconnect()
	if errors.Is(err, disconnect.ErrIntentChanged) {
		return fmt.Errorf("%w. %s Run airpods status before you disconnect again", err, intentChangedHint)
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, "disconnect: "+outcome.String())
	fmt.Fprintln(stdout, releaseHold)
	return nil
}

// setSetting writes one setting. Off refused for want of Allow Off, which the dongle forgets on
// every connection, sends allow-off on and then the write once more: the refused one never left
// the dongle. A new process cannot know what an earlier one sent, so it counts as not sent.
func setSetting(client *dongle.Client, settings *dongle.SettingsClient, name string, value uint32, stdout io.Writer) (string, error) {
	line, err := settings.Set(name, value)
	setting, _ := dongle.LookupAnySetting(name)
	off := (setting.Control == model.ControlMode && value == 1) || (setting.Control == model.ControlCycle && value&1 != 0)
	var unsupported *dongle.Unsupported
	if !off || !errors.As(err, &unsupported) || errors.Is(err, dongle.ErrNotReady) {
		return line, err
	}
	view, ok := settingsView(client, settings)
	if !ok || !view.NeedsAllowOffResend(setting.Control, value, false) {
		return line, err
	}
	if _, err := settings.Send("allow-off", 1); err != nil {
		return "", err
	}
	fmt.Fprintln(stdout, "allow-off: on (sent first; the dongle forgets it whenever the AirPods connect again)")
	return settings.Set(name, value)
}

// settingsView is what the AirPods have said about themselves and their settings. false
// means the model is unknown.
func settingsView(client *dongle.Client, settings *dongle.SettingsClient) (model.View, bool) {
	report, err := client.Cached(dongle.SlotMetadata)
	if err != nil || report == nil {
		return model.View{}, false
	}
	meta, ok := dongle.ParseMetadata(report.Packet)
	if !ok {
		return model.View{}, false
	}
	values, err := settings.Read()
	if err != nil {
		return model.View{}, false
	}
	view := model.View{Model: meta.Model, Firmware: meta.Firmware, Reported: make(map[byte]uint32, len(values))}
	for control, record := range values {
		view.Reported[control] = record.Value
	}
	return view, true
}

// jsonCommands are the commands --json applies to.
var jsonCommands = map[string]bool{"status": true, "settings": true, "get": true, "list": true, "info": true}

// jsonAllowed also knows the read subcommands of the command groups.
func jsonAllowed(command string, ns argparse.Namespace) bool {
	subcommand, _ := ns.String("subcommand")
	action, _ := ns.String("action")
	switch command {
	case "hearing":
		return subcommand == "status" || (subcommand == "profile" && action == "show")
	case "hardware":
		return subcommand == "status" || subcommand == "data"
	case "experimental":
		return ((subcommand == "transparency" || subcommand == "eq") && action == "show") || (subcommand == "loud-sound-reduction" && action == "get")
	}
	return jsonCommands[command]
}

func writeSettingsJSON(command string, ns argparse.Namespace, values map[byte]dongle.Record, stdout io.Writer) error {
	if command == "get" {
		name, _ := ns.String("setting")
		setting, _ := dongle.LookupAnySetting(name)
		entry := settingJSON(setting, values)
		err := writeJSON(stdout, struct {
			Schema  int         `json:"schema"`
			Setting jsonSetting `json:"setting"`
		}{jsonSchema, entry})
		if err == nil && entry.Raw == nil {
			return errUnreported // the same exit code as text mode
		}
		return err
	}
	var entries []jsonSetting
	for _, setting := range allSettings() {
		entries = append(entries, settingJSON(setting, values))
	}
	return writeJSON(stdout, struct {
		Schema   int           `json:"schema"`
		Settings []jsonSetting `json:"settings"`
	}{jsonSchema, entries})
}

func printSetting(stdout io.Writer, setting *dongle.Setting, values map[byte]dongle.Record) bool {
	record, ok := values[setting.Control]
	if !ok {
		fmt.Fprintf(stdout, "%s: unknown (not reported; may be unsupported)\n", setting.Name)
		return false
	}
	fmt.Fprintf(stdout, "%s: %s (report age %d ms)\n", setting.Name, setting.Decode(record.Value), record.AgeMs)
	return true
}

func pick(condition bool, yes, no string) string {
	if condition {
		return yes
	}
	return no
}
