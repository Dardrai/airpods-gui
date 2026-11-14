//go:build linux

package tray

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
)

const watcherName = "org.kde.StatusNotifierWatcher"

// Available reports whether a tray host is running. Linux trays go through the
// StatusNotifier watcher, which stock GNOME lacks without the AppIndicator extension.
func Available() bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := dbus.SessionBus()
	if err != nil {
		return false
	}
	var owned bool
	call := conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", 0, watcherName)
	if call.Store(&owned) != nil {
		return false
	}
	return owned
}
