package app

import (
	"errors"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/internal/capture"
	"github.com/Dardrai/airpods-gui/internal/device"
)

// simCloseDelay is how many routing reads a release of the "disconnect" simulator stays
// pending: more than Disconnect's own 5 s wait makes, so that the pending state shows, and
// the app's follow-up reads close it a few seconds later.
const simCloseDelay = 55

// SimConfig is the simulated dongle for a value of AIRPODS_GUI_SIM: "disconnect" releases
// the AirPods slowly enough that the closing state shows, "old" is firmware older than
// 0.1.59, without the routing handover, "legacy" is firmware 0.1.59 to 0.1.62, with the
// handover but without the reconnect policy, the extended settings or the EQ report, "case"
// puts both buds in the case so that the in-case check can start, "autopause" takes the buds
// out and puts them back every 15 s, "owner" reports only the settings a real pair was seen
// reporting, and anything else is a current dongle with AirPods Pro 3 in the ears.
// AIRPODS_GUI_SIM=several is SimPicker.
func SimConfig(value string) sim.Config {
	cfg := sim.Demo()
	capture.AddSimReplies(&cfg)
	// Firmware 0.1.64 and newer also report EN 352 protection and Gain Swipe, in the extended
	// snapshot only.
	cfg.Settings = append(cfg.Settings, sim.Setting{Control: 0x37, Age: 900, Revision: 3, Value: 2},
		sim.Setting{Control: 0x2f, Age: 900, Revision: 3, Value: 1})
	if value == "case" {
		cfg.Cached[dongle.SlotEarStatus] = sim.CachedReport{Age: 1000, Revision: 1, Packet: sim.EarPacket(2, 2)}
		cfg.Flags = 0x03 // connected and ready, not streaming
	}
	if value == "owner" {
		cfg.ANC, cfg.Codec, cfg.Battery, cfg.Present = 4, 3, [3]byte{100, 100, 255}, 3
		cfg.Settings = nil
		for _, s := range [][2]uint32{{0x0d, 4}, {0x2e, 50}, {0x28, 1}, {0x26, 2}, {0x1f, 0x5032}, {0x17, 0}, {0x18, 0}, {0x23, 0}, {0x24, 0x0300}, {0x25, 1}, {0x3b, 1}, {0x1b, 1}, {0x35, 1}} {
			cfg.Settings = append(cfg.Settings, sim.Setting{Control: byte(s[0]), Age: 900, Revision: 3, Value: s[1]})
		}
	}
	switch value {
	case "disconnect":
		cfg.RoutingCloseDelay = simCloseDelay
	case "old":
		cfg.Capabilities &^= dongle.CapRouting
	case "legacy":
		cfg.LegacySnapshotOnly, cfg.ReconnectPolicyUnsupported = true, true
		delete(cfg.Cached, dongle.SlotEQ)
	}
	return cfg
}

// SimPicker returns a picker over simulated dongles for AIRPODS_GUI_SIM=several: two that open
// and one that cannot, or nil for any other value. It keeps to serials that say they are not real.
func SimPicker(value string) *device.Picker {
	if value != "several" {
		return nil
	}
	devices := map[string]*sim.Device{}
	var list []hid.Info
	for _, serial := range []string{"SIM0001", "SIM0002"} {
		cfg := SimConfig("")
		cfg.Serial = serial
		devices[serial] = sim.New(cfg)
		list = append(list, hid.Info{Path: "sim-" + serial, Serial: serial, PID: hid.ProductAudio, Openable: true})
	}
	list = append(list, hid.Info{Path: "sim-SIM0003", Serial: "SIM0003", PID: hid.ProductControl, Err: errors.New("no permission to open it; install the AirPods udev rule; see the docs")})
	return device.NewPicker(func() ([]hid.Info, error) { return list, nil }, func(serial string) (hid.Device, error) {
		dev, ok := devices[serial]
		if !ok {
			return nil, &hid.MatchError{Found: len(list), Text: "Expected exactly one"}
		}
		return dev.Open("")
	}, "", false)
}

// SimOpener returns an opener for the simulated dongle selected by the value of
// AIRPODS_GUI_SIM, or nil when the real dongle should be used.
func SimOpener(value string) device.OpenFunc {
	if value == "" || value == "0" || value == "several" {
		return nil
	}
	dev := sim.New(SimConfig(value))
	if value == "autopause" {
		go swapEars(dev)
	}
	return func() (hid.Device, error) { return dev.Open("") }
}

// swapEars plays the buds leaving and returning for the auto-pause demo, for the life of the
// process. It exists only behind AIRPODS_GUI_SIM.
func swapEars(dev *sim.Device) {
	state := byte(0)
	for revision := uint32(2); ; revision++ {
		time.Sleep(15 * time.Second)
		state ^= 1 // 0 in the ear, 1 out
		dev.Update(func(cfg *sim.Config) {
			cfg.Cached[dongle.SlotEarStatus] = sim.CachedReport{Age: 0, Revision: revision, Packet: sim.EarPacket(state, state)}
		})
	}
}
