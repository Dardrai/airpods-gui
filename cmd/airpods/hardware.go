package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/hearing"
	"github.com/Dardrai/airpods-gui/internal/argparse"
	"github.com/Dardrai/airpods-gui/internal/cliutil"
)

// checkProgressEvery is how often a waiting check prints the time left between its changes.
const checkProgressEvery = 15 * time.Second

// withHardwareSession applies the read gates, then runs one leased sequence.
func withHardwareSession(client *dongle.Client, run func(number string, session *dongle.Session) error) error {
	status, err := client.Status()
	if err != nil {
		return err
	}
	switch {
	case !status.Connected || !status.ControlsReady:
		return errors.New("Connect the dongle's AirPods first.")
	case status.MicrophoneActive:
		return errors.New("Stop recording first; these reads wait until the microphone is free.")
	case !status.Has(dongle.CapLease) || !status.Has(dongle.CapRawTransport):
		return errors.New("Update the dongle firmware first; it cannot serve these reads.")
	}
	meta, _, err := readMetadata(client)
	if err != nil {
		return err
	}
	if !hearing.Supported(meta.Model) {
		return &hearing.Refused{Reason: "Not available on these AirPods."}
	}
	session, err := client.OpenSession()
	if err != nil {
		return err
	}
	defer session.Close()
	if err := session.AcquireLease(); err != nil {
		return err
	}
	return run(meta.Model, session)
}

type jsonEar struct {
	Result            string   `json:"result"`
	MeasuredAtSeconds *uint64  `json:"measured_at_seconds"`
	MeasuredAt        *string  `json:"measured_at"`
	Failures          []string `json:"failures"`
}

type jsonDiagnostics struct {
	Schema int      `json:"schema"`
	Saved  bool     `json:"saved"`
	ReadAt *string  `json:"read_at"`
	Left   *jsonEar `json:"left"`
	Right  *jsonEar `json:"right"`
}

func earJSON(ear hearing.EarResult) *jsonEar {
	entry := &jsonEar{Result: ear.Text(), MeasuredAt: nullable(hearing.When(ear.MeasuredAtSeconds)), Failures: []string{}}
	if ear.MeasuredAtSeconds != 0 {
		seconds := ear.MeasuredAtSeconds
		entry.MeasuredAtSeconds = &seconds
	}
	entry.Failures = append(entry.Failures, ear.Failures...)
	return entry
}

func diagnosticsJSON(record *hearing.Diagnostics) jsonDiagnostics {
	if record == nil {
		return jsonDiagnostics{Schema: jsonSchema}
	}
	readAt := record.ReadAt.Format(time.RFC3339)
	return jsonDiagnostics{Schema: jsonSchema, Saved: true, ReadAt: &readAt, Left: earJSON(record.Left), Right: earJSON(record.Right)}
}

func printDiagnostics(ctx jobContext, record *hearing.Diagnostics) {
	if record == nil {
		fmt.Fprintln(ctx.stdout, "The AirPods have no saved hardware results.")
		return
	}
	for _, ear := range []struct {
		name   string
		result hearing.EarResult
	}{{"Left", record.Left}, {"Right", record.Right}} {
		fmt.Fprintf(ctx.stdout, "%s: %s\n", ear.name, ear.result.Text())
		fmt.Fprintf(ctx.stdout, "%s measured: %s\n", ear.name, reportedOr(hearing.When(ear.result.MeasuredAtSeconds)))
	}
	fmt.Fprintln(ctx.stdout, "Read at: "+record.ReadAt.Format("15:04:05"))
}

func hardwareStatusJob(ctx jobContext) (jobFunc, *argparse.Exit) {
	asJSON := ctx.ns.Bool("json")
	return func(client *dongle.Client) error {
		return withHardwareSession(client, func(number string, session *dongle.Session) error {
			record, err := hearing.ReadDiagnostics(session.Exchange, number, client.Clock().Now())
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(ctx.stdout, diagnosticsJSON(record))
			}
			printDiagnostics(ctx, record)
			return nil
		})
	}, nil
}

type jsonDeviceData struct {
	Schema      int       `json:"schema"`
	Available   bool      `json:"available"`
	ReadAt      string    `json:"read_at"`
	TestMode    *byte     `json:"test_mode"`
	Sensitivity *float32  `json:"sensitivity"`
	VolumeCurve []float32 `json:"volume_curve"`
}

func hardwareDataJob(ctx jobContext) (jobFunc, *argparse.Exit) {
	asJSON := ctx.ns.Bool("json")
	return func(client *dongle.Client) error {
		return withHardwareSession(client, func(_ string, session *dongle.Session) error {
			data, err := hearing.ReadDeviceData(session.Exchange, client.Clock().Now())
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(ctx.stdout, jsonDeviceData{
					Schema: jsonSchema, Available: data.AnyCharacteristic, ReadAt: data.ReadAt.Format(time.RFC3339),
					TestMode: data.TestMode, Sensitivity: data.Sensitivity, VolumeCurve: data.VolumeCurve,
				})
			}
			if !data.AnyCharacteristic {
				fmt.Fprintln(ctx.stdout, "The AirPods expose none of this data.")
				return nil
			}
			testMode, sensitivity, curve := "not reported", "not reported", "not reported"
			if data.TestMode != nil {
				testMode = strconv.Itoa(int(*data.TestMode))
			}
			if data.Sensitivity != nil {
				sensitivity = strconv.FormatFloat(float64(*data.Sensitivity), 'g', 6, 32)
			}
			if data.VolumeCurve != nil {
				parts := make([]string, len(data.VolumeCurve))
				for i, value := range data.VolumeCurve {
					parts[i] = fmt.Sprintf("%.3g", value)
				}
				curve = strings.Join(parts, " ")
			}
			fmt.Fprintln(ctx.stdout, "Hearing test mode: "+testMode)
			fmt.Fprintln(ctx.stdout, "Dosimetry sensitivity: "+sensitivity)
			fmt.Fprintln(ctx.stdout, "Volume curve: "+curve)
			fmt.Fprintln(ctx.stdout, "These are raw values. They do not establish a sound level.")
			fmt.Fprintln(ctx.stdout, "Read at: "+data.ReadAt.Format("15:04:05"))
			return nil
		})
	}, nil
}

func checkPhaseText(progress hearing.CheckProgress) string {
	switch progress.Phase {
	case hearing.CheckWaitingCaseClose:
		return "close the case now"
	case hearing.CheckWaitingResult:
		return "waiting for the result; the AirPods disconnect during the check, which is expected"
	case hearing.CheckCompleted:
		return "new results arrived"
	case hearing.CheckTimedOut:
		return "no new results in time"
	case hearing.CheckStopped:
		return "stopped waiting"
	}
	return string(progress.Phase)
}

func earWord(updated bool) string { return pick(updated, "new result", "waiting") }

// hardwareCheckJob asks for the in-case check once. Ctrl-C only stops the waiting: no cancel
// command is known, so the AirPods may still finish.
func hardwareCheckJob(ctx jobContext) (jobFunc, *argparse.Exit) {
	confirmed := ctx.ns.Bool("buds_in_case")
	return func(client *dongle.Client) error {
		if err := hearing.HardwareCheckReady(client, true); err != nil {
			return err
		}
		if !confirmed {
			accepted, available := askUser("Are both AirPods in the open case, with the lid open and not in your ears?")
			switch {
			case !available:
				return errors.New("Nothing was sent: confirm with --buds-in-case that both AirPods are in the open case, or run this in a terminal.")
			case !accepted:
				return errors.New("Nothing was sent: not confirmed.")
			}
		}
		// Taken only now, so that Ctrl-C still ends the process at the question above.
		var stop <-chan struct{}
		if ctx.env.interrupt != nil {
			stop = ctx.env.interrupt()
		}
		var last hearing.CheckProgress
		var lastLine time.Duration
		final, err := hearing.RunHardwareCheck(client, hearing.HardwareCheckOptions{
			BudsInCaseConfirmed: true,
			Stop:                func() bool { return stopped(stop) },
			Progress: func(progress hearing.CheckProgress) {
				changed := progress.Phase != last.Phase || progress.LeftUpdated != last.LeftUpdated || progress.RightUpdated != last.RightUpdated
				if !changed && progress.Elapsed-lastLine < checkProgressEvery {
					return
				}
				last, lastLine = progress, progress.Elapsed
				fmt.Fprintf(ctx.stdout, "check: %s; left %s, right %s; %d s left\n", checkPhaseText(progress), earWord(progress.LeftUpdated), earWord(progress.RightUpdated), int(progress.Remaining.Seconds()))
			},
		})
		if err != nil {
			var unconfirmed *dongle.Unconfirmed
			if errors.As(err, &unconfirmed) {
				return &dongle.Unconfirmed{Message: err.Error() + " Do not start another check yet; read the results with airpods hardware status.", Err: err}
			}
			return err
		}
		switch final.Phase {
		case hearing.CheckCompleted:
			printDiagnostics(ctx, final.Record)
			fmt.Fprintln(ctx.stdout, "These are what the AirPods saved from their own self-check, not a medical or repair diagnosis.")
			return nil
		case hearing.CheckStopped:
			fmt.Fprintln(ctx.stderr, "Stopped waiting. The AirPods may still finish the check; read the results later with airpods hardware status.")
			return cliutil.ErrInterrupted
		}
		return errors.New("No new results in 2 minutes. The AirPods may still finish the check; read the results later with airpods hardware status.")
	}, nil
}

func hardwareJob(ctx jobContext) (jobFunc, *argparse.Exit) {
	subcommand, _ := ctx.ns.String("subcommand")
	switch subcommand {
	case "status":
		return hardwareStatusJob(ctx)
	case "data":
		return hardwareDataJob(ctx)
	}
	return hardwareCheckJob(ctx)
}
