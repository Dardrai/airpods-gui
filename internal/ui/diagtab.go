package ui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/hearing"
	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
	"github.com/Dardrai/airpods-gui/internal/statusdoc"
)

// notReported marks a value nobody has reported.
func notReported() string { return i18n.T("value.notReported") }

type diagTab struct {
	w       *Window
	content fyne.CanvasObject
	refresh *widget.Button
	values  map[string]*widget.Label
	reading bool
	last    *dongle.PerformanceState
	capture *captureControls
	copyDoc *widget.Button
	copying bool

	copyDongles, copyInfo, copyHearing, copyHardware *widget.Button
	copyResults, copyTransparency, copyLoud          *widget.Button

	reports *widget.Label
	records map[byte]dongle.Record
}

// counterSections is the display order of the counters.
var counterSections = []struct {
	title string
	names []string
}{
	{"Dongle", []string{"Uptime", "Input", "Codec", "Flags"}},
	{"Audio buffer", []string{"Buffer used", "Buffer high water", "Underflow frames", "Dropped frames"}},
	{"Encoder", []string{"Encoded units", "Encode time", "Encode time max", "Send failures", "Recoveries"}},
	{"Memory", []string{"Heap capacity", "Heap allocated", "Heap remaining", "Heap arena"}},
}

func newDiagTab(w *Window) *diagTab {
	t := &diagTab{w: w, values: map[string]*widget.Label{}}
	t.refresh = w.button("diag.refresh", t.read)
	rows := []fyne.CanvasObject{w.device.status}
	for _, section := range counterSections {
		rows = append(rows, w.subCaption("diag."+section.title))
		for _, name := range section.names {
			value := newValue(notReported())
			t.values[name] = value
			rows = append(rows, w.valueRow("diag."+name, value))
		}
	}
	t.reports = smallLabel("")
	rows = append(rows, w.subCaption("diag.reports"), tight(t.reports))
	t.copyDoc = w.button("diag.copyJSON", t.copyJSON)
	rows = append(rows, gap(4), container.NewHBox(layout.NewSpacer(), t.copyDoc, t.refresh))
	t.copyDongles = w.button("diag.copyDongles", t.copyDongleList)
	t.copyInfo = w.button("diag.copyInfo", t.copyInfoDoc)
	t.copyHearing = w.button("diag.copyHearing", t.copyHearingDoc)
	t.copyHardware = w.button("diag.copyHardware", t.copyHardwareDoc)
	t.copyResults = w.button("diag.copyResults", t.copyResultsDoc)
	t.copyTransparency = w.button("extra.transparency", t.copyTransparencyDoc)
	t.copyLoud = w.button("extra.loud", t.copyLoudDoc)
	rows = append(rows, w.subCaption("diag.copyAs"), container.NewGridWithColumns(2, t.copyDongles, t.copyInfo, t.copyHearing,
		t.copyResults, t.copyHardware, t.copyTransparency, t.copyLoud))
	t.capture = newCaptureControls(w)
	rows = append(rows, t.capture.content)
	t.content = container.NewVBox(rows...)
	t.updateEnabled()
	w.bind(t.showReports)
	w.texts = append(w.texts, func() {
		state := dongle.PerformanceState{}
		if t.last != nil {
			state = *t.last
		}
		t.show(state)
	})
	return t
}

// show writes the counters; an empty state means nothing has been read yet.
func (t *diagTab) show(state dongle.PerformanceState) {
	for name, text := range counterTexts(state) {
		if t.last == nil {
			text = notReported()
		}
		setLabel(t.values[name], text)
	}
}

func (t *diagTab) updateEnabled() {
	usable := t.w.state.Present && t.w.state.Status.Has(dongle.CapTypedRPC)
	setEnabled(t.refresh, usable)
	setEnabled(t.copyDoc, usable && !t.copying)
	setEnabled(t.copyDongles, !t.copying)
	setEnabled(t.copyInfo, t.w.state.Present && !t.copying)
	setEnabled(t.copyHearing, t.w.state.Present && !t.copying && !t.w.hearingBusy)
	setEnabled(t.copyHardware, t.w.state.Present && !t.copying && !t.w.hearingBusy)
	setEnabled(t.copyResults, t.w.state.Present && !t.copying && !t.w.hearingBusy)
	experimental := t.w.state.Present && t.w.Experimental() && !t.copying && !t.w.hearingBusy
	setEnabled(t.copyTransparency, experimental)
	setEnabled(t.copyLoud, experimental)
	t.capture.updateEnabled()
}

func (t *diagTab) read() {
	if t.reading || !t.w.state.Present || !t.w.state.Status.Has(dongle.CapTypedRPC) {
		return
	}
	t.reading = true
	var state dongle.PerformanceState
	var records map[byte]dongle.Record
	readReports := t.w.state.Ready()
	t.w.owner.Submit(func(c *dongle.Client) (err error) {
		state, err = c.Performance()
		if err != nil {
			return err
		}
		if readReports {
			records = readRecords(c) // The ages grow, so they are read along with the counters.
		}
		return nil
	}, func(err error) {
		t.w.do(func() {
			t.reading = false
			if err != nil {
				t.w.say(i18n.T("section.diagnostics") + ": " + device.Explain(err))
				return
			}
			kept := state
			t.last = &kept
			t.show(state)
			if records != nil {
				t.records = records
				t.showReports()
			}
		})
	})
}

// readRecords reads the settings report; nil if the dongle would not serve it just now.
func readRecords(c *dongle.Client) map[byte]dongle.Record {
	settings, err := c.SettingsClient()
	if err != nil {
		return nil
	}
	records, err := settings.Read()
	if err != nil {
		return nil
	}
	return records
}

// showReports lists every setting with the age of its report, as the CLI's settings command does.
func (t *diagTab) showReports() {
	lines := make([]string, 0, len(dongle.Settings)+len(dongle.ExtraSettings))
	for _, setting := range statusdoc.Settings(t.records) {
		switch {
		case setting.Raw == nil:
			lines = append(lines, fmt.Sprintf(i18n.T("diag.reportNone"), setting.Name))
		case setting.Decoded == nil:
			lines = append(lines, fmt.Sprintf(i18n.T("diag.reportUnknown"), setting.Name, *setting.Raw, *setting.AgeMs))
		default:
			lines = append(lines, fmt.Sprintf(i18n.T("diag.reportAge"), setting.Name, *setting.Decoded, *setting.AgeMs))
		}
	}
	setLabel(t.reports, strings.Join(lines, "\n"))
}

// copyJSON reads fresh values and copies them. What a read cannot get is null, as in the CLI.
func (t *diagTab) copyJSON() {
	if t.copying || !t.w.state.Present {
		return
	}
	t.copying = true
	t.updateEnabled()
	var doc statusdoc.Document
	t.w.owner.Submit(func(c *dongle.Client) error {
		status, err := c.Status()
		if err != nil {
			return err
		}
		var records map[byte]dongle.Record
		if status.ControlsReady {
			records = readRecords(c)
		}
		doc = statusdoc.Build(status, records)
		return nil
	}, func(err error) {
		t.w.do(func() {
			t.copying = false
			t.updateEnabled()
			if err != nil {
				t.w.say(i18n.T("section.diagnostics") + ": " + device.Explain(err))
				return
			}
			t.w.opts.Fyne.Clipboard().SetContent(doc.JSON())
			t.w.sayBriefly(i18n.T("diag.jsonCopied"))
		})
	})
}

// finishCopy ends a copy of one document: the text on the clipboard, or why there is none.
func (t *diagTab) finishCopy(text string, err error) {
	t.copying = false
	t.updateEnabled()
	if err != nil {
		t.w.say(i18n.T("section.diagnostics") + ": " + t.w.requestError(err))
		return
	}
	t.w.opts.Fyne.Clipboard().SetContent(text)
	t.w.sayBriefly(i18n.T("diag.docCopied"))
}

func (t *diagTab) startCopy() bool {
	if t.copying {
		return false
	}
	t.copying = true
	t.updateEnabled()
	return true
}

// copyRead runs one read of a copy on the owner goroutine.
func (t *diagTab) copyRead(read func(*dongle.Client) (any, error)) {
	if !t.startCopy() {
		return
	}
	var doc any
	t.w.owner.Submit(func(c *dongle.Client) (err error) {
		doc, err = read(c)
		return err
	}, func(err error) {
		t.w.do(func() {
			if err != nil {
				t.finishCopy("", err)
				return
			}
			t.finishCopy(statusdoc.Encode(doc), nil)
		})
	})
}

// copyDongleList lists off the UI thread, since opening a dongle can take a moment. The open dongle
// is the owner's, and its status is already known. Without a picker, which only a simulated
// dongle lacks, the open dongle is the list.
func (t *diagTab) copyDongleList() {
	if !t.startCopy() {
		return
	}
	state, picker := t.w.state, t.w.opts.Picker
	go func() {
		var infos []hid.Info
		var err error
		switch {
		case picker != nil:
			infos, err = picker.List()
		case state.Present:
			infos = []hid.Info{{Path: "simulated", Serial: state.Serial, PID: hid.ProductAudio, Openable: true}}
		}
		list := statusdoc.ListDongles(infos, func(info hid.Info) (bool, error) {
			if state.Present && info.Serial == state.Serial {
				return state.Status.Connected, nil
			}
			return picker.Probe(info.Serial)
		})
		t.w.do(func() { t.finishCopy(statusdoc.Encode(list), err) })
	}()
}

// copyInfoDoc reads what the AirPods say about themselves. What they did not say is null.
func (t *diagTab) copyInfoDoc() {
	t.copyRead(func(c *dongle.Client) (any, error) {
		status, err := c.Status()
		switch {
		case err != nil:
			return nil, err
		case !status.Connected:
			return nil, errors.New(i18n.T("hardware.connectFirst"))
		case !status.Has(dongle.CapRawTransport):
			return nil, errors.New(i18n.T("hardware.updateFirmware"))
		}
		report, err := c.Cached(dongle.SlotMetadata)
		if err != nil {
			return nil, err
		}
		var meta dongle.Metadata
		if report != nil {
			meta, _ = dongle.ParseMetadata(report.Packet)
		}
		return statusdoc.BuildInfo(meta), nil
	})
}

// copyHearingDoc reads the modes as Read in the Hearing section does. It writes nothing.
func (t *diagTab) copyHearingDoc() {
	if t.w.hearingBusy {
		return
	}
	if !hearing.Supported(t.w.model()) {
		t.w.say(i18n.T("section.diagnostics") + ": " + i18n.T("hardware.notAvailable"))
		return
	}
	t.copyRead(func(c *dongle.Client) (any, error) {
		modes, err := t.w.modesFor(c)
		if err != nil {
			return nil, err
		}
		state, err := modes.Read(c)
		if err != nil {
			return nil, err
		}
		return statusdoc.BuildHearingStatus(state, statusdoc.ReadHearingReports(c)), nil
	})
}

// copyHardwareDoc applies the gates of Read device data, and reads as it does.
func (t *diagTab) copyHardwareDoc() {
	t.copySession(func(_ string, s *dongle.Session) (any, error) {
		data, err := hearing.ReadDeviceData(s.Exchange, time.Now())
		if err != nil {
			return nil, err
		}
		return statusdoc.BuildDeviceData(data), nil
	})
}

// copyResultsDoc reads the saved results as Refresh in Hardware checks does.
func (t *diagTab) copyResultsDoc() {
	t.copySession(func(number string, s *dongle.Session) (any, error) {
		record, err := hearing.ReadDiagnostics(s.Exchange, number, time.Now())
		if err != nil {
			return nil, err
		}
		return statusdoc.BuildHardwareStatus(record), nil
	})
}

// copySession runs one read of the Hardware checks section, with its gates, in one leased sequence.
func (t *diagTab) copySession(read func(string, *dongle.Session) (any, error)) {
	ok, reason := t.w.hardware.readable()
	if !ok {
		if reason != "" {
			t.w.say(i18n.T("section.diagnostics") + ": " + reason)
		}
		return
	}
	if !t.startCopy() {
		return
	}
	number := t.w.model()
	var doc any
	t.w.withSession(func(_ *dongle.Client, s *dongle.Session) (err error) {
		doc, err = read(number, s)
		return err
	}, func(err error) {
		if err != nil {
			t.finishCopy("", err)
			return
		}
		t.finishCopy(statusdoc.Encode(doc), nil)
	})
}

// copyExperimental applies the gates of More experiments: the switch, as --experimental is in the
// CLI, and what the model lists. The requests enforce the same flags again.
func (t *diagTab) copyExperimental(listed func(*model.Profile) bool, read func(*dongle.Client) (any, error)) {
	if !t.w.Experimental() || t.w.hearingBusy {
		return
	}
	if !t.w.state.Ready() || !t.w.state.Status.Has(dongle.CapRawTransport) {
		t.w.say(i18n.T("section.diagnostics") + ": " + i18n.T("hardware.connectFirst"))
		return
	}
	if profile, known := model.Find(t.w.model()); !known || !listed(profile) {
		t.w.say(i18n.T("section.diagnostics") + ": " + i18n.T("hardware.notAvailable"))
		return
	}
	t.copyRead(read)
}

func (t *diagTab) copyTransparencyDoc() {
	t.copyExperimental(func(p *model.Profile) bool { return p.CustomTransparency }, func(c *dongle.Client) (any, error) {
		modes, err := t.w.modesFor(c)
		if err != nil {
			return nil, err
		}
		settings, err := modes.ExperimentalReadTransparency(c)
		if err != nil {
			return nil, err
		}
		return statusdoc.BuildTransparency(settings), nil
	})
}

func (t *diagTab) copyLoudDoc() {
	t.copyExperimental(func(p *model.Profile) bool { return p.LoudSoundReduction }, func(c *dongle.Client) (any, error) {
		value, err := hearing.ExperimentalReadLoudSoundReduction(c)
		if err != nil {
			return nil, err
		}
		return statusdoc.BuildLoudSoundReduction(value), nil
	})
}

func counter(value *uint32, format func(uint32) string) string {
	if value == nil {
		return notReported()
	}
	return format(*value)
}

func plain(value uint32) string { return fmt.Sprintf("%d", value) }

func micros(value uint32) string { return fmt.Sprintf("%d µs", value) }

func kilobytes(value uint32) string { return fmt.Sprintf("%.1f KiB", float64(value)/1024) }

func counterTexts(s dongle.PerformanceState) map[string]string {
	ofCapacity := func(value *uint32) string {
		text := counter(value, plain)
		if value != nil && s.PCMCapacity != nil {
			text += fmt.Sprintf(" of %d", *s.PCMCapacity)
		}
		return text
	}
	return map[string]string{
		"Uptime": counter(s.UptimeMs, func(v uint32) string {
			return (time.Duration(v) * time.Millisecond).Round(time.Second).String()
		}),
		"Input":             plain(uint32(s.Input)),
		"Codec":             codecName(s.Codec),
		"Flags":             fmt.Sprintf("0x%02x", s.Flags),
		"Buffer used":       ofCapacity(s.PCMUsed),
		"Buffer high water": ofCapacity(s.PCMHighWater),
		"Underflow frames":  counter(s.UnderflowFrames, plain),
		"Dropped frames":    counter(s.DroppedFrames, plain),
		"Encoded units":     counter(s.EncodedUnits, plain),
		"Encode time":       counter(s.EncodeUs, micros),
		"Encode time max":   counter(s.EncodeMaxUs, micros),
		"Send failures":     counter(s.SendFailures, plain),
		"Recoveries":        counter(s.Recoveries, plain),
		"Heap capacity":     counter(s.HeapCapacity, kilobytes),
		"Heap allocated":    counter(s.HeapAllocated, kilobytes),
		"Heap remaining":    counter(s.HeapRemaining, kilobytes),
		"Heap arena":        counter(s.HeapArena, kilobytes),
	}
}
