package main

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/internal/cliutil"
)

// sleep waits on the clock, returning early when stop closes.
func sleep(clock dongle.Clock, d time.Duration, stop <-chan struct{}) {
	if d <= 0 {
		clock.Sleep(0)
		return
	}
	done := make(chan struct{})
	go func() {
		clock.Sleep(d)
		close(done)
	}()
	select {
	case <-done:
	case <-stop:
	}
}

// mayHaveStarted says whether a failed start can have left inspection running: the request
// went out and the dongle did not refuse it, as when only the reply was lost.
func mayHaveStarted(err error) bool {
	if errors.Is(err, dongle.ErrNotSent) {
		return false
	}
	var unsupported *dongle.Unsupported
	if errors.As(err, &unsupported) {
		return false
	}
	var rejected *dongle.RequestRejected
	if errors.As(err, &rejected) && (rejected.Code == 1 || rejected.Code == 2 || rejected.Code == 4) {
		return false
	}
	return true
}

func capture(client *dongle.Client, clock dongle.Clock, seconds int, opts dongle.SampleOptions, output, stderr io.Writer, stop <-chan struct{}) (err error) {
	inspecting := false
	defer func() {
		if !inspecting {
			return
		}
		if _, stopErr := client.Transaction([]byte{14, 0}); stopErr != nil {
			fmt.Fprintln(stderr, "Inspection stop failed; firmware expires it after 120 seconds: "+stopErr.Error())
		}
	}()
	if opts.Inspect {
		if _, err := client.Transaction([]byte{14, 1}); err != nil {
			inspecting = mayHaveStarted(err)
			return err
		}
		inspecting = true
	}
	cadence := time.Second
	if opts.Radio {
		cadence = 2 * time.Second
	}
	renewed := clock.Now()
	end := renewed.Add(time.Duration(seconds) * time.Second)
	for clock.Now().Before(end) {
		select {
		case <-stop:
			return cliutil.ErrInterrupted
		default:
		}
		started := clock.Now()
		if inspecting && started.Sub(renewed) >= 10*time.Second {
			if _, err := client.Transaction([]byte{14, 1}); err != nil {
				return err
			}
			renewed = started
		}
		sample, err := client.Sample(opts)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(output, sample.String()); err != nil {
			return err
		}
		sleep(clock, cadence-clock.Now().Sub(started), stop)
	}
	return nil
}
