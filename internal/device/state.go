// Package device owns the dongle: it opens it, polls its status report and runs
// every other device access on one goroutine.
package device

import "github.com/Dardrai/airpods-gui/core/dongle"

// State is what the rest of the app knows about the dongle.
type State struct {
	// Present means a dongle is open and its status report was read.
	Present bool
	// Status is the last status report; meaningful only when Present.
	Status dongle.Status
	// Serial is the open dongle's USB serial.
	Serial string
	// Pending means a command was sent recently and status is polled faster.
	Pending bool
	// Problem is a visible error, such as no permission to open the dongle.
	Problem string
	// Several means more than one dongle is attached.
	Several bool
	// ChooseDongle means several are attached and the user has to pick one.
	ChooseDongle bool
}

// Connected reports whether the AirPods are connected to a present dongle.
func (s State) Connected() bool { return s.Present && s.Status.Connected }

// Ready reports whether the AirPods accept control requests.
func (s State) Ready() bool { return s.Connected() && s.Status.ControlsReady }
