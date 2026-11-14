package statusdoc

import (
	"encoding/json"
	"testing"

	"github.com/Dardrai/airpods-gui/core/dongle"
)

func TestFieldsMatchTheCLI(t *testing.T) {
	status := dongle.Status{Connected: true, ControlsReady: true, Codec: 2, BatteryLeft: 80, BatteryRight: 70, BatteryCase: 255}
	mode, _ := dongle.LookupSetting("mode")
	records := map[byte]dongle.Record{mode.Control: {Value: 2, AgeMs: 1200}, 0xee: {Value: 1, AgeMs: 1}}
	var got map[string]any
	if err := json.Unmarshal([]byte(Build(status, records).JSON()), &got); err != nil {
		t.Fatal(err)
	}
	if got["schema"] != 1.0 || got["state"] != "connected" || got["settings_ready"] != true || got["codec"] != status.CodecName() {
		t.Errorf("%v", got)
	}
	batteries := got["batteries"].(map[string]any)
	if value, present := batteries["case"]; !present || value != nil || batteries["left"] != 80.0 || batteries["right"] != 70.0 {
		t.Errorf("batteries %v", batteries)
	}
	settings := got["settings"].([]any)
	if len(settings) != len(dongle.Settings)+len(dongle.ExtraSettings) {
		t.Errorf("%d settings", len(settings))
	}
	seen := 0
	for _, entry := range settings {
		setting := entry.(map[string]any)
		for _, key := range []string{"name", "raw", "decoded", "age_ms", "unknown"} {
			if _, ok := setting[key]; !ok {
				t.Errorf("no %s in %v", key, setting)
			}
		}
		if setting["name"] == "mode" {
			seen++
			if setting["raw"] != 2.0 || setting["age_ms"] != 1200.0 || setting["decoded"] != "anc" || setting["unknown"] != false {
				t.Errorf("mode %v", setting)
			}
		} else if setting["raw"] != nil || setting["decoded"] != nil || setting["age_ms"] != nil || setting["unknown"] != true {
			t.Errorf("unreported %v", setting)
		}
	}
	if seen != 1 {
		t.Errorf("mode seen %d times", seen)
	}
}

func TestAValueWithNoNameIsUnknownButKeepsItsRaw(t *testing.T) {
	mode, _ := dongle.LookupSetting("mode")
	settings := Settings(map[byte]dongle.Record{mode.Control: {Value: 0, AgeMs: 1}})
	for _, setting := range settings {
		if setting.Name == "mode" && (setting.Raw == nil || *setting.Raw != 0 || setting.Decoded != nil || !setting.Unknown) {
			t.Errorf("%+v", setting)
		}
	}
}
