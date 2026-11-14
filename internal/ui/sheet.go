package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// A sheet is anything that opens over or beside the page: the ... panel, a question, a
// notice and the Show batteries window. They share one fill, one inset and one kind of
// title and button, so that none of them looks like a different program.
const (
	sheetPadH  float32 = 16
	sheetPadV  float32 = 12
	sheetWidth float32 = 440
)

// sheetInset is the padding between a sheet's edge and its content.
func sheetInset(content fyne.CanvasObject) fyne.CanvasObject {
	return container.New(insetLayout{left: sheetPadH, right: sheetPadH, top: sheetPadV, bottom: sheetPadV}, content)
}

// sheetTitle is the bold first line of a sheet.
func sheetTitle(value string) *widget.Label {
	return widget.NewLabelWithStyle(value, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
}

// sheetButton is a sheet's action; the one that carries the sheet's purpose is primary.
func sheetButton(label string, primary bool, tapped func()) *widget.Button {
	button := widget.NewButton(label, tapped)
	if primary {
		button.Importance = widget.HighImportance
	}
	return button
}

// sheet lays a sheet out: title, body, then the actions on the right.
func sheet(title, body fyne.CanvasObject, buttons ...fyne.CanvasObject) fyne.CanvasObject {
	actions := container.NewHBox(append([]fyne.CanvasObject{layout.NewSpacer()}, buttons...)...)
	return container.NewBorder(title, container.NewVBox(gap(8), actions), nil, nil, body)
}

// sheetFill is the sheet colour behind a sheet that is a window of its own; a sheet over
// the page gets the same colour from the pop-up it sits in.
type sheetFill struct {
	widget.BaseWidget
	content fyne.CanvasObject
}

func (s *sheetFill) CreateRenderer() fyne.WidgetRenderer {
	return &sheetFillRenderer{s: s, back: canvas.NewRectangle(look(theme.ColorNameOverlayBackground))}
}

type sheetFillRenderer struct {
	s    *sheetFill
	back *canvas.Rectangle
}

func (r *sheetFillRenderer) Layout(size fyne.Size) {
	r.back.Resize(size)
	r.s.content.Resize(size)
}
func (r *sheetFillRenderer) MinSize() fyne.Size { return r.s.content.MinSize() }
func (r *sheetFillRenderer) Refresh() {
	r.back.FillColor = look(theme.ColorNameOverlayBackground)
	r.back.Refresh()
	r.s.content.Refresh()
}
func (r *sheetFillRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.back, r.s.content}
}
func (r *sheetFillRenderer) Destroy() {}

// WindowSheet is the content of a sheet that is a window of its own, such as Show batteries.
func WindowSheet(title, body fyne.CanvasObject, buttons ...fyne.CanvasObject) fyne.CanvasObject {
	s := &sheetFill{content: sheetInset(sheet(title, body, buttons...))}
	s.ExtendBaseWidget(s)
	return s
}

// showSheet opens a sheet over the page and returns what closes it.
func (w *Window) showSheet(title, text string, buttons func(shut func()) []fyne.CanvasObject) {
	var pop *widget.PopUp
	shut := func() { pop.Hide() }
	width := min(sheetWidth, w.win.Canvas().Size().Width-2*overflowMargin-2*sheetPadH)
	content := sized(width, sheet(sheetTitle(title), dialogBody(text), buttons(shut)...))
	pop = widget.NewModalPopUp(sheetInset(content), w.win.Canvas())
	pop.Show()
}
