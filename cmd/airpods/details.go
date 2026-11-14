package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/internal/cliutil"
)

// Battery freshness, as the tray app reads it: a reading is current up to 30 s, or under 90 s when
// it came from the case, and only for a bud that is present.
const (
	freshEarbudMs = 30_000
	freshCaseMs   = 90_000
	unknownAgeMs  = 0xffffffff
)

type jsonBatteryDetail struct {
	Level    *int    `json:"level"`
	Current  bool    `json:"current"`
	Charging bool    `json:"charging"`
	AgeMs    *uint32 `json:"age_ms"`
}

type jsonDetails struct {
	DongleSerial   *string           `json:"dongle_serial"`
	AirPodsAddress *string           `json:"airpods_address"`
	NoiseControl   string            `json:"noise_control"`
	InUse          []string          `json:"in_use"`
	Capabilities   []string          `json:"dongle_capabilities"`
	Left           jsonBatteryDetail `json:"battery_left"`
	Right          jsonBatteryDetail `json:"battery_right"`
	Case           jsonBatteryDetail `json:"battery_case"`
}

func batteryDetail(status dongle.Status, bit byte, level byte, age uint32) jsonBatteryDetail {
	detail := jsonBatteryDetail{Level: batteryValue(level)}
	if age != unknownAgeMs {
		detail.AgeMs = &age
	}
	present := !status.Has(dongle.CapPerComponent) || status.PresentMask&bit != 0
	if detail.Level != nil && detail.AgeMs != nil && present {
		limit := uint32(freshEarbudMs) + 1
		if status.CaseSourceMask&bit != 0 {
			limit = freshCaseMs
		}
		detail.Current = age < limit
	}
	detail.Charging = detail.Current && status.ChargingMask&bit != 0
	return detail
}

func batteryDetailText(detail jsonBatteryDetail) string {
	switch {
	case detail.Level == nil:
		return "unknown"
	case detail.Charging:
		return fmt.Sprintf("%d%% (charging)", *detail.Level)
	case !detail.Current && detail.AgeMs != nil:
		return fmt.Sprintf("%d%% (%d s old)", *detail.Level, *detail.AgeMs/1000)
	case !detail.Current:
		return fmt.Sprintf("%d%% (last known)", *detail.Level)
	}
	return fmt.Sprintf("%d%%", *detail.Level)
}

func statusDetails(status dongle.Status, serial string) jsonDetails {
	details := jsonDetails{
		DongleSerial: nullable(serial),
		NoiseControl: "unknown",
		InUse:        []string{},
		Capabilities: append([]string{}, dongle.CapabilityNames(status.Capabilities)...),
		Left:         batteryDetail(status, 1, status.BatteryLeft, status.AgeLeft),
		Right:        batteryDetail(status, 2, status.BatteryRight, status.AgeRight),
		Case:         batteryDetail(status, 4, status.BatteryCase, status.AgeCase),
	}
	if status.Connected && status.Selected() {
		p := status.Peer
		address := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X", p[0], p[1], p[2], p[3], p[4], p[5])
		details.AirPodsAddress = &address
	}
	if status.ANC >= 1 && status.ANC <= 4 {
		details.NoiseControl = [...]string{"off", "anc", "transparency", "adaptive"}[status.ANC-1]
	}
	for _, flag := range []struct {
		on   bool
		name string
	}{
		{status.Streaming, "streaming"}, {status.USBPlayback, "USB playback"}, {status.MicrophoneActive, "microphone"},
		{status.PhoneConnected, "phone connected"}, {status.PhonePairing, "phone pairing open"},
	} {
		if flag.on {
			details.InUse = append(details.InUse, flag.name)
		}
	}
	return details
}

// runStatusDetails is status --details: what the tray app's Diagnostics section lists as its state.
// It reads the status report only, so it works next to another client too. The address is the
// AirPods' own, so it stays behind this flag.
func runStatusDetails(client *dongle.Client, serial string, asJSON bool, stdout io.Writer) error {
	status, err := client.Status()
	if err != nil {
		return err
	}
	details := statusDetails(status, serial)
	if asJSON {
		return writeJSON(stdout, struct {
			jsonStatus
			Details jsonDetails `json:"details"`
		}{statusJSON(status), details})
	}
	inUse := "nothing"
	if len(details.InUse) > 0 {
		inUse = strings.Join(details.InUse, ", ")
	}
	capabilities := "none"
	if len(details.Capabilities) > 0 {
		capabilities = strings.Join(details.Capabilities, ", ")
	}
	address := "none"
	if details.AirPodsAddress != nil {
		address = *details.AirPodsAddress
	}
	fmt.Fprintln(stdout, "AirPods: "+pick(status.Connected, "connected", "disconnected"))
	fmt.Fprintln(stdout, "Settings: "+pick(status.ControlsReady, "ready", "not ready"))
	fmt.Fprintln(stdout, "Codec: "+status.CodecName())
	fmt.Fprintln(stdout, "Battery left: "+batteryDetailText(details.Left))
	fmt.Fprintln(stdout, "Battery right: "+batteryDetailText(details.Right))
	fmt.Fprintln(stdout, "Battery case: "+batteryDetailText(details.Case))
	fmt.Fprintln(stdout, "Dongle serial: "+reportedOr(serial))
	fmt.Fprintln(stdout, "AirPods address: "+address)
	fmt.Fprintln(stdout, "Noise control: "+details.NoiseControl)
	fmt.Fprintln(stdout, "In use: "+inUse)
	fmt.Fprintln(stdout, "Dongle capabilities: "+capabilities)
	return nil
}

// debugDevice prints what goes over the request channel: sizes and codes, never payloads, so that
// no AirPods address or serial ends up in a bug report. Identical lines in a row print once.
type debugDevice struct {
	hid.Device
	out   io.Writer
	clock dongle.Clock
	last  string
}

func wrapDebug(open cliutil.OpenFunc, out io.Writer, clock dongle.Clock) cliutil.OpenFunc {
	return func(serial string) (hid.Device, error) {
		device, err := open(serial)
		if err != nil {
			fmt.Fprintf(out, "debug: open failed: %v\n", err)
			return nil, err
		}
		fmt.Fprintln(out, "debug: open")
		return &debugDevice{Device: device, out: out, clock: clock}, nil
	}
}

func (d *debugDevice) printf(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	if line == d.last {
		return
	}
	d.last = line
	fmt.Fprintf(d.out, "debug: %s %s\n", d.clock.Now().Format(time.TimeOnly), line)
}

func (d *debugDevice) Get(report byte) ([hid.ReportSize]byte, error) {
	data, err := d.Device.Get(report)
	if err != nil {
		d.printf("get report %d failed: %v", report, err)
		return data, err
	}
	le := binary.LittleEndian
	switch report {
	case 2: // Flags, generation, mode and capabilities only: bytes 12..17 hold the address.
		d.printf("status flags=%02x generation=%d mode=%d codec=%d capabilities=%04x", data[7], le.Uint32(data[8:]), data[18], data[23], le.Uint16(data[42:]))
	case 3:
		d.printf("<- op=%d generation=%d seq=%d size=%d phase=%d", data[7], le.Uint32(data[8:]), le.Uint32(data[12:]), data[20], data[21])
	}
	return data, nil
}

func (d *debugDevice) Set(data [hid.ReportSize]byte) error {
	le := binary.LittleEndian
	switch data[0] {
	case 2:
		d.printf("-> control opcode=%d seq=%d", data[7], le.Uint32(data[18:]))
	case 3:
		d.printf("-> op=%d generation=%d seq=%d size=%d", data[7], le.Uint32(data[8:]), le.Uint32(data[12:]), data[20])
	}
	err := d.Device.Set(data)
	if err != nil {
		d.printf("set failed: %v", err)
	}
	return err
}
