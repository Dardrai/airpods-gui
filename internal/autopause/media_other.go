//go:build !linux && !windows

package autopause

// SystemMedia has no players to control on this system.
func SystemMedia() Media { return noMedia{} }

type noMedia struct{}

func (noMedia) PauseActive() (Paused, error) { return nil, nil }
