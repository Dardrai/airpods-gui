package app

import "time"

// TrayWait is how long a --tray start waits for a tray host; at login it can come up
// a little after the app.
const TrayWait = 5 * time.Second

// ShouldOpenWindow decides whether the window opens at start. Without a tray there
// would be no other way to reach the app.
func ShouldOpenWindow(trayFlag, trayAvailable bool) bool { return !trayFlag || !trayAvailable }

// WaitForTray polls available until it holds or the wait runs out.
func WaitForTray(available func() bool, wait, poll time.Duration, sleep func(time.Duration)) bool {
	for waited := time.Duration(0); ; waited += poll {
		if available() {
			return true
		}
		if waited >= wait {
			return false
		}
		sleep(poll)
	}
}

// InstanceMessage is what a second process sends: a plain start asks for the window,
// an autostart with --tray only checks that an instance runs.
func InstanceMessage(trayFlag bool, show, none string) string {
	if trayFlag {
		return none
	}
	return show
}
