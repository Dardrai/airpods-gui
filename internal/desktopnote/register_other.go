//go:build !windows

package desktopnote

// Register has nothing to do here: Linux names the sender with Send's app name instead.
func Register([]byte) error { return nil }
