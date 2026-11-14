package ui

import (
	"errors"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/internal/device"
)

func TestAudioFinderAndCountersFollowTheExtendedCapability(t *testing.T) {
	cfg := sim.Demo()
	cfg.Capabilities = 0x0f // requests but not the extended ones
	r := newRig(t, cfg)
	r.main(r.w.Show)
	r.flush()
	r.main(func() {
		if !r.w.nearby.start.Disabled() || !r.w.diag.refresh.Disabled() || !r.w.audio.apply.Disabled() {
			t.Error("disabled without the capability")
		}
		r.w.audio.read()
		r.w.diag.read()
	})
	r.flush()
	if len(r.dev.Commits()) != 0 {
		t.Errorf("sent %x", r.dev.Commits())
	}
}

func TestAudioWorksWithoutAirPodsConnected(t *testing.T) {
	cfg := sim.Demo()
	cfg.Flags = 0
	r := newRig(t, cfg)
	r.main(r.w.Show)
	r.until(func() bool { return r.w.audio.have && r.w.nearby.state.canStart() }, "reads")
	r.main(func() {
		if r.w.audio.apply.Disabled() || r.w.nearby.start.Disabled() {
			t.Error("these do not need a connection")
		}
	})
}

func TestAudioReadApplyAndGameMode(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.main(func() {
		if r.w.audio.current.Text != "AAC, 192 kbps" {
			t.Errorf("current: %q", r.w.audio.current.Text)
		}
		r.w.audio.codec.SetSelected("AAC-ELD")
		r.w.audio.bitrate.SetSelected("256 kbps")
		r.w.audio.variable.SetChecked(true)
		test.Tap(r.w.audio.apply)
	})
	r.until(func() bool { return strings.HasPrefix(r.w.banner.Text, "Codec profile:") }, "applied")
	found := false
	for _, commit := range r.dev.Commits() {
		if commit[0] == 3 && commit[1] == 3 && commit[2] == 1 && commit[3] == 0x00 && commit[4] == 0xe8 && commit[5] == 0x03 {
			found = true
		}
	}
	if !found {
		t.Errorf("profile request not seen: %x", r.dev.Commits())
	}
}

func TestGameModeToggle(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.main(func() {
		if r.w.audio.gameMode.Disabled() {
			t.Fatal("game mode disabled although supported")
		}
		r.w.audio.gameMode.SetChecked(true)
	})
	r.until(func() bool { return strings.HasPrefix(r.w.banner.Text, "Game mode:") }, "applied")
	for _, commit := range r.dev.Commits() {
		if commit[0] == 4 {
			if commit[1] != 1 {
				t.Errorf("game mode request %x", commit)
			}
			return
		}
	}
	t.Error("no game mode request")
}

func TestAudioUncertainFailureDoesNotReplay(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.dev.Update(func(c *sim.Config) { c.Fail = map[byte]byte{3: 5} })
	if got := writeMessage(errors.New("link restarted")); got != device.UnconfirmedText {
		t.Errorf("transport failure: %q", got)
	}
	if got := writeMessage(&dongle.RequestRejected{Code: 2}); got != device.BusyText {
		t.Errorf("busy: %q", got)
	}
	r.main(func() { test.Tap(r.w.audio.apply) })
	r.until(func() bool { return strings.HasPrefix(r.w.banner.Text, "Codec profile:") }, "failure shown")
	count := 0
	for _, commit := range r.dev.Commits() {
		if commit[0] == 3 {
			count++
		}
	}
	if count != 1 {
		t.Errorf("attempts: %d", count)
	}
}
