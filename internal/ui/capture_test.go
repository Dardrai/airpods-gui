package ui

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/internal/capture"
)

// memoryFile stands in for the chosen file.
type memoryFile struct {
	mu     sync.Mutex
	data   bytes.Buffer
	closed bool
}

func (f *memoryFile) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.data.Write(p)
}

func (f *memoryFile) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *memoryFile) done() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func (f *memoryFile) lines() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Split(strings.TrimSpace(f.data.String()), "\n")
}

func captureConfig() sim.Config {
	cfg := sim.Demo()
	capture.AddSimReplies(&cfg)
	return cfg
}

func TestCaptureRunsStopsAndKeepsPollingAlive(t *testing.T) {
	r := newRig(t, captureConfig())
	r.shown()
	file := &memoryFile{}
	asked := ""
	r.main(func() {
		r.w.pickFile = func(name string, chosen func(io.WriteCloser, error)) {
			asked = name
			chosen(file, nil)
		}
		controls := r.w.diag.capture
		controls.seconds, controls.inspect.Checked = 1800, true
		controls.start.OnTapped()
		if !controls.running() || !controls.stopIt.Visible() || controls.start.Visible() {
			t.Error("not shown as running")
		}
	})
	if !strings.HasSuffix(asked, ".jsonl") {
		t.Errorf("name %q", asked)
	}
	reads := r.statusReads()
	r.until(func() bool { return r.dev.Inspecting() && r.statusReads() >= reads+10 }, "polling during the capture")
	r.main(func() { r.w.diag.capture.stopIt.OnTapped() })
	r.until(func() bool { return !r.w.diag.capture.running() }, "stopped")
	if r.dev.Inspecting() {
		t.Error("inspection left on")
	}
	if !file.done() {
		t.Error("file left open")
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(file.lines()[0]), &first); err != nil {
		t.Fatalf("%q: %v", file.lines()[0], err)
	}
	if _, ok := first["inspect"]; !ok {
		t.Errorf("%v", first)
	}
	r.main(func() {
		controls := r.w.diag.capture
		if !strings.Contains(controls.status.Text, "Stopped") || controls.start.Disabled() || !controls.start.Visible() {
			t.Errorf("status %q", controls.status.Text)
		}
	})
}

func TestCaptureCancelledDialogDoesNothing(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.main(func() {
		r.w.pickFile = func(_ string, chosen func(io.WriteCloser, error)) { chosen(nil, nil) }
		r.w.diag.capture.begin()
		if r.w.diag.capture.running() {
			t.Error("started with no file")
		}
	})
}

func TestRadioOnlyTurnsTheOtherOptionsOff(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.main(func() {
		controls := r.w.diag.capture
		controls.radio.SetChecked(true)
		if !controls.trace.Disabled() || !controls.inspect.Disabled() || !controls.phone.Disabled() {
			t.Error("options stay on with radio only")
		}
		controls.radio.SetChecked(false)
		if controls.trace.Disabled() {
			t.Error("options stay off")
		}
	})
}

func TestQuittingStopsACapture(t *testing.T) {
	r := newRig(t, captureConfig())
	file := &memoryFile{}
	r.main(func() {
		r.w.pickFile = func(_ string, chosen func(io.WriteCloser, error)) { chosen(file, nil) }
		controls := r.w.diag.capture
		controls.seconds, controls.inspect.Checked = 1800, true
		controls.begin()
	})
	r.until(func() bool { return r.dev.Inspecting() }, "inspecting")
	r.main(func() { r.w.Shutdown() })
	r.until(func() bool { return file.done() }, "closed")
	if r.dev.Inspecting() {
		t.Error("inspection left on")
	}
}

// TestRenderParityScreens saves the picker and capture controls as PNG when
// AIRPODS_SCREENS_DIR is set.
func TestRenderParityScreens(t *testing.T) {
	dir := os.Getenv("AIRPODS_SCREENS_DIR")
	if dir == "" {
		t.Skip("AIRPODS_SCREENS_DIR not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var chosen []string
	cfg := ownerConfig()
	capture.AddSimReplies(&cfg)
	r := newRigWith(t, cfg, controlled(&chosen))
	r.main(func() { r.w.win.Resize(fyne.NewSize(defaultWidth, defaultHeight)) })
	r.shown()
	r.until(settled(r), "settled")
	shot := func(name string) {
		t.Helper()
		r.main(func() {
			r.w.relayout()
			savePNG(t, filepath.Join(dir, name+".png"), r.w)
		})
	}
	r.main(func() {
		r.w.state.Several = true
		r.w.Update(r.w.state)
		r.w.SetDongles(dongleList())
		r.w.menu.open()
	})
	shot("picker")
	r.main(func() {
		r.w.menu.close()
		r.w.about.experimental.SetChecked(true)
		r.w.diagnosticsRow.fold.Toggle()
		r.w.diag.read()
	})
	r.until(func() bool { return strings.Contains(r.w.diag.reports.Text, "report age") }, "ages")
	r.main(func() {
		r.w.relayout()
		r.w.scroll.ScrollToOffset(fyne.NewPos(0, r.w.diagnosticsRow.object.Position().Y+1100))
	})
	shot("capture")
	file := &memoryFile{}
	r.main(func() {
		r.w.pickFile = func(_ string, chosen func(io.WriteCloser, error)) { chosen(file, nil) }
		test.Tap(r.w.diag.capture.start)
		r.w.relayout()
		r.w.scroll.ScrollToOffset(fyne.NewPos(0, r.w.page.MinSize().Height))
	})
	r.until(func() bool {
		return strings.Contains(r.w.diag.capture.status.Text, "Lines saved: 1") || strings.Contains(r.w.diag.capture.status.Text, "Lines saved: 2")
	}, "progress")
	shot("capture-running")
	r.main(func() { r.w.diag.capture.stopIt.OnTapped() })
	r.until(func() bool { return !r.w.diag.capture.running() }, "stopped")
}
