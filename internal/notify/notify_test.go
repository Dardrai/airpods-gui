package notify

import (
	"testing"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/tray"
)

func connected(left, right byte) device.State {
	return device.State{Present: true, Status: dongle.Status{
		Connected: true, ControlsReady: true, ANC: 2, Peer: [6]byte{1, 2, 3, 4, 5, 6},
		BatteryLeft: left, BatteryRight: right, BatteryCase: 80,
	}}
}

// run feeds a state each second for a duration and collects the notes.
type run struct {
	w   Watcher
	now time.Time
}

func newRun() *run { return &run{now: time.Unix(1000, 0)} }

func (r *run) feed(st device.State, d time.Duration) []Note {
	var notes []Note
	for end := r.now.Add(d); !r.now.After(end); r.now = r.now.Add(time.Second) {
		notes = append(notes, r.w.Observe(st, r.now)...)
	}
	return notes
}

// settled gets past the quiet window at start.
func settled(st device.State) *run {
	r := newRun()
	r.feed(st, settleQuiet+time.Second)
	return r
}

func kinds(notes []Note) []Kind {
	out := []Kind{}
	for _, note := range notes {
		out = append(out, note.Kind)
	}
	return out
}

func expect(t *testing.T, got []Note, want ...Kind) {
	t.Helper()
	k := kinds(got)
	if len(k) != len(want) {
		t.Fatalf("notes %v, want %v", k, want)
	}
	for i := range want {
		if k[i] != want[i] {
			t.Fatalf("notes %v, want %v", k, want)
		}
	}
}

func TestStartupIsSilent(t *testing.T) {
	r := newRun()
	expect(t, r.feed(connected(5, 5), time.Minute))
	r = newRun()
	expect(t, r.feed(device.State{}, 10*time.Second))
	// The AirPods connect late, within the window that follows the dongle's arrival.
	expect(t, r.feed(connected(90, 90), time.Minute))
}

func TestConnectAndDisconnectEachSayOnce(t *testing.T) {
	r := settled(connected(90, 90))
	off := connected(90, 90)
	off.Status.Connected = false
	expect(t, r.feed(off, settleDisconnect+5*time.Second), Disconnected)
	expect(t, r.feed(off, time.Minute))
	expect(t, r.feed(connected(90, 90), settleConnect+5*time.Second), Connected)
	expect(t, r.feed(connected(90, 90), time.Minute))
}

func TestReEnumerationIsSilent(t *testing.T) {
	st := connected(90, 90)
	r := settled(st)
	expect(t, r.feed(device.State{}, 3*time.Second))
	off := st
	off.Status.Connected = false
	off.Status.Generation = 9
	expect(t, r.feed(off, 4*time.Second)) // back, AirPods not yet
	expect(t, r.feed(st, time.Minute))    // and they connect late
}

func TestLongerAbsenceIsADisconnectAndOneOnly(t *testing.T) {
	st := connected(90, 90)
	r := settled(st)
	expect(t, r.feed(device.State{}, time.Minute), Disconnected)
	// Plugging back in opens the quiet window: nothing about that part of the story.
	expect(t, r.feed(st, time.Minute))
}

func TestFlappingSaysNothing(t *testing.T) {
	st := connected(90, 90)
	off := st
	off.Status.Connected = false
	r := settled(st)
	for range 5 {
		expect(t, r.feed(off, 3*time.Second))
		expect(t, r.feed(st, 2*time.Second))
	}
}

func TestBatteryOncePerThresholdPerBud(t *testing.T) {
	r := settled(connected(30, 30))
	expect(t, r.feed(connected(21, 30), time.Minute))
	notes := r.feed(connected(20, 30), time.Minute)
	expect(t, notes, BatteryLow)
	if notes[0].Bud != tray.BatteryLeft || notes[0].Level != 20 {
		t.Errorf("%+v", notes[0])
	}
	expect(t, r.feed(connected(19, 30), time.Minute))
	notes = r.feed(connected(19, 18), time.Minute)
	expect(t, notes, BatteryLow)
	if notes[0].Bud != tray.BatteryRight {
		t.Errorf("%+v", notes[0])
	}
	expect(t, r.feed(connected(10, 18), time.Minute), BatteryLow)
	expect(t, r.feed(connected(4, 18), time.Minute))
}

func TestBatteryJitterAroundAThresholdSaysOnce(t *testing.T) {
	r := settled(connected(30, 30))
	expect(t, r.feed(connected(20, 30), time.Minute), BatteryLow)
	for range 5 {
		expect(t, r.feed(connected(21, 30), 10*time.Second))
		expect(t, r.feed(connected(20, 30), 10*time.Second))
	}
}

func TestBatteryRearmsAfterCharging(t *testing.T) {
	r := settled(connected(30, 30))
	expect(t, r.feed(connected(20, 30), time.Minute), BatteryLow)
	charging := connected(20, 30)
	charging.Status.ChargingMask = 1
	expect(t, r.feed(charging, time.Minute))
	expect(t, r.feed(connected(60, 30), time.Minute))
	expect(t, r.feed(connected(20, 30), time.Minute), BatteryLow)
}

func TestPartialRecoveryRearmsOnlyTheLowerThreshold(t *testing.T) {
	r := settled(connected(30, 30))
	expect(t, r.feed(connected(20, 30), time.Minute), BatteryLow)
	expect(t, r.feed(connected(10, 30), time.Minute), BatteryLow)
	expect(t, r.feed(connected(16, 30), time.Minute))
	expect(t, r.feed(connected(10, 30), time.Minute), BatteryLow)
}

func TestBatteryIgnoresStaleOneShotDipsAndTheCase(t *testing.T) {
	r := settled(connected(30, 30))
	stale := connected(5, 30)
	stale.Status.AgeLeft = 60_000
	expect(t, r.feed(stale, time.Minute))
	expect(t, r.feed(connected(5, 30), 2*time.Second)) // a glitch shorter than the settle time
	expect(t, r.feed(connected(30, 30), time.Minute))
	lowCase := connected(30, 30)
	lowCase.Status.BatteryCase = 3
	expect(t, r.feed(lowCase, time.Minute))
}

func TestReconnectAtALowerLevelStillCounts(t *testing.T) {
	r := settled(connected(40, 40))
	off := connected(40, 40)
	off.Status.Connected = false
	expect(t, r.feed(off, time.Minute), Disconnected)
	notes := r.feed(connected(15, 40), time.Minute)
	expect(t, notes, Connected, BatteryLow)
}

func TestModeChangedOnlyByOthers(t *testing.T) {
	st := connected(90, 90)
	r := settled(st)
	r.feed(st, 2*time.Second)
	ours := st
	ours.Status.ANC = 3
	ours.Pending = true
	expect(t, r.feed(ours, 5*time.Second))
	ours.Pending = false
	expect(t, r.feed(ours, time.Minute))
	theirs := ours
	theirs.Status.ANC = 4
	notes := r.feed(theirs, time.Minute)
	expect(t, notes, ModeChanged)
	if _, body := notes[0].Text(); body != "Adaptive" {
		t.Errorf("%q", body)
	}
}

func TestNoteTexts(t *testing.T) {
	for _, note := range []Note{{Kind: Connected}, {Kind: Disconnected}, {Kind: BatteryLow, Bud: tray.BatteryRight, Level: 9}} {
		if title, body := note.Text(); title == "" || body == "" {
			t.Errorf("%+v: %q %q", note, title, body)
		}
	}
}
