package updates

import (
	"context"
	"net/http"
	"sync"
	"time"
)

const (
	// Preference is the persisted on/off value of the update check.
	Preference = "updates.check"
	// Default keeps the app offline until the user turns the check on.
	Default = false
	// What a checker keeps, under its Prefix: when GitHub was last asked (Unix seconds), the
	// newest tag seen and its page, so the window can show them before asking again, and the
	// newest tag already announced with a notification.
	lastCheckPreference = "lastCheck"
	latestPreference    = "latest"
	pagePreference      = "page"
	announcedPreference = "announced"
	// defaultPrefix keeps the app's own check under the names it always had.
	defaultPrefix = "updates"

	// Period is how often the check asks GitHub at most.
	Period = 2 * time.Hour
	// pollEvery is how often the checker looks at whether a check is due.
	pollEvery = 10 * time.Minute
)

// Prefs is the part of Fyne's preferences the checker uses.
type Prefs interface {
	BoolWithFallback(key string, fallback bool) bool
	IntWithFallback(key string, fallback int) int
	SetBool(key string, value bool)
	SetInt(key string, value int)
	StringWithFallback(key, fallback string) string
	SetString(key, value string)
}

// Result is what the window shows: the running version and, if known, a newer release.
type Result struct {
	Running Version
	// Dev marks a build without a version, which is never compared.
	Dev bool
	// Latest is the newest release seen, valid when Known.
	Latest Release
	Known  bool
	// Available is set when Latest is newer than the running build.
	Available bool
	// Err is the last check's failure, for the log and a manual check.
	Err error
	// Checking is set while a request is out.
	Checking bool
}

// Checker asks GitHub at most every two hours while it is turned on. It calls OnResult on its
// own goroutine; the caller moves that to the main thread.
type Checker struct {
	Prefs  Prefs
	Client *http.Client
	Repo   string
	// Prefix names what this checker saves; empty is the app's own check. The on/off
	// switch, Preference, is shared by every checker.
	Prefix  string
	Running string
	// OnResult hears every change of the result.
	OnResult func(Result)
	// OnNew hears a newer release the first time it is seen; it is never repeated for a tag.
	OnNew func(Release)
	Logf  func(format string, args ...any)
	Now   func() time.Time
	fetch func(context.Context) (Release, error)

	mu       sync.Mutex
	loadOnce sync.Once
	result   Result
	running  bool
	stop     chan struct{}
	stopOnce sync.Once
	wake     chan struct{}
}

// Load reads what earlier checks saved, so Result has it before Start. Start calls it too.
func (c *Checker) Load() { c.loadOnce.Do(c.init) }

func (c *Checker) key(name string) string {
	if c.Prefix == "" {
		return defaultPrefix + "." + name
	}
	return c.Prefix + "." + name
}

func (c *Checker) init() {
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Client == nil {
		c.Client = &http.Client{Timeout: requestTimeout}
	}
	if c.Repo == "" {
		c.Repo = Repo
	}
	if c.Logf == nil {
		c.Logf = func(string, ...any) {}
	}
	if c.fetch == nil {
		c.fetch = func(ctx context.Context) (Release, error) { return Latest(ctx, c.Client, c.Repo) }
	}
	c.stop, c.wake = make(chan struct{}), make(chan struct{}, 1)
	running, ok := Parse(c.Running)
	c.result = Result{Running: running, Dev: !ok}
	if tag := c.Prefs.StringWithFallback(c.key(latestPreference), ""); tag != "" {
		if v, ok := Parse(tag); ok {
			c.result.Latest = Release{Tag: tag, Version: v, Page: c.Prefs.StringWithFallback(c.key(pagePreference), "")}
			c.result.Known = true
			c.result.Available = !c.result.Dev && Newer(v, running)
		}
	}
}

// Start runs the checker until Stop. A check that is due runs right away.
func (c *Checker) Start() {
	c.Load()
	go c.loop()
}

// Stop ends the checker; a request that is out is cancelled. A checker never started has
// nothing to stop.
func (c *Checker) Stop() {
	c.stopOnce.Do(func() {
		if c.stop != nil {
			close(c.stop)
		}
	})
}

// SetFetch replaces the call to GitHub, for tests. It must come before Start.
func (c *Checker) SetFetch(fetch func(context.Context) (Release, error)) { c.fetch = fetch }

// Result is the current state, for a window that opens.
func (c *Checker) Result() Result {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.result
}

// Enabled reports whether the user has turned the check on.
func (c *Checker) Enabled() bool { return c.Prefs.BoolWithFallback(Preference, Default) }

// SetEnabled saves the choice; turning it on checks right away if a check is due.
func (c *Checker) SetEnabled(on bool) {
	c.Prefs.SetBool(Preference, on)
	if on {
		c.CheckNow()
	}
}

// CheckNow asks GitHub now, if the check is on and no request is out.
func (c *Checker) CheckNow() {
	c.Prefs.SetInt(c.key(lastCheckPreference), 0)
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *Checker) due() bool {
	if !c.Enabled() {
		return false
	}
	last := time.Unix(int64(c.Prefs.IntWithFallback(c.key(lastCheckPreference), 0)), 0)
	now := c.Now()
	return now.Sub(last) >= Period || now.Before(last)
}

func (c *Checker) loop() {
	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()
	for {
		if c.due() {
			c.check()
		}
		select {
		case <-c.stop:
			return
		case <-ticker.C:
		case <-c.wake:
		}
	}
}

func (c *Checker) check() {
	c.Prefs.SetInt(c.key(lastCheckPreference), int(c.Now().Unix()))
	c.update(func(r *Result) { r.Checking = true })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		select {
		case <-c.stop:
			cancel()
		case <-done:
		}
	}()
	rel, err := c.fetch(ctx)
	close(done)
	cancel()
	select {
	case <-c.stop:
		return // A stopped checker reports nothing more.
	default:
	}
	if err != nil {
		c.Logf("update check failed: %v", err)
		c.update(func(r *Result) { r.Checking, r.Err = false, err })
		return
	}
	c.Prefs.SetString(c.key(latestPreference), rel.Tag)
	c.Prefs.SetString(c.key(pagePreference), rel.Page)
	var announce bool
	c.update(func(r *Result) {
		r.Checking, r.Err, r.Latest, r.Known = false, nil, rel, true
		r.Available = !r.Dev && Newer(rel.Version, r.Running)
		announce = r.Available && c.Prefs.StringWithFallback(c.key(announcedPreference), "") != rel.Tag
	})
	c.Logf("update check: latest %s, running %s", rel.Tag, c.Running)
	if announce {
		c.Prefs.SetString(c.key(announcedPreference), rel.Tag)
		if c.OnNew != nil {
			c.OnNew(rel)
		}
	}
}

func (c *Checker) update(change func(*Result)) {
	c.mu.Lock()
	change(&c.result)
	r := c.result
	c.mu.Unlock()
	if c.OnResult != nil {
		c.OnResult(r)
	}
}
