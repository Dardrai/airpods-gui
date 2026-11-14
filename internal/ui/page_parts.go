package ui

import (
	"image"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	groupGap  float32 = 14
	chipGap   float32 = 6
	chipTall  float32 = 34
	valueTall float32 = 32
	foldGap   float32 = 14 // between the collapsible rows
)

// setLabel changes a label only when its text changes; a refresh on every status poll
// would redraw the page for nothing.
func setLabel(label *widget.Label, value string) {
	if label.Text != value {
		label.SetText(value)
	}
}

// setDim greys a row title out or back.
func setDim(label *widget.Label, dim bool) {
	want := widget.MediumImportance
	if dim {
		want = widget.LowImportance
	}
	if label.Importance != want {
		label.Importance = want
		label.Refresh()
	}
}

func showIf(object fyne.CanvasObject, on bool) {
	if on == object.Visible() {
		return
	}
	if on {
		object.Show()
	} else {
		object.Hide()
	}
}

// gap is empty vertical space.
func gap(height float32) fyne.CanvasObject {
	space := canvas.NewRectangle(color.Transparent)
	space.SetMinSize(fyne.NewSize(0, height))
	return space
}

// captionText is the kit's caption with its text kept, for titles that change.
func captionText(value string) (*text, fyne.CanvasObject) {
	label := newText(value, 13, true, colorSecondary, fyne.TextAlignLeading)
	return label, container.New(insetLayout{left: dividerInset, top: 4, bottom: 9}, label)
}

// valueRow is a compact read-only row; the value takes what the title leaves and wraps.
func valueRow(title string, value fyne.CanvasObject) fyne.CanvasObject {
	label := widget.NewLabel(title)
	label.Importance = widget.LowImportance
	return container.New(valueLayout{}, label, value)
}

// newValue is the label on the right of a valueRow.
func newValue(initial string) *widget.Label {
	label := widget.NewLabel(initial)
	label.Wrapping = fyne.TextWrapWord
	label.Alignment = fyne.TextAlignTrailing
	return label
}

type valueLayout struct{}

func (valueLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	title := objects[0].MinSize()
	// Labels carry an inner padding; pull them out so their text lines up with the rows.
	pull := theme.InnerPadding()
	objects[0].Resize(fyne.NewSize(title.Width, size.Height))
	objects[0].Move(fyne.NewPos(-pull, 0))
	objects[1].Move(fyne.NewPos(title.Width-pull, 0))
	objects[1].Resize(fyne.NewSize(max(0, size.Width-title.Width+2*pull), size.Height))
}

func (valueLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	title, value := objects[0].MinSize(), objects[1].MinSize()
	return fyne.NewSize(title.Width+value.Width, max(valueTall, title.Height, value.Height))
}

// subCaption titles a run of rows inside a section.
func subCaption(value string) fyne.CanvasObject {
	return container.NewVBox(gap(4), newText(value, 12, true, colorSecondary, fyne.TextAlignLeading))
}

// buttonRight holds a button at its natural width on the right.
func buttonRight(button fyne.CanvasObject) fyne.CanvasObject {
	return container.NewHBox(layout.NewSpacer(), button)
}

// chip is a pill that is on or off; it has the same shape as Switch. With an icon set it
// shows the icon before its label.
type chip struct {
	widget.DisableableWidget
	Label     string
	Checked   bool
	OnChanged func(bool)
	Icon      int // an icon kind, or noIcon
}

const (
	noIcon               = -1
	chipIcon     float32 = 18
	chipIconGap  float32 = 3
	chipIconTall float32 = 56 // a chip with its icon above the label
	chipRadius   float32 = 17
)

func newChip(label string, changed func(bool)) *chip {
	c := &chip{Label: label, OnChanged: changed, Icon: noIcon}
	c.ExtendBaseWidget(c)
	return c
}

// SetChecked changes the state and reports a change.
func (c *chip) SetChecked(on bool) {
	if c.Checked == on {
		return
	}
	c.Checked = on
	c.Refresh()
	if c.OnChanged != nil {
		c.OnChanged(on)
	}
}

// Tapped flips the chip.
func (c *chip) Tapped(*fyne.PointEvent) {
	if !c.Disabled() {
		c.SetChecked(!c.Checked)
	}
}

// Cursor marks it clickable.
func (c *chip) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (c *chip) CreateRenderer() fyne.WidgetRenderer {
	back := canvas.NewRectangle(color.Transparent)
	back.CornerRadius = chipRadius
	label := canvas.NewText(c.Label, look(theme.ColorNameForeground))
	label.TextSize = 13
	r := &chipRenderer{c: c, back: back, label: label}
	r.icon = canvas.NewRaster(func(w, h int) image.Image {
		if c.Icon == noIcon {
			return image.NewNRGBA(image.Rect(0, 0, w, h))
		}
		return iconImage(c.Icon, w, h, r.label.Color)
	})
	r.Refresh()
	return r
}

type chipRenderer struct {
	c     *chip
	back  *canvas.Rectangle
	icon  *canvas.Raster
	label *canvas.Text
}

// Layout centres the label, under the icon when there is one: beside it, the longest
// mode name would no longer fit four chips across the page.
func (r *chipRenderer) Layout(size fyne.Size) {
	r.back.Resize(size)
	text := r.label.MinSize()
	if r.c.Icon == noIcon {
		r.label.Resize(text)
		r.label.Move(fyne.NewPos((size.Width-text.Width)/2, (size.Height-text.Height)/2))
		return
	}
	y := (size.Height - chipIcon - chipIconGap - text.Height) / 2
	r.icon.Resize(fyne.NewSquareSize(chipIcon))
	r.icon.Move(fyne.NewPos((size.Width-chipIcon)/2, y))
	r.label.Resize(text)
	r.label.Move(fyne.NewPos((size.Width-text.Width)/2, y+chipIcon+chipIconGap))
}

func (r *chipRenderer) MinSize() fyne.Size {
	tall := chipTall
	if r.c.Icon != noIcon {
		tall = chipIconTall
	}
	return fyne.NewSize(r.label.MinSize().Width+20, tall)
}

func (r *chipRenderer) Refresh() {
	r.label.Text = r.c.Label
	foreground := look(theme.ColorNameForeground)
	if r.c.Disabled() {
		foreground = look(theme.ColorNameDisabled)
	}
	r.label.Color = foreground
	if r.c.Checked {
		r.back.FillColor, r.back.StrokeWidth = look(colorThumb), 0
	} else {
		r.back.FillColor, r.back.StrokeColor, r.back.StrokeWidth = color.Transparent, look(colorDivider), 1
	}
	showIf(r.icon, r.c.Icon != noIcon)
	r.Layout(r.c.Size())
	r.back.Refresh()
	r.icon.Refresh()
	r.label.Refresh()
}

func (r *chipRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.back, r.icon, r.label}
}
func (r *chipRenderer) Destroy() {}

// chipRowLayout shares the width equally between the visible chips.
type chipRowLayout struct{}

func (chipRowLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	count := float32(0)
	for _, object := range objects {
		if object.Visible() {
			count++
		}
	}
	if count == 0 {
		return
	}
	each := (size.Width - chipGap*(count-1)) / count
	x := float32(0)
	for _, object := range objects {
		if object.Visible() {
			object.Move(fyne.NewPos(x, 0))
			object.Resize(fyne.NewSize(each, size.Height))
			x += each + chipGap
		}
	}
}

func (chipRowLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var widest, count float32
	tall := chipTall
	for _, object := range objects {
		if object.Visible() {
			widest = max(widest, object.MinSize().Width)
			tall = max(tall, object.MinSize().Height)
			count++
		}
	}
	return fyne.NewSize(widest*count+chipGap*max(0, count-1), tall)
}

// dashPill stands in for a switch whose value is unknown. It is the switch's size, dim,
// and still works: a tap chooses a value.
type dashPill struct {
	widget.DisableableWidget
	onTap func()
}

func newDashPill(onTap func()) *dashPill {
	d := &dashPill{onTap: onTap}
	d.ExtendBaseWidget(d)
	return d
}

// Tapped chooses.
func (d *dashPill) Tapped(*fyne.PointEvent) {
	if !d.Disabled() && d.onTap != nil {
		d.onTap()
	}
}

// Cursor marks it clickable.
func (d *dashPill) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (d *dashPill) CreateRenderer() fyne.WidgetRenderer {
	track := canvas.NewRectangle(color.Transparent)
	track.CornerRadius = switchHeight / 2
	track.StrokeWidth = 1
	mark := canvas.NewText(unknownValue, look(colorSecondary))
	mark.Alignment = fyne.TextAlignCenter
	r := &dashRenderer{d: d, track: track, mark: mark}
	r.Refresh()
	return r
}

type dashRenderer struct {
	d     *dashPill
	track *canvas.Rectangle
	mark  *canvas.Text
}

func (r *dashRenderer) Layout(size fyne.Size) {
	top := (size.Height - switchHeight) / 2
	left := size.Width - switchWidth - 3
	r.track.Move(fyne.NewPos(left, top))
	r.track.Resize(fyne.NewSize(switchWidth, switchHeight))
	r.mark.Move(fyne.NewPos(left, top))
	r.mark.Resize(fyne.NewSize(switchWidth, switchHeight))
}

func (r *dashRenderer) MinSize() fyne.Size { return fyne.NewSize(switchWidth+6, switchTarget) }

func (r *dashRenderer) Refresh() {
	tint := look(colorSecondary)
	if r.d.Disabled() {
		tint = look(colorDim)
	}
	r.track.StrokeColor = look(colorDivider)
	r.mark.Color = tint
	r.track.Refresh()
	r.mark.Refresh()
}

func (r *dashRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.track, r.mark} }
func (r *dashRenderer) Destroy()                     {}

// menuItem is a flat row of the ... panel.
type menuItem struct {
	widget.DisableableWidget
	Label    string
	OnTapped func()
	hovered  bool
}

const menuItemTall float32 = 38

func newMenuItem(label string, tapped func()) *menuItem {
	m := &menuItem{Label: label, OnTapped: tapped}
	m.ExtendBaseWidget(m)
	return m
}

// Tapped runs the action.
func (m *menuItem) Tapped(*fyne.PointEvent) {
	if !m.Disabled() && m.OnTapped != nil {
		m.OnTapped()
	}
}

// Cursor marks it clickable.
func (m *menuItem) Cursor() desktop.Cursor {
	if m.Disabled() {
		return desktop.DefaultCursor
	}
	return desktop.PointerCursor
}

// MouseIn shows the hover state.
func (m *menuItem) MouseIn(*desktop.MouseEvent) { m.hovered = true; m.Refresh() }

// MouseMoved does nothing.
func (m *menuItem) MouseMoved(*desktop.MouseEvent) {}

// MouseOut clears the hover state.
func (m *menuItem) MouseOut() { m.hovered = false; m.Refresh() }

func (m *menuItem) CreateRenderer() fyne.WidgetRenderer {
	back := canvas.NewRectangle(color.Transparent)
	back.CornerRadius = 8
	label := canvas.NewText(m.Label, look(theme.ColorNameForeground))
	label.TextSize = textBody
	r := &menuItemRenderer{m: m, back: back, label: label}
	r.Refresh()
	return r
}

type menuItemRenderer struct {
	m     *menuItem
	back  *canvas.Rectangle
	label *canvas.Text
}

func (r *menuItemRenderer) Layout(size fyne.Size) {
	r.back.Move(fyne.NewPos(-8, 0))
	r.back.Resize(fyne.NewSize(size.Width+16, size.Height))
	r.label.Move(fyne.NewPos(0, 0))
	r.label.Resize(size)
}

func (r *menuItemRenderer) MinSize() fyne.Size {
	return fyne.NewSize(r.label.MinSize().Width, menuItemTall)
}

func (r *menuItemRenderer) Refresh() {
	r.label.Text = r.m.Label
	r.label.Color = look(theme.ColorNameForeground)
	if r.m.Disabled() {
		r.label.Color = look(colorDim)
	}
	r.back.FillColor = color.Transparent
	if r.m.hovered && !r.m.Disabled() {
		r.back.FillColor = look(colorThumb)
	}
	r.back.Refresh()
	r.label.Refresh()
}

func (r *menuItemRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.back, r.label} }
func (r *menuItemRenderer) Destroy()                     {}
