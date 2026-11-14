package ui

import (
	"errors"
	"testing"

	"github.com/Dardrai/airpods-gui/core/dongle"
)

func TestRowValueDisplay(t *testing.T) {
	for name, c := range map[string]struct {
		value rowValue
		want  string
		known bool
	}{
		"nothing yet":             {rowValue{}, "", false},
		"a report":                {rowValue{reported: true, known: true, decoded: "on"}, "on", true},
		"a value nobody can name": {rowValue{reported: true, decoded: "unknown device value 9"}, "unknown device value 9", false},
		"sent beats the report":   {rowValue{reported: true, known: true, decoded: "off", requested: "on"}, "on", true},
		"the status with none":    {rowValue{status: "anc"}, "anc", true},
		"a report beats status":   {rowValue{reported: true, known: true, decoded: "off", status: "anc"}, "off", true},
		"sent beats the status":   {rowValue{requested: "transparency", status: "anc"}, "transparency", true},
		"away shows nothing":      {rowValue{away: true, reported: true, known: true, decoded: "on", requested: "off", status: "anc"}, "", false},
	} {
		if got, known := c.value.display(); got != c.want || known != c.known {
			t.Errorf("%s: %q %v", name, got, known)
		}
	}
}

func TestRowValueReport(t *testing.T) {
	const revision, newer = uint32(4), uint32(5)
	for name, c := range map[string]struct {
		start    rowValue
		decoded  string
		revision uint32
		first    bool
		changed  bool
		want     rowValue
		note     noteChange
	}{
		"a first report": {
			rowValue{}, "on", revision, true, true,
			rowValue{reported: true, known: true, decoded: "on", revision: revision}, noteClear,
		},
		"the same report again changes nothing": {
			rowValue{reported: true, known: true, decoded: "on", revision: revision, problem: true},
			"on", revision, false, false,
			rowValue{reported: true, known: true, decoded: "on", revision: revision, problem: true}, noteKeep,
		},
		"the first read of a link goes through though nothing moved": {
			rowValue{reported: true, known: true, decoded: "on", revision: revision},
			"on", revision, true, true,
			rowValue{reported: true, known: true, decoded: "on", revision: revision}, noteClear,
		},
		"a newer revision settles what was sent": {
			rowValue{reported: true, known: true, decoded: "off", revision: revision, requested: "on", sentRevision: revision},
			"on", newer, false, true,
			rowValue{reported: true, known: true, decoded: "on", revision: newer, sentRevision: revision}, noteClear,
		},
		"the old revision with another value leaves what was sent": {
			rowValue{reported: true, known: true, decoded: "off", revision: revision, requested: "on", sentRevision: revision},
			"other", revision, false, true,
			rowValue{reported: true, known: true, decoded: "other", revision: revision, requested: "on", sentRevision: revision}, noteSent,
		},
		"a newer revision clears a failure and doubt": {
			rowValue{reported: true, known: true, decoded: "off", revision: revision, problem: true, unconfirmed: true},
			"on", newer, false, true,
			rowValue{reported: true, known: true, decoded: "on", revision: newer}, noteClear,
		},
		"the same revision with a new value keeps the failure's note": {
			rowValue{reported: true, known: true, decoded: "off", revision: revision, problem: true},
			"on", revision, false, true,
			rowValue{reported: true, known: true, decoded: "on", revision: revision, problem: true}, noteKeep,
		},
		"an unknown value is shown as a note": {
			rowValue{}, "unknown device value 9", revision, false, true,
			rowValue{reported: true, decoded: "unknown device value 9", revision: revision}, noteDecoded,
		},
	} {
		value := c.start
		if got := value.report(c.decoded, c.revision, c.first); got != c.changed {
			t.Errorf("%s: changed %v", name, got)
		}
		if value != c.want {
			t.Errorf("%s: %+v", name, value)
		}
		if got := value.reportNote(); c.changed && got != c.note {
			t.Errorf("%s: note %v", name, got)
		}
	}
}

func TestRowValueReportMissing(t *testing.T) {
	for name, c := range map[string]struct {
		start   rowValue
		first   bool
		changed bool
		want    rowValue
		note    noteChange
	}{
		"nothing known stays that way": {rowValue{}, false, false, rowValue{}, noteUnreported},
		"the first read says so":       {rowValue{}, true, true, rowValue{}, noteUnreported},
		"a report that went": {
			rowValue{reported: true, known: true, decoded: "on", revision: 3}, false, true,
			rowValue{revision: 3}, noteUnreported,
		},
		"what was sent stays": {
			rowValue{requested: "on"}, false, true,
			rowValue{requested: "on"}, noteSent,
		},
		"a failed write's note stays": {
			rowValue{reported: true, known: true, decoded: "on", problem: true}, false, true,
			rowValue{problem: true}, noteKeep,
		},
	} {
		value := c.start
		if got := value.reportMissing(c.first); got != c.changed {
			t.Errorf("%s: changed %v", name, got)
		}
		if value != c.want {
			t.Errorf("%s: %+v", name, value)
		}
		if got := value.reportMissingNote(); got != c.note {
			t.Errorf("%s: note %v", name, got)
		}
	}
}

func TestRowValueWriteFinished(t *testing.T) {
	unconfirmed := &dongle.Unconfirmed{}
	failed := errors.New("refused")
	for name, c := range map[string]struct {
		err                  error
		unconfirmed, problem bool
	}{
		"went through":   {nil, false, false},
		"no report":      {unconfirmed, true, false},
		"a real failure": {failed, false, true},
	} {
		value := rowValue{writing: true, problem: !c.problem, unconfirmed: !c.unconfirmed}
		value.writeFinished(c.err)
		if value.writing || value.unconfirmed != c.unconfirmed || value.problem != c.problem {
			t.Errorf("%s: %+v", name, value)
		}
	}
}

func TestRowValueSentWaitsForANewerRevision(t *testing.T) {
	value := rowValue{reported: true, known: true, decoded: "off", revision: 7}
	value.sent("on")
	if got, known := value.display(); got != "on" || !known {
		t.Errorf("%q %v", got, known)
	}
	value.report("off", 7, false) // the same report, read again
	if value.requested != "on" {
		t.Error("an old report undid what was sent")
	}
	value.report("on", 8, false)
	if value.requested != "" || value.decoded != "on" {
		t.Errorf("%+v", value)
	}
}

func TestKeepsChoice(t *testing.T) {
	unconfirmed := &dongle.Unconfirmed{}
	failed := errors.New("refused")
	for _, c := range []struct {
		silent, isMode bool
		err            error
		want           bool
	}{
		{true, false, nil, true},
		{false, false, nil, false},
		{true, false, failed, false},
		{false, false, unconfirmed, true},
		{true, false, unconfirmed, true},
		{false, true, unconfirmed, false},
		{true, true, nil, true},
	} {
		if got := keepsChoice(c.silent, c.isMode, c.err); got != c.want {
			t.Errorf("%+v: %v", c, got)
		}
	}
}
