package ui

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

func keysOf(doc map[string]any) []string {
	keys := make([]string, 0, len(doc))
	for key := range doc {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// copied taps a button and returns the document it put on the clipboard.
func (r *rig) copied(button func() fyne.Tappable) map[string]any {
	r.t.Helper()
	r.main(func() {
		r.w.opts.Fyne.Clipboard().SetContent("")
		button().Tapped(&fyne.PointEvent{})
	})
	var text string
	r.until(func() bool {
		text = r.w.opts.Fyne.Clipboard().Content()
		return text != "" && !r.w.diag.copying
	}, "copied")
	var doc map[string]any
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		r.t.Fatalf("%q: %v", text, err)
	}
	return doc
}

func TestCopyEachDocument(t *testing.T) {
	r := experimentalRig(t, sim.Demo())
	r.main(func() { r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348", Serial: "SIMSERIAL123"}) })
	d := r.w.diag
	for _, tc := range []struct {
		name   string
		button func() fyne.Tappable
		keys   []string
	}{
		{"list", func() fyne.Tappable { return d.copyDongles }, []string{"dongles", "schema"}},
		{"info", func() fyne.Tappable { return d.copyInfo }, []string{"capabilities", "firmware", "model_name", "model_number", "schema", "serial", "serial_hidden"}},
		{"hearing", func() fyne.Tappable { return d.copyHearing }, []string{"assist_enabled", "hearing_aid", "hearing_enrolled", "media_assist", "media_enabled", "report_age_ms", "schema", "voice_enabled", "voice_enrolled"}},
		{"hardware", func() fyne.Tappable { return d.copyHardware }, []string{"available", "read_at", "schema", "sensitivity", "test_mode", "volume_curve"}},
		{"hardware status", func() fyne.Tappable { return d.copyResults }, []string{"left", "read_at", "right", "saved", "schema"}},
		{"transparency", func() fyne.Tappable { return d.copyTransparency }, []string{"enabled", "left_ambient_noise_reduction", "left_amplification", "left_conversation_boost", "left_tone", "right_ambient_noise_reduction", "right_amplification", "right_conversation_boost", "right_tone", "schema"}},
		{"loud sound reduction", func() fyne.Tappable { return d.copyLoud }, []string{"schema", "state"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := r.copied(tc.button)
			if got := keysOf(doc); !slices.Equal(got, tc.keys) {
				t.Errorf("fields %v", got)
			}
			if doc["schema"] != 1.0 {
				t.Errorf("schema %v", doc["schema"])
			}
		})
	}
	doc := r.copied(func() fyne.Tappable { return d.copyInfo })
	if doc["serial"] != nil || doc["serial_hidden"] != true || doc["model_number"] != "A3063" {
		t.Errorf("the serial must stay hidden: %v", doc)
	}
}

func TestCopiedListNamesOpenAndOtherDongles(t *testing.T) {
	other := sim.New(sim.Demo())
	list := []hid.Info{
		{Path: "open", Serial: "SIM0001", PID: hid.ProductAudio, Openable: true},
		{Path: "other", Serial: "SIM0002", PID: hid.ProductControl, Openable: true},
		{Path: "denied", Serial: "SIM0003", PID: hid.ProductControl, Err: errors.New("no permission")},
	}
	opened := []string{}
	picker := device.NewPicker(func() ([]hid.Info, error) { return list, nil }, func(serial string) (hid.Device, error) {
		opened = append(opened, serial)
		return other.Open("")
	}, "", false)
	cfg := sim.Demo()
	cfg.Serial = "SIM0001"
	r := newRigWith(t, cfg, func(o *Options) { o.Picker = picker })
	doc := r.copied(func() fyne.Tappable { return r.w.diag.copyDongles })
	rows := doc["dongles"].([]any)
	if len(rows) != 3 {
		t.Fatalf("%v", rows)
	}
	first, second, third := rows[0].(map[string]any), rows[1].(map[string]any), rows[2].(map[string]any)
	if first["connected"] != true || first["usb_id"] != "4015" || second["connected"] == nil || second["usb_id"] != "4005" {
		t.Errorf("%v %v", first, second)
	}
	if third["connected"] != nil || third["error"] != "cannot be opened: no permission" {
		t.Errorf("%v", third)
	}
	if !slices.Equal(opened, []string{"SIM0002"}) {
		t.Errorf("opened %v: the open dongle belongs to the owner", opened)
	}
}

func TestCopyHearingAndHardwareFollowTheModelAndTheBusyState(t *testing.T) {
	r := experimentalRig(t, sim.Demo())
	r.main(func() {
		r.w.setHearingBusy(true)
		if !r.w.diag.copyHearing.Disabled() || !r.w.diag.copyHardware.Disabled() {
			t.Error("must wait for a hearing change")
		}
		r.w.setHearingBusy(false)
		r.w.SetInfo(dongle.Metadata{Model: "A2096", Firmware: "7A1"})
		r.w.opts.Fyne.Clipboard().SetContent("")
		test.Tap(r.w.diag.copyHearing)
		test.Tap(r.w.diag.copyHardware)
		if r.w.diag.copying || r.w.opts.Fyne.Clipboard().Content() != "" {
			t.Error("a model without them has nothing to copy")
		}
		if r.w.banner.Text != i18n.T("section.diagnostics")+": "+i18n.T("hardware.notAvailable") {
			t.Errorf("%q", r.w.banner.Text)
		}
	})
}

func TestCopyExperimentalNeedsTheSwitchAndTheModel(t *testing.T) {
	r := experimentalRig(t, sim.Demo())
	d := r.w.diag
	r.main(func() {
		r.w.opts.Fyne.Clipboard().SetContent("")
		r.w.opts.Fyne.Preferences().SetBool(experimentalPreference, false)
		r.w.applyExperimental()
		if !d.copyTransparency.Disabled() || !d.copyLoud.Disabled() {
			t.Error("the experimental switch is off")
		}
		r.w.opts.Fyne.Preferences().SetBool(experimentalPreference, true)
		r.w.applyExperimental()
		if d.copyTransparency.Disabled() || d.copyLoud.Disabled() {
			t.Error("the experimental switch is on")
		}
		r.w.SetInfo(dongle.Metadata{Model: "A3053", Firmware: "9A348"}) // AirPods 4 lists neither
		test.Tap(d.copyTransparency)
		test.Tap(d.copyLoud)
		if d.copying || r.w.opts.Fyne.Clipboard().Content() != "" {
			t.Error("a model without them has nothing to copy")
		}
		if r.w.banner.Text != i18n.T("section.diagnostics")+": "+i18n.T("hardware.notAvailable") {
			t.Errorf("%q", r.w.banner.Text)
		}
	})
}

// TestStartsWithTheExperimentalSwitchSaved applies the saved choice while the window is being built.
func TestStartsWithTheExperimentalSwitchSaved(t *testing.T) {
	r := newRigWith(t, sim.Demo(), func(o *Options) { o.Fyne.Preferences().SetBool(experimentalPreference, true) })
	r.until(func() bool { return !r.w.diag.copyTransparency.Disabled() }, "the copy offered")
}
