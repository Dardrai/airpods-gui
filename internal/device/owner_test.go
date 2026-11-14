package device

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/hid"
)

// bench is an owner driven by hand: no goroutine, so every poll is deterministic.
type bench struct {
	owner  *Owner
	dev    *sim.Device
	openFn func() (hid.Device, error)
	states []State
	clock  time.Time
	opens  int
}

func newBench(t *testing.T, cfg sim.Config) *bench {
	t.Helper()
	b := &bench{dev: sim.New(cfg), clock: time.Unix(1000, 0)}
	b.openFn = func() (hid.Device, error) { return b.dev.Open("") }
	b.owner = NewOwner(func() (hid.Device, error) { b.opens++; return b.openFn() }, DefaultOptions(),
		func(st State) { b.states = append(b.states, st) })
	b.owner.now = func() time.Time { return b.clock }
	return b
}

func (b *bench) last() State { return b.states[len(b.states)-1] }

var noDongle = &hid.MatchError{Found: 0, Text: "Expected exactly one"}

// unplugged wraps a device whose reads fail after the cable is pulled.
type unplugged struct {
	hid.Device
	pulled *bool
}

func (u unplugged) Get(report byte) ([hid.ReportSize]byte, error) {
	if *u.pulled {
		return [hid.ReportSize]byte{}, errors.New("device gone")
	}
	return u.Device.Get(report)
}

func TestIdlePollingIsReportTwoOnly(t *testing.T) {
	b := newBench(t, sim.Demo())
	for range 50 {
		b.owner.poll()
		b.clock = b.clock.Add(2 * time.Second)
	}
	status, rpc, sets := b.dev.Counts()
	if status != 50 || rpc != 0 || sets != 0 {
		t.Errorf("status=%d rpc=%d sets=%d", status, rpc, sets)
	}
	if len(b.dev.Commits()) != 0 || len(b.dev.Controls()) != 0 {
		t.Error("idle polling sent a request")
	}
}

func TestStateTransitions(t *testing.T) {
	b := newBench(t, sim.Demo())
	b.owner.poll()
	if st := b.last(); !st.Present || !st.Connected() || !st.Ready() || st.Serial != "SIM0001" {
		t.Fatalf("connected: %+v", st)
	}
	b.dev.Update(func(c *sim.Config) { c.Flags = 0 })
	b.owner.poll()
	if st := b.last(); !st.Present || st.Connected() {
		t.Fatalf("away: %+v", st)
	}
	b.dev.Update(func(c *sim.Config) { c.Flags = 3; c.Battery[0] = 12 })
	b.owner.poll()
	if st := b.last(); !st.Ready() || st.Status.BatteryLeft != 12 {
		t.Fatalf("back: %+v", st)
	}
	count := len(b.states)
	b.owner.poll()
	if len(b.states) != count {
		t.Error("an unchanged report published a state")
	}
}

func TestPollIntervals(t *testing.T) {
	b := newBench(t, sim.Demo())
	if got := b.owner.delay(); got != time.Second {
		t.Errorf("no device: %v", got)
	}
	b.owner.poll()
	if got := b.owner.delay(); got != 2*time.Second {
		t.Errorf("idle: %v", got)
	}
	ran := false
	b.owner.run(queued{job: func(*dongle.Client) error { ran = true; return nil }, command: true})
	if !ran {
		t.Fatal("job did not run")
	}
	if got := b.owner.delay(); got != 500*time.Millisecond {
		t.Errorf("pending: %v", got)
	}
	b.owner.poll()
	if !b.last().Pending {
		t.Error("state does not show pending")
	}
	b.clock = b.clock.Add(11 * time.Second)
	if got := b.owner.delay(); got != 2*time.Second {
		t.Errorf("after window: %v", got)
	}
	b.owner.poll()
	if b.last().Pending {
		t.Error("pending did not clear")
	}
}

func TestPlainJobDoesNotSpeedUpPolling(t *testing.T) {
	b := newBench(t, sim.Demo())
	b.owner.poll()
	b.owner.run(queued{job: func(*dongle.Client) error { return nil }})
	if got := b.owner.delay(); got != 2*time.Second {
		t.Errorf("delay: %v", got)
	}
}

func TestNoDongleIsQuietAndReopens(t *testing.T) {
	b := newBench(t, sim.Demo())
	b.openFn = func() (hid.Device, error) { return nil, noDongle }
	for range 10 {
		b.owner.poll()
	}
	if len(b.states) != 1 {
		t.Fatalf("published %d states while absent", len(b.states))
	}
	if st := b.last(); st.Present || st.Problem != "" {
		t.Fatalf("absent: %+v", st)
	}
	if b.opens != 10 {
		t.Errorf("opened %d times, want one per poll", b.opens)
	}
	b.openFn = func() (hid.Device, error) { return b.dev.Open("") }
	b.owner.poll()
	if !b.last().Present {
		t.Fatal("did not pick up the dongle")
	}
}

func TestUnplugAndReplug(t *testing.T) {
	b := newBench(t, sim.Demo())
	pulled := false
	first := b.dev
	b.openFn = func() (hid.Device, error) { return unplugged{first, &pulled}, nil }
	b.owner.poll()
	if !b.last().Present {
		t.Fatal("open")
	}
	pulled = true
	b.openFn = func() (hid.Device, error) { return nil, noDongle }
	b.owner.poll()
	if st := b.last(); st.Present || st.Problem != "" {
		t.Fatalf("unplugged: %+v", st)
	}
	if !first.Closed() {
		t.Error("the dead handle was not closed")
	}
	replug := sim.New(sim.Demo())
	b.openFn = func() (hid.Device, error) { return replug, nil }
	b.owner.poll()
	if !b.last().Present {
		t.Fatal("replug")
	}
}

func TestGlitchDoesNotFlashNoDongle(t *testing.T) {
	b := newBench(t, sim.Demo())
	pulled := false
	b.openFn = func() (hid.Device, error) { return unplugged{sim.New(sim.Demo()), &pulled}, nil }
	b.owner.poll()
	pulled = true
	b.openFn = func() (hid.Device, error) { pulled = false; return unplugged{sim.New(sim.Demo()), &pulled}, nil }
	b.owner.poll()
	if len(b.states) != 1 || !b.last().Present {
		t.Fatalf("states: %+v", b.states)
	}
}

func TestSeveralDonglesIsAVisibleProblem(t *testing.T) {
	b := newBench(t, sim.Demo())
	b.openFn = func() (hid.Device, error) { return nil, &hid.MatchError{Found: 2, Text: "x"} }
	b.owner.poll()
	if st := b.last(); st.Present || st.Problem == "" {
		t.Fatalf("state: %+v", st)
	}
}

func TestNoPermissionIsAVisibleProblem(t *testing.T) {
	b := newBench(t, sim.Demo())
	const text = "found /dev/hidraw3 but no permission to open it; install the AirPods udev rule; see the docs"
	b.openFn = func() (hid.Device, error) { return nil, errors.New(text) }
	b.owner.poll()
	if st := b.last(); st.Present || st.Problem != text {
		t.Fatalf("state: %+v", st)
	}
}

func TestOtherOpenErrorIsVisible(t *testing.T) {
	b := newBench(t, sim.Demo())
	b.openFn = func() (hid.Device, error) { return nil, errors.New("enumeration failed") }
	b.owner.poll()
	if b.last().Problem != "enumeration failed" {
		t.Fatalf("state: %+v", b.last())
	}
}

func TestJobWithoutDongleFailsWithoutRunning(t *testing.T) {
	b := newBench(t, sim.Demo())
	var got error
	b.owner.run(queued{job: func(*dongle.Client) error { t.Error("ran"); return nil }, done: func(err error) { got = err }})
	if !errors.Is(got, ErrNoDongle) {
		t.Errorf("err: %v", got)
	}
}

func TestExplain(t *testing.T) {
	busy := &dongle.RequestRejected{Code: 2}
	cases := []struct {
		err  error
		want string
	}{
		{nil, ""},
		{busy, BusyText},
		{&dongle.Unconfirmed{Message: "x", Err: busy}, UnconfirmedText},
		{&dongle.RequestRejected{Code: 4}, "RPC failure: AirPods disconnected"},
		{&dongle.Unsupported{Message: "nope"}, "nope"},
		{errors.New("plain"), "plain"},
	}
	for _, c := range cases {
		if got := Explain(c.err); got != c.want {
			t.Errorf("%v: %q want %q", c.err, got, c.want)
		}
	}
}

func TestLiveOwnerSerialisesJobsAndDrainsOnStop(t *testing.T) {
	dev := sim.New(sim.Demo())
	var mu sync.Mutex
	var seen []State
	fast := Options{Idle: 5 * time.Millisecond, Pending: time.Millisecond, PendingWindow: 10 * time.Millisecond, Reopen: time.Millisecond}
	owner := NewOwner(func() (hid.Device, error) { return dev.Open("") }, fast, func(st State) {
		mu.Lock()
		seen = append(seen, st)
		mu.Unlock()
	})
	owner.Start()
	first := make(chan error, 1)
	owner.Submit(func(c *dongle.Client) error { return nil }, func(err error) { first <- err })
	if err := <-first; err != nil {
		t.Fatal(err)
	}

	order := make(chan int, 3)
	release := make(chan struct{})
	owner.Submit(func(*dongle.Client) error { <-release; order <- 1; return nil }, nil)
	owner.Submit(func(*dongle.Client) error { order <- 2; return nil }, nil)
	close(release)
	if a, b := <-order, <-order; a != 1 || b != 2 {
		t.Errorf("order %d %d", a, b)
	}

	finished := make(chan struct{})
	owner.Submit(func(*dongle.Client) error { close(finished); return nil }, nil)
	owner.Stop(5 * time.Second)
	select {
	case <-finished:
	default:
		t.Error("a queued job was dropped on stop")
	}
	if !dev.Closed() {
		t.Error("device left open")
	}
	stopped := make(chan error, 1)
	owner.Submit(func(*dongle.Client) error { t.Error("ran after stop"); return nil }, func(err error) { stopped <- err })
	if err := <-stopped; !errors.Is(err, ErrStopped) {
		t.Errorf("after stop: %v", err)
	}
}

func TestReadBackoffSteps(t *testing.T) {
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second}
	for i, d := range want {
		if got := ReadBackoff(i + 1); got != d {
			t.Errorf("%d: %v", i+1, got)
		}
	}
}

func TestListingFailureLogsOnceAndAgainAfterRecovery(t *testing.T) {
	var failing atomic.Bool
	failing.Store(true)
	listed := make(chan struct{}, 64)
	picker := NewPicker(func() ([]hid.Info, error) {
		defer func() {
			select {
			case listed <- struct{}{}:
			default:
			}
		}()
		if failing.Load() {
			return nil, errors.New("no permission")
		}
		return nil, nil
	}, func(string) (hid.Device, error) { return nil, &hid.MatchError{} }, "", false)
	var mu sync.Mutex
	logged := 0
	owner := NewOwner(picker.Open, Options{Idle: time.Millisecond, Pending: time.Millisecond, PendingWindow: time.Millisecond, Reopen: time.Millisecond}, nil)
	owner.SetPicker(picker, time.Millisecond, nil)
	owner.SetLog(func(string, ...any) {
		mu.Lock()
		defer mu.Unlock()
		logged++
	}, nil)
	owner.Start()
	defer owner.Stop(5 * time.Second)
	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return logged
	}
	wait := func(lists int) {
		for range lists {
			select {
			case <-listed:
			case <-time.After(5 * time.Second):
				t.Fatal("the dongles were not listed")
			}
		}
	}
	wait(5)
	if got := count(); got != 1 {
		t.Errorf("logged %d times", got)
	}
	failing.Store(false)
	wait(5)
	failing.Store(true)
	wait(5)
	if got := count(); got != 2 {
		t.Errorf("logged %d times after coming back", got)
	}
}
