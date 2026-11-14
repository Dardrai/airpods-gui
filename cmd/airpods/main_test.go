package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/Dardrai/airpods-gui/core/disconnect"
	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/internal/cliutil"
)

type golden struct {
	Name   string          `json:"name"`
	Argv   []string        `json:"argv"`
	Sim    json.RawMessage `json:"sim"`
	Stdout string          `json:"stdout"`
	Stderr string          `json:"stderr"`
	Exit   int             `json:"exit"`
	Ops    int             `json:"ops"`
	Trace  string          `json:"trace"`
}

func run(args []string, stdout, stderr io.Writer, open cliutil.OpenFunc) int {
	return runClock(args, stdout, stderr, open, dongle.RealClock)
}

// runClock gives a test no way to the real dongles: a test that lists them passes its own enumerate.
func runClock(args []string, stdout, stderr io.Writer, open cliutil.OpenFunc, clock dongle.Clock) int {
	noEnumerate := func() ([]hid.Info, error) { panic("this test would have enumerated the real dongles") }
	return runEnv(args, stdout, stderr, environment{open, noEnumerate, clock, nil})
}

func exec(t *testing.T, cfg sim.Config, args ...string) (*sim.Device, string, string, int) {
	t.Helper()
	device := sim.New(cfg)
	var stdout, stderr bytes.Buffer
	code := runClock(args, &stdout, &stderr, device.Open, sim.NewVirtualClock())
	return device, stdout.String(), stderr.String(), code
}

// TestGolden checks stdout, stderr, the exit code and the exact HID traffic.
func TestGolden(t *testing.T) {
	data, err := os.ReadFile("testdata/cli_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []golden
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 300 {
		t.Fatalf("only %d cases", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			cfg, err := sim.ParseConfig(tc.Sim)
			if err != nil {
				t.Fatal(err)
			}
			device, stdout, stderr, code := exec(t, cfg, tc.Argv...)
			if code != tc.Exit {
				t.Errorf("exit = %d, want %d", code, tc.Exit)
			}
			if stdout != tc.Stdout {
				t.Errorf("stdout:\n%q\nwant:\n%q", stdout, tc.Stdout)
			}
			if stderr != tc.Stderr {
				t.Errorf("stderr:\n%q\nwant:\n%q", stderr, tc.Stderr)
			}
			ops, trace := device.TraceSummary()
			if ops != tc.Ops || trace != tc.Trace {
				t.Errorf("traffic differs: %d ops, want %d", ops, tc.Ops)
			}
		})
	}
}

func TestHelpNeverOpensHardware(t *testing.T) {
	open := func(string) (hid.Device, error) {
		t.Fatal("hardware opened")
		return nil, nil
	}
	for _, args := range [][]string{{"help"}, {"--help"}, {"-h"}, {"help", "set"}, {"help", "mode"}, {"help", "disconnect"}, {"disconnect", "-h"}, {"disconnect", "extra"}, {"help", "connect"}, {"reconnect-policy", "-h"}, {"reconnect-policy", "sometimes"}, {"help", "BOGUS"}, {"set", "mode", "bad"}, {}} {
		var stdout, stderr bytes.Buffer
		run(args, &stdout, &stderr, open)
	}
}

func TestVersion(t *testing.T) {
	defer func(saved string) { version = saved }(version)
	open := func(string) (hid.Device, error) {
		t.Fatal("hardware opened")
		return nil, nil
	}
	for value, want := range map[string]string{"dev": "airpods dev\n", "v0.0.2-alpha": "airpods v0.0.2-alpha\n"} {
		version = value
		var stdout, stderr bytes.Buffer
		if code := run([]string{"--version"}, &stdout, &stderr, open); code != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Errorf("version %s: exit %d, stdout %q, stderr %q", value, code, stdout.String(), stderr.String())
		}
	}
}

func TestDeviceClosedOnEveryPath(t *testing.T) {
	cfg := sim.Default()
	cfg.GetFailAt = 2
	for _, args := range [][]string{{"status"}, {"get", "mode"}, {"get", "microphone"}, {"reconnect"}, {"connect"}, {"disconnect"}, {"reconnect-policy"}, {"reconnect-policy", "auto"}} {
		device, _, _, _ := exec(t, cfg, args...)
		if !device.Closed() {
			t.Errorf("%v left the device open", args)
		}
	}
}

func TestMatrixRows(t *testing.T) {
	cfg := sim.Default()
	for _, tc := range []struct {
		name   string
		mutate func(*sim.Config)
		args   []string
		code   int
		stream string
		text   string
	}{
		{"help", nil, []string{"help"}, 0, "out", "Examples:\n  airpods get mode\n"},
		{"help topic", nil, []string{"help", "BOGUS"}, 2, "err", "airpods: error: unknown help topic: BOGUS\n"},
		{"status", nil, []string{"status"}, 0, "out", "AirPods: connected\nSettings: ready\nCodec: AAC\nBattery left: 80%\nBattery right: 70%\nBattery case: unknown\n"},
		{"settings", nil, []string{"settings"}, 0, "out", "mode: anc (report age 1200 ms)\nmicrophone: unknown (not reported; may be unsupported)\n"},
		{"get", nil, []string{"get", "mode"}, 0, "out", "mode: anc (report age 1200 ms)\n"},
		{"get unreported", nil, []string{"get", "microphone"}, 1, "out", "microphone: unknown (not reported; may be unsupported)\n"},
		{"set confirmed", nil, []string{"set", "mode", "off"}, 0, "out", "mode: off (confirmed by AirPods)\n"},
		{"set already", nil, []string{"set", "mode", "anc"}, 0, "out", "mode: anc (already reported, 1200 ms old; no write sent)\n"},
		{"set bad", nil, []string{"set", "tone-volume", "+5"}, 2, "err", "airpods set: error: tone-volume: expected integer 0..100\n"},
		{"mic", func(c *sim.Config) { c.Flags = 0x13 }, []string{"set", "mode", "off"}, 1, "err", "Error: Stop microphone use before changing AirPods settings\n"},
		{"not ready", func(c *sim.Config) { c.Flags = 1 }, []string{"set", "mode", "off"}, 1, "err", "Error: AirPods settings connection is not ready\n"},
		{"rejected", func(c *sim.Config) { c.Fail = map[byte]byte{1: 4} }, []string{"set", "mode", "off"}, 1, "err", "Error: RPC failure: AirPods disconnected\n"},
		{"open failure", func(c *sim.Config) { c.OpenError = "no device" }, []string{"status"}, 1, "err", "Error: no device\n"},
		{"disconnect", nil, []string{"disconnect"}, 0, "out", "disconnect: released; the AirPods stay paired\n"},
		{"disconnect old firmware", func(c *sim.Config) { c.Capabilities &^= dongle.CapRouting }, []string{"disconnect"}, 1, "err", "Error: " + disconnect.ErrUnsupported.Error() + "\n"},
		{"lost", func(c *sim.Config) { c.WriteMode = sim.WriteLost }, []string{"set", "mode", "off"}, 3, "err", "mode: sent, but no fresh matching device report. It may have applied; use get mode before retrying.\n"},
		{"rejected unknown", func(c *sim.Config) { c.Fail = map[byte]byte{1: 5} }, []string{"set", "mode", "off"}, 3, "err", "mode: change not confirmed: RPC failure: device timeout. Read it again before retrying.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := cfg
			if tc.mutate != nil {
				tc.mutate(&c)
			}
			_, stdout, stderr, code := exec(t, c, tc.args...)
			if code != tc.code {
				t.Errorf("exit = %d, want %d (%q %q)", code, tc.code, stdout, stderr)
			}
			got := stdout
			if tc.stream == "err" {
				got = stderr
			}
			if !strings.Contains(got, tc.text) {
				t.Errorf("%s = %q, want %q", tc.stream, got, tc.text)
			}
		})
	}
}

func TestHiddenSettingsInCLI(t *testing.T) {
	cfg := sim.Default()
	cfg.Settings = append(cfg.Settings, sim.Setting{Control: 0x20, Age: 9, Revision: 1, Value: 1})
	for _, tc := range []struct {
		args []string
		code int
		out  string
	}{
		{[]string{"get", "connect-automatically"}, 0, "connect-automatically: on (report age 9 ms)\n"},
		{[]string{"get", "allow-auto-connect"}, 1, "allow-auto-connect: unknown (not reported; may be unsupported)\n"},
		{[]string{"set", "connect-automatically", "on"}, 0, "connect-automatically: on (already reported, 9 ms old; no write sent)\n"},
		{[]string{"set", "connect-automatically", "off"}, 0, "connect-automatically: off (confirmed by AirPods)\n"},
		{[]string{"set", "allow-auto-connect", "off"}, 0, "allow-auto-connect: off (confirmed by AirPods)\n"},
		{[]string{"help", "allow-auto-connect"}, 0, "allow-auto-connect: on, off\nGet: airpods get allow-auto-connect\nSet: airpods set allow-auto-connect VALUE\n"},
	} {
		if _, stdout, stderr, code := exec(t, cfg, tc.args...); code != tc.code || stdout != tc.out || stderr != "" {
			t.Errorf("%v: %d %q %q", tc.args, code, stdout, stderr)
		}
	}
	if _, _, stderr, code := exec(t, cfg, "set", "allow-auto-connect", "maybe"); code != 2 || !strings.Contains(stderr, "allow-auto-connect: expected on, off") {
		t.Errorf("bad value: %d %q", code, stderr)
	}
}

// settingWrites lists the setting writes (kind 1) the dongle received, as control and value.
func settingWrites(device *sim.Device) [][2]uint32 {
	var writes [][2]uint32
	for _, payload := range device.Commits() {
		if len(payload) == 6 && payload[0] == 1 {
			writes = append(writes, [2]uint32{uint32(payload[1]), binary.LittleEndian.Uint32(payload[2:])})
		}
	}
	return writes
}

// TestAllowOffBeforeOff: a dongle that forgot Allow Off refuses Off on AirPods that never report
// it, so allow-off on goes first. Other models get the refusal as it is.
func TestAllowOffBeforeOff(t *testing.T) {
	sent := "allow-off: on (sent first; the dongle forgets it whenever the AirPods connect again)\n"
	refused := "Error: Firmware does not support this request for this device\n"
	for _, tc := range []struct {
		name   string
		number string
		gate   bool
		args   []string
		code   int
		out    string
		err    string
		writes [][2]uint32
	}{
		{"mode", pro3Number, true, []string{"set", "mode", "off"}, 0, sent + "mode: off (confirmed by AirPods)\n", "", [][2]uint32{{0x0d, 1}, {0x34, 1}, {0x0d, 1}}},
		{"hold-modes", pro3Number, true, []string{"set", "hold-modes", "off,anc"}, 0, sent + "hold-modes: off,anc (confirmed by AirPods)\n", "", [][2]uint32{{0x1a, 3}, {0x34, 1}, {0x1a, 3}}},
		{"not forgotten", pro3Number, false, []string{"set", "mode", "off"}, 0, "mode: off (confirmed by AirPods)\n", "", [][2]uint32{{0x0d, 1}}},
		{"hold-modes without off", pro3Number, true, []string{"set", "hold-modes", "anc,transparency"}, 0, "hold-modes: anc,transparency (confirmed by AirPods)\n", "", [][2]uint32{{0x1a, 6}}},
		{"model that reports it", "A3047", true, []string{"set", "mode", "off"}, 1, "", refused, [][2]uint32{{0x0d, 1}}},
		{"unlisted model", "A9999", true, []string{"set", "mode", "off"}, 1, "", refused, [][2]uint32{{0x0d, 1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := sim.NewHearingAirPods(tc.number, pro3Firmware).Config()
			cfg.AllowOffGate = tc.gate
			device, stdout, stderr, code := exec(t, cfg, tc.args...)
			if code != tc.code || stdout != tc.out || stderr != tc.err {
				t.Errorf("got %d %q %q", code, stdout, stderr)
			}
			if got := settingWrites(device); !slices.Equal(got, tc.writes) {
				t.Errorf("writes %x, want %x", got, tc.writes)
			}
		})
	}
}

func TestControlSubcommands(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*sim.Config)
		args   []string
		code   int
		out    string
		err    string
		sent   int
	}{
		{"reconnect", nil, []string{"reconnect"}, 0, "reconnect: accepted by dongle\n", "", 1},
		{"pair", nil, []string{"pair", "--yes"}, 0, "pair: accepted by dongle\n", pairWarning + "\n", 1},
		{"pair-phone", nil, []string{"pair-phone"}, 0, "pair-phone: accepted by dongle\n", "", 1},
		{"with serial", nil, []string{"--serial", "SIM0001", "reconnect"}, 0, "reconnect: accepted by dongle\n", "", 1},
		{"wrong serial", nil, []string{"--serial", "X", "reconnect"}, 1, "", "Error: Expected exactly one matching Pico HID interface (CAFE:4015/4005)\n", 0},
		{"pair busy phone", func(c *sim.Config) { c.Flags = 3 | 8 }, []string{"pair", "--yes"}, 1, "", "Error: Control failure: busy; another companion request or settings session is running, or USB audio, the microphone or a phone is in use\n", 0},
		{"pair busy mic", func(c *sim.Config) { c.Flags = 3 | 16 }, []string{"pair", "--yes"}, 1, "", "Error: Control failure: busy; another companion request or settings session is running, or USB audio, the microphone or a phone is in use\n", 0},
		{"pair busy usb", func(c *sim.Config) { c.Flags = 3 | 64 }, []string{"pair", "--yes"}, 1, "", "Error: Control failure: busy; another companion request or settings session is running, or USB audio, the microphone or a phone is in use\n", 0},
		{"pair busy receipt", func(c *sim.Config) { c.ControlResults = map[byte]byte{2: 2} }, []string{"pair", "--yes"}, 3, "", pairWarning + "\n" + "pair: the dongle answered busy, but it may already have disconnected the AirPods and erased their pairing; check status before retrying.\n", 1},
		{"pair-phone busy usb", func(c *sim.Config) { c.Flags = 3 | 64 }, []string{"pair-phone"}, 1, "", "Error: Control failure: busy; another companion request or settings session is running, or USB audio, the microphone or a phone is in use\n", 0},
		{"pair-phone busy mic", func(c *sim.Config) { c.Flags = 3 | 16 }, []string{"pair-phone"}, 1, "", "Error: Control failure: busy; another companion request or settings session is running, or USB audio, the microphone or a phone is in use\n", 0},
		{"pair-phone with a phone connected", func(c *sim.Config) { c.Flags = 3 | 8 }, []string{"pair-phone"}, 0, "pair-phone: accepted by dongle\n", "", 1},
		{"no capability", func(c *sim.Config) { c.Capabilities = 0 }, []string{"reconnect"}, 1, "", "Error: Control failure: not supported by this dongle\n", 0},
		{"rejected", func(c *sim.Config) { c.ControlResults = map[byte]byte{1: 5} }, []string{"reconnect"}, 1, "", "Error: Control failure: AirPods disconnected\n", 1},
		{"no receipt", func(c *sim.Config) { c.ControlNoReceipt = true }, []string{"reconnect"}, 3, "", "reconnect: sent, but no receipt from the dongle. It may have applied; check status before retrying.\n", 1},
		{"write failed", func(c *sim.Config) { c.SetFailAt = 1; c.FailMessage = "boom" }, []string{"pair-phone"}, 3, "", "pair-phone: not confirmed: boom. It may have applied; check status before retrying.\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := sim.Default()
			if tc.mutate != nil {
				tc.mutate(&cfg)
			}
			device, stdout, stderr, code := exec(t, cfg, tc.args...)
			if code != tc.code || stdout != tc.out || stderr != tc.err {
				t.Errorf("got %d %q %q", code, stdout, stderr)
			}
			if got := len(device.Controls()); got != tc.sent {
				t.Errorf("sent %d control reports, want %d", got, tc.sent)
			}
		})
	}
}

// TestDisconnect covers the disconnect command on firmware without the routing handover. It
// must never send anything to the dongle.
func TestDisconnect(t *testing.T) {
	unsupported := "Error: this dongle firmware has no command to disconnect AirPods while keeping them paired; update it to 0.1.59 or newer, or put the AirPods in their case to disconnect\n"
	noDevice := "Error: Expected exactly one matching Pico HID interface (CAFE:4015/4005)\n"
	for _, tc := range []struct {
		name   string
		mutate func(*sim.Config)
		args   []string
		code   int
		out    string
		err    string
	}{
		{"connected", nil, []string{"disconnect"}, 1, "", unsupported},
		{"already disconnected", func(c *sim.Config) { c.Flags = 0 }, []string{"disconnect"}, 1, "", unsupported},
		{"no capabilities", func(c *sim.Config) { c.Capabilities = 0 }, []string{"disconnect"}, 1, "", unsupported},
		{"with serial", nil, []string{"--serial", "SIM0001", "disconnect"}, 1, "", unsupported},
		{"wrong serial", nil, []string{"--serial", "X", "disconnect"}, 1, "", noDevice},
		{"no dongle", func(c *sim.Config) { c.OpenError = "Expected exactly one matching Pico HID interface (CAFE:4015/4005)" }, []string{"disconnect"}, 1, "", noDevice},
		{"status read fails", func(c *sim.Config) { c.GetFailAt = 1; c.FailMessage = "boom" }, []string{"disconnect"}, 1, "", "Error: boom\n"},
		{"usage", nil, []string{"disconnect", "-h"}, 0, helpTexts["disconnect"].usage + helpTexts["disconnect"].body, ""},
		{"help topic", nil, []string{"help", "disconnect"}, 0, helpTexts["disconnect"].usage + helpTexts["disconnect"].body, ""},
		{"extra argument", nil, []string{"disconnect", "extra"}, 2, "", helpTexts[""].usage + "airpods: error: unrecognized arguments: extra\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := sim.Default()
			cfg.Capabilities &^= dongle.CapRouting // firmware up to 0.1.58
			if tc.mutate != nil {
				tc.mutate(&cfg)
			}
			device, stdout, stderr, code := exec(t, cfg, tc.args...)
			if code != tc.code || stdout != tc.out || stderr != tc.err {
				t.Errorf("got %d %q %q", code, stdout, stderr)
			}
			if got := len(device.Controls()); got != 0 {
				t.Errorf("sent %d control reports", got)
			}
			if ops, _ := device.TraceSummary(); ops > 1 {
				t.Errorf("%d HID operations, want at most the one status read", ops)
			}
		})
	}
}

// handovers counts the routing handover requests (kind 21) the dongle received.
func handovers(device *sim.Device) int {
	count := 0
	for _, payload := range device.Commits() {
		if len(payload) > 0 && payload[0] == 21 {
			count++
		}
	}
	return count
}

// TestDisconnectWithRouting covers the routing handover of firmware 0.1.59 and newer.
func TestDisconnectWithRouting(t *testing.T) {
	declines := releaseHold + "\n"
	stale := make([]byte, 32)
	stale[0], stale[1], stale[2] = 1, 32, 0x63
	binary.LittleEndian.PutUint32(stale[4:], 99) // the sim's own revision is 1
	for _, tc := range []struct {
		name   string
		mutate func(*sim.Config)
		code   int
		out    string
		err    string
		sent   int
	}{
		{"released", nil, 0, "disconnect: released; the AirPods stay paired\n" + declines, "", 1},
		{"not closed yet", func(c *sim.Config) { c.RoutingCloseDelay = 1000 }, 0, "disconnect: release accepted, not yet closed\n" + declines, "", 1},
		{"closes while waiting", func(c *sim.Config) { c.RoutingCloseDelay = 3 }, 0, "disconnect: released; the AirPods stay paired\n" + declines, "", 1},
		{"busy", func(c *sim.Config) { c.RoutingBusy = true }, 1, "", "Error: the dongle cannot release the AirPods right now: the microphone or a call is in use, a takeover or audio connection is pending, audio is recovering, a Game Mode change is in flight, or another app holds the settings lease\n", 1},
		{"microphone", func(c *sim.Config) { c.Flags = 0x13 }, 1, "", "Error: the dongle cannot release the AirPods right now: the microphone or a call is in use, a takeover or audio connection is pending, audio is recovering, a Game Mode change is in flight, or another app holds the settings lease\n", 1},
		{"no pair", func(c *sim.Config) { c.Peer = sim.Hex{0, 0, 0, 0, 0, 0} }, 1, "", "Error: the dongle has no saved AirPods pair to release\n", 1},
		{"intent changed", func(c *sim.Config) { c.Replies = map[byte]sim.Hex{20: stale} }, 1, "", "Error: routing intent changed; read status again. The request was not retried (now owned by the dongle). " + intentChangedHint + " Run airpods status before you disconnect again\n", 1},
		{"not connected sets the hold", func(c *sim.Config) { c.Flags = 0 }, 0, "disconnect: released; the AirPods stay paired\n" + declines, "", 1},
		{"auto policy takeover under way", func(c *sim.Config) { c.Flags = 0; c.ReconnectPolicy = 1; c.RoutingTakeoverDelay = 2 }, 1, "", "Error: routing intent changed; read status again. The request was not retried (now takeover pending). " + intentChangedHint + " Run airpods status before you disconnect again\n", 1},
		{"no receipt", func(c *sim.Config) { c.Fail = map[byte]byte{21: 5} }, 3, "", "release: not confirmed: RPC failure: device timeout. It may have applied; read status before trying again.\n", 1},
		{"routing read fails", func(c *sim.Config) { c.Fail = map[byte]byte{20: 7} }, 1, "", "Error: Firmware does not support this request for this device\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := sim.Default()
			if tc.mutate != nil {
				tc.mutate(&cfg)
			}
			device, stdout, stderr, code := exec(t, cfg, "disconnect")
			if code != tc.code || stdout != tc.out || stderr != tc.err {
				t.Errorf("got %d %q %q", code, stdout, stderr)
			}
			if got := handovers(device); got != tc.sent {
				t.Errorf("sent %d handovers, want %d", got, tc.sent)
			}
			if got := len(device.Controls()); got != 0 {
				t.Errorf("sent %d control reports", got)
			}
			if !device.Closed() {
				t.Error("device left open")
			}
		})
	}
}

// keptOpen lets several commands run against one simulated dongle, as against a real one.
type keptOpen struct{ *sim.Device }

func (keptOpen) Close() error { return nil }

// TestDisconnectThenReconnect: a release keeps the pair, a second disconnect sends nothing, and
// reconnect (opcode 1) ends the release.
func TestDisconnectThenReconnect(t *testing.T) {
	device := sim.New(sim.Default())
	open := func(string) (hid.Device, error) { return keptOpen{device}, nil }
	clock := sim.NewVirtualClock()
	for _, step := range []struct {
		args []string
		out  string
	}{
		{[]string{"disconnect"}, "disconnect: released; the AirPods stay paired\n"},
		{[]string{"status"}, "AirPods: disconnected\n"},
		{[]string{"disconnect"}, "disconnect: released; the AirPods stay paired\n"},
		{[]string{"reconnect"}, "reconnect: accepted by dongle\n"},
		{[]string{"status"}, "AirPods: connected\n"},
	} {
		var stdout, stderr bytes.Buffer
		if code := runClock(step.args, &stdout, &stderr, open, clock); code != 0 || !strings.HasPrefix(stdout.String(), step.out) || stderr.Len() != 0 {
			t.Fatalf("%v: %d %q %q", step.args, code, stdout.String(), stderr.String())
		}
	}
	if got := handovers(device); got != 1 {
		t.Errorf("sent %d handovers, want 1", got)
	}
}

// TestDisconnectThenConnect: connect takes the AirPods back through the handover and waits for
// them, and a second connect sends nothing.
func TestDisconnectThenConnect(t *testing.T) {
	device := sim.New(sim.Default())
	open := func(string) (hid.Device, error) { return keptOpen{device}, nil }
	clock := sim.NewVirtualClock()
	for i, step := range []struct {
		args      []string
		out       string
		handovers int
	}{
		{[]string{"connect"}, "connect: AirPods connected; audio ownership confirmed.\n", 0},
		{[]string{"disconnect"}, "disconnect: released; the AirPods stay paired\n", 1},
		{[]string{"connect"}, "connect: AirPods connected; audio ownership confirmed.\n", 2},
		{[]string{"status"}, "AirPods: connected\n", 2},
		{[]string{"connect"}, "connect: AirPods connected; audio ownership confirmed.\n", 2},
	} {
		var stdout, stderr bytes.Buffer
		if code := runClock(step.args, &stdout, &stderr, open, clock); code != 0 || !strings.HasPrefix(stdout.String(), step.out) || stderr.Len() != 0 {
			t.Fatalf("%d %v: %d %q %q", i, step.args, code, stdout.String(), stderr.String())
		}
		if got := handovers(device); got != step.handovers {
			t.Errorf("%d %v: %d handovers, want %d", i, step.args, got, step.handovers)
		}
	}
	if got := len(device.Controls()); got != 0 {
		t.Errorf("sent %d control reports", got)
	}
}

// TestConnect covers the outcomes of connect, and its plain request on firmware before 0.1.59.
func TestConnect(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mutate    func(*sim.Config)
		code      int
		out       string
		err       string
		handovers int
		controls  int
	}{
		{"already connected", nil, 0, "connect: AirPods connected; audio ownership confirmed.\n", "", 0, 0},
		{"disconnected", func(c *sim.Config) { c.Flags = 0 }, 0, "connect: AirPods connected; audio ownership confirmed.\n", "", 1, 0},
		{"still in progress", func(c *sim.Config) { c.Flags = 0; c.RoutingTakeoverDelay = 1000 }, 0, "connect: takeover accepted, still in progress. Use airpods status to follow it.\n", "", 1, 0},
		{"auto policy attempt under way", func(c *sim.Config) { c.Flags = 0; c.ReconnectPolicy = 1; c.RoutingTakeoverDelay = 2 }, 0, "connect: AirPods connected; audio ownership confirmed.\n", "", 1, 0},
		{"busy", func(c *sim.Config) { c.Flags = 0; c.RoutingBusy = true }, 1, "", "Error: the dongle cannot take the AirPods back right now: the microphone or a call is in use, a takeover or audio change is pending, audio is recovering, a Game Mode change is in flight, or another app holds the settings lease\n", 1, 0},
		{"no pair", func(c *sim.Config) { c.Flags = 0; c.Peer = sim.Hex{0, 0, 0, 0, 0, 0} }, 1, "", "Error: the dongle has no saved AirPods pair to takeover\n", 1, 0},
		{"no receipt", func(c *sim.Config) { c.Flags = 0; c.Fail = map[byte]byte{21: 5} }, 3, "", "", 1, 0},
		{"before 0.1.59", func(c *sim.Config) { c.Capabilities &^= dongle.CapRouting }, 0, "connect: AirPods connection requested; not yet confirmed. Use airpods status to check the connection.\n", "", 0, 1},
		{"before 0.1.59 no receipt", func(c *sim.Config) { c.Capabilities &^= dongle.CapRouting; c.ControlNoReceipt = true }, 3, "", "reconnect: sent, but no receipt from the dongle. It may have applied; check status before retrying.\n", 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := sim.Default()
			if tc.mutate != nil {
				tc.mutate(&cfg)
			}
			device, stdout, stderr, code := exec(t, cfg, "connect")
			if code != tc.code || stdout != tc.out || (tc.err != "" && stderr != tc.err) {
				t.Errorf("got %d %q %q", code, stdout, stderr)
			}
			if got := handovers(device); got != tc.handovers {
				t.Errorf("sent %d handovers, want %d", got, tc.handovers)
			}
			if got := len(device.Controls()); got != tc.controls {
				t.Errorf("sent %d control reports, want %d", got, tc.controls)
			}
		})
	}
}

// policyWrites counts the reconnect policy saves (kind 23 with a value) the dongle received.
func policyWrites(device *sim.Device) int {
	count := 0
	for _, payload := range device.Commits() {
		if len(payload) == 2 && payload[0] == 23 {
			count++
		}
	}
	return count
}

// TestReconnectPolicy reads and saves the policy; a save that may have happened is not retried.
func TestReconnectPolicy(t *testing.T) {
	unavailable := "Error: Reconnect policy unavailable: the dongle firmware is older than 0.1.63, or its saved configuration is unreadable"
	for _, tc := range []struct {
		name   string
		mutate func(*sim.Config)
		args   []string
		code   int
		out    string
		err    string
		writes int
	}{
		{"read", nil, nil, 0, "Reconnect policy: manual\n", "", 0},
		{"read auto", func(c *sim.Config) { c.ReconnectPolicy = 1 }, nil, 0, "Reconnect policy: auto\n", "", 0},
		{"read unsupported", func(c *sim.Config) { c.ReconnectPolicyUnsupported = true }, nil, 1, "", unavailable + "\n", 0},
		{"save auto", nil, []string{"auto"}, 0, "Reconnect policy: auto (saved and verified)\n", autoPolicyWarning + "\n", 1},
		{"save manual", func(c *sim.Config) { c.ReconnectPolicy = 1 }, []string{"manual"}, 0, "Reconnect policy: manual (saved and verified)\n", "", 1},
		{"already manual", nil, []string{"manual"}, 0, "Reconnect policy: manual (no change needed; no write sent)\n", "", 0},
		{"already auto", func(c *sim.Config) { c.ReconnectPolicy = 1 }, []string{"auto"}, 0, "Reconnect policy: auto (no change needed; no write sent)\n", autoPolicyWarning + "\n", 0},
		{"save unsupported", func(c *sim.Config) { c.ReconnectPolicyUnsupported = true }, []string{"auto"}, 1, "", unavailable + "; nothing was saved\n", 1},
		{"save busy", func(c *sim.Config) { c.Flags = 3 | 64 }, []string{"auto"}, 1, "", "Error: The dongle saves the reconnect policy only while idle", 1},
		{"save unverified", func(c *sim.Config) { c.ReconnectPolicyWriteFails = true }, []string{"auto"}, 3, "", "reconnect policy: the dongle could not verify the save, so it may have been saved. Do not retry it automatically", 1},
		{"no typed requests", func(c *sim.Config) { c.Capabilities &^= dongle.CapTypedRPC }, []string{"auto"}, 1, "", "Error: Firmware does not advertise typed settings support\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := sim.Default()
			if tc.mutate != nil {
				tc.mutate(&cfg)
			}
			device, stdout, stderr, code := exec(t, cfg, append([]string{"reconnect-policy"}, tc.args...)...)
			if code != tc.code || stdout != tc.out || !strings.HasPrefix(stderr, tc.err) || (tc.err == "" && stderr != "") {
				t.Errorf("got %d %q %q", code, stdout, stderr)
			}
			if got := policyWrites(device); got != tc.writes {
				t.Errorf("sent %d policy saves, want %d", got, tc.writes)
			}
			if got := handovers(device) + len(device.Controls()); got != 0 {
				t.Errorf("sent %d connection requests", got)
			}
		})
	}
}

// TestExtendedSettings: settings, get and set ask for the extended snapshot, which carries
// en352-protection and gain-swipe, and fall back once to the original on older firmware.
func TestExtendedSettings(t *testing.T) {
	extended := []sim.Setting{{Control: 0x0d, Age: 5, Revision: 1, Value: 2}, {Control: 0x37, Age: 6, Revision: 1, Value: 1}, {Control: 0x2f, Age: 7, Revision: 1, Value: 2}}
	snapshots := func(device *sim.Device) [][]byte {
		var requests [][]byte
		for _, payload := range device.Commits() {
			if len(payload) > 0 && payload[0] == 2 {
				requests = append(requests, payload)
			}
		}
		return requests
	}
	for _, tc := range []struct {
		name    string
		legacy  bool
		args    []string
		code    int
		out     string
		asked   [][]byte
		written [][2]uint32
	}{
		{"get", false, []string{"get", "en352-protection"}, 0, "en352-protection: on (report age 6 ms)\n", [][]byte{{2, 1}}, nil},
		{"get legacy", true, []string{"get", "en352-protection"}, 1, "en352-protection: unknown (not reported; may be unsupported)\n", [][]byte{{2, 1}, {2}}, nil},
		{"get legacy other setting", true, []string{"get", "mode"}, 0, "mode: anc (report age 5 ms)\n", [][]byte{{2, 1}, {2}}, nil},
		{"set", false, []string{"set", "gain-swipe", "on"}, 0, "gain-swipe: on (confirmed by AirPods)\n", nil, [][2]uint32{{0x2f, 1}}},
		{"set same", false, []string{"set", "gain-swipe", "off"}, 0, "gain-swipe: off (already reported, 7 ms old; no write sent)\n", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := sim.Default()
			cfg.Settings = extended
			cfg.LegacySnapshotOnly = tc.legacy
			device, stdout, stderr, code := exec(t, cfg, tc.args...)
			if code != tc.code || stdout != tc.out || stderr != "" {
				t.Errorf("got %d %q %q", code, stdout, stderr)
			}
			if got := snapshots(device); tc.asked != nil && !slices.EqualFunc(got, tc.asked, bytes.Equal) {
				t.Errorf("snapshot requests % x, want % x", got, tc.asked)
			}
			if got := settingWrites(device); !slices.Equal(got, tc.written) {
				t.Errorf("writes %x, want %x", got, tc.written)
			}
		})
	}
	_, stdout, _, code := exec(t, sim.Default(), "settings")
	if code != 0 || !strings.HasSuffix(stdout, "en352-protection: unknown (not reported; may be unsupported)\ngain-swipe: unknown (not reported; may be unsupported)\n") {
		t.Errorf("settings: %d %q", code, stdout)
	}
}

func TestUsageListsDisconnect(t *testing.T) {
	_, stdout, _, _ := exec(t, sim.Default(), "help")
	for _, want := range []string{"reconnect,connect,pair,pair-phone,disconnect,reconnect-policy,firmware,diag}", "    disconnect          disconnect AirPods but keep them paired",
		"    connect             connect the AirPods", "    reconnect-policy    read or save", "  en352-protection: on, off\n  gain-swipe: on, off\n"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help is missing %q", want)
		}
	}
}

// TestRealClockPath runs the wall-clock entry point once, including one short wait.
func TestRealClockPath(t *testing.T) {
	cfg := sim.Default()
	cfg.CommitDelay = 2
	device := sim.New(cfg)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"get", "mode"}, &stdout, &stderr, device.Open); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
}

// fullDisk fails every write, as stdout does with >/dev/full.
type fullDisk struct{}

func (fullDisk) Write([]byte) (int, error) { return 0, syscall.ENOSPC }

// TestStdoutFailure: every command that prints must notice, in text mode as in JSON.
func TestStdoutFailure(t *testing.T) {
	for _, args := range [][]string{
		{"status"}, {"--json", "status"}, {"settings"}, {"get", "mode"}, {"list"}, {"--version"}, {"help"}, {"completion", "bash"}, {"audio"}, {"perf"},
	} {
		device := sim.New(sim.Default())
		var stderr bytes.Buffer
		code := runEnv(args, fullDisk{}, &stderr, environment{device.Open, func() ([]hid.Info, error) { return nil, nil }, sim.NewVirtualClock(), nil})
		if want := "Error: cannot write to stdout: no space left on device\n"; code != 1 || stderr.String() != want {
			t.Errorf("%v: exit %d, stderr %q", args, code, stderr.String())
		}
	}
}

// closedPipe is a stdout whose reader has gone, as with | head -0.
type closedPipe struct{}

func (closedPipe) Write([]byte) (int, error) {
	return 0, &os.PathError{Op: "write", Path: "/dev/stdout", Err: syscall.EPIPE}
}

// TestClosedPipeIsQuiet: a reader that left is no failure to describe, whichever way the command prints.
func TestClosedPipeIsQuiet(t *testing.T) {
	for _, args := range [][]string{{"status"}, {"--json", "status"}, {"settings"}, {"list"}, {"--version"}, {"--help"}, {"help"}, {"completion", "bash"}, {"audio"}} {
		device := sim.New(sim.Default())
		var stderr bytes.Buffer
		code := runEnv(args, closedPipe{}, &stderr, environment{device.Open, func() ([]hid.Info, error) { return nil, nil }, sim.NewVirtualClock(), nil})
		if code != cliutil.ExitInterrupted || stderr.Len() != 0 {
			t.Errorf("%v: exit %d, stderr %q", args, code, stderr.String())
		}
	}
}

// TestStdoutFailureKeepsTheCommandsOwnCode: a code that already says what failed stays; see TestLatchWriter for 3 and 130.
func TestStdoutFailureKeepsTheCommandsOwnCode(t *testing.T) {
	device := sim.New(sim.Default())
	var stderr bytes.Buffer
	code := runEnv([]string{"set", "bogus", "x"}, fullDisk{}, &stderr, environment{device.Open, nil, sim.NewVirtualClock(), nil})
	if code != 2 || strings.Contains(stderr.String(), "no space left") {
		t.Errorf("exit %d, stderr %q", code, stderr.String())
	}
}

func TestFirmwareShowsTheVersionAndChangesNothing(t *testing.T) {
	cfg := sim.Default()
	cfg.Firmware = "0.1.66-experimental-radio0076"
	device, stdout, stderr, code := exec(t, cfg, "firmware")
	if code != 0 || stdout != "Firmware: 0.1.66 (experimental radio0076)\n" || stderr != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	for _, payload := range device.Commits() {
		if len(payload) != 1 || payload[0] != 24 {
			t.Errorf("sent % x", payload)
		}
	}
	_, stdout, stderr, code = exec(t, sim.Default(), "firmware")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "does not report its version") {
		t.Errorf("older firmware: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}
