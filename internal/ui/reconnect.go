package ui

import (
	"errors"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/disconnect"
	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

// takeoverLimit is how long the Disconnect card says a takeover is under way when the status
// shows no connection: past it, the card offers Reconnect again.
const takeoverLimit = 20 * time.Second

// ReconnectJob takes the AirPods back. On dongles with the routing handover it asks for them
// through it and waits for the dongle to own them: outcome is then disconnect.Acquired or
// AcquirePending. Older firmware gets the plain reconnect command (opcode 1), and outcome
// stays 0. Both also end a release.
func ReconnectJob(outcome *disconnect.Outcome) device.Job {
	return func(c *dongle.Client) error {
		*outcome = 0
		status, err := c.Status()
		if err != nil {
			return err
		}
		if status.Has(dongle.CapRouting) {
			*outcome, err = disconnect.Acquire(c)
			if !errors.Is(err, disconnect.ErrAcquireUnsupported) {
				return err
			}
			*outcome = 0
		}
		return c.Reconnect()
	}
}

// ReconnectText words a reconnect the dongle took: owned, under way, or sent as the plain
// command, which has no answer beyond being taken.
func ReconnectText(outcome disconnect.Outcome) string {
	switch outcome {
	case disconnect.Acquired:
		return i18n.T("reconnect.acquired")
	case disconnect.AcquirePending:
		return i18n.T("reconnect.pending")
	}
	return i18n.T("device.sent")
}

// ReconnectProblem words a failed reconnect. An uncertain outcome comes first, since it can
// wrap the others.
func ReconnectProblem(err error) string {
	switch {
	case device.IsUnconfirmed(err):
	case errors.Is(err, disconnect.ErrIntentChanged):
		return i18n.T("disconnect.intentChanged")
	case device.IsBusy(err) && !errors.Is(err, dongle.ErrNotSent):
		return i18n.T("reconnect.busy")
	}
	return device.Explain(err)
}

// StartReconnect runs Reconnect for the window and the tray. The window shows how it went;
// notify, when not nil, also hears of a problem, for the tray, which has no place for one.
func (w *Window) StartReconnect(notify func(title, text string)) {
	var outcome disconnect.Outcome
	w.reconnecting = true
	name := i18n.T("device.reconnect")
	w.say(name + ": " + i18n.T("device.sending"))
	w.device.update(w.state)
	w.owner.SubmitCommand(ReconnectJob(&outcome), func(err error) {
		w.do(func() {
			problem := ""
			if err != nil {
				problem = ReconnectProblem(err)
				if notify != nil {
					notify(i18n.T("tray.reconnect"), problem)
				}
			}
			w.ReconnectDone(outcome, problem)
		})
	})
}

// ReconnectDone shows how a reconnect ended: the outcome when problem is empty, else the problem.
func (w *Window) ReconnectDone(outcome disconnect.Outcome, problem string) {
	w.reconnecting = false
	name := i18n.T("device.reconnect") + ": "
	if problem != "" {
		w.say(name + problem)
	} else {
		w.SetRelease(0)
		w.takeover = outcome == disconnect.AcquirePending && !w.state.Status.Connected
		w.takeoverPeer, w.takeoverUntil = w.state.Status.Peer, time.Now().Add(takeoverLimit)
		w.sayBriefly(name + ReconnectText(outcome))
	}
	w.device.update(w.state)
}

// followTakeover ends the takeover note once the AirPods are connected, another pair or no
// dongle shows, or it has run too long to be believed.
func (w *Window) followTakeover(st device.State) {
	if w.takeover && (!st.Present || st.Status.Connected || st.Status.Peer != w.takeoverPeer || time.Now().After(w.takeoverUntil)) {
		w.takeover = false
	}
}

// policyBox is the reconnect policy control in the Connection section. It reads the policy only
// when the section opens or Read is pressed, never with the status poll.
type policyBox struct {
	w         *Window
	choice    *widget.Select
	read      *widget.Button
	status    *widget.Label
	statusBox fyne.CanvasObject
	// known is the policy last read or saved on this dongle; nil when not known.
	known  *dongle.ReconnectPolicy
	serial string
	busy   bool
	// loading is set while the page, not the user, moves the choice.
	loading bool
}

var policyChoices = []struct {
	policy dongle.ReconnectPolicy
	key    string
}{{dongle.ReconnectManual, "policy.manual"}, {dongle.ReconnectAutomatic, "policy.automatic"}}

func newPolicyBox(w *Window) *policyBox {
	p := &policyBox{w: w}
	p.choice = widget.NewSelect(nil, p.chosen)
	p.read = w.button("policy.read", p.readPolicy)
	p.status, p.statusBox = statusLine()
	w.bind(p.relabel)
	return p
}

func (p *policyBox) relabel() {
	p.loading = true
	defer func() { p.loading = false }()
	var labels []string
	for _, c := range policyChoices {
		labels = append(labels, i18n.T(c.key))
	}
	p.choice.SetOptions(labels)
	p.choice.PlaceHolder = i18n.T("value.unknown")
	if p.known == nil {
		p.choice.ClearSelected()
	} else {
		p.choice.SetSelected(labels[*p.known])
	}
}

func (p *policyBox) policyOf(label string) (dongle.ReconnectPolicy, bool) {
	for _, c := range policyChoices {
		if i18n.T(c.key) == label {
			return c.policy, true
		}
	}
	return 0, false
}

// follow forgets the policy when the dongle goes or another one comes.
func (p *policyBox) follow(st device.State) {
	if !st.Present || st.Serial != p.serial {
		p.serial = st.Serial
		if p.known != nil {
			p.known = nil
			p.relabel()
			p.say("")
		}
	}
	p.updateEnabled()
}

// idle is what the dongle needs to save: no playback, no microphone, no phone audio.
func idle(status dongle.Status) bool {
	return !status.USBPlayback && !status.Streaming && !status.MicrophoneActive
}

func (p *policyBox) readable() bool {
	st := p.w.state
	return st.Present && st.Status.Has(dongle.CapTypedRPC) && !p.busy
}

func (p *policyBox) updateEnabled() {
	enableWidget(p.read, p.readable())
	// The app must not hold a lease of its own while saving, so nothing else of its own runs.
	enableWidget(p.choice, p.readable() && p.known != nil && !p.w.hearingBusy && idle(p.w.state.Status))
}

func (p *policyBox) say(text string) {
	setStatus(p.statusBox, p.status, text)
	p.w.relayout()
}

// show records a policy the dongle answered with.
func (p *policyBox) show(policy dongle.ReconnectPolicy) {
	p.known = &policy
	p.relabel()
	p.w.device.update(p.w.state)
}

func (p *policyBox) forget() {
	p.known = nil
	p.relabel()
	p.w.device.update(p.w.state)
}

// readPolicy asks the dongle once. It is read-only and allowed at any time.
func (p *policyBox) readPolicy() {
	if !p.readable() {
		return
	}
	p.busy = true
	p.updateEnabled()
	var policy dongle.ReconnectPolicy
	p.w.owner.Submit(func(c *dongle.Client) (err error) {
		policy, err = c.ReconnectPolicy()
		return err
	}, func(err error) {
		p.w.do(func() {
			p.busy = false
			if err != nil {
				p.forget()
				p.say(policyProblem(err))
			} else {
				p.say("")
				p.show(policy)
			}
			p.updateEnabled()
		})
	})
}

// chosen is a pick in the list. Automatic asks first; either is saved once, never retried.
func (p *policyBox) chosen(label string) {
	if p.loading || p.known == nil {
		return
	}
	policy, ok := p.policyOf(label)
	if !ok || policy == *p.known {
		return
	}
	if policy != dongle.ReconnectAutomatic {
		p.save(policy)
		return
	}
	p.w.confirm(i18n.T("policy.autoTitle"), i18n.T("policy.autoText"), i18n.T("policy.autoAccept"), func(accepted bool) {
		if accepted {
			p.save(policy)
			return
		}
		p.relabel()
	})
}

func (p *policyBox) save(policy dongle.ReconnectPolicy) {
	p.busy = true
	p.updateEnabled()
	p.say(i18n.T("device.sending"))
	var changed bool
	p.w.owner.SubmitCommand(func(c *dongle.Client) (err error) {
		changed, err = c.SetReconnectPolicy(policy)
		return err
	}, func(err error) {
		p.w.do(func() {
			p.busy = false
			switch {
			case err == nil:
				p.show(policy)
				if changed {
					p.say(i18n.T("policy.saved"))
				} else {
					p.say(i18n.T("policy.noChange"))
				}
			case device.IsUnconfirmed(err):
				// No answer: nothing is sent again. Not read back either: until the dongle
				// restarts, a read after this can claim the policy is not available.
				p.w.logf("reconnect policy sent, no answer: %v", err)
				p.forget()
				p.say(device.UnconfirmedNote())
			case errors.As(err, new(*dongle.Unsupported)):
				// Nothing was saved, and the dongle no longer offers a policy to show.
				p.forget()
				p.say(policyProblem(err))
			default:
				// Nothing was saved; what was read still holds.
				p.relabel()
				p.say(policyProblem(err))
			}
			p.updateEnabled()
		})
	})
}

// policyProblem words a policy request that changed nothing.
func policyProblem(err error) string {
	var unsupported *dongle.Unsupported
	switch {
	case errors.As(err, &unsupported):
		return i18n.T("policy.unavailable")
	case device.IsBusy(err) && !errors.Is(err, dongle.ErrNotSent):
		return i18n.T("policy.busy")
	}
	return device.Explain(err)
}

// automatic reports whether the dongle is known to keep the Automatic policy.
func (p *policyBox) automatic() bool {
	return p.known != nil && *p.known == dongle.ReconnectAutomatic
}
