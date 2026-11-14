package ui

import (
	"image"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type imageNRGBA = image.Image

// Switch is the page's toggle: green when on, outlined grey when off.
type Switch struct {
	widget.DisableableWidget
	Checked   bool
	OnChanged func(bool)
	hovered   bool
	focused   bool
}

const (
	switchWidth  float32 = 42
	switchHeight float32 = 22
	switchKnob   float32 = 14
	switchTarget float32 = 32
)

// NewSwitch makes a switch.
func NewSwitch(changed func(bool)) *Switch {
	s := &Switch{OnChanged: changed}
	s.ExtendBaseWidget(s)
	return s
}

// SetChecked changes the state and reports a change, as widget.Check does.
func (s *Switch) SetChecked(on bool) {
	if s.Checked == on {
		return
	}
	s.Checked = on
	s.Refresh()
	if s.OnChanged != nil {
		s.OnChanged(on)
	}
}

// Tapped toggles the switch.
func (s *Switch) Tapped(*fyne.PointEvent) {
	if !s.Disabled() {
		s.SetChecked(!s.Checked)
	}
}

// Cursor marks it clickable.
func (s *Switch) Cursor() desktop.Cursor {
	if s.Disabled() {
		return desktop.DefaultCursor
	}
	return desktop.PointerCursor
}

// MouseIn, MouseMoved and MouseOut track the hover state.
func (s *Switch) MouseIn(*desktop.MouseEvent) { s.hovered = true; s.Refresh() }

// MouseMoved does nothing.
func (s *Switch) MouseMoved(*desktop.MouseEvent) {}

// MouseOut clears the hover state.
func (s *Switch) MouseOut() { s.hovered = false; s.Refresh() }

// FocusGained shows the focus ring.
func (s *Switch) FocusGained() { s.focused = true; s.Refresh() }

// FocusLost hides the focus ring.
func (s *Switch) FocusLost() { s.focused = false; s.Refresh() }

// TypedRune toggles on space.
func (s *Switch) TypedRune(r rune) {
	if r == ' ' {
		s.Tapped(nil)
	}
}

// TypedKey does nothing; space arrives as a rune.
func (s *Switch) TypedKey(*fyne.KeyEvent) {}

// CreateRenderer builds the track and knob.
func (s *Switch) CreateRenderer() fyne.WidgetRenderer {
	track := canvas.NewRectangle(color.Transparent)
	track.CornerRadius = switchHeight / 2
	ring := canvas.NewRectangle(color.Transparent)
	ring.CornerRadius = switchHeight/2 + 3
	knob := canvas.NewCircle(color.White)
	r := &switchRenderer{s: s, track: track, knob: knob, ring: ring}
	r.Refresh()
	return r
}

type switchRenderer struct {
	s           *Switch
	track, ring *canvas.Rectangle
	knob        *canvas.Circle
}

func (r *switchRenderer) MinSize() fyne.Size { return fyne.NewSize(switchWidth, switchTarget) }

func (r *switchRenderer) Layout(size fyne.Size) {
	top := (size.Height - switchHeight) / 2
	left := size.Width - switchWidth
	r.track.Move(fyne.NewPos(left, top))
	r.track.Resize(fyne.NewSize(switchWidth, switchHeight))
	r.ring.Move(fyne.NewPos(left-3, top-3))
	r.ring.Resize(fyne.NewSize(switchWidth+6, switchHeight+6))
	margin := (switchHeight - switchKnob) / 2
	x := left + margin
	if r.s.Checked {
		x = left + switchWidth - switchKnob - margin
	}
	r.knob.Move(fyne.NewPos(x, top+margin))
	r.knob.Resize(fyne.NewSquareSize(switchKnob))
}

func (r *switchRenderer) Refresh() {
	switch {
	case r.s.Checked && r.s.Disabled():
		r.track.FillColor, r.track.StrokeWidth = fade(look(colorGreen), 0.38), 0
		r.knob.FillColor = fade(look(colorKnob), 0.75)
	case r.s.Checked:
		r.track.FillColor, r.track.StrokeWidth = look(colorGreen), 0
		r.knob.FillColor = look(colorKnob)
	case r.s.Disabled():
		r.track.FillColor, r.track.StrokeColor, r.track.StrokeWidth = color.Transparent, look(colorDim), 1
		r.knob.FillColor = look(colorDim)
	default:
		r.track.FillColor, r.track.StrokeColor, r.track.StrokeWidth = color.Transparent, look(colorSwitchOff), 1
		r.knob.FillColor = look(colorKnobOff)
		if r.s.hovered {
			r.track.FillColor = look(theme.ColorNameHover)
		}
	}
	r.ring.FillColor = color.Transparent
	r.ring.StrokeWidth = 0
	if r.s.focused && !r.s.Disabled() {
		r.ring.StrokeColor, r.ring.StrokeWidth = look(theme.ColorNameFocus), 2
	}
	r.Layout(r.s.Size())
	r.track.Refresh()
	r.ring.Refresh()
	r.knob.Refresh()
}

func (r *switchRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.ring, r.track, r.knob}
}
func (r *switchRenderer) Destroy() {}

// Segmented is the listening mode control: a pill rail with an icon per mode, a raised
// capsule on the selected one and labels below. It has the same shape as widget.Select.
// A selected iconAdaptive mode opens up to three units wide, with a strength lane under
// the capsule, when the control is wide enough to keep the labels apart.
type Segmented struct {
	widget.DisableableWidget
	Options   []string
	Selected  string
	Icons     map[string]int
	OnChanged func(string)
	// OnStrength, when set, lets a drag that ends in the Adaptive lane set the strength. It
	// reports the 0-100 position on release.
	OnStrength func(int)
	strength   int
	focused    bool
	// dragging holds the capsule under the pointer, centred on dragAt, until release. The
	// selection, and so the segments, stay as they were for the whole drag.
	dragging bool
	dragAt   float32
}

// NewSegmented makes the control.
func NewSegmented(options []string, icons map[string]int, changed func(string)) *Segmented {
	s := &Segmented{Options: options, Icons: icons, OnChanged: changed, strength: -1}
	s.ExtendBaseWidget(s)
	return s
}

// SetSelected changes the selection and reports a change, as widget.Select does.
func (s *Segmented) SetSelected(option string) {
	if s.Selected == option {
		return
	}
	s.Selected = option
	s.Refresh()
	if s.OnChanged != nil {
		s.OnChanged(option)
	}
}

// ClearSelected shows no selection, without reporting a change.
func (s *Segmented) ClearSelected() {
	s.Selected = ""
	s.dragging = false
	s.Refresh()
}

// SetOptions changes the modes on offer.
func (s *Segmented) SetOptions(options []string) {
	s.Options = options
	s.Refresh()
}

// SetStrength places the capsule in the Adaptive lane, without reporting a change. Below
// zero is unknown and centres it.
func (s *Segmented) SetStrength(value int) {
	s.strength = min(value, 100)
	s.Refresh()
}

type segment struct{ x, width float32 }

const (
	segmentRail   float32 = 62
	segmentGap    float32 = 4
	segmentLabel  float32 = 28
	segmentIcon   float32 = 22
	segmentMarker float32 = 16
	segmentShadow float32 = 8
	// segmentLabelGap is the least space between two labels under the rail.
	segmentLabelGap float32 = 6
)

func (s *Segmented) selectedIndex() int {
	for i, option := range s.Options {
		if option == s.Selected {
			return i
		}
	}
	return -1
}

func (s *Segmented) widestLabel() float32 {
	widest := float32(64)
	for _, option := range s.Options {
		widest = max(widest, fyne.MeasureText(option, textLabel, fyne.TextStyle{}).Width+8)
	}
	return widest
}

// lane reports the index of the segment opened as the Adaptive lane, or -1.
func (s *Segmented) lane(width float32) int {
	selected := s.selectedIndex()
	if selected < 0 || len(s.Options) < 2 {
		return -1
	}
	kind, ok := s.Icons[s.Options[selected]]
	if !ok || kind != iconAdaptive {
		return -1
	}
	if !s.laneFits(width, selected) {
		return -1
	}
	return selected
}

// laneCapable reports whether the Adaptive mode, once selected, gets its lane at this
// width. It does not depend on what is selected now.
func (s *Segmented) laneCapable(width float32) bool {
	for i, option := range s.Options {
		if kind, ok := s.Icons[option]; ok && kind == iconAdaptive {
			return len(s.Options) >= 2 && s.laneFits(width, i)
		}
	}
	return false
}

// laneFits reports whether the labels still stand apart, and inside the control, once the
// selected segment has opened to three units. Only neighbours can collide, so a long label
// beside the wide lane does not keep the lane shut.
func (s *Segmented) laneFits(width float32, selected int) bool {
	unit := (width - 2*segmentGap) / float32(len(s.Options)+2)
	x, edge := segmentGap, float32(0)
	for i, option := range s.Options {
		span := unit
		if i == selected {
			span = 3 * unit
		}
		half := fyne.MeasureText(option, textLabel, fyne.TextStyle{}).Width / 2
		center := x + span/2
		if center-half < edge || center+half > width {
			return false
		}
		edge = center + half + segmentLabelGap
		x += span
	}
	return true
}

// segments is the one geometry for drawing and hit testing.
func (s *Segmented) segments(width float32) []segment {
	count := len(s.Options)
	if count == 0 {
		return nil
	}
	lane := s.lane(width)
	units := float32(count)
	if lane >= 0 {
		units += 2
	}
	unit := (width - 2*segmentGap) / units
	out := make([]segment, count)
	x := segmentGap
	for i := range out {
		w := unit
		if i == lane {
			w = 3 * unit
		}
		out[i] = segment{x, w}
		x += w
	}
	return out
}

// thumbSpan is where the capsule sits, given the current segments.
func (s *Segmented) thumbSpan(width float32) (segment, bool) {
	selected := s.selectedIndex()
	segments := s.segments(width)
	if selected < 0 || selected >= len(segments) {
		return segment{}, false
	}
	span := segments[selected]
	open := s.lane(width) == selected
	if s.dragging && !s.Disabled() {
		// Every segment but the lane is one unit wide, and the capsule is always one unit.
		unit := span.width
		if open {
			unit = span.width / 3
		}
		x := max(segmentGap, min(width-segmentGap-unit, s.dragAt-unit/2))
		return segment{x, unit}, true
	}
	if !open {
		return span, true
	}
	unit := span.width / 3
	position := float32(50)
	if s.strength >= 0 {
		position = float32(s.strength)
	}
	return segment{span.x + (span.width-unit)*position/100, unit}, true
}

// segmentAt is the index of the segment at x; past either end it is the nearest one.
func (s *Segmented) segmentAt(width, x float32) int {
	for i, span := range s.segments(width) {
		if x < span.x+span.width {
			return i
		}
	}
	return len(s.Options) - 1
}

// Tapped selects the segment under the pointer.
func (s *Segmented) Tapped(event *fyne.PointEvent) {
	if s.Disabled() || len(s.Options) == 0 {
		return
	}
	s.SetSelected(s.Options[s.segmentAt(s.Size().Width, event.Position.X)])
}

// Dragged slides the capsule along the whole rail with the pointer. Nothing is selected or
// reported until the release.
func (s *Segmented) Dragged(event *fyne.DragEvent) {
	if s.Disabled() || s.selectedIndex() < 0 {
		if s.dragging {
			s.dragging = false
			s.Refresh()
		}
		return
	}
	s.dragging, s.dragAt = true, event.Position.X
	s.Refresh()
}

// DragEnd drops the capsule on the segment under its centre. Over another mode it selects
// that mode, as a tap does, and leaves the strength alone. Over the open Adaptive lane it
// reports the position there as the strength. Anywhere else it goes back where it was.
func (s *Segmented) DragEnd() {
	if !s.dragging {
		return
	}
	width := s.Size().Width
	thumb, ok := s.thumbSpan(width)
	s.dragging = false
	selected := s.selectedIndex()
	if s.Disabled() || !ok || selected < 0 {
		s.Refresh()
		return
	}
	target := s.segmentAt(width, thumb.x+thumb.width/2)
	if target != selected {
		s.SetSelected(s.Options[target])
		return
	}
	if s.OnStrength != nil && s.lane(width) == selected {
		span := s.segments(width)[selected]
		s.strength = max(0, min(100, int((thumb.x-span.x)/(span.width-thumb.width)*100+0.5)))
		s.Refresh()
		s.OnStrength(s.strength)
		return
	}
	s.Refresh()
}

// Cursor marks it clickable.
func (s *Segmented) Cursor() desktop.Cursor {
	if s.Disabled() {
		return desktop.DefaultCursor
	}
	return desktop.PointerCursor
}

// FocusGained shows the focus ring.
func (s *Segmented) FocusGained() { s.focused = true; s.Refresh() }

// FocusLost hides the focus ring.
func (s *Segmented) FocusLost() { s.focused = false; s.Refresh() }

// TypedRune does nothing.
func (s *Segmented) TypedRune(rune) {}

// TypedKey moves the selection with the arrow keys.
func (s *Segmented) TypedKey(event *fyne.KeyEvent) {
	if s.Disabled() || len(s.Options) == 0 {
		return
	}
	index := s.selectedIndex()
	switch event.Name {
	case fyne.KeyLeft:
		index = max(index-1, 0)
	case fyne.KeyRight:
		index = min(index+1, len(s.Options)-1)
	default:
		return
	}
	s.SetSelected(s.Options[index])
}

// CreateRenderer builds the rail.
func (s *Segmented) CreateRenderer() fyne.WidgetRenderer {
	track := canvas.NewRectangle(look(colorRail))
	track.CornerRadius = segmentRail / 2
	r := &segmentedRenderer{s: s, track: track}
	r.region = canvas.NewRaster(regionImage)
	r.thumb = canvas.NewRaster(func(w, h int) image.Image {
		scale := float64(h) / float64(segmentRail-2*segmentGap+2*segmentShadow)
		img := thumbImage(w, h, float64(segmentShadow)*scale, r.laneOpen)
		if r.s.Disabled() {
			pix := img.(*image.NRGBA).Pix
			for i := 3; i < len(pix); i += 4 {
				pix[i] /= 2
			}
		}
		return img
	})
	r.low = canvas.NewRaster(func(w, h int) image.Image { return iconImage(iconStrengthLow, w, h, r.tint()) })
	r.high = canvas.NewRaster(func(w, h int) image.Image { return iconImage(iconStrengthHigh, w, h, r.tint()) })
	r.rebuild()
	return r
}

type segmentedRenderer struct {
	s             *Segmented
	track         *canvas.Rectangle
	region, thumb *canvas.Raster
	low, high     *canvas.Raster
	icons         []*canvas.Raster
	labels        []*text
	built         []string
	laneOpen      bool
	objects       []fyne.CanvasObject
}

func (r *segmentedRenderer) tint() color.Color {
	if r.s.Disabled() {
		return look(colorDim)
	}
	return look(theme.ColorNameForeground)
}

func (r *segmentedRenderer) rebuild() {
	r.icons, r.labels = nil, nil
	r.built = append([]string(nil), r.s.Options...)
	// The capsule lies under the lane's markers and the icons: it slides beneath them, and
	// whichever it reaches shows on it.
	r.objects = []fyne.CanvasObject{r.track, r.region, r.thumb, r.low, r.high}
	for _, option := range r.s.Options {
		kind := r.s.Icons[option]
		icon := canvas.NewRaster(func(w, h int) image.Image { return iconImage(kind, w, h, r.tint()) })
		label := newText(option, textLabel, false, theme.ColorNameForeground, fyne.TextAlignCenter)
		r.icons = append(r.icons, icon)
		r.labels = append(r.labels, label)
		r.objects = append(r.objects, icon, label)
	}
}

func (r *segmentedRenderer) MinSize() fyne.Size {
	return fyne.NewSize(r.s.widestLabel()*float32(max(1, len(r.labels))), segmentRail+segmentLabel)
}

func (r *segmentedRenderer) Layout(size fyne.Size) {
	r.track.Resize(fyne.NewSize(size.Width, segmentRail))
	segments := r.s.segments(size.Width)
	selected := r.s.selectedIndex()
	lane := r.s.lane(size.Width)
	r.laneOpen = lane >= 0
	inner := segmentRail - 2*segmentGap

	thumb, hasThumb := r.s.thumbSpan(size.Width)
	for i := range r.icons {
		if i >= len(segments) {
			break
		}
		// The Adaptive icon stays in the middle of its lane; only the capsule moves.
		center := segments[i].x + segments[i].width/2
		r.icons[i].Resize(fyne.NewSquareSize(segmentIcon))
		r.icons[i].Move(fyne.NewPos(center-segmentIcon/2, (segmentRail-segmentIcon)/2))
		r.labels[i].Resize(fyne.NewSize(segments[i].width, segmentLabel-6))
		r.labels[i].Move(fyne.NewPos(segments[i].x, segmentRail+6))
	}
	if hasThumb {
		r.thumb.Show()
		r.thumb.Move(fyne.NewPos(thumb.x-segmentShadow, segmentGap-segmentShadow))
		r.thumb.Resize(fyne.NewSize(thumb.width+2*segmentShadow, inner+2*segmentShadow))
	} else {
		r.thumb.Hide()
	}
	if lane >= 0 && selected >= 0 {
		span := segments[selected]
		r.region.Show()
		r.region.Move(fyne.NewPos(span.x, segmentGap))
		r.region.Resize(fyne.NewSize(span.width, inner))
		r.low.Show()
		r.high.Show()
		y := (segmentRail - segmentMarker) / 2
		r.low.Resize(fyne.NewSquareSize(segmentMarker))
		r.high.Resize(fyne.NewSquareSize(segmentMarker))
		r.low.Move(fyne.NewPos(span.x+16, y))
		r.high.Move(fyne.NewPos(span.x+span.width-16-segmentMarker, y))
	} else {
		r.region.Hide()
		r.low.Hide()
		r.high.Hide()
	}
}

func (r *segmentedRenderer) Refresh() {
	same := len(r.built) == len(r.s.Options)
	for i := 0; same && i < len(r.built); i++ {
		same = r.built[i] == r.s.Options[i]
	}
	if !same {
		r.rebuild()
	}
	r.track.FillColor = look(colorRail)
	r.track.StrokeWidth = 0
	if r.s.focused && !r.s.Disabled() {
		r.track.StrokeColor, r.track.StrokeWidth = look(theme.ColorNameFocus), 2
	}
	r.Layout(r.s.Size())
	r.track.Refresh()
	r.region.Refresh()
	r.thumb.Refresh()
	r.low.Refresh()
	r.high.Refresh()
	for i := range r.icons {
		r.icons[i].Refresh()
		r.labels[i].Fill = theme.ColorNameForeground
		if r.s.Disabled() {
			r.labels[i].Fill = colorDim
		}
		r.labels[i].Refresh()
	}
}

func (r *segmentedRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *segmentedRenderer) Destroy()                     {}

// batteryColumn is one of the three: artwork, gauge, percentage and label.
type batteryColumn struct {
	art     *canvas.Raster
	ring    *canvas.Raster
	percent *text
	label   *text
	note    *text
	level   int
	dim     bool
	artDim  bool
	kind    int
	set     string
	box     fyne.CanvasObject
}

const (
	batteryArt  float32 = 125
	batteryRing float32 = 32
)

func newBatteryColumn(kind int, name string) *batteryColumn {
	return newBatteryColumnSized(kind, name, batteryArt, columnGaps)
}

// newBatteryColumnSized is the same column around a picture of another size, with its own
// gaps: the Show batteries window is the page's header in small.
func newBatteryColumnSized(kind int, name string, art float32, gaps []float32) *batteryColumn {
	c := &batteryColumn{level: -1, dim: true, artDim: true, kind: kind, set: defaultArtSet}
	c.art = canvas.NewRaster(func(w, h int) image.Image { return modelArtImage(c.set, c.kind, w, h, c.artDim) })
	c.art.SetMinSize(fyne.NewSquareSize(art))
	c.ring = canvas.NewRaster(func(w, h int) image.Image { return ringImage(w, h, c.level) })
	c.ring.SetMinSize(fyne.NewSquareSize(batteryRing))
	c.percent = newText("–", textPercent, false, colorDim, fyne.TextAlignCenter)
	c.label = newText(name, textLabel, false, colorSecondary, fyne.TextAlignCenter)
	c.note = newText(" ", 11, false, colorSecondary, fyne.TextAlignCenter)
	c.box = &fyne.Container{Layout: columnBoxLayout{gaps: gaps}, Objects: []fyne.CanvasObject{c.art, c.ring, c.percent, c.label, c.note}}
	return c
}

// setArt changes the picture, for a model of its own or the headphones' single image.
func (c *batteryColumn) setArt(set string, kind int) {
	if set != c.set || kind != c.kind {
		c.set, c.kind = set, kind
		c.art.Refresh()
	}
}

// show applies a reading; level below zero is unknown.
func (c *batteryColumn) show(level int, percent, note string, dim bool) {
	c.level, c.dim, c.artDim = level, dim, dim
	if percent == "" {
		percent = "–"
	}
	c.percent.Fill = theme.ColorNameForeground
	if level < 0 || dim {
		c.percent.Fill = colorSecondary
	}
	if level < 0 {
		c.percent.Fill = colorDim
	}
	c.percent.Text = percent
	c.percent.Refresh()
	if note == "" {
		note = " "
	}
	c.note.SetText(note)
	c.ring.Refresh()
	c.art.Refresh()
}

// setArtDim decides the picture on its own: an old reading dims the numbers, but earbuds
// that are connected are still there.
func (c *batteryColumn) setArtDim(dim bool) {
	if dim != c.artDim {
		c.artDim = dim
		c.art.Refresh()
	}
}

// columnBoxLayout stacks a battery column with its fixed gaps, centred. Each child keeps a
// fixed slot so a changing reading never moves anything.
type columnBoxLayout struct{ gaps []float32 }

var columnGaps = []float32{23, 10, 8, 0, 0}

func (l columnBoxLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	y := float32(0)
	for i, object := range objects {
		m := object.MinSize()
		if i >= 2 {
			object.Resize(fyne.NewSize(size.Width, m.Height))
			object.Move(fyne.NewPos(0, y))
		} else {
			object.Resize(m)
			object.Move(fyne.NewPos((size.Width-m.Width)/2, y))
		}
		y += m.Height + l.gaps[i]
	}
}

func (l columnBoxLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var out fyne.Size
	for i, object := range objects {
		m := object.MinSize()
		out.Width = max(out.Width, m.Width)
		out.Height += m.Height + l.gaps[i]
	}
	return out
}

func newColumnBox(objects ...fyne.CanvasObject) *fyne.Container {
	return &fyne.Container{Layout: columnBoxLayout{gaps: columnGaps}, Objects: objects}
}
