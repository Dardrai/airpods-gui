package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/hearing"
	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/internal/cliutil"
)

// parityCase is one golden of the commands that came with feature parity. Scenario names a
// simulated pair of AirPods; those carry callbacks, so they cannot be written as JSON. Answer is
// what a person would type at a confirmation, and empty means nobody is there.
type parityCase struct {
	Name     string   `json:"name"`
	Scenario string   `json:"scenario"`
	Argv     []string `json:"argv"`
	Answer   string   `json:"answer,omitempty"`
	Stdout   string   `json:"stdout"`
	Stderr   string   `json:"stderr"`
	Exit     int      `json:"exit"`
	Ops      int      `json:"ops"`
	Trace    string   `json:"trace"`
	Asked    int      `json:"asked,omitempty"`
	Written  int      `json:"written"`
}

const (
	pro3Firmware = "9A348"
	pro3Number   = "A3063"
)

// scenario builds simulated AirPods. A nil pair is a dongle with nothing hearing related on it.
func scenario(t *testing.T, name string) (sim.Config, *sim.HearingAirPods) {
	t.Helper()
	pair := func(number, firmware string) (sim.Config, *sim.HearingAirPods) {
		airpods := sim.NewHearingAirPods(number, firmware)
		return airpods.Config(), airpods
	}
	switch name {
	case "pro3":
		return pair(pro3Number, pro3Firmware)
	case "pro3-any-packet":
		// A dongle that passes on the packets current firmware refuses, for what follows a send.
		cfg, airpods := pair(pro3Number, pro3Firmware)
		cfg.AnyAAP = true
		return cfg, airpods
	case "pro1-any-packet":
		cfg, airpods := pair("A2083", "6F21")
		cfg.AnyAAP = true
		return cfg, airpods
	case "pro3-old-firmware":
		return pair(pro3Number, "6F21")
	case "pro1":
		return pair("A2083", "6F21")
	case "airpods4":
		return pair("A3053", pro3Firmware)
	case "unlisted":
		return pair("A9999", pro3Firmware)
	case "silent":
		cfg, airpods := pair(pro3Number, pro3Firmware)
		airpods.Silent = true
		return cfg, airpods
	case "hearing-on-silent":
		cfg, airpods := pair(pro3Number, pro3Firmware)
		airpods.Slots[2] = sim.CachedReport{Age: 1000, Revision: 1, Packet: []byte{4, 0, 4, 0, 9, 0, 0x2c, 1, 1, 0, 0}}
		airpods.Slots[3] = sim.CachedReport{Age: 1000, Revision: 1, Packet: []byte{4, 0, 4, 0, 9, 0, 0x33, 1, 0, 0, 0}}
		airpods.Silent = true
		return cfg, airpods
	case "silent-until-off":
		// Turning on goes unreported, while turning off again is reported: a recovery that works.
		cfg, airpods := pair(pro3Number, pro3Firmware)
		deliver := cfg.OnAAP
		cfg.OnAAP = func(pdu []byte) {
			turning := len(pdu) == 11 && pdu[6] == 0x2c
			if turning && pdu[8] == 1 {
				airpods.Silent = true
			}
			if turning && pdu[8] == 2 {
				airpods.Silent = false
				airpods.Report(3, []byte{4, 0, 4, 0, 9, 0, 0x33, 2, 0, 0, 0})
			}
			deliver(pdu)
		}
		return cfg, airpods
	case "profile-read-only":
		cfg, airpods := pair(pro3Number, pro3Firmware)
		airpods.Server.Find(sim.UUIDHearing).Props = 0x02
		return cfg, airpods
	case "transparency-on":
		cfg, airpods := pair(pro3Number, pro3Firmware)
		airpods.Server.Find(sim.UUIDTransparency).Value = sim.TransparencyRecord(true)
		return cfg, airpods
	case "refuse-writes", "refuse-writes-hearing-on":
		cfg, airpods := pair(pro3Number, pro3Firmware)
		airpods.Server.RefuseWrites = true
		if name == "refuse-writes-hearing-on" {
			airpods.Slots[2] = sim.CachedReport{Age: 1000, Revision: 1, Packet: []byte{4, 0, 4, 0, 9, 0, 0x2c, 1, 1, 0, 0}}
			airpods.Slots[3] = sim.CachedReport{Age: 1000, Revision: 1, Packet: []byte{4, 0, 4, 0, 9, 0, 0x33, 1, 0, 0, 0}}
		}
		return cfg, airpods
	case "case":
		return checkScenario(t, pro3Number, pro3Firmware, 2)
	case "case-other-firmware":
		return checkScenario(t, pro3Number, "8A100", 2)
	case "case-in-ear":
		return checkScenario(t, pro3Number, pro3Firmware, 0)
	case "case-streaming":
		cfg, airpods := checkScenario(t, pro3Number, pro3Firmware, 2)
		cfg.Flags |= 64
		return cfg, airpods
	case "no-diagnostics":
		cfg, airpods := pair(pro3Number, pro3Firmware)
		attributes := airpods.Server.Attributes[:0]
		for _, attribute := range airpods.Server.Attributes {
			if attribute.UUID != sim.UUIDDiagnostics && attribute.UUID != sim.UUIDTestMode && attribute.UUID != sim.UUIDSensitivity && attribute.UUID != sim.UUIDCurve {
				attributes = append(attributes, attribute)
			}
		}
		airpods.Server.Attributes = attributes
		return cfg, airpods
	case "transparency-short":
		cfg, airpods := pair(pro3Number, pro3Firmware)
		airpods.Server.Find(sim.UUIDTransparency).Value = sim.ShortTransparencyRecord(false)
		return cfg, airpods
	case "no-eq":
		cfg, airpods := pair(pro3Number, pro3Firmware)
		delete(airpods.Slots, dongle.SlotEQ)
		return cfg, airpods
	case "max2":
		return pair("A3454", pro3Firmware)
	case "loud-on":
		cfg, airpods := pair(pro3Number, pro3Firmware)
		airpods.Server.Find(sim.UUIDLoudSound).Value = []byte{1}
		return cfg, airpods
	case "max":
		return pair("A2096", "6F21")
	case "not-enrolled":
		cfg, airpods := pair(pro3Number, pro3Firmware)
		airpods.Slots[2] = sim.CachedReport{Age: 1000, Revision: 1, Packet: []byte{4, 0, 4, 0, 9, 0, 0x2c, 2, 0, 0, 0}}
		return cfg, airpods
	case "unreported":
		cfg, airpods := pair(pro3Number, pro3Firmware)
		delete(airpods.Slots, 2)
		delete(airpods.Slots, 3)
		delete(airpods.Slots, 4)
		return cfg, airpods
	case "hearing-on":
		cfg, airpods := pair(pro3Number, pro3Firmware)
		airpods.Slots[2] = sim.CachedReport{Age: 1000, Revision: 1, Packet: []byte{4, 0, 4, 0, 9, 0, 0x2c, 1, 1, 0, 0}}
		airpods.Slots[3] = sim.CachedReport{Age: 1000, Revision: 1, Packet: []byte{4, 0, 4, 0, 9, 0, 0x33, 1, 0, 0, 0}}
		return cfg, airpods
	case "no-metadata":
		cfg, airpods := pair(pro3Number, pro3Firmware)
		delete(airpods.Slots, 1)
		return cfg, airpods
	case "disconnected":
		cfg, airpods := pair(pro3Number, pro3Firmware)
		cfg.Flags = 0
		return cfg, airpods
	case "microphone":
		cfg, airpods := pair(pro3Number, pro3Firmware)
		cfg.Flags |= 16
		return cfg, airpods
	case "no-rpc":
		cfg, airpods := pair(pro3Number, pro3Firmware)
		cfg.Capabilities &^= dongle.CapRawTransport
		return cfg, airpods
	}
	t.Fatalf("unknown scenario %q", name)
	return sim.Config{}, nil
}

// checkScenario is a pair whose saved results change as soon as the check has been asked for.
func checkScenario(t *testing.T, number, firmware string, ear byte) (sim.Config, *sim.HearingAirPods) {
	t.Helper()
	airpods := sim.NewHearingAirPods(number, firmware)
	airpods.Slots[dongle.SlotEarStatus] = sim.CachedReport{Age: 1000, Revision: 1, Packet: sim.EarPacket(ear, ear)}
	cfg := airpods.Config()
	exchange := cfg.Exchange
	cfg.Exchange = func(pdu []byte) []byte {
		reply := exchange(pdu)
		if request := airpods.Server.Find(sim.UUIDCheck); request != nil && len(pdu) > 3 && pdu[0] == 0x12 && pdu[1] == byte(request.Handle) {
			airpods.Server.Find(sim.UUIDDiagnostics).Value = sim.DiagnosticRecord(1760000000, 1760000300, 0, 0)
		}
		return reply
	}
	return cfg, airpods
}

// aapWrites counts what reached the AirPods that could change them: everything but report requests.
func aapWrites(airpods *sim.HearingAirPods) int {
	count := 0
	for _, packet := range airpods.Received {
		if len(packet) >= 5 && packet[4] == 0x0f {
			continue
		}
		count++
	}
	return count + airpods.Server.Writes
}

// runParity runs one command with a person, or nobody, at the confirmation.
func runParity(t *testing.T, cfg sim.Config, answer string, args ...string) (*sim.Device, string, string, int, int) {
	t.Helper()
	asked := 0
	old := askUser
	defer func() { askUser = old }()
	askUser = func(string) (bool, bool) {
		if answer == "" {
			return false, false
		}
		asked++
		return answer == "y", true
	}
	device := sim.New(cfg)
	var stdout, stderr bytes.Buffer
	code := runEnv(args, &stdout, &stderr, environment{device.Open, func() ([]hid.Info, error) { return nil, nil }, sim.NewVirtualClock(), nil})
	return device, stdout.String(), stderr.String(), code, asked
}

// TestParityGolden checks stdout, stderr, the exit code, the HID traffic and what was written.
func TestParityGolden(t *testing.T) {
	time.Local = time.UTC // The AirPods' stored times and the read time print in local time.
	const path = "testdata/parity_golden.json"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cases []parityCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for i := range cases {
		tc := &cases[i]
		t.Run(tc.Name, func(t *testing.T) {
			cfg, airpods := scenario(t, tc.Scenario)
			device, stdout, stderr, code, asked := runParity(t, cfg, tc.Answer, tc.Argv...)
			ops, trace := device.TraceSummary()
			written := aapWrites(airpods)
			if *update {
				tc.Stdout, tc.Stderr, tc.Exit, tc.Ops, tc.Trace, tc.Asked, tc.Written = stdout, stderr, code, ops, trace, asked, written
				return
			}
			if code != tc.Exit {
				t.Errorf("exit = %d, want %d", code, tc.Exit)
			}
			if stdout != tc.Stdout {
				t.Errorf("stdout:\n%s\nwant:\n%s", stdout, tc.Stdout)
			}
			if stderr != tc.Stderr {
				t.Errorf("stderr:\n%s\nwant:\n%s", stderr, tc.Stderr)
			}
			if ops != tc.Ops || trace != tc.Trace {
				t.Errorf("traffic differs: %d ops, want %d", ops, tc.Ops)
			}
			if asked != tc.Asked {
				t.Errorf("asked %d times, want %d", asked, tc.Asked)
			}
			if written != tc.Written {
				t.Errorf("wrote %d, want %d", written, tc.Written)
			}
		})
	}
	if *update {
		var buffer bytes.Buffer
		encoder := json.NewEncoder(&buffer)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", " ")
		if err := encoder.Encode(cases); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, buffer.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestHearingOnNeverWritesUnconfirmed: no yes means nothing reaches the AirPods.
func TestHearingOnNeverWritesUnconfirmed(t *testing.T) {
	for _, tc := range []struct {
		args   []string
		answer string
	}{
		{[]string{"hearing", "aid", "on"}, ""},
		{[]string{"hearing", "aid", "on"}, "n"},
		{[]string{"hearing", "media-assist", "on"}, ""},
		{[]string{"hearing", "media-assist", "on"}, "n"},
	} {
		cfg, airpods := scenario(t, "pro3")
		_, _, stderr, code, _ := runParity(t, cfg, tc.answer, tc.args...)
		if code != 1 || !strings.Contains(stderr, "Nothing was sent") || !strings.Contains(stderr, "how loud") {
			t.Errorf("%v %q: exit %d, stderr %q", tc.args, tc.answer, code, stderr)
		}
		if written := aapWrites(airpods); written != 0 {
			t.Errorf("%v %q: wrote %d", tc.args, tc.answer, written)
		}
	}
}

// TestHearingUnconfirmedOffer: the run that sent the change holds the Recovery, so it can offer it.
// A no sends nothing more, and a yes sends the off from that same run.
func TestHearingUnconfirmedOffer(t *testing.T) {
	cfg, airpods := scenario(t, "silent")
	_, _, stderr, code, asked := runParity(t, cfg, "n", "hearing", "aid", "on", "--yes")
	if code != 3 || asked != 1 || !strings.Contains(stderr, "hearing recover --off hearing-aid") {
		t.Errorf("exit %d, asked %d, stderr %q", code, asked, stderr)
	}
	declined := aapWrites(airpods)
	if declined == 0 {
		t.Fatal("nothing was sent")
	}
	cfg, airpods = scenario(t, "silent")
	runParity(t, cfg, "y", "hearing", "aid", "on", "--yes")
	if accepted := aapWrites(airpods); accepted <= declined {
		t.Errorf("a yes sent %d packets, a no %d", accepted, declined)
	}
}

// TestProfileApplyWritesOnlyAfterYes: however it goes wrong, no yes writes nothing. A yes writes
// the profile once and leaves Hearing Aid off as it was.
func TestProfileApplyWritesOnlyAfterYes(t *testing.T) {
	args := []string{"hearing", "profile", "apply", "--tone", "0.5"}
	for _, answer := range []string{"", "n"} {
		cfg, airpods := scenario(t, "pro3")
		_, stdout, stderr, code, _ := runParity(t, cfg, answer, args...)
		if code != 1 || aapWrites(airpods) != 0 {
			t.Errorf("%q: exit %d, wrote %d", answer, code, aapWrites(airpods))
		}
		if !strings.Contains(stdout, "Left tone: 0.125 -> 0.5") || !strings.Contains(stderr, "how loud") {
			t.Errorf("%q: stdout %q, stderr %q", answer, stdout, stderr)
		}
	}
	cfg, airpods := scenario(t, "pro3")
	_, stdout, _, code, _ := runParity(t, cfg, "", append(args, "--yes")...)
	if code != 0 || airpods.Server.Writes != 1 || aapWrites(airpods) != 1 || !strings.Contains(stdout, "applied and read back") {
		t.Errorf("exit %d, %d writes, %d in all, stdout %q", code, airpods.Server.Writes, aapWrites(airpods), stdout)
	}
}

// TestProfileGainsAreRejectedNotClamped: an ear outside -1.5 to 1.5 is refused before anything is
// sent: on the command line when both values are given, and after the read when one of them is
// the stored one.
func TestProfileGainsAreRejectedNotClamped(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
		text string
	}{
		{[]string{"--amplification", "1.5", "--balance", "1"}, 2, "puts the ears' gains at left 1, right 2; each must be -1.5 to 1.5"},
		{[]string{"--amplification", "1.4"}, 1, "Error: Nothing was sent: amplification 1.4 with balance -0.5 puts the ears' gains at left 1.65, right 1.15"},
		{[]string{"--balance", "3"}, 0, ""},
	} {
		cfg, airpods := scenario(t, "pro3")
		_, _, stderr, code, _ := runParity(t, cfg, "", append([]string{"hearing", "profile", "apply", "--yes"}, tc.args...)...)
		if code != tc.code || !strings.Contains(stderr, tc.text) {
			t.Errorf("%v: exit %d, stderr %q", tc.args, code, stderr)
		}
		if wrote := airpods.Server.Writes; (code == 0) != (wrote == 1) {
			t.Errorf("%v: %d writes", tc.args, wrote)
		}
	}
}

// TestProfileShowRoundTrip: applying what show printed changes nothing and writes nothing, even
// though the two ears differ and the combined values only summarise them.
func TestProfileShowRoundTrip(t *testing.T) {
	cfg, _ := scenario(t, "pro3")
	_, stdout, _, code, _ := runParity(t, cfg, "", "--json", "hearing", "profile", "show")
	var document struct {
		Profile json.RawMessage `json:"profile"`
	}
	if err := json.Unmarshal([]byte(stdout), &document); err != nil || code != 0 {
		t.Fatalf("exit %d, %v in %s", code, err, stdout)
	}
	path := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(path, document.Profile, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, airpods := scenario(t, "pro3")
	_, stdout, stderr, code, _ := runParity(t, cfg, "y", "hearing", "profile", "apply", "--file", path)
	if code != 0 || airpods.Server.Writes != 0 || aapWrites(airpods) != 0 || !strings.Contains(stdout, "no write sent") {
		t.Errorf("exit %d, %d writes, stdout %q, stderr %q", code, airpods.Server.Writes, stdout, stderr)
	}
}

// TestProfileApplyBadInputOpensNothing: the arguments are checked before any device is touched.
func TestProfileApplyBadInputOpensNothing(t *testing.T) {
	open := func(string) (hid.Device, error) {
		t.Fatal("hardware opened")
		return nil, nil
	}
	for _, args := range [][]string{
		{"--tone", "2"}, {"--amplification", "NaN"}, {"--amplification", "1.6"}, {"--amplification=-1.6"}, {"--balance", "-3.5"},
		{"--balance", "3", "--amplification", "1"}, {"--left", "1,2,3"},
		{"--right", "1,2,3,4,5,6,7,91"}, {"--left", "a,b"}, {"--file", "testdata/profile_misspelt.json"},
		{"--file", "testdata/profile_range.json"}, {"--file", "testdata/no-such-file.json"}, {},
	} {
		var stdout, stderr bytes.Buffer
		code := run(append([]string{"hearing", "profile", "apply", "--yes"}, args...), &stdout, &stderr, open)
		if code != 2 {
			t.Errorf("%v: exit %d, stderr %q", args, code, stderr.String())
		}
	}
}

// TestRenameCheckMatchesCore: the local check must refuse what the library refuses, and no more.
func TestRenameCheckMatchesCore(t *testing.T) {
	long := strings.Repeat("a", hearing.MaxNameBytes)
	for _, name := range []string{"x", "New name", "é", long, long[1:] + "é"[:1], long + "a", " ", "", "\t", "a\nb", "a\x7fb", "\xff", strings.Repeat("é", hearing.MaxNameBytes/2+1)} {
		cfg, _ := scenario(t, "pro3")
		device := sim.New(cfg)
		client := dongle.NewClient(device)
		client.SetClock(sim.NewVirtualClock())
		_, err := hearing.ExperimentalRename(client, name)
		var refused *hearing.Refused
		coreRefuses := errors.As(err, &refused) && strings.HasPrefix(err.Error(), "Use a name")
		if validRename(name) == coreRefuses {
			t.Errorf("%q: local accepts %v, core says %v", name, validRename(name), err)
		}
	}
}

// TestRenameBadNameOpensNothing: a name that cannot be sent is a bad command line.
func TestRenameBadNameOpensNothing(t *testing.T) {
	open := func(string) (hid.Device, error) {
		t.Fatal("hardware opened")
		return nil, nil
	}
	for _, name := range []string{strings.Repeat("a", hearing.MaxNameBytes+1), " ", "a\nb"} {
		var stdout, stderr bytes.Buffer
		code := run([]string{"experimental", "--experimental", "rename", name}, &stdout, &stderr, open)
		if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "airpods experimental rename: error: Use a name of 1 to 64") || strings.Contains(stderr.String(), experimentalWarning) {
			t.Errorf("%q: exit %d, %q, %q", name, code, stdout.String(), stderr.String())
		}
	}
}

// TestProfileReportsEveryBadFlagInOrder: this ranged over a map, so the message varied between runs.
func TestProfileReportsEveryBadFlagInOrder(t *testing.T) {
	want := "airpods hearing profile apply: error: --left: \"a\" is not a number\n--right: \"b\" is not a number\n--amplification: \"c\" is not a number\n--balance: \"d\" is not a number\n--tone: \"e\" is not a number\n"
	for range 50 {
		_, _, stderr, code := exec(t, sim.Default(), "hearing", "profile", "apply", "--tone", "e", "--balance", "d", "--amplification", "c", "--right", "b", "--left", "a")
		if code != 2 || !strings.HasSuffix(stderr, want) {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
	}
}

// TestProfileValueThatReadsAsAFlag: the error names the way out, and that way works.
func TestProfileValueThatReadsAsAFlag(t *testing.T) {
	cfg, _ := scenario(t, "pro3")
	_, _, stderr, code, _ := runParity(t, cfg, "", "hearing", "profile", "apply", "--yes", "--balance", "-1e-1")
	if want := "airpods hearing profile apply: error: argument --balance: expected one argument (use --balance=VALUE if the value starts with -)\n"; code != 2 || !strings.HasSuffix(stderr, want) {
		t.Errorf("space: exit %d, %q", code, stderr)
	}
	_, _, stderr, code, _ = runParity(t, cfg, "", "hearing", "profile", "apply", "--yes", "--balance=-1e-1")
	if strings.Contains(stderr, "expected one argument") || code == 2 {
		t.Errorf("equals: exit %d, %q", code, stderr)
	}
}

// TestHardwareCheckNeedsConfirmation: nothing is requested without the yes or the flag.
func TestHardwareCheckNeedsConfirmation(t *testing.T) {
	for _, answer := range []string{"", "n"} {
		cfg, airpods := scenario(t, "case")
		_, _, stderr, code, _ := runParity(t, cfg, answer, "hardware", "check")
		if code != 1 || airpods.Server.Writes != 0 || !strings.Contains(stderr, "Nothing was sent") {
			t.Errorf("%q: exit %d, %d writes, stderr %q", answer, code, airpods.Server.Writes, stderr)
		}
	}
}

// TestHardwareCheckCtrlC: Ctrl-C ends the waiting, says that the check goes on, and sent only the
// one request. It never waits out the two minutes.
func TestHardwareCheckCtrlC(t *testing.T) {
	airpods := sim.NewHearingAirPods(pro3Number, pro3Firmware)
	airpods.Slots[dongle.SlotEarStatus] = sim.CachedReport{Age: 1000, Revision: 1, Packet: sim.EarPacket(2, 2)}
	device := sim.New(airpods.Config())
	stop := make(chan struct{})
	close(stop)
	asked := 0
	var stdout, stderr bytes.Buffer
	clock := sim.NewVirtualClock()
	started := clock.Now()
	code := runEnv([]string{"hardware", "check", "--buds-in-case"}, &stdout, &stderr, environment{device.Open, nil, clock, func() <-chan struct{} { asked++; return stop }})
	if code != cliutil.ExitInterrupted || asked != 1 || airpods.Server.Writes != 1 || !strings.Contains(stderr.String(), "may still finish") {
		t.Errorf("exit %d, asked %d, %d writes, stderr %q", code, asked, airpods.Server.Writes, stderr.String())
	}
	if waited := clock.Now().Sub(started); waited > 10*time.Second {
		t.Errorf("waited %v after Ctrl-C", waited)
	}
}

// TestExperimentalNeedsItsFlag: without the flag no device is even opened.
func TestExperimentalNeedsItsFlag(t *testing.T) {
	open := func(string) (hid.Device, error) {
		t.Fatal("hardware opened")
		return nil, nil
	}
	for _, args := range [][]string{
		{"transparency", "show"}, {"transparency", "set", "--enabled", "on"}, {"loud-sound-reduction", "get"},
		{"loud-sound-reduction", "set", "on"}, {"eq", "show"}, {"eq", "set", "--low", "1"}, {"rename", "x"},
	} {
		var stdout, stderr bytes.Buffer
		code := run(append([]string{"experimental"}, args...), &stdout, &stderr, open)
		if code != 2 || !strings.Contains(stderr.String(), "may not work through the dongle") {
			t.Errorf("%v: exit %d, stderr %q", args, code, stderr.String())
		}
	}
}

// TestUnconfirmableSendsSaySent: a send without an answer never claims more than sent.
func TestUnconfirmableSendsSaySent(t *testing.T) {
	for _, args := range [][]string{
		{"experimental", "rename", "--experimental", "New name"},
	} {
		cfg, airpods := scenario(t, "pro3-any-packet")
		_, stdout, stderr, code, _ := runParity(t, cfg, "", args...)
		if code != 0 || !strings.Contains(stdout, ": sent") || strings.Contains(strings.ToLower(stdout), "confirmed") {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
		if strings.Count(stderr, "\n") != 1 || !strings.Contains(stderr, "may not work through the dongle") {
			t.Errorf("%v: the warning should be one line: %q", args, stderr)
		}
		if len(airpods.Received) != 1 {
			t.Errorf("%v: sent %d packets", args, len(airpods.Received))
		}
	}
}

// TestRefusedSendsSayNothingWasSent: current firmware refuses a rename, and nothing reaches the AirPods.
func TestRefusedSendsSayNothingWasSent(t *testing.T) {
	for _, args := range [][]string{
		{"experimental", "rename", "--experimental", "New name"},
	} {
		cfg, airpods := scenario(t, "pro3")
		_, stdout, stderr, code, _ := runParity(t, cfg, "", args...)
		if code != 1 || stdout != "" || !strings.HasSuffix(stderr, "its firmware only sends a fixed list of AirPods requests. Nothing was sent.\n") {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
		if len(airpods.Received) != 0 {
			t.Errorf("%v: sent %d packets", args, len(airpods.Received))
		}
	}
}

// TestDebugHoldsNoPayloads: a debug log goes into bug reports, so no address or serial may show.
func TestDebugHoldsNoPayloads(t *testing.T) {
	cfg := sim.Demo()
	_, stdout, stderr, code, _ := runParity(t, cfg, "", "--debug", "info", "--show-serial")
	if code != 0 || !strings.Contains(stderr, "debug: ") || !strings.Contains(stdout, "SIMSERIAL123") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	for _, secret := range []string{"SIMSERIAL123", cfg.Serial, "4C:56", "4c:56", "4c569d"} {
		if strings.Contains(stderr, secret) {
			t.Errorf("the log holds %q: %s", secret, stderr)
		}
	}
	silent := sim.Default()
	if _, _, stderr, _, _ := runParity(t, silent, "", "status"); stderr != "" {
		t.Errorf("no --debug, yet stderr %q", stderr)
	}
}

// TestSimEnvironment: the variable replaces the dongle, and leaves it alone when unset.
func TestSimEnvironment(t *testing.T) {
	base := environment{clock: sim.NewVirtualClock()}
	for _, value := range []string{"", "0"} {
		var note bytes.Buffer
		if env := simEnvironment(value, &note, base); env.open != nil || note.Len() != 0 {
			t.Errorf("%q replaced the dongle, or said so: %q", value, note.String())
		}
	}
	for _, value := range []string{"1", "case"} {
		var stdout, stderr bytes.Buffer
		env := simEnvironment(value, &stderr, base)
		if got := stderr.String(); got != cliutil.SimNote+"\n" {
			t.Errorf("%s: stderr %q, want the one note", value, got)
		}
		if stdout.Len() != 0 {
			t.Errorf("%s: the note reached stdout", value)
		}
		if code := runEnv([]string{"--json", "hearing", "status"}, &stdout, &stderr, env); code != 0 {
			t.Errorf("%s: exit %d, %s", value, code, stderr.String())
		}
		if !json.Valid(stdout.Bytes()) {
			t.Errorf("%s: stdout is not one JSON document: %q", value, stdout.String())
		}
	}
	var stdout, stderr bytes.Buffer
	if code := runEnv([]string{"hardware", "check", "--buds-in-case"}, &stdout, &stderr, simEnvironment("case", &stderr, base)); code == 0 || !strings.Contains(stdout.String(), "check: ") {
		t.Errorf("the case simulation should start a check and then time out: exit %d, %s %s", code, stdout.String(), stderr.String())
	}
}

// TestRecoverWithoutOffReadsOnly: hearing recover alone is a read.
func TestRecoverWithoutOffReadsOnly(t *testing.T) {
	for _, name := range []string{"pro3", "hearing-on", "unreported"} {
		cfg, airpods := scenario(t, name)
		_, stdout, _, code, _ := runParity(t, cfg, "y", "hearing", "recover")
		if written := aapWrites(airpods); code != 0 || written != 0 || stdout == "" {
			t.Errorf("%s: exit %d, wrote %d, stdout %q", name, code, written, stdout)
		}
	}
}

// TestInfoHidesSerialUnlessAsked also covers the JSON document a script would read.
func TestInfoHidesSerialUnlessAsked(t *testing.T) {
	const serial = "SIMSERIAL123"
	for _, args := range [][]string{{"info"}, {"info", "--capabilities"}, {"--json", "info"}, {"--json", "info", "--capabilities"}} {
		cfg, _ := scenario(t, "pro3")
		_, stdout, stderr, code, _ := runParity(t, cfg, "", args...)
		if code != 0 || stderr != "" || strings.Contains(stdout, serial) {
			t.Errorf("%v: exit %d, stderr %q, stdout %s", args, code, stderr, stdout)
		}
	}
	cfg, _ := scenario(t, "pro3")
	_, stdout, _, _, _ := runParity(t, cfg, "", "--json", "info", "--show-serial", "--capabilities")
	var document jsonInfo
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatal(err)
	}
	if document.Serial == nil || *document.Serial != serial || document.SerialHidden || document.Capabilities == nil || !document.Capabilities.HearingAid {
		t.Errorf("document = %+v", document)
	}
}

// TestEQReadModifyConfirm: an EQ change keeps what was not given, goes out once, and counts as
// done only when a new EQ report carries it. Without a report, or when the dongle declines,
// nothing reaches the AirPods.
func TestEQReadModifyConfirm(t *testing.T) {
	set := []string{"experimental", "--experimental", "eq", "set", "--enabled", "on", "--mid", "60"}
	for _, tc := range []struct {
		scenario string
		code     int
		sent     int
		text     string
	}{
		{"pro3", 0, 1, "eq: confirmed by the AirPods\nEQ: on\nLow: 30\nMid: 60\nHigh: 70\n"},
		{"silent", 3, 1, "the AirPods did not report it"},
		{"no-eq", 1, 0, "have not reported their EQ on this connection"},
		{"unlisted", 1, 0, "The dongle did not pass the EQ change on"},
	} {
		cfg, airpods := scenario(t, tc.scenario)
		_, stdout, stderr, code, _ := runParity(t, cfg, "", set...)
		if code != tc.code || !strings.Contains(stdout+stderr, tc.text) {
			t.Errorf("%s: exit %d, stdout %q, stderr %q", tc.scenario, code, stdout, stderr)
		}
		if sent := len(airpods.Received); sent != tc.sent {
			t.Errorf("%s: sent %d packets, want %d", tc.scenario, sent, tc.sent)
		}
		if tc.sent == 1 && !bytes.Equal(airpods.Received[0], sim.EQReport(true, 30, 60, 70)) {
			t.Errorf("%s: sent % x", tc.scenario, airpods.Received[0])
		}
	}
}
