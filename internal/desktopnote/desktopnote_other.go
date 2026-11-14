//go:build !linux && !windows

package desktopnote

// Send reports that the toolkit's own notification is the one to use here.
func Send(string, string) error { return ErrUnsupported }
