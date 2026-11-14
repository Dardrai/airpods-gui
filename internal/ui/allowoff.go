package ui

import "github.com/Dardrai/airpods-gui/core/model"

// The dongle forgets Allow Off whenever the AirPods connect again. For AirPods that never
// report it, it then refuses Off, as a mode and in the hold modes, until allow-off on is sent
// in the new connection generation. The app sends the remembered choice once per generation;
// a write of Off that comes first sends it itself, just before.

// AllowOffSent records an Allow Off value that went out in a connection generation.
func (w *Window) AllowOffSent(generation uint32, on bool) {
	w.allowOff = allowOffSent{generation: generation, on: on, known: true}
}

// AllowOffOnSent reports whether allow-off on went out in that generation.
func (w *Window) AllowOffOnSent(generation uint32) bool {
	return w.allowOff.known && w.allowOff.generation == generation && w.allowOff.on
}

// allowOffSent is the last Allow Off value sent, and in which generation.
type allowOffSent struct {
	generation uint32
	on, known  bool
}

// NeedsAllowOffFirst reports whether a write must be preceded by allow-off on, sent on the
// user's behalf: only when their choice for this pair is on, since the write would be
// refused otherwise. sent says whether allow-off on already went out in this generation.
func NeedsAllowOffFirst(view model.View, control byte, value uint32, sent bool) bool {
	return view.AllowOffRequested != nil && *view.AllowOffRequested && view.NeedsAllowOffResend(control, value, sent)
}
