// Package ui is the settings window. Everything except the jobs it hands to the
// device owner runs on the Fyne main thread, and no HID call happens on that thread.
package ui

import (
	"io"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/disconnect"
	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/hearing"
	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/autostart"
	"github.com/Dardrai/airpods-gui/internal/choices"
	"github.com/Dardrai/airpods-gui/internal/debuglog"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
	"github.com/Dardrai/airpods-gui/internal/tray"
	"github.com/Dardrai/airpods-gui/internal/updates"
)

const (
	readRetryTicks    = 5
	settingsPollTicks = 2 // NTPods reads the settings every 2 s too
	quietReadFailures = 3 // repeated read failures before the banner speaks up
	rpcTickDefault    = time.Second
	diagnosticsPeriod = 3 // in ticks
	noticeTime        = 4 * time.Second

	defaultWidth  float32 = 640
	defaultHeight float32 = 900
	// dialogScrollLines is how many lines a dialog shows before it scrolls.
	dialogScrollLines = 12
)

// Options configures the window.
type Options struct {
	Fyne    fyne.App
	Owner   *device.Owner
	Version string
	// Do runs a function on the UI thread; nil means fyne.Do.
	Do func(func())
	// Quit ends the app.
	Quit func()
	// Tray decides whether closing the window can hide it to a tray. It runs on the main
	// thread, so it must answer at once, with tray.Unknown while nobody can tell yet; the
	// answer that follows goes to TrayAnswered. nil means there is always a tray.
	Tray func() tray.Answer
	// TrayRecheck asks for a fresh answer soon, without waiting for it; nil does nothing.
	TrayRecheck func()
	// Autostart is the start-with-OS entry; nil hides the option.
	Autostart autostart.Manager
	// Log receives a record for the debug log; nil keeps nothing.
	Log *debuglog.Log
	// Choices remembers per-pair values; nil keeps nothing.
	Choices *choices.Store
	// OnReports receives the settings the AirPods reported, on the main thread.
	OnReports func(map[byte]uint32)
	// Disconnect ends the AirPods connection, or explains how to; nil hides the action.
	Disconnect func()
	// Controls are the app-wide choices in the ... panel; nil leaves them out.
	Controls *AppControls
	// Picker lists the attached dongles for the copied list; nil means only the open one is known.
	Picker *device.Picker
	// Tick overrides the period of the visible-window refresh.
	Tick time.Duration
}

// Window is the settings window. Its methods run on the main thread.
type Window struct {
	// dongleFirmware and firmwareUpdate are the dongle's firmware and the newest release seen.
	dongleFirmware DongleFirmware
	firmwareUpdate updates.Result

	opts  Options
	do    func(func())
	owner *device.Owner
	win   fyne.Window
	state device.State
	info  dongle.Metadata

	banner *widget.Label
	scroll *container.Scroll
	page   *fyne.Container
	header *pageHeader
	menu   *overflowPanel

	device   *deviceTab
	settings *settingsTab
	audio    *audioTab
	nearby   *nearbyTab
	diag     *diagTab
	about    *appTab
	hardware *hardwareTab
	hearing  *hearingTab
	extra    *experimentalTab

	// texts put the current language on what they labelled; each came through bind.
	texts []func()
	// hearingModes tracks mode reports per pair. Only owner jobs may touch it.
	hearingModes map[[6]byte]*hearing.Modes
	// hearingBusy holds the hearing, check and experimental requests to one at a time.
	hearingBusy bool
	// checkEvery is how often a running in-case check looks for results.
	checkEvery time.Duration
	// stepEvery is how often a hearing change that is being confirmed looks for its outcome.
	stepEvery time.Duration

	// release is the routing release the last Disconnect started, for releasePeer; 0 for none.
	release     disconnect.Outcome
	releasePeer [6]byte
	// disconnecting is set while a Disconnect is out, reconnecting while a Reconnect is.
	disconnecting, reconnecting bool
	// takeover is set while a Reconnect the dongle accepted is still taking the AirPods over,
	// for takeoverPeer and until takeoverUntil at the latest.
	takeover      bool
	takeoverPeer  [6]byte
	takeoverUntil time.Time
	// allowOff is the last Allow Off value sent, by the app or the page.
	allowOff allowOffSent

	// sections are the page's collapsible rows, each with what decides if it shows.
	sections                              []*pageSection
	extras                                *fyne.Container
	playbackRow, nearbyRow, connectionRow *pageSection
	diagnosticsRow, hardwareRow           *pageSection

	visible bool
	// closedBlind is set while the window is hidden by a close made before the tray check
	// had answered.
	closedBlind bool
	ticks       int
	stopTick    chan struct{}
	lastLink    linkKey
	haveFirst   bool
	notice      *time.Timer
	closed      bool
	// stopRetext ends the following of the language, which outlives the window otherwise.
	stopRetext func()

	// Replaced in tests; dialogs need a real window to answer them.
	confirm func(title, text, accept string, answer func(bool))
	inform  func(title, text string)
	// pickFile asks where to save; it calls chosen with a nil writer if the user cancels.
	pickFile func(name string, chosen func(io.WriteCloser, error))
	// openFile asks what to read; it calls chosen with a nil reader if the user cancels.
	openFile func(chosen func(io.ReadCloser, error))
}

// linkKey is what forces settings to be read again when it changes.
type linkKey struct {
	present    bool
	ready      bool
	generation uint32
}

// New builds the hidden window.
func New(opts Options) *Window {
	w := &Window{opts: opts, do: opts.Do, owner: opts.Owner, hearingModes: map[[6]byte]*hearing.Modes{}, checkEvery: time.Second, stepEvery: 250 * time.Millisecond}
	if w.do == nil {
		w.do = fyne.Do
	}
	if w.opts.Tick == 0 {
		w.opts.Tick = rpcTickDefault
	}
	w.win = opts.Fyne.NewWindow("AirPods")
	w.confirm = w.confirmDialog
	w.pickFile = w.pickFileDialog
	w.openFile = w.openFileDialog
	w.inform = w.informDialog
	w.banner = smallLabel("")
	w.banner.Alignment = fyne.TextAlignCenter
	w.banner.Hide()

	w.header = newPageHeader(func() { w.menu.open() })
	w.settings = newSettingsTab(w)
	w.device = newDeviceTab(w)
	w.audio = newAudioTab(w)
	w.nearby = newNearbyTab(w)
	w.diag = newDiagTab(w)
	w.about = newAppTab(w)
	w.hardware = newHardwareTab(w)
	w.hearing = newHearingTab(w)
	w.extra = newExperimentalTab(w)
	w.menu = newOverflowPanel(w)

	w.buildSections()
	w.extras = container.New(tightLayout{})
	// What the AirPods' own settings page has comes first; the rest needs the experimental switch.
	w.page = container.New(tightLayout{}, w.header.content, w.settings.content, w.device.about, w.device.leave,
		w.settings.experiments, w.playbackRow.object, w.nearbyRow.object, w.connectionRow.object, w.diagnosticsRow.object,
		w.extras, w.hardwareRow.object)
	w.scroll = container.NewVScroll(container.New(columnLayout{}, container.New(insetLayout{top: 15, bottom: 32}, w.page)))
	w.win.SetContent(container.NewBorder(nil, w.banner, nil, nil, w.scroll))
	w.win.Resize(fyne.NewSize(defaultWidth, defaultHeight))
	w.win.SetCloseIntercept(w.CloseRequested)
	w.addFeatureSections()
	w.Update(device.State{})
	w.settings.fitModel()
	w.stopRetext = i18n.OnChange(w.retext)
	return w
}

// retext puts the current language on the whole window, in place: nothing is rebuilt, so
// open sections, unsent edits and requests in flight all carry on.
func (w *Window) retext() {
	if w.closed {
		return
	}
	for _, apply := range w.texts {
		apply()
	}
	w.header.relabel()
	w.settings.relabel()
	w.Update(w.state)
	w.relayout()
	if w.menu.pop.Visible() {
		w.menu.open()
	}
}

// buildSections makes the collapsible rows, all of them behind the experimental switch. A
// row with no way to be useful right now is hidden, never shown empty, and folds when it goes.
func (w *Window) buildSections() {
	present := func() bool { return w.state.Present && w.Experimental() }
	usesRequests := func() bool { return present() && w.state.Status.Has(dongle.CapTypedRPC) }
	w.playbackRow = w.keyedSection("section.playback", "section.playbackHint", w.audio.content, usesRequests, nil)
	w.nearbyRow = w.keyedSection("section.nearby", "section.nearbyHint", w.nearby.content, usesRequests, func(open bool) {
		if !open {
			w.nearby.stop()
		}
	})
	w.connectionRow = w.keyedSection("section.connection", "section.connectionHint", w.device.connection, present, func(open bool) {
		if open {
			w.device.policy.readPolicy() // On demand only: never with the status poll.
		}
	})
	w.diagnosticsRow = w.keyedSection("section.diagnostics", "section.diagnosticsHint", w.diag.content, present, func(open bool) {
		if open {
			w.diag.read()
		}
	})
	w.hardwareRow = w.keyedSection("section.hardware", "section.experimental", w.hardware.content, func() bool {
		view := w.view()
		return present() && (!view.Identified() || hearing.Supported(view.Model))
	}, nil)
}

// keyedSection is newPageSection with texts that follow the language.
func (w *Window) keyedSection(titleKey, subtitleKey string, body fyne.CanvasObject, show func() bool, toggled func(bool)) *pageSection {
	entry := w.newPageSection(i18n.T(titleKey), i18n.T(subtitleKey), body, show, toggled)
	w.bind(func() {
		entry.fold.Title.SetText(i18n.T(titleKey))
		entry.fold.Subtitle.SetText(i18n.T(subtitleKey))
	})
	return entry
}

// pageSection is one collapsible row and its gate.
type pageSection struct {
	fold   *section
	object fyne.CanvasObject
	show   func() bool
}

func (w *Window) newPageSection(title, subtitle string, body fyne.CanvasObject, show func() bool, toggled func(bool)) *pageSection {
	fold := newSection(title, subtitle, body)
	fold.OnToggle = func(open bool) {
		if toggled != nil {
			toggled(open)
		}
		w.relayout()
	}
	entry := &pageSection{fold: fold, object: container.New(insetLayout{bottom: foldGap}, fold), show: show}
	entry.object.Hide()
	w.sections = append(w.sections, entry)
	return entry
}

// AddSection adds a collapsible row above the hardware checks, for a feature wired in
// later. show decides whether it is on the page; toggled, which may be nil, hears it open
// and close, so that a feature can start and stop what it reads.
func (w *Window) AddSection(title, subtitle string, body fyne.CanvasObject, show func() bool, toggled func(open bool)) {
	entry := w.newPageSection(title, subtitle, body, show, toggled)
	w.extras.Add(entry.object)
	w.gate()
}

// gate applies each section's rule. It moves nothing unless a rule's answer changed, so
// it is safe on every status poll.
func (w *Window) gate() {
	changed := false
	for _, entry := range w.sections {
		on := entry.show()
		if on == entry.object.Visible() {
			continue
		}
		changed = true
		if !on && entry.fold.Open() {
			entry.fold.Toggle()
		}
		showIf(entry.object, on)
	}
	if changed {
		w.relayout()
	}
}

// relayout fits the page to rows that appeared, went or changed height. The scroll offset
// stays where it is unless the page became too short for it.
func (w *Window) relayout() {
	if w.page == nil {
		return
	}
	w.page.Refresh()
	w.scroll.Refresh()
}

// SetInform replaces the message dialog, for tests in other packages.
func (w *Window) SetInform(inform func(title, text string)) { w.inform = inform }

// Canvas returns the window's canvas.
func (w *Window) Canvas() fyne.Canvas { return w.win.Canvas() }

// Visible reports whether the window is showing.
func (w *Window) Visible() bool { return w.visible }

// CloseRequested is the window's close button: to the tray when there is one, out of the
// app when there is none.
func (w *Window) CloseRequested() {
	answer := tray.Yes
	if w.opts.Tray != nil {
		answer = w.opts.Tray()
	}
	switch answer {
	case tray.No:
		// With no tray a hidden window would leave no way back.
		if w.opts.Quit != nil {
			w.opts.Quit()
		}
		return
	case tray.Unknown:
		// Nobody can tell yet, and a session bus that hangs may never say. Quitting on a
		// guess would end an app that has a tray, so the window hides as it would with
		// one, and TrayAnswered does what this close meant once the answer is in.
		w.closedBlind = true
	}
	w.Hide()
}

// TrayAnswered takes the answer to the tray check, on the main thread, and applies it to a
// window closed before anyone knew: with a tray it stays hidden, and with none the app
// quits, as that close would have done. A window opened again since is left alone.
func (w *Window) TrayAnswered(answer tray.Answer) {
	if answer == tray.Unknown || !w.closedBlind {
		return
	}
	w.closedBlind = false
	if answer != tray.No || w.visible || w.closed {
		return
	}
	if w.opts.Quit == nil {
		// Nothing to quit with: the window comes back, since nothing else leads to it.
		w.Show()
		return
	}
	w.logf("the window was closed before the tray check answered, and there is no tray: quitting")
	w.opts.Quit()
}

// Show opens the window and starts the refreshes that only run while it is visible.
func (w *Window) Show() {
	w.win.Show()
	w.win.RequestFocus()
	// Shown again, by whatever way, the close made blind is undone; and trays come and
	// go, so the next close should go by a fresh answer.
	w.closedBlind = false
	if w.opts.TrayRecheck != nil {
		w.opts.TrayRecheck()
	}
	if w.visible {
		return
	}
	w.visible = true
	w.refresh()
	w.startTicker()
}

// Hide closes the window and stops all report 3 traffic it caused.
func (w *Window) Hide() {
	w.win.Hide()
	if !w.visible {
		return
	}
	w.visible = false
	w.stopTicker()
	w.nearby.stop()
}

// Shutdown stops what must not outlive the app. Queued jobs still run before the
// owner closes the device.
func (w *Window) Shutdown() {
	w.closed = true
	w.stopRetext()
	if w.notice != nil {
		w.notice.Stop()
	}
	w.stopTicker()
	w.visible = false
	w.nearby.stop()
	w.hardware.stopCheck()
	w.diag.capture.shutdown()
}

func (w *Window) startTicker() {
	stop := make(chan struct{})
	w.stopTick = stop
	period := w.opts.Tick
	go func() {
		ticker := time.NewTicker(period)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				w.do(w.tick)
			}
		}
	}()
}

func (w *Window) stopTicker() {
	if w.stopTick != nil {
		close(w.stopTick)
		w.stopTick = nil
	}
}

func (w *Window) now() time.Time { return time.Now() }

func (w *Window) logf(format string, args ...any) { w.opts.Log.Printf(format, args...) }

// tick runs the periodic reads; each kind has at most one read in flight.
func (w *Window) tick() {
	if !w.visible {
		return
	}
	w.ticks++
	w.settings.retry()
	if w.settings.due(w.ticks) {
		w.settings.poll()
	}
	w.settings.followArrival()
	if w.state.Present && w.ticks%readRetryTicks == 0 {
		if !w.audio.have {
			w.audio.read()
		}
		if w.nearby.state.snapshot == nil {
			w.nearby.read()
		}
	}
	w.nearby.poll()
	if w.diagnosticsRow.fold.Open() && w.ticks%diagnosticsPeriod == 0 {
		w.diag.read()
	}
}

// refresh reads what the window shows.
func (w *Window) refresh() {
	if !w.visible || !w.state.Present {
		return
	}
	w.settings.read()
	w.audio.read()
	w.nearby.read()
	if w.diagnosticsRow.fold.Open() {
		w.diag.read()
	}
}

// model is the reported AirPods model number, empty until it is known.
func (w *Window) model() string { return w.info.Model }

// Experimental reports whether the user has turned on the experimental cards.
func (w *Window) Experimental() bool {
	return w.opts.Fyne.Preferences().BoolWithFallback(experimentalPreference, false)
}

// applyExperimental shows or hides what the experimental switch controls.
func (w *Window) applyExperimental() {
	if w.hardwareRow != nil { // Not yet while the panel is built with the saved choice.
		w.diag.updateEnabled()
		w.settings.fitModel()
		w.gate()
		w.relayout()
	}
}

// SetInfo applies what the AirPods reported about themselves.
func (w *Window) SetInfo(info dongle.Metadata) {
	if info == w.info {
		return
	}
	w.info = info
	w.device.showInfo(info)
	enableWidget(w.about.catalog, info.Model != "")
	w.settings.fitModel()
	w.audio.updateEnabled()
	w.hardware.updateEnabled()
	w.hearing.updateEnabled()
	w.extra.fit()
	w.showHeader()
	w.gate()
}

// showHeader fills the header in, with whether the page still waits for the first reports.
func (w *Window) showHeader() {
	w.settings.settled = w.settings.settling()
	w.header.settling = w.settings.settled
	w.header.update(w.state, w.info.Model)
}

// view is the current identity and reports for the model gates.
func (w *Window) view() model.View {
	view := model.View{Model: w.info.Model, Firmware: w.info.Firmware, Reported: nil}
	if w.settings != nil {
		view.Reported = w.settings.viewReports()
		view.AllowOffRequested = w.settings.allowOffRequested()
	}
	return view
}

// AllowOffRequested is the Allow Off value sent or remembered for this pair and not
// reported back, nil when there is none.
func (w *Window) AllowOffRequested() *bool { return w.settings.allowOffRequested() }

// Update applies a new device state.
func (w *Window) Update(st device.State) {
	refit := st.Present != w.state.Present
	// A mode or capability the status gained means the AirPods just reported something.
	moved := st.Status.ANC != w.state.Status.ANC || st.Status.Capabilities != w.state.Status.Capabilities
	peerChanged := st.Status.Peer != w.state.Status.Peer
	w.followRelease(st)
	w.followTakeover(st)
	w.state = st
	if peerChanged {
		w.settings.loadChoices()
	}
	w.showHeader()
	w.device.update(st)
	w.settings.fitAway()
	if w.settings.followStatus() {
		w.settings.fitModel()
	}
	w.settings.updateEnabled()
	w.audio.updateEnabled()
	w.nearby.updateEnabled()
	w.diag.updateEnabled()
	w.hardware.updateEnabled()
	w.hearing.linkUpdate(st)
	w.extra.linkUpdate(st)
	w.about.showDongles()
	if refit {
		w.settings.fitModel()
		w.relayout()
	}
	w.gate()
	key := linkKey{present: st.Present, ready: st.Ready(), generation: st.Status.Generation}
	if !w.haveFirst || key != w.lastLink {
		w.logf("link present=%v ready=%v generation=%d", key.present, key.ready, key.generation)
		w.haveFirst, w.lastLink = true, key
		w.settings.linkChanged()
		w.refresh()
	} else if moved && w.visible {
		w.settings.poll()
	}
}

// say shows the last problem in one quiet line at the bottom of the window; empty clears it.
func (w *Window) say(text string) {
	setLabel(w.banner, text)
	showIf(w.banner, text != "")
}

// sayBriefly shows a result that needs no action and clears it after a moment, unless
// something else has taken the line by then.
func (w *Window) sayBriefly(text string) {
	w.say(text)
	if w.notice != nil {
		w.notice.Stop()
	}
	w.notice = time.AfterFunc(noticeTime, func() {
		w.do(func() {
			if w.banner.Text == text && !w.closed {
				w.say("")
			}
		})
	})
}

// Message shows the window with a dialog on top of it.
func (w *Window) Message(title, text string) {
	w.Show()
	w.inform(title, text)
}

func wrapped(text string) *widget.Label {
	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapWord
	return label
}

// dialogBody is the text of a dialog. A long list scrolls, so that the buttons stay on the window.
func dialogBody(text string) fyne.CanvasObject {
	if strings.Count(text, "\n") < dialogScrollLines {
		return wrapped(text)
	}
	scroll := container.NewVScroll(wrapped(text))
	scroll.SetMinSize(fyne.NewSize(400, 320))
	return scroll
}

func (w *Window) confirmDialog(title, text, accept string, answer func(bool)) {
	w.showSheet(title, text, func(shut func()) []fyne.CanvasObject {
		return []fyne.CanvasObject{
			sheetButton(i18n.T("dialog.cancel"), false, func() { shut(); answer(false) }),
			sheetButton(accept, true, func() { shut(); answer(true) }),
		}
	})
}

func (w *Window) informDialog(title, text string) {
	w.showSheet(title, text, func(shut func()) []fyne.CanvasObject {
		return []fyne.CanvasObject{sheetButton(i18n.T("dialog.ok"), true, shut)}
	})
}
