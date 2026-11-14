package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

const (
	weakestRSSI    = -100.0
	strongestRSSI  = -40.0
	freshSignalMs  = 10_000
	startingWindow = 5 * time.Second
)

// finder is the finder's state, kept apart from the widgets so its rules can be tested.
type finder struct {
	snapshot   *dongle.NearbyState
	received   time.Time
	mine       bool // this window started the search
	starting   bool
	startedAt  time.Time
	loadFailed bool
}

func (f *finder) canStart() bool {
	return !f.mine && f.snapshot != nil && f.snapshot.AvailableIdentity && !f.snapshot.Active
}

func (f *finder) age(now time.Time) (int64, bool) {
	if f.snapshot == nil || f.snapshot.LastHeardAgeMs == nil {
		return 0, false
	}
	elapsed := max(0, now.Sub(f.received).Milliseconds())
	return int64(*f.snapshot.LastHeardAgeMs) + elapsed, true
}

func (f *finder) fresh(now time.Time) bool {
	if !f.mine || f.snapshot == nil || !f.snapshot.Active || f.snapshot.Paused || f.snapshot.RSSI == nil {
		return false
	}
	age, ok := f.age(now)
	return ok && age <= freshSignalMs
}

// signal maps RSSI onto the bar. It measures signal, not distance.
func (f *finder) signal(now time.Time) float64 {
	if !f.fresh(now) {
		return 0
	}
	return max(0, min(1, (float64(*f.snapshot.RSSI)-weakestRSSI)/(strongestRSSI-weakestRSSI)))
}

func (f *finder) status(now time.Time) string {
	switch {
	case f.loadFailed || f.snapshot == nil:
		return i18n.T("nearby.unavailable")
	case !f.snapshot.AvailableIdentity:
		return i18n.T("nearby.noIdentity")
	case f.starting:
		return i18n.T("nearby.starting")
	case !f.mine && f.snapshot.Active:
		return i18n.T("nearby.otherApp")
	case !f.mine:
		return i18n.T("nearby.ready")
	case f.snapshot.Paused:
		return i18n.T("nearby.paused")
	}
	if !f.fresh(now) {
		if _, heard := f.age(now); heard {
			return i18n.T("nearby.waitAnother")
		}
		return i18n.T("nearby.waitFirst")
	}
	switch rssi := *f.snapshot.RSSI; {
	case rssi >= -60:
		return i18n.T("nearby.strong")
	case rssi >= -80:
		return i18n.T("nearby.moderate")
	}
	return i18n.T("nearby.weak")
}

func (f *finder) trend(now time.Time) string {
	if !f.fresh(now) {
		return ""
	}
	switch f.snapshot.Trend {
	case "stronger":
		return i18n.T("nearby.stronger")
	case "weaker":
		return i18n.T("nearby.weaker")
	case "steady":
		return i18n.T("nearby.steady")
	}
	return ""
}

func (f *finder) heard(now time.Time) string {
	age, ok := f.age(now)
	switch {
	case !ok:
		return ""
	case age < 1000:
		return i18n.T("nearby.justNow")
	case age < 60_000:
		return fmt.Sprintf(i18n.T("nearby.ageSeconds"), age/1000)
	case age < 3_600_000:
		return fmt.Sprintf(i18n.T("page.ageMinutes"), age/60_000)
	}
	return fmt.Sprintf(i18n.T("page.ageHours"), age/3_600_000)
}

type nearbyTab struct {
	w       *Window
	content fyne.CanvasObject
	state   finder
	now     func() time.Time

	start, end   *widget.Button
	buttons      *fyne.Container
	stateText    *widget.Label
	signal       *widget.ProgressBar
	signalText   string
	trend, heard *widget.Label
	inFlight     bool
}

func newNearbyTab(w *Window) *nearbyTab {
	t := &nearbyTab{w: w, signalText: i18n.T("nearby.noSignal"), now: time.Now}
	t.start = w.button("nearby.start", t.begin)
	t.end = w.button("nearby.stop", t.stop)
	t.stateText = newValue("")
	t.signal = widget.NewProgressBar()
	t.signal.TextFormatter = func() string { return t.signalText }
	t.trend, t.heard = newValue(""), newValue("")
	help := w.small("nearby.help")
	t.end.Hide()
	t.buttons = container.NewHBox(layout.NewSpacer(), t.end, t.start)
	t.content = container.NewVBox(
		tight(help),
		t.signal,
		w.valueRow("nearby.finder", t.stateText),
		w.valueRow("nearby.trend", t.trend),
		w.valueRow("nearby.lastHeard", t.heard),
		t.buttons,
		gap(4),
	)
	t.render()
	w.texts = append(w.texts, func() {
		t.render()
		t.signal.Refresh()
	})
	return t
}

func (t *nearbyTab) available() bool {
	return t.w.state.Present && t.w.state.Status.Has(dongle.CapTypedRPC)
}

func (t *nearbyTab) updateEnabled() {
	setEnabled(t.start, t.available() && t.state.canStart())
	setEnabled(t.end, t.state.mine)
	if t.end.Visible() != t.state.mine {
		showIf(t.end, t.state.mine)
		showIf(t.start, !t.state.mine)
		t.buttons.Refresh()
	}
}

func (t *nearbyTab) render() {
	now := t.now()
	setLabel(t.stateText, t.state.status(now))
	t.signalText = i18n.T("nearby.noSignal")
	if t.state.fresh(now) {
		t.signalText = fmt.Sprintf("%d dBm", *t.state.snapshot.RSSI)
	}
	if value := t.state.signal(now); value != t.signal.Value {
		t.signal.SetValue(value)
	}
	setLabel(t.trend, t.state.trend(now))
	setLabel(t.heard, t.state.heard(now))
	t.updateEnabled()
}

// read checks whether a search is possible, without starting one.
func (t *nearbyTab) read() {
	if t.available() && !t.inFlight {
		t.request((*dongle.Client).NearbySnapshot)
	}
}

func (t *nearbyTab) begin() {
	if !t.state.canStart() {
		return
	}
	t.state.mine, t.state.starting, t.state.startedAt = true, true, t.now()
	t.render()
	t.request((*dongle.Client).NearbyStart)
}

// poll takes one snapshot while this window's search runs.
func (t *nearbyTab) poll() {
	if t.state.starting && t.now().Sub(t.state.startedAt) >= startingWindow {
		t.stop()
		t.w.say(i18n.T("nearby.title") + ": " + i18n.T("nearby.didNotStart"))
		return
	}
	if t.state.mine && !t.inFlight {
		t.request((*dongle.Client).NearbySnapshot)
		return
	}
	if t.state.mine {
		t.render()
	}
}

func (t *nearbyTab) request(call func(*dongle.Client) (dongle.NearbyState, error)) {
	t.inFlight = true
	var state dongle.NearbyState
	t.w.owner.Submit(func(c *dongle.Client) (err error) {
		state, err = call(c)
		return err
	}, func(err error) {
		t.w.do(func() {
			t.inFlight = false
			if err != nil {
				t.state.loadFailed = t.state.snapshot == nil
				if t.state.mine {
					t.w.say(i18n.T("nearby.title") + ": " + device.Explain(err))
					t.state.mine, t.state.starting = false, false
				}
				t.render()
				return
			}
			t.show(state)
		})
	})
}

func (t *nearbyTab) show(state dongle.NearbyState) {
	t.state.snapshot, t.state.received, t.state.loadFailed = &state, t.now(), false
	if state.Active {
		t.state.starting = false
	} else if !t.state.starting {
		t.state.mine = false
	}
	t.render()
}

// stop asks the dongle to stop if this window started the finder. Safe at any time.
func (t *nearbyTab) stop() {
	if !t.state.mine {
		return
	}
	t.state.mine, t.state.starting = false, false
	t.render()
	t.w.owner.Submit(func(c *dongle.Client) error { return c.NearbyStop() }, func(err error) {
		t.w.do(func() {
			if err != nil {
				t.w.say(i18n.T("nearby.title") + ": " + device.Explain(err))
				return
			}
			t.read()
		})
	})
}
