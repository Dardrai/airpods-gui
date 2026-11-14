package ui

import (
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/internal/device"
)

// rig runs a window against a simulated dongle. The mutex stands in for Fyne's main
// thread: test code and owner callbacks both take it.
type rig struct {
	t     *testing.T
	mu    sync.Mutex
	w     *Window
	dev   *sim.Device
	owner *device.Owner
	quits int
}

func (r *rig) main(f func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f()
}

func newRig(t *testing.T, cfg sim.Config) *rig {
	t.Helper()
	r := startRig(t, cfg, "")
	r.until(func() bool { return r.w.state.Present }, "dongle present")
	return r
}

// startRig does not wait for the dongle, for a sim that has none to find.
func startRig(t *testing.T, cfg sim.Config, themeName string) *rig {
	t.Helper()
	return startRigWith(t, cfg, themeName, nil)
}

// newRigWith is newRig with changed window options.
func newRigWith(t *testing.T, cfg sim.Config, change func(*Options)) *rig {
	t.Helper()
	r := startRigWith(t, cfg, "", change)
	r.until(func() bool { return r.w.state.Present }, "dongle present")
	return r
}

func startRigWith(t *testing.T, cfg sim.Config, themeName string, change func(*Options)) *rig {
	t.Helper()
	r := &rig{t: t, dev: sim.New(cfg)}
	app := test.NewTempApp(t)
	if themeName != "" {
		app.Preferences().SetString(themePreference, themeName)
	}
	ApplyTheme(app)
	options := device.Options{Idle: 5 * time.Millisecond, Pending: 2 * time.Millisecond, PendingWindow: 20 * time.Millisecond, Reopen: 2 * time.Millisecond}
	var w *Window
	r.owner = device.NewOwner(func() (hid.Device, error) {
		if cfg.OpenError == noDongleSim {
			return nil, &hid.MatchError{}
		}
		return r.dev.Open("")
	}, options, func(st device.State) {
		r.main(func() {
			if w != nil {
				w.Update(st)
			}
		})
	})
	r.main(func() {
		options := Options{Fyne: app, Owner: r.owner, Do: r.main, Tick: time.Hour, Quit: func() { r.quits++ }}
		if change != nil {
			change(&options)
		}
		w = New(options)
		r.w = w
	})
	r.owner.Start()
	t.Cleanup(func() {
		r.main(func() { r.w.Shutdown() })
		r.owner.Stop(5 * time.Second)
	})
	return r
}

func (r *rig) until(cond func() bool, what string) {
	r.t.Helper()
	r.untilWithin(5*time.Second, cond, what)
}

func (r *rig) untilWithin(wait time.Duration, cond func() bool, what string) {
	r.t.Helper()
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		ok := false
		r.main(func() { ok = cond() })
		if ok {
			return
		}
		time.Sleep(time.Millisecond)
	}
	r.t.Fatalf("not reached: %s", what)
}

// flushRounds is how many times flush goes through the owner: a job's callback can queue the
// next job (a write its read, a read its waiting read), and the poll after each job can
// publish a state that queues one more.
const flushRounds = 3

// flush is the owner barrier: it returns once every job queued so far has run and its
// callback has finished, so "nothing was sent" can be checked without guessing at a sleep.
// Never call it from inside r.main, since the callbacks need that lock.
func (r *rig) flush() {
	r.t.Helper()
	for range flushRounds {
		done := make(chan struct{})
		r.owner.Submit(func(*dongle.Client) error { return nil }, func(error) { close(done) })
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			r.t.Fatal("the owner did not reach the barrier")
		}
	}
}

// idle reports, on the main thread, that the window has no read in flight or waiting.
func (r *rig) idle() bool {
	w := r.w
	return !w.settings.reading && !w.settings.again && !w.audio.reading && !w.nearby.inFlight && !w.diag.reading
}

// whenIdle runs f on the main thread at a moment when nothing is in flight. Holding the main
// thread from the check through f means no callback can start another request in between.
func (r *rig) whenIdle(f func()) {
	r.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.flush()
		ran := false
		r.main(func() {
			if r.idle() {
				f()
				ran = true
			}
		})
		if ran {
			return
		}
	}
	r.t.Fatal("requests were still in flight")
}

func (r *rig) shown() {
	r.t.Helper()
	r.main(r.w.Show)
	r.until(func() bool { return r.w.settings.byName["mode"].reported && r.w.audio.have }, "first read")
}

// settle lets the ticks pass that keep the rows saying their reports are still arriving.
func (r *rig) settle() {
	r.main(func() {
		s := r.w.settings
		s.sinceLink = max(s.sinceLink, s.eagerUntil) + 1
		s.followArrival()
	})
}

// experiments sets the experimental switch, as the ... panel does.
func (r *rig) experiments(on bool) {
	r.main(func() { r.w.about.experimental.SetChecked(on) })
}

// onScreen reports whether an object and everything that holds it on the page are showing.
func (r *rig) onScreen(target fyne.CanvasObject) bool {
	return showing(r.w.page, target)
}

func showing(root, target fyne.CanvasObject) bool {
	if !root.Visible() {
		return false
	}
	if root == target {
		return true
	}
	var children []fyne.CanvasObject
	switch item := root.(type) {
	case *fyne.Container:
		children = item.Objects
	case fyne.Widget:
		children = test.WidgetRenderer(item).Objects()
	}
	for _, child := range children {
		if showing(child, target) {
			return true
		}
	}
	return false
}

func (r *rig) rowDisabled(name string) bool {
	row := r.w.settings.byName[name]
	switch {
	case row.toggle != nil:
		return row.toggle.Disabled() && row.dash.Disabled()
	case row.choice != nil:
		return row.choice.Disabled()
	case row.segment != nil:
		return row.segment.Disabled()
	case row.slider != nil:
		return row.slider.Disabled()
	case row.chips != nil:
		for _, item := range row.chips {
			if !item.Disabled() {
				return false
			}
		}
		return true
	case row.actions[0] != nil:
		return row.actions[0].Disabled() && row.actions[1].Disabled()
	}
	r.t.Fatalf("no control on %s", name)
	return false
}

// performanceReads counts the diagnostics requests sent so far.
func (r *rig) performanceReads() int {
	count := 0
	for _, commit := range r.dev.Commits() {
		if commit[0] == 5 {
			count++
		}
	}
	return count
}

func writes(cfg sim.Config, mutate func(*sim.Config)) sim.Config {
	mutate(&cfg)
	return cfg
}
