package ui

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/hearing"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
	"github.com/Dardrai/airpods-gui/internal/statusdoc"
)

const (
	profileFileName = "airpods-hearing-profile.json"
	// maxProfileFile is far more than a profile takes. A longer file fails as bad JSON.
	maxProfileFile = 1 << 16
)

func modeName(mode hearing.Mode) string {
	if mode == hearing.MediaAssist {
		return i18n.T("hearing.mediaAssist")
	}
	return i18n.T("hearing.hearingAid")
}

// recoveryCopy gives the UI thread its own copy of a value that belongs to the owner goroutine.
func recoveryCopy(m *hearing.Modes) *hearing.Recovery {
	if r := m.Recovery(); r != nil {
		kept := *r
		return &kept
	}
	return nil
}

// modeControl is a mode's switch, with a dead dash in its place while the mode is unknown:
// an unreported mode is not off.
type modeControl struct {
	toggle *Switch
	dash   *dashPill
	object fyne.CanvasObject
}

// hearingTab is the Hearing section. Every request goes through the owner goroutine, one at
// a time; nothing here retries a write.
type hearingTab struct {
	w       *Window
	content fyne.CanvasObject

	read       *widget.Button
	status     *widget.Label
	statusBox  fyne.CanvasObject
	aid, media modeControl
	recoverBt  *widget.Button
	recoverBox fyne.CanvasObject
	aidSetUp   *widget.Label
	mediaSetUp *widget.Label
	ages       *widget.Label
	reportBox  fyne.CanvasObject

	editor    fyne.CanvasObject
	editNote  *widget.Label
	noteBox   fyne.CanvasObject
	bands     [2][hearing.Bands]*widget.Entry
	amp       *sliderRow
	balance   *sliderRow
	tone      *sliderRow
	boost     *Switch
	apply     *widget.Button
	storedANR *widget.Label
	ownVoice  *widget.Label
	importBt  *widget.Button
	exportBt  *widget.Button

	state    hearing.ModeState
	have     bool
	needRead bool // an uncertain outcome: nothing is offered until the state is read again
	recovery *hearing.Recovery
	reports  *statusdoc.HearingReports
	edit     *hearing.ProfileEdit
	canEdit  bool // the profile last read can be written back
	loading  bool
	link     linkKey
	peer     [6]byte
}

func newHearingTab(w *Window) *hearingTab {
	t := &hearingTab{w: w}
	t.status = smallLabel("")
	t.statusBox = tight(t.status)
	t.statusBox.Hide()
	t.read = w.button("hearing.read", t.readAll)
	t.aid = t.newModeControl(hearing.HearingAid)
	t.media = t.newModeControl(hearing.MediaAssist)
	t.recoverBt = w.button("hearing.recover", t.recoverMode)
	t.recoverBox = buttonRight(t.recoverBt)
	t.recoverBox.Hide()
	t.aidSetUp, t.mediaSetUp, t.ages = newValue(dashText), newValue(dashText), smallLabel("")
	t.reportBox = container.NewVBox(w.valueRow("hearing.aidSetUp", t.aidSetUp), w.valueRow("hearing.mediaSetUp", t.mediaSetUp), tight(t.ages))
	t.reportBox.Hide()
	t.buildEditor(w)
	t.content = container.NewVBox(
		tight(w.small("hearing.help")),
		t.statusBox,
		w.optionRow("hearing.hearingAid", "", t.aid.object),
		w.optionRow("hearing.mediaAssist", "", t.media.object),
		t.reportBox,
		t.recoverBox,
		buttonRight(t.read),
		t.editor,
		gap(4),
	)
	t.updateEnabled()
	w.texts = append(w.texts, func() {
		t.showRecovery()
		t.showReports()
		t.showStored()
	})
	return t
}

func (t *hearingTab) newModeControl(mode hearing.Mode) modeControl {
	control := modeControl{dash: newDashPill(nil)}
	control.dash.Disable()
	control.toggle = NewSwitch(func(on bool) {
		if !t.loading {
			t.changeMode(mode, on)
		}
	})
	control.object = container.NewStack(control.toggle, control.dash)
	control.toggle.Hide()
	return control
}

func (t *hearingTab) buildEditor(w *Window) {
	grid := []fyne.CanvasObject{gap(0)}
	for _, hz := range hearing.BandHz {
		text := strconv.Itoa(hz)
		if hz >= 1000 {
			text = strconv.Itoa(hz/1000) + "k"
		}
		grid = append(grid, newText(text, 12, false, colorSecondary, fyne.TextAlignCenter))
	}
	for ear, key := range []string{"battery.shortL", "battery.shortR"} {
		name := newText(i18n.T(key), 13, false, colorSecondary, fyne.TextAlignLeading)
		w.bind(func() { name.SetText(i18n.T(key)) })
		grid = append(grid, name)
		for band := range hearing.Bands {
			entry := widget.NewEntry()
			entry.OnChanged = func(string) { t.curveChanged() }
			t.bands[ear][band] = entry
			grid = append(grid, entry)
		}
	}
	// A value the draft rejects, such as a gain that would put one ear out of range, is not
	// clamped: the slider goes back to what the draft holds, and a note says why.
	slider := func(low, high, step float64, key string, changed func(float32) bool) *sliderRow {
		s := NewSlider(low, high)
		s.Step = step
		row := newSliderRow(i18n.T(key), "", -1, s)
		w.texts = append(w.texts, func() { row.Title.SetText(i18n.T(key)) })
		s.OnChanged = func(value float64) {
			row.SetValue(fmt.Sprintf("%.2f", value))
			if !t.loading && t.edit != nil {
				accepted := changed(float32(value))
				t.afterEdit()
				if !accepted {
					t.showDraft()
					setStatus(t.noteBox, t.editNote, i18n.T("hearing.gainRejected"))
					t.w.relayout()
				}
			}
		}
		return row
	}
	// Amplification is the mean of the two ears' gains and balance is right minus left, in
	// the ranges the AirPods' own app uses; each ear has to stay within -1.5 to 1.5.
	t.amp = slider(-1.5, 1.5, 0.01, "hearing.amplification", func(v float32) bool { return t.edit.EditAmplification(v) })
	t.balance = slider(-3, 3, 0.01, "hearing.balance", func(v float32) bool { return t.edit.EditBalance(v) })
	t.tone = slider(-1, 1, 0.01, "hearing.tone", func(v float32) bool { return t.edit.EditTone(v) })
	t.boost = NewSwitch(func(on bool) {
		if !t.loading && t.edit != nil {
			t.edit.EditConversationBoost(on)
			t.afterEdit()
		}
	})
	t.storedANR, t.ownVoice = newValue(dashText), newValue(dashText)
	t.apply = w.button("hearing.apply", t.applyDraft)
	t.importBt = w.button("hearing.import", t.importProfile)
	t.exportBt = w.button("hearing.export", t.exportProfile)
	t.editNote = smallLabel("")
	t.noteBox = tight(t.editNote)
	t.noteBox.Hide()
	t.editor = container.NewVBox(
		w.subCaption("hearing.profile"),
		tight(w.small("hearing.audiogramHelp")),
		container.New(bandGridLayout{}, grid...),
		t.amp, t.balance, t.tone,
		w.optionRow("hearing.boost", "", t.boost),
		w.valueRow("extra.noiseReduction", t.storedANR),
		w.valueRow("hearing.ownVoice", t.ownVoice),
		t.noteBox,
		container.NewHBox(layout.NewSpacer(), t.importBt, t.exportBt, t.apply),
	)
	t.editor.Hide()
}

// bandGridLayout is the audiogram: a label column and one equal column per band, on three
// rows. Its minimum ignores the entries' own, so that the band count sets no window width.
type bandGridLayout struct{}

const (
	bandLabelWidth float32 = 24
	bandMinCell    float32 = 44
	bandHeadHeight float32 = 20
	bandRowGap     float32 = 4
)

func (bandGridLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	columns := hearing.Bands + 1
	cell := (size.Width - bandLabelWidth) / float32(hearing.Bands)
	entry := objects[columns+1].MinSize().Height
	for i, object := range objects {
		column, row := i%columns, i/columns
		x, width := float32(0), bandLabelWidth
		if column > 0 {
			x, width = bandLabelWidth+cell*float32(column-1), cell-2
		}
		y, height := float32(0), bandHeadHeight
		if row > 0 {
			y, height = bandHeadHeight+float32(row-1)*(entry+bandRowGap), entry
		}
		object.Move(fyne.NewPos(x, y))
		object.Resize(fyne.NewSize(width, height))
	}
}

func (bandGridLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	entry := objects[hearing.Bands+2].MinSize().Height
	return fyne.NewSize(bandLabelWidth+bandMinCell*float32(hearing.Bands), bandHeadHeight+2*entry+bandRowGap)
}

// usable is whether a request can go out right now.
func (t *hearingTab) usable() bool {
	return t.w.state.Ready() && !t.w.hearingBusy && t.w.state.Status.Has(dongle.CapRawTransport)
}

func (t *hearingTab) updateEnabled() {
	usable := t.usable()
	setEnabled(t.read, usable)
	setEnabled(t.recoverBt, usable && t.recovery != nil)
	for _, row := range []struct {
		control modeControl
		flag    hearing.Flag
	}{{t.aid, t.state.HearingEnabled}, {t.media, t.state.Media}} {
		known := t.have && !t.needRead && row.flag != hearing.FlagUnknown
		showIf(row.control.toggle, known)
		showIf(row.control.dash, !known)
		enableWidget(row.control.toggle, usable && known)
	}
	canApply := t.edit != nil && !t.needRead && t.edit.Dirty && t.edit.CanApply()
	setEnabled(t.apply, usable && canApply)
	setEnabled(t.importBt, usable && t.edit != nil && t.canEdit && !t.needRead)
	setEnabled(t.exportBt, t.edit != nil && !t.edit.InvalidCurve && !t.w.hearingBusy)
	editable := usable && t.edit != nil && t.canEdit && !t.needRead
	for ear := range t.bands {
		for band := range t.bands[ear] {
			enableWidget(t.bands[ear][band], editable)
		}
	}
	enableWidget(t.amp.Slider, editable)
	enableWidget(t.balance.Slider, editable)
	enableWidget(t.tone.Slider, editable)
	enableWidget(t.boost, editable)
}

// linkUpdate forgets what a previous connection proved. A pending recovery survives, since
// the mode may still be on when the same pair comes back; a read fetches it again.
func (t *hearingTab) linkUpdate(st device.State) {
	key := linkKey{present: st.Present, ready: st.Ready(), generation: st.Status.Generation}
	if key.generation != t.link.generation || st.Status.Peer != t.peer || key.present != t.link.present {
		t.state, t.have, t.edit, t.needRead, t.reports = hearing.ModeState{}, false, nil, false, nil
		if st.Status.Peer != t.peer {
			t.recovery = nil
			t.showRecovery()
		}
		showIf(t.editor, false)
		t.w.relayout()
	}
	t.link, t.peer = key, st.Status.Peer
	t.updateEnabled()
}

func (t *hearingTab) say(text string) {
	setStatus(t.statusBox, t.status, text)
	t.w.relayout()
}

func (t *hearingTab) showRecovery() {
	if t.recovery == nil {
		showIf(t.recoverBox, false)
		return
	}
	t.recoverBt.SetText(fmt.Sprintf(i18n.T("hearing.recoverAction"), modeName(t.recovery.Mode)))
	showIf(t.recoverBox, true)
}

// showReports says whether each mode is set up and how old the reports behind the switches are.
func (t *hearingTab) showReports() {
	showIf(t.reportBox, t.have)
	if !t.have {
		return
	}
	setLabel(t.aidSetUp, setUpText(t.state.HearingEnrolled))
	setLabel(t.mediaSetUp, setUpText(t.state.VoiceEnrolled))
	var ages statusdoc.HearingReports
	if t.reports != nil {
		ages = *t.reports
	}
	setLabel(t.ages, fmt.Sprintf(i18n.T("hearing.reportAges"), reportAgeText(ages.HearingAid), reportAgeText(ages.Assist), reportAgeText(ages.Media)))
}

func setUpText(flag hearing.Flag) string {
	switch flag {
	case hearing.FlagOn:
		return i18n.T("hearing.setUpYes")
	case hearing.FlagOff:
		return i18n.T("hearing.setUpNo")
	}
	return i18n.T("hearing.setUpUnknown")
}

func reportAgeText(age *uint32) string {
	if age == nil {
		return i18n.T("hearing.ageNone")
	}
	return fmt.Sprintf(i18n.T("hearing.ageMs"), *age)
}

// readReports reads the ages again after a change, which is when they matter most: the report on
// show may be older than the change.
func (t *hearingTab) readReports() {
	var reports statusdoc.HearingReports
	t.w.owner.Submit(func(c *dongle.Client) error {
		reports = statusdoc.ReadHearingReports(c)
		return nil
	}, func(err error) {
		t.w.do(func() {
			if err == nil && t.have {
				t.reports = &reports
				t.showReports()
				t.w.relayout()
			}
		})
	})
}

func (t *hearingTab) showState() {
	t.loading = true
	t.aid.toggle.SetChecked(t.state.HearingEnabled == hearing.FlagOn)
	t.media.toggle.SetChecked(t.state.Media == hearing.FlagOn)
	t.loading = false
	t.showRecovery()
	t.showReports()
	t.updateEnabled()
	t.w.relayout()
}

// readAll reads the kept mode reports and then the stored profile. It writes nothing.
func (t *hearingTab) readAll() {
	if !t.usable() {
		return
	}
	t.w.setHearingBusy(true)
	t.say(i18n.T("hearing.reading"))
	var state hearing.ModeState
	var recovery *hearing.Recovery
	var profile *hearing.Profile
	var profileErr error
	var reports statusdoc.HearingReports
	t.w.owner.Submit(func(c *dongle.Client) error {
		modes, err := t.w.modesFor(c)
		if err != nil {
			return err
		}
		state, err = modes.Read(c)
		recovery = recoveryCopy(modes)
		if err != nil {
			return err
		}
		reports = statusdoc.ReadHearingReports(c)
		profile, profileErr = modes.ReadProfile(c)
		return nil
	}, func(err error) {
		t.w.do(func() {
			t.w.setHearingBusy(false)
			if err != nil {
				t.say(t.w.requestError(err))
				return
			}
			t.state, t.have, t.needRead, t.recovery, t.reports = state, true, false, recovery, &reports
			t.say("")
			if profileErr != nil {
				t.edit = nil
				showIf(t.editor, false)
				t.say(t.w.requestError(profileErr))
			} else if profile != nil {
				t.loadProfile(*profile)
			}
			t.showState()
		})
	})
}

// changeMode turns a mode off at once. Turning one on reads the reports first, so that a change
// that would be refused asks nothing, then warns and asks, as the CLI does. Only a yes sends.
func (t *hearingTab) changeMode(mode hearing.Mode, on bool) {
	if !on {
		t.setMode(mode, false)
		return
	}
	t.w.setHearingBusy(true)
	t.say(i18n.T("hearing.checking"))
	var needed bool
	var recovery *hearing.Recovery
	t.w.owner.Submit(func(c *dongle.Client) error {
		modes, err := t.w.modesFor(c)
		if err != nil {
			return err
		}
		_, err = modes.Read(c)
		recovery = recoveryCopy(modes)
		if err != nil {
			return err
		}
		plan, err := modes.Plan(mode, true)
		if err != nil {
			return err
		}
		needed = len(plan.Packets()) > 0
		return nil
	}, func(err error) {
		t.w.do(func() {
			if err != nil {
				t.w.setHearingBusy(false)
				t.recovery = recovery
				t.say(t.w.requestError(err))
				t.showState()
				return
			}
			t.say("")
			if !needed {
				t.setMode(mode, true)
				return
			}
			title := fmt.Sprintf(i18n.T("hearing.onTitle"), modeName(mode))
			t.w.confirm(title, i18n.T("hearing.warning"), i18n.T("hearing.onAccept"), func(accepted bool) {
				if accepted {
					t.setMode(mode, true)
					return
				}
				t.w.setHearingBusy(false)
				t.showState()
			})
		})
	})
}

// setMode changes a mode once. The switch goes back to the last report on failure.
func (t *hearingTab) setMode(mode hearing.Mode, on bool) {
	t.startChange(func(modes *hearing.Modes, c *dongle.Client) (*hearing.ModeChange, error) {
		return modes.StartSet(c, mode, on)
	})
}

// recoverMode is the explicit action offered after an unconfirmed change.
func (t *hearingTab) recoverMode() {
	if !t.usable() || t.recovery == nil {
		return
	}
	t.startChange(func(modes *hearing.Modes, c *dongle.Client) (*hearing.ModeChange, error) {
		return modes.StartRecover(c)
	})
}

// startChange sends a change in one short owner job, then follows it in others.
func (t *hearingTab) startChange(start func(*hearing.Modes, *dongle.Client) (*hearing.ModeChange, error)) {
	t.w.setHearingBusy(true)
	t.say(i18n.T("hearing.sending"))
	var modes *hearing.Modes
	var change *hearing.ModeChange
	var recovery *hearing.Recovery
	t.w.owner.SubmitCommand(func(c *dongle.Client) (err error) {
		if modes, err = t.w.modesFor(c); err != nil {
			return err
		}
		change, err = start(modes, c)
		recovery = recoveryCopy(modes)
		return err
	}, func(err error) {
		if err != nil {
			t.w.do(func() { t.modeDone(nil, recovery, err) })
			return
		}
		var result *hearing.ModeResult
		var stepErr error
		t.w.followSteps(func(c *dongle.Client) (done bool) {
			result, done, stepErr = change.Step(c)
			recovery = recoveryCopy(modes)
			return done
		}, func(cause error) {
			result, _, stepErr = change.Absent(cause)
			recovery = recoveryCopy(modes)
		}, func() {
			t.w.do(func() { t.modeDone(result, recovery, stepErr) })
		})
	})
}

func (t *hearingTab) modeDone(result *hearing.ModeResult, recovery *hearing.Recovery, err error) {
	t.w.setHearingBusy(false)
	t.recovery = recovery
	t.readReports()
	if err != nil {
		var refused *hearing.Refused
		if !errors.As(err, &refused) && !errors.Is(err, device.ErrNoDongle) {
			t.needRead = true
		}
		t.say(t.w.requestError(err))
		t.showState()
		return
	}
	t.state, t.have, t.needRead = result.State, true, false
	if result.Profile != nil && (t.edit == nil || !t.edit.Dirty) {
		t.loadProfile(*result.Profile)
	}
	t.say("")
	t.showState()
}

// setSliderRow shows a value that came from the device, without reporting a change. A drag
// in progress keeps the row.
func setSliderRow(row *sliderRow, value float64) {
	if row.Slider.Dragging() {
		return
	}
	row.Slider.ShowValue(value)
	row.SetValue(fmt.Sprintf("%.2f", value))
}

func bandText(value float32) string { return strconv.FormatFloat(float64(value), 'g', -1, 32) }

// loadProfile starts a fresh draft and puts it in the editor.
func (t *hearingTab) loadProfile(profile hearing.Profile) {
	t.edit, t.canEdit = hearing.NewProfileEdit(profile), profile.Editable
	t.showDraft()
	t.showStored()
	note := ""
	if !profile.Editable {
		note = i18n.T("hearing.notEditable")
	}
	setStatus(t.noteBox, t.editNote, note)
	showIf(t.editor, true)
}

// showStored shows what the AirPods hold and the editor cannot change, as hearing profile show does.
func (t *hearingTab) showStored() {
	if t.edit == nil {
		return
	}
	s := t.edit.Baseline()
	setLabel(t.storedANR, fmt.Sprintf(i18n.T("hearing.leftRight"), bandText(s.LeftAmbientNoiseReduction), bandText(s.RightAmbientNoiseReduction)))
	setLabel(t.ownVoice, bandText(s.OwnVoice))
}

// showDraft puts the draft in the editor's fields.
func (t *hearingTab) showDraft() {
	t.loading = true
	s := t.edit.Draft
	for band := range hearing.Bands {
		t.bands[0][band].SetText(bandText(s.LeftEQ[band]))
		t.bands[1][band].SetText(bandText(s.RightEQ[band]))
	}
	setSliderRow(t.amp, float64(t.edit.Amplification))
	setSliderRow(t.balance, float64(t.edit.Balance))
	setSliderRow(t.tone, float64((s.LeftTone+s.RightTone)/2))
	t.boost.SetChecked(s.LeftConversationBoost && s.RightConversationBoost)
	t.loading = false
}

// exportProfile saves what the editor shows, in the form `airpods hearing profile apply --file` takes.
func (t *hearingTab) exportProfile() {
	if t.edit == nil || t.edit.InvalidCurve {
		return
	}
	text := statusdoc.Encode(statusdoc.ProfileOf(t.edit))
	t.w.pickFile(profileFileName, func(file io.WriteCloser, err error) {
		if file == nil {
			if err != nil {
				t.say(fmt.Sprintf(i18n.T("hearing.exportFailed"), err.Error()))
			}
			return
		}
		_, err = io.WriteString(file, text)
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			t.say(fmt.Sprintf(i18n.T("hearing.exportFailed"), err.Error()))
			return
		}
		t.say(i18n.T("hearing.exported"))
	})
}

// importProfile loads a file into the editor. It never applies: nothing is sent until Apply.
func (t *hearingTab) importProfile() {
	if t.edit == nil || !t.canEdit {
		return
	}
	t.w.openFile(func(file io.ReadCloser, err error) {
		if file == nil {
			if err != nil {
				t.say(fmt.Sprintf(i18n.T("hearing.importFailed"), err.Error()))
			}
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, maxProfileFile))
		if err != nil {
			t.say(fmt.Sprintf(i18n.T("hearing.importFailed"), err.Error()))
			return
		}
		t.loadFile(data)
	})
}

// loadFile starts again from the stored profile: what the file does not name stays as stored, and
// edits made so far go, as with a fresh read. A file that fails leaves the editor as it was.
func (t *hearingTab) loadFile(data []byte) {
	if t.edit == nil || !t.canEdit {
		return // The link went away while the dialog was open.
	}
	input, err := statusdoc.ParseProfile(data)
	if err != nil {
		t.say(fmt.Sprintf(i18n.T("hearing.importFailed"), err.Error()))
		return
	}
	edit := hearing.NewProfileEdit(hearing.Profile{Settings: t.edit.Baseline(), Editable: true})
	if !input.ApplyTo(edit) || !edit.CanApply() || !edit.Draft.Valid() {
		t.say(fmt.Sprintf(i18n.T("hearing.importFailed"), i18n.T("hearing.importInvalid")))
		return
	}
	t.edit = edit
	t.showDraft()
	setStatus(t.noteBox, t.editNote, "")
	if edit.Dirty {
		t.say(i18n.T("hearing.imported"))
	} else {
		t.say(i18n.T("hearing.importSame"))
	}
	t.updateEnabled()
}

// curveChanged passes the audiogram to the draft. An empty or bad cell is NaN, which the
// draft turns into a curve that cannot be applied until it is fixed.
func (t *hearingTab) curveChanged() {
	if t.loading || t.edit == nil {
		return
	}
	var curves [2][]float64
	for ear := range t.bands {
		curves[ear] = make([]float64, hearing.Bands)
		for band, entry := range t.bands[ear] {
			value, err := strconv.ParseFloat(entry.Text, 64)
			if err != nil {
				value = math.NaN()
			}
			curves[ear][band] = value
		}
	}
	t.edit.EditCurve(curves[0], curves[1])
	t.afterEdit()
}

func (t *hearingTab) afterEdit() {
	note := ""
	if t.edit.InvalidCurve {
		note = i18n.T("hearing.badCurve")
	}
	setStatus(t.noteBox, t.editNote, note)
	t.updateEnabled()
	t.w.relayout()
}

// applyDraft checks on a fresh read first, then asks with the list of changes. The write needs
// hearing assistance set up on the AirPods and leaves Hearing Aid on or off as it is.
func (t *hearingTab) applyDraft() {
	if !t.usable() || t.edit == nil || !t.edit.CanApply() {
		return
	}
	baseline, draft := t.edit.Baseline(), t.edit.Draft
	changes := profileChanges(baseline, draft)
	if len(changes) == 0 {
		t.say(i18n.T("hearing.noChanges")) // The edits came back to what is stored.
		return
	}
	t.w.setHearingBusy(true)
	t.say(i18n.T("hearing.checking"))
	var recovery *hearing.Recovery
	t.w.owner.Submit(func(c *dongle.Client) error {
		modes, err := t.w.modesFor(c)
		if err != nil {
			return err
		}
		err = modes.PrepareProfile(c, baseline)
		recovery = recoveryCopy(modes)
		return err
	}, func(err error) {
		t.w.do(func() {
			if err != nil {
				t.w.setHearingBusy(false)
				t.recovery = recovery
				t.say(t.w.requestError(err))
				t.showState()
				return
			}
			t.say("")
			// The write leaves Hearing Aid as it is, on or off.
			text := i18n.T("hearing.changesHead") + "\n" + strings.Join(changes, "\n") + "\n\n" + i18n.T("hearing.warning")
			t.w.confirm(i18n.T("hearing.applyTitle"), text, i18n.T("hearing.apply"), func(accepted bool) {
				if accepted {
					t.write(baseline, draft)
				} else {
					t.w.setHearingBusy(false)
				}
			})
		})
	})
}

// profileChanges words every difference between two profiles, one per line, in the CLI's order.
func profileChanges(before, after hearing.Settings) []string {
	var lines []string
	bands := func(ear string, from, to [hearing.Bands]float32) {
		for i := range from {
			if from[i] != to[i] {
				lines = append(lines, fmt.Sprintf(i18n.T("hearing.changeBand"), ear, hearing.BandHz[i], bandText(from[i]), bandText(to[i])))
			}
		}
	}
	bands(i18n.T("battery.left"), before.LeftEQ, after.LeftEQ)
	bands(i18n.T("battery.right"), before.RightEQ, after.RightEQ)
	for _, row := range []struct {
		key      string
		from, to float32
	}{
		{"hearing.changeLeftGain", before.LeftAmplification, after.LeftAmplification},
		{"hearing.changeRightGain", before.RightAmplification, after.RightAmplification},
		{"hearing.changeLeftTone", before.LeftTone, after.LeftTone},
		{"hearing.changeRightTone", before.RightTone, after.RightTone},
	} {
		if row.from != row.to {
			lines = append(lines, fmt.Sprintf(i18n.T("hearing.changeValue"), i18n.T(row.key), bandText(row.from), bandText(row.to)))
		}
	}
	word := func(on bool) string {
		if on {
			return i18n.T("word.on")
		}
		return i18n.T("word.off")
	}
	for _, row := range []struct {
		key      string
		from, to bool
	}{
		{"hearing.changeLeftBoost", before.LeftConversationBoost, after.LeftConversationBoost},
		{"hearing.changeRightBoost", before.RightConversationBoost, after.RightConversationBoost},
	} {
		if row.from != row.to {
			lines = append(lines, fmt.Sprintf(i18n.T("hearing.changeValue"), i18n.T(row.key), word(row.from), word(row.to)))
		}
	}
	return lines
}

// write sends the draft once; the caller holds the busy state. The write and the steps that
// confirm it each run in short owner jobs, so that status polling goes on in between.
func (t *hearingTab) write(baseline, draft hearing.Settings) {
	t.say(i18n.T("hearing.sending"))
	var modes *hearing.Modes
	var apply *hearing.ProfileApply
	var recovery *hearing.Recovery
	t.w.owner.SubmitCommand(func(c *dongle.Client) (err error) {
		if modes, err = t.w.modesFor(c); err != nil {
			return err
		}
		apply, err = modes.StartApplyProfile(c, baseline, draft)
		recovery = recoveryCopy(modes)
		return err
	}, func(err error) {
		if err != nil {
			t.w.do(func() { t.writeDone(nil, recovery, err) })
			return
		}
		var result *hearing.ProfileResult
		var stepErr error
		t.w.followSteps(func(c *dongle.Client) (done bool) {
			result, done, stepErr = apply.Step(c)
			recovery = recoveryCopy(modes)
			return done
		}, func(cause error) {
			result, _, stepErr = apply.Absent(cause)
			recovery = recoveryCopy(modes)
		}, func() {
			t.w.do(func() { t.writeDone(result, recovery, stepErr) })
		})
	})
}

func (t *hearingTab) writeDone(result *hearing.ProfileResult, recovery *hearing.Recovery, err error) {
	t.w.setHearingBusy(false)
	t.recovery = recovery
	t.readReports()
	if err != nil {
		var refused *hearing.Refused
		if !errors.As(err, &refused) && !errors.Is(err, device.ErrNoDongle) {
			t.needRead = true
		}
		t.say(t.w.requestError(err))
		t.showState()
		return
	}
	t.state, t.have, t.needRead = result.State, true, false
	t.loadProfile(hearing.Profile{Settings: result.Settings, Editable: true})
	text := i18n.T("hearing.applied")
	if !result.Written {
		text = i18n.T("hearing.noChanges")
	}
	t.say(text)
	t.showState()
}

// followSteps waits off the UI thread and off the owner goroutine. Each step is its own short
// owner job, so between steps the owner keeps polling status and serving its queue. step reports
// when the work has ended. absent ends it when the owner has no device; it runs on the owner
// goroutine as step does. finished runs after the end, unless the app is going away.
func (w *Window) followSteps(step func(*dongle.Client) bool, absent func(error), finished func()) {
	go func() {
		for {
			time.Sleep(w.stepEvery)
			done := make(chan bool, 1)
			w.owner.Submit(func(c *dongle.Client) error {
				done <- step(c)
				return nil
			}, func(err error) {
				switch {
				case err == nil:
				case errors.Is(err, device.ErrNoDongle):
					// Not the error itself: it says a request never ran, and this one did.
					absent(errors.New(err.Error()))
					done <- true
				case errors.Is(err, device.ErrStopped):
					close(done)
				default: // A full queue: nothing ran, so look again.
					done <- false
				}
			})
			ended, open := <-done
			if !open {
				return // The app is going away, and the window with it.
			}
			if ended {
				finished()
				return
			}
		}
	}()
}
