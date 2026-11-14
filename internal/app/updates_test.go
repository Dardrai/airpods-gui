package app

import (
	"context"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/internal/i18n"
	"github.com/Dardrai/airpods-gui/internal/updates"
)

// The check stays offline until it is turned on; then the App tab shows the new version,
// and a newer release is announced once.
func TestUpdateCheckIsOptInAndShowsTheNewVersion(t *testing.T) {
	main := &mainThread{}
	r := &rig{main: main}
	base := test.NewTempApp(t)
	base.Preferences().SetString(i18n.Preference, i18n.English)
	var asked atomic.Int32
	latest, _ := updates.Parse("v9.0.0")
	a := New(Config{
		Fyne: notifyingApp{base, r}, Host: fakeHost{}, Options: fastOptions(), Do: main.do, Notify: noService,
		Open:          func() (hid.Device, error) { return nil, &hid.MatchError{} },
		Version:       "v0.2.1-beta",
		FirmwareFetch: func(context.Context) (updates.Release, error) { return updates.Release{}, updates.ErrNoRelease },
		UpdateFetch: func(context.Context) (updates.Release, error) {
			asked.Add(1)
			return updates.Release{Tag: "v9.0.0", Page: "https://github.com/x", Version: latest}, nil
		},
	})
	r.App = a
	a.Start()
	t.Cleanup(func() {
		main.do(a.win.Shutdown)
		a.stopBackground()
		a.owner.Stop(shutdownWait)
	})
	r.flush(t)
	if asked.Load() != 0 {
		t.Fatal("asked GitHub while the check is off")
	}
	// What the App tab's switch does.
	main.do(func() { a.setUpdatesEnabled(true) })
	r.eventually(t, func() bool { return a.win.UpdateStatus() == "Version 9.0.0 is available." })
	if asked.Load() != 1 {
		t.Errorf("asked %d times", asked.Load())
	}
	// The note goes out after the result; it arrives on its own time.
	const note = "Version 9.0.0 is available. Open the app to update."
	r.eventually(t, func() bool {
		for _, seen := range r.seen {
			if seen == note {
				return true
			}
		}
		return false
	})
	var notes []string
	r.main.do(func() { notes = append(notes, r.seen...) })
	if len(notes) != 1 || notes[0] != note {
		t.Errorf("notes %q", notes)
	}
}

// The dongle's own firmware version shows in its row; with the check on, a newer release
// is named there and in About, and announced once.
func TestDongleFirmwareAndItsUpdate(t *testing.T) {
	main := &mainThread{}
	r := &rig{main: main}
	base := test.NewTempApp(t)
	base.Preferences().SetString(i18n.Preference, i18n.English)
	cfg := sim.Default()
	cfg.Firmware = "0.1.66-experimental-radio0076"
	dev := sim.New(cfg)
	latest, _ := updates.Parse("v0.1.70-0123456789ab")
	a := New(Config{
		Fyne: notifyingApp{base, r}, Host: fakeHost{}, Options: fastOptions(), Do: main.do, Notify: noService,
		Open:        func() (hid.Device, error) { return dev.Open("") },
		UpdateFetch: func(context.Context) (updates.Release, error) { return updates.Release{}, updates.ErrNoRelease },
		FirmwareFetch: func(context.Context) (updates.Release, error) {
			return updates.Release{Tag: "v0.1.70-0123456789ab", Page: "https://github.com/f", Version: latest}, nil
		},
	})
	r.App = a
	a.Start()
	t.Cleanup(func() {
		main.do(a.win.Shutdown)
		a.stopBackground()
		a.owner.Stop(shutdownWait)
	})
	r.eventually(t, func() bool { return a.win.FirmwareStatus() == "0.1.66" })
	main.do(func() { a.setUpdatesEnabled(true) })
	r.eventually(t, func() bool { return a.win.FirmwareStatus() == "0.1.66 · 0.1.70 available" })
	const note = "Firmware 0.1.70 is available for the dongle."
	r.eventually(t, func() bool {
		for _, seen := range r.seen {
			if seen == note {
				return true
			}
		}
		return false
	})
}
