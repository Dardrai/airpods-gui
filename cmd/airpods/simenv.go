package main

import (
	"io"

	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/internal/cliutil"
)

// simEnvironment replaces the dongle when value asks for a simulated one; see cliutil.SimDevice.
func simEnvironment(value string, stderr io.Writer, env environment) environment {
	device, cfg := cliutil.SimDevice(value, stderr)
	if device == nil {
		return env
	}
	env.open = device.Open
	env.enumerate = func() ([]hid.Info, error) {
		return []hid.Info{{Path: "simulated", Serial: cfg.Serial, PID: hid.ProductAudio, Openable: true}}, nil
	}
	return env
}
