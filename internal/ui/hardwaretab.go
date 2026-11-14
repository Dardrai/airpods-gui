package ui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/hearing"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

type hardwareTab struct {
	w       *Window
	content fyne.CanvasObject

	refresh, readData         *widget.Button
	status, readAt            *widget.Label
	left, right               *widget.Label
	leftWhen, rightWhen       *widget.Label
	testMode, sensitivity     *widget.Label
	curve, dataStatus, dataAt *widget.Label
	statusBox, dataBox        fyne.CanvasObject
	record                    *hearing.Diagnostics
	busy                      bool
	gateNote                  bool // the status line is the gate's reason, not a result

	run, stopWait *widget.Button
	progress      *widget.ProgressBar
	progressBox   fyne.CanvasObject
	checkText     *widget.Label
	checkEars     *widget.Label
	checkStatus   fyne.CanvasObject
	checking      bool
	stop          *atomic.Bool
	last          *hearing.CheckProgress
	lastData      *hearing.DeviceData
}

func newHardwareTab(w *Window) *hardwareTab {
	t := &hardwareTab{w: w}
	t.readAt, t.left, t.right, t.leftWhen, t.rightWhen = newValue(dashText), newValue(dashText), newValue(dashText), newValue(dashText), newValue(dashText)
	t.testMode, t.sensitivity, t.curve, t.dataAt = newValue(dashText), newValue(dashText), newValue(dashText), newValue(dashText)
	t.status, t.dataStatus = smallLabel(""), smallLabel("")
	t.statusBox, t.dataBox = tight(t.status), tight(t.dataStatus)
	t.statusBox.Hide()
	t.dataBox.Hide()
	t.refresh = w.button("diag.refresh", t.readResults)
	t.readData = w.button("hardware.readData", t.readDeviceData)
	t.run = w.button("check.run", t.askCheck)
	t.stopWait = w.button("check.stopWaiting", t.stopCheck)
	t.stopWait.Hide()
	t.progress = widget.NewProgressBar()
	t.progress.TextFormatter = func() string {
		if t.last == nil {
			return ""
		}
		return fmt.Sprintf(i18n.T("check.secondsLeft"), int(t.last.Remaining.Seconds()))
	}
	t.checkText, t.checkEars = smallLabel(""), smallLabel("")
	t.progressBox = container.NewVBox(t.progress, tight(t.checkEars))
	t.progressBox.Hide()
	t.checkStatus = tight(t.checkText)
	t.checkStatus.Hide()
	t.content = container.NewVBox(
		tight(w.small("hardware.help")),
		t.statusBox,
		w.valueRow("battery.left", t.left),
		w.valueRow("hardware.measured", t.leftWhen),
		w.valueRow("battery.right", t.right),
		w.valueRow("hardware.measured", t.rightWhen),
		w.valueRow("hardware.readAt", t.readAt),
		buttonRight(t.refresh),
		w.subCaption("check.title"),
		tight(w.small("check.help")),
		t.progressBox,
		t.checkStatus,
		container.NewHBox(layout.NewSpacer(), t.stopWait, t.run),
		w.subCaption("hardware.deviceData"),
		tight(w.small("hardware.dataHelp")),
		t.dataBox,
		w.valueRow("hardware.testMode", t.testMode),
		w.valueRow("hardware.sensitivity", t.sensitivity),
		w.valueRow("hardware.curve", t.curve),
		w.valueRow("hardware.readAt", t.dataAt),
		buttonRight(t.readData),
		gap(4),
	)
	t.updateEnabled()
	w.texts = append(w.texts, func() {
		if t.gateNote {
			t.updateEnabled()
		}
		if t.last != nil {
			t.showCheck(*t.last)
		}
		if t.lastData != nil {
			t.showData(*t.lastData, nil)
		}
	})
	return t
}

// setStatus shows a line of the section, taking no room while it is empty.
func setStatus(box fyne.CanvasObject, label *widget.Label, value string) {
	setLabel(label, value)
	showIf(box, value != "")
}

// readable applies the read gates: connected, a supported model, no microphone in use,
// nothing else running.
func (t *hardwareTab) readable() (bool, string) {
	st := t.w.state
	switch {
	case !st.Ready():
		return false, i18n.T("hardware.connectFirst")
	case !hearing.Supported(t.w.model()):
		return false, i18n.T("hardware.notAvailable")
	case st.Status.MicrophoneActive:
		return false, i18n.T("hardware.stopRecording")
	case !st.Status.Has(dongle.CapLease):
		return false, i18n.T("hardware.updateFirmware")
	}
	return !t.busy && !t.w.hearingBusy, ""
}

func (t *hardwareTab) updateEnabled() {
	ok, reason := t.readable()
	setEnabled(t.refresh, ok)
	setEnabled(t.readData, ok)
	setEnabled(t.run, ok)
	if !ok && reason != "" {
		t.gateNote = true
		setStatus(t.statusBox, t.status, reason)
	} else if t.gateNote {
		t.gateNote = false
		setStatus(t.statusBox, t.status, "")
	}
}

// withSession runs one leased sequence on the owner goroutine.
func (w *Window) withSession(run func(*dongle.Client, *dongle.Session) error, done func(error)) {
	w.owner.SubmitCommand(func(c *dongle.Client) error {
		session, err := c.OpenSession()
		if err != nil {
			return err
		}
		defer session.Close()
		if err := session.AcquireLease(); err != nil {
			return err
		}
		return run(c, session)
	}, func(err error) { w.do(func() { done(err) }) })
}

func (t *hardwareTab) readResults() {
	if ok, _ := t.readable(); !ok {
		return
	}
	t.busy = true
	t.updateEnabled()
	number := t.w.model()
	var record *hearing.Diagnostics
	t.w.withSession(func(_ *dongle.Client, s *dongle.Session) (err error) {
		record, err = hearing.ReadDiagnostics(s.Exchange, number, time.Now())
		return err
	}, func(err error) {
		t.busy = false
		t.showResults(record, err)
		t.updateEnabled()
	})
}

func (t *hardwareTab) showResults(record *hearing.Diagnostics, err error) {
	t.gateNote = false
	switch {
	case err != nil:
		setStatus(t.statusBox, t.status, device.Explain(err))
		return
	case record == nil:
		setStatus(t.statusBox, t.status, i18n.T("hardware.noneSaved"))
		return
	}
	t.record = record
	setStatus(t.statusBox, t.status, "")
	t.left.SetText(record.Left.Text())
	t.right.SetText(record.Right.Text())
	t.leftWhen.SetText(hearing.When(record.Left.MeasuredAtSeconds))
	t.rightWhen.SetText(hearing.When(record.Right.MeasuredAtSeconds))
	t.readAt.SetText(record.ReadAt.Format("15:04:05"))
}

func (t *hardwareTab) readDeviceData() {
	if ok, _ := t.readable(); !ok {
		return
	}
	t.busy = true
	t.updateEnabled()
	var data hearing.DeviceData
	t.w.withSession(func(_ *dongle.Client, s *dongle.Session) (err error) {
		data, err = hearing.ReadDeviceData(s.Exchange, time.Now())
		return err
	}, func(err error) {
		t.busy = false
		t.showData(data, err)
		t.updateEnabled()
	})
}

func (t *hardwareTab) showData(data hearing.DeviceData, err error) {
	if err != nil {
		setStatus(t.dataBox, t.dataStatus, device.Explain(err))
		return
	}
	kept := data
	t.lastData = &kept
	setStatus(t.dataBox, t.dataStatus, "")
	if !data.AnyCharacteristic {
		setStatus(t.dataBox, t.dataStatus, i18n.T("hardware.noData"))
	}
	t.testMode.SetText(notReported())
	if data.TestMode != nil {
		t.testMode.SetText(strconv.Itoa(int(*data.TestMode)))
	}
	t.sensitivity.SetText(notReported())
	if data.Sensitivity != nil {
		t.sensitivity.SetText(strconv.FormatFloat(float64(*data.Sensitivity), 'g', 6, 32))
	}
	t.curve.SetText(notReported())
	if data.VolumeCurve != nil {
		parts := make([]string, len(data.VolumeCurve))
		for i, value := range data.VolumeCurve {
			parts[i] = fmt.Sprintf("%.3g", value)
		}
		t.curve.SetText(strings.Join(parts, "  "))
	}
	t.dataAt.SetText(data.ReadAt.Format("15:04:05"))
}

// askCheck is the one way into the in-case check: the user says where the buds are.
func (t *hardwareTab) askCheck() {
	if ok, _ := t.readable(); !ok {
		return
	}
	t.w.confirm(i18n.T("check.confirmTitle"), i18n.T("check.confirmText"), i18n.T("check.confirmAccept"), func(accepted bool) {
		if accepted {
			t.beginCheck()
		}
	})
}

// beginCheck sends the request once, in one short owner job.
func (t *hardwareTab) beginCheck() {
	t.w.setHearingBusy(true)
	t.checking = true
	stop := new(atomic.Bool)
	t.stop = stop
	t.last = nil
	setStatusObject(t.checkStatus, t.checkText, i18n.T("check.starting"))
	options := hearing.HardwareCheckOptions{BudsInCaseConfirmed: true, Stop: stop.Load, TolerateLinkLoss: true}
	var check *hearing.HardwareCheck
	var first hearing.CheckProgress
	t.w.owner.SubmitCommand(func(c *dongle.Client) error {
		if err := hearing.HardwareCheckReady(c, true); err != nil {
			return err
		}
		var err error
		check, first, err = hearing.StartHardwareCheck(c, options)
		return err
	}, func(err error) {
		if err != nil {
			t.w.do(func() { t.endCheck(first, err) })
			return
		}
		t.w.do(func() { t.showCheck(first) })
		go t.followCheck(check)
	})
}

// checkStep is one look at a running check.
type checkStep struct {
	progress hearing.CheckProgress
	done     bool
	err      error
	skipped  error // the owner did not run the step, for this reason
}

// followCheck waits off the UI thread and off the owner goroutine. Each look at the check is
// its own short owner job, so between looks the owner keeps polling status and serving its
// queue through the whole wait, up to 120 s, and the tray and page stay live.
func (t *hardwareTab) followCheck(check *hearing.HardwareCheck) {
	for {
		time.Sleep(t.w.checkEvery)
		result := make(chan checkStep, 1)
		t.w.owner.Submit(func(c *dongle.Client) error {
			progress, done, err := check.Step(c)
			result <- checkStep{progress: progress, done: done, err: err}
			return nil
		}, func(err error) {
			if err != nil {
				result <- checkStep{skipped: err}
			}
		})
		step := <-result
		if errors.Is(step.skipped, device.ErrStopped) {
			return // The app is going away, and the window with it.
		}
		if step.skipped != nil {
			// No device right now, or a full queue. The wait and Stop still count.
			step.progress, step.done, step.err = check.Absent()
		}
		if step.done {
			t.w.do(func() { t.endCheck(step.progress, step.err) })
			return
		}
		t.w.do(func() { t.showCheck(step.progress) })
	}
}

// stopCheck ends the waiting. No cancel command is known: the AirPods may still finish.
func (t *hardwareTab) stopCheck() {
	if t.stop != nil {
		t.stop.Store(true)
	}
}

func (t *hardwareTab) endCheck(progress hearing.CheckProgress, err error) {
	t.checking = false
	t.stop = nil
	t.w.setHearingBusy(false)
	kept := progress
	t.last = &kept
	t.showCheck(progress)
	if err != nil {
		setStatusObject(t.checkStatus, t.checkText, t.w.requestError(err))
	}
	if progress.Phase == hearing.CheckCompleted && progress.Record != nil {
		t.showResults(progress.Record, nil)
	}
}

func checkPhaseText(progress hearing.CheckProgress) string {
	switch progress.Phase {
	case hearing.CheckWaitingCaseClose:
		return i18n.T("check.waitClose")
	case hearing.CheckWaitingResult:
		return i18n.T("check.waitResult")
	case hearing.CheckCompleted:
		return i18n.T("check.completed")
	case hearing.CheckTimedOut:
		return i18n.T("check.timedOut")
	case hearing.CheckStopped:
		return i18n.T("check.stopped")
	case hearing.CheckError:
		return progress.Message
	}
	return ""
}

func (t *hardwareTab) showCheck(progress hearing.CheckProgress) {
	kept := progress
	t.last = &kept
	waiting := t.checking && progress.Phase != ""
	showIf(t.progressBox, waiting)
	showIf(t.stopWait, waiting)
	showIf(t.run, !waiting)
	if total := progress.Elapsed + progress.Remaining; total > 0 {
		t.progress.SetValue(float64(progress.Elapsed) / float64(total))
	}
	setLabel(t.checkEars, fmt.Sprintf(i18n.T("check.ears"), earWord(progress.LeftUpdated), earWord(progress.RightUpdated)))
	setStatusObject(t.checkStatus, t.checkText, checkPhaseText(progress))
	t.updateEnabled()
	t.w.relayout()
}

func earWord(updated bool) string {
	if updated {
		return i18n.T("check.earNew")
	}
	return i18n.T("check.earWaiting")
}
