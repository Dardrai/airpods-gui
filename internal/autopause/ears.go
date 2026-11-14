package autopause

import (
	"bytes"

	"github.com/Dardrai/airpods-gui/core/dongle"
)

// earState is what one bud reports in the ear status report.
type earState byte

const (
	earIn earState = iota
	earOut
	earCase
	earGone
	earMoving // between resting states; it says nothing about where the bud ends up
)

// Action is what a change in the ear status asks for.
type Action int

const (
	None Action = iota
	Pause
	Resume
)

// maxFreshAge is how old a new report may be and still pause anything. Polls are a
// second apart and one may be skipped, so this is enough for a report that just arrived
// and too little for one left over from before a gap.
const maxFreshAge = 2500

var earHeader = []byte{4, 0, 4, 0, 6, 0}

func parseEars(packet []byte) ([2]earState, bool) {
	if len(packet) < 8 || !bytes.Equal(packet[:6], earHeader) || packet[6] > 4 || packet[7] > 4 {
		return [2]earState{}, false
	}
	return [2]earState{earState(packet[6]), earState(packet[7])}, true
}

// listening means the AirPods are in use: both buds in, or one in with its partner
// resting in the case.
func listening(s [2]earState) bool {
	return (s[0] == earIn && s[1] == earIn) ||
		(s[0] == earIn && s[1] == earCase) ||
		(s[0] == earCase && s[1] == earIn)
}

func wearing(s [2]earState) [2]bool { return [2]bool{s[0] == earIn, s[1] == earIn} }

func decide(before, now [2]earState) Action {
	if listening(now) {
		if listening(before) {
			return None
		}
		return Resume
	}
	wasWorn := before[0] == earIn || before[1] == earIn
	if wasWorn && (listening(before) || wearing(before) != wearing(now)) {
		return Pause
	}
	return None
}

// noCase forgets case positions, which only count while they are current.
func noCase(s [2]earState) [2]earState {
	for i := range s {
		if s[i] == earCase {
			s[i] = earGone
		}
	}
	return s
}

// Ears follows ear status reports and turns fresh transitions into actions. The first
// report is only a baseline: nothing already in the dongle's memory can pause anything.
type Ears struct {
	have     bool
	revision uint32
	state    [2]earState
	valid    bool
}

// Reset forgets everything, as after a new connection.
func (e *Ears) Reset() { *e = Ears{} }

// Observe takes the report just read. ok is false when the slot is empty or the report
// cannot be read, which ends any claim to a paused player.
func (e *Ears) Observe(report *dongle.Cached) (action Action, ok bool) {
	if report == nil {
		e.valid = false
		return None, false
	}
	stale := report.AgeMs > maxFreshAge
	if e.have && report.Revision == e.revision {
		if stale && e.valid {
			e.state = noCase(e.state)
		}
		return None, true
	}
	if e.have && report.Revision-e.revision >= 1<<31 {
		return None, true // an older report than the one already seen
	}
	first := !e.have
	e.have, e.revision = true, report.Revision
	now, parsed := parseEars(report.Packet)
	if !parsed {
		e.valid = false
		return None, false
	}
	before, known := e.state, e.valid
	if !known {
		before = [2]earState{earGone, earGone}
	}
	for i := range now {
		if now[i] == earMoving {
			now[i] = before[i]
		}
	}
	e.state, e.valid = now, true
	if first && stale {
		e.state = noCase(now)
	}
	if !known {
		return None, true
	}
	action = decide(before, now)
	if stale && action == Pause {
		return None, true // too late to be the removal that is happening now
	}
	return action, true
}
