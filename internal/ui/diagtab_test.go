package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
)

func TestDiagnosticsShowsCountersAndUnreported(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	if before := r.performanceReads(); before != 0 {
		t.Fatalf("read %d times while collapsed", before)
	}
	r.main(func() {
		for range diagnosticsPeriod {
			r.w.tick()
		}
	})
	r.flush()
	if got := r.performanceReads(); got != 0 {
		t.Fatalf("read %d times while collapsed", got)
	}
	r.main(func() {
		if r.w.diagnosticsRow.object.Visible() {
			t.Error("Diagnostics needs the experimental switch")
		}
	})
	r.experiments(true)
	r.main(func() { test.Tap(r.w.diagnosticsRow.fold.header) })
	r.until(func() bool { return r.w.diag.values["Uptime"].Text == "1h2m5s" }, "counters")
	r.main(func() {
		if got := r.w.diag.values["Buffer used"].Text; got != "960 of 4000" {
			t.Errorf("buffer: %q", got)
		}
	})
	if got := counterTexts(dongle.PerformanceState{})["Uptime"]; got != notReported() {
		t.Errorf("nil counter: %q", got)
	}
}
