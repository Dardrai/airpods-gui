package tray

import (
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/systray"

	"github.com/Dardrai/airpods-gui/internal/icon"
)

const trayPNGSize = 64

// DesktopHost adapts a Fyne desktop app. Fyne has no tooltip call and reduces Windows
// icons to one size, so those go to the same systray instance Fyne drives.
type DesktopHost struct{ desktop.App }

// SetTrayTooltip implements Host.
func (DesktopHost) SetTrayTooltip(text string) { systray.SetTooltip(text) }

// SetTrayIcon implements Host, tinted for the system's light or dark mode, the best guess at
// the panel that a tray host allows: PNG through Fyne, which also remembers it for when the
// tray finishes starting, then the multi-size ICO on Windows.
func (h DesktopHost) SetTrayIcon(v icon.Variant) {
	v.LightPanel = fyne.CurrentApp().Settings().ThemeVariant() == theme.VariantLight
	h.App.SetSystemTrayIcon(fyne.NewStaticResource("tray.png", icon.PNG(v, trayPNGSize)))
	if runtime.GOOS == "windows" {
		systray.SetIcon(icon.ICO(v))
	}
}

// OnTapped sets what a primary click on the tray icon does. It goes around Fyne's
// tray window support, which would show the window behind the app's back.
func OnTapped(action func()) { systray.SetOnTapped(action) }
