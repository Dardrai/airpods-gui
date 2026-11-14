package ui

import (
	"errors"
	"strings"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

// A new link is read on every tick at first, for longer while nothing at all has been
// reported, and for a moment after the reports last grew.
const (
	eagerLinkTicks  = 10
	eagerEmptyTicks = 60
	eagerGrowTicks  = 3
)

// due reports whether this tick reads the settings: each one while a new link's reports
// are still arriving, every other one after that.
func (t *settingsTab) due(ticks int) bool {
	t.sinceLink++
	return ticks%settingsPollTicks == 0 || t.sinceLink <= max(eagerLinkTicks, t.eagerUntil) ||
		(len(t.reported) == 0 && t.sinceLink <= eagerEmptyTicks)
}

// refreshClicked reads again, then holds the button for a moment: every read occupies
// the channel other apps share.
func (t *settingsTab) refreshClicked() {
	t.coolingDown = true
	t.updateEnabled()
	time.AfterFunc(t.cooldown, func() {
		t.w.do(func() {
			t.coolingDown = false
			t.updateEnabled()
		})
	})
	t.read()
}

// read fetches the snapshot when the AirPods are ready. A read asked for while another
// is out runs after it, so a change of connection is never missed.
func (t *settingsTab) read() {
	if !t.w.state.Present || !t.w.state.Status.Has(dongle.CapTypedRPC) {
		return
	}
	if !t.w.state.Ready() {
		// The header says what the page is waiting for.
		t.clearReadProblem()
		return
	}
	if t.reading {
		t.again = true
		return
	}
	t.reading = true
	var records map[byte]dongle.Record
	t.w.owner.Submit(func(c *dongle.Client) error {
		settings, err := c.SettingsClient()
		if err != nil {
			return err
		}
		records, err = settings.Read()
		return err
	}, func(err error) {
		t.w.do(func() { t.readDone(records, err) })
	})
}

func (t *settingsTab) readDone(records map[byte]dongle.Record, err error) {
	t.reading = false
	loading := errors.Is(err, dongle.ErrNotReady)
	if loading != t.loading {
		t.loading = loading
		t.w.showHeader()
	}
	if err != nil {
		t.failures++
		t.nextRetry = t.w.now().Add(device.ReadBackoff(t.failures))
		t.w.logf("settings read failed (%d): %v", t.failures, err)
		// One lost read among many says nothing; the retry usually fixes it unseen. A
		// refusal while the AirPods controls come up is part of connecting, and the header
		// says the page is waiting, until it has gone on too long.
		if text := device.ExplainRead(err, t.w.state.Ready()); text != device.NotReady() && ((!t.everRead && !loading) || t.failures >= quietReadFailures) {
			t.showReadProblem(text)
		}
	} else {
		if t.failures > 0 {
			t.w.logf("settings read recovered after %d failures", t.failures)
		}
		t.failures = 0
		t.clearReadProblem()
		t.applyRecords(records)
	}
	if t.again {
		t.again = false
		t.read()
	}
}

// poll reads again while the window is open: the AirPods fill the dongle's reports over
// time and can change them from the stem or another app, and nothing announces either.
func (t *settingsTab) poll() {
	if t.failures == 0 && !t.reading && t.w.state.Ready() && !t.w.state.Status.MicrophoneActive {
		t.read()
	}
}

// linkChanged forgets what the last link's read proved.
func (t *settingsTab) linkChanged() {
	t.failures = 0
	t.everRead, t.loading = false, false
	t.sinceLink, t.eagerUntil = 0, 0
	if !t.w.state.Present {
		t.clearReadProblem()
	}
	for _, row := range t.rows {
		// What a write's outcome and a report's revision said was about the old link.
		if !row.writing {
			row.unconfirmed, row.problem, row.revision = false, false, 0
		}
		if !row.reported && row.requested == "" && !row.writing {
			setNote(row, t.unreportedNote(row.setting))
		}
	}
	if strength := t.byName["adaptive-strength"]; strength != nil && !strength.reported && strength.requested == "" {
		setNote(strength, "")
	}
}

func readProblemPrefix() string { return i18n.T("settings.problemPrefix") + ": " }

func (t *settingsTab) showReadProblem(text string) { t.w.say(readProblemPrefix() + text) }

func (t *settingsTab) clearReadProblem() {
	if strings.HasPrefix(t.w.banner.Text, readProblemPrefix()) {
		t.w.say("")
	}
}

// retry reads again after a failure, more slowly each time.
func (t *settingsTab) retry() {
	if t.failures > 0 && !t.reading && !t.w.now().Before(t.nextRetry) {
		t.read()
	}
}

// applyRecords shows a read. A row whose report did not change is left alone, so repeated
// reads cannot undo a half-made choice or a note about a failed write.
func (t *settingsTab) applyRecords(records map[byte]dongle.Record) {
	first := !t.everRead
	t.everRead = true
	// Any read after the write settles a mode change: the AirPods took it or they did not,
	// and a report that stayed the same must not leave the strength waiting for ever.
	if mode := t.byName["mode"]; mode != nil && !mode.writing {
		mode.unconfirmed = false
	}
	for _, row := range t.rows {
		if row.writing {
			continue
		}
		record, ok := records[row.setting.Control]
		if !ok {
			if row.reportMissing(first) {
				t.restore(row)
				t.showNote(row, row.reportMissingNote())
			}
			continue
		}
		if row.report(row.setting.Decode(record.Value), record.Revision, first) {
			t.restore(row)
			t.showNote(row, row.reportNote())
		}
	}
	if len(records) != len(t.reported) {
		t.eagerUntil = t.sinceLink + eagerGrowTicks
	}
	t.followStatus()
	t.dropIgnoredMode()
	t.publishReports(records)
	t.wasArriving = t.arriving()
	t.fitModel()
	if settling := t.settling(); settling != t.settled {
		t.w.showHeader()
	}
}

// showNote carries out what a change of value means for the row's note.
func (t *settingsTab) showNote(row *settingRow, change noteChange) {
	switch change {
	case noteSent:
		setNote(row, sentText())
	case noteUnreported:
		setNote(row, t.unreportedNote(row.setting))
	case noteDecoded:
		setNote(row, row.decoded)
	case noteClear:
		setNote(row, "")
	}
}

func (t *settingsTab) publishReports(records map[byte]dongle.Record) {
	t.overlay = nil
	t.reported = make(map[byte]uint32, len(records))
	for control, record := range records {
		t.reported[control] = record.Value
	}
	if t.w.opts.OnReports != nil {
		t.w.opts.OnReports(t.reported)
	}
}
