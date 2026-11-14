//go:build linux

package desktopnote

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
)

// callTimeout keeps a notification daemon that does not answer from holding the caller.
const callTimeout = 2 * time.Second

// expiry is how long a note stays on screen. The notes report a state, not a message to
// come back to, so they also stay out of the notification history.
const expiry = 6 * time.Second

// last is the id of the note on screen; the next one takes its place, so that connecting
// does not sit above a note still saying disconnected.
var last atomic.Uint32

// Send shows a notification through the session's notification service.
func Send(title, text string) error {
	conn, err := dbus.SessionBus() // shared connection, not ours to close
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	obj := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	call := obj.CallWithContext(ctx, "org.freedesktop.Notifications.Notify", 0, args(title, text, last.Load())...)
	if call.Err != nil {
		return call.Err
	}
	var id uint32
	if call.Store(&id) == nil {
		last.Store(id)
	}
	return nil
}

// args are the Notify arguments in order: app name, replaced id, icon, summary, body,
// actions, hints and the expiry in milliseconds.
func args(title, text string, replaces uint32) []any {
	hints := map[string]dbus.Variant{
		"desktop-entry": dbus.MakeVariant(desktopEntry),
		"transient":     dbus.MakeVariant(true),
	}
	return []any{appName, replaces, desktopEntry, title, text, []string{}, hints, int32(expiry / time.Millisecond)}
}
