package ui

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func TestSwitchTogglesAndRespectsDisabled(t *testing.T) {
	test.NewApp()
	var got []bool
	s := NewSwitch(func(on bool) { got = append(got, on) })
	test.Tap(s)
	s.Disable()
	test.Tap(s)
	s.Enable()
	s.SetChecked(false)
	if len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("changes = %v", got)
	}
}

func TestSegmentedHitTestingFollowsTheOpenLane(t *testing.T) {
	test.NewApp()
	options := []string{"T", "A", "N"}
	icons := map[string]int{"T": iconTransparency, "A": iconAdaptive, "N": iconCancellation}
	var got []string
	s := NewSegmented(options, icons, func(v string) { got = append(got, v) })
	s.Resize(fyne.NewSize(640, s.MinSize().Height))
	s.SetSelected("A")
	got = nil
	// With Adaptive open, a tap a quarter along still lands in the wider Adaptive lane.
	s.Tapped(&fyne.PointEvent{Position: fyne.NewPos(640*0.25, 20)})
	if len(got) != 0 {
		t.Fatalf("tap in lane changed selection: %v", got)
	}
	s.Tapped(&fyne.PointEvent{Position: fyne.NewPos(30, 20)})
	if len(got) != 1 || got[0] != "T" {
		t.Fatalf("tap on first segment: %v", got)
	}
	s.ClearSelected()
	if s.Selected != "" || len(got) != 1 {
		t.Fatalf("clear reported a change: %v", got)
	}
}

// dragSegmented is the four modes at a width where Adaptive opens its lane, with what the
// control reported.
func dragSegmented(selected string) (s *Segmented, modes *[]string, strengths *[]int) {
	test.NewApp()
	icons := map[string]int{"O": iconOff, "T": iconTransparency, "A": iconAdaptive, "N": iconCancellation}
	modes, strengths = new([]string), new([]int)
	s = NewSegmented([]string{"O", "T", "A", "N"}, icons, func(v string) { *modes = append(*modes, v) })
	s.OnStrength = func(v int) { *strengths = append(*strengths, v) }
	s.Resize(fyne.NewSize(640, s.MinSize().Height))
	if selected != "" {
		s.SetSelected(selected)
	}
	*modes = nil
	return s, modes, strengths
}

func dragSegmentTo(s *Segmented, xs ...float32) {
	for _, x := range xs {
		s.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(x, 20)}})
	}
}

func thumbCenter(t *testing.T, s *Segmented) float32 {
	t.Helper()
	thumb, ok := s.thumbSpan(s.Size().Width)
	if !ok {
		t.Fatal("no capsule")
	}
	return thumb.x + thumb.width/2
}

func TestSegmentedDragAcrossSelectsOnRelease(t *testing.T) {
	s, modes, strengths := dragSegmented("O")
	before := s.segments(640)
	for _, x := range []float32{120, 250, 400} {
		dragSegmentTo(s, x)
		if got := thumbCenter(t, s); got != x {
			t.Fatalf("capsule at %v with the pointer at %v", got, x)
		}
	}
	// Past the end the capsule stops inside the rail.
	dragSegmentTo(s, 900)
	if thumb, _ := s.thumbSpan(640); thumb.x+thumb.width != 640-segmentGap {
		t.Fatalf("capsule not held at the rail's end: %+v", thumb)
	}
	after := s.segments(640)
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("segments moved during the drag: %v -> %v", before, after)
		}
	}
	if len(*modes) != 0 || s.Selected != "O" {
		t.Fatalf("selection changed before release: %v, %q", *modes, s.Selected)
	}
	s.DragEnd()
	if len(*modes) != 1 || (*modes)[0] != "N" || s.Selected != "N" {
		t.Fatalf("after release: %v, %q", *modes, s.Selected)
	}
	if span := s.segments(640)[3]; thumbCenter(t, s) != span.x+span.width/2 {
		t.Fatal("capsule did not snap to the selected segment")
	}
	// A second release, with no drag before it, reports nothing.
	s.DragEnd()
	if len(*modes) != 1 || len(*strengths) != 0 {
		t.Fatalf("modes %v, strengths %v", *modes, *strengths)
	}
}

func TestSegmentedDragBackToStartReportsNothing(t *testing.T) {
	s, modes, strengths := dragSegmented("O")
	dragSegmentTo(s, 30, 300, 600, 300, 40)
	s.DragEnd()
	if len(*modes) != 0 || len(*strengths) != 0 || s.Selected != "O" {
		t.Fatalf("modes %v, strengths %v, selected %q", *modes, *strengths, s.Selected)
	}
	if span := s.segments(640)[0]; thumbCenter(t, s) != span.x+span.width/2 {
		t.Fatal("capsule did not go back to its segment")
	}
}

func TestSegmentedDragNeedsAnEnabledSelection(t *testing.T) {
	s, modes, strengths := dragSegmented("O")
	s.Disable()
	at := thumbCenter(t, s)
	dragSegmentTo(s, 300, 600)
	if thumbCenter(t, s) != at {
		t.Fatal("disabled capsule moved")
	}
	s.DragEnd()
	// Disabled half way through a drag: the capsule goes back and the release does nothing.
	s.Enable()
	dragSegmentTo(s, 600)
	s.Disable()
	if thumbCenter(t, s) != at {
		t.Fatal("capsule stayed out after Disable")
	}
	s.DragEnd()
	s.Enable()
	if thumbCenter(t, s) != at {
		t.Fatal("capsule stayed out after the release")
	}

	s.ClearSelected()
	dragSegmentTo(s, 300, 600)
	if _, ok := s.thumbSpan(640); ok {
		t.Fatal("a drag with nothing selected made a capsule")
	}
	s.DragEnd()
	// The selection lost half way through a drag ends the drag.
	s.SetSelected("O")
	*modes = nil
	dragSegmentTo(s, 600)
	s.ClearSelected()
	s.DragEnd()
	if len(*modes) != 0 || len(*strengths) != 0 || s.Selected != "" {
		t.Fatalf("modes %v, strengths %v, selected %q", *modes, *strengths, s.Selected)
	}
}

func TestSegmentedDragInLaneSetsStrength(t *testing.T) {
	s, modes, strengths := dragSegmented("A")
	s.SetStrength(30)
	lane := s.lane(640)
	if lane != 2 {
		t.Fatalf("lane = %d", lane)
	}
	span := s.segments(640)[lane]
	dragSegmentTo(s, span.x+span.width/3, span.x+span.width/2)
	if len(*strengths) != 0 || s.strength != 30 {
		t.Fatalf("strength changed before release: %v, %d", *strengths, s.strength)
	}
	s.DragEnd()
	if len(*strengths) != 1 || (*strengths)[0] != 50 || s.strength != 50 {
		t.Fatalf("strengths %v, strength %d", *strengths, s.strength)
	}
	if off := thumbCenter(t, s) - (span.x + span.width/2); off < -0.01 || off > 0.01 {
		t.Fatalf("capsule %v from where it was dropped", off)
	}
	// The lane's ends are its limits, though the capsule's centre can pass them.
	dragSegmentTo(s, span.x+1)
	s.DragEnd()
	dragSegmentTo(s, span.x+span.width-1)
	s.DragEnd()
	if len(*strengths) != 3 || (*strengths)[1] != 0 || (*strengths)[2] != 100 {
		t.Fatalf("strengths %v", *strengths)
	}
	if len(*modes) != 0 || s.Selected != "A" {
		t.Fatalf("modes %v, selected %q", *modes, s.Selected)
	}
	// Without OnStrength the lane takes no drag, and the capsule goes back.
	s.OnStrength = nil
	dragSegmentTo(s, span.x+span.width/2)
	s.DragEnd()
	if s.strength != 100 || len(*strengths) != 3 {
		t.Fatalf("strength %d, strengths %v", s.strength, *strengths)
	}
}

func TestSegmentedDragOutOfLaneChangesModeOnly(t *testing.T) {
	s, modes, strengths := dragSegmented("A")
	s.SetStrength(30)
	span := s.segments(640)[2]
	dragSegmentTo(s, span.x+span.width/2, span.x+span.width-1, 600)
	s.DragEnd()
	if len(*modes) != 1 || (*modes)[0] != "N" || s.Selected != "N" {
		t.Fatalf("modes %v, selected %q", *modes, s.Selected)
	}
	if len(*strengths) != 0 || s.strength != 30 {
		t.Fatalf("strengths %v, strength %d", *strengths, s.strength)
	}
	// Back onto Adaptive from outside: the lane was shut during the drag, so the mode
	// changes and the strength stays.
	dragSegmentTo(s, 600, 400)
	s.DragEnd()
	if len(*modes) != 2 || (*modes)[1] != "A" || s.Selected != "A" {
		t.Fatalf("modes %v, selected %q", *modes, s.Selected)
	}
	if len(*strengths) != 0 || s.strength != 30 {
		t.Fatalf("strengths %v, strength %d", *strengths, s.strength)
	}
	s.DragEnd()
	if len(*strengths) != 0 {
		t.Fatalf("a release with no drag reported %v", *strengths)
	}
}

func TestStackTidiesDividers(t *testing.T) {
	test.NewApp()
	a, b := widget.NewLabel("a"), widget.NewLabel("b")
	s := newStack(a, b)
	if s.dividers[0].Visible() || !s.dividers[1].Visible() {
		t.Fatal("divider only belongs between rows")
	}
	a.Hide()
	s.tidy()
	if s.dividers[1].Visible() || !s.anyVisible() {
		t.Fatal("hiding the first row must drop the divider")
	}
}

func TestSectionToggles(t *testing.T) {
	test.NewApp()
	body := widget.NewLabel("body")
	s := newSection("Title", "sub", body)
	var got []bool
	s.OnToggle = func(open bool) { got = append(got, open) }
	before := s.MinSize().Height
	s.Toggle()
	if !s.Open() || s.MinSize().Height <= before || len(got) != 1 || !got[0] {
		t.Fatalf("open=%v min %v -> %v, got %v", s.Open(), before, s.MinSize().Height, got)
	}
}

func TestThemeChangeReachesRasterParts(t *testing.T) {
	a := test.NewApp()
	a.Settings().SetTheme(themeFor(ThemeDark))
	dark := ringImage(40, 40, -1).(*image.NRGBA)
	a.Settings().SetTheme(themeFor(ThemeLight))
	light := ringImage(40, 40, -1).(*image.NRGBA)
	same := true
	for i := range dark.Pix {
		same = same && dark.Pix[i] == light.Pix[i]
	}
	if same {
		t.Fatal("ring did not follow the theme")
	}
}

func TestSegmentedOptionsCanChange(t *testing.T) {
	test.NewApp()
	s := gallerySegmented(galleryModes3, "Adaptive", false)
	c := software.NewCanvas()
	c.SetContent(s)
	c.Resize(fyne.NewSize(640, 120))
	c.Capture()
	s.SetOptions(galleryModes4)
	s.SetStrength(30)
	c.Capture()
	if got := len(test.WidgetRenderer(s).Objects()); got != 5+2*len(galleryModes4) {
		t.Fatalf("objects after SetOptions = %d", got)
	}
}

// A live control must repaint its drawn parts when the theme changes.
func TestThemeChangeRepaintsLiveParts(t *testing.T) {
	a := test.NewApp()
	a.Settings().SetTheme(themeFor(ThemeDark))
	s := gallerySegmented(galleryModes3, "", false)
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(s)
	c.Resize(fyne.NewSize(640, 100))
	extreme := func(img image.Image) (lightest, darkest uint32) {
		darkest = 0xffff
		for y := 2; y < int(segmentRail)-2; y++ {
			for x := 40; x < 600; x++ {
				r, _, _, _ := img.At(x, y).RGBA()
				lightest, darkest = max(lightest, r), min(darkest, r)
			}
		}
		return lightest, darkest
	}
	if lightest, _ := extreme(c.Capture()); lightest < 0xc000 {
		t.Fatalf("dark theme icons not light: %#x", lightest)
	}
	if _, darkest := extreme(software.RenderCanvas(c, themeFor(ThemeLight))); darkest > 0x4000 {
		t.Fatalf("light theme icons not dark: %#x", darkest)
	}
}

func TestArtworkLoadsDimsAndTints(t *testing.T) {
	test.NewApp()
	opaque := func(img image.Image) (count int) {
		n := img.(*image.NRGBA)
		for i := 3; i < len(n.Pix); i += 4 {
			if n.Pix[i] > 200 {
				count++
			}
		}
		return count
	}
	for _, kind := range []int{artLeft, artRight, artCase, artHeadphones} {
		if opaque(artImage(kind, 125, 125, false)) == 0 {
			t.Fatalf("art %d drew nothing", kind)
		}
		if opaque(artImage(kind, 125, 125, true)) != 0 {
			t.Fatalf("art %d not dimmed", kind)
		}
	}
	for _, kind := range []int{iconOff, iconTransparency, iconAdaptive, iconCancellation} {
		img := iconImage(kind, 44, 44, hex(0x102030)).(*image.NRGBA)
		if opaque(img) == 0 {
			t.Fatalf("icon %d drew nothing", kind)
		}
		for i := 3; i < len(img.Pix); i += 4 {
			if img.Pix[i] > 200 && (img.Pix[i-3] != 0x10 || img.Pix[i-1] != 0x30) {
				t.Fatalf("icon %d not tinted", kind)
			}
		}
	}
}

// TestRenderGallery writes every part in every state to dist/gallery.
func TestRenderGallery(t *testing.T) {
	if os.Getenv("RENDER_GALLERY") == "" {
		t.Skip("set RENDER_GALLERY=1 to write the gallery")
	}
	app := test.NewApp()
	dir := filepath.Join("..", "..", "dist", "gallery")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{ThemeDark, ThemeLight} {
		app.Settings().SetTheme(themeFor(variant))
		for _, scale := range []float32{1, 2} {
			name := variant + "-" + map[float32]string{1: "1x", 2: "2x"}[scale]
			shoot(t, filepath.Join(dir, "page-"+name+".png"), galleryPage(), 688, scale)
			shoot(t, filepath.Join(dir, "parts-"+name+".png"), galleryParts(), 688, scale)
			shoot(t, filepath.Join(dir, "segmented-"+name+".png"), gallerySegments(), 688, scale)
			shoot(t, filepath.Join(dir, "controls-"+name+".png"), galleryControls(), 688, scale)
			shoot(t, filepath.Join(dir, "inputs-"+name+".png"), galleryInputs(), 688, scale)
		}
	}
}

func shoot(t *testing.T, path string, content fyne.CanvasObject, width, scale float32) {
	t.Helper()
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetScale(scale)
	c.SetContent(content)
	for range 3 { // wrapped text settles its height after the first layout
		c.Resize(fyne.NewSize(width, content.MinSize().Height))
	}
	img := c.Capture()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
}

func spaced(top float32, object fyne.CanvasObject) fyne.CanvasObject {
	return container.New(insetLayout{top: top}, object)
}

func pageBox(objects ...fyne.CanvasObject) fyne.CanvasObject {
	return container.New(columnLayout{}, container.New(insetLayout{top: 15, bottom: 32}, container.New(tightLayout{}, objects...)))
}

func gallerySwitch(on, disabled bool) *Switch {
	s := NewSwitch(nil)
	s.Checked = on
	if disabled {
		s.Disable()
	}
	return s
}

var (
	galleryModes4 = []string{"Off", "Transparency", "Adaptive", "Noise Cancellation"}
	galleryModes3 = []string{"Transparency", "Adaptive", "Noise Cancellation"}
	galleryIcons  = map[string]int{"Off": iconOff, "Transparency": iconTransparency, "Adaptive": iconAdaptive, "Noise Cancellation": iconCancellation}
)

func gallerySegmented(options []string, selected string, disabled bool) *Segmented {
	s := NewSegmented(options, galleryIcons, nil)
	s.Selected = selected
	if disabled {
		s.Disable()
	}
	return s
}

// galleryPage repeats the reference composition from the kit's parts.
func galleryPage() fyne.CanvasObject {
	header := container.NewStack(
		container.New(insetLayout{top: 0}, newText("AirPods Pro", textTitle, true, theme.ColorNameForeground, fyne.TextAlignCenter)),
		container.NewHBox(layoutSpacer(), newOverflowButton(nil)),
	)
	status := newStatusDot("Connected", colorGreen)

	left, right, box := newBatteryColumn(artLeft, "Left"), newBatteryColumn(artRight, "Right"), newBatteryColumn(artCase, "Case")
	left.show(86, "86%", "", false)
	right.show(82, "82%", "", false)
	box.show(74, "74%", "", false)
	batteries := container.NewGridWithColumns(3, left.box, right.box, box.box)

	mode := gallerySegmented(galleryModes3, "Adaptive", false)

	on := gallerySwitch(true, false)
	audio := card(newStack(
		newOptionRow("Conversation Awareness", "", gallerySwitch(false, false)),
		newOptionRow("Personalized Volume", "", on),
		newOptionRow(`Allow "Off" mode`, "", gallerySwitch(false, false)),
	))

	slider := NewSlider(0, 100)
	slider.Value = 42
	volume := newSliderRow("", "", iconSpeaker, slider)
	volume.SetValue("42%")

	eqBody := newStack(newSelectRow("Preset", "", gallerySelect("Recommended sound")))
	equalizer := newSection("Equalizer", "Recommended sound", eqBody)
	microphone := newSection("Microphone", "", widget.NewLabel("body"))

	return pageBox(
		header, spaced(11, status), spaced(42, batteries),
		spaced(4, caption("Listening Mode")), mode,
		spaced(sectionGap, caption("Audio")), audio,
		spaced(sectionGap, caption("Volume")), card(volume),
		spaced(14, equalizer), spaced(14, microphone),
	)
}

func gallerySelect(selected string) *widget.Select {
	sel := widget.NewSelect([]string{"Recommended sound", "Flat", "Bass boost"}, nil)
	sel.Selected = selected
	return sel
}

// galleryParts lists artwork, gauges, icons and the indicator parts.
func galleryParts() fyne.CanvasObject {
	art := func(kind int, dim bool) fyne.CanvasObject {
		c := newBatteryColumn(kind, map[int]string{artLeft: "Left", artRight: "Right", artCase: "Case"}[kind])
		if dim {
			c.show(40, "40%", "2 hours ago", true)
		} else {
			c.show(90, "90%", "Charging", false)
		}
		return c.box
	}
	column := func(level int, value string) fyne.CanvasObject {
		ring := imageOf(func(w, h int) image.Image { return ringImage(w, h, level) }, batteryRing)
		return container.NewVBox(container.NewCenter(ring), newText(value, textLabel, false, colorSecondary, fyne.TextAlignCenter))
	}
	var iconsRow []fyne.CanvasObject
	for _, kind := range []int{iconOff, iconTransparency, iconAdaptive, iconCancellation, iconSpeaker, iconDots, iconStrengthLow, iconStrengthHigh} {
		iconsRow = append(iconsRow, container.NewCenter(imageOf(func(w, h int) image.Image {
			return iconImage(kind, w, h, look(theme.ColorNameForeground))
		}, 28)))
	}
	unknown := newBatteryColumn(artLeft, "Left")
	return pageBox(
		caption("Artwork, live and dimmed"),
		container.NewGridWithColumns(3, art(artLeft, false), art(artRight, false), art(artCase, false)),
		container.NewGridWithColumns(3, art(artLeft, true), art(artRight, true), art(artCase, true)),
		spaced(8, caption("Unknown reading")),
		container.NewGridWithColumns(3, unknown.box),
		spaced(8, caption("Gauges")),
		container.NewGridWithColumns(8, column(-1, "unknown"), column(0, "0"), column(5, "5"), column(20, "20"),
			column(50, "50"), column(86, "86"), column(99, "99"), column(100, "100")),
		spaced(12, caption("Icons")),
		card(container.New(insetLayout{top: 12, bottom: 12}, container.NewGridWithColumns(8, iconsRow...))),
		spaced(12, caption("Status and overflow")),
		card(newStack(
			newOptionRow("Connected", "", newStatusDot("Connected", colorGreen)),
			newOptionRow("Connecting", "", newStatusDot("Connecting", theme.ColorNameWarning)),
			newOptionRow("Offline", "", newStatusDot("Dongle not found", colorRed)),
			newOptionRow("Plain", "", newStatusDot("No dot", "")),
			newOptionRow("Overflow", "", newOverflowButton(nil)),
		)),
	)
}

// gallerySegments lists the segmented control in every state.
func gallerySegments() fyne.CanvasObject {
	strong := gallerySegmented(galleryModes3, "Adaptive", false)
	strong.strength = 85
	narrow := sized(380, gallerySegmented(galleryModes3, "Adaptive", false))
	return pageBox(
		caption("Segmented"),
		gallerySegmented(galleryModes3, "Transparency", false), spaced(6, gallerySegmented(galleryModes3, "Adaptive", false)),
		spaced(6, strong), spaced(6, gallerySegmented(galleryModes3, "Noise Cancellation", false)),
		spaced(6, gallerySegmented(galleryModes4, "Off", false)), spaced(6, gallerySegmented(galleryModes4, "Adaptive", false)),
		spaced(6, gallerySegmented(galleryModes3, "", false)), spaced(6, gallerySegmented(galleryModes3, "Adaptive", true)),
		spaced(6, container.NewHBox(narrow)),
	)
}

// galleryControls lists every control in every state.
func galleryControls() fyne.CanvasObject {
	focused := gallerySwitch(true, false)
	focused.focused = true

	sliderAt := func(v float64, disabled bool) *Slider {
		s := NewSlider(0, 100)
		s.Value = v
		if disabled {
			s.Disable()
		}
		return s
	}
	volume := newSliderRow("", "", iconSpeaker, sliderAt(42, false))
	volume.SetValue("42%")
	titled := newSliderRow("Adaptive strength", "How much outside sound Adaptive mode lets through.", -1, sliderAt(70, false))
	titled.SetValue("70")
	dimmed := newSliderRow("Tone volume", "", -1, sliderAt(0, true))
	unknown := sliderAt(0, false)
	unknown.SetUnknown(true)
	unreported := newSliderRow("Case volume", "", -1, unknown)

	unselected := gallerySelect("")
	unselected.PlaceHolder = "Choose"
	disabled := gallerySelect("Flat")
	disabled.Disable()

	return pageBox(
		caption("Switches"),
		card(newStack(
			newOptionRow("On", "", gallerySwitch(true, false)),
			newOptionRow("Off", "", gallerySwitch(false, false)),
			newOptionRow("On, disabled", "Not available in this mode", gallerySwitch(true, true)),
			newOptionRow("Off, disabled", "", gallerySwitch(false, true)),
			newOptionRow("Focused", "", focused),
		)),
		spaced(12, caption("Rows")),
		card(newStack(
			newOptionRow("Value", "", newValueText("Firmware 8A356")),
			newOptionRow("Unknown value", "", newValueText("")),
			newOptionRow("Row with a hint", "One small grey line under the title.", newValueText("12")),
			newOptionRow("A long title that has to wrap when the window is narrow, because the control keeps its size and the title takes what is left over", "", gallerySwitch(false, false)),
		)),
		spaced(12, caption("Sliders")),
		card(newStack(volume, titled, dimmed, unreported)),
		spaced(12, caption("Selects")),
		card(newStack(
			newSelectRow("Selected", "", gallerySelect("Flat")),
			newSelectRow("Nothing chosen", "With a hint below.", unselected),
			newSelectRow("Disabled", "", disabled),
		)),
	)
}

// galleryInputs lists sections and the native inputs as the theme leaves them.
func galleryInputs() fyne.CanvasObject {
	open := newSection("Equalizer", "Recommended sound", newStack(
		newOptionRow("Spatial audio", "", gallerySwitch(true, false)),
		newSelectRow("Preset", "", gallerySelect("Flat")),
	))
	open.Toggle()

	entry := widget.NewEntry()
	entry.SetText("Alex's AirPods")
	placeholder := widget.NewEntry()
	placeholder.SetPlaceHolder("Name")
	button := widget.NewButton("Repair", nil)
	primary := widget.NewButton("Connect", nil)
	primary.Importance = widget.HighImportance
	check := widget.NewCheck("Native check", nil)
	check.Checked = true

	return pageBox(
		caption("Sections"),
		newSection("Closed", "", widget.NewLabel("x")),
		spaced(10, newSection("Closed, subtitle", "Recommended sound", widget.NewLabel("x"))),
		spaced(10, open),
		spaced(12, caption("Dropdown menu")),
		container.NewGridWrap(fyne.NewSize(200, 170), widget.NewMenu(fyne.NewMenu("", fyne.NewMenuItem("Recommended sound", nil), fyne.NewMenuItem("Flat", nil), fyne.NewMenuItem("Bass boost", nil)))),
		spaced(12, caption("Native inputs under the theme")),
		card(newStack(
			container.New(insetLayout{top: 12, bottom: 12}, entry),
			container.New(insetLayout{top: 12, bottom: 12}, placeholder),
			newOptionRow("Buttons", "", container.NewHBox(button, primary)),
			newOptionRow("Check", "", check),
		)),
	)
}

func layoutSpacer() fyne.CanvasObject { return layout.NewSpacer() }

func imageOf(generate func(w, h int) image.Image, side float32) fyne.CanvasObject {
	raster := canvas.NewRaster(generate)
	raster.SetMinSize(fyne.NewSquareSize(side))
	return raster
}
