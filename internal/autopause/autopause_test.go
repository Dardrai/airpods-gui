package autopause

import (
	"testing"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/internal/device"
)

func earPacket(left, right earState) []byte { return []byte{4, 0, 4, 0, 6, 0, byte(left), byte(right)} }

func TestEarTransitions(t *testing.T) {
	type step struct {
		left, right earState
		want        Action
	}
	cases := map[string][]step{
		"both out then back": {{earIn, earIn, None}, {earIn, earOut, Pause}, {earOut, earOut, Pause}, {earIn, earIn, Resume}},
		"movement holds":     {{earIn, earIn, None}, {earMoving, earIn, None}, {earOut, earIn, Pause}},
		"one bud in the case": {{earIn, earCase, None}, {earOut, earCase, Pause}, {earIn, earCase, Resume},
			{earCase, earCase, Pause}, {earCase, earIn, Resume}},
		"never worn":   {{earOut, earOut, None}, {earCase, earOut, None}, {earCase, earCase, None}},
		"returns solo": {{earOut, earOut, None}, {earIn, earOut, None}, {earOut, earOut, Pause}},
	}
	for name, steps := range cases {
		t.Run(name, func(t *testing.T) {
			var ears Ears
			for i, s := range steps {
				got, ok := ears.Observe(&dongle.Cached{Revision: uint32(i + 1), Packet: earPacket(s.left, s.right)})
				if !ok || got != s.want {
					t.Errorf("step %d: %v %v, want %v", i, got, ok, s.want)
				}
			}
		})
	}
}

func TestStaleReportsNeverPause(t *testing.T) {
	var ears Ears
	worn := earPacket(earIn, earIn)
	out := earPacket(earOut, earIn)
	if got, ok := ears.Observe(&dongle.Cached{Revision: 5, AgeMs: 60000, Packet: out}); !ok || got != None {
		t.Fatalf("first report acted: %v", got)
	}
	ears.Observe(&dongle.Cached{Revision: 6, Packet: worn})
	if got, _ := ears.Observe(&dongle.Cached{Revision: 7, AgeMs: maxFreshAge + 1, Packet: out}); got != None {
		t.Errorf("old removal paused")
	}
	if got, _ := ears.Observe(&dongle.Cached{Revision: 7, AgeMs: 90000, Packet: out}); got != None {
		t.Errorf("same report acted again")
	}
	if got, _ := ears.Observe(&dongle.Cached{Revision: 8, AgeMs: maxFreshAge + 1, Packet: worn}); got != Resume {
		t.Errorf("a late return must still resume what we hold: %v", got)
	}
	if got, _ := ears.Observe(&dongle.Cached{Revision: 7, Packet: out}); got != None {
		t.Errorf("an older revision acted: %v", got)
	}
}

func TestStaleCaseDoesNotCountAsResting(t *testing.T) {
	var ears Ears
	ears.Observe(&dongle.Cached{Revision: 1, Packet: earPacket(earIn, earCase)})
	ears.Observe(&dongle.Cached{Revision: 1, AgeMs: 90000, Packet: earPacket(earIn, earCase)})
	if ears.state != [2]earState{earIn, earGone} {
		t.Errorf("state %v", ears.state)
	}
}

func TestBadReportsEndTheClaim(t *testing.T) {
	var ears Ears
	if _, ok := ears.Observe(nil); ok {
		t.Error("empty slot accepted")
	}
	for _, packet := range [][]byte{{4, 0, 4, 0, 6, 0, 9, 0}, {4, 0, 4, 0, 7, 0, 0, 0}, {4, 0}} {
		if _, ok := ears.Observe(&dongle.Cached{Revision: 1, Packet: packet}); ok {
			t.Errorf("accepted % x", packet)
		}
		ears.Reset()
	}
}

// inline runs jobs right away on a simulated dongle and can act as a busy channel.
type inline struct {
	client *dongle.Client
	jobs   int
	busy   bool
}

func (i *inline) Submit(job device.Job, done func(error)) {
	i.jobs++
	if i.busy {
		done(&dongle.RequestRejected{Code: 2})
		return
	}
	done(job(i.client))
}

type fakeMedia struct {
	playing        bool
	pauses         int
	resumes        int
	releases       int
	notOurs        bool
	playingAtPause bool
}

func (f *fakeMedia) PauseActive() (Paused, error) {
	f.pauses++
	f.playingAtPause = f.playing
	if !f.playing {
		return nil, nil
	}
	f.playing = false
	return &fakePaused{f}, nil
}

type fakePaused struct{ f *fakeMedia }

func (p *fakePaused) StillOurs() bool { return !p.f.notOurs }
func (p *fakePaused) Resume() error   { p.f.resumes++; p.f.playing = true; return nil }
func (p *fakePaused) Release()        { p.f.releases++ }

type rig struct {
	t         *testing.T
	dev       *sim.Device
	owner     *inline
	media     *fakeMedia
	c         *Controller
	clock     time.Time
	revision  uint32
	flags     byte
	audioByte byte
}

const (
	flagsReady   = 1 | 2
	flagsPlaying = 1 | 2 | 64
	flagsMic     = 1 | 2 | 16
)

func newRig(t *testing.T) *rig {
	t.Helper()
	cfg := sim.Demo()
	cfg.Flags = flagsPlaying
	dev := sim.New(cfg)
	handle, err := dev.Open("")
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{t: t, dev: dev, owner: &inline{client: dongle.NewClient(handle)}, media: &fakeMedia{playing: true},
		clock: time.Unix(1000, 0), flags: flagsPlaying, audioByte: 1}
	r.c = New(Options{Owner: r.owner, Media: r.media, Enabled: true, Now: func() time.Time { return r.clock }})
	r.sync()
	return r
}

// sync gives the controller what the owner would have published.
func (r *rig) sync() {
	r.dev.SetFlags(r.flags)
	r.dev.Update(func(c *sim.Config) {
		audio := append(sim.Hex(nil), c.Replies[9]...)
		audio[14] = r.audioByte
		c.Replies[9] = audio
	})
	status, err := r.owner.client.Status()
	if err != nil {
		r.t.Fatal(err)
	}
	r.c.SetState(device.State{Present: true, Status: status})
}

func (r *rig) ears(left, right earState) {
	r.revision++
	r.dev.Update(func(c *sim.Config) {
		c.Cached[dongle.SlotEarStatus] = sim.CachedReport{Revision: r.revision, Packet: earPacket(left, right)}
	})
}

func (r *rig) tick() {
	r.clock = r.clock.Add(time.Second)
	r.c.Tick()
}

// baseline gets past the first tick, which only notes the connection, and the first
// report, which only sets a starting point.
func (r *rig) baseline() {
	r.ears(earIn, earIn)
	r.tick()
	r.tick()
}

func TestPausesAndResumesOnlyWhatItPaused(t *testing.T) {
	r := newRig(t)
	r.baseline()
	if r.media.pauses != 0 {
		t.Fatal("baseline paused")
	}
	r.ears(earOut, earIn)
	r.tick()
	if r.media.pauses != 1 || r.media.playing {
		t.Fatalf("not paused: %+v", r.media)
	}
	// USB goes idle once paused, and the wait for the ears has to go on regardless.
	r.flags = flagsReady
	r.sync()
	r.tick()
	r.ears(earIn, earIn)
	r.tick()
	if r.media.resumes != 1 || !r.media.playing || r.media.releases != 1 {
		t.Fatalf("not resumed: %+v", r.media)
	}
	r.ears(earOut, earOut)
	r.tick()
	if r.media.pauses != 1 {
		t.Errorf("paused again with USB idle: %+v", r.media)
	}
}

// Dongle firmware 0.1.65 and newer pause by themselves when a bud comes out, and usually
// first. USB playback still shows on for a moment, so the app goes on to ask the player, which
// is no longer playing: the app claims nothing, and leaves the dongle's own Play alone.
func TestTheDonglesOwnPauseLeavesNothingToClaim(t *testing.T) {
	r := newRig(t)
	r.baseline()
	r.media.playing = false // the dongle's Pause landed first
	r.ears(earOut, earIn)
	r.tick()
	if r.media.pauses != 1 || r.media.playingAtPause {
		t.Fatalf("asked %+v", r.media)
	}
	r.flags = flagsReady
	r.sync()
	r.tick()
	r.media.playing = true // the dongle's Play
	r.ears(earIn, earIn)
	r.tick()
	r.tick()
	if r.media.resumes != 0 || r.media.releases != 0 {
		t.Errorf("acted on a pause it never made: %+v", r.media)
	}
}

func TestOffUntilChosen(t *testing.T) {
	if Default {
		t.Error("auto-pause must start off: dongle firmware 0.1.65 and newer pause by themselves")
	}
}

// USB outlasts a pause by a moment, and a player that can be asked is safe to resume then.
func TestPlayerThatCanBeAskedResumesWhileUSBIsStillOn(t *testing.T) {
	r := newRig(t)
	r.baseline()
	r.ears(earOut, earIn)
	r.tick()
	r.ears(earIn, earIn)
	r.tick()
	if r.media.resumes != 1 {
		t.Errorf("not resumed: %+v", r.media)
	}
}

func TestNothingPlayingIsNotResumed(t *testing.T) {
	r := newRig(t)
	r.media.playing = false
	r.baseline()
	r.ears(earOut, earIn)
	r.tick()
	r.ears(earIn, earIn)
	r.tick()
	if r.media.resumes != 0 {
		t.Errorf("resumed something it did not pause: %+v", r.media)
	}
}

func TestNoTrafficWithoutUSBPlayback(t *testing.T) {
	r := newRig(t)
	r.flags = flagsReady
	r.sync()
	r.ears(earIn, earIn)
	for range 5 {
		r.tick()
	}
	if r.owner.jobs != 0 {
		t.Errorf("%d jobs with nothing playing", r.owner.jobs)
	}
}

func TestNoTrafficWhenOffOrNotReady(t *testing.T) {
	r := newRig(t)
	r.ears(earIn, earIn)
	r.c.SetEnabled(false)
	r.tick()
	r.tick()
	r.c.SetEnabled(true)
	r.flags = 1 | 64
	r.sync()
	r.tick()
	r.tick()
	r.c.SetState(device.State{})
	r.tick()
	if r.owner.jobs != 0 {
		t.Errorf("%d jobs", r.owner.jobs)
	}
}

func TestPhoneOwningAudioIsLeftAlone(t *testing.T) {
	r := newRig(t)
	r.baseline()
	r.audioByte = inputPhone
	r.sync()
	r.ears(earOut, earIn)
	r.tick()
	if r.media.pauses != 0 {
		t.Errorf("paused the host for the phone's audio")
	}
}

func TestPhoneTakingOverCancelsTheResume(t *testing.T) {
	r := newRig(t)
	r.baseline()
	r.ears(earOut, earIn)
	r.tick()
	r.audioByte = inputPhone
	r.sync()
	r.ears(earIn, earIn)
	r.tick()
	if r.media.resumes != 0 || r.media.releases != 1 {
		t.Errorf("%+v", r.media)
	}
}

func TestPlayerChangedByTheUserIsNotResumed(t *testing.T) {
	r := newRig(t)
	r.baseline()
	r.ears(earOut, earIn)
	r.tick()
	r.media.notOurs = true
	r.tick()
	r.ears(earIn, earIn)
	r.tick()
	if r.media.resumes != 0 || r.media.releases != 1 {
		t.Errorf("%+v", r.media)
	}
}

func TestDisablingAndReconnectingEndTheClaim(t *testing.T) {
	for name, interrupt := range map[string]func(r *rig){
		"disabled":     func(r *rig) { r.c.SetEnabled(false) },
		"disconnected": func(r *rig) { r.c.SetState(device.State{Present: true}) },
		"new session":  func(r *rig) { r.dev.BumpGeneration(); r.sync() },
		"microphone":   func(r *rig) { r.flags = flagsMic; r.sync() },
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.baseline()
			r.ears(earOut, earIn)
			r.tick()
			interrupt(r)
			r.tick()
			if r.media.releases != 1 {
				t.Errorf("claim kept: %+v", r.media)
			}
			r.c.SetEnabled(true)
			r.flags = flagsPlaying
			r.sync()
			r.ears(earIn, earIn)
			r.tick()
			r.tick()
			if r.media.resumes != 0 {
				t.Errorf("resumed after the claim ended: %+v", r.media)
			}
		})
	}
}

func TestBusyIsASilentSkip(t *testing.T) {
	r := newRig(t)
	var logged []string
	r.c.opts.Log = func(format string, _ ...any) { logged = append(logged, format) }
	r.baseline()
	r.owner.busy = true
	r.ears(earOut, earIn)
	r.tick()
	if r.media.pauses != 0 {
		t.Fatal("paused without a reading")
	}
	r.owner.busy = false
	r.tick()
	if r.media.pauses != 1 {
		t.Errorf("the removal was lost after one busy tick: %+v", r.media)
	}
	if len(logged) != 0 {
		t.Errorf("logged: %q", logged)
	}
}

func TestBusyAudioReadRetriesOnTheNextTick(t *testing.T) {
	r := newRig(t)
	r.baseline()
	r.ears(earOut, earIn)
	busyOnce := &audioBusy{inline: r.owner}
	r.c.opts.Owner = busyOnce
	r.tick()
	if r.media.pauses != 0 {
		t.Fatal("paused without the audio state")
	}
	r.tick()
	if r.media.pauses != 1 {
		t.Errorf("not retried: %+v", r.media)
	}
}

// audioBusy lets the ear read through and refuses the job right after it, once.
type audioBusy struct {
	*inline
	count int
}

func (a *audioBusy) Submit(job device.Job, done func(error)) {
	a.count++
	if a.count == 2 {
		done(&dongle.RequestRejected{Code: 2})
		return
	}
	a.inline.Submit(job, done)
}

func TestLoopStops(t *testing.T) {
	r := newRig(t)
	c := New(Options{Owner: r.owner, Media: r.media, Period: time.Millisecond})
	c.Start()
	time.Sleep(5 * time.Millisecond)
	c.Stop(time.Second)
	select {
	case <-c.done:
	default:
		t.Error("still running")
	}
	c.Stop(time.Second)
	New(Options{}).Stop(time.Second)
}
