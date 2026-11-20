package ui

import (
	"errors"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/tray"
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

// withAudio is the demo with its audio state replies changed by change.
func withAudio(change func(audio []byte)) sim.Config {
	cfg := sim.Demo()
	audio := append([]byte(nil), cfg.Replies[9]...)
	change(audio)
	cfg.Replies[3], cfg.Replies[4], cfg.Replies[9] = audio, audio, audio
	return cfg
}

// requestsOfKind counts the typed requests of one kind the dongle received.
func requestsOfKind(dev *sim.Device, kind byte) (count int, last []byte) {
	for _, commit := range dev.Commits() {
		if len(commit) > 0 && commit[0] == kind {
			count, last = count+1, commit
		}
	}
	return count, last
}

// Game Mode is on the default page, with no experimental switch, and its switch and the
// experimental one show the same request; the tray hears about it too.
func TestGameModeIsOnTheDefaultPage(t *testing.T) {
	var told []tray.GameMode
	r := newRigWith(t, sim.Demo(), func(o *Options) { o.OnGameMode = func(game tray.GameMode) { told = append(told, game) } })
	r.shown()
	r.main(func() {
		if r.w.Experimental() || !r.onScreen(r.w.audio.gameModeOnPage) || r.w.audio.gameModeOnPage.Disabled() {
			t.Fatal("Game Mode is not usable on the default page")
		}
		if got := r.w.audio.gameModeRow.Hint.Text; got != "Lower audio delay, for games. Now: Off" {
			t.Errorf("hint %q", got)
		}
		if last := told[len(told)-1]; last != (tray.GameMode{Supported: true}) {
			t.Errorf("the tray was told %+v", last)
		}
		test.Tap(r.w.audio.gameModeOnPage)
		if !r.w.audio.gameMode.Checked {
			t.Error("the experimental switch did not follow")
		}
		if !told[len(told)-1].Busy {
			t.Error("the tray row is not held while the request is out")
		}
	})
	r.until(func() bool { return strings.HasPrefix(r.w.banner.Text, "Game mode:") }, "applied")
	if count, last := requestsOfKind(r.dev, 4); count != 1 || last[1] != 1 {
		t.Errorf("%d game mode requests, last %x", count, last)
	}
	r.main(func() {
		// The demo dongle answers with the request still off, and both switches say so.
		if r.w.audio.gameModeOnPage.Checked || r.w.audio.gameMode.Checked || told[len(told)-1].Busy {
			t.Error("the switches or the tray do not follow the reply")
		}
	})
}

// Without Game Mode, or without typed requests, the default page has no Game Mode card.
func TestGameModeStaysOffTheDefaultPageWhereItCannotWork(t *testing.T) {
	// Each rig is stopped before the next starts: two at once would share Fyne's state.
	t.Run("not supported", func(t *testing.T) {
		r := newRig(t, withAudio(func(audio []byte) { audio[15] = 0 }))
		r.shown()
		r.main(func() {
			if r.onScreen(r.w.audio.gameModeCard) {
				t.Error("Game Mode shown although the dongle does not support it")
			}
		})
	})
	t.Run("no typed requests", func(t *testing.T) {
		untyped := sim.Demo()
		untyped.Capabilities = 0x0f
		r := newRig(t, untyped)
		r.main(r.w.Show)
		r.flush()
		r.main(func() {
			if r.onScreen(r.w.audio.gameModeCard) {
				t.Error("Game Mode shown without typed requests")
			}
		})
	})
}

// As the command line tool does, a request the dongle already holds is not written again.
func TestGameModeAlreadyRequestedIsNotWritten(t *testing.T) {
	r := newRig(t, withAudio(func(audio []byte) { audio[12], audio[13] = 1, dongle.GameModeConfirmed }))
	r.shown()
	var notes []string
	r.main(func() {
		if !r.w.audio.gameModeOnPage.Checked || r.w.audio.gameModeRow.Hint.Text != "Lower audio delay, for games. Now: On" {
			t.Errorf("checked %v, hint %q", r.w.audio.gameModeOnPage.Checked, r.w.audio.gameModeRow.Hint.Text)
		}
		r.w.SetGameMode(true, func(title, text string) { notes = append(notes, title+": "+text) })
	})
	r.until(func() bool { return r.w.banner.Text == "Game mode: On" }, "nothing to send")
	if count, _ := requestsOfKind(r.dev, 4); count != 0 || len(notes) != 0 {
		t.Errorf("%d game mode requests, notes %q", count, notes)
	}
}

// A dongle that takes the request and reports it failed is a problem, worth a note from the tray.
func TestGameModeFailedPhaseIsReported(t *testing.T) {
	cfg := sim.Demo()
	failed := append([]byte(nil), cfg.Replies[4]...)
	failed[12], failed[13] = 1, dongle.GameModeFailed
	cfg.Replies[4] = failed
	r := newRig(t, cfg)
	r.shown()
	var notes []string
	r.main(func() { r.w.SetGameMode(true, func(title, text string) { notes = append(notes, title+": "+text) }) })
	r.until(func() bool { return r.w.banner.Text == "Game mode: Failed" }, "failure shown")
	r.main(func() {
		if len(notes) != 1 || notes[0] != "Game mode: Failed" {
			t.Errorf("notes %q", notes)
		}
		if r.w.audio.gameModeRow.Hint.Text != "Lower audio delay, for games. Now: Failed" {
			t.Errorf("hint %q", r.w.audio.gameModeRow.Hint.Text)
		}
	})
}

// The audio state is read again while Game Mode is on its way somewhere, and only then.
func TestGameModeIsFollowedOnlyWhileItChanges(t *testing.T) {
	for _, entry := range []struct {
		phase  byte
		follow bool
	}{{dongle.GameModePending, true}, {dongle.GameModeUnconfirmed, true}, {dongle.GameModeConfirmed, false}, {dongle.GameModeOff, false}} {
		t.Run(dongle.GameModeLabel(entry.phase), func(t *testing.T) {
			r := newRig(t, withAudio(func(audio []byte) { audio[12], audio[13] = 1, entry.phase }))
			r.shown()
			r.flush()
			before, _ := requestsOfKind(r.dev, 9)
			r.main(func() {
				r.w.FollowGameMode() // too soon after the first read
				r.w.audio.lastRead = r.w.audio.lastRead.Add(-gameModeFollowPeriod)
				r.w.FollowGameMode()
			})
			r.flush()
			if after, _ := requestsOfKind(r.dev, 9); (after == before+1) != entry.follow || after > before+1 {
				t.Errorf("%d reads, then %d", before, after)
			}
		})
	}
}
