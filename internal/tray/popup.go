package tray

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

const (
	popupWait  = 6 * time.Second
	popupWidth = 330
)

// PopupContent is what the popup shows. The settings window's package draws it, so that
// the popup looks like the top of the page; this package cannot import that one.
type PopupContent interface {
	Object() fyne.CanvasObject
	Update(st device.State, view model.View)
}

// PopupWindow returns what makes the small borderless batteries window, or nil when the
// driver has no such window.
func PopupWindow(app fyne.App) func() fyne.Window {
	drv, ok := app.Driver().(desktop.Driver)
	if !ok {
		return nil
	}
	return func() fyne.Window {
		win := drv.CreateSplashWindow()
		if top, ok := win.(desktop.Window); ok {
			top.RequestAlwaysOnTop() // Windows and X11; Wayland lets the compositor decide
		}
		return win
	}
}

// Popup is the Show batteries window. It stays hidden between uses and hides itself
// again after a few seconds. All methods must run on the Fyne main thread.
type Popup struct {
	create func() fyne.Window
	do     func(func())
	wait   time.Duration
	win    fyne.Window
	body   PopupContent
	height float32
	timer  *time.Timer
	shown  uint64
	// Content makes what the popup shows, given what closes it; without one it is a
	// plain column of text.
	Content func(close func()) PopupContent
	// name and rows are the same reading as text, whatever draws it.
	name, rows string
	open       bool
}

// plainContent is the popup without the app's look: the name, the rows and a button.
type plainContent struct {
	name, rows *widget.Label
	button     *widget.Button
	box        fyne.CanvasObject
}

func newPlainContent(close func()) *plainContent {
	c := &plainContent{
		name:   widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		rows:   widget.NewLabel(""),
		button: widget.NewButton(i18n.T("popup.close"), close),
	}
	c.box = container.NewPadded(container.NewVBox(c.name, c.rows, c.button))
	return c
}

func (c *plainContent) Object() fyne.CanvasObject { return c.box }

func (c *plainContent) Update(st device.State, view model.View) {
	c.name.SetText(DeviceName(view))
	c.rows.SetText(Rows(st, view))
	c.button.SetText(i18n.T("popup.close"))
}

// NewPopup makes a popup; do runs a function on the main thread.
func NewPopup(create func() fyne.Window, do func(func())) *Popup {
	return &Popup{create: create, do: do, wait: popupWait}
}

// Rows are the popup's lines: one per battery, then the mode.
func Rows(st device.State, view model.View) string {
	text := ""
	if view.Headphone() {
		for _, component := range []int{BatteryLeft, BatteryRight} {
			if r := ReadBattery(st, component); r.Known {
				text = spelled(i18n.T("battery.battery"), r)
				break
			}
		}
		if text == "" {
			text = spelled(i18n.T("battery.battery"), Reading{})
		}
	} else {
		text = spelled(i18n.T("battery.left"), ReadBattery(st, BatteryLeft)) + "\n" +
			spelled(i18n.T("battery.right"), ReadBattery(st, BatteryRight)) + "\n" +
			spelled(i18n.T("battery.case"), ReadBattery(st, BatteryCase))
	}
	if mode := ModeName(st); mode != "" {
		text += "\n" + mode
	}
	return text
}

func (p *Popup) build() {
	p.win = p.create()
	p.win.SetTitle("AirPods")
	if p.Content != nil {
		p.body = p.Content(p.Hide)
	} else {
		p.body = newPlainContent(p.Hide)
	}
	p.win.SetContent(p.body.Object())
}

// fit makes the window as tall as what it shows now: the content is filled after the
// window is made, and a mode line or one column instead of three changes its height.
func (p *Popup) fit() {
	if height := p.win.Content().MinSize().Height; height != p.height {
		p.height = height
		p.win.Resize(fyne.NewSize(popupWidth, height))
	}
}

// Show opens the popup, or keeps it open for longer.
func (p *Popup) Show(st device.State, view model.View) {
	if p.win == nil {
		p.build()
	}
	p.open = true
	p.Update(st, view)
	p.win.Show()
	p.shown++
	shown := p.shown
	if p.timer != nil {
		p.timer.Stop()
	}
	p.timer = time.AfterFunc(p.wait, func() {
		p.do(func() {
			if shown == p.shown {
				p.Hide()
			}
		})
	})
}

// Update refreshes the open popup.
func (p *Popup) Update(st device.State, view model.View) {
	if !p.open {
		return
	}
	p.name, p.rows = DeviceName(view), Rows(st, view)
	p.body.Update(st, view)
	p.fit()
}

// Hide closes the popup until the next Show.
func (p *Popup) Hide() {
	if p.timer != nil {
		p.timer.Stop()
	}
	if p.open {
		p.open = false
		p.win.Hide()
	}
}

// Visible reports whether the popup is open.
func (p *Popup) Visible() bool { return p.open }

// Text returns what the popup shows, for tests.
func (p *Popup) Text() (name, rows string) {
	if p.win == nil {
		return "", ""
	}
	return p.name, p.rows
}
