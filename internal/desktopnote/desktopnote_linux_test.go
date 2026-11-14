//go:build linux

package desktopnote

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

// The daemon reads the sender from the first argument and the desktop-entry hint, so
// neither may fall back to the app id.
func TestArgsNameTheApp(t *testing.T) {
	got := args("Connected", "Left 80%", 7)
	if len(got) != 8 {
		t.Fatalf("%d arguments, Notify takes 8", len(got))
	}
	if got[0] != "AirPods" || got[2] != "airpods-tray" {
		t.Errorf("app name %v icon %v", got[0], got[2])
	}
	if got[3] != "Connected" || got[4] != "Left 80%" {
		t.Errorf("summary %v body %v", got[3], got[4])
	}
	hints, ok := got[6].(map[string]dbus.Variant)
	if !ok || hints["desktop-entry"].Value() != "airpods-tray" {
		t.Errorf("hints %v", got[6])
	}
	// A state note replaces the last one, leaves by itself and stays out of the history.
	if got[1] != uint32(7) || got[7] != int32(6000) || hints["transient"].Value() != true {
		t.Errorf("replaces %v expiry %v hints %v", got[1], got[7], hints)
	}
	if sig := dbus.SignatureOf(got...).String(); sig != "susssasa{sv}i" {
		t.Errorf("signature %q", sig)
	}
}
