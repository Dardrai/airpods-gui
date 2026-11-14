package ui

import (
	"strings"

	"github.com/Dardrai/airpods-gui/internal/device"
)

// rowValue is what a row knows about its setting's value, and how that changes, with no widget
// in sight. settingRow embeds it and does the showing.
type rowValue struct {
	decoded  string
	known    bool
	reported bool
	writing  bool
	// unconfirmed means the last write may or may not have applied.
	unconfirmed bool
	revision    uint32 // of the last report
	// requested is the value sent and not yet reported back; while set it is what the
	// row shows, separately from any older report, until a newer report arrives.
	requested    string
	sentRevision uint32
	// problem is set while the note tells of a failed write; reads must not wipe it.
	problem bool
	// status is the value the dongle's status carries for a row with no report yet.
	status string
	// away is set while the AirPods are not there: nothing the row holds is current then.
	away bool
}

// display is what the row should show: a value sent silently, the last report, or the
// status's word for it until there is a report. Without AirPods it shows nothing.
func (v *rowValue) display() (string, bool) {
	switch {
	case v.away:
		return "", false
	case v.requested != "":
		return v.requested, true
	case !v.reported && v.status != "":
		return v.status, true
	}
	return v.decoded, v.known
}

// noteChange is what a change of value means for the row's note.
type noteChange int

const (
	noteKeep       noteChange = iota // leave it, for what it says about a failed write
	noteSent                         // the value was sent and not reported back
	noteUnreported                   // no report: the hint for that, if the row needs one
	noteDecoded                      // the report's text, for a value nobody can name
	noteClear                        // nothing to say
)

// reportMissing takes a read with no record for this row. It reports whether the row changed;
// an unchanged row is left alone, so repeated reads cannot undo a half-made choice.
func (v *rowValue) reportMissing(first bool) bool {
	if !first && !v.reported && v.requested == "" {
		return false
	}
	v.reported, v.known, v.decoded = false, false, ""
	return true
}

// reportMissingNote is the note after reportMissing changed the row.
func (v *rowValue) reportMissingNote() noteChange {
	switch {
	case v.requested != "":
		return noteSent
	case !v.problem:
		return noteUnreported
	}
	return noteKeep
}

// report takes a record read for this row. It reports whether the row changed. A newer revision
// settles what was sent and what the last write's outcome was.
func (v *rowValue) report(decoded string, revision uint32, first bool) bool {
	if !first && v.reported && revision == v.revision && decoded == v.decoded {
		return false
	}
	if v.requested != "" && revision != v.sentRevision {
		v.requested = ""
	}
	if revision != v.revision {
		v.unconfirmed = false
		v.problem = false
	}
	v.revision = revision
	v.reported = true
	v.known = !strings.HasPrefix(decoded, "unknown device value")
	v.decoded = decoded
	return true
}

// reportNote is the note after report changed the row.
func (v *rowValue) reportNote() noteChange {
	switch {
	case v.requested != "":
		return noteSent
	case !v.known:
		return noteDecoded
	case !v.problem:
		return noteClear
	}
	return noteKeep
}

// sent records a value sent to the AirPods that they may never report back.
func (v *rowValue) sent(value string) { v.requested, v.sentRevision = value, v.revision }

// keepsChoice decides whether a finished write counts as chosen though no report says so: a
// silent write that went through, or one the dongle took and the AirPods never echoed. Several
// settings apply without a report, and a report that does come still corrects the row. The
// mode is the exception, since the status always tells the real one.
func keepsChoice(silent, isMode bool, err error) bool {
	return (silent && err == nil) || (device.IsUnconfirmed(err) && !isMode)
}

// writeFinished takes the outcome of a write.
func (v *rowValue) writeFinished(err error) {
	v.writing = false
	v.unconfirmed = device.IsUnconfirmed(err)
	v.problem = err != nil && !v.unconfirmed
}
