//go:build windows

package winpath

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// environmentKey holds the user's own variables, under HKEY_CURRENT_USER.
const environmentKey = "Environment"

// Update adds dir to the user's PATH, or removes it, and tells running programs that the
// environment changed, so that a terminal opened afterwards sees it. The value keeps its
// registry type, and its other entries stay unexpanded.
func Update(dir string, add bool) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, environmentKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	list, kind, err := key.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		list, kind, err = "", registry.EXPAND_SZ, nil
	}
	if err != nil {
		return err
	}
	edit := Remove
	if add {
		edit = Add
	}
	result, changed := edit(list, dir)
	if !changed {
		return nil
	}
	if kind == registry.SZ {
		err = key.SetStringValue("Path", result)
	} else {
		err = key.SetExpandStringValue("Path", result)
	}
	if err != nil {
		return err
	}
	announce()
	return nil
}

// announce is best effort: without it only programs started after the next sign-in see the
// change.
func announce() {
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001a
		abortIfHung     = 0x0002
	)
	name, err := syscall.UTF16PtrFromString(environmentKey)
	if err != nil {
		return
	}
	var result uintptr
	windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW").Call(
		hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(name)), abortIfHung, 5000, uintptr(unsafe.Pointer(&result)))
}
