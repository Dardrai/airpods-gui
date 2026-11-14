package device

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/hid"
)

func infos(serials ...string) []hid.Info {
	list := make([]hid.Info, len(serials))
	for i, serial := range serials {
		list[i] = hid.Info{Path: "node" + serial, Serial: serial, Openable: true}
	}
	return list
}

// openLog records which serial each Open asked for.
type openLog struct {
	asked []string
	err   error
}

func (l *openLog) open(serial string) (hid.Device, error) {
	l.asked = append(l.asked, serial)
	if l.err != nil {
		return nil, l.err
	}
	return sim.New(sim.Default()).Open("")
}

func TestPickerRules(t *testing.T) {
	blocked := hid.Info{Serial: "SIM0003", Err: errors.New("no permission")}
	nameless := hid.Info{Openable: true}
	cases := []struct {
		name   string
		list   []hid.Info
		wanted string
		strict bool
		asked  string
		opens  bool
		err    error
	}{
		{name: "none", list: nil, asked: "", opens: true},
		{name: "one, nothing remembered", list: infos("SIM0001"), asked: "", opens: true},
		{name: "one, another remembered", list: infos("SIM0001"), wanted: "SIM0009", asked: "", opens: true},
		{name: "several, none chosen", list: infos("SIM0001", "SIM0002"), err: ErrChooseDongle},
		{name: "several, one remembered", list: infos("SIM0001", "SIM0002"), wanted: "SIM0002", asked: "SIM0002", opens: true},
		{name: "several, the remembered one gone", list: infos("SIM0001", "SIM0002"), wanted: "SIM0009", err: ErrChooseDongle},
		{name: "several, one can open", list: []hid.Info{blocked, infos("SIM0001")[0]}, asked: "SIM0001", opens: true},
		{name: "several, one without a serial", list: []hid.Info{nameless, infos("SIM0001")[0]}, asked: "SIM0001", opens: true},
		{name: "several, none can open", list: []hid.Info{blocked, blocked}, err: blocked.Err},
		{name: "flag", list: infos("SIM0001", "SIM0002"), wanted: "SIM0002", strict: true, asked: "SIM0002", opens: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			log := &openLog{}
			picker := NewPicker(func() ([]hid.Info, error) { return c.list, nil }, log.open, c.wanted, c.strict)
			dev, err := picker.Open()
			if !errors.Is(err, c.err) || (dev != nil) != c.opens {
				t.Fatalf("dev %v, err %v", dev, err)
			}
			if c.opens && (len(log.asked) != 1 || log.asked[0] != c.asked) {
				t.Errorf("asked %q", log.asked)
			}
			if !c.opens && len(log.asked) != 0 {
				t.Errorf("opened %q", log.asked)
			}
		})
	}
}

func TestPickerFlagSaysItsDongleIsMissing(t *testing.T) {
	log := &openLog{err: &hid.MatchError{Found: 0, Text: "Expected exactly one"}}
	picker := NewPicker(func() ([]hid.Info, error) { return infos("SIM0001"), nil }, log.open, "SIM0009", true)
	_, err := picker.Open()
	if !errors.Is(err, errSerialMissing) || classify(err) == "" {
		t.Fatalf("err %v, shown %q", err, classify(err))
	}
	picker.Choose("SIM0001")
	log.err = nil
	if _, err := picker.Open(); err != nil {
		t.Errorf("a choice ends the requirement: %v", err)
	}
}

func TestOwnerPublishesTheListAndWhichOneIsOpen(t *testing.T) {
	first, second := sim.Default(), sim.Default()
	first.Serial, second.Serial = "SIM0001", "SIM0002"
	devices := map[string]*sim.Device{"SIM0001": sim.New(first), "SIM0002": sim.New(second)}
	list := infos("SIM0001", "SIM0002")
	var lists [][]hid.Info
	var states []State
	picker := NewPicker(func() ([]hid.Info, error) { return list, nil }, func(serial string) (hid.Device, error) {
		return devices[serial].Open("")
	}, "", false)
	owner := NewOwner(picker.Open, DefaultOptions(), func(st State) { states = append(states, st) })
	now := time.Unix(1000, 0)
	owner.now = func() time.Time { return now }
	owner.SetPicker(picker, time.Second, func(got []hid.Info) { lists = append(lists, got) })
	owner.poll()
	last := states[len(states)-1]
	if last.Present || !last.ChooseDongle || !last.Several || last.Problem != "" {
		t.Fatalf("%+v", last)
	}
	if len(lists) != 1 || len(lists[0]) != 2 {
		t.Fatalf("lists %v", lists)
	}
	owner.applyChoice("SIM0002")
	last = states[len(states)-1]
	if !last.Present || last.Serial != "SIM0002" || !last.Several || last.ChooseDongle {
		t.Fatalf("%+v", last)
	}
	if states[len(states)-2].Present {
		t.Error("the switch must pass through no dongle, so that everything is read again")
	}
	owner.poll()
	if len(lists) != 1 {
		t.Error("an unchanged list is not published again")
	}
	list = infos("SIM0002")
	now = now.Add(2 * time.Second)
	owner.poll()
	last = states[len(states)-1]
	if len(lists) != 2 || last.Several || !last.Present {
		t.Errorf("after an unplug: lists %v, state %+v", lists, last)
	}
}

func TestChoosingRunsQueuedJobsOnTheCurrentDongleFirst(t *testing.T) {
	first, second := sim.Default(), sim.Default()
	first.Serial, second.Serial = "SIM0001", "SIM0002"
	devices := map[string]*sim.Device{"SIM0001": sim.New(first), "SIM0002": sim.New(second)}
	picker := NewPicker(func() ([]hid.Info, error) { return infos("SIM0001", "SIM0002"), nil }, func(serial string) (hid.Device, error) {
		return devices[serial].Open("")
	}, "SIM0001", false)
	switched := make(chan struct{})
	var once sync.Once
	owner := NewOwner(picker.Open, DefaultOptions(), func(st State) {
		if st.Serial == "SIM0002" {
			once.Do(func() { close(switched) })
		}
	})
	owner.SetPicker(picker, time.Hour, nil)
	serial := make(chan string, 1)
	gate := make(chan struct{})
	owner.Start()
	defer owner.Stop(time.Second)
	owner.Submit(func(*dongle.Client) error { <-gate; return nil }, nil) // Keeps the owner busy.
	owner.Submit(func(c *dongle.Client) error {
		status, err := c.Status()
		_ = status
		serial <- devices["SIM0001"].Serial()
		return err
	}, nil)
	owner.Choose("SIM0002")
	close(gate)
	select {
	case <-switched:
	case <-time.After(5 * time.Second):
		t.Fatal("not switched")
	}
	select {
	case <-serial:
	default:
		t.Error("the queued job did not run before the switch")
	}
}
