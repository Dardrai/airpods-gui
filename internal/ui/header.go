package ui

import (
	"fmt"
	"image"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
	"github.com/Dardrai/airpods-gui/internal/icon"
	"github.com/Dardrai/airpods-gui/internal/tray"
)

const fallbackName = "AirPods"

// reading is what a battery column last showed; it is redrawn only when this changes.
type reading struct {
	level         int
	percent, note string
	dim           bool
	artDim        bool
}

// pageHeader is the top of the page: the name, the connection state and the batteries.
type pageHeader struct {
	name      *text
	status    *statusDot
	detail    *widget.Label
	detailBox fyne.CanvasObject
	overflow  *overflowButton
	columns   [3]*batteryColumn
	shown     [3]reading
	headphone bool
	settling  bool // set by the window before update
	batteries *fyne.Container
	// noDongle stands in for the batteries while there is no dongle: the tray's outline
	// glyph, rather than AirPods and empty rings that look like something is loading.
	noDongle *canvas.Raster
	content  fyne.CanvasObject
}

// noDongleArt is the size of the glyph that replaces the batteries.
const noDongleArt = 96

func newPageHeader(openOverflow func()) *pageHeader {
	h := &pageHeader{}
	h.name = newText(fallbackName, textTitle, true, theme.ColorNameForeground, fyne.TextAlignCenter)
	h.status = newStatusDot("", colorDim)
	h.detail = smallLabel("")
	h.detail.Alignment = fyne.TextAlignCenter
	h.overflow = newOverflowButton(openOverflow)
	h.detailBox = tight(h.detail)
	h.detailBox.Hide()
	top := container.NewStack(h.name, container.NewHBox(layout.NewSpacer(), h.overflow))
	h.columns = [3]*batteryColumn{
		newBatteryColumn(artLeft, i18n.T("battery.left")), newBatteryColumn(artRight, i18n.T("battery.right")), newBatteryColumn(artCase, i18n.T("battery.case")),
	}
	for i := range h.shown {
		// As a new column draws itself, so that the first update changes what differs.
		h.shown[i] = reading{level: -1, percent: dashText, dim: true, artDim: true}
	}
	h.batteries = container.NewGridWithColumns(3, h.columns[0].box, h.columns[1].box, h.columns[2].box)
	h.noDongle = canvas.NewRaster(func(w, _ int) image.Image {
		return icon.Render(icon.Variant{Level: icon.NoBadge, NoDongle: true, LightPanel: isLight()}, w)
	})
	h.noDongle.SetMinSize(fyne.NewSquareSize(noDongleArt))
	h.noDongle.Hide()
	h.content = container.New(tightLayout{}, top,
		container.New(insetLayout{top: 11}, h.status), h.detailBox,
		container.New(insetLayout{top: 42}, container.NewStack(h.batteries, container.NewCenter(h.noDongle))))
	return h
}

// deviceName is the model's name, or the plain fallback while it isn't known.
func deviceName(number string) string {
	if profile, ok := model.Find(number); ok {
		return profile.Name
	}
	return fallbackName
}

// connectionText is the line under the name, and the smaller one below it when the page
// is waiting for something.
func connectionText(st device.State) (string, string) {
	status, detail := connectionWords(st)
	if st.Several && st.Present && st.Serial != "" {
		which := fmt.Sprintf(i18n.T("page.whichDongle"), st.Serial)
		if detail == "" {
			return status, which
		}
		return status, detail + " · " + which
	}
	return status, detail
}

func connectionWords(st device.State) (string, string) {
	switch {
	case st.ChooseDongle:
		return i18n.T("state.chooseDongle"), i18n.T("page.chooseDongle")
	case st.Problem != "":
		return i18n.T("state.problem"), st.Problem
	case !st.Present:
		return i18n.T("state.noDongle"), i18n.T("page.plugIn")
	case !st.Status.Connected:
		return i18n.T("state.disconnect"), i18n.T("page.waiting")
	case !st.Ready():
		return i18n.T("state.connected"), i18n.T("page.waiting")
	}
	return i18n.T("state.connected"), ""
}

// relabel puts the current language on the fixed names; update does the rest.
func (h *pageHeader) relabel() {
	for i, key := range []string{"battery.left", "battery.right", "battery.case"} {
		if i > 0 || !h.headphone {
			h.columns[i].label.SetText(i18n.T(key))
		}
	}
}

func (h *pageHeader) update(st device.State, number string) {
	h.name.SetText(deviceName(number))
	status, detail := connectionText(st)
	if h.settling {
		// Connected, and the AirPods have not reported a single setting yet.
		detail = strings.TrimSuffix(i18n.T("page.waiting")+" · "+detail, " · ")
	}
	fill := colorDim
	if st.Connected() {
		fill = colorGreen
	}
	if status != h.status.value || fill != h.status.fill {
		h.status.Set(status, fill)
	}
	setLabel(h.detail, detail)
	showIf(h.detailBox, detail != "")
	absent := !st.Present || st.ChooseDongle
	showIf(h.batteries, !absent)
	showIf(h.noDongle, absent)
	h.fitModel(number)
	for i, component := range h.components(st) {
		next := batteryReading(tray.ReadBattery(st, component))
		next.artDim = !tray.ArtAvailable(st, component)
		if next != h.shown[i] {
			h.shown[i] = next
			h.columns[i].show(next.level, next.percent, next.note, next.dim)
			h.columns[i].setArtDim(next.artDim)
		}
	}
}

// components is what each column reads: over-ear models have one battery, under whichever
// side reports it, and no case.
func (h *pageHeader) components(st device.State) []int {
	if !h.headphone {
		return []int{tray.BatteryLeft, tray.BatteryRight, tray.BatteryCase}
	}
	if tray.ReadBattery(st, tray.BatteryLeft).Known {
		return []int{tray.BatteryLeft}
	}
	return []int{tray.BatteryRight}
}

// fitModel gives the columns the model's own pictures.
func (h *pageHeader) fitModel(number string) {
	set := artSetFor(number)
	profile, known := model.Find(number)
	headphone := known && profile.Headphone
	if headphone != h.headphone {
		h.batteries.Layout = layout.NewGridLayoutWithColumns(3)
		if headphone {
			h.batteries.Layout = layout.NewGridLayoutWithColumns(1)
		}
		defer h.batteries.Refresh()
	}
	h.headphone = headphone
	if h.headphone {
		h.columns[0].setArt(set, artHeadphones)
	} else {
		h.columns[0].setArt(set, artLeft)
		h.columns[1].setArt(set, artRight)
		h.columns[2].setArt(set, artCase)
	}
	showIf(h.columns[1].box, !h.headphone)
	showIf(h.columns[2].box, !h.headphone)
	key := "battery.left"
	if h.headphone {
		key = "battery.battery"
	}
	h.columns[0].label.SetText(i18n.T(key))
}

// batteryReading applies the page's wording: a charging note, the age of an old reading,
// and a dimmed column with a dash when there is no reading at all.
func batteryReading(r tray.Reading) reading {
	switch {
	case !r.Known:
		return reading{level: -1, percent: dashText, dim: true}
	case r.Charging:
		return reading{level: r.Level, percent: fmt.Sprintf("%d%%", r.Level), note: i18n.T("page.charging")}
	case !r.Current && r.HasAge:
		return reading{level: r.Level, percent: fmt.Sprintf("%d%%", r.Level), note: ageText(r.AgeMs), dim: true}
	case !r.Current:
		return reading{level: r.Level, percent: fmt.Sprintf("%d%%", r.Level), note: i18n.T("page.lastKnown"), dim: true}
	}
	return reading{level: r.Level, percent: fmt.Sprintf("%d%%", r.Level)}
}

// ageText rounds to minutes and hours, so an old reading doesn't redraw every second.
func ageText(ms uint32) string {
	minutes := ms / 60_000
	switch {
	case minutes < 1:
		return i18n.T("page.ageNow")
	case minutes < 60:
		return fmt.Sprintf(i18n.T("page.ageMinutes"), minutes)
	}
	return fmt.Sprintf(i18n.T("page.ageHours"), minutes/60)
}
