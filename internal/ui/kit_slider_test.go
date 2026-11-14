package ui

import (
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// testSlider has a 200 wide track, so x = sliderEndPad + 2*percent.
func testSlider() (s *Slider, changed, ended *[]float64) {
	test.NewApp()
	changed, ended = &[]float64{}, &[]float64{}
	s = NewSlider(0, 100)
	s.OnChanged = func(v float64) { *changed = append(*changed, v) }
	s.OnChangeEnded = func(v float64) { *ended = append(*ended, v) }
	s.Resize(fyne.NewSize(200+sliderEndPad*2, sliderMinHeight))
	return s, changed, ended
}

func dragTo(s *Slider, percent float32) {
	s.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(sliderEndPad+2*percent, 10)}})
}

func equalValues(a []float64, b ...float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSliderClickJumpsAndFocuses(t *testing.T) {
	s, changed, ended := testSlider()
	window := test.NewWindow(s)
	defer window.Close()
	window.Resize(fyne.NewSize(300, 100))
	s.Resize(fyne.NewSize(200+sliderEndPad*2, sliderMinHeight))
	test.TapAt(s, fyne.NewPos(sliderEndPad+80, 10))
	if s.Value != 40 || !equalValues(*changed, 40) || !equalValues(*ended, 40) {
		t.Errorf("value %v changed %v ended %v", s.Value, *changed, *ended)
	}
	if !s.focused {
		t.Error("a click did not focus the slider")
	}
	test.TapAt(s, fyne.NewPos(sliderEndPad+80, 10))
	if len(*changed) != 1 || len(*ended) != 1 {
		t.Errorf("a click on the same value reported: %v %v", *changed, *ended)
	}
}

func TestSliderDragReportsMovesAndOneEnd(t *testing.T) {
	s, changed, ended := testSlider()
	for _, percent := range []float32{10, 10.1, 30, 55} {
		dragTo(s, percent)
	}
	if !s.Dragging() || !equalValues(*changed, 10, 30, 55) || len(*ended) != 0 {
		t.Fatalf("during the drag: dragging %v changed %v ended %v", s.Dragging(), *changed, *ended)
	}
	s.DragEnd()
	if s.Dragging() || !equalValues(*ended, 55) {
		t.Fatalf("after the drag: dragging %v ended %v", s.Dragging(), *ended)
	}
	s.DragEnd()
	if len(*ended) != 1 {
		t.Fatalf("a second end reported: %v", *ended)
	}
}

func TestSliderDragPastTheEndsClamps(t *testing.T) {
	s, _, ended := testSlider()
	dragTo(s, 500)
	if s.Value != 100 {
		t.Errorf("past the right end: %v", s.Value)
	}
	dragTo(s, -500)
	if s.Value != 0 {
		t.Errorf("past the left end: %v", s.Value)
	}
	s.DragEnd()
	if !equalValues(*ended, 0) {
		t.Errorf("ended %v", *ended)
	}
}

func TestSliderDragThatChangesNothingEndsIdle(t *testing.T) {
	s, changed, ended := testSlider()
	s.Value = 50
	idle := 0
	s.onDragIdle = func() { idle++ }
	dragTo(s, 50)
	s.DragEnd()
	if idle != 1 || len(*changed) != 0 || len(*ended) != 0 {
		t.Errorf("idle %d changed %v ended %v", idle, *changed, *ended)
	}
}

func TestSliderKeyboardSteps(t *testing.T) {
	s, changed, ended := testSlider()
	s.Min, s.Max, s.Step = -1, 1, 0.25
	s.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	s.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	s.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	s.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp}) // as in widget.Slider, not on a level slider
	if s.Value != 0.25 || !equalValues(*changed, 0.25, 0.5, 0.25) || !equalValues(*ended, 0.25, 0.5, 0.25) {
		t.Errorf("value %v changed %v ended %v", s.Value, *changed, *ended)
	}
	for range 10 {
		s.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	}
	if s.Value != 1 {
		t.Errorf("keys passed the maximum: %v", s.Value)
	}
}

func TestSliderStepAndRange(t *testing.T) {
	s, _, _ := testSlider()
	s.Min, s.Max, s.Step = -1, 1, 0.01
	for _, c := range []struct{ in, want float64 }{{0.57, 0.57}, {-0.57, -0.57}, {0.574, 0.57}, {3, 1}, {-3, -1}} {
		if s.SetValue(c.in); s.Value != c.want {
			t.Errorf("SetValue(%v) = %v, want %v", c.in, s.Value, c.want)
		}
	}
	s.Step = 0
	if s.SetValue(0.123); s.Value != 0.123 {
		t.Errorf("no step: %v", s.Value)
	}
}

func TestSliderSetValueReportsAndShowValueDoesNot(t *testing.T) {
	s, changed, ended := testSlider()
	s.SetValue(20)
	if !equalValues(*changed, 20) || !equalValues(*ended, 20) {
		t.Fatalf("SetValue: changed %v ended %v", *changed, *ended)
	}
	s.ShowValue(70)
	if s.Value != 70 || len(*changed) != 1 || len(*ended) != 1 {
		t.Fatalf("ShowValue: value %v changed %v ended %v", s.Value, *changed, *ended)
	}
	s.ShowValue(500)
	if s.Value != 100 {
		t.Fatalf("ShowValue clamps: %v", s.Value)
	}
}

func TestSliderDragOwnsTheValue(t *testing.T) {
	s, changed, ended := testSlider()
	dragTo(s, 30)
	s.ShowValue(80)
	s.SetValue(90)
	if s.Value != 30 || !equalValues(*changed, 30) || len(*ended) != 0 {
		t.Fatalf("value %v changed %v ended %v", s.Value, *changed, *ended)
	}
	dragTo(s, 40)
	s.DragEnd()
	if s.Value != 40 || !equalValues(*ended, 40) {
		t.Fatalf("value %v ended %v", s.Value, *ended)
	}
	s.ShowValue(80)
	if s.Value != 80 || len(*ended) != 1 {
		t.Fatalf("after release: value %v ended %v", s.Value, *ended)
	}
}

func TestSliderDisabledIgnoresInput(t *testing.T) {
	s, changed, ended := testSlider()
	s.Disable()
	test.TapAt(s, fyne.NewPos(sliderEndPad+80, 10))
	dragTo(s, 50)
	s.DragEnd()
	s.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if s.Value != 0 || len(*changed) != 0 || len(*ended) != 0 {
		t.Errorf("value %v changed %v ended %v", s.Value, *changed, *ended)
	}
	s.ShowValue(30)
	if s.Value != 30 {
		t.Errorf("a disabled slider still shows readings: %v", s.Value)
	}
}

func TestSliderDisableAbandonsADrag(t *testing.T) {
	s, _, ended := testSlider()
	dragTo(s, 30)
	s.Disable()
	if s.Dragging() {
		t.Error("still dragging")
	}
	s.DragEnd()
	s.Enable()
	s.SetValue(s.Value)
	if len(*ended) != 0 {
		t.Errorf("an abandoned drag reported: %v", *ended)
	}
}

func TestSliderLostDragEndIsCleared(t *testing.T) {
	s, _, _ := testSlider()
	dragTo(s, 30)
	s.MouseDown(nil)
	if s.Dragging() {
		t.Error("the next press did not clear the drag")
	}
	s.MouseUp(nil)
	s.ShowValue(70)
	if s.Value != 70 {
		t.Errorf("value %v", s.Value)
	}
}

func TestSliderUnknownHasNoBrightThumb(t *testing.T) {
	s, _, _ := testSlider()
	r := test.TempWidgetRenderer(t, s).(*sliderRenderer)
	s.SetUnknown(true)
	if !r.dot.Hidden || !r.fill.Hidden || r.disc.FillColor != look(colorDim) {
		t.Errorf("unknown: dot hidden %v fill hidden %v disc %v", r.dot.Hidden, r.fill.Hidden, r.disc.FillColor)
	}
	dragTo(s, 30)
	if r.dot.Hidden || r.fill.Hidden {
		t.Error("a drag shows the user's own value")
	}
	s.DragEnd()
	if !r.dot.Hidden {
		t.Error("still unknown after the drag until a value is reported")
	}
	s.SetUnknown(false)
	if r.dot.Hidden || r.fill.Hidden {
		t.Error("known: the thumb stays dim")
	}
}

func TestSliderRendererFollowsTheValue(t *testing.T) {
	s, _, _ := testSlider()
	r := test.TempWidgetRenderer(t, s).(*sliderRenderer)
	s.SetValue(50)
	center := r.disc.Position().X + r.disc.Size().Width/2
	if center != sliderEndPad+100 {
		t.Errorf("thumb centre %v", center)
	}
	if end := r.fill.Position().X + r.fill.Size().Width; end != center {
		t.Errorf("fill ends at %v", end)
	}
	if r.rest.Position().X != center || r.rest.Position().X+r.rest.Size().Width != sliderEndPad+200 {
		t.Errorf("rest %v %v", r.rest.Position(), r.rest.Size())
	}
	s.Disable()
	if !r.dot.Hidden {
		t.Error("a disabled thumb keeps its blue dot")
	}
}

// A reading during a drag must not move the thumb, change the number or write; release
// writes the dragged value once.
func TestSettingSliderPollDuringDragWritesOnceOnRelease(t *testing.T) {
	r := newRig(t, ownerConfig())
	r.shown()
	r.until(func() bool { return !r.w.settings.reading }, "read")
	const toneControl = 0x1f
	row := r.w.settings.byName["tone-volume"]
	writes := func() int {
		count := 0
		for _, commit := range r.dev.Commits() {
			if len(commit) == 6 && commit[0] == 1 && commit[1] == toneControl {
				count++
			}
		}
		return count
	}
	before := writes()
	r.main(func() {
		if row.slider.Disabled() {
			t.Fatal("tone-volume is disabled")
		}
		row.slider.Resize(fyne.NewSize(200+sliderEndPad*2, sliderMinHeight))
		dragTo(row.slider, 20)
		dragTo(row.slider, 30)
		row.decoded, row.known = "77", true
		r.w.settings.restore(row)
		r.w.settings.relabel()
		if row.slider.Value != 30 || row.number.Text != "30%" {
			t.Errorf("the poll moved the drag: slider %v, number %q", row.slider.Value, row.number.Text)
		}
	})
	if got := writes(); got != before {
		t.Errorf("wrote %d times during the drag", got-before)
	}
	r.main(func() {
		row.slider.Resize(fyne.NewSize(200+sliderEndPad*2, sliderMinHeight)) // the page laid it out again
		dragTo(row.slider, 35)
		row.slider.DragEnd()
	})
	r.until(func() bool { return !row.writing }, "write finished")
	if got := writes(); got != before+1 {
		t.Errorf("wrote %d times on release, want one", got-before)
	}
	if got := lastCommitValue(t, r.dev, toneControl); got != 0x5000|35 {
		t.Errorf("wrote 0x%x", got)
	}
}

// A drag that changed nothing writes nothing and catches up with what was reported meanwhile.
func TestSettingSliderIdleDragShowsTheHeldReading(t *testing.T) {
	r := newRig(t, ownerConfig())
	r.shown()
	r.until(func() bool { return !r.w.settings.reading }, "read")
	row := r.w.settings.byName["tone-volume"]
	r.main(func() {
		row.slider.Resize(fyne.NewSize(200+sliderEndPad*2, sliderMinHeight))
		row.slider.ShowValue(40)
		dragTo(row.slider, 40)
		row.decoded, row.known = "77", true
		r.w.settings.restore(row)
		row.slider.DragEnd()
		if row.slider.Value != 77 || row.number.Text != "77%" {
			t.Errorf("slider %v, number %q", row.slider.Value, row.number.Text)
		}
	})
}

// A device load in the hearing and transparency editors stays silent.
func TestSetSliderRowDoesNotReportOrFightADrag(t *testing.T) {
	test.NewApp()
	s := NewSlider(0, 1)
	s.Step = 0.01
	row := newSliderRow("Amount", "", -1, s)
	reports := 0
	s.OnChanged = func(float64) { reports++ }
	setSliderRow(row, 0.5)
	if s.Value != 0.5 || row.Value.Text != "0.50" || reports != 0 {
		t.Fatalf("value %v text %q reports %d", s.Value, row.Value.Text, reports)
	}
	s.Resize(fyne.NewSize(200+sliderEndPad*2, sliderMinHeight))
	s.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(sliderEndPad+50, 10)}})
	row.SetValue("0.25")
	setSliderRow(row, 0.9)
	if s.Value != 0.25 || row.Value.Text != "0.25" {
		t.Fatalf("the load moved the drag: %v %q", s.Value, row.Value.Text)
	}
}

// TestSliderSheet saves each state in both themes when AIRPODS_SCREENS_DIR is set.
func TestSliderSheet(t *testing.T) {
	dir := os.Getenv("AIRPODS_SCREENS_DIR")
	if dir == "" {
		t.Skip("AIRPODS_SCREENS_DIR not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	app := test.NewApp()
	for _, variant := range []string{ThemeDark, ThemeLight} {
		app.Settings().SetTheme(themeFor(variant))
		shoot(t, filepath.Join(dir, "slider-"+variant+".png"), sliderSheet(), 560, 2)
	}
}

func sliderSheet() fyne.CanvasObject {
	line := func(title, value string, change func(*Slider), percent float64) fyne.CanvasObject {
		s := NewSlider(0, 100)
		s.Value = percent
		if change != nil {
			change(s)
		}
		row := newSliderRow(title, "", -1, s)
		row.SetValue(value)
		return row
	}
	volume := NewSlider(0, 100)
	volume.Value = 42
	tiled := newSliderRow("", "", iconSpeaker, volume)
	tiled.SetValue("42%")
	return pageBox(
		caption("Volume"),
		card(tiled),
		spaced(12, caption("States")),
		card(newStack(
			line("Normal", "42", nil, 42),
			line("Hover", "42", func(s *Slider) { s.hovered = true }, 42),
			line("Pressed", "42", func(s *Slider) { s.pressed = true }, 42),
			line("Focused", "42", func(s *Slider) { s.focused = true }, 42),
			line("Disabled", "42", (*Slider).Disable, 42),
			line("Unknown", "", func(s *Slider) { s.unknown = true }, 0),
			line("Unknown, disabled", "", func(s *Slider) { s.unknown = true; s.Disable() }, 0),
			line("Minimum", "0", nil, 0),
			line("Maximum", "100", nil, 100),
		)),
	)
}
