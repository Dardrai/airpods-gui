package ui

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	sliderTrackHeight float32 = 4
	sliderThumb       float32 = 20
	sliderThumbShadow float32 = 23
	sliderFocusRing   float32 = 25 // fits inside sliderEndPad, so nothing is clipped at the ends
	sliderDot         float32 = 12
	sliderDotHover    float32 = 14
	sliderDotPressed  float32 = 10
	sliderMinWidth    float32 = 48
	sliderMinHeight   float32 = 32
)

var (
	_ fyne.Draggable    = (*Slider)(nil)
	_ fyne.Focusable    = (*Slider)(nil)
	_ fyne.Tappable     = (*Slider)(nil)
	_ fyne.Disableable  = (*Slider)(nil)
	_ desktop.Hoverable = (*Slider)(nil)
	_ desktop.Mouseable = (*Slider)(nil)
)

// Slider is the page's slider: a rounded track, blue up to a round thumb and grey after it.
// Tap, drag, arrow keys, SetValue, OnChanged and OnChangeEnded behave as in widget.Slider.
//
// A value that comes from the device goes through ShowValue, which reports nothing. While
// the user drags, the drag owns the value: both ShowValue and SetValue leave it alone.
type Slider struct {
	widget.DisableableWidget
	Value, Min, Max, Step float64
	OnChanged             func(float64)
	OnChangeEnded         func(float64)

	// onDragIdle runs when a drag ends without changing the value, so OnChangeEnded does
	// not run. A reading held back during the drag can be shown then.
	onDragIdle func()

	unknown  bool
	hovered  bool
	focused  bool
	pressed  bool
	dragging bool
	pending  bool // the value changed since the last OnChangeEnded
}

// NewSlider makes a slider with a step of one, like widget.NewSlider.
func NewSlider(low, high float64) *Slider {
	s := &Slider{Min: low, Max: high, Step: 1}
	s.ExtendBaseWidget(s)
	return s
}

// Dragging reports a drag in progress. A reading that arrives now should wait.
func (s *Slider) Dragging() bool { return s.dragging }

// SetUnknown says nobody has reported the value, so the thumb must not claim one. A drag
// shows its thumb anyway, for it is the user's own value.
func (s *Slider) SetUnknown(unknown bool) {
	if s.unknown != unknown {
		s.unknown = unknown
		s.Refresh()
	}
}

// SetValue moves the thumb and reports the change, as widget.Slider does. It does nothing
// during a drag.
func (s *Slider) SetValue(value float64) {
	if s.dragging || s.Value == value {
		return
	}
	last := s.Value
	s.Value = s.snap(value)
	s.moved(last)
	s.fireChangeEnded()
}

// ShowValue moves the thumb to a value that came from elsewhere. It reports nothing, and
// does nothing during a drag.
func (s *Slider) ShowValue(value float64) {
	if s.dragging {
		return
	}
	if value = s.snap(value); value != s.Value {
		s.Value = value
		s.Refresh()
	}
}

// Tapped jumps to the pointer, focuses the slider and reports the end of a change.
func (s *Slider) Tapped(e *fyne.PointEvent) {
	if s.Disabled() {
		return
	}
	if !s.focused {
		if c := fyne.CurrentApp().Driver().CanvasForObject(s); c != nil {
			c.Focus(s)
		}
	}
	s.moveTo(e.Position.X)
	s.fireChangeEnded()
}

// Dragged follows the pointer.
func (s *Slider) Dragged(e *fyne.DragEvent) {
	if s.Disabled() {
		return
	}
	if !s.dragging {
		s.dragging = true
		s.Refresh() // an unknown slider lights up for the drag
	}
	s.moveTo(e.Position.X)
}

// DragEnd reports the end of a change, once, however far the drag went.
func (s *Slider) DragEnd() {
	wasDragging := s.dragging
	s.dragging, s.pressed = false, false
	s.Refresh()
	if s.Disabled() || !wasDragging {
		return
	}
	if !s.pending && s.onDragIdle != nil {
		s.onDragIdle()
	}
	s.fireChangeEnded()
}

// Disable also abandons a drag: its value is not reported.
func (s *Slider) Disable() {
	s.dragging, s.pressed, s.pending = false, false, false
	s.DisableableWidget.Disable()
}

// Hide also abandons a drag, whose end may then never arrive.
func (s *Slider) Hide() {
	s.dragging, s.pressed, s.pending = false, false, false
	s.DisableableWidget.Hide()
}

// MouseDown shows the pressed thumb. It also clears a drag whose end got lost, for example
// when the window went away during it.
func (s *Slider) MouseDown(*desktop.MouseEvent) {
	s.dragging = false
	if !s.Disabled() {
		s.pressed = true
		s.Refresh()
	}
}

// MouseUp ends the pressed look.
func (s *Slider) MouseUp(*desktop.MouseEvent) {
	if s.pressed {
		s.pressed = false
		s.Refresh()
	}
}

// MouseIn tracks the hover state.
func (s *Slider) MouseIn(*desktop.MouseEvent) { s.hovered = true; s.Refresh() }

// MouseMoved does nothing.
func (s *Slider) MouseMoved(*desktop.MouseEvent) {}

// MouseOut tracks the hover state.
func (s *Slider) MouseOut() { s.hovered = false; s.Refresh() }

// FocusGained shows the focus ring.
func (s *Slider) FocusGained() { s.focused = true; s.Refresh() }

// FocusLost hides the focus ring.
func (s *Slider) FocusLost() { s.focused = false; s.Refresh() }

// TypedKey moves one step on the left and right arrows.
func (s *Slider) TypedKey(key *fyne.KeyEvent) {
	if s.Disabled() {
		return
	}
	switch key.Name {
	case fyne.KeyLeft:
		s.SetValue(s.Value - s.Step)
	case fyne.KeyRight:
		s.SetValue(s.Value + s.Step)
	}
}

// TypedRune does nothing.
func (s *Slider) TypedRune(rune) {}

func (s *Slider) moveTo(x float32) {
	last := s.Value
	s.Value = s.snap(s.Min + s.ratioAt(x)*(s.Max-s.Min))
	s.moved(last)
}

func (s *Slider) moved(last float64) {
	if math.Abs(last-s.Value) <= s.Step/2 {
		return
	}
	s.Refresh()
	s.pending = true
	if s.OnChanged != nil {
		s.OnChanged(s.Value)
	}
}

func (s *Slider) fireChangeEnded() {
	if !s.pending {
		return
	}
	s.pending = false
	if s.OnChangeEnded != nil {
		s.OnChangeEnded(s.Value)
	}
}

func (s *Slider) ratioAt(x float32) float64 {
	travel := s.Size().Width - sliderEndPad*2
	if travel <= 0 {
		return 0
	}
	return math.Min(1, math.Max(0, float64((x-sliderEndPad)/travel)))
}

// snap clamps to the range and rounds to a multiple of the step. Dividing by the inverse
// gives the closest float to 0.57, where multiplying by 0.01 would not.
func (s *Slider) snap(value float64) float64 {
	if value >= s.Max {
		return s.Max
	}
	if value <= s.Min {
		return s.Min
	}
	if s.Step > 0 {
		inverse := 1 / s.Step
		value = math.Round(value*inverse) / inverse
	}
	return math.Min(s.Max, math.Max(s.Min, value))
}

func (s *Slider) ratio() float32 {
	if s.Max <= s.Min {
		return 0
	}
	return float32(math.Min(1, math.Max(0, (s.Value-s.Min)/(s.Max-s.Min))))
}

// showsUnknown is the state where no bright thumb may appear.
func (s *Slider) showsUnknown() bool { return s.unknown && !s.dragging }

// CreateRenderer builds the track and thumb.
func (s *Slider) CreateRenderer() fyne.WidgetRenderer {
	s.ExtendBaseWidget(s)
	fill := canvas.NewRectangle(color.Transparent)
	fill.CornerRadius = sliderTrackHeight / 2
	rest := canvas.NewRectangle(color.Transparent)
	rest.CornerRadius = sliderTrackHeight / 2
	r := &sliderRenderer{
		s: s, fill: fill, rest: rest,
		ring:   canvas.NewCircle(color.Transparent),
		shadow: canvas.NewCircle(color.Transparent),
		disc:   canvas.NewCircle(color.Transparent),
		dot:    canvas.NewCircle(color.Transparent),
	}
	r.Refresh()
	return r
}

type sliderRenderer struct {
	s                       *Slider
	fill, rest              *canvas.Rectangle
	ring, shadow, disc, dot *canvas.Circle
}

func (r *sliderRenderer) MinSize() fyne.Size { return fyne.NewSize(sliderMinWidth, sliderMinHeight) }

func (r *sliderRenderer) dotSize() float32 {
	switch {
	case r.s.Disabled():
		return sliderDot
	case r.s.pressed || r.s.dragging:
		return sliderDotPressed
	case r.s.hovered:
		return sliderDotHover
	}
	return sliderDot
}

func (r *sliderRenderer) Layout(size fyne.Size) {
	travel := max(0, size.Width-sliderEndPad*2)
	x := sliderEndPad + travel*r.s.ratio()
	y := size.Height / 2
	// The two parts meet under the thumb, so no grey shows around the blue caps.
	r.fill.Move(fyne.NewPos(sliderEndPad, y-sliderTrackHeight/2))
	r.fill.Resize(fyne.NewSize(x-sliderEndPad, sliderTrackHeight))
	r.fill.Hidden = x-sliderEndPad < sliderTrackHeight || r.s.showsUnknown()
	restStart := x
	if r.s.showsUnknown() {
		restStart = sliderEndPad
	}
	r.rest.Move(fyne.NewPos(restStart, y-sliderTrackHeight/2))
	r.rest.Resize(fyne.NewSize(sliderEndPad+travel-restStart, sliderTrackHeight))
	r.rest.Hidden = sliderEndPad+travel-restStart < sliderTrackHeight
	center := func(c *canvas.Circle, side float32) {
		c.Move(fyne.NewPos(x-side/2, y-side/2))
		c.Resize(fyne.NewSquareSize(side))
	}
	center(r.ring, sliderFocusRing)
	center(r.shadow, sliderThumbShadow)
	center(r.disc, sliderThumb)
	center(r.dot, r.dotSize())
}

func (r *sliderRenderer) Refresh() {
	s := r.s
	active, rest, disc := look(theme.ColorNamePrimary), look(colorTrack), look(colorThumb)
	switch {
	case s.showsUnknown():
		disc, rest = look(colorDim), fade(rest, 0.6)
		if s.Disabled() {
			rest = fade(rest, 0.6)
		}
	case s.Disabled():
		active, rest, disc = look(colorDim), fade(rest, 0.5), look(colorDim)
	}
	r.fill.FillColor, r.rest.FillColor = active, rest
	r.disc.FillColor = disc
	r.dot.FillColor = active
	r.dot.Hidden = s.showsUnknown() || s.Disabled()
	r.shadow.FillColor = look(theme.ColorNameShadow)
	r.ring.StrokeColor, r.ring.StrokeWidth = color.Transparent, 0
	if s.focused && !s.Disabled() {
		r.ring.StrokeColor, r.ring.StrokeWidth = look(theme.ColorNameFocus), 2
	}
	r.Layout(s.Size())
	canvas.Refresh(s)
}

func (r *sliderRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.fill, r.rest, r.ring, r.shadow, r.disc, r.dot}
}

func (r *sliderRenderer) Destroy() {}
