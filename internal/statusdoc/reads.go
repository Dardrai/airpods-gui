package statusdoc

import (
	"fmt"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/hearing"
	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/core/model"
)

// The documents here are those of the CLI's --json, field for field: list, info --capabilities,
// hearing status, hardware status and data, and the two experimental reads. Its docs/CLI.md describes them.

// Dongle is one row of the list document.
type Dongle struct {
	Serial    *string `json:"serial"`
	USBID     string  `json:"usb_id"`
	Path      string  `json:"path"`
	Connected *bool   `json:"connected"`
	Error     *string `json:"error"`
}

// DongleList is `airpods --json list`.
type DongleList struct {
	Schema  int      `json:"schema"`
	Dongles []Dongle `json:"dongles"`
}

func nullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// ListDongles makes the list document. A dongle that cannot be used is a row with its reason, and
// probe asks one that can be used whether its AirPods are connected.
func ListDongles(infos []hid.Info, probe func(hid.Info) (bool, error)) DongleList {
	sameSerial := map[string]int{}
	openable := 0
	for _, info := range infos {
		sameSerial[info.Serial]++
		if info.Openable {
			openable++
		}
	}
	list := DongleList{Schema: Schema, Dongles: make([]Dongle, 0, len(infos))}
	for _, info := range infos {
		row := Dongle{Serial: nullable(info.Serial), USBID: fmt.Sprintf("%04x", info.PID), Path: info.Path}
		reason := ""
		switch {
		case !info.Openable:
			reason = "cannot be opened"
			if info.Err != nil {
				reason = "cannot be opened: " + info.Err.Error()
			}
		case info.Serial == "" && openable > 1:
			reason = "cannot be selected: it has no USB serial"
		case info.Serial != "" && sameSerial[info.Serial] > 1:
			reason = "cannot be selected: its USB serial is not unique"
		default:
			connected, err := probe(info)
			if err != nil {
				reason = "cannot be opened: " + err.Error()
			} else {
				row.Connected = &connected
			}
		}
		row.Error = nullable(reason)
		list.Dongles = append(list.Dongles, row)
	}
	return list
}

// Info is `airpods --json info --capabilities`. The serial stays hidden, as it does there without
// --show-serial, so that what is copied is safe to paste.
type Info struct {
	Schema       int           `json:"schema"`
	ModelName    *string       `json:"model_name"`
	ModelNumber  *string       `json:"model_number"`
	Firmware     *string       `json:"firmware"`
	Serial       *string       `json:"serial"`
	SerialHidden bool          `json:"serial_hidden"`
	Capabilities *Capabilities `json:"capabilities,omitempty"`
}

// BuildInfo makes the info document from what the AirPods reported.
func BuildInfo(meta dongle.Metadata) Info {
	name := ""
	if profile, ok := model.Find(meta.Model); ok {
		name = profile.Name
	}
	return Info{
		Schema: Schema, ModelName: nullable(name), ModelNumber: nullable(meta.Model), Firmware: nullable(meta.Firmware),
		SerialHidden: meta.Serial != "", Capabilities: CapabilitiesOf(meta.Model),
	}
}

// HearingStatus is `airpods --json hearing status`.
type HearingStatus struct {
	Schema          int            `json:"schema"`
	HearingAid      string         `json:"hearing_aid"`
	MediaAssist     string         `json:"media_assist"`
	HearingEnrolled string         `json:"hearing_enrolled"`
	VoiceEnrolled   string         `json:"voice_enrolled"`
	AssistEnabled   string         `json:"assist_enabled"`
	VoiceEnabled    string         `json:"voice_enabled"`
	MediaEnabled    string         `json:"media_enabled"`
	ReportAgeMs     HearingReports `json:"report_age_ms"`
}

// BuildHearingStatus makes the hearing document from a state and the ages read with it.
func BuildHearingStatus(state hearing.ModeState, ages HearingReports) HearingStatus {
	return HearingStatus{
		Schema: Schema, HearingAid: state.HearingEnabled.String(), MediaAssist: state.Media.String(),
		HearingEnrolled: state.HearingEnrolled.String(), VoiceEnrolled: state.VoiceEnrolled.String(),
		AssistEnabled: state.AssistEnabled.String(), VoiceEnabled: state.VoiceEnabled.String(),
		MediaEnabled: state.PMEMediaEnabled.String(), ReportAgeMs: ages,
	}
}

// DeviceData is `airpods --json hardware data`.
type DeviceData struct {
	Schema      int       `json:"schema"`
	Available   bool      `json:"available"`
	ReadAt      string    `json:"read_at"`
	TestMode    *byte     `json:"test_mode"`
	Sensitivity *float32  `json:"sensitivity"`
	VolumeCurve []float32 `json:"volume_curve"`
}

// BuildDeviceData makes the hardware document.
func BuildDeviceData(data hearing.DeviceData) DeviceData {
	return DeviceData{
		Schema: Schema, Available: data.AnyCharacteristic, ReadAt: data.ReadAt.Format(time.RFC3339),
		TestMode: data.TestMode, Sensitivity: data.Sensitivity, VolumeCurve: data.VolumeCurve,
	}
}

// Ear is one ear in the hardware status document.
type Ear struct {
	Result            string   `json:"result"`
	MeasuredAtSeconds *uint64  `json:"measured_at_seconds"`
	MeasuredAt        *string  `json:"measured_at"`
	Failures          []string `json:"failures"`
}

// HardwareStatus is `airpods --json hardware status`. Without a saved record, saved is false and
// the rest is null.
type HardwareStatus struct {
	Schema int     `json:"schema"`
	Saved  bool    `json:"saved"`
	ReadAt *string `json:"read_at"`
	Left   *Ear    `json:"left"`
	Right  *Ear    `json:"right"`
}

func earOf(ear hearing.EarResult) *Ear {
	entry := &Ear{Result: ear.Text(), MeasuredAt: nullable(hearing.When(ear.MeasuredAtSeconds)), Failures: []string{}}
	if ear.MeasuredAtSeconds != 0 {
		seconds := ear.MeasuredAtSeconds
		entry.MeasuredAtSeconds = &seconds
	}
	entry.Failures = append(entry.Failures, ear.Failures...)
	return entry
}

// BuildHardwareStatus makes the hardware status document; a nil record means none is saved.
func BuildHardwareStatus(record *hearing.Diagnostics) HardwareStatus {
	if record == nil {
		return HardwareStatus{Schema: Schema}
	}
	readAt := record.ReadAt.Format(time.RFC3339)
	return HardwareStatus{Schema: Schema, Saved: true, ReadAt: &readAt, Left: earOf(record.Left), Right: earOf(record.Right)}
}

// Transparency is `airpods --json experimental --experimental transparency show`.
type Transparency struct {
	Schema                     int     `json:"schema"`
	Enabled                    bool    `json:"enabled"`
	LeftAmplification          float32 `json:"left_amplification"`
	RightAmplification         float32 `json:"right_amplification"`
	LeftTone                   float32 `json:"left_tone"`
	RightTone                  float32 `json:"right_tone"`
	LeftConversationBoost      bool    `json:"left_conversation_boost"`
	RightConversationBoost     bool    `json:"right_conversation_boost"`
	LeftAmbientNoiseReduction  float32 `json:"left_ambient_noise_reduction"`
	RightAmbientNoiseReduction float32 `json:"right_ambient_noise_reduction"`
}

// BuildTransparency makes the Custom Transparency document.
func BuildTransparency(s hearing.TransparencySettings) Transparency {
	return Transparency{
		Schema, s.Enabled, s.LeftAmplification, s.RightAmplification, s.LeftTone, s.RightTone,
		s.LeftConversationBoost, s.RightConversationBoost, s.LeftAmbientNoiseReduction, s.RightAmbientNoiseReduction,
	}
}

// LoudSoundReduction is `airpods --json experimental --experimental loud-sound-reduction get`.
type LoudSoundReduction struct {
	Schema int    `json:"schema"`
	State  string `json:"state"`
}

// BuildLoudSoundReduction makes the Loud Sound Reduction document.
func BuildLoudSoundReduction(on bool) LoudSoundReduction {
	state := "off"
	if on {
		state = "on"
	}
	return LoudSoundReduction{Schema, state}
}
