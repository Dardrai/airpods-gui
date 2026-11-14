package device

import (
	"errors"
	"sync"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/hid"
)

// Errors a job's done callback can receive without the job having run.
var (
	ErrNoDongle  = errors.New("no dongle")
	ErrStopped   = errors.New("the app is shutting down")
	ErrQueueFull = errors.New("too many actions are waiting, try again")
)

// OpenFunc opens the dongle.
type OpenFunc func() (hid.Device, error)

// Job runs on the owner goroutine with exclusive use of the device.
type Job func(*dongle.Client) error

// Options are the owner's timings.
type Options struct {
	Idle          time.Duration // status poll period
	Pending       time.Duration // status poll period after a command
	PendingWindow time.Duration // how long a command counts as pending
	Reopen        time.Duration // open retry period while there is no dongle
}

// DefaultOptions are the production timings.
func DefaultOptions() Options {
	return Options{Idle: 2 * time.Second, Pending: 500 * time.Millisecond, PendingWindow: 10 * time.Second, Reopen: time.Second}
}

type queued struct {
	job     Job
	done    func(error)
	command bool
}

// Owner is the only code that touches the device. Idle polling reads status report 2
// and nothing else, so another client can use the shared report 3 channel alongside it.
// Report 3 traffic only comes from submitted jobs, and no job is ever retried. A job holds
// the goroutine until it returns, so a long wait must be split into short jobs. While idle,
// the one source of report 3 traffic is the auto-pause job, when that option is on; every
// other job follows a user action or the visible window.
type Owner struct {
	open    OpenFunc
	options Options
	onState func(State)
	now     func() time.Time
	// logf and protect are nil until set; see SetLog.
	logf    func(string, ...any)
	protect func(func())

	jobs chan queued
	stop chan struct{}
	done chan struct{}

	mu       sync.Mutex // guards stopped and started
	stopped  bool
	started  bool
	stopOnce sync.Once

	// Owned by the goroutine.
	dev          hid.Device
	client       *dongle.Client
	state        State
	published    bool
	pendingUntil time.Time

	// Set before Start.
	picker    *Picker
	listEvery time.Duration
	onList    func([]hid.Info)
	choices   chan string
	// Owned by the goroutine.
	nextList  time.Time
	several   bool
	listed    []hid.Info
	haveList  bool
	listError string // the failure already logged, so a lasting one logs once
}

// NewOwner creates an owner; onState is called on the owner goroutine on every change
// and must not block.
func NewOwner(open OpenFunc, options Options, onState func(State)) *Owner {
	return &Owner{
		open: open, options: options, onState: onState, now: time.Now,
		jobs: make(chan queued, 32), choices: make(chan string, 4), stop: make(chan struct{}), done: make(chan struct{}),
	}
}

// SetLog gives the owner somewhere to record what it would otherwise only retry, and a wrapper
// for its goroutine that records a panic. Call this before Start.
func (o *Owner) SetLog(logf func(string, ...any), protect func(func())) {
	o.logf, o.protect = logf, protect
}

// SetPicker makes the owner list the attached dongles every period, and tell onList when the
// list changes. Listing only opens and closes device nodes and sends no reports. onList runs on
// the owner goroutine and must not block. Call this before Start.
func (o *Owner) SetPicker(picker *Picker, every time.Duration, onList func([]hid.Info)) {
	o.picker, o.listEvery, o.onList = picker, every, onList
}

// Choose moves to another dongle, after the jobs already queued have run on the current one. It
// never blocks, so the UI thread can call it freely; a burst of more than a few is cut short.
func (o *Owner) Choose(serial string) {
	select {
	case o.choices <- serial:
	default:
	}
}

// Start launches the owner goroutine.
func (o *Owner) Start() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.started || o.stopped {
		return
	}
	o.started = true
	if o.protect != nil {
		go o.protect(o.loop)
		return
	}
	go o.loop()
}

// Submit queues a job. done is always called exactly once and never on the caller's
// goroutine, so the UI thread can call this freely.
func (o *Owner) Submit(job Job, done func(error)) { o.enqueue(queued{job: job, done: done}) }

// SubmitCommand queues a job that changes device state, which speeds up status polling.
func (o *Owner) SubmitCommand(job Job, done func(error)) {
	o.enqueue(queued{job: job, done: done, command: true})
}

func (o *Owner) enqueue(q queued) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stopped || !o.started {
		go q.finish(ErrStopped)
		return
	}
	select {
	case o.jobs <- q:
	default:
		go q.finish(ErrQueueFull)
	}
}

func (q queued) finish(err error) {
	if q.done != nil {
		q.done(err)
	}
}

// Stop runs what is already queued, closes the device and waits up to timeout.
func (o *Owner) Stop(timeout time.Duration) {
	o.stopOnce.Do(func() {
		o.mu.Lock()
		started := o.started
		o.stopped = true
		o.mu.Unlock()
		close(o.stop)
		if !started {
			close(o.done)
		}
	})
	select {
	case <-o.done:
	case <-time.After(timeout):
	}
}

func (o *Owner) delay() time.Duration {
	switch {
	case o.client == nil:
		return o.options.Reopen
	case o.now().Before(o.pendingUntil):
		return o.options.Pending
	}
	return o.options.Idle
}

func (o *Owner) loop() {
	defer close(o.done)
	o.poll()
	timer := time.NewTimer(o.delay())
	defer timer.Stop()
	for {
		select {
		case <-o.stop:
			o.drain()
			o.drop()
			return
		case q := <-o.jobs:
			o.run(q)
			o.poll()
		case serial := <-o.choices:
			o.drain()
			o.applyChoice(serial)
		case <-timer.C:
			o.poll()
		}
		timer.Reset(o.delay())
	}
}

// applyChoice lets go of the open dongle and takes the chosen one. Passing through no dongle
// makes everything that depends on the connection read again, even if the numbers happen to match.
func (o *Owner) applyChoice(serial string) {
	if o.picker == nil {
		return
	}
	o.picker.Choose(serial)
	o.drop()
	o.publish(State{Several: o.several})
	o.poll()
}

// refreshList lists the dongles when that is due. A failed listing keeps the last one.
func (o *Owner) refreshList() {
	if o.picker == nil || o.now().Before(o.nextList) {
		return
	}
	o.nextList = o.now().Add(o.listEvery)
	list, err := o.picker.List()
	if err != nil {
		if text := err.Error(); text != o.listError && o.logf != nil {
			o.logf("listing the dongles failed: %v", err)
			o.listError = text
		}
		return
	}
	o.listError = ""
	o.several = len(list) > 1
	if o.haveList && sameDongles(o.listed, list) {
		return
	}
	o.listed, o.haveList = list, true
	if o.onList != nil {
		o.onList(list)
	}
}

func sameDongles(a, b []hid.Info) bool {
	if len(a) != len(b) {
		return false
	}
	text := func(err error) string {
		if err == nil {
			return ""
		}
		return err.Error()
	}
	for i := range a {
		if a[i].Path != b[i].Path || a[i].Serial != b[i].Serial || a[i].Openable != b[i].Openable || text(a[i].Err) != text(b[i].Err) {
			return false
		}
	}
	return true
}

// drain runs what is queued right now.
func (o *Owner) drain() {
	for {
		select {
		case q := <-o.jobs:
			o.run(q)
		default:
			return
		}
	}
}

func (o *Owner) run(q queued) {
	if o.client == nil {
		q.finish(ErrNoDongle)
		return
	}
	if q.command {
		o.pendingUntil = o.now().Add(o.options.PendingWindow)
	}
	err := q.job(o.client)
	if q.command {
		o.pendingUntil = o.now().Add(o.options.PendingWindow)
	}
	q.finish(err)
}

func (o *Owner) drop() {
	if o.dev != nil {
		o.dev.Close()
	}
	o.dev, o.client = nil, nil
}

func (o *Owner) read() (dongle.Status, error) {
	if o.client == nil {
		dev, err := o.open()
		if err != nil {
			return dongle.Status{}, err
		}
		o.dev, o.client = dev, dongle.NewClient(dev)
	}
	return o.client.Status()
}

// poll reads report 2 once. A read failing on an open device gets one immediate
// reopen, so a passing glitch does not flash "No dongle".
func (o *Owner) poll() {
	o.refreshList()
	hadDevice := o.client != nil
	status, err := o.read()
	if err != nil && hadDevice {
		o.drop()
		status, err = o.read()
	}
	next := State{Pending: o.now().Before(o.pendingUntil), Several: o.several}
	switch {
	case errors.Is(err, ErrChooseDongle):
		o.drop()
		next.ChooseDongle = true
	case err != nil:
		o.drop()
		next.Problem = classify(err)
	default:
		next.Present, next.Status, next.Serial = true, status, o.dev.Serial()
	}
	o.publish(next)
}

func (o *Owner) publish(next State) {
	if !o.published || next != o.state {
		o.state, o.published = next, true
		if o.onState != nil {
			o.onState(next)
		}
	}
}

// classify splits open failures into quiet absence and problems worth showing.
func classify(err error) string {
	var mismatch *hid.MatchError
	if errors.As(err, &mismatch) {
		if mismatch.Found > 1 {
			return "Several dongles are plugged in. Keep one plugged in."
		}
		return ""
	}
	return err.Error()
}
