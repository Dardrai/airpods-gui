package ui

import (
	"github.com/Dardrai/airpods-gui/core/disconnect"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

// After a routing release the dongle declines the AirPods' incoming connections until a
// reconnect, a button press or a takeover. The window keeps what the last Disconnect
// outcome said, with no reads of its own: the status report shows the connection closing
// and coming back, and the app follows a pending release until it closes.

// DisconnectSent shows that a disconnect is on its way, and holds the page's button until
// DisconnectDone.
func (w *Window) DisconnectSent() {
	w.disconnecting = true
	w.say(i18n.T("device.disconnect") + ": " + i18n.T("device.sending"))
	w.device.update(w.state)
}

// DisconnectDone shows how a disconnect ended: the outcome when problem is empty, else
// the problem.
func (w *Window) DisconnectDone(outcome disconnect.Outcome, problem string) {
	w.disconnecting = false
	name := i18n.T("device.disconnect") + ": "
	switch {
	case problem != "":
		w.say(name + problem)
	case outcome == disconnect.Released || outcome == disconnect.ReleasePending:
		w.SetRelease(outcome)
		w.sayBriefly(name + OutcomeText(outcome))
	}
	w.device.update(w.state)
}

// OutcomeText words an accepted release.
func OutcomeText(outcome disconnect.Outcome) string {
	if outcome == disconnect.ReleasePending {
		return i18n.T("disconnect.pending")
	}
	return i18n.T("disconnect.released")
}

// SetRelease records how far a release has got: disconnect.Released, ReleasePending, or 0
// once it has ended. A pending release whose connection is already gone has closed.
func (w *Window) SetRelease(outcome disconnect.Outcome) {
	if outcome == disconnect.ReleasePending && w.state.Present && !w.state.Status.Connected {
		outcome = disconnect.Released
	}
	if outcome != disconnect.Released && outcome != disconnect.ReleasePending {
		outcome = 0
	}
	if outcome == w.release {
		return
	}
	w.release, w.releasePeer = outcome, w.state.Status.Peer
	w.device.update(w.state)
}

// Release is the release the window knows of, 0 for none.
func (w *Window) Release() disconnect.Outcome { return w.release }

// followRelease moves the release along with a new state, before the window shows it. The
// connection closing completes a pending release; a new connection, another pair or no
// dongle ends any release.
func (w *Window) followRelease(st device.State) {
	switch {
	case w.release == 0:
	case !st.Present || st.Status.Peer != w.releasePeer:
		w.release = 0
	case w.release == disconnect.ReleasePending && !st.Status.Connected:
		w.release = disconnect.Released
	case w.release == disconnect.Released && st.Status.Connected && !w.state.Status.Connected:
		w.release = 0
	}
}
