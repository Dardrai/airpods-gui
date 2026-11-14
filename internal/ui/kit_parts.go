package ui

import (
	"image"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// overflowButton is the quiet three-dot action at the end of a header.
type overflowButton struct {
	widget.BaseWidget
	OnTapped func()
	hovered  bool
}

const overflowSize float32 = 40

func newOverflowButton(tapped func()) *overflowButton {
	b := &overflowButton{OnTapped: tapped}
	b.ExtendBaseWidget(b)
	return b
}

// Tapped runs the action.
func (b *overflowButton) Tapped(*fyne.PointEvent) {
	if b.OnTapped != nil {
		b.OnTapped()
	}
}

// Cursor marks it clickable.
func (b *overflowButton) Cursor() desktop.Cursor { return desktop.PointerCursor }

// MouseIn shows the hover state.
func (b *overflowButton) MouseIn(*desktop.MouseEvent) { b.hovered = true; b.Refresh() }

// MouseMoved does nothing.
func (b *overflowButton) MouseMoved(*desktop.MouseEvent) {}

// MouseOut clears the hover state.
func (b *overflowButton) MouseOut() { b.hovered = false; b.Refresh() }

// CreateRenderer builds the background and the dots.
func (b *overflowButton) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	bg.CornerRadius = overflowSize / 2
	dots := canvas.NewRaster(func(w, h int) image.Image {
		return iconImage(iconDots, w, h, look(theme.ColorNameForeground))
	})
	r := &overflowRenderer{b: b, bg: bg, dots: dots}
	r.Refresh()
	return r
}

type overflowRenderer struct {
	b    *overflowButton
	bg   *canvas.Rectangle
	dots *canvas.Raster
}

func (r *overflowRenderer) MinSize() fyne.Size { return fyne.NewSquareSize(overflowSize) }

func (r *overflowRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	const icon = 24
	r.dots.Resize(fyne.NewSquareSize(icon))
	r.dots.Move(fyne.NewPos((size.Width-icon)/2, (size.Height-icon)/2))
}

func (r *overflowRenderer) Refresh() {
	r.bg.FillColor = color.Transparent
	if r.b.hovered {
		r.bg.FillColor = look(colorCard)
	}
	r.bg.Refresh()
	r.dots.Refresh()
}

func (r *overflowRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.bg, r.dots} }
func (r *overflowRenderer) Destroy()                     {}

// statusDot is a small coloured dot before a short line of secondary text.
type statusDot struct {
	widget.BaseWidget
	value string
	fill  fyne.ThemeColorName
}

// newStatusDot makes one. An empty fill leaves the dot out.
func newStatusDot(value string, fill fyne.ThemeColorName) *statusDot {
	d := &statusDot{value: value, fill: fill}
	d.ExtendBaseWidget(d)
	return d
}

// Set changes the text and the dot.
func (d *statusDot) Set(value string, fill fyne.ThemeColorName) {
	d.value, d.fill = value, fill
	d.Refresh()
}

// CreateRenderer builds the dot and the text.
func (d *statusDot) CreateRenderer() fyne.WidgetRenderer {
	r := &statusDotRenderer{d: d, dot: canvas.NewCircle(color.Transparent), line: newText(d.value, textStatus, false, colorSecondary, fyne.TextAlignLeading)}
	r.Refresh()
	return r
}

type statusDotRenderer struct {
	d    *statusDot
	dot  *canvas.Circle
	line *text
}

const (
	statusDotSize float32 = 8
	statusDotGap  float32 = 8
)

func (r *statusDotRenderer) lead() float32 {
	if r.d.fill == "" {
		return 0
	}
	return statusDotSize + statusDotGap
}

func (r *statusDotRenderer) MinSize() fyne.Size {
	m := r.line.MinSize()
	return fyne.NewSize(m.Width+r.lead(), m.Height)
}

// Layout centres the pair, so the status can sit under a centred title.
func (r *statusDotRenderer) Layout(size fyne.Size) {
	m := r.line.MinSize()
	x := (size.Width - m.Width - r.lead()) / 2
	r.dot.Resize(fyne.NewSquareSize(statusDotSize))
	r.dot.Move(fyne.NewPos(x, (size.Height-statusDotSize)/2))
	r.line.Resize(m)
	r.line.Move(fyne.NewPos(x+r.lead(), (size.Height-m.Height)/2))
}

func (r *statusDotRenderer) Refresh() {
	r.line.SetText(r.d.value)
	if r.d.fill == "" {
		r.dot.Hide()
	} else {
		r.dot.Show()
		r.dot.FillColor = look(r.d.fill)
	}
	r.Layout(r.d.Size())
	r.dot.Refresh()
	r.line.Refresh()
}

func (r *statusDotRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.dot, r.line}
}
func (r *statusDotRenderer) Destroy() {}

// unknownValue is the dimmed mark for a value nobody has reported.
const unknownValue = "–"

// valueText is a right-aligned reading for a row. Empty shows a dimmed dash.
type valueText struct{ text }

func newValueText(value string) *valueText {
	v := &valueText{text{TextSize: textBody, Fill: colorSecondary, Align: fyne.TextAlignTrailing}}
	v.ExtendBaseWidget(v)
	v.SetValue(value)
	return v
}

// SetValue changes the reading.
func (v *valueText) SetValue(value string) {
	if value == "" || value == unknownValue {
		v.text.Fill = colorDim
		value = unknownValue
	} else {
		v.text.Fill = colorSecondary
	}
	v.text.Text = value
	v.text.Refresh()
}

// optionRow is the common row: title on the left, control on the right, and one optional
// small grey hint below the title.
type optionRow struct {
	holder
	Title   *widget.Label
	Hint    *widget.Label
	Control fyne.CanvasObject
}

func newOptionRow(title, hint string, control fyne.CanvasObject) *optionRow {
	label := widget.NewLabel(title)
	label.Wrapping = fyne.TextWrapWord
	small := smallLabel(hint)
	if hint == "" {
		small.Hide()
	}
	main := tight(label, small)
	r := &optionRow{holder: holder{Container: container.New(rowLayout{}, main, control)}, Title: label, Hint: small, Control: control}
	r.ExtendBaseWidget(r)
	return r
}

// SetHint changes the hint; empty removes it.
func (r *optionRow) SetHint(hint string) {
	r.Hint.SetText(hint)
	if hint == "" {
		r.Hint.Hide()
	} else {
		r.Hint.Show()
	}
	r.Refresh()
}

// selectWidth keeps a dropdown as wide as its widest option, not its current one.
type selectWidth struct{ sel *widget.Select }

func (l selectWidth) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, object := range objects {
		object.Move(fyne.NewPos(0, 0))
		object.Resize(size)
	}
}

func (l selectWidth) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var out fyne.Size
	for _, object := range objects {
		out = out.Max(object.MinSize())
	}
	widest := fyne.MeasureText(l.sel.PlaceHolder, compactSelectText, fyne.TextStyle{}).Width
	for _, option := range l.sel.Options {
		widest = max(widest, fyne.MeasureText(option, compactSelectText, fyne.TextStyle{}).Width)
	}
	const icon, chrome = 20, 26
	out.Width = max(out.Width, widest+icon+chrome)
	return out
}

const compactSelectText float32 = 14

// compactSelect makes a dropdown a small quiet button instead of a form field. Place it
// through newSelectRow or rowLayout so it keeps its small size.
func compactSelect(sel *widget.Select) fyne.CanvasObject {
	tuned := container.NewThemeOverride(sel, tunedTheme{
		sizes: map[fyne.ThemeSizeName]float32{
			theme.SizeNameText:         compactSelectText,
			theme.SizeNameInnerPadding: 6,
			theme.SizeNameInlineIcon:   18,
		},
	})
	return container.New(selectWidth{sel}, tuned)
}

// newSelectRow is a optionRow whose control is a compact dropdown at the right.
func newSelectRow(title, hint string, sel *widget.Select) *optionRow {
	return newOptionRow(title, hint, compactSelect(sel))
}

// styledSlider is the slider as a row places it; Slider draws the page look itself.
func styledSlider(slider *Slider) fyne.CanvasObject { return slider }

// sliderRow is one line of icon tile, slider and value, with an optional title and hint
// above.
type sliderRow struct {
	holder
	Title  *widget.Label
	Hint   *widget.Label
	Slider *Slider
	Value  *valueText
}

const (
	sliderValueWidth float32 = 44
	sliderTile       float32 = 34
	sliderEndPad     float32 = 12.5
)

// newSliderRow makes the row. Pass an icon below zero to leave out the tile.
func newSliderRow(title, hint string, icon int, slider *Slider) *sliderRow {
	r := &sliderRow{Slider: slider, Value: newValueText("")}
	var tile fyne.CanvasObject = newIconTile(icon)
	if icon < 0 {
		tile.Hide()
	}
	line := container.New(sliderLineLayout{}, tile, styledSlider(slider), r.Value)
	objects := []fyne.CanvasObject{}
	if title != "" {
		var main fyne.CanvasObject
		r.Title, r.Hint, main = labelled(title, hint)
		objects = append(objects, container.New(insetLayout{top: 14}, main))
	}
	objects = append(objects, line)
	r.Container = container.New(tightLayout{}, objects...)
	r.ExtendBaseWidget(r)
	return r
}

// SetValue shows the reading at the end of the line; empty shows a dimmed dash.
func (r *sliderRow) SetValue(value string) { r.Value.SetValue(value) }

type sliderLineLayout struct{}

func (sliderLineLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	x := float32(0)
	tile, slider, value := objects[0], objects[1], objects[2]
	if tile.Visible() {
		tile.Resize(fyne.NewSquareSize(sliderTile))
		tile.Move(fyne.NewPos(0, (size.Height-sliderTile)/2))
		x = sliderTile + 8
	}
	if !tile.Visible() {
		x = -sliderEndPad // the slider insets its track; line the track up with the title
	}
	width := max(sliderValueWidth, value.MinSize().Width)
	slider.Move(fyne.NewPos(x, 0))
	slider.Resize(fyne.NewSize(size.Width-x-width-8, size.Height))
	value.Resize(fyne.NewSize(width, size.Height))
	value.Move(fyne.NewPos(size.Width-width, 0))
}

func (sliderLineLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	width := max(sliderValueWidth, objects[2].MinSize().Width) + 8 + objects[1].MinSize().Width
	height := max(objects[1].MinSize().Height, objects[2].MinSize().Height)
	if objects[0].Visible() {
		width += sliderTile + 8
		height = max(height, sliderTile)
	}
	return fyne.NewSize(width, max(height, 34)+2*rowPadV+4)
}

// iconTile is the small quiet square holding an icon, such as the speaker next to volume.
type iconTile struct {
	widget.BaseWidget
	kind int
}

func newIconTile(kind int) *iconTile {
	t := &iconTile{kind: kind}
	t.ExtendBaseWidget(t)
	return t
}

// CreateRenderer builds the tile.
func (t *iconTile) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(look(colorQuiet))
	bg.CornerRadius = 8
	bg.StrokeWidth = 1
	icon := canvas.NewRaster(func(w, h int) image.Image {
		return iconImage(t.kind, w, h, look(theme.ColorNameForeground))
	})
	r := &iconTileRenderer{bg: bg, icon: icon}
	r.Refresh()
	return r
}

type iconTileRenderer struct {
	bg   *canvas.Rectangle
	icon *canvas.Raster
}

func (r *iconTileRenderer) MinSize() fyne.Size { return fyne.NewSquareSize(sliderTile) }
func (r *iconTileRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	const icon = 22
	r.icon.Resize(fyne.NewSquareSize(icon))
	r.icon.Move(fyne.NewPos((size.Width-icon)/2, (size.Height-icon)/2))
}
func (r *iconTileRenderer) Refresh() {
	r.bg.FillColor, r.bg.StrokeColor = look(colorQuiet), look(colorQuietRim)
	r.bg.Refresh()
	r.icon.Refresh()
}
func (r *iconTileRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.bg, r.icon} }
func (r *iconTileRenderer) Destroy()                     {}
