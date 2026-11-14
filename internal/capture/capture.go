// Package capture records dongle diagnostics as JSON Lines, in the same format as airpods diag.
// It reads through dongle.Client.Sample and asks for nothing but what that tool asks for.
package capture

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/internal/device"
)

// Duration limits, as in airpods diag.
const (
	MinSeconds = 1
	MaxSeconds = 1800
)

// renewEvery keeps a running inspection alive; the firmware ends it by itself after 120 s.
const renewEvery = 10 * time.Second

// maxSkipped is how many samples in a row can be lost to another client holding the channel
// or to an unplugged dongle before the capture ends. At one sample a second that rides out a
// reconnect, and it stops a capture that has nothing left to record.
const maxSkipped = 30

// Options selects what a capture holds. Radio is a separate capture of cached radio and
// adaptive bitrate readings, without the others.
type Options struct {
	Seconds                      int
	Trace, Inspect, Phone, Radio bool
}

func (o Options) sample() dongle.SampleOptions {
	if o.Radio {
		return dongle.SampleOptions{Radio: true}
	}
	return dongle.SampleOptions{Trace: o.Trace, Inspect: o.Inspect, Phone: o.Phone}
}

func (o Options) cadence() time.Duration {
	if o.Radio {
		return 2 * time.Second
	}
	return time.Second
}

// Submit runs one job with exclusive use of the device, and waits for it to end.
type Submit func(job func(*dongle.Client) error) error

// Result is how a capture ended. Lines already written stay in the file in every case.
type Result struct {
	Lines   int
	Stopped bool
	// Skipped counts the samples lost to a busy channel or a missing dongle, each of which
	// left a marker line in the file. Marker lines are not part of Lines.
	Skipped int
	// Warning says that inspection could not be turned off; the firmware expires it.
	Warning string
	Err     error
}

// skipReason names a failure that loses one sample and not the capture, empty for any other.
func skipReason(err error) string {
	switch {
	case device.IsBusy(err):
		return "busy"
	case errors.Is(err, device.ErrNoDongle):
		return "no dongle"
	}
	return ""
}

// Run captures until the time is up, stop closes, or a read fails for good. Every sample is its own
// short job, and the file is written between jobs, so a slow disk cannot hold up the device.
// A running inspection is turned off on every way out.
func Run(submit Submit, clock dongle.Clock, output io.Writer, options Options, stop <-chan struct{}, progress func(Result)) (result Result) {
	opts := options.sample()
	inspecting := false
	defer func() {
		if !inspecting {
			return
		}
		if err := submit(func(c *dongle.Client) error {
			_, err := c.Transaction([]byte{14, 0})
			return err
		}); err != nil {
			result.Warning = "Inspection stop failed; firmware expires it after 120 seconds: " + err.Error()
		}
	}()
	if opts.Inspect {
		if err := submit(func(c *dongle.Client) error {
			_, err := c.Transaction([]byte{14, 1})
			return err
		}); err != nil {
			result.Err = err
			return result
		}
		inspecting = true
	}
	skipped := 0
	renewed := clock.Now()
	end := renewed.Add(time.Duration(options.Seconds) * time.Second)
	for clock.Now().Before(end) {
		select {
		case <-stop:
			result.Stopped = true
			return result
		default:
		}
		started := clock.Now()
		var sample dongle.Object
		err := submit(func(c *dongle.Client) error {
			if inspecting && started.Sub(renewed) >= renewEvery {
				if _, err := c.Transaction([]byte{14, 1}); err != nil {
					return err
				}
				renewed = started
			}
			var err error
			sample, err = c.Sample(opts)
			return err
		})
		switch {
		case errors.Is(err, device.ErrQueueFull):
			// Nothing ran; the next sample is due soon enough.
		case skipReason(err) != "":
			skipped++
			result.Skipped++
			marker := dongle.Object{{Key: "time", Value: float64(clock.Now().UnixNano()) / 1e9}, {Key: "skipped", Value: skipReason(err)}, {Key: "consecutive", Value: skipped}}
			if _, err := fmt.Fprintln(output, marker.String()); err != nil {
				result.Err = err
				return result
			}
			if skipped >= maxSkipped {
				result.Err = fmt.Errorf("%d samples in a row failed: %w", skipped, err)
				return result
			}
		case err != nil:
			result.Err = err
			return result
		default:
			skipped = 0
			if _, err := fmt.Fprintln(output, sample.String()); err != nil {
				result.Err = err
				return result
			}
			result.Lines++
		}
		if progress != nil {
			progress(result)
		}
		sleep(clock, options.cadence()-clock.Now().Sub(started), stop)
	}
	return result
}

// sleep waits on the clock, returning early when stop closes.
func sleep(clock dongle.Clock, d time.Duration, stop <-chan struct{}) {
	if d <= 0 {
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
