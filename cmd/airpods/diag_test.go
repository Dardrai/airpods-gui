package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/internal/cliutil"
)

type diagGolden struct {
	Name   string          `json:"name"`
	Argv   []string        `json:"argv"`
	Sim    json.RawMessage `json:"sim"`
	Stdout string          `json:"stdout"`
	Stderr string          `json:"stderr"`
	Exit   int             `json:"exit"`
	Ops    int             `json:"ops"`
	Trace  string          `json:"trace"`
	File   *string         `json:"file"`
}

// runDiagArgs runs airpods diag with these arguments, as the separate airpods-diag tool ran them.
func runDiagArgs(args []string, stdout, stderr io.Writer, open cliutil.OpenFunc, clock dongle.Clock, stop <-chan struct{}) int {
	noEnumerate := func() ([]hid.Info, error) { panic("this test would have enumerated the real dongles") }
	interrupt := func() <-chan struct{} { return stop }
	return runEnv(append([]string{"diag"}, args...), stdout, stderr, environment{open, noEnumerate, clock, interrupt})
}

func TestDiagGolden(t *testing.T) {
	data, err := os.ReadFile("testdata/diag_cli_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []diagGolden
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 100 {
		t.Fatalf("only %d cases", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			cfg, err := sim.ParseConfig(tc.Sim)
			if err != nil {
				t.Fatal(err)
			}
			device := sim.New(cfg)
			target := filepath.Join(t.TempDir(), "capture.jsonl")
			args := make([]string, len(tc.Argv))
			for i, a := range tc.Argv {
				args[i] = strings.ReplaceAll(a, "{OUT}", target)
			}
			var stdout, stderr bytes.Buffer
			code := runDiagArgs(args, &stdout, &stderr, device.Open, sim.NewVirtualClock(), nil)
			if code != tc.Exit {
				t.Errorf("exit = %d, want %d", code, tc.Exit)
			}
			if stdout.String() != tc.Stdout {
				t.Errorf("stdout:\n%q\nwant:\n%q", stdout.String(), tc.Stdout)
			}
			if stderr.String() != tc.Stderr {
				t.Errorf("stderr:\n%q\nwant:\n%q", stderr.String(), tc.Stderr)
			}
			ops, trace := device.TraceSummary()
			if ops != tc.Ops || trace != tc.Trace {
				t.Errorf("traffic differs: %d ops, want %d", ops, tc.Ops)
			}
			content, readErr := os.ReadFile(target)
			switch {
			case tc.File == nil && readErr == nil:
				t.Errorf("unexpected output file")
			case tc.File != nil && (readErr != nil || string(content) != *tc.File):
				t.Errorf("file = %q (%v), want %q", content, readErr, *tc.File)
			}
		})
	}
}

func TestDiagVersion(t *testing.T) {
	defer func(saved string) { version = saved }(version)
	device := sim.New(sim.Default())
	for value, want := range map[string]string{"dev": "airpods dev\n", "v0.0.2-alpha": "airpods v0.0.2-alpha\n"} {
		version = value
		var stdout, stderr bytes.Buffer
		code := runDiagArgs([]string{"--version"}, &stdout, &stderr, device.Open, sim.NewVirtualClock(), nil)
		if code != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Errorf("version %s: exit %d, stdout %q, stderr %q", value, code, stdout.String(), stderr.String())
		}
	}
	if ops, _ := device.TraceSummary(); ops != 0 {
		t.Fatalf("hardware touched: %d ops", ops)
	}
}

func TestOutputIsExclusiveCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "taken.jsonl")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	device := sim.New(sim.Default())
	var stdout, stderr bytes.Buffer
	code := runDiagArgs([]string{"--seconds", "1", "--output", path}, &stdout, &stderr, device.Open, sim.NewVirtualClock(), nil)
	if code != 1 || !strings.HasPrefix(stderr.String(), "Error: ") {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	if content, _ := os.ReadFile(path); string(content) != "keep" {
		t.Fatalf("existing file overwritten: %q", content)
	}
	if ops, _ := device.TraceSummary(); ops != 0 {
		t.Fatalf("HID traffic before the output check: %d ops", ops)
	}
	if !device.Closed() {
		t.Fatal("device left open after the output check failed")
	}
}

func TestReplaceOverwritesOnlyWhenAsked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.jsonl")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	device := sim.New(sim.Default())
	code := runDiagArgs([]string{"--seconds", "1", "--output", path, "--replace"}, &stdout, &stderr, device.Open, sim.NewVirtualClock(), nil)
	data, err := os.ReadFile(path)
	if err != nil || code != 0 || strings.Contains(string(data), "old") || len(data) == 0 {
		t.Errorf("exit %d, %v, stderr %q, file %q", code, err, stderr.String(), data)
	}
	code = runDiagArgs([]string{"--seconds", "1", "--replace"}, &stdout, &stderr, device.Open, sim.NewVirtualClock(), nil)
	if code != 2 {
		t.Errorf("--replace without --output: exit %d", code)
	}
}

// TestNoTraceCombines: what the GUI's Audio trace switch does when it is off, with other options on.
func TestNoTraceCombines(t *testing.T) {
	sample := func(args ...string) string {
		var stdout, stderr bytes.Buffer
		device := sim.New(sim.Default())
		if code := runDiagArgs(append([]string{"--seconds", "1"}, args...), &stdout, &stderr, device.Open, sim.NewVirtualClock(), nil); code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, stderr.String())
		}
		return stdout.String()
	}
	if with, without := sample(), sample("--no-trace"); !strings.Contains(with, "trace") || strings.Contains(without, "trace") {
		t.Errorf("with %q, without %q", with, without)
	}
	if got := sample("--no-trace", "--phone"); strings.Contains(got, "trace") || !strings.Contains(got, "phone") {
		t.Errorf("--no-trace --phone: %q", got)
	}
}

func TestOutputNotCreatedWhenOpenFails(t *testing.T) {
	cfg := sim.Default()
	cfg.OpenError = "no device"
	path := filepath.Join(t.TempDir(), "capture.jsonl")
	var stdout, stderr bytes.Buffer
	code := runDiagArgs([]string{"--seconds", "1", "--output", path}, &stdout, &stderr, sim.New(cfg).Open, sim.NewVirtualClock(), nil)
	if code != 1 || stderr.String() != "Error: no device\n" {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("output file left behind: %v", err)
	}
}

func TestInspectLeaseRenewedAndStopped(t *testing.T) {
	cfg := sim.Default()
	cfg.Replies = map[byte]sim.Hex{13: inspectFixture()}
	var stdout, stderr bytes.Buffer
	device := sim.New(cfg)
	if code := runDiagArgs([]string{"--seconds", "25", "--inspect"}, &stdout, &stderr, device.Open, sim.NewVirtualClock(), nil); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	starts, stops := 0, 0
	for _, c := range device.Commits() {
		if len(c) == 2 && c[0] == 14 {
			if c[1] == 1 {
				starts++
			} else {
				stops++
			}
		}
	}
	if starts != 3 || stops != 1 {
		t.Fatalf("lease starts %d stops %d, want 3 and 1", starts, stops)
	}
	if device.Inspecting() {
		t.Fatal("inspection left running")
	}
}

func inspectFixture() []byte {
	data := make([]byte, 400)
	data[0], data[1], data[3] = 1, 3, 1
	return data
}

func TestInterruptStopsInspection(t *testing.T) {
	cfg := sim.Default()
	cfg.Replies = map[byte]sim.Hex{13: inspectFixture()}
	device := sim.New(cfg)
	stop := make(chan struct{})
	close(stop)
	var stdout, stderr bytes.Buffer
	code := runDiagArgs([]string{"--seconds", "60", "--inspect"}, &stdout, &stderr, device.Open, sim.NewVirtualClock(), stop)
	if code != cliutil.ExitInterrupted {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if device.Inspecting() {
		t.Fatal("interrupt left inspection running")
	}
	if !device.Closed() {
		t.Fatal("interrupt left the device open")
	}
}

// brokenPipe is a stdout whose reader went away after the first line.
type brokenPipe struct{ lines int }

func (b *brokenPipe) Write(p []byte) (int, error) {
	if b.lines--; b.lines < 0 {
		return 0, &os.PathError{Op: "write", Path: "|1", Err: syscall.EPIPE}
	}
	return len(p), nil
}

// TestClosedPipeStopsInspection: airpods diag --inspect | head must not leave inspection running.
func TestClosedPipeStopsInspection(t *testing.T) {
	cfg := sim.Default()
	cfg.Replies = map[byte]sim.Hex{13: inspectFixture()}
	device := sim.New(cfg)
	var stderr bytes.Buffer
	code := runDiagArgs([]string{"--seconds", "60", "--inspect"}, &brokenPipe{lines: 1}, &stderr, device.Open, sim.NewVirtualClock(), nil)
	if code != cliutil.ExitInterrupted || stderr.Len() != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	if device.Inspecting() {
		t.Fatal("a closed pipe left inspection running")
	}
	if !device.Closed() {
		t.Fatal("a closed pipe left the device open")
	}
}

// TestFailedStartStopsInspection fails every read and write in turn. A start that went out can
// have taken effect with its reply lost, so inspection is never left running without a warning.
func TestFailedStartStopsInspection(t *testing.T) {
	for _, fault := range []struct {
		name string
		set  func(*sim.Config, int)
	}{
		{"read fails", func(cfg *sim.Config, at int) { cfg.GetFailAt = at }},
		{"session changes", func(cfg *sim.Config, at int) { cfg.GenerationChangeAtGet = at }},
		{"write fails", func(cfg *sim.Config, at int) { cfg.SetFailAt = at }},
	} {
		t.Run(fault.name, func(t *testing.T) {
			started := 0
			for at := 1; at <= 60; at++ {
				cfg := sim.Default()
				cfg.Replies = map[byte]sim.Hex{13: inspectFixture()}
				fault.set(&cfg, at)
				device := sim.New(cfg)
				var stdout, stderr bytes.Buffer
				code := runDiagArgs([]string{"--seconds", "1", "--inspect"}, &stdout, &stderr, device.Open, sim.NewVirtualClock(), nil)
				for _, commit := range device.Commits() {
					if code != 0 && bytes.Equal(commit, []byte{14, 1}) {
						started++
					}
				}
				if device.Inspecting() && !strings.Contains(stderr.String(), "Inspection stop failed") {
					t.Errorf("fault at %d: exit %d left inspection running, stderr %q", at, code, stderr.String())
				}
			}
			if started == 0 {
				t.Error("no failed run got as far as starting inspection")
			}
		})
	}
}

// TestRefusedStartSendsNoStop: a start the dongle refused did not begin, so there is nothing to
// stop, and no second failure to print.
func TestRefusedStartSendsNoStop(t *testing.T) {
	for _, code := range []byte{1, 2, 4, 7} {
		cfg := sim.Default()
		cfg.Fail = map[byte]byte{14: code}
		device := sim.New(cfg)
		var stdout, stderr bytes.Buffer
		if exit := runDiagArgs([]string{"--seconds", "1", "--inspect"}, &stdout, &stderr, device.Open, sim.NewVirtualClock(), nil); exit != 1 {
			t.Errorf("code %d: exit %d", code, exit)
		}
		if commits := device.Commits(); len(commits) != 1 || strings.Contains(stderr.String(), "Inspection stop failed") {
			t.Errorf("code %d: %d requests committed, stderr %q", code, len(commits), stderr.String())
		}
	}
}

// TestHelpIntoClosedPipeIsQuiet: airpods diag --help | head -0 printed a broken pipe error.
func TestHelpIntoClosedPipeIsQuiet(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"--version"}} {
		var stderr bytes.Buffer
		code := runDiagArgs(args, &brokenPipe{}, &stderr, nil, sim.NewVirtualClock(), nil)
		if code != cliutil.ExitInterrupted || stderr.Len() != 0 {
			t.Errorf("%v: exit %d, stderr %q", args, code, stderr.String())
		}
	}
}

// TestStdoutFailureHasOneWording: the capture reports its failed write as --help does.
func TestStdoutFailureHasOneWording(t *testing.T) {
	const want = "Error: cannot write to stdout: no space left on device\n"
	for _, args := range [][]string{{"--help"}, {"--seconds", "1"}} {
		device := sim.New(sim.Default())
		var stderr bytes.Buffer
		if code := runDiagArgs(args, fullDisk{}, &stderr, device.Open, sim.NewVirtualClock(), nil); code != 1 || stderr.String() != want {
			t.Errorf("%v: exit %d, stderr %q", args, code, stderr.String())
		}
	}
}

func TestStopFailureWarnsButKeepsExit(t *testing.T) {
	cfg := sim.Default()
	cfg.Replies = map[byte]sim.Hex{13: inspectFixture()}
	cfg.FailInspectStop = 5
	device := sim.New(cfg)
	var stdout, stderr bytes.Buffer
	code := runDiagArgs([]string{"--seconds", "1", "--inspect"}, &stdout, &stderr, device.Open, sim.NewVirtualClock(), nil)
	want := "Inspection stop failed; firmware expires it after 120 seconds: RPC failure: device timeout\n"
	if code != 0 || stderr.String() != want {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
}

// TestEveryFlagIsInItsHelp: the parser and the hand-kept help text must not drift apart.
func TestEveryDiagFlagIsInItsHelp(t *testing.T) {
	for _, action := range diagParser().Actions {
		for _, flag := range action.Flags {
			word := regexp.MustCompile(`(^|[^-\w])` + regexp.QuoteMeta(flag) + `($|[^-\w])`)
			if !word.MatchString(helpTexts["diag"].body) {
				t.Errorf("%s is not in the help body", flag)
			}
			if flag != "--help" && !word.MatchString(helpTexts["diag"].usage) {
				t.Errorf("%s is not in the usage", flag)
			}
		}
	}
}
