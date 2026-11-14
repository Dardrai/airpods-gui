package capture

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/internal/device"
)

func rig() (Submit, *sim.Device, *sim.VirtualClock) {
	cfg := sim.Default()
	AddSimReplies(&cfg)
	dev := sim.New(cfg)
	clock := sim.NewVirtualClock()
	client := dongle.NewClient(dev)
	client.SetClock(clock)
	return func(job func(*dongle.Client) error) error { return job(client) }, dev, clock
}

func lines(t *testing.T, output *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var value map[string]any
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		out = append(out, value)
	}
	return out
}

func TestSamplesEverySecondWithWhatWasAskedFor(t *testing.T) {
	submit, dev, clock := rig()
	var output bytes.Buffer
	result := Run(submit, clock, &output, Options{Seconds: 3, Trace: true, Inspect: true, Phone: true}, nil, nil)
	if result.Err != nil || result.Stopped || result.Lines != 3 || result.Warning != "" {
		t.Fatalf("%+v", result)
	}
	for _, line := range lines(t, &output) {
		for _, key := range []string{"time", "performance", "trace", "inspect", "phone"} {
			if _, ok := line[key]; !ok {
				t.Errorf("no %s in %v", key, line)
			}
		}
	}
	if dev.Inspecting() {
		t.Error("inspection left on")
	}
}

func TestPlainCaptureLeavesOutWhatWasNotAskedFor(t *testing.T) {
	submit, dev, clock := rig()
	var output bytes.Buffer
	result := Run(submit, clock, &output, Options{Seconds: 1}, nil, nil)
	if result.Lines != 1 {
		t.Fatalf("%+v", result)
	}
	line := lines(t, &output)[0]
	for _, key := range []string{"trace", "inspect", "phone", "radio"} {
		if _, ok := line[key]; ok {
			t.Errorf("%s in %v", key, line)
		}
	}
	if dev.Inspecting() {
		t.Error("inspection turned on")
	}
}

func TestRadioIsItsOwnCaptureEveryTwoSeconds(t *testing.T) {
	submit, _, clock := rig()
	var output bytes.Buffer
	result := Run(submit, clock, &output, Options{Seconds: 4, Radio: true, Trace: true, Inspect: true, Phone: true}, nil, nil)
	if result.Err != nil || result.Lines != 2 {
		t.Fatalf("%+v", result)
	}
	for _, line := range lines(t, &output) {
		if _, ok := line["radio"]; !ok || len(line) != 2 {
			t.Errorf("%v", line)
		}
	}
}

func TestStopEndsItAndTurnsInspectionOff(t *testing.T) {
	submit, dev, clock := rig()
	stop := make(chan struct{})
	var output bytes.Buffer
	result := Run(submit, clock, &output, Options{Seconds: 1800, Inspect: true}, stop, func(r Result) {
		if r.Lines == 2 {
			close(stop)
		}
	})
	if !result.Stopped || result.Lines != 2 || result.Err != nil {
		t.Fatalf("%+v", result)
	}
	if dev.Inspecting() {
		t.Error("inspection left on")
	}
}

func TestInspectionIsRenewedEveryTenSeconds(t *testing.T) {
	submit, dev, clock := rig()
	var output bytes.Buffer
	if result := Run(submit, clock, &output, Options{Seconds: 25, Inspect: true}, nil, nil); result.Err != nil {
		t.Fatalf("%+v", result)
	}
	starts, stops := 0, 0
	for _, commit := range dev.Commits() {
		if len(commit) == 2 && commit[0] == 14 {
			if commit[1] == 1 {
				starts++
			} else {
				stops++
			}
		}
	}
	if starts != 3 || stops != 1 {
		t.Errorf("starts %d, stops %d, want 3 and 1 as airpods diag", starts, stops)
	}
}

func TestFailedStopWarnsAndKeepsTheLines(t *testing.T) {
	cfg := sim.Default()
	AddSimReplies(&cfg)
	cfg.FailInspectStop = 2
	dev := sim.New(cfg)
	clock := sim.NewVirtualClock()
	client := dongle.NewClient(dev)
	client.SetClock(clock)
	var output bytes.Buffer
	result := Run(func(job func(*dongle.Client) error) error { return job(client) }, clock, &output, Options{Seconds: 1, Inspect: true}, nil, nil)
	if result.Lines != 1 || !strings.Contains(result.Warning, "120 seconds") {
		t.Fatalf("%+v", result)
	}
}

func TestLostDongleEndsItAfterTheLimitWithTheLinesKept(t *testing.T) {
	submit, _, clock := rig()
	jobs := 0
	failing := func(job func(*dongle.Client) error) error {
		if jobs++; jobs > 2 {
			return device.ErrNoDongle
		}
		return submit(job)
	}
	var output bytes.Buffer
	result := Run(failing, clock, &output, Options{Seconds: 60}, nil, nil)
	if !errors.Is(result.Err, device.ErrNoDongle) || result.Lines != 2 || result.Skipped != maxSkipped {
		t.Fatalf("%+v", result)
	}
}

func TestBusyAndMissingDongleSkipSamplesWithAMarker(t *testing.T) {
	submit, _, clock := rig()
	jobs := 0
	flaky := func(job func(*dongle.Client) error) error {
		jobs++
		switch jobs {
		case 2:
			return &dongle.RequestRejected{Code: 2}
		case 3:
			return device.ErrNoDongle
		}
		return submit(job)
	}
	var output bytes.Buffer
	result := Run(flaky, clock, &output, Options{Seconds: 5}, nil, nil)
	if result.Err != nil || result.Lines != 3 || result.Skipped != 2 {
		t.Fatalf("%+v", result)
	}
	all := lines(t, &output)
	if len(all) != 5 {
		t.Fatalf("%d lines", len(all))
	}
	if all[1]["skipped"] != "busy" || all[1]["consecutive"] != 1.0 || all[1]["time"] == nil {
		t.Errorf("%v", all[1])
	}
	if all[2]["skipped"] != "no dongle" || all[2]["consecutive"] != 2.0 {
		t.Errorf("%v", all[2])
	}
	if _, marked := all[3]["skipped"]; marked {
		t.Errorf("a sample is marked: %v", all[3])
	}
}

func TestAGoodSampleStartsTheCountAgain(t *testing.T) {
	submit, _, clock := rig()
	jobs := 0
	flaky := func(job func(*dongle.Client) error) error {
		// Busy all the time but for every maxSkipped'th sample, for longer than the limit.
		if jobs++; jobs%maxSkipped == 0 {
			return submit(job)
		}
		return &dongle.RequestRejected{Code: 2}
	}
	var output bytes.Buffer
	result := Run(flaky, clock, &output, Options{Seconds: maxSkipped * 3}, nil, nil)
	if result.Err != nil || result.Lines != 3 {
		t.Fatalf("%+v", result)
	}
}

func TestOtherFailuresStillEndIt(t *testing.T) {
	submit, _, clock := rig()
	jobs := 0
	broken := errors.New("broken")
	failing := func(job func(*dongle.Client) error) error {
		if jobs++; jobs > 1 {
			return broken
		}
		return submit(job)
	}
	var output bytes.Buffer
	result := Run(failing, clock, &output, Options{Seconds: 5}, nil, nil)
	if !errors.Is(result.Err, broken) || result.Lines != 1 || result.Skipped != 0 {
		t.Fatalf("%+v", result)
	}
}

func TestFullQueueSkipsASample(t *testing.T) {
	submit, _, clock := rig()
	jobs := 0
	busy := func(job func(*dongle.Client) error) error {
		if jobs++; jobs == 1 {
			return device.ErrQueueFull
		}
		return submit(job)
	}
	var output bytes.Buffer
	result := Run(busy, clock, &output, Options{Seconds: 2}, nil, nil)
	if result.Err != nil || result.Lines != 1 {
		t.Fatalf("%+v", result)
	}
}

func TestWriteFailureEndsIt(t *testing.T) {
	submit, _, clock := rig()
	result := Run(submit, clock, failingWriter{}, Options{Seconds: 5}, nil, nil)
	if result.Err == nil || result.Lines != 0 {
		t.Fatalf("%+v", result)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }
