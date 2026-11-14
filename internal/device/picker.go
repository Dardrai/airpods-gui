package device

import (
	"errors"
	"sync"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/hid"
)

// ErrChooseDongle means several dongles are attached and none is chosen.
var ErrChooseDongle = errors.New("several dongles are plugged in and none is chosen")

// errSerialMissing has no serial in it, because open errors reach the debug log.
var errSerialMissing = errors.New("The dongle chosen with --serial is not plugged in.")

// Picker decides which dongle to open when several are attached. With one attached it opens
// that one, whatever was remembered, without changing what is remembered.
type Picker struct {
	enumerate func() ([]hid.Info, error)
	open      func(serial string) (hid.Device, error)

	mu     sync.Mutex
	wanted string
	strict bool
}

// NewPicker takes the system's listing and opener, the serial to prefer, and whether that
// serial is required, as with the --serial flag. The user's next choice ends the requirement.
func NewPicker(enumerate func() ([]hid.Info, error), open func(string) (hid.Device, error), wanted string, strict bool) *Picker {
	return &Picker{enumerate: enumerate, open: open, wanted: wanted, strict: strict && wanted != ""}
}

// Choose makes a serial the one to open from now on.
func (p *Picker) Choose(serial string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.wanted, p.strict = serial, false
}

// List reports the attached dongles.
func (p *Picker) List() ([]hid.Info, error) { return p.enumerate() }

// Open opens the dongle the rules point at.
func (p *Picker) Open() (hid.Device, error) {
	p.mu.Lock()
	wanted, strict := p.wanted, p.strict
	p.mu.Unlock()
	if strict {
		dev, err := p.open(wanted)
		var mismatch *hid.MatchError
		if errors.As(err, &mismatch) && mismatch.Found == 0 {
			return nil, errSerialMissing
		}
		return dev, err
	}
	list, err := p.enumerate()
	if err != nil || len(list) < 2 {
		return p.open("") // One dongle, or none, and Open words the outcome.
	}
	var usable []string
	for _, info := range list {
		if info.Openable && info.Serial != "" {
			if info.Serial == wanted {
				return p.open(wanted)
			}
			usable = append(usable, info.Serial)
		}
	}
	switch len(usable) {
	case 0:
		for _, info := range list {
			if info.Err != nil {
				return nil, info.Err
			}
		}
		return nil, ErrChooseDongle
	case 1:
		return p.open(usable[0]) // Nothing to choose between. The others show their reasons.
	}
	return nil, ErrChooseDongle
}

// Probe opens one dongle, reads its status report and closes it again, as `airpods list` does. It
// reports whether the AirPods are connected. Never call it for the dongle the owner holds open.
func (p *Picker) Probe(serial string) (bool, error) {
	dev, err := p.open(serial)
	if err != nil {
		return false, err
	}
	defer dev.Close()
	status, err := dongle.NewClient(dev).Status()
	if err != nil {
		return false, err
	}
	return status.Connected, nil
}
