package desktopnote

import "path/filepath"

// ToastID is the sender id of the app's Windows toasts. It is not the Fyne app id: Windows
// keeps the name it first saw for a sender, and toasts under the app id were sent before
// any name was registered, so they show the bare id for good.
const ToastID = "Dardrai.AirPodsTray"

// RegistryKey is where Windows looks up the name and icon of a toast sender that has no
// Start menu shortcut of its own, under HKEY_CURRENT_USER.
const RegistryKey = `Software\Classes\AppUserModelId\` + ToastID

// oldRegistryKey is what an earlier build registered under the app id; Register removes it.
const oldRegistryKey = `Software\Classes\AppUserModelId\` + AppID

// iconFile is the sender's icon, next to the single-instance lock in the user cache directory.
const iconFile = "notify.png"

// IconPath is where Register keeps the sender's icon.
func IconPath(cacheDir func() (string, error)) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "airpods-tray", iconFile), nil
}
