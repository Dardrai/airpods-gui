package ui

import (
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/hearing"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

// openEverything has every section open and everything read, the widest the page gets.
func openEverything(t *testing.T, r *rig) {
	t.Helper()
	r.main(func() {
		r.w.about.experimental.SetChecked(true)
		r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348", Serial: "SIMSERIAL123"})
		r.w.applyExperimental()
	})
	r.until(func() bool { return r.w.state.Ready() }, "ready")
	r.main(r.w.hearing.readAll)
	r.until(func() bool { return r.w.hearing.have && !r.w.hearingBusy }, "hearing read")
	r.main(r.w.extra.readTransparency)
	r.until(func() bool { return r.w.extra.transBase != nil && !r.w.hearingBusy }, "transparency read")
	r.main(func() {
		for _, row := range r.w.sections {
			row.fold.Toggle()
		}
		r.w.hearing.say(i18n.T("hearing.imported"))
		r.w.relayout()
	})
}

// TestEveryLanguageFitsWithEverythingOpen is the width limit for what Hearing, Custom Transparency,
// Diagnostics and the panel added.
func TestEveryLanguageFitsWithEverythingOpen(t *testing.T) {
	for _, code := range []string{i18n.English, i18n.Portuguese, i18n.French, i18n.Spanish, i18n.German} {
		t.Run(code, func(t *testing.T) {
			r := newRig(t, sim.Demo())
			defer r.main(func() { i18n.SetLanguage(i18n.English) })
			r.main(func() { i18n.SetLanguage(code) })
			r.shown()
			openEverything(t, r)
			r.main(func() {
				got := r.w.win.Content().MinSize().Width
				t.Logf("%s: page %v", code, got)
				if got > defaultWidth {
					t.Errorf("minimum width %v", got)
				}
				panel := r.w.menu.content.MinSize().Width
				t.Logf("%s: panel %v", code, panel)
				if panel > overflowWidth-2*overflowPad {
					t.Errorf("the panel needs %v", panel)
				}
				for _, row := range []fyne.CanvasObject{r.w.hearing.content, r.w.extra.content, r.w.diag.content} {
					if width := row.MinSize().Width; width > defaultWidth {
						t.Errorf("a section needs %v", width)
					}
				}
			})
		})
	}
}

func TestDialogsScrollALongList(t *testing.T) {
	if _, long := dialogBody("a\nb").(interface{ ScrollToTop() }); long {
		t.Error("a short text does not need to scroll")
	}
	lines := catalogText("A3063")
	if _, long := dialogBody(lines).(interface{ ScrollToTop() }); !long {
		t.Error("the catalog must scroll")
	}
}

// TestRenderGapScreens saves what the parity work added when AIRPODS_SCREENS_DIR is set.
func TestRenderGapScreens(t *testing.T) {
	dir := os.Getenv("AIRPODS_SCREENS_DIR")
	if dir == "" {
		t.Skip("AIRPODS_SCREENS_DIR not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{i18n.English, i18n.German} {
		renderGaps(t, code, filepath.Join(dir, "gaps-"+code+"-"))
	}
}

func renderGaps(t *testing.T, code, prefix string) {
	t.Helper()
	r := startRig(t, sim.Demo(), ThemeDark)
	defer r.main(func() { i18n.SetLanguage(i18n.English) })
	r.until(func() bool { return r.w.state.Present }, "dongle present")
	r.main(func() {
		i18n.SetLanguage(code)
		r.w.win.Resize(fyne.NewSize(defaultWidth, defaultHeight))
		r.w.Show()
	})
	r.until(func() bool { return r.w.settings.byName["mode"].reported && r.w.audio.have }, "first reads")
	openEverything(t, r)
	shot := func(name string) {
		t.Helper()
		r.main(func() { savePNG(t, prefix+name+".png", r.w) })
	}
	at := func(content fyne.CanvasObject) {
		r.main(func() {
			r.w.relayout()
			pos, ok := positionOf(r.w.page, content, fyne.Position{})
			if !ok {
				t.Error("not on the page")
			}
			r.w.scroll.ScrollToOffset(fyne.NewPos(0, pos.Y))
		})
	}
	at(r.w.hearing.content)
	shot("hearing")
	at(r.w.hearing.apply)
	shot("hearing-profile")
	at(r.w.extra.content)
	shot("transparency")
	at(r.w.diag.copyHearing)
	shot("diagnostics")
	r.main(func() { r.w.scroll.ScrollToOffset(fyne.NewPos(0, 0)) })

	asked := make(chan struct{}, 1)
	var title, body, accept string
	dialogShot := func(name string) {
		t.Helper()
		r.main(func() { r.w.confirmDialog(title, body, accept, func(bool) {}) })
		shot(name)
		r.main(func() { r.w.win.Canvas().Overlays().Remove(r.w.win.Canvas().Overlays().Top()) })
	}
	r.main(func() {
		r.w.confirm = func(a, b, c string, answer func(bool)) {
			title, body, accept = a, b, c
			answer(false)
			asked <- struct{}{}
		}
		r.w.hearing.changeMode(hearing.HearingAid, true)
	})
	<-asked
	r.until(func() bool { return !r.w.hearingBusy }, "first question")
	dialogShot("confirm-on")
	r.main(func() {
		h := r.w.hearing
		for band := range hearing.Bands {
			h.bands[0][band].SetText("40")
			h.bands[1][band].SetText("45")
		}
		h.tone.Slider.SetValue(0.5)
		h.amp.Slider.SetValue(0.75)
		h.applyDraft()
	})
	<-asked
	r.until(func() bool { return !r.w.hearingBusy }, "second question")
	dialogShot("confirm-apply")
	r.main(func() { r.w.informDialog(i18n.T("app.catalog"), catalogText(r.w.info.Model)) })
	shot("catalog")
	r.main(func() {
		r.w.win.Canvas().Overlays().Remove(r.w.win.Canvas().Overlays().Top())
		r.w.menu.open()
		r.w.menu.scroll.ScrollToBottom()
	})
	shot("panel")
	r.main(r.w.menu.close)
}
