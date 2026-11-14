package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/internal/i18n"
	"github.com/Dardrai/airpods-gui/internal/statusdoc"
	"github.com/Dardrai/airpods-gui/internal/updates"
)

var themeChoices = []struct{ value, key string }{
	{ThemeSystem, "app.themeSystem"}, {ThemeDark, "app.themeDark"}, {ThemeLight, "app.themeLight"},
}

// AppControls are the app-wide choices the ... panel shows. They run on the main thread.
type AppControls struct {
	AutoPauseEnabled            func() bool
	SetAutoPauseEnabled         func(bool)
	NotificationsEnabled        func() bool
	SetNotificationsEnabled     func(bool)
	ModeNotificationsEnabled    func() bool
	SetModeNotificationsEnabled func(bool)
	Language                    func() string
	SetLanguage                 func(string)
	ShowBatteries               func()
	// ChooseDongle moves to the dongle with this serial and remembers it.
	ChooseDongle func(serial string)

	// The update check; nil UpdatesEnabled hides it.
	UpdatesEnabled    func() bool
	SetUpdatesEnabled func(bool)
	CheckUpdates      func()
	OpenRelease       func()
	// OpenFirmwareRelease opens the newest dongle firmware release.
	OpenFirmwareRelease func()
	// InstallUpdate downloads and runs the new installer; offered only with CanInstallUpdate.
	InstallUpdate    func()
	CanInstallUpdate bool
}

// appTab is the app settings and the about text, shown in the ... panel.
type appTab struct {
	w            *Window
	settings     fyne.CanvasObject
	info         fyne.CanvasObject
	theme        *widget.Select
	startup      *Switch
	repair       *widget.Label
	loading      bool
	experimental *Switch

	language *widget.Select
	pause    *Switch
	notes    *Switch
	modes    *Switch
	version  *widget.Label
	// firmware repeats the Diagnostics row: the dongle's firmware version, next to the app's.
	firmware *widget.Label
	catalog  *widget.Button

	updates       *Switch
	update        updates.Result
	updateText    *widget.Label
	updateCheck   *widget.Button
	updatePage    *widget.Button
	updateInstall *widget.Button
	firmwareText  *widget.Label
	firmwarePage  *widget.Button
	updateBox     fyne.CanvasObject
	// progress is what an install last said; it wins over the check's result. installing
	// holds the buttons while the download and the installer run.
	progress   string
	installing bool

	dongle        *widget.Select
	dongleReasons *widget.Label
	dongleBox     fyne.CanvasObject
	dongles       []hid.Info
}

const experimentalPreference = "experimental"

func menuCaption(w *Window, key string) fyne.CanvasObject {
	label := newText(i18n.T(key), 13, true, colorSecondary, fyne.TextAlignLeading)
	w.bind(func() { label.SetText(i18n.T(key)) })
	return label
}

func newAppTab(w *Window) *appTab {
	t := &appTab{w: w}
	t.theme = widget.NewSelect(nil, t.themeChanged)
	t.dongle = widget.NewSelect(nil, t.dongleChanged)
	t.dongleReasons = smallLabel("")
	t.dongleBox = container.New(tightLayout{}, w.selectRow("app.dongle", "app.dongleHint", t.dongle), tight(t.dongleReasons), gap(8))
	t.dongleBox.Hide()
	w.bind(func() { t.dongle.PlaceHolder = i18n.T("app.donglePlaceholder") })
	rows := newStack(w.selectRow("app.theme", "", t.theme))
	w.bind(t.relabelTheme)
	controls := w.opts.Controls
	if controls != nil {
		t.language = widget.NewSelect(nil, t.languageChanged)
		rows.add(w.selectRow("app.language", "", t.language))
		w.bind(t.relabelLanguage)
	}
	if w.opts.Autostart != nil {
		t.startup = NewSwitch(t.startupChanged)
		t.repair = w.small("app.startupRepair")
		rows.add(container.New(tightLayout{}, w.optionRow("app.startup", "app.startupHint", t.startup), tight(t.repair)))
		t.loadStartup()
	}
	if controls != nil {
		t.pause = NewSwitch(controls.SetAutoPauseEnabled)
		t.pause.Checked = controls.AutoPauseEnabled()
		rows.add(w.optionRow("app.autoPause", "app.autoPauseHint", t.pause))
		t.notes = NewSwitch(func(on bool) {
			controls.SetNotificationsEnabled(on)
			enableWidget(t.modes, on)
		})
		t.notes.Checked = controls.NotificationsEnabled()
		rows.add(w.optionRow("app.notifications", "app.notificationsHint", t.notes))
		t.modes = NewSwitch(controls.SetModeNotificationsEnabled)
		t.modes.Checked = controls.ModeNotificationsEnabled()
		enableWidget(t.modes, t.notes.Checked)
		rows.add(w.optionRow("app.modeNotes", "app.modeNotesHint", t.modes))
	}
	t.experimental = NewSwitch(func(on bool) {
		w.opts.Fyne.Preferences().SetBool(experimentalPreference, on)
		w.applyExperimental()
	})
	t.experimental.SetChecked(w.opts.Fyne.Preferences().BoolWithFallback(experimentalPreference, false))
	rows.add(w.optionRow("app.experimental", "app.experimentalHint", t.experimental))
	rows.add(w.optionRow("app.debugLog", "app.debugLogHint", w.button("app.copy", t.copyLog)))
	t.settings = container.NewVBox(menuCaption(w, "app.settings"), t.dongleBox, rows)
	w.bind(t.showDongles)
	t.version = newValue("")
	w.bind(func() {
		version := w.opts.Version
		if version == "" {
			version = i18n.T("app.devBuild")
		}
		setLabel(t.version, version)
	})
	t.catalog = w.button("app.catalogShow", t.showCatalog)
	t.catalog.Disable()
	t.firmware = newValue(dashText)
	about := []fyne.CanvasObject{menuCaption(w, "app.about"), w.valueRow("app.version", t.version),
		w.valueRow("device.dongleFirmware", t.firmware)}
	if controls != nil && controls.UpdatesEnabled != nil {
		t.updates = NewSwitch(func(on bool) {
			controls.SetUpdatesEnabled(on)
			t.w.showFirmware()
		})
		t.updates.Checked = controls.UpdatesEnabled()
		rows.add(w.optionRow("app.updates", "app.updatesHint", t.updates))
		t.updateText = smallLabel("")
		t.updateCheck = w.button("update.checkNow", controls.CheckUpdates)
		t.updatePage = w.button("update.page", controls.OpenRelease)
		t.updateInstall = w.button("update.install", controls.InstallUpdate)
		t.updateInstall.Importance = widget.HighImportance
		t.firmwareText = smallLabel("")
		t.firmwarePage = w.button("update.firmwarePage", controls.OpenFirmwareRelease)
		t.updateBox = container.New(tightLayout{}, tight(t.updateText),
			container.NewHBox(layout.NewSpacer(), t.updateCheck, t.updatePage, t.updateInstall),
			tight(t.firmwareText), container.NewHBox(layout.NewSpacer(), t.firmwarePage))
		about = append(about, t.updateBox)
		w.bind(t.showUpdate)
	}
	about = append(about, tight(w.small("app.aboutText")), w.optionRow("app.catalog", "app.catalogHint", t.catalog))
	t.info = container.NewVBox(about...)
	return t
}

// relabelTheme fills the picker in the current language, keeping the saved choice.
func (t *appTab) relabelTheme() {
	current := t.w.opts.Fyne.Preferences().StringWithFallback(themePreference, defaultTheme)
	labels := make([]string, len(themeChoices))
	selected := ""
	for i, choice := range themeChoices {
		labels[i] = i18n.T(choice.key)
		if choice.value == current {
			selected = labels[i]
		}
	}
	t.loading = true
	t.theme.SetOptions(labels)
	t.theme.SetSelected(selected)
	t.loading = false
}

// SetDongles applies a new list of attached dongles. With fewer than two, none of the picker shows.
func (w *Window) SetDongles(list []hid.Info) {
	w.about.dongles = list
	w.about.showDongles()
	if w.menu.pop.Visible() {
		w.menu.open()
	}
}

// showDongles fills the picker: what can be chosen in the list, and under it why the others cannot.
func (t *appTab) showDongles() {
	several := len(t.dongles) > 1 && t.w.opts.Controls != nil
	showIf(t.dongleBox, several)
	if !several {
		return
	}
	var options, reasons []string
	for _, info := range t.dongles {
		switch {
		case info.Err != nil:
			reasons = append(reasons, fmt.Sprintf(i18n.T("app.dongleReason"), dongleName(info), info.Err))
		case info.Serial == "":
			reasons = append(reasons, fmt.Sprintf(i18n.T("app.dongleReason"), dongleName(info), i18n.T("app.dongleNoSerial")))
		case !info.Openable:
			reasons = append(reasons, fmt.Sprintf(i18n.T("app.dongleReason"), dongleName(info), i18n.T("app.dongleBlocked")))
		default:
			options = append(options, info.Serial)
		}
	}
	t.loading = true
	t.dongle.SetOptions(options)
	if t.w.state.Serial == "" {
		t.dongle.ClearSelected()
	} else {
		t.dongle.SetSelected(t.w.state.Serial)
	}
	t.loading = false
	setLabel(t.dongleReasons, strings.Join(reasons, "\n"))
	showIf(t.dongleReasons, len(reasons) > 0)
	t.updateDongle()
}

func dongleName(info hid.Info) string {
	if info.Serial != "" {
		return info.Serial
	}
	return i18n.T("app.dongleNoName")
}

// updateDongle holds the picker still while work that belongs to the open dongle is running.
func (t *appTab) updateDongle() {
	enableWidget(t.dongle, !t.w.hearingBusy && !t.w.diag.capture.running())
}

func (t *appTab) dongleChanged(serial string) {
	if t.loading || serial == t.w.state.Serial || t.w.opts.Controls == nil {
		return
	}
	t.w.nearby.stop() // The finder runs on the dongle being left, and only this window can end it.
	t.w.opts.Controls.ChooseDongle(serial)
}

// relabelLanguage does the same for the languages; System's name is itself translated.
func (t *appTab) relabelLanguage() {
	current := t.w.opts.Controls.Language()
	choices := i18n.Choices()
	labels := make([]string, len(choices))
	selected := ""
	for i, choice := range choices {
		labels[i] = choice.Name
		if choice.Code == current {
			selected = choice.Name
		}
	}
	t.loading = true
	t.language.SetOptions(labels)
	t.language.SetSelected(selected)
	t.loading = false
}

func (t *appTab) languageChanged(label string) {
	if t.loading {
		return
	}
	for _, choice := range i18n.Choices() {
		if choice.Name == label {
			t.w.opts.Controls.SetLanguage(choice.Code)
			return
		}
	}
}

func (t *appTab) loadStartup() {
	on, err := t.w.opts.Autostart.Enabled()
	if err != nil {
		t.w.say(i18n.T("app.startup") + ": " + err.Error())
	}
	t.loading = true
	t.startup.SetChecked(on)
	t.loading = false
	stale, err := t.w.opts.Autostart.Stale()
	if err == nil && stale {
		t.repair.Show()
	} else {
		t.repair.Hide()
	}
}

func (t *appTab) startupChanged(on bool) {
	if t.loading {
		return
	}
	if err := t.w.opts.Autostart.SetEnabled(on); err != nil {
		t.w.say(i18n.T("app.startup") + ": " + err.Error())
	} else {
		t.w.say("")
	}
	t.loadStartup()
}

func (t *appTab) themeChanged(label string) {
	if t.loading {
		return
	}
	for _, choice := range themeChoices {
		if i18n.T(choice.key) == label {
			t.w.opts.Fyne.Preferences().SetString(themePreference, choice.value)
			ApplyTheme(t.w.opts.Fyne)
		}
	}
}

// showCatalog lists what the catalog says about the reported model, as info --capabilities does.
func (t *appTab) showCatalog() {
	t.w.menu.close()
	t.w.inform(i18n.T("app.catalog"), catalogText(t.w.info.Model))
}

func catalogText(number string) string {
	listed := statusdoc.CapabilitiesOf(number)
	if !listed.Listed {
		return i18n.T("app.catalogNotListed")
	}
	yesNo := func(on bool) string {
		if on {
			return i18n.T("hearing.setUpYes")
		}
		return i18n.T("hearing.setUpNo")
	}
	lines := []string{i18n.T("app.catalogFrom")}
	for _, flag := range []struct {
		key string
		on  bool
	}{
		{"app.catalogOverEar", listed.OverEar}, {"app.catalogListening", listed.Listening}, {"app.catalogAdaptive", listed.Adaptive},
		{"app.catalogAllowOff", listed.AllowOffOption}, {"app.catalogMic", listed.HighQualityMic},
		{"app.catalogEQ", listed.CustomEQ}, {"app.catalogHearing", listed.HearingAid},
		{"app.catalogTransparency", listed.CustomTransparency}, {"app.catalogLoud", listed.LoudSoundReduction},
	} {
		lines = append(lines, i18n.T(flag.key)+": "+yesNo(flag.on))
	}
	lines = append(lines, "", i18n.T("app.catalogControls"))
	for _, control := range listed.Controls {
		name := i18n.T("app.catalogNoSetting")
		if control.Name != nil {
			name = *control.Name
		}
		lines = append(lines, control.ID+": "+name)
	}
	return strings.Join(lines, "\n")
}

func (t *appTab) copyLog() {
	text := i18n.T("app.noDebugLog")
	if t.w.opts.Log != nil {
		text = t.w.opts.Log.Text()
	}
	t.w.opts.Fyne.Clipboard().SetContent(text)
	t.w.sayBriefly(i18n.T("app.logCopied"))
}

// showUpdate words the update check: off, checking, failed, up to date or a new version,
// with the buttons that fit. An install in progress says how far it got instead.
func (t *appTab) showUpdate() {
	if t.updateBox == nil {
		return
	}
	controls := t.w.opts.Controls
	on := controls.UpdatesEnabled()
	if t.updates.Checked != on {
		t.updates.Checked = on
		t.updates.Refresh()
	}
	r := t.update
	text := ""
	switch {
	case t.progress != "":
		text = t.progress
	case !on:
	case r.Checking:
		text = i18n.T("update.checking")
	case r.Err != nil && !r.Known:
		text = i18n.T("update.failed")
	case !r.Known:
	case r.Dev:
		text = fmt.Sprintf(i18n.T("update.latest"), r.Latest.Version)
	case r.Available:
		text = fmt.Sprintf(i18n.T("update.available"), r.Latest.Version)
	default:
		text = i18n.T("update.upToDate")
	}
	setLabel(t.updateText, text)
	showIf(t.updateText, text != "")
	busy := t.installing
	showIf(t.updateCheck, on && !r.Available && !busy)
	enableWidget(t.updateCheck, !r.Checking)
	showIf(t.updatePage, on && r.Known && r.Available)
	showIf(t.updateInstall, on && r.Available && controls.CanInstallUpdate)
	enableWidget(t.updateInstall, !busy)
	// The dongle firmware: a newer release than the dongle's own, or only the newest release
	// for firmware that does not report its version.
	fwText := ""
	if latest, newer := t.w.firmwareNewer(); on && newer {
		fwText = fmt.Sprintf(i18n.T("update.firmwareAvailable"), latest.Numbers())
	} else if fu := t.w.firmwareUpdate; on && fu.Known && t.w.dongleFirmware.Unreported {
		fwText = fmt.Sprintf(i18n.T("update.firmwareLatest"), fu.Latest.Version.Numbers())
	}
	setLabel(t.firmwareText, fwText)
	showIf(t.firmwareText, fwText != "")
	showIf(t.firmwarePage, fwText != "")
	showIf(t.updateBox, on || t.progress != "")
	t.w.relayout()
}

// SetUpdate shows the update check's latest result.
func (w *Window) SetUpdate(r updates.Result) {
	if w.about == nil {
		return
	}
	w.about.update = r
	w.about.showUpdate()
}

// UpdateStatus is the line the App tab shows about updates; empty when there is none.
func (w *Window) UpdateStatus() string {
	if w.about == nil || w.about.updateText == nil || !w.about.updateText.Visible() {
		return ""
	}
	return w.about.updateText.Text
}

// UpdateProgress shows how far an install got; busy holds the buttons meanwhile.
func (w *Window) UpdateProgress(text string, busy bool) {
	if w.about == nil {
		return
	}
	w.about.progress, w.about.installing = text, busy
	w.about.showUpdate()
}
