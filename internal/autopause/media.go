package autopause

// Media is the desktop's media players. Its calls may wait on other programs for a
// bounded time, so they never run on the device goroutine.
//
// It never toggles blindly: a Media pauses only players it can ask and that report playing,
// and resumes only those, after checking they are still paused. The dongle may have paused
// them already (firmware 0.1.65 and newer), and a toggle would start them again.
type Media interface {
	// PauseActive pauses what is playing. It returns nil when nothing was playing, and an
	// error when it cannot tell, without touching anything.
	PauseActive() (Paused, error)
}

// Paused is one successful pause: exactly the players it stopped and nothing else.
type Paused interface {
	// StillOurs reports whether those players are still paused as we left them. Someone
	// pressing play, or moving on to another track, ends the claim.
	StillOurs() bool
	// Resume plays them again, only if they are still paused.
	Resume() error
	// Release gives up the claim without playing anything.
	Release()
}
