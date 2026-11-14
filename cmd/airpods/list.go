package main

import (
	"fmt"
	"io"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/hid"
)

// dongleRow is one line of airpods list.
type dongleRow struct {
	info      hid.Info
	connected *bool  // the AirPods link; nil when the dongle could not be asked
	reason    string // why it could not be asked; empty otherwise
}

// runList reports every attached dongle. Only the status report is read, so it neither
// disturbs a request in progress nor depends on one. A dongle that cannot be used is a
// result here, not a failure.
func runList(serial string, asJSON bool, stdout, stderr io.Writer, env environment) int {
	rows, err := listDongles(serial, env)
	if err != nil {
		return report(err, stderr)
	}
	if asJSON {
		if err := writeJSON(stdout, listJSON(rows)); err != nil {
			return report(err, stderr)
		}
		return 0
	}
	printList(rows, serial, stdout)
	return 0
}

// runSerials prints the serials for scripts and shell completion, without opening any
// dongle for its status. Names outside a plain alphabet are left out, because completion
// would put them on a command line.
func runSerials(serial string, stdout, stderr io.Writer, env environment) int {
	infos, err := env.enumerate()
	if err != nil {
		return report(err, stderr)
	}
	seen := map[string]bool{}
	for _, info := range infos {
		if info.Serial == "" || seen[info.Serial] || !plainSerial(info.Serial) || (serial != "" && info.Serial != serial) {
			continue
		}
		seen[info.Serial] = true
		fmt.Fprintln(stdout, info.Serial)
	}
	return 0
}

func plainSerial(serial string) bool {
	for _, r := range serial {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}

// listDongles enumerates, keeps the dongles matching serial if one is given, and asks each
// openable one for its connection. Every dongle is closed before the next is opened.
func listDongles(serial string, env environment) ([]dongleRow, error) {
	infos, err := env.enumerate()
	if err != nil {
		return nil, err
	}
	var rows []dongleRow
	sameSerial := map[string]int{}
	openable := 0
	for _, info := range infos {
		if serial != "" && info.Serial != serial {
			continue
		}
		rows = append(rows, dongleRow{info: info})
		sameSerial[info.Serial]++
		if info.Openable {
			openable++
		}
	}
	for i := range rows {
		row := &rows[i]
		switch {
		case !row.info.Openable:
			row.reason = "cannot be opened"
			if row.info.Err != nil {
				row.reason = "cannot be opened: " + row.info.Err.Error()
			}
		case row.info.Serial == "" && openable > 1:
			row.reason = "cannot be selected: it has no USB serial"
		case row.info.Serial != "" && sameSerial[row.info.Serial] > 1:
			row.reason = "cannot be selected: its USB serial is not unique"
		default:
			connected, err := probe(row.info.Serial, env)
			if err != nil {
				row.reason = "cannot be opened: " + err.Error()
			} else {
				row.connected = &connected
			}
		}
	}
	return rows, nil
}

func probe(serial string, env environment) (bool, error) {
	device, err := env.open(serial)
	if err != nil {
		return false, err
	}
	defer device.Close()
	client := dongle.NewClient(device)
	client.SetClock(env.clock)
	status, err := client.Status()
	if err != nil {
		return false, err
	}
	return status.Connected, nil
}

func printList(rows []dongleRow, serial string, stdout io.Writer) {
	if len(rows) == 0 {
		if serial != "" {
			fmt.Fprintf(stdout, "No dongle with serial %s found.\n", serial)
		} else {
			fmt.Fprintln(stdout, "No dongles found.")
		}
		return
	}
	serialWidth, pathWidth := 0, 0
	for _, row := range rows {
		serialWidth = max(serialWidth, len(serialText(row.info)))
		pathWidth = max(pathWidth, len(row.info.Path))
	}
	for _, row := range rows {
		state := row.reason
		if row.connected != nil {
			state = pick(*row.connected, "connected", "not connected")
		}
		fmt.Fprintf(stdout, "%-*s  %04x  %-*s  %s\n", serialWidth, serialText(row.info), row.info.PID, pathWidth, row.info.Path, state)
	}
	if len(rows) > 1 {
		fmt.Fprintln(stdout, "Several dongles attached. Pick one with: airpods --serial SERIAL COMMAND")
	}
}

// serialText keeps the columns whitespace-separated when a dongle reports no serial.
func serialText(info hid.Info) string {
	if info.Serial == "" {
		return "-"
	}
	return info.Serial
}
