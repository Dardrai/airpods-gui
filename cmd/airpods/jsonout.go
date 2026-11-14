package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Dardrai/airpods-gui/core/dongle"
)

// jsonSchema is the schema field of every JSON document. docs/CLI.md describes the
// schema; a change that breaks a reader of it must raise this number.
const jsonSchema = 1

type jsonStatus struct {
	Schema        int         `json:"schema"`
	State         string      `json:"state"`
	SettingsReady bool        `json:"settings_ready"`
	Codec         string      `json:"codec"`
	Batteries     jsonBattery `json:"batteries"`
}

type jsonBattery struct {
	Left  *int `json:"left"`
	Right *int `json:"right"`
	Case  *int `json:"case"`
}

type jsonSetting struct {
	Name    string  `json:"name"`
	Raw     *uint32 `json:"raw"`
	Decoded *string `json:"decoded"`
	AgeMs   *uint32 `json:"age_ms"`
	Unknown bool    `json:"unknown"`
}

type jsonDongle struct {
	Serial    *string `json:"serial"`
	USBID     string  `json:"usb_id"`
	Path      string  `json:"path"`
	Connected *bool   `json:"connected"`
	Error     *string `json:"error"`
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false) // Windows device paths hold an ampersand.
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func batteryValue(raw byte) *int {
	if raw > 100 {
		return nil
	}
	value := int(raw)
	return &value
}

func statusJSON(status dongle.Status) jsonStatus {
	return jsonStatus{
		Schema:        jsonSchema,
		State:         pick(status.Connected, "connected", "disconnected"),
		SettingsReady: status.ControlsReady,
		Codec:         status.CodecName(),
		Batteries:     jsonBattery{batteryValue(status.BatteryLeft), batteryValue(status.BatteryRight), batteryValue(status.BatteryCase)},
	}
}

// settingJSON leaves decoded null for a value this tool cannot name, and raw too when the
// device never reported the setting; unknown is true in both cases.
func settingJSON(setting *dongle.Setting, values map[byte]dongle.Record) jsonSetting {
	result := jsonSetting{Name: setting.Name, Unknown: true}
	record, ok := values[setting.Control]
	if !ok {
		return result
	}
	raw, age := record.Value, record.AgeMs
	result.Raw, result.AgeMs = &raw, &age
	if decoded := setting.Decode(raw); !strings.HasPrefix(decoded, "unknown device value") {
		result.Decoded, result.Unknown = &decoded, false
	}
	return result
}

func listJSON(rows []dongleRow) any {
	dongles := make([]jsonDongle, 0, len(rows))
	for _, row := range rows {
		entry := jsonDongle{USBID: fmt.Sprintf("%04x", row.info.PID), Path: row.info.Path, Connected: row.connected}
		if row.info.Serial != "" {
			serial := row.info.Serial
			entry.Serial = &serial
		}
		if row.reason != "" {
			reason := row.reason
			entry.Error = &reason
		}
		dongles = append(dongles, entry)
	}
	return struct {
		Schema  int          `json:"schema"`
		Dongles []jsonDongle `json:"dongles"`
	}{jsonSchema, dongles}
}
