package tray

import (
	"sync"
	"sync/atomic"
	"time"
)

// Answer is what is known about a tray host.
type Answer int32

const (
	// Unknown holds until the first check has answered, which a hung session bus can keep
	// from ever happening.
	Unknown Answer = iota
	Yes
	No
)

// Availability keeps the answer of a slow tray check on hand, so that the main thread can
// ask without waiting on the session bus. Trays come and go, so it asks again every period.
// Until the first answer nothing is known, which is neither a tray nor the lack of one.
type Availability struct {
	check func() bool
	every time.Duration
	// OnChange, when set before Start, is told the first answer and each one that differs
	// from the last. It runs on the checking goroutine and must not block.
	OnChange func(Answer)

	cached   atomic.Int32
	started  atomic.Bool
	start    sync.Once
	stopOnce sync.Once
	again    chan struct{}
	stop     chan struct{}
	done     chan struct{}
}

// NewAvailability wraps check, which may block; nothing runs until Start.
func NewAvailability(check func() bool, every time.Duration) *Availability {
	return &Availability{check: check, every: every, again: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
}

// Answer returns what the last check said, Unknown before the first, and never blocks.
func (a *Availability) Answer() Answer { return Answer(a.cached.Load()) }

// Available reports a tray that is known to be there. It never blocks.
func (a *Availability) Available() bool { return a.Answer() == Yes }

// Recheck asks again soon instead of at the next period, and never blocks.
func (a *Availability) Recheck() {
	select {
	case a.again <- struct{}{}:
	default:
	}
}

// Start begins asking on a goroutine of its own, right away and then every period.
func (a *Availability) Start() {
	a.start.Do(func() {
		a.started.Store(true)
		go a.run()
	})
}

func (a *Availability) run() {
	defer close(a.done)
	ticker := time.NewTicker(a.every)
	defer ticker.Stop()
	for {
		answer := No
		if a.check() {
			answer = Yes
		}
		if Answer(a.cached.Swap(int32(answer))) != answer && a.OnChange != nil {
			a.OnChange(answer)
		}
		select {
		case <-a.stop:
			return
		case <-ticker.C:
		case <-a.again:
		}
	}
}

// Stop ends the asking and waits up to wait for a check still under way.
func (a *Availability) Stop(wait time.Duration) {
	a.stopOnce.Do(func() { close(a.stop) })
	if !a.started.Load() {
		return
	}
	select {
	case <-a.done:
	case <-time.After(wait):
	}
}
