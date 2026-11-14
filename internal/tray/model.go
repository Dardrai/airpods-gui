// Package tray builds the system tray menu and icon from device state.
package tray

import (
	"github.com/Dardrai/airpods-gui/core/disconnect"
	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
	"github.com/Dardrai/airpods-gui/internal/icon"
)

// Row is one menu row's appearance.
type Row struct {
	Label    string
	Disabled bool
	Checked  bool
	Hidden   bool
}

// Mode is one Noise Control row.
type Mode struct {
	Word  string // the value word of the "mode" setting
	Value byte   // the status report's mode byte
	Key   string // the text's i18n key
}

// Label is the mode's name in the current language.
func (m Mode) Label() string { return i18n.T(m.Key) }

// Modes lists the Noise Control rows in menu order.
var Modes = []Mode{
	{"off", 1, "mode.off"},
	{"anc", 2, "mode.anc"},
	{"transparency", 3, "mode.transparency"},
	{"adaptive", 4, "mode.adaptive"},
}

// Model is the complete appearance of the menu for one state. It is comparable,
// so unchanged states do not rebuild the OS menu.
type Model struct {
	Header     Row
	Battery    Row
	Modes      [4]Row
	Batteries  Row
	Disconnect Row
	Reconnect  Row
	Settings   string
	Quit       string
	Tooltip    string
	Icon       icon.Variant
}

const (
	defaultName = "AirPods"
	// tooltipLimit is what a Windows notification icon keeps of its tooltip.
	tooltipLimit = 127
)

// DeviceName is the marketing name of the connected model, once it is known.
func DeviceName(view model.View) string {
	if profile, ok := model.Find(view.Model); ok {
		return profile.Name
	}
	return defaultName
}

// Headline names the device and its connection state.
func Headline(st device.State, view model.View) string {
	text := headline(st, view)
	if st.Several && st.Present && st.Serial != "" {
		text += " · " + st.Serial // Which dongle this is, when there is more than one.
	}
	return text
}

func headline(st device.State, view model.View) string {
	switch {
	case st.ChooseDongle:
		return defaultName + " · " + i18n.T("state.chooseDongle")
	case st.Problem != "":
		return defaultName + " · " + i18n.T("state.problem")
	case !st.Present:
		return defaultName + " · " + i18n.T("state.noDongle")
	case st.Status.Connected:
		return DeviceName(view) + " · " + i18n.T("state.connected")
	}
	return DeviceName(view) + " · " + i18n.T("state.disconnect")
}

func clip(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit-1]) + "…"
}

// ModeName names the current listening mode, or "" when there is none to show.
func ModeName(st device.State) string {
	if st.Connected() {
		for _, mode := range Modes {
			if mode.Value == st.Status.ANC {
				return mode.Label()
			}
		}
	}
	return ""
}

// modeHidden leaves out rows the model has no use for.
func modeHidden(view model.View, mode Mode) bool {
	if !view.Identified() {
		return false
	}
	switch {
	case !view.Listening():
		return true
	case mode.Value == 1:
		return !view.OffAvailable()
	case mode.Value == 4:
		return !view.Adaptive()
	}
	return false
}

func modeAvailable(st device.State, mode Mode) bool {
	if !st.Ready() {
		return false
	}
	if st.Status.Has(dongle.CapTypedRPC) {
		return true
	}
	return st.Status.Has(dongle.CapListening) &&
		(mode.Value == dongle.ListeningANC || mode.Value == dongle.ListeningTransparency)
}

// IconFor picks the tray icon for a state.
func IconFor(st device.State) icon.Variant {
	switch {
	case !st.Present:
		return icon.Variant{Level: icon.NoBadge, NoDongle: true}
	case !st.Status.Connected:
		return icon.Variant{Level: icon.NoBadge, Dim: true}
	}
	if level, ok := LowestEarbud(st); ok {
		return icon.Variant{Level: level}
	}
	return icon.Variant{Level: icon.NoBadge}
}

// disconnectRow: with the routing handover the row needs a connection to act on; without
// it the row stays enabled, because clicking it is how the user learns what to do instead.
func disconnectRow(st device.State) Row {
	supported := disconnect.Pick(nil, st.Status).Kind() == disconnect.KindRouting
	return Row{Label: i18n.T("tray.disconnect"), Disabled: supported && !st.Connected()}
}

// BuildModel derives the menu appearance from a state. Reconnect stays available
// while the AirPods are away since that is when it is useful; it needs a dongle.
func BuildModel(st device.State, view model.View) Model {
	model := Model{
		Header:     Row{Label: Headline(st, view), Disabled: true},
		Battery:    Row{Label: batteryRow(st, view), Disabled: true},
		Batteries:  Row{Label: i18n.T("tray.batteries"), Disabled: !HasReading(st)},
		Disconnect: disconnectRow(st),
		Reconnect:  Row{Label: i18n.T("tray.reconnect"), Disabled: !st.Present || !st.Status.Selected() || !st.Status.Has(dongle.CapControl)},
		Settings:   i18n.T("tray.settings"),
		Quit:       i18n.T("tray.quit"),
		Icon:       IconFor(st),
	}
	for i, mode := range Modes {
		model.Modes[i] = Row{
			Label:    mode.Label(),
			Disabled: !modeAvailable(st, mode),
			Checked:  st.Connected() && st.Status.ANC == mode.Value,
			Hidden:   modeHidden(view, mode),
		}
	}
	model.Tooltip = model.Header.Label
	if st.Connected() {
		model.Tooltip = DeviceName(view) + ": " + model.Battery.Label
		if name := ModeName(st); name != "" {
			model.Tooltip += " · " + name
		}
	}
	model.Tooltip = clip(model.Tooltip, tooltipLimit)
	return model
}
