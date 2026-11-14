//go:build !windows && !linux

package updates

// Detect finds no way to install here.
func Detect() (Installer, bool) { return Installer{}, false }

// Relaunch has nothing to do here.
func Relaunch() error { return nil }
