package main

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/internal/cliutil"
)

const (
	nearbyDefaultSeconds = 30
	nearbyMaxSeconds     = 300
	nearbyPoll           = time.Second
	nearbySleepSlice     = 100 * time.Millisecond
	// nearbyStartWindow is how long a started finder may still read as idle.
	nearbyStartWindow = 5 * time.Second
	// nearbyFreshMs is how old a signal reading may be and still count.
	nearbyFreshMs = 10_000
)

func stopped(stop <-chan struct{}) bool {
	select {
	case <-stop:
		return true
	default:
		return false
	}
}

// pause waits on the clock in short slices, so that a stop is noticed quickly.
func pause(clock dongle.Clock, d time.Duration, stop <-chan struct{}) {
	for d > 0 && !stopped(stop) {
		slice := min(d, nearbySleepSlice)
		clock.Sleep(slice)
		d -= slice
	}
}

// refusedRequest tells a request the dongle turned down from one that may have applied.
func refusedRequest(err error) bool {
	var unsupported *dongle.Unsupported
	var rejected *dongle.RequestRejected
	return errors.As(err, &unsupported) || errors.As(err, &rejected)
}

// runNearby searches for a bounded time. Once a start has been sent, the finder is stopped
// on every way out of here, because the dongle would otherwise keep searching.
func runNearby(client *dongle.Client, seconds int, stdout, stderr io.Writer, stop <-chan struct{}) (err error) {
	if err := requireTyped(client); err != nil {
		return err
	}
	state, err := client.NearbySnapshot()
	if err != nil {
		return err
	}
	switch {
	case !state.AvailableIdentity:
		return errors.New("Searching is not available for this pair of AirPods yet")
	case state.Active:
		return errors.New("Another app is already searching; stop the search there first")
	case stopped(stop):
		return cliutil.ErrInterrupted
	}
	mayRun := true
	defer func() {
		if !mayRun {
			return
		}
		if stopErr := client.NearbyStop(); stopErr != nil {
			fmt.Fprintf(stderr, "nearby: could not stop the finder: %v. Replug the dongle if it keeps searching.\n", stopErr)
			if err == nil {
				err = errUnreported
			}
			return
		}
		fmt.Fprintln(stdout, "nearby: stopped")
	}()
	clock := client.Clock()
	started := clock.Now()
	end := started.Add(time.Duration(seconds) * time.Second)
	state, err = client.NearbyStart()
	if err != nil {
		mayRun = !refusedRequest(err)
		return err
	}
	seen := state.Active
	for clock.Now().Before(end) {
		if stopped(stop) {
			return cliutil.ErrInterrupted
		}
		if state.Active {
			seen = true
			if _, err := fmt.Fprintln(stdout, nearbyLine(state)); err != nil {
				return err
			}
		} else if seen {
			fmt.Fprintln(stdout, "nearby: the search ended on the dongle")
			return nil
		} else if clock.Now().Sub(started) >= nearbyStartWindow {
			return errors.New("The finder did not start")
		}
		pause(clock, nearbyPoll, stop)
		if stopped(stop) {
			return cliutil.ErrInterrupted
		}
		if state, err = client.NearbySnapshot(); err != nil {
			return err
		}
	}
	return nil
}

func nearbyLine(state dongle.NearbyState) string {
	switch {
	case state.Paused:
		return "nearby: paused while audio is playing"
	case state.LastHeardAgeMs == nil:
		return "nearby: waiting for the first signal"
	case state.RSSI == nil || *state.LastHeardAgeMs > nearbyFreshMs:
		return fmt.Sprintf("nearby: waiting for another signal; last heard %d ms ago", *state.LastHeardAgeMs)
	}
	strength := "weak"
	switch rssi := *state.RSSI; {
	case rssi >= -60:
		strength = "strong"
	case rssi >= -80:
		strength = "moderate"
	}
	return fmt.Sprintf("nearby: %d dBm, %s signal, trend %s, heard %d ms ago", *state.RSSI, strength, state.Trend, *state.LastHeardAgeMs)
}
