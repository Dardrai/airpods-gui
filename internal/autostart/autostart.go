// Package autostart registers the tray app to start with the user's session, in the
// tray: a registry Run value on Windows, an autostart .desktop file on Linux.
package autostart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// RunKey and RunValue locate the Windows entry under HKEY_CURRENT_USER.
	RunKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	RunValue = "AirPodsTray"
	// DesktopFile is the Linux entry's file name.
	DesktopFile = "airpods-tray.desktop"
)

// Manager turns the entry on and off. SetEnabled reads its result back and fails if
// the entry is not what was asked for.
type Manager interface {
	Enabled() (bool, error)
	SetEnabled(bool) error
	// Stale reports an entry that exists but would start something other than this
	// executable, as after moving it. Turning the option on repairs it.
	Stale() (bool, error)
}

// WindowsCommand is the Run value's data for an executable.
func WindowsCommand(exe string) string { return `"` + exe + `" --tray` }

// ValueStore is one registry value.
type ValueStore interface {
	Get() (value string, present bool, err error)
	Set(string) error
	Delete() error
}

// RunEntry is the Windows entry on top of a ValueStore.
type RunEntry struct {
	Store ValueStore
	Exe   string
}

// Enabled reports an entry of ours; one pointing at another path counts as off, so
// turning it on repairs it after the executable moved.
func (r RunEntry) Enabled() (bool, error) {
	value, present, err := r.Store.Get()
	if err != nil || !present {
		return false, err
	}
	return value == WindowsCommand(r.Exe), nil
}

// Stale implements Manager.
func (r RunEntry) Stale() (bool, error) {
	value, present, err := r.Store.Get()
	if err != nil {
		return false, err
	}
	return present && value != WindowsCommand(r.Exe), nil
}

// SetEnabled implements Manager.
func (r RunEntry) SetEnabled(on bool) error {
	if on {
		if err := r.Store.Set(WindowsCommand(r.Exe)); err != nil {
			return err
		}
	} else if err := r.Store.Delete(); err != nil {
		return err
	}
	value, present, err := r.Store.Get()
	switch {
	case err != nil:
		return err
	case on && value != WindowsCommand(r.Exe):
		return fmt.Errorf("the Run value reads back as %q", value)
	case !on && present:
		return errors.New("the Run value is still there")
	}
	return nil
}

// DesktopPath locates the autostart file. A relative XDG_CONFIG_HOME is invalid by
// the XDG spec and ignored.
func DesktopPath(xdgConfigHome, home string) string {
	base := filepath.Join(home, ".config")
	if filepath.IsAbs(xdgConfigHome) {
		base = xdgConfigHome
	}
	return filepath.Join(base, "autostart", DesktopFile)
}

// quoteExec applies the Desktop Entry rules: arguments with reserved characters are
// double-quoted with \ " ` $ escaped, and the key's value escapes each \ once more.
func quoteExec(path string) string {
	if !strings.ContainsAny(path, " \t\n\"'\\><~|&;$*?#()`%") {
		return path
	}
	var quoted strings.Builder
	quoted.WriteByte('"')
	for _, r := range path {
		if strings.ContainsRune("\"`$\\", r) {
			quoted.WriteByte('\\')
		}
		quoted.WriteRune(r)
	}
	quoted.WriteByte('"')
	value := strings.ReplaceAll(quoted.String(), `\`, `\\`)
	return strings.ReplaceAll(value, "%", "%%")
}

// DesktopEntry is the autostart file's content.
func DesktopEntry(exe string) string {
	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=AirPods Tray\n" +
		"Comment=Tray app for the AirPods USB dongle\n" +
		"Exec=" + quoteExec(exe) + " --tray\n" +
		"Terminal=false\n" +
		"X-GNOME-Autostart-enabled=true\n"
}

// DesktopEntryFile is the Linux entry.
type DesktopEntryFile struct {
	Path string
	Exe  string
}

// Enabled implements Manager; a file for another path counts as off.
func (d DesktopEntryFile) Enabled() (bool, error) {
	content, err := os.ReadFile(d.Path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return string(content) == DesktopEntry(d.Exe), nil
}

// Stale implements Manager.
func (d DesktopEntryFile) Stale() (bool, error) {
	content, err := os.ReadFile(d.Path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return string(content) != DesktopEntry(d.Exe), nil
}

// SetEnabled implements Manager.
func (d DesktopEntryFile) SetEnabled(on bool) error {
	if on {
		if err := os.MkdirAll(filepath.Dir(d.Path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(d.Path, []byte(DesktopEntry(d.Exe)), 0o644); err != nil {
			return err
		}
	} else if err := os.Remove(d.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	enabled, err := d.Enabled()
	if err != nil {
		return err
	}
	if enabled != on {
		return fmt.Errorf("%s does not read back as expected", d.Path)
	}
	return nil
}

func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Abs(exe)
}
