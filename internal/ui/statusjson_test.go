package ui

import (
	"encoding/json"
	"testing"
)

func TestCopyStatusAsJSON(t *testing.T) {
	r := newRig(t, ownerConfig())
	r.shown()
	r.main(func() { r.w.diag.copyDoc.OnTapped() })
	var copied string
	r.until(func() bool {
		copied = r.w.opts.Fyne.Clipboard().Content()
		return copied != "" && !r.w.diag.copying
	}, "copied")
	var doc map[string]any
	if err := json.Unmarshal([]byte(copied), &doc); err != nil {
		t.Fatalf("%q: %v", copied, err)
	}
	for _, key := range []string{"schema", "state", "settings_ready", "codec", "batteries", "settings"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("no %s", key)
		}
	}
	if doc["batteries"].(map[string]any)["case"] != nil {
		t.Errorf("the case has no reading: %v", doc["batteries"])
	}
	reported := 0
	for _, entry := range doc["settings"].([]any) {
		setting := entry.(map[string]any)
		if setting["raw"] != nil && setting["age_ms"] == 900.0 {
			reported++
		}
	}
	if reported == 0 {
		t.Errorf("no setting with its age: %v", doc["settings"])
	}
}
