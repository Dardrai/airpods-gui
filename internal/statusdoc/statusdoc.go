// Package statusdoc is what "Copy status as JSON" copies and what Diagnostics lists: the fields
// of `airpods --json status` and `airpods --json settings` from the CLI, schema 1, in one document.
package statusdoc

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Dardrai/airpods-gui/core/dongle"
)

// Schema is the CLI's JSON schema version these fields follow.
const Schema = 1

// Batteries are percentages; nil, null in the JSON, means no reading.
type Batteries struct {
	Left  *int `json:"left"`
	Right *int `json:"right"`
	Case  *int `json:"case"`
}

// Setting is one setting as the AirPods last reported it. Raw and AgeMs are nil when it was not
// reported. Decoded is nil, and Unknown true, when it was not reported or has no name here.
type Setting struct {
	Name    string  `json:"name"`
	Raw     *uint32 `json:"raw"`
	Decoded *string `json:"decoded"`
	AgeMs   *uint32 `json:"age_ms"`
	Unknown bool    `json:"unknown"`
}

// Document is the whole thing.
type Document struct {
	Schema        int       `json:"schema"`
	State         string    `json:"state"`
	SettingsReady bool      `json:"settings_ready"`
	Codec         string    `json:"codec"`
	Batteries     Batteries `json:"batteries"`
	Settings      []Setting `json:"settings"`
}

func battery(raw byte) *int {
	if raw > 100 {
		return nil
	}
	value := int(raw)
	return &value
}

// Settings lists every setting in the CLI's order. No records leaves them all unreported.
func Settings(records map[byte]dongle.Record) []Setting {
	settings := make([]Setting, 0, len(dongle.Settings)+len(dongle.ExtraSettings))
	for _, table := range [][]dongle.Setting{dongle.Settings, dongle.ExtraSettings} {
		for i := range table {
			entry := Setting{Name: table[i].Name, Unknown: true}
			if record, ok := records[table[i].Control]; ok {
				raw, age := record.Value, record.AgeMs
				entry.Raw, entry.AgeMs = &raw, &age
				if decoded := table[i].Decode(raw); !strings.HasPrefix(decoded, "unknown device value") {
					entry.Decoded, entry.Unknown = &decoded, false
				}
			}
			settings = append(settings, entry)
		}
	}
	return settings
}

// Build makes the document from a status report and the settings read with it.
func Build(status dongle.Status, records map[byte]dongle.Record) Document {
	state := "disconnected"
	if status.Connected {
		state = "connected"
	}
	return Document{
		Schema:        Schema,
		State:         state,
		SettingsReady: status.ControlsReady,
		Codec:         status.CodecName(),
		Batteries:     Batteries{battery(status.BatteryLeft), battery(status.BatteryRight), battery(status.BatteryCase)},
		Settings:      Settings(records),
	}
}

// JSON prints the document as the CLI prints its own.
func (d Document) JSON() string { return Encode(d) }

// Encode prints a value as the CLI prints its documents.
func Encode(value any) string {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return "" // Plain fields cannot fail to encode.
	}
	return out.String()
}
