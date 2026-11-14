package cliutil

import (
	"fmt"
	"io"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
)

// SimEnv names the variable that replaces the dongle with a simulated one.
const SimEnv = "AIRPODS_CLI_SIM"

// SimNote is what a run on the simulated dongle says first, so that its results are not taken
// for those of real AirPods. It goes to stderr, which keeps JSON on stdout clean.
const SimNote = "note: simulated dongle (" + SimEnv + ")"

// SimDevice gives a simulated dongle for a value of AIRPODS_CLI_SIM, to try the commands with no
// hardware: "case" has both buds in the case, so that the in-case check can start, and any other
// value has them in the ears. An empty value or 0 gives nil, for the real dongle. It lasts one
// run, so a change is gone by the next command.
func SimDevice(value string, stderr io.Writer) (*sim.Device, sim.Config) {
	if value == "" || value == "0" {
		return nil, sim.Config{}
	}
	fmt.Fprintln(stderr, SimNote)
	cfg := sim.Demo()
	if value == "case" {
		cfg.Cached[dongle.SlotEarStatus] = sim.CachedReport{Age: 1000, Revision: 1, Packet: sim.EarPacket(2, 2)}
		cfg.Flags = 0x03 // Connected and ready, not streaming.
	}
	return sim.New(cfg), cfg
}
