//go:build !linux

package tray

// Available reports whether a tray host is running; other systems always have one.
func Available() bool { return true }
