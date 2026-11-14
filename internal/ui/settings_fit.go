package ui

import (
	"strconv"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

// linkStrength puts the strength on the pill too: the capsule follows the value in the
// Adaptive lane, and dragging it writes the same setting as the slider.
func (t *settingsTab) linkStrength() {
	mode, strength := t.byName["mode"], t.byName["adaptive-strength"]
	if mode == nil || strength == nil {
		return
	}
	show := strength.show
	strength.show = func(decoded string, known bool) {
		show(decoded, known)
		value := -1
		if parsed, err := strconv.Atoi(decoded); known && err == nil {
			value = parsed
		}
		if value != mode.segment.strength {
			mode.segment.SetStrength(value)
		}
	}
	t.strengthDrag = func(value int) { t.write(strength, strconv.Itoa(value)) }
}

// fitStrengthDrag leaves the capsule draggable only while the strength can be written.
func (t *settingsTab) fitStrengthDrag() {
	mode, strength := t.byName["mode"], t.byName["adaptive-strength"]
	if mode == nil || strength == nil {
		return
	}
	usable := t.w.state.Ready() && !strength.writing && t.adaptiveReady(strength)
	if usable != (mode.segment.OnStrength != nil) {
		mode.segment.OnStrength = nil
		if usable {
			mode.segment.OnStrength = t.strengthDrag
		}
		mode.segment.Refresh()
	}
}

// fitModel shows what the reported model has, under the names it uses for them.
func (t *settingsTab) fitModel() {
	view := t.w.view()
	for _, row := range t.rows {
		if row.refit != nil {
			row.refit(view)
		}
		label := settingLabel(row.setting.Name)
		if view.Headphone() {
			label = translated("setting.hp."+row.setting.Name, label)
		}
		setLabel(row.label, label)
		showIf(row.body, view.SupportsControl(row.setting.Control))
	}
	t.fitStrength()
	for _, group := range t.cards {
		any := false
		for _, row := range group.rows {
			any = any || row.body.Visible()
		}
		title := cardTitle(group.base)
		if group.base == holdGroup && view.Headphone() {
			title = i18n.T("group.Buttons")
		}
		group.title.SetText(title)
		showIf(group.group, any && t.w.state.Present && (group.base != experimentGroup || t.w.Experimental()))
		group.list.tidy()
	}
	for _, row := range t.rows {
		if !row.reported && row.requested == "" && !row.writing && !row.problem {
			setNote(row, t.unreportedNote(row.setting))
		}
	}
	t.updateEnabled()
}

// tidyHints keeps one "not reported" or "still arriving" line per card: the rows are
// already dimmed, and the same words on each of them would only stack up.
func (t *settingsTab) tidyHints() {
	hint, arriving := notReportedHint(), arrivingHint()
	changed := false
	for _, group := range t.cards {
		seen := false
		for _, row := range group.rows {
			if row.note.Text != hint && row.note.Text != arriving {
				continue
			}
			wanted := !seen && row.body.Visible()
			seen = seen || wanted
			changed = row.note.Visible() != wanted || changed
			showIf(row.note, wanted)
			if row.help != nil {
				showIf(row.help, !wanted)
			}
		}
	}
	if changed && t.w.page != nil {
		for _, group := range t.cards {
			group.list.tidy()
		}
		t.w.relayout()
	}
}

// relabel puts the current language on everything the rows show.
func (t *settingsTab) relabel() {
	t.applying = true
	for _, row := range t.rows {
		if row.relabel != nil {
			row.relabel()
		}
		if row.help != nil {
			setLabel(row.help, settingHelp(row.setting.Name))
		}
		value, known := row.display()
		row.show(value, known)
		switch {
		case row.writing:
		case row.requested != "":
			setNote(row, sentText())
		case !row.reported:
			setNote(row, t.unreportedNote(row.setting))
		case row.known:
			setNote(row, "")
		}
	}
	t.applying = false
	t.fitModel()
}

// fitStrength shows the strength slider only while Adaptive is the mode on show.
func (t *settingsTab) fitStrength() {
	strength, mode := t.byName["adaptive-strength"], t.byName["mode"]
	if strength == nil || mode == nil || t.strength == nil {
		return
	}
	value, known := mode.display()
	adaptive := known && value == "adaptive" && mode.body.Visible() && t.w.view().SupportsControl(strength.setting.Control)
	// The pill's Adaptive lane is the slider. The separate one is only for a pill too
	// narrow to open the lane, and only while the pill itself is on Adaptive: judged by
	// what the pill can do, not by what it shows right now, or the slider would flash
	// while a change of mode is on its way.
	width := mode.segment.Size().Width
	onPill := width <= 0 || mode.segment.laneCapable(width)
	showIf(t.strength, adaptive && !onPill && mode.segment.Selected == wordLabel("adaptive"))
}

// unreportedNote is the hint of a row with no report: none when it can be used anyway
// or when no read has finished yet, one short line when it can't. While the reports are
// still arriving that line says so, rather than that the AirPods do not report it.
func (t *settingsTab) unreportedNote(s *dongle.Setting) string {
	// With nothing reported at all the AirPods have not spoken yet; the header says so.
	if t.w.view().CanChoose(s.Control) || !t.everRead || len(t.reported) == 0 {
		return ""
	}
	if t.arriving() {
		return arrivingHint()
	}
	return notReportedHint()
}

// arriving holds for a moment after a read found more reports than the one before, the
// first read of a link included: the dongle lists only what the AirPods have sent so far,
// and right after they connect that grows one setting at a time.
func (t *settingsTab) arriving() bool {
	return t.everRead && t.sinceLink <= t.eagerUntil && t.w.state.Ready()
}

// followArrival rewords the rows without a report once the reports stop arriving.
func (t *settingsTab) followArrival() {
	if arriving := t.arriving(); arriving != t.wasArriving {
		t.wasArriving = arriving
		t.fitModel()
	}
}

// adaptiveReady holds for adaptive strength only with Adaptive confirmed by a report or
// the status, no mode change outstanding, and a strength report to start from.
func (t *settingsTab) adaptiveReady(strength *settingRow) bool {
	mode := t.byName["mode"]
	if mode == nil || !strength.reported || mode.writing || mode.requested != "" || mode.unconfirmed {
		return false
	}
	current, known := mode.display()
	return known && current == "adaptive" && (mode.reported || mode.status != "")
}

func (t *settingsTab) rowUsable(row *settingRow) bool {
	if row.setting.Control == adaptiveStrengthControl {
		return t.adaptiveReady(row)
	}
	return row.reported || row.status != "" || t.w.view().CanChoose(row.setting.Control)
}

// followStatus shows the listening mode the status carries while the mode has no report
// of its own: the status has it first, and for some AirPods the report never comes. It
// reports whether the pill changed.
func (t *settingsTab) followStatus() bool {
	mode := t.byName["mode"]
	if mode == nil {
		return false
	}
	word, value := "", t.w.state.Status.ANC
	if t.w.state.Ready() && !mode.reported && value >= 1 && value <= 4 {
		word = mode.setting.Decode(uint32(value))
	}
	if word == mode.status {
		return false
	}
	// A mode the status moved to answers what was sent, whoever changed it.
	if word != "" && !mode.writing {
		mode.requested = ""
	}
	mode.status, t.overlay = word, nil
	if !mode.writing {
		t.restore(mode)
	}
	return true
}

// dropIgnoredMode runs after each read. With no report for the mode a tap is shown as
// sent, and only the status can answer it; a status that still carries another mode once
// the write is done and a read has passed means the AirPods never took it.
func (t *settingsTab) dropIgnoredMode() {
	mode := t.byName["mode"]
	if mode == nil || mode.writing || mode.reported || mode.requested == "" || mode.status == "" || mode.status == mode.requested {
		return
	}
	t.w.logf("mode: %s was sent, the status stays on %s", mode.requested, mode.status)
	mode.requested = ""
	setNote(mode, "")
	t.restore(mode)
}

// viewReports is what the model gates see: the reports, and the status's listening mode
// until the mode has a report of its own.
func (t *settingsTab) viewReports() map[byte]uint32 {
	mode := t.byName["mode"]
	if mode == nil || mode.status == "" {
		return t.reported
	}
	if _, ok := t.reported[model.ControlMode]; ok {
		return t.reported
	}
	if t.overlay == nil {
		t.overlay = make(map[byte]uint32, len(t.reported)+1)
		for control, value := range t.reported {
			t.overlay[control] = value
		}
		t.overlay[model.ControlMode] = uint32(t.w.state.Status.ANC)
	}
	return t.overlay
}

// fitAway blanks every row while the AirPods are not there and brings the values back
// with them: an old value, even dimmed, reads as the current one.
func (t *settingsTab) fitAway() {
	away := !t.w.state.Ready()
	if away == t.away {
		return
	}
	t.away = away
	for _, row := range t.rows {
		row.away = away
		if away || !row.writing {
			t.restore(row)
		}
	}
}

// settling holds from a link's first read until the AirPods report anything at all, and
// while the dongle refuses the snapshot because the AirPods controls are not up yet.
func (t *settingsTab) settling() bool {
	return (t.loading || (t.everRead && len(t.reported) == 0)) && t.w.state.Ready() && t.w.state.Status.Has(dongle.CapTypedRPC)
}

func (t *settingsTab) updateEnabled() {
	ready := t.w.state.Ready()
	for _, row := range t.rows {
		row.enable(ready && !row.writing && t.rowUsable(row))
	}
	t.fitStrength()
	t.fitStrengthDrag()
	if strength := t.byName["adaptive-strength"]; strength != nil && !strength.writing && strength.requested == "" {
		switch {
		case !strength.reported:
			setNote(strength, t.unreportedNote(strength.setting))
		case !t.adaptiveReady(strength):
			setNote(strength, adaptiveOnlyText())
		case strength.note.Text == adaptiveOnlyText():
			setNote(strength, "")
		}
	}
	t.tidyHints()
	canRead := !t.coolingDown && !t.w.state.Status.MicrophoneActive && t.w.state.Present && t.w.state.Status.Has(dongle.CapTypedRPC)
	setEnabled(t.rereadBt, canRead)
	enableWidget(t.rereadItem, canRead)
}
