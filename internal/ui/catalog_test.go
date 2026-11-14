package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

// TestCatalogMatchesTheCLI compares with `airpods info --capabilities` for the same model. The CLI
// says "this tool" where the app says "this app".
func TestCatalogMatchesTheCLI(t *testing.T) {
	want := `From the model catalog, not a report from these AirPods:
over-ear: yes
listening modes: yes
adaptive: no
allow-off option: no
high quality microphone: no
custom EQ: no
hearing assistance: no
custom transparency: no
loud sound reduction: no

Controls:
0x0a: ear-detection
0x0d: mode
0x1a: `
	got := catalogText("A2096")
	if !strings.HasPrefix(got, want) {
		t.Errorf("%s", got)
	}
	if !strings.Contains(catalogText("A3157"), "0x0d: mode") || strings.Count(catalogText("A3063"), ": yes") < 8 {
		t.Errorf("%s", catalogText("A3063"))
	}
	if got := catalogText("X9999"); got != i18n.T("app.catalogNotListed") {
		t.Errorf("%q", got)
	}
}

func TestCatalogButtonFollowsTheModel(t *testing.T) {
	r := newRig(t, sim.Demo())
	var title, text string
	r.main(func() {
		r.w.inform = func(a, b string) { title, text = a, b }
		r.w.SetInfo(dongle.Metadata{})
		if !r.w.about.catalog.Disabled() {
			t.Error("no model, yet it can be shown")
		}
		r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"})
		if r.w.about.catalog.Disabled() {
			t.Error("disabled with a model")
		}
		test.Tap(r.w.about.catalog)
		if title != i18n.T("app.catalog") || !strings.Contains(text, "hearing assistance: yes") {
			t.Errorf("%q %q", title, text)
		}
	})
}
