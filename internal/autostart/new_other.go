//go:build !windows && !linux

package autostart

import "errors"

// New is unavailable on this platform; only Windows and Linux are supported.
func New() (Manager, error) {
	return nil, errors.New("autostart is only implemented for Windows and Linux")
}
