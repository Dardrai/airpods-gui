package ui

import (
	"errors"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/hearing"
	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

// experimentalTab is what may not work through the dongle: Custom Transparency, Loud Sound
// Reduction, EQ and Rename. Each block reports only what really happened: "sent" where the
// AirPods never answer or a write went out with no answer, and "confirmed" where they
// reported the new value back.
type experimentalTab struct {
	w       *Window
	content fyne.CanvasObject

	transparencyBox fyne.CanvasObject
	readTrans       *widget.Button
	applyTrans      *widget.Button
	transEditor     fyne.CanvasObject
	transEnabled    *Switch
	transEar        *widget.Select
	transAmp        *sliderRow
	transTone       *sliderRow
	transNoise      *sliderRow
	transBoost      *Switch
	transStored     *widget.Label
	transStatus     *widget.Label
	transStatusBox  fyne.CanvasObject
	transBase       *hearing.TransparencySettings
	transDraft      hearing.TransparencySettings
	transRight      bool
	// transLocked is set when the last read found hearing assistance on, or not known to be
	// off, on a model that has it: then only turning Custom Transparency off may be sent.
	transLocked  bool
	transLockBox fyne.CanvasObject
	loading      bool

	loudBox       fyne.CanvasObject
	readLoud      *widget.Button
	loud          *Switch
	loudDash      *dashPill
	loudKnown     bool
	loudValue     bool
	loudStatus    *widget.Label
	loudStatusBox fyne.CanvasObject

	eqBox       fyne.CanvasObject
	eqEditor    fyne.CanvasObject
	eqOn        *Switch
	eqLow       *sliderRow
	eqMid       *sliderRow
	eqHigh      *sliderRow
	readEQ      *widget.Button
	applyEQ     *widget.Button
	eqStatus    *widget.Label
	eqStatusBox fyne.CanvasObject
	// eqBase is the EQ the AirPods last reported, as read; nil until read, and again after
	// anything that leaves it unknown. eqDraft is what the controls show.
	eqBase  *hearing.EQ
	eqDraft hearing.EQ

	name          *widget.Entry
	sendName      *widget.Button
	nameStatus    *widget.Label
	nameStatusBox fyne.CanvasObject

	link linkKey
	peer [6]byte
}

func statusLine() (*widget.Label, fyne.CanvasObject) {
	label := smallLabel("")
	box := tight(label)
	box.Hide()
	return label, box
}

func (t *experimentalTab) earSlider(low, high float64, key string, set func(*hearing.TransparencySettings, float32)) *sliderRow {
	s := NewSlider(low, high)
	s.Step = 0.01
	row := newSliderRow(i18n.T(key), "", -1, s)
	t.w.texts = append(t.w.texts, func() { row.Title.SetText(i18n.T(key)) })
	s.OnChanged = func(value float64) {
		row.SetValue(fmt.Sprintf("%.2f", value))
		if !t.loading {
			set(&t.transDraft, float32(value))
			t.updateEnabled()
		}
	}
	return row
}

func newExperimentalTab(w *Window) *experimentalTab {
	t := &experimentalTab{w: w}
	t.buildTransparency()
	t.buildLoud()
	t.buildEQ()
	t.buildRename()
	t.content = container.NewVBox(tight(w.small("extra.help")), t.transparencyBox, t.loudBox, t.eqBox,
		w.subCaption("extra.rename"), tight(w.small("extra.renameHelp")), t.name, t.nameStatusBox,
		buttonRight(t.sendName), gap(4))
	t.fit()
	t.updateEnabled()
	return t
}

func (t *experimentalTab) buildTransparency() {
	w := t.w
	t.readTrans = w.button("extra.read", t.readTransparency)
	t.applyTrans = w.button("hearing.apply", t.writeTransparency)
	t.transStatus, t.transStatusBox = statusLine()
	t.transEnabled = NewSwitch(func(on bool) {
		if !t.loading {
			t.transDraft.Enabled = on
			t.updateEnabled()
		}
	})
	t.transEar = widget.NewSelect(nil, func(label string) {
		if t.loading {
			return
		}
		t.transRight = label == i18n.T("battery.right")
		t.showEar()
	})
	w.bind(func() {
		t.loading = true
		labels := []string{i18n.T("battery.left"), i18n.T("battery.right")}
		t.transEar.SetOptions(labels)
		if t.transRight {
			t.transEar.SetSelected(labels[1])
		} else {
			t.transEar.SetSelected(labels[0])
		}
		t.loading = false
	})
	t.transAmp = t.earSlider(-1, 2, "hearing.amplification", func(d *hearing.TransparencySettings, v float32) {
		if t.transRight {
			d.RightAmplification = v
		} else {
			d.LeftAmplification = v
		}
	})
	t.transTone = t.earSlider(-1, 1, "hearing.tone", func(d *hearing.TransparencySettings, v float32) {
		if t.transRight {
			d.RightTone = v
		} else {
			d.LeftTone = v
		}
	})
	t.transNoise = t.earSlider(0, 1, "extra.noiseReduction", func(d *hearing.TransparencySettings, v float32) {
		if t.transRight {
			d.RightAmbientNoiseReduction = v
		} else {
			d.LeftAmbientNoiseReduction = v
		}
	})
	t.transBoost = NewSwitch(func(on bool) {
		if t.loading {
			return
		}
		if t.transRight {
			t.transDraft.RightConversationBoost = on
		} else {
			t.transDraft.LeftConversationBoost = on
		}
		t.updateEnabled()
	})
	t.transStored = newValue(dashText)
	w.texts = append(w.texts, t.showStoredNoise)
	t.transEditor = container.NewVBox(
		w.optionRow("extra.transparencyOn", "", t.transEnabled),
		w.selectRow("extra.ear", "", t.transEar),
		t.transAmp, t.transTone, t.transNoise,
		w.optionRow("hearing.boost", "", t.transBoost),
		w.valueRow("extra.noiseReduction", t.transStored),
	)
	t.transEditor.Hide()
	t.transLockBox = tight(w.small("extra.transLocked"))
	t.transLockBox.Hide()
	t.transparencyBox = container.NewVBox(w.subCaption("extra.transparency"), t.transLockBox, t.transStatusBox, t.transEditor,
		container.NewHBox(layout.NewSpacer(), t.readTrans, t.applyTrans))
}

func (t *experimentalTab) buildLoud() {
	w := t.w
	t.readLoud = w.button("extra.read", t.readLoudSound)
	t.loud = NewSwitch(func(on bool) {
		if !t.loading {
			t.setLoudSound(on)
		}
	})
	t.loud.Hide()
	t.loudDash = newDashPill(nil)
	t.loudDash.Disable()
	t.loudStatus, t.loudStatusBox = statusLine()
	t.loudBox = container.NewVBox(w.subCaption("extra.loud"),
		w.optionRow("extra.loudRow", "extra.loudHint", container.NewStack(t.loud, t.loudDash)),
		t.loudStatusBox, buttonRight(t.readLoud))
}

func (t *experimentalTab) buildEQ() {
	w := t.w
	t.eqOn = NewSwitch(func(on bool) {
		if !t.loading {
			t.eqDraft.On = on
			t.updateEnabled()
		}
	})
	band := func(key string, set func(*hearing.EQ, byte)) *sliderRow {
		s := NewSlider(0, 100)
		s.Step = 1
		row := newSliderRow(i18n.T(key), "", -1, s)
		w.texts = append(w.texts, func() { row.Title.SetText(i18n.T(key)) })
		s.OnChanged = func(value float64) {
			row.SetValue(fmt.Sprintf("%.0f", value))
			if !t.loading {
				set(&t.eqDraft, byte(value))
				t.updateEnabled()
			}
		}
		return row
	}
	t.eqLow = band("extra.eqLow", func(e *hearing.EQ, v byte) { e.Low = v })
	t.eqMid = band("extra.eqMid", func(e *hearing.EQ, v byte) { e.Mid = v })
	t.eqHigh = band("extra.eqHigh", func(e *hearing.EQ, v byte) { e.High = v })
	t.readEQ = w.button("extra.read", t.readEqualizer)
	t.applyEQ = w.button("hearing.apply", t.applyEqualizer)
	t.eqStatus, t.eqStatusBox = statusLine()
	t.eqEditor = container.NewVBox(w.optionRow("extra.eqOn", "", t.eqOn), t.eqLow, t.eqMid, t.eqHigh)
	t.eqEditor.Hide()
	t.eqBox = container.NewVBox(w.subCaption("extra.eq"), tight(w.small("extra.eqHelp")), t.eqStatusBox, t.eqEditor,
		container.NewHBox(layout.NewSpacer(), t.readEQ, t.applyEQ))
}

func (t *experimentalTab) buildRename() {
	w := t.w
	t.name = widget.NewEntry()
	t.name.Validator = nil
	t.name.OnChanged = func(string) { t.updateEnabled() }
	w.bind(func() { t.name.SetPlaceHolder(i18n.T("extra.renamePlaceholder")) })
	t.sendName = w.button("extra.send", t.rename)
	t.nameStatus, t.nameStatusBox = statusLine()
}

// fit shows what the reported model has. The requests enforce the same flags again.
func (t *experimentalTab) fit() {
	profile, known := model.Find(t.w.info.Model)
	showIf(t.transparencyBox, known && profile.CustomTransparency)
	showIf(t.loudBox, known && profile.LoudSoundReduction)
	// Whether these AirPods send an EQ report at all is for the read to tell.
	showIf(t.eqBox, known && profile.CustomEQ)
	t.w.relayout()
}

func (t *experimentalTab) usable() bool {
	return t.w.state.Ready() && !t.w.hearingBusy && t.w.state.Status.Has(dongle.CapRawTransport)
}

func (t *experimentalTab) updateEnabled() {
	usable := t.usable()
	haveTrans := t.transBase != nil
	setEnabled(t.readTrans, usable)
	setEnabled(t.applyTrans, usable && haveTrans && t.transDraft != *t.transBase && t.transDraft.Valid() && t.transAllowed())
	// While hearing assistance is on, the AirPods' record may only be turned off: core refuses
	// anything else, and the page does not offer it.
	editable := usable && haveTrans && !t.transLocked
	for _, item := range []fyne.Disableable{t.transEar, t.transAmp.Slider, t.transTone.Slider, t.transNoise.Slider, t.transBoost} {
		enableWidget(item, editable)
	}
	enableWidget(t.transEnabled, editable || (usable && haveTrans && t.transBase.Enabled))
	setEnabled(t.readLoud, usable)
	enableWidget(t.loud, usable && t.loudKnown)
	showIf(t.loud, t.loudKnown)
	showIf(t.loudDash, !t.loudKnown)
	haveEQ := t.eqBase != nil
	setEnabled(t.readEQ, usable)
	setEnabled(t.applyEQ, usable && haveEQ && t.eqDraft != *t.eqBase && t.eqDraft.Valid())
	for _, item := range []fyne.Disableable{t.eqOn, t.eqLow.Slider, t.eqMid.Slider, t.eqHigh.Slider} {
		enableWidget(item, usable && haveEQ)
	}
	setEnabled(t.sendName, usable && t.name.Text != "")
}

// linkUpdate forgets values the last connection read: a write needs a fresh baseline.
func (t *experimentalTab) linkUpdate(st device.State) {
	key := linkKey{present: st.Present, ready: st.Ready(), generation: st.Status.Generation}
	if key.generation != t.link.generation || st.Status.Peer != t.peer || key.present != t.link.present {
		t.transBase, t.loudKnown, t.eqBase, t.transLocked = nil, false, nil, false
		showIf(t.transEditor, false)
		showIf(t.transLockBox, false)
		showIf(t.eqEditor, false)
		t.w.relayout()
	}
	t.link, t.peer = key, st.Status.Peer
	t.updateEnabled()
}

func (t *experimentalTab) showStatus(box fyne.CanvasObject, label *widget.Label, value string) {
	setStatus(box, label, value)
	t.w.relayout()
}

// run submits one request holding the shared busy state. command marks what changes the device.
func (t *experimentalTab) run(command bool, job device.Job, done func(error)) {
	t.w.setHearingBusy(true)
	finish := func(err error) {
		t.w.do(func() {
			t.w.setHearingBusy(false)
			done(err)
			t.updateEnabled()
		})
	}
	if command {
		t.w.owner.SubmitCommand(job, finish)
	} else {
		t.w.owner.Submit(job, finish)
	}
}

// transAllowed says whether the draft may be sent: anything while hearing assistance is off,
// and only turning Custom Transparency off while it is on.
func (t *experimentalTab) transAllowed() bool {
	if !t.transLocked || t.transBase == nil {
		return true
	}
	off := *t.transBase
	off.Enabled = false
	return t.transDraft == off
}

// assistanceActive is the rule core applies before a Custom Transparency edit, as far as the
// mode reports tell it: on a model with hearing assistance, Hearing Aid and Media Assist must be
// reported off. Unknown counts as on. Core checks again, with the media assist details.
func assistanceActive(modelNumber string, state hearing.ModeState) bool {
	if profile, ok := model.Find(modelNumber); !ok || !profile.HearingAid {
		return false
	}
	for _, flag := range []hearing.Flag{state.HearingEnabled, state.AssistEnabled, state.Media} {
		if flag != hearing.FlagOff {
			return true
		}
	}
	return false
}

func (t *experimentalTab) showTransparency(settings hearing.TransparencySettings, locked bool) {
	t.transLocked = locked
	showIf(t.transLockBox, locked)
	base := settings
	t.transBase, t.transDraft = &base, settings
	t.loading = true
	t.transEnabled.SetChecked(settings.Enabled)
	t.loading = false
	t.showEar()
	t.showStoredNoise()
	showIf(t.transEditor, true)
	t.w.relayout()
}

// showStoredNoise shows both ears as the AirPods hold them, as transparency show does. The slider
// above it has one ear at a time, and what is not applied yet.
func (t *experimentalTab) showStoredNoise() {
	if t.transBase != nil {
		setLabel(t.transStored, fmt.Sprintf(i18n.T("hearing.leftRight"), bandText(t.transBase.LeftAmbientNoiseReduction), bandText(t.transBase.RightAmbientNoiseReduction)))
	}
}

// showEar puts the picked ear's values on the sliders.
func (t *experimentalTab) showEar() {
	d := t.transDraft
	amp, tone, noise, boost := d.LeftAmplification, d.LeftTone, d.LeftAmbientNoiseReduction, d.LeftConversationBoost
	if t.transRight {
		amp, tone, noise, boost = d.RightAmplification, d.RightTone, d.RightAmbientNoiseReduction, d.RightConversationBoost
	}
	t.loading = true
	setSliderRow(t.transAmp, float64(amp))
	setSliderRow(t.transTone, float64(tone))
	setSliderRow(t.transNoise, float64(noise))
	t.transBoost.SetChecked(boost)
	t.loading = false
}

// readTransparency reads the record and the hearing modes with it, which decide what may be
// edited.
func (t *experimentalTab) readTransparency() {
	var settings hearing.TransparencySettings
	var state hearing.ModeState
	modelNumber := t.w.info.Model
	t.run(false, func(c *dongle.Client) error {
		modes, err := t.w.modesFor(c)
		if err != nil {
			return err
		}
		if settings, err = modes.ExperimentalReadTransparency(c); err != nil {
			return err
		}
		state, err = modes.Read(c)
		return err
	}, func(err error) {
		if err != nil {
			t.showStatus(t.transStatusBox, t.transStatus, t.w.requestError(err))
			return
		}
		t.showStatus(t.transStatusBox, t.transStatus, "")
		t.showTransparency(settings, assistanceActive(modelNumber, state))
	})
}

func (t *experimentalTab) writeTransparency() {
	if t.transBase == nil {
		return
	}
	expected, requested := *t.transBase, t.transDraft
	var settings hearing.TransparencySettings
	t.run(true, func(c *dongle.Client) error {
		modes, err := t.w.modesFor(c)
		if err != nil {
			return err
		}
		settings, err = modes.ExperimentalWriteTransparency(c, expected, requested)
		return err
	}, func(err error) {
		if err != nil {
			if device.IsUnconfirmed(err) {
				t.transBase = nil
				showIf(t.transEditor, false)
			}
			t.showStatus(t.transStatusBox, t.transStatus, t.w.requestError(err))
			return
		}
		t.showStatus(t.transStatusBox, t.transStatus, i18n.T("extra.confirmed"))
		t.showTransparency(settings, t.transLocked)
	})
}

func (t *experimentalTab) showLoud(value bool) {
	t.loudKnown, t.loudValue = true, value
	t.loading = true
	t.loud.SetChecked(value)
	t.loading = false
}

func (t *experimentalTab) readLoudSound() {
	var value bool
	t.run(false, func(c *dongle.Client) (err error) {
		value, err = hearing.ExperimentalReadLoudSoundReduction(c)
		return err
	}, func(err error) {
		if err != nil {
			t.loudKnown = false
			t.showStatus(t.loudStatusBox, t.loudStatus, t.w.requestError(err))
			return
		}
		t.showStatus(t.loudStatusBox, t.loudStatus, "")
		t.showLoud(value)
	})
}

func (t *experimentalTab) setLoudSound(on bool) {
	expected := t.loudValue
	var value bool
	t.run(true, func(c *dongle.Client) (err error) {
		value, err = hearing.ExperimentalSetLoudSoundReduction(c, expected, on)
		return err
	}, func(err error) {
		if err != nil {
			// Whatever happened, the value on show must be read again before it counts.
			t.loudKnown = false
			t.showStatus(t.loudStatusBox, t.loudStatus, t.w.requestError(err))
			return
		}
		t.showStatus(t.loudStatusBox, t.loudStatus, i18n.T("extra.confirmed"))
		t.showLoud(value)
	})
}

// showEQ takes an EQ the AirPods reported as the new baseline, and puts it on the controls.
func (t *experimentalTab) showEQ(e hearing.EQ) {
	base := e
	t.eqBase, t.eqDraft = &base, e
	t.loading = true
	t.eqOn.SetChecked(e.On)
	setSliderRow(t.eqLow, float64(e.Low))
	setSliderRow(t.eqMid, float64(e.Mid))
	setSliderRow(t.eqHigh, float64(e.High))
	t.loading = false
	showIf(t.eqEditor, true)
	t.w.relayout()
}

// forgetEQ drops the baseline: the EQ on the AirPods is not known any more.
func (t *experimentalTab) forgetEQ() {
	t.eqBase = nil
	showIf(t.eqEditor, false)
	t.w.relayout()
}

// eqError words a failed EQ request. The dongle declining the packet means nothing was sent: it
// has no EQ report from these AirPods yet, or does not know their model.
func (t *experimentalTab) eqError(err error) string {
	var refused *hearing.Refused
	var rejected *dongle.RequestRejected
	if errors.As(err, &refused) && errors.As(err, &rejected) && rejected.Code == 6 {
		return i18n.T("extra.eqRefused")
	}
	return t.w.requestError(err)
}

func (t *experimentalTab) readEqualizer() {
	var e hearing.EQ
	t.run(false, func(c *dongle.Client) (err error) {
		e, err = hearing.ExperimentalReadEQ(c)
		return err
	}, func(err error) {
		if err != nil {
			t.forgetEQ()
			t.showStatus(t.eqStatusBox, t.eqStatus, t.eqError(err))
			return
		}
		t.showStatus(t.eqStatusBox, t.eqStatus, "")
		t.showEQ(e)
	})
}

// applyEqualizer is a read-modify-write of the AirPods' EQ report: the start reads the report
// again and sends once, and each step after it is a short owner job that looks for a new
// report carrying exactly what was sent. Nothing is ever sent twice.
func (t *experimentalTab) applyEqualizer() {
	if t.eqBase == nil {
		return
	}
	expected, requested := *t.eqBase, t.eqDraft
	t.w.setHearingBusy(true)
	t.showStatus(t.eqStatusBox, t.eqStatus, i18n.T("hearing.sending"))
	var change *hearing.EQChange
	t.w.owner.SubmitCommand(func(c *dongle.Client) (err error) {
		change, err = hearing.ExperimentalStartSetEQ(c, expected, requested)
		return err
	}, func(err error) {
		if err != nil || !change.Sent() {
			t.w.do(func() { t.eqDone(requested, false, err) })
			return
		}
		var result hearing.EQ
		var stepErr error
		t.w.followSteps(func(c *dongle.Client) (done bool) {
			result, done, stepErr = change.Step(c)
			return done
		}, func(cause error) {
			result, _, stepErr = change.Absent(cause)
		}, func() {
			t.w.do(func() { t.eqDone(result, true, stepErr) })
		})
	})
}

func (t *experimentalTab) eqDone(result hearing.EQ, sent bool, err error) {
	t.w.setHearingBusy(false)
	switch {
	case err != nil:
		// Uncertain, or refused because the report moved or went: read before anything else.
		t.forgetEQ()
		t.showStatus(t.eqStatusBox, t.eqStatus, t.eqError(err))
	case !sent:
		t.showStatus(t.eqStatusBox, t.eqStatus, i18n.T("hearing.noChanges"))
		t.showEQ(result)
	default:
		t.showStatus(t.eqStatusBox, t.eqStatus, i18n.T("extra.eqConfirmed"))
		t.showEQ(result)
	}
	t.updateEnabled()
}

func (t *experimentalTab) rename() {
	name := t.name.Text
	t.run(true, func(c *dongle.Client) error {
		_, err := hearing.ExperimentalRename(c, name)
		return err
	}, func(err error) {
		if err != nil {
			t.showStatus(t.nameStatusBox, t.nameStatus, t.w.requestError(err))
			return
		}
		t.showStatus(t.nameStatusBox, t.nameStatus, i18n.T("extra.nameSent"))
	})
}
