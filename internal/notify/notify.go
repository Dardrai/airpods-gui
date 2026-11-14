// Package notify decides which device changes are worth a system notification. It only
// looks at the status report and the clock, and says nothing until a change has lasted:
// a dongle re-enumerating or AirPods connecting late must not reach the user at all.
package notify

import (
	"fmt"
	"time"

	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
	"github.com/Dardrai/airpods-gui/internal/tray"
)

const (
	// Preference is the name of the persisted on/off value.
	Preference = "notifications.enabled"
	// Default is the value before the user has chosen.
	Default = true
	// ModePreference is the name of the persisted choice for mode change notifications.
	ModePreference = "notifications.mode"
	// ModeDefault leaves those off: pressing the stem is its own feedback.
	ModeDefault = false
)

// Timings. Disconnects wait longer than connects, since a re-enumerating dongle drops
// the AirPods briefly. Start and every dongle arrival open a window in which what is
// seen becomes the starting point without a word.
const (
	settleQuiet      = 20 * time.Second
	settleConnect    = 3 * time.Second
	settleDisconnect = 10 * time.Second
	settleBattery    = 5 * time.Second
	settleMode       = 2 * time.Second
	rearmMargin      = 5
)

// productName is the product's name, which is not translated.
const productName = "AirPods"

// Thresholds are the battery levels that notify, highest first.
var Thresholds = []int{20, 10}

// Note is one notification to show. Its text is worked out when it is shown.
type Note struct {
	Kind  Kind
	Bud   int // tray.BatteryLeft or tray.BatteryRight, for BatteryLow
	Level int // percent, for BatteryLow
	Mode  byte
}

// Kind says what a Note is about.
type Kind int

const (
	Connected Kind = iota
	Disconnected
	BatteryLow
	ModeChanged
)

// Text is the title and body of a note in the current language.
func (n Note) Text() (title, body string) {
	switch n.Kind {
	case Connected:
		return productName, i18n.T("note.connected")
	case Disconnected:
		return productName, i18n.T("note.disconnected")
	case BatteryLow:
		key := "note.lowLeft"
		if n.Bud == tray.BatteryRight {
			key = "note.lowRight"
		}
		return i18n.T("note.lowTitle"), fmt.Sprintf(i18n.T(key), n.Level)
	case ModeChanged:
		for _, mode := range tray.Modes {
			if mode.Value == n.Mode {
				return productName, mode.Label()
			}
		}
	}
	return productName, ""
}

// bud follows one earbud's battery.
type bud struct {
	armed    bool // false until a first reading is in
	notified int  // lowest threshold already announced; 0 when none
	since    time.Time
	waiting  int // threshold waiting to settle; 0 when none
}

// Watcher turns states into notes. It is not safe for concurrent use: the app calls it
// on its main thread.
type Watcher struct {
	started    bool
	quietUntil time.Time

	announced  bool // the connection state last settled on
	present    bool
	changeAt   time.Time
	changing   bool
	haveStable bool

	peer [6]byte
	buds [2]bud

	mode      byte
	modeSince time.Time
	modeNext  byte
}

// Observe takes the current state, or the same one again as time passes, and returns
// what should be shown now.
func (w *Watcher) Observe(st device.State, now time.Time) []Note {
	if !w.started {
		w.started = true
		w.quietUntil = now.Add(settleQuiet)
	}
	if st.Present && !w.present {
		w.quietUntil = now.Add(settleQuiet)
	}
	w.present = st.Present

	var notes []Note
	if note, ok := w.connection(st, now); ok {
		notes = append(notes, note)
	}
	notes = append(notes, w.batteries(st, now)...)
	if note, ok := w.modeChange(st, now); ok {
		notes = append(notes, note)
	}
	return notes
}

func (w *Watcher) connection(st device.State, now time.Time) (Note, bool) {
	connected := st.Connected()
	if !w.haveStable {
		w.haveStable, w.announced = true, connected
		return Note{}, false
	}
	if connected == w.announced {
		w.changing = false
		return Note{}, false
	}
	if !w.changing {
		w.changing, w.changeAt = true, now
	}
	wait := settleConnect
	if !connected {
		wait = settleDisconnect
	}
	if now.Sub(w.changeAt) < wait {
		return Note{}, false
	}
	w.announced, w.changing = connected, false
	if now.Before(w.quietUntil) {
		return Note{}, false
	}
	if connected {
		return Note{Kind: Connected}, true
	}
	return Note{Kind: Disconnected}, true
}

// crossed is the lowest threshold a level is at or below; 0 when it is above them all.
func crossed(level int) int {
	lowest := 0
	for _, threshold := range Thresholds {
		if level <= threshold {
			lowest = threshold
		}
	}
	return lowest
}

func (w *Watcher) batteries(st device.State, now time.Time) []Note {
	if !st.Connected() {
		return nil
	}
	if st.Status.Peer != w.peer {
		w.peer, w.buds = st.Status.Peer, [2]bud{}
	}
	var notes []Note
	for _, component := range []int{tray.BatteryLeft, tray.BatteryRight} {
		reading := tray.ReadBattery(st, component)
		if !reading.Known || !reading.Current {
			continue
		}
		b := &w.buds[component]
		threshold := crossed(reading.Level)
		if !b.armed {
			// What it already was at the first reading is no crossing.
			b.armed, b.notified = true, threshold
			continue
		}
		if reading.Charging {
			b.notified, b.waiting = 0, 0
			continue
		}
		if b.notified != 0 && reading.Level >= b.notified+rearmMargin {
			b.notified = crossed(reading.Level - rearmMargin)
		}
		if threshold == 0 || (b.notified != 0 && threshold >= b.notified) {
			b.waiting = 0
			continue
		}
		if b.waiting != threshold {
			b.waiting, b.since = threshold, now
		}
		if now.Sub(b.since) < settleBattery {
			continue
		}
		b.notified, b.waiting = threshold, 0
		notes = append(notes, Note{Kind: BatteryLow, Bud: component, Level: reading.Level})
	}
	return notes
}

// modeChange reports a mode the app did not just ask for; a command sent through the
// owner sets Pending, so that covers the tray and the window alike.
func (w *Watcher) modeChange(st device.State, now time.Time) (Note, bool) {
	if !st.Connected() {
		w.mode, w.modeNext = 0, 0
		return Note{}, false
	}
	current := st.Status.ANC
	if w.mode == 0 || st.Pending || now.Before(w.quietUntil) {
		w.mode, w.modeNext = current, 0
		return Note{}, false
	}
	if current == w.mode {
		w.modeNext = 0
		return Note{}, false
	}
	if current != w.modeNext {
		w.modeNext, w.modeSince = current, now
	}
	if now.Sub(w.modeSince) < settleMode {
		return Note{}, false
	}
	w.mode, w.modeNext = current, 0
	return Note{Kind: ModeChanged, Mode: current}, true
}
