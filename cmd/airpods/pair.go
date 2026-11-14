package main

import (
	"fmt"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/internal/argparse"
)

const pairWarning = "Warning: this erases the saved pairing on the dongle. The AirPods paired now will have to be paired again to come back."

// pairRefused is what Pair itself refuses before it sends anything.
func pairRefused(status dongle.Status) bool {
	return !status.Has(dongle.CapControl) || status.USBPlayback || status.MicrophoneActive || status.PhoneConnected
}

// pairJob warns and asks unless a refusal is certain, so that a pair that would be refused asks
// nothing. Pair then gives that refusal, as it did before there was a question.
func pairJob(ctx jobContext) (jobFunc, *argparse.Exit) {
	yes := ctx.ns.Bool("yes")
	return func(client *dongle.Client) error {
		status, err := client.Status()
		if err != nil {
			return err
		}
		if !pairRefused(status) {
			fmt.Fprintln(ctx.stderr, pairWarning)
			if !yes {
				if err := confirmHearing("Replace the saved pairing?", "pairing"); err != nil {
					return err
				}
			}
		}
		if err := client.Pair(); err != nil {
			return err
		}
		fmt.Fprintln(ctx.stdout, "pair: accepted by dongle")
		return nil
	}, nil
}
