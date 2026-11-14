package tray

import (
	"fyne.io/fyne/v2"

	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/icon"
)

// Host is the part of the desktop app the tray needs; tests substitute a fake.
type Host interface {
	SetSystemTrayMenu(*fyne.Menu)
	SetTrayIcon(icon.Variant)
	SetTrayTooltip(string)
}

// Actions are the menu's handlers. They run on the Fyne main thread and must not block.
type Actions struct {
	SetMode       func(word string)
	ShowBatteries func()
	Disconnect    func()
	Reconnect     func()
	OpenSettings  func()
	Quit          func()
}

// Tray owns the menu items. All methods must run on the Fyne main thread.
type Tray struct {
	host       Host
	menu       *fyne.Menu
	header     *fyne.MenuItem
	battery    *fyne.MenuItem
	modes      [4]*fyne.MenuItem
	batteries  *fyne.MenuItem
	disconnect *fyne.MenuItem
	reconnect  *fyne.MenuItem
	quit       *fyne.MenuItem
	separator  *fyne.MenuItem
	open       *fyne.MenuItem
	state      device.State
	view       model.View
	last       Model
}

// New builds the menu in its "no dongle" state and installs it.
func New(host Host, actions Actions) *Tray {
	t := &Tray{host: host}
	t.header = fyne.NewMenuItem("", nil)
	t.battery = fyne.NewMenuItem("", nil)
	items := []*fyne.MenuItem{t.header, t.battery}
	for i, mode := range Modes {
		word := mode.Word
		t.modes[i] = fyne.NewMenuItem(mode.Label(), func() { actions.SetMode(word) })
		items = append(items, t.modes[i])
	}
	t.batteries = fyne.NewMenuItem("", actions.ShowBatteries)
	t.disconnect = fyne.NewMenuItem("", actions.Disconnect)
	t.reconnect = fyne.NewMenuItem("", actions.Reconnect)
	quit := fyne.NewMenuItem("", actions.Quit)
	quit.IsQuit = true
	t.separator = fyne.NewMenuItemSeparator()
	t.open = fyne.NewMenuItem("", actions.OpenSettings)
	items = append(items, t.separator, t.batteries, t.disconnect, t.reconnect, t.open, quit)
	t.quit = quit
	t.menu = fyne.NewMenu("AirPods", items...)
	t.apply(BuildModel(device.State{}, model.View{}))
	host.SetSystemTrayMenu(t.menu)
	t.Reapply()
	return t
}

// Menu returns the installed menu.
func (t *Tray) Menu() *fyne.Menu { return t.menu }

func setRow(item *fyne.MenuItem, row Row) {
	item.Label = row.Label
	item.Disabled = row.Disabled
	item.Checked = row.Checked
}

func (t *Tray) apply(model Model) {
	setRow(t.header, model.Header)
	setRow(t.battery, model.Battery)
	for i := range t.modes {
		setRow(t.modes[i], model.Modes[i])
	}
	setRow(t.batteries, model.Batteries)
	setRow(t.disconnect, model.Disconnect)
	setRow(t.reconnect, model.Reconnect)
	t.open.Label, t.quit.Label = model.Settings, model.Quit
	t.last = model
	items := []*fyne.MenuItem{t.header, t.battery}
	for i := range t.modes {
		if !model.Modes[i].Hidden {
			items = append(items, t.modes[i])
		}
	}
	t.menu.Items = append(items, t.separator, t.batteries, t.disconnect, t.reconnect, t.open, t.quit)
}

// Update applies a state. The OS menu is only rebuilt when a row actually changed.
func (t *Tray) Update(st device.State) {
	t.state = st
	t.refresh()
}

// SetView applies a new model identity or settings report.
func (t *Tray) SetView(view model.View) {
	t.view = view
	t.refresh()
}

func (t *Tray) refresh() {
	model := BuildModel(t.state, t.view)
	if model == t.last {
		return
	}
	iconChanged := model.Icon != t.last.Icon
	t.apply(model)
	t.menu.Refresh()
	t.host.SetTrayTooltip(model.Tooltip)
	if iconChanged {
		t.host.SetTrayIcon(model.Icon)
	}
}

// Refresh rebuilds the rows, after a change of language.
func (t *Tray) Refresh() { t.refresh() }

// Reapply sends the icon and tooltip again. Windows drops both if they arrive before
// the tray has finished starting.
func (t *Tray) Reapply() {
	t.host.SetTrayIcon(t.last.Icon)
	t.host.SetTrayTooltip(t.last.Tooltip)
}
