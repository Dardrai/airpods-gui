package ui

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/hearing"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

// experimentalRig is a Pro 3 pair with the experimental switch on.
func experimentalRig(t *testing.T, cfg sim.Config) *rig {
	t.Helper()
	r := newRig(t, cfg)
	r.main(func() {
		r.w.opts.Fyne.Preferences().SetBool(experimentalPreference, true)
		r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"})
		r.w.applyExperimental()
	})
	r.until(func() bool { return r.w.state.Ready() }, "ready")
	return r
}

func TestSectionsNeedTheSwitchAndTheModel(t *testing.T) {
	r := newRig(t, sim.Demo())
	section := func(i int) *pageSection { return r.w.sections[i] }
	r.main(func() {
		count := len(r.w.sections)
		hearingRow, extraRow := section(count-2), section(count-1)
		r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"})
		if hearingRow.object.Visible() || extraRow.object.Visible() {
			t.Error("shown with the experimental switch off")
		}
		for _, entry := range r.w.sections {
			if entry.object.Visible() {
				t.Errorf("%s shows with the experimental switch off", entry.fold.Title.Text)
			}
		}
		r.w.opts.Fyne.Preferences().SetBool(experimentalPreference, true)
		r.w.applyExperimental()
		if !hearingRow.object.Visible() || !extraRow.object.Visible() {
			t.Error("hidden with the switch on")
		}
		for _, entry := range r.w.sections {
			if !entry.object.Visible() {
				t.Errorf("%s is hidden with the switch on", entry.fold.Title.Text)
			}
		}
		r.w.SetInfo(dongle.Metadata{Model: "A2032", Firmware: "1A"})
		if hearingRow.object.Visible() {
			t.Error("hearing shown on a model without it")
		}
		if !extraRow.object.Visible() || r.w.extra.transparencyBox.Visible() || r.w.extra.eqBox.Visible() {
			t.Error("the experiments must follow the model's flags")
		}
	})
}

func TestHearingReadAndModeThroughTheOwner(t *testing.T) {
	r := experimentalRig(t, sim.Demo())
	h := r.w.hearing
	r.main(func() {
		if h.aid.toggle.Visible() || !h.aid.dash.Visible() || !h.aid.dash.Disabled() {
			t.Error("an unread mode must not look off, nor be tappable")
		}
		h.readAll()
	})
	r.until(func() bool { return h.have && !r.w.hearingBusy }, "read")
	r.main(func() {
		if !h.aid.toggle.Visible() || h.aid.toggle.Checked || h.edit == nil || !h.editor.Visible() {
			t.Errorf("after read: %+v", h.state)
		}
		if !h.apply.Disabled() {
			t.Error("apply enabled with nothing edited")
		}
		r.w.confirm = func(_, _, _ string, answer func(bool)) { answer(true) }
		h.aid.toggle.SetChecked(true)
	})
	r.until(func() bool { return !r.w.hearingBusy }, "set")
	r.main(func() {
		if h.state.HearingEnabled != hearing.FlagOn || h.needRead {
			t.Errorf("not confirmed on: %+v, banner %q", h.state, h.status.Text)
		}
	})
}

// statusReads counts polls of the status report, and a long job would stop it dead.
func (r *rig) statusReads() int {
	reads, _, _ := r.dev.Counts()
	return reads
}

func TestStatusPollingGoesOnWhileAModeChangeIsConfirmed(t *testing.T) {
	a := sim.NewHearingAirPods("A3063", "9A348")
	a.Silent = true
	r := experimentalRig(t, a.Config())
	h := r.w.hearing
	r.main(h.readAll)
	r.until(func() bool { return h.have && !r.w.hearingBusy }, "read")
	r.main(func() {
		r.w.confirm = func(_, _, _ string, answer func(bool)) { answer(true) }
		h.aid.toggle.SetChecked(true)
	})
	r.until(func() bool { return len(r.dev.AAPSent()) >= 4 }, "sent")
	sent := len(r.dev.AAPSent())
	before := r.statusReads()
	r.until(func() bool { return r.statusReads() >= before+20 }, "status polls during the wait")
	r.main(func() {
		if !r.w.hearingBusy {
			t.Error("the change has already ended")
		}
		if !h.read.Disabled() {
			t.Error("other hearing requests must wait for the change")
		}
	})
	r.dev.Update(func(*sim.Config) {
		a.Silent = false
		a.Report(2, []byte{4, 0, 4, 0, 9, 0, 0x2c, 1, 1, 0, 0})
		a.Report(3, []byte{4, 0, 4, 0, 9, 0, 0x33, 1, 0, 0, 0})
	})
	r.until(func() bool { return !r.w.hearingBusy }, "confirmed")
	r.main(func() {
		if h.state.HearingEnabled != hearing.FlagOn || h.needRead {
			t.Errorf("state %+v, status %q", h.state, h.status.Text)
		}
	})
	if got := len(r.dev.AAPSent()); got != sent {
		t.Errorf("sent %d packets, then %d: nothing may be resent", sent, got)
	}
	if r.dev.LeaseHeld() {
		t.Error("a lease is kept")
	}
}

func TestReconnectEndsAModeChangeUncertain(t *testing.T) {
	a := sim.NewHearingAirPods("A3063", "9A348")
	a.Silent = true
	r := experimentalRig(t, a.Config())
	h := r.w.hearing
	r.main(h.readAll)
	r.until(func() bool { return h.have && !r.w.hearingBusy }, "read")
	r.main(func() {
		r.w.confirm = func(_, _, _ string, answer func(bool)) { answer(true) }
		h.aid.toggle.SetChecked(true)
	})
	r.until(func() bool { return len(r.dev.AAPSent()) >= 4 }, "sent")
	sent := len(r.dev.AAPSent())
	r.dev.BumpGeneration()
	r.until(func() bool { return !r.w.hearingBusy }, "ended")
	r.main(func() {
		if h.recovery == nil || h.recovery.Mode != hearing.HearingAid {
			t.Errorf("recovery %+v, status %q", h.recovery, h.status.Text)
		}
	})
	if got := len(r.dev.AAPSent()); got != sent {
		t.Errorf("sent %d packets, then %d: nothing may be resent", sent, got)
	}
}

func TestStepsWithNoDongleEndAsAbsent(t *testing.T) {
	cfg := sim.Demo()
	cfg.OpenError = noDongleSim
	r := startRig(t, cfg, "")
	var cause error
	ended := make(chan struct{})
	r.main(func() {
		r.w.followSteps(func(*dongle.Client) bool {
			t.Error("a step ran with no device")
			return true
		}, func(err error) { cause = err }, func() { close(ended) })
	})
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("not ended")
	}
	if cause == nil || errors.Is(cause, device.ErrNoDongle) {
		t.Errorf("cause %v: it must not read as a request that never ran", cause)
	}
}

// The profile write leaves Hearing Aid as it is: off here, and off afterwards, with one
// question that lists the changes and asks nothing about turning anything on.
func TestApplyAsksAndLeavesHearingAidAsItIs(t *testing.T) {
	r := experimentalRig(t, sim.Demo())
	h := r.w.hearing
	asked := 0
	r.main(func() {
		r.w.confirm = func(_, text, _ string, answer func(bool)) {
			asked++
			if !strings.Contains(text, i18n.T("hearing.changesHead")) || strings.Contains(text, "stays on") {
				t.Errorf("confirmation %q", text)
			}
			answer(asked > 1)
		}
		h.readAll()
	})
	r.until(func() bool { return h.have && !r.w.hearingBusy }, "read")
	writes := len(r.dev.Exchanges())
	r.main(func() {
		h.amp.Slider.SetValue(0.75)
		if h.apply.Disabled() {
			t.Fatal("apply disabled after an edit")
		}
		h.applyDraft()
	})
	r.until(func() bool { return asked == 1 && !r.w.hearingBusy }, "declined")
	r.main(func() {
		if h.state.HearingEnabled != hearing.FlagOff {
			t.Errorf("declined, yet %v", h.state.HearingEnabled)
		}
		h.applyDraft()
	})
	r.until(func() bool { return asked == 2 && !r.w.hearingBusy }, "applied")
	r.main(func() {
		if h.state.HearingEnabled != hearing.FlagOff || h.status.Text != i18n.T("hearing.applied") {
			t.Errorf("%+v %q", h.state, h.status.Text)
		}
		if h.edit.Dirty || h.edit.Amplification != 0.75 {
			t.Errorf("the draft should start again from what read back: %+v", h.edit)
		}
	})
	if len(r.dev.Exchanges()) == writes {
		t.Error("nothing was written")
	}
	for _, packet := range r.dev.AAPSent() {
		if len(packet) > 6 && packet[4] == 9 && packet[6] == 0x2c {
			t.Errorf("a Hearing Aid packet went out: % x", packet)
		}
	}
}

// A gain that would put one ear past 1.5 is refused, never clamped: the slider goes back.
func TestOutOfRangeGainIsRejected(t *testing.T) {
	r := experimentalRig(t, sim.Demo())
	h := r.w.hearing
	r.main(h.readAll)
	r.until(func() bool { return h.have && !r.w.hearingBusy }, "read")
	r.main(func() {
		before := h.edit.Draft
		h.amp.Slider.SetValue(1.5) // the stored balance is -0.5, so the left ear would reach 1.75
		if h.edit.Draft != before || h.edit.Dirty {
			t.Errorf("the draft moved: %+v", h.edit.Draft)
		}
		if h.amp.Slider.Value != float64(h.edit.Amplification) || h.editNote.Text != i18n.T("hearing.gainRejected") {
			t.Errorf("slider %v, note %q", h.amp.Slider.Value, h.editNote.Text)
		}
		if h.amp.Slider.Min != -1.5 || h.amp.Slider.Max != 1.5 || h.balance.Slider.Min != -3 || h.balance.Slider.Max != 3 {
			t.Error("the gain sliders take the AirPods' ranges")
		}
	})
}

func TestBadAudiogramBlocksApply(t *testing.T) {
	r := experimentalRig(t, sim.Demo())
	h := r.w.hearing
	r.main(h.readAll)
	r.until(func() bool { return h.have && !r.w.hearingBusy }, "read")
	r.main(func() {
		h.bands[0][3].SetText("200")
		if !h.apply.Disabled() || !h.noteBox.Visible() {
			t.Error("an out of range value must block apply")
		}
		h.bands[0][3].SetText("40")
		if h.apply.Disabled() || h.noteBox.Visible() {
			t.Error("a fixed value must allow apply")
		}
	})
}

func TestUncertainModeChangeOffersRecoveryAndNeverResends(t *testing.T) {
	r := experimentalRig(t, sim.Demo())
	h := r.w.hearing
	r.main(h.readAll)
	r.until(func() bool { return h.have && !r.w.hearingBusy }, "read")
	before := r.dev.Writes()
	r.main(func() {
		recovery := &hearing.Recovery{Mode: hearing.HearingAid}
		err := &hearing.ModeUnconfirmed{Mode: hearing.HearingAid, On: true, Recovery: recovery, Message: "no answer"}
		h.modeDone(nil, recovery, err)
		if !h.recoverBox.Visible() || h.recoverBt.Disabled() {
			t.Error("no recovery offered")
		}
		if h.status.Text != device.UnconfirmedNote() {
			t.Errorf("wording %q", h.status.Text)
		}
		if h.aid.toggle.Visible() || !h.apply.Disabled() {
			t.Error("nothing is offered until the state is read again")
		}
	})
	r.flush()
	if after := r.dev.Writes(); after != before {
		t.Errorf("%d writes on their own", after-before)
	}
	refused := &hearing.Refused{Reason: "Set it up from an iPhone."}
	if got := (&Window{}).requestError(refused); got != refused.Reason {
		t.Errorf("refusal %q", got)
	}
	unconfirmed := &dongle.Unconfirmed{Message: "No reply.", Err: errors.New("x")}
	if got := (&Window{}).requestError(unconfirmed); got != device.UnconfirmedNote() {
		t.Errorf("uncertain %q", got)
	}
}

func inCaseConfig() sim.Config {
	cfg := sim.Demo()
	cfg.Cached[dongle.SlotEarStatus] = sim.CachedReport{Age: 1000, Revision: 1, Packet: sim.EarPacket(2, 2)}
	cfg.Flags = 0x03
	return cfg
}

// The wait must not hold the owner: status polls and queued jobs keep going all through it.
func TestRunningCheckLeavesTheOwnerFree(t *testing.T) {
	r := experimentalRig(t, inCaseConfig())
	hw := r.w.hardware
	confirmed := false
	r.main(func() {
		r.w.checkEvery = 5 * time.Millisecond
		r.w.confirm = func(_, _, _ string, answer func(bool)) {
			confirmed = true
			answer(true)
		}
		hw.askCheck()
	})
	r.until(func() bool {
		if !hw.checking {
			t.Fatalf("ended: %q", hw.checkText.Text)
		}
		return hw.progressBox.Visible() && !hw.stopWait.Hidden
	}, "waiting")
	if !confirmed {
		t.Fatal("started with no confirmation")
	}
	before, _, _ := r.dev.Counts()
	ran := make(chan struct{})
	r.owner.Submit(func(*dongle.Client) error { close(ran); return nil }, nil)
	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("the queue froze during the wait")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		now, _, _ := r.dev.Counts()
		if now > before+5 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("status polling froze during the wait")
		}
		time.Sleep(5 * time.Millisecond)
	}
	r.main(func() {
		if !r.w.hearing.read.Disabled() {
			t.Error("other hearing requests must wait for the check")
		}
		hw.stopCheck()
	})
	r.until(func() bool { return !hw.checking }, "stopped")
	r.main(func() {
		if hw.last.Phase != hearing.CheckStopped || hw.stopWait.Visible() || r.w.hearingBusy {
			t.Errorf("after stop: %+v", hw.last)
		}
		if hw.checkText.Text != i18n.T("check.stopped") {
			t.Errorf("wording %q", hw.checkText.Text)
		}
	})
}

func TestCheckRefusedWithBudsInTheEars(t *testing.T) {
	r := experimentalRig(t, sim.Demo())
	hw := r.w.hardware
	r.main(hw.beginCheck)
	r.until(func() bool { return !hw.checking && !r.w.hearingBusy }, "refused")
	r.main(func() {
		if !strings.Contains(hw.checkText.Text, "open case") {
			t.Errorf("wording %q", hw.checkText.Text)
		}
	})
}

// EQ is a read-modify-write of the report the dongle keeps, confirmed by the next report;
// rename can only say it was sent. The simulator passes rename on here, as the firmware does not.
func TestExperimentsConfirmOrSaySent(t *testing.T) {
	cfg := sim.Demo()
	cfg.AnyAAP = true
	r := experimentalRig(t, cfg)
	x := r.w.extra
	r.main(func() {
		if !x.applyEQ.Disabled() || !x.eqOn.Disabled() {
			t.Error("EQ editable before it was read")
		}
		x.readEqualizer()
	})
	r.until(func() bool { return x.eqBase != nil && !r.w.hearingBusy }, "eq read")
	r.main(func() {
		if *x.eqBase != (hearing.EQ{Low: 30, Mid: 50, High: 70}) || x.eqMid.Slider.Value != 50 || x.eqOn.Checked {
			t.Errorf("read %+v", *x.eqBase)
		}
		if !x.applyEQ.Disabled() {
			t.Error("apply enabled with nothing edited")
		}
		x.eqOn.SetChecked(true)
		x.eqMid.Slider.SetValue(60)
		x.applyEqualizer()
	})
	r.until(func() bool { return !r.w.hearingBusy && x.eqStatus.Text != i18n.T("hearing.sending") }, "eq applied")
	r.main(func() {
		want := hearing.EQ{On: true, Low: 30, Mid: 60, High: 70}
		if x.eqStatus.Text != i18n.T("extra.eqConfirmed") || x.eqBase == nil || *x.eqBase != want {
			t.Errorf("eq says %q, %+v", x.eqStatus.Text, x.eqBase)
		}
		x.name.SetText("Test")
		x.rename()
	})
	sent := r.dev.AAPSent()
	if len(sent) != 1 || !bytes.Equal(sent[0], sim.EQReport(true, 30, 60, 70)) {
		t.Errorf("sent % x", sent)
	}
	r.until(func() bool { return x.nameStatus.Text != "" && !r.w.hearingBusy }, "rename")
	r.main(func() {
		if x.nameStatus.Text != i18n.T("extra.nameSent") {
			t.Errorf("rename says %q", x.nameStatus.Text)
		}
		x.readTransparency()
	})
	r.until(func() bool { return x.transBase != nil && !r.w.hearingBusy }, "transparency read")
	r.main(func() {
		if !x.applyTrans.Disabled() || x.transLocked || x.transAmp.Slider.Disabled() {
			t.Error("apply enabled with nothing edited, or the editor locked with hearing assistance off")
		}
		if x.transAmp.Slider.Max != 2 || x.transAmp.Slider.Min != -1 {
			t.Error("transparency amplification takes -1 to 2")
		}
		x.transAmp.Slider.SetValue(1.75)
		if x.transDraft.LeftAmplification != 1.75 || x.transDraft.RightAmplification != 0.5 || x.applyTrans.Disabled() {
			t.Errorf("draft %+v", x.transDraft)
		}
		x.transEar.SetSelected(i18n.T("battery.right"))
		if x.transAmp.Slider.Value != 0.5 {
			t.Errorf("right ear shows %v", x.transAmp.Slider.Value)
		}
		x.writeTransparency()
	})
	r.until(func() bool { return x.transStatus.Text != "" && !r.w.hearingBusy }, "transparency write")
	r.main(func() {
		if x.transStatus.Text != i18n.T("extra.confirmed") || x.transBase.LeftAmplification != 1.75 {
			t.Errorf("%q %+v", x.transStatus.Text, x.transBase)
		}
		x.readLoudSound()
	})
	r.until(func() bool { return x.loudKnown && !r.w.hearingBusy }, "loud read")
	r.main(func() { x.loud.SetChecked(true) })
	r.until(func() bool { return !r.w.hearingBusy && x.loudValue }, "loud set")
}

// While Hearing Aid is on, Custom Transparency may only be turned off; the page offers no more.
func TestTransparencyLocksWhileHearingAssistanceIsOn(t *testing.T) {
	a := sim.NewHearingAirPods("A3063", "9A348")
	a.Slots[2] = sim.CachedReport{Age: 1000, Revision: 2, Packet: []byte{4, 0, 4, 0, 9, 0, 0x2c, 1, 1, 0, 0}}
	a.Slots[3] = sim.CachedReport{Age: 1000, Revision: 2, Packet: []byte{4, 0, 4, 0, 9, 0, 0x33, 1, 0, 0, 0}}
	a.Server.Attributes[len(a.Server.Attributes)-2].Value = sim.TransparencyRecord(true)
	r := experimentalRig(t, a.Config())
	x := r.w.extra
	r.main(x.readTransparency)
	r.until(func() bool { return x.transBase != nil && !r.w.hearingBusy }, "read")
	r.main(func() {
		if !x.transLocked || !x.transLockBox.Visible() {
			t.Fatal("not locked with Hearing Aid on")
		}
		for name, item := range map[string]fyne.Disableable{"amplification": x.transAmp.Slider, "tone": x.transTone.Slider, "ear": x.transEar, "boost": x.transBoost} {
			if !item.Disabled() {
				t.Errorf("%s editable", name)
			}
		}
		if x.transEnabled.Disabled() {
			t.Fatal("turning it off must stay possible")
		}
		x.transEnabled.SetChecked(false)
		if x.applyTrans.Disabled() {
			t.Error("turning it off cannot be applied")
		}
	})
}

// The dongle declines EQ when it does not know the model or holds no EQ report, and sends
// nothing; rename is refused outright. The page says so, never that anything was sent.
func TestExperimentsTheDongleRefusesSayRefused(t *testing.T) {
	cfg := sim.Demo()
	cfg.Cached[dongle.SlotMetadata] = sim.CachedReport{Age: 1000, Revision: 1, Packet: sim.Metadata("A9999", "9A348", "SIMSERIAL123")}
	r := experimentalRig(t, cfg)
	x := r.w.extra
	r.main(x.readEqualizer)
	r.until(func() bool { return x.eqBase != nil && !r.w.hearingBusy }, "eq read")
	r.main(func() {
		x.eqHigh.Slider.SetValue(20)
		x.applyEqualizer()
	})
	r.until(func() bool { return !r.w.hearingBusy && x.eqStatus.Text != i18n.T("hearing.sending") }, "eq")
	r.main(func() {
		if x.eqStatus.Text != i18n.T("extra.eqRefused") || x.eqBase != nil {
			t.Errorf("eq says %q", x.eqStatus.Text)
		}
		x.name.SetText("Test")
		x.rename()
	})
	r.until(func() bool { return x.nameStatus.Text != "" && !r.w.hearingBusy }, "rename")
	r.main(func() {
		if !strings.Contains(x.nameStatus.Text, "refused") || strings.Contains(x.nameStatus.Text, i18n.T("extra.nameSent")) {
			t.Errorf("rename says %q", x.nameStatus.Text)
		}
	})
	if sent := len(r.dev.AAPSent()); sent != 0 {
		t.Errorf("%d packets went out", sent)
	}
}

// AirPods that have sent no EQ report on this connection are said to have none.
func TestNoEQReportIsSaidBeforeAnyEdit(t *testing.T) {
	cfg := sim.Demo()
	delete(cfg.Cached, dongle.SlotEQ)
	r := experimentalRig(t, cfg)
	x := r.w.extra
	r.main(x.readEqualizer)
	r.until(func() bool { return x.eqStatus.Text != "" && !r.w.hearingBusy }, "eq read")
	r.main(func() {
		if x.eqBase != nil || x.eqEditor.Visible() || !strings.Contains(x.eqStatus.Text, "EQ") {
			t.Errorf("%q", x.eqStatus.Text)
		}
	})
}

func TestLaneDragWritesTheStrength(t *testing.T) {
	r := newRig(t, ownerConfig())
	r.shown()
	r.main(func() {
		r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"})
		r.w.win.Resize(fyne.NewSize(defaultWidth, defaultHeight))
	})
	row := r.w.settings.byName["adaptive-strength"]
	mode := r.w.settings.byName["mode"]
	r.until(func() bool { return mode.segment.OnStrength != nil }, "draggable")
	before := r.dev.Writes()
	r.main(func() {
		width := mode.segment.Size().Width
		lane := mode.segment.lane(width)
		if lane < 0 {
			t.Fatalf("no lane at width %v with %v", width, mode.segment.Options)
		}
		span := mode.segment.segments(width)[lane]
		mode.segment.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(span.x+span.width-1, 20)}})
		mode.segment.DragEnd()
	})
	r.until(func() bool { return !row.writing && r.dev.Writes() == before+1 }, "the write")
	if got := lastCommitValue(t, r.dev, adaptiveStrengthControl); got != 100 {
		t.Errorf("wrote %d", got)
	}
}

func TestUnknownSliderHasADimThumb(t *testing.T) {
	r := newRig(t, ownerConfig())
	r.shown()
	r.main(func() {
		if row := r.w.settings.byName["case-volume"]; !row.dimmed || row.known {
			t.Errorf("case-volume dimmed %v known %v", row.dimmed, row.known)
		}
		if row := r.w.settings.byName["tone-volume"]; row.dimmed || !row.known {
			t.Errorf("tone-volume dimmed %v known %v", row.dimmed, row.known)
		}
	})
}

func TestNotReportedShowsOncePerCard(t *testing.T) {
	r := newRig(t, ownerConfig())
	r.shown()
	r.until(func() bool { return !r.w.settings.reading }, "read")
	r.settle()
	r.main(func() {
		shownAny := false
		for _, group := range r.w.settings.cards {
			shown := 0
			for _, row := range group.rows {
				if row.note.Visible() && row.note.Text == notReportedHint() {
					shown++
				}
			}
			if shown > 1 {
				t.Errorf("%s shows it %d times", group.base, shown)
			}
			shownAny = shownAny || shown > 0
		}
		if !shownAny {
			t.Error("never shown")
		}
	})
}

func TestLanguageChangesInPlace(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	t.Cleanup(func() { r.main(func() { i18n.SetLanguage(i18n.English) }) })
	mode := r.w.settings.byName["mode"]
	var writes int
	r.main(func() {
		selected := mode.segment.Selected
		writes = r.dev.Writes()
		if selected == "" {
			t.Fatal("no mode shown")
		}
		r.w.about.experimental.SetChecked(true)
		r.w.playbackRow.fold.Toggle()
		i18n.SetLanguage(i18n.Portuguese)
		if !r.w.playbackRow.fold.Open() {
			t.Error("a language change closed a section")
		}
		if got := r.w.playbackRow.fold.Title.Text; got != "Reprodução" {
			t.Errorf("section title %q", got)
		}
		if got := r.w.settings.byName["conversation-awareness"].label.Text; got != "Deteção de conversa" {
			t.Errorf("setting name %q", got)
		}
		if mode.segment.Selected == selected || mode.segment.Selected != wordLabel("anc") {
			t.Errorf("mode shows %q", mode.segment.Selected)
		}
		if got := r.w.header.status.value; got != "Ligados" {
			t.Errorf("status %q", got)
		}
		i18n.SetLanguage(i18n.English)
		if got := r.w.settings.byName["conversation-awareness"].label.Text; got != "Conversation Awareness" {
			t.Errorf("back to %q", got)
		}
	})
	r.flush()
	if after := r.dev.Writes(); after != writes {
		t.Errorf("a language change wrote %d times", after-writes)
	}
}

func TestAppPanelControls(t *testing.T) {
	var set string
	pause, notes := true, true
	r := newRigWith(t, sim.Demo(), func(o *Options) {
		o.Controls = &AppControls{
			AutoPauseEnabled: func() bool { return pause }, SetAutoPauseEnabled: func(on bool) { pause = on },
			NotificationsEnabled: func() bool { return notes }, SetNotificationsEnabled: func(on bool) { notes = on },
			ModeNotificationsEnabled: func() bool { return false }, SetModeNotificationsEnabled: func(bool) {},
			Language: func() string { return i18n.System }, SetLanguage: func(code string) { set = code },
			ShowBatteries: func() {},
		}
	})
	r.main(func() {
		a := r.w.about
		if !a.pause.Checked || a.language.Selected != i18n.T("language.system") {
			t.Errorf("start: pause %v, language %q", a.pause.Checked, a.language.Selected)
		}
		a.pause.SetChecked(false)
		a.notes.SetChecked(false)
		if pause || notes || !a.modes.Disabled() {
			t.Errorf("pause %v notes %v, mode switch must follow", pause, notes)
		}
		a.language.SetSelected("Français")
		if set != i18n.French {
			t.Errorf("language %q", set)
		}
	})
}

// TestArtSheet saves each model's pictures, lit and dimmed, when AIRPODS_SCREENS_DIR is set.
func TestArtSheet(t *testing.T) {
	dir := os.Getenv("AIRPODS_SCREENS_DIR")
	sets := []string{"1", "2", "3", "4", "5", "pro_1", "pro_2", "pro_3", "max_1", "max_2"}
	const box = 125
	sheet := image.NewNRGBA(image.Rect(0, 0, box*4, box*len(sets)))
	draw.Draw(sheet, sheet.Bounds(), &image.Uniform{color.NRGBA{0x1c, 0x1c, 0x1e, 0xff}}, image.Point{}, draw.Src)
	for row, set := range sets {
		kinds := []int{artLeft, artRight, artCase}
		if strings.HasPrefix(set, "max") {
			kinds = []int{artHeadphones}
		}
		for column, kind := range kinds {
			img := modelArtImage(set, kind, box, box, false)
			at := image.Rect(column*box, row*box, (column+1)*box, (row+1)*box)
			draw.Draw(sheet, at, img, image.Point{}, draw.Over)
			if alpha := img.(*image.NRGBA); alpha.Pix[3] != 0 && alpha.Pix[len(alpha.Pix)-1] != 0 {
				t.Errorf("%s %d fills the box corner to corner", set, kind)
			}
		}
		dim := modelArtImage(set, kinds[0], box, box, true)
		draw.Draw(sheet, image.Rect(3*box, row*box, 4*box, (row+1)*box), dim, image.Point{}, draw.Over)
	}
	if dir == "" {
		return
	}
	file, err := os.Create(filepath.Join(dir, "art-sheet.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, sheet); err != nil {
		t.Fatal(err)
	}
}

func TestArtSetFollowsTheModel(t *testing.T) {
	for number, want := range map[string]string{
		"A3063": "pro_3", "A3047": "pro_2", "A3050": "4", "A3056": "4", "A3439": "5", "A2084": "pro_1",
		"A2096": "max_1", "A3184": "max_1", "A3454": "max_2", "A9999": "pro_2", "": "pro_2", "A3157": "pro_2",
	} {
		if got := artSetFor(number); got != want {
			t.Errorf("%s: %s, want %s", number, got, want)
		}
	}
}

func TestAClosedWindowStopsFollowingTheLanguage(t *testing.T) {
	r := newRig(t, sim.Demo())
	label := r.w.settings.byName["conversation-awareness"].label
	r.main(func() {
		r.w.Shutdown()
		i18n.SetLanguage(i18n.Portuguese)
		if label.Text != "Conversation Awareness" {
			t.Errorf("a closed window changed language: %q", label.Text)
		}
		i18n.SetLanguage(i18n.English)
	})
}
