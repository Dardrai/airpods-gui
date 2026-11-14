package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
)

func TestHardwareSectionFollowsTheSwitchAndTheModel(t *testing.T) {
	r := newRig(t, sim.Demo())
	has := func() bool { return r.w.hardwareRow.object.Visible() }
	r.main(func() {
		if has() {
			t.Error("hidden by default")
		}
		r.w.about.experimental.SetChecked(true)
		if !has() {
			t.Error("the switch shows it")
		}
		if !r.w.hardware.refresh.Disabled() {
			t.Error("no model known yet")
		}
		r.w.SetInfo(dongle.Metadata{Model: "A2564", Firmware: "6A1"})
		if !r.w.hardware.refresh.Disabled() || has() {
			t.Errorf("AirPods 3 must hide the section: %q", r.w.hardware.status.Text)
		}
		r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"})
		if r.w.hardware.refresh.Disabled() || !has() {
			t.Error("Pro 3 can read")
		}
		r.w.about.experimental.SetChecked(false)
		if has() {
			t.Error("the switch hides it")
		}
	})
}

func TestHardwareReadShowsResultsAndWritesNothing(t *testing.T) {
	server := sim.NewAirPodsServer()
	cfg := sim.Demo()
	cfg.Exchange = server.Handle
	r := newRig(t, cfg)
	r.main(func() {
		r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"})
		test.Tap(r.w.hardware.refresh)
	})
	r.until(func() bool { return r.w.hardware.record != nil && !r.w.hardware.busy }, "results")
	r.main(func() {
		if r.w.hardware.left.Text != "Passed" || !strings.Contains(r.w.hardware.right.Text, "Passed") || !strings.Contains(r.w.hardware.right.Text, "speaker") {
			t.Errorf("%q %q", r.w.hardware.left.Text, r.w.hardware.right.Text)
		}
		test.Tap(r.w.hardware.readData)
	})
	r.until(func() bool { return r.w.hardware.testMode.Text == "0" && !r.w.hardware.busy }, "data")
	if server.Writes != 0 {
		t.Error("reading wrote")
	}
	if r.dev.LeaseHeld() {
		t.Error("the lease must be given back")
	}
}

func TestHardwareReadBlockedWhileRecording(t *testing.T) {
	cfg := sim.Demo()
	cfg.Flags = 0x13
	r := newRig(t, cfg)
	r.main(func() {
		r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"})
		if !r.w.hardware.refresh.Disabled() {
			t.Error("microphone in use")
		}
		r.w.hardware.readResults()
	})
	r.flush()
	if len(r.dev.Exchanges()) != 0 {
		t.Error("read anyway")
	}
}

func TestHardwareFailureIsShown(t *testing.T) {
	server := sim.NewAirPodsServer()
	server.Unreachable = true
	cfg := sim.Demo()
	cfg.Exchange = server.Handle
	r := newRig(t, cfg)
	r.main(func() {
		r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"})
		test.Tap(r.w.hardware.refresh)
	})
	r.until(func() bool { return r.w.hardware.status.Text != "" && !r.w.hardware.busy }, "error")
	r.main(func() {
		if r.w.hardware.record != nil {
			t.Error("no fake result")
		}
	})
}
