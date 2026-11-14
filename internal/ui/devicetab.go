package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/disconnect"
	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
	"github.com/Dardrai/airpods-gui/internal/tray"
	"github.com/Dardrai/airpods-gui/internal/updates"
)

// deviceTab is what the AirPods say about themselves, the connection state in words and
// the connection actions. The first is the About card and the state goes in Diagnostics;
// the actions are in their section and in the ... panel, and Disconnect has its own card.
// While the AirPods are away that card offers Reconnect instead, on dongles with the routing
// handover: under the default Manual reconnect policy the dongle does not connect to them on
// its own, and after a release it declines them until a reconnect or a restart. The
// Connection section also has the reconnect policy, read when the section opens.
type deviceTab struct {
	w        *Window
	about    fyne.CanvasObject
	leave    fyne.CanvasObject // the Disconnect card
	leaveRow *optionRow
	// leaveReconnects is set while the Disconnect card offers Reconnect.
	leaveReconnects bool
	status          fyne.CanvasObject
	connection      fyne.CanvasObject
	menu            fyne.CanvasObject

	dongle, peer, battery, mode    *widget.Label
	codec, activity, serial, ready *widget.Label
	dongleFirmware                 *widget.Label

	reconnect, disconnect, pair, pairPhone                 *widget.Button
	menuReconnect, menuDisconnect, menuPair, menuPairPhone *menuItem

	policy *policyBox

	modelName, firmware, airpodsSerial *widget.Label
	revealSerial                       *chip
	serialValue                        string
	info                               dongle.Metadata
}

func newDeviceTab(w *Window) *deviceTab {
	t := &deviceTab{w: w}
	t.dongle, t.serial, t.peer, t.ready = newValue(""), newValue(""), newValue(""), newValue("")
	t.dongleFirmware = newValue(dashText)
	t.battery, t.mode, t.codec, t.activity = newValue(""), newValue(""), newValue(""), newValue("")
	t.modelName, t.firmware, t.airpodsSerial = newValue(""), newValue(""), newValue("")
	t.revealSerial = newChip(i18n.T("device.show"), func(bool) { t.showSerial() })
	w.bind(func() {
		t.revealSerial.Label = i18n.T("device.show")
		t.revealSerial.Refresh()
	})
	title, head := captionText("")
	w.bind(func() { title.SetText(i18n.T("group.About")) })
	// The compact value rows are made for a section's well; in a card they need air.
	aboutRow := func(key string, value fyne.CanvasObject) fyne.CanvasObject {
		return container.New(insetLayout{top: 10, bottom: 10}, w.valueRow(key, value))
	}
	t.about = container.New(tightLayout{}, head, card(newStack(
		aboutRow("device.model", t.modelName),
		aboutRow("device.version", t.firmware),
		aboutRow("device.serial", container.NewBorder(nil, nil, nil, t.revealSerial, t.airpodsSerial)),
	)), gap(sectionGap))
	t.status = newStack(
		w.valueRow("device.dongle", t.dongle),
		w.valueRow("device.dongleSerial", t.serial),
		w.valueRow("device.dongleFirmware", t.dongleFirmware),
		w.valueRow("device.airpods", t.peer),
		w.valueRow("device.controls", t.ready),
		w.valueRow("device.batteries", t.battery),
		w.valueRow("device.noiseControl", t.mode),
		w.valueRow("device.codec", t.codec),
		w.valueRow("device.inUse", t.activity),
	)
	t.showInfo(dongle.Metadata{})
	t.reconnect = w.button("device.reconnect", t.doReconnect)
	t.disconnect = widget.NewButton("", t.leaveTapped)
	t.pairPhone = w.button("device.pairPhone", t.doPairPhone)
	t.pair = w.button("device.pair", t.requestPair)
	t.leaveRow = newOptionRow("", "", t.disconnect)
	t.leave = container.New(tightLayout{}, card(t.leaveRow), gap(sectionGap))
	t.policy = newPolicyBox(w)
	rows := newStack(
		w.optionRow("tray.reconnect", "device.reconnectHint", t.reconnect),
		container.New(tightLayout{}, w.selectRow("policy.title", "policy.hint", t.policy.choice), tight(t.policy.statusBox),
			buttonRight(t.policy.read)),
		w.optionRow("device.pairPhoneRow", "device.pairPhoneHint", t.pairPhone),
		w.optionRow("device.pairRow", "device.pairHint", t.pair),
		w.optionRow("device.readAgain", "", t.w.settings.rereadBt),
	)
	t.connection = rows
	t.menuReconnect = w.menuItem("tray.reconnect", t.doReconnect)
	t.menuDisconnect = w.menuItem("tray.disconnect", t.doDisconnect)
	t.menuPairPhone = w.menuItem("device.pairPhoneRow", t.doPairPhone)
	t.menuPair = w.menuItem("device.pairMenu", t.requestPair)
	items := []fyne.CanvasObject{menuCaption(w, "section.connection")}
	if w.opts.Controls != nil && w.opts.Controls.ShowBatteries != nil {
		items = append(items, w.menuItem("tray.batteries", func() {
			w.menu.close()
			w.opts.Controls.ShowBatteries()
		}))
	}
	t.menu = container.NewVBox(append(items, t.menuReconnect, t.menuDisconnect, t.menuPairPhone, t.menuPair, w.settings.rereadItem)...)
	w.texts = append(w.texts, func() { t.showInfo(t.info) })
	return t
}

// ModelText names a model, keeping the number so an unknown one is still useful.
func ModelText(number string) string {
	if number == "" {
		return notReported()
	}
	if profile, ok := model.Find(number); ok {
		return profile.Name + " (" + number + ")"
	}
	return fmt.Sprintf(i18n.T("device.unknownModel"), number)
}

func (t *deviceTab) showInfo(info dongle.Metadata) {
	t.info = info
	t.modelName.SetText(ModelText(info.Model))
	firmware := info.Firmware
	if firmware == "" {
		firmware = notReported()
	}
	t.firmware.SetText(firmware)
	t.serialValue = info.Serial
	t.showSerial()
}

// showSerial keeps the AirPods serial off the screen unless asked for.
func (t *deviceTab) showSerial() {
	switch {
	case t.serialValue == "":
		t.airpodsSerial.SetText(notReported())
		t.revealSerial.Hide()
	case t.revealSerial.Checked:
		t.airpodsSerial.SetText(t.serialValue)
		t.revealSerial.Show()
	default:
		t.airpodsSerial.SetText(i18n.T("device.hidden"))
		t.revealSerial.Show()
	}
}

func setEnabled(button *widget.Button, enabled bool) {
	if enabled {
		button.Enable()
	} else {
		button.Disable()
	}
}

func peerText(st device.State) string {
	switch {
	case !st.Present:
		return i18n.T("value.unknown")
	case !st.Status.Connected:
		return i18n.T("state.disconnect")
	}
	p := st.Status.Peer
	if p == [6]byte{} {
		return i18n.T("state.connected")
	}
	return fmt.Sprintf("%s (%02X:%02X:%02X:%02X:%02X:%02X)", i18n.T("state.connected"), p[0], p[1], p[2], p[3], p[4], p[5])
}

func activityText(status dongle.Status) string {
	var parts []string
	for _, flag := range []struct {
		on   bool
		name string
	}{
		{status.Streaming, i18n.T("activity.streaming")},
		{status.USBPlayback, i18n.T("activity.usb")},
		{status.MicrophoneActive, i18n.T("activity.microphone")},
		{status.PhoneConnected, i18n.T("activity.phone")},
		{status.PhonePairing, i18n.T("activity.pairing")},
	} {
		if flag.on {
			parts = append(parts, flag.name)
		}
	}
	if len(parts) == 0 {
		return i18n.T("activity.nothing")
	}
	return strings.Join(parts, ", ")
}

func (t *deviceTab) update(st device.State) {
	unknown := i18n.T("value.unknown")
	dongleText, serial := i18n.T("state.noDongle"), unknown
	if st.Problem != "" {
		dongleText = i18n.T("device.problem")
	}
	if st.Present {
		dongleText, serial = i18n.T("device.pluggedIn"), st.Serial
	}
	setLabel(t.dongle, dongleText)
	setLabel(t.serial, serial)
	setLabel(t.peer, peerText(st))
	if st.Ready() {
		setLabel(t.ready, i18n.T("device.ready"))
	} else {
		setLabel(t.ready, i18n.T("device.notReady"))
	}
	setLabel(t.battery, tray.BatteryDetail(st))
	mode := tray.ModeName(st)
	if mode == "" {
		mode = unknown
	}
	setLabel(t.mode, mode)
	if st.Connected() {
		setLabel(t.codec, st.Status.CodecName())
	} else {
		setLabel(t.codec, unknown)
	}
	if st.Present {
		setLabel(t.activity, activityText(st.Status))
	} else {
		setLabel(t.activity, unknown)
	}
	t.policy.follow(st)
	canReconnect := st.Present && st.Status.Selected() && st.Status.Has(dongle.CapControl) && !t.w.reconnecting
	canPair := st.Present && st.Status.Has(dongle.CapControl)
	canPairPhone := st.Present && st.Status.Has(dongle.CapPairPhone)
	// Without a working command the action stays enabled: using it is how the user
	// learns what to do instead.
	wired := t.w.opts.Disconnect != nil
	showIf(t.menuDisconnect, wired)
	if shown := wired && st.Present; shown != t.leave.Visible() || st.Present != t.about.Visible() {
		showIf(t.leave, shown)
		showIf(t.about, st.Present)
		t.w.relayout()
	}
	routing := disconnect.Pick(nil, st.Status).Kind() == disconnect.KindRouting
	canDisconnect := !routing || (st.Connected() && t.w.release != disconnect.ReleasePending && !t.w.disconnecting)
	t.showLeave(st, routing, canReconnect)
	for _, action := range []struct {
		on    bool
		items []fyne.Disableable
	}{
		{canReconnect, []fyne.Disableable{t.reconnect, t.menuReconnect}},
		{canPair, []fyne.Disableable{t.pair, t.menuPair}},
		{canPairPhone, []fyne.Disableable{t.pairPhone, t.menuPairPhone}},
		{canDisconnect, []fyne.Disableable{t.menuDisconnect}},
		{canDisconnect || (t.leaveReconnects && !t.w.takeover), []fyne.Disableable{t.disconnect}},
	} {
		for _, item := range action.items {
			enableWidget(item, action.on)
		}
	}
}

// showLeave words the Disconnect card for the state: Disconnect while the AirPods are
// connected, with a note while a release closes the connection, and Reconnect while they
// are away, with a note on whether the dongle will take them back by itself, which depends
// on the reconnect policy when it has been read. While a takeover is under way it says so.
func (t *deviceTab) showLeave(st device.State, routing, canReconnect bool) {
	title, button, hint := "tray.disconnect", "device.disconnect", "device.disconnectHint"
	t.leaveReconnects = routing && st.Present && !st.Status.Connected && (canReconnect || t.w.reconnecting)
	automatic := t.policy.automatic()
	switch {
	case t.leaveReconnects && (t.w.takeover || t.w.reconnecting):
		title, button, hint = "tray.reconnect", "device.reconnect", "device.takingOverHint"
	case t.leaveReconnects && t.w.release == disconnect.Released && automatic:
		title, button, hint = "tray.reconnect", "device.reconnect", "device.releasedAutoHint"
	case t.leaveReconnects && t.w.release == disconnect.Released:
		title, button, hint = "tray.reconnect", "device.reconnect", "device.releasedHint"
	case t.leaveReconnects && automatic:
		title, button, hint = "tray.reconnect", "device.reconnect", "device.awayAutoHint"
	case t.leaveReconnects:
		title, button, hint = "tray.reconnect", "device.reconnect", "device.awayHint"
	case routing && t.w.release != 0:
		hint = "device.closingHint"
	}
	setLabel(t.leaveRow.Title, i18n.T(title))
	if text := i18n.T(hint); t.leaveRow.Hint.Text != text {
		t.leaveRow.SetHint(text)
	}
	if text := i18n.T(button); t.disconnect.Text != text {
		t.disconnect.SetText(text)
	}
}

// leaveTapped runs what the Disconnect card offers.
func (t *deviceTab) leaveTapped() {
	if t.leaveReconnects {
		t.doReconnect()
		return
	}
	t.doDisconnect()
}

// command runs a control command once and reports its result in the window.
func (t *deviceTab) command(name string, job device.Job, sent func()) {
	t.w.say(name + ": " + i18n.T("device.sending"))
	t.w.owner.SubmitCommand(job, func(err error) {
		t.w.do(func() {
			if err != nil {
				t.w.say(name + ": " + device.Explain(err))
				return
			}
			t.w.sayBriefly(name + ": " + i18n.T("device.sent"))
			if sent != nil {
				sent()
			}
		})
	})
}

// doReconnect also ends a release: the dongle takes the reconnect as a new choice.
func (t *deviceTab) doReconnect() {
	t.w.menu.close()
	if !t.w.reconnecting {
		t.w.StartReconnect(nil)
	}
}

func (t *deviceTab) doPairPhone() {
	t.w.menu.close()
	t.command(i18n.T("device.pairPhone"), (*dongle.Client).PairPhone, nil)
}

// doDisconnect hands over to the app, which has the command and its fallback.
func (t *deviceTab) doDisconnect() {
	t.w.menu.close()
	if t.w.opts.Disconnect != nil {
		t.w.opts.Disconnect()
	}
}

func pairBlocked(status dongle.Status) bool {
	return status.USBPlayback || status.MicrophoneActive || status.PhoneConnected
}

func (t *deviceTab) requestPair() {
	t.w.menu.close()
	if pairBlocked(t.w.state.Status) {
		t.w.inform(i18n.T("device.pairRow"), i18n.T("device.pairBlocked"))
		return
	}
	t.w.confirm(i18n.T("device.pairRow"), i18n.T("device.pairConfirm"), i18n.T("device.pairAccept"), func(accepted bool) {
		if accepted {
			t.pairNow()
		}
	})
}

// pairNow sends the pair command once. A busy answer is no proof that nothing changed: the
// dongle can answer busy after it has already dropped the AirPods and erased their pairing,
// so it is never sent again, and the status report shows what happened.
func (t *deviceTab) pairNow() {
	name := i18n.T("device.pairRow")
	t.w.say(name + ": " + i18n.T("device.sending"))
	t.w.owner.SubmitCommand((*dongle.Client).Pair, func(err error) {
		t.w.do(func() {
			switch {
			case err == nil:
				t.w.sayBriefly(name + ": " + i18n.T("device.sent"))
			case device.IsUnconfirmed(err):
				// The status report shows whether the AirPods dropped; nothing is sent again.
				t.w.logf("pair: sent, no answer: %v", err)
				t.w.sayBriefly(name + ": " + i18n.T("device.sent"))
			default:
				t.w.say(name + ": " + device.Explain(err))
			}
		})
	})
}

// DongleFirmware is what the dongle reported about its firmware: Known with a version, or
// Unreported for firmware older than the version report. The zero value is no dongle.
type DongleFirmware struct {
	Present, Known, Unreported bool
	Info                       dongle.FirmwareInfo
}

// SetDongleFirmware shows the dongle's firmware version, and whether a newer one is out.
func (w *Window) SetDongleFirmware(fw DongleFirmware) {
	w.dongleFirmware = fw
	w.showFirmware()
}

// SetFirmwareUpdate takes the firmware update check's latest result.
func (w *Window) SetFirmwareUpdate(r updates.Result) {
	w.firmwareUpdate = r
	w.showFirmware()
}

// firmwareNewer is the newest firmware release when it is newer than the dongle's own.
func (w *Window) firmwareNewer() (updates.Version, bool) {
	fw, r := w.dongleFirmware, w.firmwareUpdate
	running := updates.Version{Major: fw.Info.Major, Minor: fw.Info.Minor, Patch: fw.Info.Patch}
	return r.Latest.Version, fw.Known && r.Known && updates.Newer(r.Latest.Version, running)
}

func (w *Window) showFirmware() {
	if w.device == nil {
		return
	}
	fw := w.dongleFirmware
	text := dashText
	switch {
	case fw.Known:
		text = fw.Info.String()
		if latest, newer := w.firmwareNewer(); newer && w.updatesOn() {
			text += " · " + fmt.Sprintf(i18n.T("update.firmwareShort"), latest.Numbers())
		}
	case fw.Unreported:
		text = i18n.T("device.firmwareUnreported")
	}
	setLabel(w.device.dongleFirmware, text)
	if w.about != nil {
		setLabel(w.about.firmware, text)
		w.about.showUpdate()
	}
}

// updatesOn reports whether the user turned the update check on.
func (w *Window) updatesOn() bool {
	c := w.opts.Controls
	return c != nil && c.UpdatesEnabled != nil && c.UpdatesEnabled()
}

// FirmwareStatus is what the Dongle firmware row shows.
func (w *Window) FirmwareStatus() string {
	if w.device == nil {
		return ""
	}
	return w.device.dongleFirmware.Text
}
