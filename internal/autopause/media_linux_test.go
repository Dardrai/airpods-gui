package autopause

import (
	"errors"
	"testing"
	"time"
)

type fakePlayer struct {
	status      string
	track       string
	noPause     bool
	ignoresCall bool
	calls       []string
}

type fakeBus map[string]*fakePlayer

func (b fakeBus) Players() ([]string, error) {
	var names []string
	for _, name := range []string{":1.1", ":1.2", ":1.3"} {
		if _, ok := b[name]; ok {
			names = append(names, name)
		}
	}
	return names, nil
}

func (b fakeBus) Status(player string) (string, error) {
	if p, ok := b[player]; ok {
		return p.status, nil
	}
	return "", errors.New("gone")
}
func (b fakeBus) CanPause(player string) bool { return !b[player].noPause }
func (b fakeBus) Track(player string) string {
	if p, ok := b[player]; ok {
		return p.track
	}
	return ""
}
func (b fakeBus) Call(player, method string) error {
	p, ok := b[player]
	if !ok {
		return errors.New("gone")
	}
	p.calls = append(p.calls, method)
	if p.ignoresCall {
		return nil
	}
	switch method {
	case "Pause":
		p.status = "Paused"
	case "Play":
		p.status = "Playing"
	}
	return nil
}

func media(bus mprisBus) *mprisMedia { return &mprisMedia{bus: bus, sleep: func(time.Duration) {}} }

func TestMPRISResumesOnlyThePlayersItPaused(t *testing.T) {
	bus := fakeBus{
		":1.1": {status: "Playing", track: "a"},
		":1.2": {status: "Paused", track: "b"},
		":1.3": {status: "Playing", noPause: true},
	}
	paused, err := media(bus).PauseActive()
	if err != nil || paused == nil {
		t.Fatal(paused, err)
	}
	if bus[":1.1"].status != "Paused" || len(bus[":1.2"].calls) != 0 || len(bus[":1.3"].calls) != 0 {
		t.Fatalf("%+v", bus[":1.1"])
	}
	if !paused.StillOurs() {
		t.Fatal("claim lost")
	}
	if err := paused.Resume(); err != nil {
		t.Fatal(err)
	}
	if bus[":1.1"].status != "Playing" || len(bus[":1.2"].calls) != 0 {
		t.Errorf("%+v %+v", bus[":1.1"], bus[":1.2"])
	}
}

func TestMPRISNothingPlaying(t *testing.T) {
	paused, err := media(fakeBus{":1.1": {status: "Paused"}}).PauseActive()
	if paused != nil || err != nil {
		t.Errorf("%v %v", paused, err)
	}
}

func TestMPRISUnconfirmedPauseHoldsNoClaim(t *testing.T) {
	paused, err := media(fakeBus{":1.1": {status: "Playing", ignoresCall: true}}).PauseActive()
	if paused != nil || err == nil {
		t.Errorf("%v %v", paused, err)
	}
}

func TestMPRISClaimEnds(t *testing.T) {
	for name, change := range map[string]func(bus fakeBus){
		"played":     func(bus fakeBus) { bus[":1.1"].status = "Playing" },
		"next track": func(bus fakeBus) { bus[":1.1"].track = "other" },
		"closed":     func(bus fakeBus) { delete(bus, ":1.1") },
	} {
		t.Run(name, func(t *testing.T) {
			bus := fakeBus{":1.1": {status: "Playing", track: "a"}}
			paused, err := media(bus).PauseActive()
			if err != nil {
				t.Fatal(err)
			}
			change(bus)
			if paused.StillOurs() {
				t.Error("claim kept")
			}
			if err := paused.Resume(); err != nil {
				t.Error(err)
			}
			if p := bus[":1.1"]; p != nil && len(p.calls) != 1 {
				t.Errorf("calls: %q", p.calls)
			}
		})
	}
}
