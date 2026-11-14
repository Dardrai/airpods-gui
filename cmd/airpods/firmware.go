package main

import (
	"fmt"
	"io"

	"github.com/Dardrai/airpods-gui/core/dongle"
)

// runFirmware prints the version of the firmware the dongle runs. It is read-only.
func runFirmware(client *dongle.Client, stdout io.Writer) error {
	if err := requireTyped(client); err != nil {
		return err
	}
	info, err := client.FirmwareInfo()
	if err != nil {
		return err
	}
	line := "Firmware: " + info.String()
	if info.Experimental {
		line += " (experimental radio0076)"
	}
	_, err = fmt.Fprintln(stdout, line)
	return err
}
