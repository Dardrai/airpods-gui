package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/Dardrai/airpods-gui/core/disconnect"
	"github.com/Dardrai/airpods-gui/core/dongle"
)

// releaseHold is what a release leaves behind. It is not saved: a restart of the dongle ends it.
const releaseHold = "The dongle now declines the AirPods' incoming connections and does not reconnect by itself until airpods connect, airpods reconnect, a press of its button or a restart of the dongle."

// intentChangedHint names what can move the routing intent first.
const intentChangedHint = "Something else changed the connection first: a press of the dongle's button, another app, or an automatic reconnect under the auto reconnect policy."

// autoPolicyWarning goes with every outcome that leaves the auto policy saved.
const autoPolicyWarning = "Warning: under the auto policy the dongle connects to the AirPods by itself whenever they are not connected and not released, and can take them from another device using them. airpods disconnect holds it off only until the dongle restarts."

// runConnect asks for the AirPods through the routing handover and waits for them to confirm the
// dongle. Firmware without the handover gets the plain reconnect request, which only says that
// the dongle took it.
func runConnect(client *dongle.Client, stdout io.Writer) error {
	outcome, err := disconnect.Acquire(client)
	switch {
	case errors.Is(err, disconnect.ErrAcquireUnsupported):
		if err := client.Reconnect(); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "connect: AirPods connection requested; not yet confirmed. Use airpods status to check the connection.")
		return nil
	case errors.Is(err, disconnect.ErrIntentChanged):
		return fmt.Errorf("%w. %s Run airpods status before you connect again", err, intentChangedHint)
	case err != nil:
		return err
	case outcome == disconnect.AcquirePending:
		fmt.Fprintln(stdout, "connect: takeover accepted, still in progress. Use airpods status to follow it.")
		return nil
	}
	fmt.Fprintln(stdout, "connect: AirPods connected; audio ownership confirmed.")
	return nil
}

// runReconnectPolicy reads the saved reconnect policy, or saves another one and checks the value
// the dongle echoes. A save that may have happened is never retried.
func runReconnectPolicy(client *dongle.Client, word string, given bool, stdout, stderr io.Writer) error {
	if err := requireTyped(client); err != nil {
		return err
	}
	if !given {
		policy, err := client.ReconnectPolicy()
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, "Reconnect policy: "+policy.String())
		return nil
	}
	policy := dongle.ReconnectManual
	if word == "auto" {
		policy = dongle.ReconnectAutomatic
	}
	changed, err := client.SetReconnectPolicy(policy)
	if err != nil {
		return err
	}
	if changed {
		fmt.Fprintf(stdout, "Reconnect policy: %s (saved and verified)\n", policy)
	} else {
		fmt.Fprintf(stdout, "Reconnect policy: %s (no change needed; no write sent)\n", policy)
	}
	if policy == dongle.ReconnectAutomatic {
		fmt.Fprintln(stderr, autoPolicyWarning)
	}
	return nil
}
