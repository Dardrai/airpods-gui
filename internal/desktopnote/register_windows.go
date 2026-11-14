//go:build windows

package desktopnote

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"

	"golang.org/x/sys/windows/registry"
)

// registered is set once the sender has its name, so that Send never shows the bare id.
var registered atomic.Bool

// Register names the toast sender "AirPods" with the app's icon, for this user only. It
// needs no admin rights and is redone on every start, so a moved exe or a new icon follow.
func Register(icon []byte) error {
	path, err := IconPath(os.UserCacheDir)
	if err != nil {
		return err
	}
	if old, err := os.ReadFile(path); err != nil || !bytes.Equal(old, icon) {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(path, icon, 0o600); err != nil {
			return err
		}
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, RegistryKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.SetStringValue("DisplayName", appName); err != nil {
		return err
	}
	if err := key.SetStringValue("IconUri", path); err != nil {
		return err
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, oldRegistryKey); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	registered.Store(true)
	return nil
}
