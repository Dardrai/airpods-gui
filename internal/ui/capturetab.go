package ui

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/internal/capture"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

// captureDurations are the lengths offered, in seconds, all within capture.MaxSeconds.
var captureDurations = []int{10, 30, 60, 120, 300, 600, 1800}

const defaultCaptureSeconds = 60

// captureShutdownWait bounds how long quitting waits for inspection to be turned off.
const captureShutdownWait = 2 * time.Second

// captureControls is the capture part of Diagnostics. It runs on the main thread, and the
// capture itself runs off it, with one short owner job per sample.
type captureControls struct {
	w       *Window
	content fyne.CanvasObject

	duration *widget.Select
	seconds  int
	trace    *Switch
	inspect  *Switch
	phone    *Switch
	radio    *Switch
	start    *widget.Button
	stopIt   *widget.Button
	status   *widget.Label
	box      fyne.CanvasObject

	stop     chan struct{}
	finished chan struct{}
}

func newCaptureControls(w *Window) *captureControls {
	t := &captureControls{w: w, seconds: defaultCaptureSeconds}
	t.duration = widget.NewSelect(nil, t.durationChanged)
	w.bind(t.relabelDurations)
	t.trace = NewSwitch(func(bool) {})
	t.trace.Checked = true
	t.inspect = NewSwitch(func(bool) {})
	t.phone = NewSwitch(func(bool) {})
	t.radio = NewSwitch(func(bool) { t.updateEnabled() })
	t.start = w.button("capture.start", t.begin)
	t.stopIt = w.button("capture.stop", t.requestStop)
	t.stopIt.Hide()
	t.status = smallLabel("")
	t.box = tight(t.status)
	t.box.Hide()
	t.content = container.NewVBox(
		w.subCaption("capture.title"),
		tight(w.small("capture.help")),
		w.selectRow("capture.duration", "", t.duration),
		w.optionRow("capture.trace", "capture.traceHint", t.trace),
		w.optionRow("capture.inspect", "capture.inspectHint", t.inspect),
		w.optionRow("capture.phone", "capture.phoneHint", t.phone),
		w.optionRow("capture.radio", "capture.radioHint", t.radio),
		t.box,
		container.NewHBox(layout.NewSpacer(), t.stopIt, t.start),
	)
	return t
}

func durationLabel(seconds int) string {
	if seconds < 120 {
		return fmt.Sprintf(i18n.T("capture.seconds"), seconds)
	}
	return fmt.Sprintf(i18n.T("capture.minutes"), seconds/60)
}

func (t *captureControls) relabelDurations() {
	labels := make([]string, len(captureDurations))
	for i, seconds := range captureDurations {
		labels[i] = durationLabel(seconds)
	}
	t.duration.OnChanged = nil
	t.duration.SetOptions(labels)
	t.duration.SetSelected(durationLabel(t.seconds))
	t.duration.OnChanged = t.durationChanged
}

func (t *captureControls) durationChanged(label string) {
	for _, seconds := range captureDurations {
		if durationLabel(seconds) == label {
			t.seconds = seconds
		}
	}
}

func (t *captureControls) running() bool { return t.finished != nil }

func (t *captureControls) updateEnabled() {
	idle := !t.running()
	st := t.w.state
	setEnabled(t.start, idle && st.Present && st.Status.Has(dongle.CapTypedRPC))
	enableWidget(t.duration, idle)
	enableWidget(t.radio, idle)
	for _, option := range []*Switch{t.trace, t.inspect, t.phone} {
		enableWidget(option, idle && !t.radio.Checked)
	}
	showIf(t.stopIt, !idle)
	showIf(t.start, idle)
}

func (t *captureControls) say(text string) {
	setStatus(t.box, t.status, text)
	t.w.relayout()
}

// begin asks where to save, then starts. Nothing touches the device until a file is chosen.
func (t *captureControls) begin() {
	if t.running() {
		return
	}
	name := "airpods-capture-" + t.w.now().Format("20060102-150405") + ".jsonl"
	t.w.pickFile(name, func(file io.WriteCloser, err error) {
		switch {
		case err != nil:
			t.say(fmt.Sprintf(i18n.T("capture.failed"), err.Error()))
		case file != nil:
			t.run(file)
		}
	})
}

func (t *captureControls) options() capture.Options {
	return capture.Options{
		Seconds: t.seconds, Trace: t.trace.Checked, Inspect: t.inspect.Checked,
		Phone: t.phone.Checked, Radio: t.radio.Checked,
	}
}

func (t *captureControls) run(file io.WriteCloser) {
	options := t.options()
	stop, finished := make(chan struct{}), make(chan struct{})
	t.stop, t.finished = stop, finished
	t.updateEnabled()
	t.w.about.updateDongle()
	t.say(fmt.Sprintf(i18n.T("capture.running"), 0, options.Seconds, 0))
	go func() {
		started := time.Now()
		result := capture.Run(t.submit, dongle.RealClock, file, options, stop, func(progress capture.Result) {
			elapsed := int(time.Since(started).Seconds())
			lines := progress.Lines
			t.w.do(func() {
				if t.finished == finished && !isClosed(stop) {
					t.say(fmt.Sprintf(i18n.T("capture.running"), elapsed, options.Seconds, lines))
				}
			})
		})
		if err := file.Close(); err != nil && result.Err == nil {
			result.Err = err
		}
		close(finished) // Shutdown waits for this, so it comes before anything needing the UI.
		t.w.do(func() { t.ended(result) })
	}()
}

// submit runs one capture job on the owner goroutine and waits for it.
func (t *captureControls) submit(job func(*dongle.Client) error) error {
	result := make(chan error, 1)
	t.w.owner.Submit(job, func(err error) { result <- err })
	return <-result
}

func isClosed(stop <-chan struct{}) bool {
	select {
	case <-stop:
		return true
	default:
		return false
	}
}

func (t *captureControls) requestStop() {
	if t.running() && !isClosed(t.stop) {
		close(t.stop)
		t.say(i18n.T("capture.stopping"))
	}
}

// shutdown stops a running capture and gives it a moment to turn inspection off, while the
// owner can still serve that job.
func (t *captureControls) shutdown() {
	if !t.running() {
		return
	}
	t.requestStop()
	select {
	case <-t.finished:
	case <-time.After(captureShutdownWait):
	}
}

func (t *captureControls) ended(result capture.Result) {
	t.stop, t.finished = nil, nil
	if t.w.closed {
		return
	}
	var text string
	switch {
	case errors.Is(result.Err, device.ErrNoDongle):
		text = fmt.Sprintf(i18n.T("capture.failedLines"), i18n.T("state.noDongle"), result.Lines)
	case result.Err != nil:
		text = fmt.Sprintf(i18n.T("capture.failedLines"), device.Explain(result.Err), result.Lines)
	case result.Stopped:
		text = fmt.Sprintf(i18n.T("capture.stopped"), result.Lines)
	default:
		text = fmt.Sprintf(i18n.T("capture.done"), result.Lines)
	}
	if result.Warning != "" {
		text += "\n" + i18n.T("capture.inspectStill")
	}
	t.updateEnabled()
	t.w.about.updateDongle()
	t.say(text)
}

func (w *Window) pickFileDialog(name string, chosen func(io.WriteCloser, error)) {
	d := dialog.NewFileSave(func(file fyne.URIWriteCloser, err error) {
		if file == nil {
			chosen(nil, err)
			return
		}
		chosen(file, err)
	}, w.win)
	d.SetFileName(name)
	d.SetFilter(storage.NewExtensionFileFilter([]string{filepath.Ext(name)}))
	d.Resize(fyne.NewSize(600, 500))
	d.Show()
}

// openFileDialog asks for a JSON file to read.
func (w *Window) openFileDialog(chosen func(io.ReadCloser, error)) {
	d := dialog.NewFileOpen(func(file fyne.URIReadCloser, err error) {
		if file == nil {
			chosen(nil, err)
			return
		}
		chosen(file, err)
	}, w.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".json"}))
	d.Resize(fyne.NewSize(600, 500))
	d.Show()
}
