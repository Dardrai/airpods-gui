package ui

import (
	"strings"
	"testing"

	"github.com/Dardrai/airpods-gui/core/disconnect"
	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

// policyRequests counts kind 23 requests: reads [23] and saves [23 mode].
func policyRequests(dev *sim.Device) (reads, saves int) {
	for _, commit := range dev.Commits() {
		if len(commit) >= 1 && len(commit) <= 2 && commit[0] == 23 {
			if len(commit) == 1 {
				reads++
			} else {
				saves++
			}
		}
	}
	return reads, saves
}

// openConnection shows the Connection section, which reads the policy once.
func openConnection(r *rig) {
	r.experiments(true)
	r.main(func() { r.w.connectionRow.fold.Toggle() })
	r.until(func() bool { return !r.w.device.policy.busy && r.w.device.policy.read != nil }, "policy read")
}

// idleDemo is the demo dongle connected and idle: the dongle saves its policy only then.
func idleDemo() sim.Config {
	cfg := sim.Demo()
	cfg.Flags = 0x03
	return cfg
}

func TestThePolicyIsReadOnDemandOnly(t *testing.T) {
	r := newRig(t, idleDemo())
	r.shown()
	r.settle()
	if reads, _ := policyRequests(r.dev); reads != 0 {
		t.Fatalf("%d policy reads with the section closed", reads)
	}
	openConnection(r)
	r.until(func() bool { return r.w.device.policy.known != nil }, "known")
	r.settle()
	if reads, saves := policyRequests(r.dev); reads != 1 || saves != 0 {
		t.Errorf("%d reads, %d saves", reads, saves)
	}
	p := r.w.device.policy
	r.main(func() {
		if p.choice.Selected != i18n.T("policy.manual") || p.choice.Disabled() {
			t.Errorf("shows %q, disabled %v", p.choice.Selected, p.choice.Disabled())
		}
		p.readPolicy()
	})
	r.until(func() bool { return !p.busy }, "read again")
	if reads, _ := policyRequests(r.dev); reads != 2 {
		t.Errorf("Read did not read: %d", reads)
	}
}

func TestAutomaticAsksFirstAndIsVerified(t *testing.T) {
	r := newRig(t, idleDemo())
	openConnection(r)
	p := r.w.device.policy
	r.until(func() bool { return p.known != nil }, "known")
	asked := ""
	r.main(func() {
		r.w.confirm = func(_, text, _ string, answer func(bool)) { asked = text; answer(false) }
		p.choice.SetSelected(i18n.T("policy.automatic"))
	})
	r.flush()
	if _, saves := policyRequests(r.dev); saves != 0 || !strings.Contains(asked, "phone") {
		t.Fatalf("declined, yet %d saves; asked %q", saves, asked)
	}
	r.main(func() {
		if p.choice.Selected != i18n.T("policy.manual") {
			t.Errorf("a declined choice stays shown: %q", p.choice.Selected)
		}
		r.w.confirm = func(_, _, _ string, answer func(bool)) { answer(true) }
		p.choice.SetSelected(i18n.T("policy.automatic"))
	})
	r.until(func() bool { return !p.busy && p.status.Text != i18n.T("device.sending") }, "saved")
	r.main(func() {
		if p.status.Text != i18n.T("policy.saved") || !p.automatic() {
			t.Errorf("status %q, known %v", p.status.Text, p.known)
		}
	})
	if _, saves := policyRequests(r.dev); saves != 1 {
		t.Errorf("%d saves", saves)
	}
	// Away, the card says the dongle comes back by itself.
	r.dev.Update(func(c *sim.Config) { c.Flags = 0 })
	r.until(func() bool { return !r.w.state.Status.Connected }, "away")
	r.main(func() {
		if got := r.w.device.leaveRow.Hint.Text; got != i18n.T("device.awayAutoHint") {
			t.Errorf("hint %q", got)
		}
		r.w.SetRelease(disconnect.Released)
		if got := r.w.device.leaveRow.Hint.Text; got != i18n.T("device.releasedAutoHint") {
			t.Errorf("released hint %q", got)
		}
	})
}

func TestPolicyOutcomes(t *testing.T) {
	for name, tc := range map[string]struct {
		change func(*sim.Config)
		want   string
		known  bool
	}{
		"unsupported": {func(c *sim.Config) { c.ReconnectPolicyUnsupported = true }, "policy.unavailable", false},
		// No answer to the save: shown as sent, with no warning.
		"unverified": {func(c *sim.Config) { c.ReconnectPolicyWriteFails = true }, "device.sent", false},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, idleDemo())
			openConnection(r)
			p := r.w.device.policy
			r.until(func() bool { return p.known != nil }, "known")
			r.dev.Update(tc.change)
			r.main(func() { p.save(dongle.ReconnectManual + 1) })
			r.until(func() bool { return !p.busy && p.status.Text != i18n.T("device.sending") }, "saved")
			r.main(func() {
				want := ""
				if tc.want != "" {
					want = i18n.T(tc.want)
				}
				if p.status.Text != want || (p.known != nil) != tc.known {
					t.Errorf("status %q, known %v", p.status.Text, p.known)
				}
			})
			r.flush()
			if _, saves := policyRequests(r.dev); saves > 1 {
				t.Errorf("%d saves: an uncertain save must never be retried", saves)
			}
		})
	}
}

func TestPolicyIsBusyWhileTheDongleIsNotIdle(t *testing.T) {
	r := newRig(t, idleDemo())
	openConnection(r)
	p := r.w.device.policy
	r.until(func() bool { return p.known != nil }, "known")
	r.dev.SetFlags(0x03 | 64) // USB playback
	r.until(func() bool { return r.w.state.Status.USBPlayback }, "playing")
	r.main(func() {
		if !p.choice.Disabled() || p.read.Disabled() {
			t.Error("saving must wait for idle; reading need not")
		}
		// Had it been sent anyway, the dongle's busy answer is explained.
		p.save(dongle.ReconnectAutomatic)
	})
	r.until(func() bool { return !p.busy && p.status.Text != i18n.T("device.sending") }, "refused")
	r.main(func() {
		if p.status.Text != i18n.T("policy.busy") || p.automatic() {
			t.Errorf("status %q", p.status.Text)
		}
	})
}

// A takeover the dongle accepted but has not finished shows on the card, and nothing is sent twice.
func TestTakeoverInProgressShowsOnTheCard(t *testing.T) {
	cfg := sim.Demo()
	cfg.Flags = 0
	cfg.RoutingTakeoverDelay = 1000
	r := newRigWith(t, cfg, func(o *Options) { o.Disconnect = func() {} })
	r.until(func() bool { return r.w.state.Present && !r.w.state.Status.Connected }, "away")
	d := r.w.device
	r.main(func() {
		if d.disconnect.Text != i18n.T("device.reconnect") {
			t.Fatalf("card offers %q", d.disconnect.Text)
		}
		r.w.StartReconnect(nil)
	})
	r.untilWithin(15e9, func() bool { return !r.w.reconnecting }, "takeover answered")
	r.main(func() {
		if !r.w.takeover || d.leaveRow.Hint.Text != i18n.T("device.takingOverHint") || !d.disconnect.Disabled() {
			t.Errorf("takeover %v, hint %q, disabled %v", r.w.takeover, d.leaveRow.Hint.Text, d.disconnect.Disabled())
		}
		if !strings.HasSuffix(r.w.banner.Text, i18n.T("reconnect.pending")) {
			t.Errorf("banner %q", r.w.banner.Text)
		}
	})
	if n := len(r.dev.Controls()); n != 0 {
		t.Errorf("%d control commands", n)
	}
}

// A busy answer to pairing may follow an erased pairing: it is never sent again, and shown
// as sent with no dialog, since the status report tells what happened.
func TestPairBusyIsShownAsSent(t *testing.T) {
	cfg := sim.Demo()
	cfg.ControlResults = map[byte]byte{dongle.OpPair: dongle.ResultBusy}
	r := newRig(t, cfg)
	informed := ""
	r.main(func() {
		r.w.inform = func(_, text string) { informed = text }
		r.w.confirm = func(_, _, _ string, answer func(bool)) { answer(true) }
		r.w.device.requestPair()
	})
	r.until(func() bool { return strings.HasSuffix(r.w.banner.Text, i18n.T("device.sent")) }, "answer")
	if informed != "" {
		t.Errorf("dialog %q", informed)
	}
	if n := len(r.dev.Controls()); n != 1 {
		t.Errorf("%d commands: never resent", n)
	}
}

func TestGamePhaseWords(t *testing.T) {
	for phase, want := range map[byte]string{
		dongle.GameModePending: "Waiting for playback", dongle.GameModeConfirmed: "On", dongle.GameModeRestoring: "Turning off", 42: "Unknown",
	} {
		if got := gamePhaseText(phase); got != want {
			t.Errorf("%d: %q", phase, got)
		}
	}
}
