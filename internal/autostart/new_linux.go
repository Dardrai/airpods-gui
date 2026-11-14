//go:build linux

package autostart

import "os"

// New returns the Linux entry for the running executable.
func New() (Manager, error) {
	exe, err := executable()
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return DesktopEntryFile{Path: DesktopPath(os.Getenv("XDG_CONFIG_HOME"), home), Exe: exe}, nil
}
