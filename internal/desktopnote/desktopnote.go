// Package desktopnote sends a desktop notification under the app's own name. The
// toolkit names the sender by its app id, which notification daemons show as is.
package desktopnote

import "errors"

const (
	// AppID is the app's Fyne id, which names its preferences directory.
	AppID = "io.github.dardrai.airpods-gui"
	// appName is what the daemon shows as the sender.
	appName = "AirPods"
	// desktopEntry is the installed desktop file without its suffix; daemons take the
	// name and icon from it. The icon has the same name.
	desktopEntry = "airpods-tray"
)

// ErrUnsupported means this system has no sender of its own; the caller uses the toolkit's.
var ErrUnsupported = errors.New("desktopnote: not supported on this system")
