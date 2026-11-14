package updates

import (
	"strings"
)

// linuxExe is where the packages install the app; a copy anywhere else was put there by
// hand and is left alone.
const linuxExe = "/usr/bin/airpods-tray"

// packageName is the name of the app's package in every format.
const packageName = "airpods-gui"

// Package file names in a release, as build/fyne-cross.sh names them.
func archAsset(tag string) string {
	return packageName + "_" + strings.TrimPrefix(tag, "v") + "-x86_64.pkg.tar.zst"
}
func debAsset(tag string) string {
	return packageName + "_" + strings.TrimPrefix(tag, "v") + "_amd64.deb"
}
func rpmAsset(tag string) string {
	return packageName + "_" + strings.TrimPrefix(tag, "v") + ".x86_64.rpm"
}

// system is what detection asks of the computer, so that tests can answer instead.
type system struct {
	exe      string
	arch     string
	lookPath func(string) (string, error)
	output   func(name string, args ...string) (string, error)
}

func (s system) has(tool string) bool {
	_, err := s.lookPath(tool)
	return err == nil
}

// owns reports whether the package manager names the app's package as the owner of the
// installed executable.
func (s system) owns(name string, args ...string) bool {
	if !s.has(name) {
		return false
	}
	out, err := s.output(name, args...)
	return err == nil && strings.TrimSpace(out) == packageName
}

// linuxPlan picks the package format and the command that installs it. It needs the app
// running from the package's own path, an x86-64 package, and pkexec for the password
// prompt. The returned command gets the file's path appended.
func linuxPlan(s system) (kind string, asset func(string) string, command []string, ok bool) {
	if s.exe != linuxExe || s.arch != "amd64" || !s.has("pkexec") {
		return "", nil, nil, false
	}
	switch {
	case s.owns("pacman", "-Qqo", linuxExe):
		return "pacman", archAsset, []string{"pkexec", "pacman", "-U", "--noconfirm"}, true
	case s.ownsDpkg():
		if s.has("apt-get") {
			return "apt", debAsset, []string{"pkexec", "apt-get", "install", "-y", "--allow-downgrades"}, true
		}
		return "dpkg", debAsset, []string{"pkexec", "dpkg", "-i"}, true
	case s.owns("rpm", "-qf", "--queryformat", "%{NAME}", linuxExe):
		switch {
		case s.has("dnf"):
			return "dnf", rpmAsset, []string{"pkexec", "dnf", "install", "-y"}, true
		case s.has("zypper"):
			return "zypper", rpmAsset, []string{"pkexec", "zypper", "--non-interactive", "install", "--allow-unsigned-rpm"}, true
		}
		return "rpm", rpmAsset, []string{"pkexec", "rpm", "-U"}, true
	}
	return "", nil, nil, false
}

// ownsDpkg reads dpkg's "package: path" answer.
func (s system) ownsDpkg() bool {
	if !s.has("dpkg-query") {
		return false
	}
	out, err := s.output("dpkg-query", "-S", linuxExe)
	if err != nil {
		return false
	}
	name, path, ok := strings.Cut(strings.TrimSpace(out), ": ")
	return ok && name == packageName && path == linuxExe
}
