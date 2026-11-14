//go:build windows

package updates

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// Detect offers the release's installer to a copy the installer set up. A plain exe
// started from elsewhere is left alone: the user put it there.
func Detect() (Installer, bool) {
	exe, err := os.Executable()
	if err != nil {
		return Installer{}, false
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, uninstallKey, registry.QUERY_VALUE)
	if err != nil {
		return Installer{}, false
	}
	defer key.Close()
	dir, _, err := key.GetStringValue("InstallLocation")
	if err != nil || dir == "" || !strings.EqualFold(filepath.Clean(dir), filepath.Dir(exe)) {
		return Installer{}, false
	}
	return Installer{Kind: "windows", Asset: InstallerName, Restarts: true, run: runInstaller}, true
}

// runInstaller starts the downloaded installer silently and returns at once. The installer
// asks this app to quit, replaces it, keeps the user's choices and starts it again.
func runInstaller(path string) error {
	cmd := exec.Command(path, "/S", "/RELAUNCH")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200} // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// Relaunch is not needed on Windows: the installer starts the new version.
func Relaunch() error { return nil }
