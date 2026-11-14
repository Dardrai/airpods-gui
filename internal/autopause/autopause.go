// Package autopause pauses the desktop's media player when the AirPods come out of the
// ears, and resumes it when they return.
//
// # Sharing the dongle
//
// Polling was idle-free before this: only report 2, so a command line client could run
// beside the tray whenever the settings window was closed. Ear status is only available
// as a kept report on the shared report 3 channel, so polling it does weaken that. To
// keep the overlap as small as it can be:
//
//   - Nothing is polled unless auto-pause is on, the AirPods are ready, and either the
//     dongle reports USB playback or a pause of ours is waiting for the ears to return.
//     With no audio going to the dongle the channel stays completely free.
//   - It is one short request a second through the device owner, never queued twice.
//   - The audio state is only read when a transition needs it.
//   - A busy channel skips the tick silently and nothing is retried beyond the next tick.
//
// While audio plays, another client's request can still find the channel busy for a few
// milliseconds a second, and a multi-page read by another client can be interrupted, as
// with any two clients of the channel. Turning auto-pause off removes this entirely.
//
// # The dongle's own ear pause
//
// Since firmware 0.1.65 the dongle itself presses Pause on its USB media keys when a bud
// comes out during USB playback, and Play when it goes back in, while the AirPods' ear
// detection is on. Nothing tells a host whether its dongle does this, so both can act on the
// same removal. That is why the switch is off until the user turns it on, and why a Media
// only ever pauses a player it asked and saw playing, and resumes only that player once it
// is still paused: after the dongle's own Pause there is nothing playing, so the app claims
// nothing, and the dongle's Play is left to the dongle. A blind play/pause toggle has no
// such check and would start music the dongle had just paused, so no Media may use one.
package autopause

import (
	"errors"
	"sync"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/internal/device"
)

const (
	// Preference is the name of the persisted on/off value.
	Preference = "autopause.enabled"
	// Default is the value before the user has chosen: off, because dongle firmware 0.1.65
	// and newer pause and resume by themselves. A choice the user saved is kept.
	Default = false

	// Period is the time between polls.
	Period = time.Second

	inputPhone   = 2 // the audio input of a phone that owns the AirPods
	pauseTimeout = 3 * time.Second
	refusedRest  = 30 * time.Second
)

// Submitter runs jobs on the device owner goroutine; device.Owner is one.
type Submitter interface {
	Submit(job device.Job, done func(error))
}

// Options configure a Controller.
type Options struct {
	Owner   Submitter
	Media   Media
	Enabled bool
	// Period overrides the poll period.
	Period time.Duration
	// Now overrides the clock.
	Now func() time.Time
	// Log receives rare events; nil keeps them to itself.
	Log func(format string, args ...any)
	// Protect wraps the controller's goroutine, to record a panic; nil runs it as it is.
	Protect func(func())
}

// Controller decides when to pause and resume. SetState and SetEnabled may be called
// from any goroutine; everything else stays on its own goroutine.
type Controller struct {
	opts Options
	stop chan struct{}
	done chan struct{}
	once sync.Once

	mu      sync.Mutex
	enabled bool
	state   device.State
	started bool

	// Owned by the goroutine.
	ears       Ears
	paused     Paused
	generation uint32
	due        Action
	dueSince   time.Time
	restUntil  time.Time
	lastError  string
}

// New makes a Controller; it does nothing until Start.
func New(opts Options) *Controller {
	if opts.Period <= 0 {
		opts.Period = Period
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Log == nil {
		opts.Log = func(string, ...any) {}
	}
	return &Controller{opts: opts, enabled: opts.Enabled, stop: make(chan struct{}), done: make(chan struct{})}
}

// Start begins polling.
func (c *Controller) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return
	}
	c.started = true
	if c.opts.Protect != nil {
		go c.opts.Protect(c.loop)
		return
	}
	go c.loop()
}

// Stop ends polling and forgets any pause; the player stays as it is.
func (c *Controller) Stop(timeout time.Duration) {
	c.mu.Lock()
	started := c.started
	c.mu.Unlock()
	c.once.Do(func() { close(c.stop) })
	if !started {
		return
	}
	select {
	case <-c.done:
	case <-time.After(timeout):
	}
}

// SetState gives the controller the latest device state.
func (c *Controller) SetState(st device.State) {
	c.mu.Lock()
	c.state = st
	c.mu.Unlock()
}

// SetEnabled turns auto-pause on or off. Off also ends the claim to a paused player.
func (c *Controller) SetEnabled(on bool) {
	c.mu.Lock()
	c.enabled = on
	c.mu.Unlock()
}

// Enabled reports the current choice.
func (c *Controller) Enabled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enabled
}

func (c *Controller) loop() {
	defer close(c.done)
	defer c.forget()
	ticker := time.NewTicker(c.opts.Period)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			c.Tick()
		}
	}
}

func (c *Controller) forget() {
	if c.paused != nil {
		c.paused.Release()
		c.paused = nil
	}
	c.ears.Reset()
	c.due = None
}

// logOnce records an error only when it differs from the last one.
func (c *Controller) logOnce(text string) {
	if text != c.lastError {
		c.lastError = text
		c.opts.Log("auto-pause: %s", text)
	}
}

// run runs a job on the owner goroutine and waits for it. false means it did not finish
// because the controller is stopping; the job's results must not be read then.
func (c *Controller) run(job device.Job) (finished bool, err error) {
	result := make(chan error, 1)
	c.opts.Owner.Submit(job, func(err error) { result <- err })
	select {
	case err = <-result:
		return true, err
	case <-c.stop:
		return false, nil
	}
}

// Tick is one poll. The goroutine calls it; tests call it directly.
func (c *Controller) Tick() {
	c.mu.Lock()
	enabled, st := c.enabled, c.state
	c.mu.Unlock()

	status := st.Status
	if !enabled || !st.Ready() || !status.Has(dongle.CapRawTransport) || status.MicrophoneActive ||
		c.generation != status.Generation {
		c.forget()
		c.generation = status.Generation
		return
	}
	if c.paused == nil && !status.USBPlayback {
		// Nothing to protect, so nothing is asked of the shared channel.
		c.forget()
		return
	}
	now := c.opts.Now()
	if now.Before(c.restUntil) {
		return
	}

	var report *dongle.Cached
	finished, err := c.run(func(client *dongle.Client) error {
		var err error
		report, err = client.Cached(dongle.SlotEarStatus)
		return err
	})
	if !finished {
		return
	}
	if err != nil {
		c.readFailed(err, now)
		return
	}

	action, ok := c.ears.Observe(report)
	if !ok {
		c.forget()
		return
	}
	if c.paused != nil && !c.paused.StillOurs() {
		c.paused.Release()
		c.paused = nil
		c.due = None
	}
	if action != None {
		c.due, c.dueSince = action, now
	}
	switch c.due {
	case Pause:
		if c.paused != nil || now.Sub(c.dueSince) > pauseTimeout {
			c.due = None
			return
		}
		c.pause(status.Generation)
	case Resume:
		if c.paused == nil {
			c.due = None
			return
		}
		c.resume(status.Generation)
	}
}

func (c *Controller) readFailed(err error, now time.Time) {
	switch {
	case device.IsBusy(err), errors.Is(err, device.ErrNoDongle), errors.Is(err, device.ErrQueueFull), errors.Is(err, device.ErrStopped):
		return
	}
	var refused *dongle.Unsupported
	if errors.As(err, &refused) {
		c.restUntil = now.Add(refusedRest)
	}
	c.logOnce(err.Error())
}

// mayRoute reads the audio state and status on the owner goroutine and says whether the
// host, on the same connection and with no microphone in use, owns the audio, and whether
// USB playback is on. retry means the answer has to wait for another tick.
func (c *Controller) mayRoute(generation uint32) (allowed, playing, retry bool) {
	var audio dongle.AudioState
	var status dongle.Status
	finished, err := c.run(func(client *dongle.Client) error {
		var err error
		if audio, err = client.AudioState(); err != nil {
			return err
		}
		status, err = client.Status()
		return err
	})
	if !finished {
		return false, false, false
	}
	if err != nil {
		if device.IsBusy(err) || errors.Is(err, device.ErrQueueFull) {
			return false, false, true
		}
		c.logOnce(err.Error())
		return false, false, false
	}
	allowed = audio.Input != inputPhone && !audio.MicrophoneActive && !status.MicrophoneActive &&
		status.Connected && status.ControlsReady && status.Generation == generation
	return allowed, status.USBPlayback, false
}

func (c *Controller) pause(generation uint32) {
	allowed, playing, retry := c.mayRoute(generation)
	if retry {
		return
	}
	c.due = None
	if !allowed || !playing {
		return
	}
	paused, err := c.opts.Media.PauseActive()
	if err != nil {
		c.logOnce("pause: " + err.Error())
		return
	}
	c.paused = paused
}

func (c *Controller) resume(generation uint32) {
	allowed, _, retry := c.mayRoute(generation)
	if retry {
		return
	}
	c.due = None
	paused := c.paused
	c.paused = nil
	if !allowed {
		paused.Release()
		return
	}
	// The player checks for itself that it is still paused as we left it.
	if err := paused.Resume(); err != nil {
		c.logOnce("resume: " + err.Error())
	}
	paused.Release()
}
