package ui

import (
	"testing"

	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

// TestEveryLanguageFitsTheDefaultWindow has all four mode chips showing, the widest row each
// language has, and Diagnostics open, first as the page is by default and then with the
// experimental switch on. The language changes on the rig's main thread, as it does in the app, since
// it retexts the window.
func TestEveryLanguageFitsTheDefaultWindow(t *testing.T) {
	for _, code := range []string{i18n.English, i18n.Portuguese, i18n.French, i18n.Spanish, i18n.German} {
		t.Run(code, func(t *testing.T) {
			cfg := ownerConfig()
			cfg.Settings = append(cfg.Settings, sim.Setting{Control: 0x34, Age: 900, Revision: 3, Value: 1})
			r := newRig(t, cfg)
			defer r.main(func() { i18n.SetLanguage(i18n.English) })
			r.main(func() { i18n.SetLanguage(code) })
			r.shown()
			r.until(settled(r), "settled")
			r.main(func() {
				r.w.relayout()
				if off := r.w.settings.byName["hold-modes"].chips[0]; !off.Visible() {
					t.Error("the Off chip is hidden")
				}
				if got := r.w.win.Content().MinSize().Width; got > defaultWidth {
					t.Errorf("minimum width %v", got)
				}
				r.w.about.experimental.SetChecked(true)
				r.w.diagnosticsRow.fold.Toggle()
				r.w.relayout()
				got := r.w.win.Content().MinSize().Width
				t.Logf("%s: minimum width %v", code, got)
				if got > defaultWidth {
					t.Errorf("minimum width with the experiments %v", got)
				}
			})
		})
	}
}
