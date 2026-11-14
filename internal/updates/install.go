package updates

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// sumsAsset lists the SHA-256 of every file in a release.
	sumsAsset = "SHA256SUMS"
	// maxInstaller bounds the download; the installer is about 12 MB.
	maxInstaller = 200 << 20
	// downloadTimeout bounds the whole download.
	downloadTimeout = 10 * time.Minute
)

// uninstallKey is where the installer records itself, under HKEY_CURRENT_USER; a test keeps
// it equal to build/windows/installer.nsi.
const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\AirPodsTray`

// ErrNoInstaller means the release has no file for this system, or no checksum for it.
var ErrNoInstaller = errors.New("updates: the release has no file for this system")

// ErrCancelled means the password prompt was closed or refused; nothing was installed.
var ErrCancelled = errors.New("updates: the password prompt was cancelled")

// Installer replaces the running copy with a downloaded release: the Windows installer, or
// the distribution's package manager behind a password prompt on Linux.
type Installer struct {
	// Kind names it in the log: "windows", "pacman", "apt", "dnf", "zypper" or "rpm".
	Kind string
	// Asset names the release file it takes.
	Asset func(tag string) string
	// Restarts is set when the installer starts the new version itself; otherwise the app
	// starts it with Relaunch once Run returns.
	Restarts bool
	run      func(path string) error
}

// Run installs the downloaded file. On Linux it waits for the password prompt and the
// package manager; on Windows it starts the installer and returns.
func (i Installer) Run(path string) error { return i.run(path) }

// CanInstall reports whether this copy can update itself.
func CanInstall() bool {
	_, ok := Detect()
	return ok
}

// DownloadDir makes a private folder for one download: only this user can write to it, so
// nothing can swap the file before it is installed.
func DownloadDir() (string, error) { return os.MkdirTemp("", "airpods-tray-update-") }

// InstallerName is the Windows installer's file name in a release.
func InstallerName(tag string) string {
	return "airpods-gui-" + strings.TrimPrefix(tag, "v") + "-setup-x64.exe"
}

// Download fetches the named release file into dir and checks it against the release's
// SHA256SUMS. It returns the file's path; a file that does not match is removed.
func Download(ctx context.Context, client *http.Client, rel Release, dir, name string) (string, error) {
	installer, ok := rel.Asset(func(n string) bool { return n == name })
	if !ok {
		return "", ErrNoInstaller
	}
	sums, ok := rel.Asset(func(n string) bool { return n == sumsAsset })
	if !ok {
		return "", ErrNoInstaller
	}
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	list, err := get(ctx, client, sums.URL, 1<<16)
	if err != nil {
		return "", err
	}
	want, ok := sumFor(string(list), name)
	if !ok {
		return "", ErrNoInstaller
	}
	path := filepath.Join(dir, filepath.Base(name))
	if err := fetchTo(ctx, client, installer.URL, path, want); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

// sumFor finds a file's hash in sha256sum output.
func sumFor(list, name string) (string, bool) {
	scanner := bufio.NewScanner(strings.NewReader(list))
	for scanner.Scan() {
		hash, file, ok := strings.Cut(strings.TrimSpace(scanner.Text()), " ")
		file = strings.TrimPrefix(strings.TrimSpace(file), "*")
		if ok && file == name && len(hash) == sha256.Size*2 {
			if _, err := hex.DecodeString(hash); err == nil {
				return strings.ToLower(hash), true
			}
		}
	}
	return "", false
}

func request(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	if !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("updates: not an https download: %q", url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("updates: download answered %s", resp.Status)
	}
	return resp, nil
}

func get(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	resp, err := request(ctx, client, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

func fetchTo(ctx context.Context, client *http.Client, url, path, wantSum string) error {
	resp, err := request(ctx, client, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, maxInstaller+1))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	switch {
	case err != nil:
		return err
	case n > maxInstaller:
		return errors.New("updates: the installer is larger than expected")
	case hex.EncodeToString(hash.Sum(nil)) != wantSum:
		return errors.New("updates: the installer does not match the release's SHA256SUMS")
	}
	return nil
}
