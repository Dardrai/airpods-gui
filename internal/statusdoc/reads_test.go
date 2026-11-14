package statusdoc

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/hearing"
	"github.com/Dardrai/airpods-gui/core/hid"
)

// The expected bytes below are the samples and the field lists of the CLI's docs/CLI.md.

func TestListDocumentMatchesTheCLI(t *testing.T) {
	list := ListDongles([]hid.Info{{Serial: "SIM0002", PID: 0x4005, Path: "/dev/hidraw12", Openable: true}}, func(hid.Info) (bool, error) { return false, nil })
	want := `{
  "schema": 1,
  "dongles": [
    {
      "serial": "SIM0002",
      "usb_id": "4005",
      "path": "/dev/hidraw12",
      "connected": false,
      "error": null
    }
  ]
}
`
	if got := Encode(list); got != want {
		t.Errorf("%s", got)
	}
	if got := Encode(ListDongles(nil, nil)); got != "{\n  \"schema\": 1,\n  \"dongles\": []\n}\n" {
		t.Errorf("%s", got)
	}
}

func TestListReasonsMatchTheCLI(t *testing.T) {
	probe := func(info hid.Info) (bool, error) {
		if info.Path == "broken" {
			return false, errors.New("busy")
		}
		return true, nil
	}
	list := ListDongles([]hid.Info{
		{Path: "a", Serial: "A", PID: 0x4015, Openable: true},
		{Path: "broken", Serial: "B", PID: 0x4015, Openable: true},
		{Path: "denied", Serial: "C", PID: 0x4015, Err: errors.New("no permission")},
		{Path: "closed", Serial: "D", PID: 0x4015},
		{Path: "noname", PID: 0x4015, Openable: true},
		{Path: "same1", Serial: "E", PID: 0x4015, Openable: true},
		{Path: "same2", Serial: "E", PID: 0x4015, Openable: true},
	}, probe)
	wants := []string{
		"", "cannot be opened: busy", "cannot be opened: no permission", "cannot be opened",
		"cannot be selected: it has no USB serial", "cannot be selected: its USB serial is not unique", "cannot be selected: its USB serial is not unique",
	}
	for i, want := range wants {
		row := list.Dongles[i]
		got := ""
		if row.Error != nil {
			got = *row.Error
		}
		if got != want || (want == "") != (row.Connected != nil) {
			t.Errorf("%d: %q, connected %v", i, got, row.Connected)
		}
	}
	if list.Dongles[4].Serial != nil {
		t.Error("no serial must be null")
	}
}

func TestInfoDocumentMatchesTheCLI(t *testing.T) {
	want := `{
  "schema": 1,
  "model_name": "AirPods Max (Lightning)",
  "model_number": "A2096",
  "firmware": "7A1",
  "serial": null,
  "serial_hidden": true,
  "capabilities": {
    "listed": true,
    "family": "airpods_max",
    "over_ear": true,
    "listening": true,
    "adaptive": false,
    "allow_off_option": false,
    "high_quality_mic": false,
    "custom_eq": false,
    "hearing_aid": false,
    "custom_transparency": false,
    "loud_sound_reduction": false,
    "controls": [
      {
        "id": "0x0a",
        "name": "ear-detection"
      },
`
	got := Encode(BuildInfo(dongle.Metadata{Model: "A2096", Firmware: "7A1", Serial: "SIMSERIAL123"}))
	if len(got) < len(want) || got[:len(want)] != want {
		t.Errorf("%s", got)
	}
	empty := "{\n  \"schema\": 1,\n  \"model_name\": null,\n  \"model_number\": null,\n  \"firmware\": null,\n  \"serial\": null,\n  \"serial_hidden\": false,\n  \"capabilities\": {\n    \"listed\": false,\n    \"family\": null,\n"
	if got := Encode(BuildInfo(dongle.Metadata{})); len(got) < len(empty) || got[:len(empty)] != empty {
		t.Errorf("%s", got)
	}
}

func TestHearingDocumentMatchesTheCLI(t *testing.T) {
	one, two := uint32(1000), uint32(2000)
	state := hearing.ModeState{HearingEnabled: hearing.FlagOff, Media: hearing.FlagOn, HearingEnrolled: hearing.FlagOn, VoiceEnrolled: hearing.FlagOff, AssistEnabled: hearing.FlagOn, VoiceEnabled: hearing.FlagOff, PMEMediaEnabled: hearing.FlagUnknown}
	want := `{
  "schema": 1,
  "hearing_aid": "off",
  "media_assist": "on",
  "hearing_enrolled": "on",
  "voice_enrolled": "off",
  "assist_enabled": "on",
  "voice_enabled": "off",
  "media_enabled": "unknown",
  "report_age_ms": {
    "hearing_aid": 1000,
    "assist": 2000,
    "media": null
  }
}
`
	if got := Encode(BuildHearingStatus(state, HearingReports{HearingAid: &one, Assist: &two})); got != want {
		t.Errorf("%s", got)
	}
}

func TestDeviceDataDocumentMatchesTheCLI(t *testing.T) {
	mode := byte(2)
	sensitivity := float32(0.5)
	at := time.Date(2025, 6, 15, 22, 13, 20, 0, time.UTC)
	want := `{
  "schema": 1,
  "available": true,
  "read_at": "2025-06-15T22:13:20Z",
  "test_mode": 2,
  "sensitivity": 0.5,
  "volume_curve": [
    0.25
  ]
}
`
	if got := Encode(BuildDeviceData(hearing.DeviceData{ReadAt: at, TestMode: &mode, Sensitivity: &sensitivity, VolumeCurve: []float32{0.25}, AnyCharacteristic: true})); got != want {
		t.Errorf("%s", got)
	}
	nothing := "{\n  \"schema\": 1,\n  \"available\": false,\n  \"read_at\": \"2025-06-15T22:13:20Z\",\n  \"test_mode\": null,\n  \"sensitivity\": null,\n  \"volume_curve\": null\n}\n"
	if got := Encode(BuildDeviceData(hearing.DeviceData{ReadAt: at})); got != nothing {
		t.Errorf("%s", got)
	}
}

func TestHardwareStatusDocumentMatchesTheCLI(t *testing.T) {
	if got := Encode(BuildHardwareStatus(nil)); got != "{\n  \"schema\": 1,\n  \"saved\": false,\n  \"read_at\": null,\n  \"left\": null,\n  \"right\": null\n}\n" {
		t.Errorf("%s", got)
	}
	at := time.Date(2025, 6, 15, 22, 13, 20, 0, time.UTC)
	record := &hearing.Diagnostics{ReadAt: at, Left: hearing.EarResult{}, Right: hearing.EarResult{Failures: []string{"speaker"}}}
	got := BuildHardwareStatus(record)
	if !got.Saved || *got.ReadAt != "2025-06-15T22:13:20Z" || got.Left.MeasuredAtSeconds != nil || got.Left.MeasuredAt != nil {
		t.Errorf("%+v %+v", got, got.Left)
	}
	want := "\"failures\": [\n      \"speaker\"\n    ]"
	if text := Encode(got); !strings.Contains(text, want) || !strings.Contains(text, "\"failures\": []") {
		t.Errorf("%s", text)
	}
	if text := Encode(got.Left); !strings.HasPrefix(text, "{\n  \"result\": ") || !strings.Contains(text, "\n  \"measured_at_seconds\": null,\n  \"measured_at\": null,\n  \"failures\": []\n}") {
		t.Errorf("%s", text)
	}
}

func TestExperimentalDocumentsMatchTheCLI(t *testing.T) {
	want := `{
  "schema": 1,
  "enabled": false,
  "left_amplification": 0.5,
  "right_amplification": 0.5,
  "left_tone": 0,
  "right_tone": 0,
  "left_conversation_boost": false,
  "right_conversation_boost": true,
  "left_ambient_noise_reduction": 0.25,
  "right_ambient_noise_reduction": 0.25
}
`
	s := hearing.TransparencySettings{LeftAmplification: 0.5, RightAmplification: 0.5, RightConversationBoost: true, LeftAmbientNoiseReduction: 0.25, RightAmbientNoiseReduction: 0.25}
	if got := Encode(BuildTransparency(s)); got != want {
		t.Errorf("%s", got)
	}
	if got := Encode(BuildLoudSoundReduction(true)); got != "{\n  \"schema\": 1,\n  \"state\": \"on\"\n}\n" {
		t.Errorf("%s", got)
	}
	if got := Encode(BuildLoudSoundReduction(false)); got != "{\n  \"schema\": 1,\n  \"state\": \"off\"\n}\n" {
		t.Errorf("%s", got)
	}
}
