package ui

import (
	"strings"
	"testing"
)

func TestDiagnosticsShowReportAges(t *testing.T) {
	r := newRig(t, ownerConfig())
	r.shown()
	r.experiments(true)
	r.main(func() {
		r.w.diagnosticsRow.fold.Toggle()
		r.w.diag.read()
	})
	r.until(func() bool { return strings.Contains(r.w.diag.reports.Text, "report age 900 ms") }, "ages")
	r.main(func() {
		if !strings.Contains(r.w.diag.reports.Text, "mode: adaptive (report age 900 ms)") {
			t.Errorf("%q", r.w.diag.reports.Text)
		}
		if !strings.Contains(r.w.diag.reports.Text, ": not reported") {
			t.Errorf("unreported settings are missing: %q", r.w.diag.reports.Text)
		}
	})
}
