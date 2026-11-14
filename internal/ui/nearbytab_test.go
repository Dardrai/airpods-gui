package ui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
)

func TestFinderRules(t *testing.T) {
	now := time.Unix(1000, 0)
	rssi := func(v int16) *int16 { return &v }
	heard := func(v uint32) *uint32 { return &v }
	f := finder{}
	if f.canStart() || f.status(now) != "Finder not available right now" {
		t.Errorf("no snapshot: %q", f.status(now))
	}
	f.snapshot = &dongle.NearbyState{}
	if f.canStart() || f.status(now) != "Searching isn't available for this pair yet" {
		t.Errorf("no identity: %q", f.status(now))
	}
	f.snapshot = &dongle.NearbyState{AvailableIdentity: true, Active: true}
	if f.canStart() || f.status(now) != "Another app is already searching" {
		t.Errorf("busy: %q", f.status(now))
	}
	f.snapshot = &dongle.NearbyState{AvailableIdentity: true}
	if !f.canStart() || f.status(now) != "Ready to search" {
		t.Errorf("ready: %q", f.status(now))
	}
	f.mine, f.received = true, now
	f.snapshot = &dongle.NearbyState{AvailableIdentity: true, Active: true}
	if f.status(now) != "Waiting for the first signal" {
		t.Errorf("waiting: %q", f.status(now))
	}
	for value, want := range map[int16]string{-60: "Strong signal", -61: "Moderate signal", -80: "Moderate signal", -81: "Weak signal"} {
		f.snapshot = &dongle.NearbyState{AvailableIdentity: true, Active: true, RSSI: rssi(value), LastHeardAgeMs: heard(100), Trend: "steady"}
		if got := f.status(now); got != want {
			t.Errorf("%d: %q", value, got)
		}
	}
	f.snapshot.RSSI = rssi(-40)
	if f.signal(now) != 1 {
		t.Errorf("top of scale %v", f.signal(now))
	}
	f.snapshot.RSSI = rssi(-100)
	if f.signal(now) != 0 {
		t.Errorf("bottom of scale %v", f.signal(now))
	}
	f.snapshot.RSSI = rssi(-70)
	if f.signal(now) != 0.5 || f.trend(now) != "Steady" {
		t.Errorf("middle %v trend %q", f.signal(now), f.trend(now))
	}
	later := now.Add(10 * time.Second)
	if f.fresh(later) || f.status(later) != "Waiting for another signal" || f.signal(later) != 0 || f.trend(later) != "" {
		t.Errorf("old signal: %q %v", f.status(later), f.signal(later))
	}
	if got := f.heard(now.Add(90 * time.Second)); got != "1 min ago" {
		t.Errorf("heard %q", got)
	}
	f.snapshot.Paused = true
	if f.status(now) != "Paused while audio is playing" {
		t.Errorf("paused %q", f.status(now))
	}
}

func TestFinderGivesUpOnAStartThatNeverComes(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.until(func() bool { return r.w.nearby.state.canStart() }, "identity known")
	r.main(func() {
		r.w.nearby.state.mine, r.w.nearby.state.starting = true, true
		r.w.nearby.state.startedAt = time.Now().Add(-6 * time.Second)
		r.w.nearby.poll()
		if r.w.nearby.state.mine {
			t.Error("still waiting")
		}
	})
}

func TestNearbyStartPollStopOnHide(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.until(func() bool { return r.w.nearby.state.canStart() }, "identity known")
	r.main(func() { test.Tap(r.w.nearby.start) })
	r.until(func() bool { return r.w.nearby.state.mine && !r.w.nearby.inFlight && !r.w.nearby.state.starting }, "started")
	r.main(r.w.tick)
	r.until(func() bool { return r.w.nearby.signalText == "-58 dBm" }, "signal")
	r.main(func() {
		if r.w.nearby.trend.Text != "Getting stronger" || r.w.nearby.heard.Text != "Just now" || r.w.nearby.stateText.Text != "Strong signal" {
			t.Errorf("state %q trend %q heard %q", r.w.nearby.stateText.Text, r.w.nearby.trend.Text, r.w.nearby.heard.Text)
		}
	})
	r.main(r.w.Hide)
	stops := func() int {
		count := 0
		for _, commit := range r.dev.Commits() {
			if commit[0] == 8 {
				count++
			}
		}
		return count
	}
	r.until(func() bool { return stops() == 1 }, "stop sent")
	r.main(r.w.Hide)
	r.main(r.w.tick)
	r.flush()
	if stops() != 1 {
		t.Errorf("stops: %d", stops())
	}
}

func TestNearbyStopsOnShutdown(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.until(func() bool { return r.w.nearby.state.canStart() }, "identity known")
	r.main(func() { test.Tap(r.w.nearby.start) })
	r.until(func() bool { return r.w.nearby.state.mine && !r.w.nearby.state.starting }, "started")
	r.main(r.w.Shutdown)
	r.owner.Stop(5 * time.Second)
	last := r.dev.Commits()[len(r.dev.Commits())-1]
	if last[0] != 8 {
		t.Errorf("last request kind %d", last[0])
	}
}

func TestStartIdleNeverStopsFinder(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.main(r.w.Hide)
	r.flush()
	for _, commit := range r.dev.Commits() {
		if commit[0] == 8 {
			t.Error("stop sent for a finder never started")
		}
	}
}
