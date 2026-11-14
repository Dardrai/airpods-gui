package ui

import (
	"image"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
	"github.com/Dardrai/airpods-gui/internal/tray"
)

const (
	sheetArt      float32 = 64
	sheetRing     float32 = 26
	sheetModeIcon float32 = 18
)

// sheetColumnGaps are columnGaps for the small picture: the same order, closer together.
var sheetColumnGaps = []float32{8, 6, 4, 0, 0}

// batterySheet is the Show batteries window: the top of the page in small. The name, a
// column per battery with the page's pictures, gauges and wording, and the listening mode.
type batterySheet struct {
	title     *widget.Label
	columns   [3]*batteryColumn
	shown     [3]reading
	headphone bool
	grid      *fyne.Container
	mode      *text
	modeIcon  *canvas.Raster
	modeKind  int
	modeRow   fyne.CanvasObject
	close     *widget.Button
	content   fyne.CanvasObject
}

// NewBatterySheet makes the content of the Show batteries window; close is its Close button.
func NewBatterySheet(close func()) tray.PopupContent {
	return newBatterySheet(close)
}

func newBatterySheet(close func()) *batterySheet {
	s := &batterySheet{title: sheetTitle(fallbackName), modeKind: -1}
	for i, kind := range []int{artLeft, artRight, artCase} {
		s.columns[i] = newBatteryColumnSized(kind, "", sheetArt, sheetColumnGaps)
		s.columns[i].ring.SetMinSize(fyne.NewSquareSize(sheetRing))
		s.shown[i] = reading{level: -1, percent: dashText, dim: true, artDim: true}
	}
	s.grid = container.NewGridWithColumns(3, s.columns[0].box, s.columns[1].box, s.columns[2].box)
	s.mode = newText("", textStatus, false, colorSecondary, fyne.TextAlignLeading)
	s.modeIcon = canvas.NewRaster(func(w, h int) image.Image {
		return iconImage(s.modeKind, w, h, look(colorSecondary))
	})
	s.modeIcon.SetMinSize(fyne.NewSquareSize(sheetModeIcon))
	s.modeRow = container.NewHBox(layout.NewSpacer(), container.NewCenter(s.modeIcon), s.mode, layout.NewSpacer())
	s.modeRow.Hide()
	s.close = sheetButton(i18n.T("popup.close"), false, close)
	body := container.New(tightLayout{}, container.New(insetLayout{top: 10}, s.grid), container.New(insetLayout{top: 6}, s.modeRow))
	s.content = WindowSheet(s.title, body, s.close)
	return s
}

// Object is what goes in the window.
func (s *batterySheet) Object() fyne.CanvasObject { return s.content }

// Update shows a state, in the current language.
func (s *batterySheet) Update(st device.State, view model.View) {
	setLabel(s.title, deviceName(view.Model))
	s.close.SetText(i18n.T("popup.close"))
	s.fitModel(view.Model)
	components := []int{tray.BatteryLeft, tray.BatteryRight, tray.BatteryCase}
	if s.headphone {
		// One battery, under whichever side reports it.
		components = []int{tray.BatteryRight}
		if tray.ReadBattery(st, tray.BatteryLeft).Known {
			components = []int{tray.BatteryLeft}
		}
	}
	for i, component := range components {
		next := batteryReading(tray.ReadBattery(st, component))
		next.artDim = !tray.ArtAvailable(st, component)
		if next != s.shown[i] {
			s.shown[i] = next
			s.columns[i].show(next.level, next.percent, next.note, next.dim)
			s.columns[i].setArtDim(next.artDim)
		}
	}
	s.showMode(st)
}

// fitModel gives the columns the model's pictures and names; over-ear models have one.
func (s *batterySheet) fitModel(number string) {
	set := artSetFor(number)
	profile, known := model.Find(number)
	headphone := known && profile.Headphone
	if headphone != s.headphone {
		s.headphone = headphone
		s.grid.Layout = layout.NewGridLayoutWithColumns(3)
		if headphone {
			s.grid.Layout = layout.NewGridLayoutWithColumns(1)
		}
		showIf(s.columns[1].box, !headphone)
		showIf(s.columns[2].box, !headphone)
		for i := range s.shown {
			s.shown[i].level = -2 // the columns read other batteries now: draw them again
		}
		s.grid.Refresh()
	}
	kinds, keys := []int{artLeft, artRight, artCase}, []string{"battery.left", "battery.right", "battery.case"}
	if headphone {
		kinds, keys = []int{artHeadphones}, []string{"battery.battery"}
	}
	for i, kind := range kinds {
		s.columns[i].setArt(set, kind)
		s.columns[i].label.SetText(i18n.T(keys[i]))
	}
}

// showMode is the listening mode with its icon; the line is left out when there is none.
func (s *batterySheet) showMode(st device.State) {
	name := tray.ModeName(st)
	showIf(s.modeRow, name != "")
	if name == "" {
		return
	}
	s.mode.SetText(name)
	for _, mode := range tray.Modes {
		if icon, ok := wordIcons[mode.Word]; ok && mode.Value == st.Status.ANC && icon != s.modeKind {
			s.modeKind = icon
			s.modeIcon.Refresh()
		}
	}
}
