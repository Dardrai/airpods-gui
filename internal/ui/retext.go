package ui

import (
	"errors"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/hearing"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

// bind runs apply now and again after each language change. Everything on the page that
// shows fixed text goes through it, by way of the constructors below.
func (w *Window) bind(apply func()) {
	w.texts = append(w.texts, apply)
	apply()
}

// valueRow is valueRow with a title that follows the language.
func (w *Window) valueRow(key string, value fyne.CanvasObject) fyne.CanvasObject {
	row := valueRow(i18n.T(key), value).(*fyne.Container)
	label := row.Objects[0].(*widget.Label)
	w.texts = append(w.texts, func() { label.SetText(i18n.T(key)) })
	return row
}

// optionRow is newOptionRow with texts that follow the language; an empty hint key has no hint.
func (w *Window) optionRow(titleKey, hintKey string, control fyne.CanvasObject) *optionRow {
	row := newOptionRow("", "", control)
	w.bind(func() {
		row.Title.SetText(i18n.T(titleKey))
		if hintKey != "" {
			row.SetHint(i18n.T(hintKey))
		}
	})
	return row
}

func (w *Window) selectRow(titleKey, hintKey string, sel *widget.Select) *optionRow {
	return w.optionRow(titleKey, hintKey, compactSelect(sel))
}

func (w *Window) button(key string, tapped func()) *widget.Button {
	button := widget.NewButton("", tapped)
	w.bind(func() { button.SetText(i18n.T(key)) })
	return button
}

func (w *Window) small(key string) *widget.Label {
	label := smallLabel("")
	w.bind(func() { label.SetText(i18n.T(key)) })
	return label
}

func (w *Window) subCaption(key string) fyne.CanvasObject {
	label := newText(i18n.T(key), 12, true, colorSecondary, fyne.TextAlignLeading)
	w.bind(func() { label.SetText(i18n.T(key)) })
	return container.NewVBox(gap(4), label)
}

func (w *Window) menuItem(key string, tapped func()) *menuItem {
	item := newMenuItem(i18n.T(key), tapped)
	w.bind(func() {
		item.Label = i18n.T(key)
		item.Refresh()
	})
	return item
}

func setStatusObject(box fyne.CanvasObject, label *widget.Label, value string) {
	setStatus(box, label, value)
}

// requestError words a failed hearing or experimental request. A refusal is shown as it was
// given, since nothing changed. A request with no answer is shown as sent; the log keeps
// the detail, and it is never sent again.
func (w *Window) requestError(err error) string {
	var refused *hearing.Refused
	var mode *hearing.ModeUnconfirmed
	var uncertain *dongle.Unconfirmed
	switch {
	case errors.Is(err, device.ErrNoDongle):
		return i18n.T("state.noDongle")
	case errors.As(err, &refused):
		return err.Error()
	case errors.As(err, &mode), errors.As(err, &uncertain):
		w.logf("request sent, no answer: %v", err)
		return device.UnconfirmedNote()
	}
	return device.Explain(err)
}

// setHearingBusy holds or frees the hearing, check and experimental requests together.
func (w *Window) setHearingBusy(busy bool) {
	w.hearingBusy = busy
	w.hardware.updateEnabled()
	w.hearing.updateEnabled()
	w.extra.updateEnabled()
	w.about.updateDongle()
	w.diag.updateEnabled()
	w.device.policy.updateEnabled()
}

// modesFor is the mode tracker of the connected pair. Only owner jobs may call it.
func (w *Window) modesFor(c *dongle.Client) (*hearing.Modes, error) {
	status, err := c.Status()
	if err != nil {
		return nil, err
	}
	modes := w.hearingModes[status.Peer]
	if modes == nil {
		modes = new(hearing.Modes)
		w.hearingModes[status.Peer] = modes
	}
	return modes, nil
}

// addFeatureSections puts the experimental features on the page.
func (w *Window) addFeatureSections() {
	w.addKeyed("section.hearing", "section.experimental", w.hearing.content, func() bool {
		return w.state.Present && w.Experimental() && w.info.Model != "" && hearing.Supported(w.info.Model)
	})
	w.addKeyed("section.extra", "section.extraHint", w.extra.content, func() bool {
		return w.state.Present && w.Experimental() && w.info.Model != ""
	})
}

func (w *Window) addKeyed(titleKey, subtitleKey string, body fyne.CanvasObject, show func() bool) {
	w.AddSection(i18n.T(titleKey), i18n.T(subtitleKey), body, show, nil)
	entry := w.sections[len(w.sections)-1]
	w.bind(func() {
		entry.fold.Title.SetText(i18n.T(titleKey))
		entry.fold.Subtitle.SetText(i18n.T(subtitleKey))
	})
}
