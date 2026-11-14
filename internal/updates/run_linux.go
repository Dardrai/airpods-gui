//go:build linux

package updates

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// Detect offers the package manager to a copy installed from the app's package.
func Detect() (Installer, bool) {
	exe, err := os.Executable()
	if err != nil {
		return Installer{}, false
	}
	s := system{exe: exe, arch: runtime.GOARCH, lookPath: exec.LookPath, output: func(name string, args ...string) (string, error) {
		out, err := exec.Command(name, args...).Output()
		return string(out), err
	}}
	kind, asset, command, ok := linuxPlan(s)
	if !ok {
		return Installer{}, false
	}
	return Installer{Kind: kind, Asset: asset, run: func(path string) error { return runPackageManager(command, path) }}, true
}

// runPackageManager installs the package through pkexec, which shows the desktop's
// password prompt, and waits for it.
func runPackageManager(command []string, path string) error {
	cmd := exec.Command(command[0], append(command[1:], path)...)
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if errors.As(err, &exit) && (exit.ExitCode() == 126 || exit.ExitCode() == 127) {
		// pkexec: 126 when the user dismissed the prompt, 127 when not authorized.
		return ErrCancelled
	}
	if err != nil {
		text := strings.TrimSpace(string(out))
		if len(text) > 300 {
			text = text[len(text)-300:]
		}
		return fmt.Errorf("%s: %w: %s", command[1], err, text)
	}
	return nil
}

// Relaunch starts the new version once this process has ended, so that the single-instance
// guard lets it run. It does not wait; the caller quits right after.
func Relaunch() error {
	script := `while kill -0 "$1" 2>/dev/null; do sleep 0.2; done; exec "$2"`
	cmd := exec.Command("/bin/sh", "-c", script, "sh", strconv.Itoa(os.Getpid()), linuxExe)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
