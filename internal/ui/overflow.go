package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

const (
	overflowWidth  float32 = 380
	overflowMargin float32 = 12
)

// overflowPanel is what the header's ... button opens: app settings and connection
// actions, in a panel over the page.
type overflowPanel struct {
	w       *Window
	content *fyne.Container
	scroll  *container.Scroll
	pop     *widget.PopUp
}

func newOverflowPanel(w *Window) *overflowPanel {
	p := &overflowPanel{w: w}
	p.content = container.NewVBox(w.about.settings, gap(8), w.device.menu, gap(8), w.about.info)
	p.scroll = container.NewVScroll(sheetInset(p.content))
	p.pop = widget.NewPopUp(p.scroll, w.win.Canvas())
	// A pop-up counts as visible until it is hidden once; unhidden, the first refresh of
	// an open panel would open it at every start.
	p.pop.Hide()
	return p
}

// open shows the panel under the top right corner, no taller than the window.
func (p *overflowPanel) open() {
	canvas := p.w.win.Canvas().Size()
	inner := p.content.MinSize()
	width := min(overflowWidth, canvas.Width-2*overflowMargin)
	height := min(inner.Height+24+overflowPad, canvas.Height-2*overflowMargin)
	p.pop.Resize(fyne.NewSize(width, height))
	left := max(overflowMargin, (canvas.Width+min(pageMaxWidth, canvas.Width))/2-width-overflowMargin)
	p.pop.ShowAtPosition(fyne.NewPos(left, overflowMargin))
}

// overflowPad is the panel's own padding around its content.
const overflowPad = sheetPadH

func (p *overflowPanel) close() { p.pop.Hide() }
