package ui

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/hearing"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

type profileBuffer struct {
	bytes.Buffer
	closed bool
}

func (f *profileBuffer) Close() error {
	f.closed = true
	return nil
}

func readRig(t *testing.T, cfg sim.Config) *rig {
	t.Helper()
	r := experimentalRig(t, cfg)
	r.main(r.w.hearing.readAll)
	r.until(func() bool { return r.w.hearing.have && !r.w.hearingBusy }, "read")
	return r
}

func TestExportThenImportOfTheSameFileChangesNothing(t *testing.T) {
	r := readRig(t, sim.Demo())
	h := r.w.hearing
	file := &profileBuffer{}
	sent := len(r.dev.AAPSent())
	r.main(func() {
		r.w.pickFile = func(name string, chosen func(io.WriteCloser, error)) {
			if !strings.HasSuffix(name, ".json") {
				t.Errorf("file name %q", name)
			}
			chosen(file, nil)
		}
		h.exportProfile()
		if !file.closed || !strings.Contains(file.String(), `"left_db_hl"`) || h.status.Text != i18n.T("hearing.exported") {
			t.Errorf("%v %q %q", file.closed, file.String(), h.status.Text)
		}
		r.w.openFile = func(chosen func(io.ReadCloser, error)) { chosen(io.NopCloser(bytes.NewReader(file.Bytes())), nil) }
		before := h.edit.Draft
		h.importProfile()
		if h.edit.Dirty || h.edit.Draft != before || !h.apply.Disabled() || h.status.Text != i18n.T("hearing.importSame") {
			t.Errorf("dirty %v, apply off %v, %q", h.edit.Dirty, h.apply.Disabled(), h.status.Text)
		}
	})
	if got := len(r.dev.AAPSent()); got != sent {
		t.Errorf("import or export sent %d", got-sent)
	}
}

func TestImportLoadsTheEditorAndNeverApplies(t *testing.T) {
	r := readRig(t, sim.Demo())
	h := r.w.hearing
	sent := len(r.dev.AAPSent())
	r.main(func() {
		baseline := h.edit.Baseline()
		h.bands[0][0].SetText("44") // goes: what the file does not name returns to what is stored
		h.loadFile([]byte(`{"right_db_hl": [1, 2, 3, 4, 5, 6, 7, 8], "amplification": 0.75, "tone": 0.25, "conversation_boost": true}`))
		if h.status.Text != i18n.T("hearing.imported") || !h.edit.Dirty || h.apply.Disabled() {
			t.Errorf("%q, dirty %v, apply off %v", h.status.Text, h.edit.Dirty, h.apply.Disabled())
		}
		if h.edit.Baseline() != baseline {
			t.Error("the baseline moved")
		}
		if h.bands[1][7].Text != "8" || h.bands[0][0].Text != bandText(baseline.LeftEQ[0]) {
			t.Errorf("cells %q %q", h.bands[1][7].Text, h.bands[0][0].Text)
		}
		if h.amp.Slider.Value != 0.75 || h.tone.Slider.Value != 0.25 || !h.boost.Checked {
			t.Errorf("controls %v %v %v", h.amp.Slider.Value, h.tone.Slider.Value, h.boost.Checked)
		}
	})
	if got := len(r.dev.AAPSent()); got != sent {
		t.Errorf("import sent %d", got-sent)
	}
}

func TestABadFileLeavesTheEditorAlone(t *testing.T) {
	r := readRig(t, sim.Demo())
	h := r.w.hearing
	r.main(func() {
		h.bands[0][0].SetText("44")
		draft, cell := h.edit.Draft, h.bands[0][0].Text
		for _, text := range []string{`{"tones": 1}`, `{"tone": 3}`, `{`, `{}`, `{"left_db_hl": [1]}`} {
			h.say("")
			h.loadFile([]byte(text))
			if h.edit.Draft != draft || h.bands[0][0].Text != cell {
				t.Errorf("%s moved the editor", text)
			}
			if !strings.HasPrefix(h.status.Text, "The file was not loaded:") {
				t.Errorf("%s: %q", text, h.status.Text)
			}
		}
	})
}

// TestImportRefusesWhatTheStoredProfileCannotTake has a stored value out of range, so that
// Settings.Valid fails on the draft while the file itself is fine.
func TestImportRefusesWhatTheStoredProfileCannotTake(t *testing.T) {
	r := readRig(t, sim.Demo())
	h := r.w.hearing
	r.main(func() {
		stored := h.edit.Baseline()
		stored.OwnVoice = 7
		h.loadProfile(hearing.Profile{Settings: stored, Editable: true})
		draft := h.edit.Draft
		h.loadFile([]byte(`{"tone": 0.5}`))
		if h.edit.Draft != draft || !strings.Contains(h.status.Text, i18n.T("hearing.importInvalid")) {
			t.Errorf("%q", h.status.Text)
		}
	})
}

func TestImportNeedsAnEditableProfile(t *testing.T) {
	r := readRig(t, sim.Demo())
	h := r.w.hearing
	r.main(func() {
		stored := h.edit.Baseline()
		h.loadProfile(hearing.Profile{Settings: stored, Editable: false})
		h.updateEnabled()
		if !h.importBt.Disabled() || h.exportBt.Disabled() {
			t.Errorf("import off %v, export off %v", h.importBt.Disabled(), h.exportBt.Disabled())
		}
		h.importProfile() // the opener is the real dialog, which a test could not answer
		h.loadFile([]byte(`{"tone": 0.5}`))
		if h.edit.Draft != stored {
			t.Error("an uneditable profile took a file")
		}
	})
}

func TestFileErrorsAreSaid(t *testing.T) {
	r := readRig(t, sim.Demo())
	h := r.w.hearing
	r.main(func() {
		r.w.pickFile = func(_ string, chosen func(io.WriteCloser, error)) { chosen(nil, errors.New("disk full")) }
		h.exportProfile()
		if !strings.Contains(h.status.Text, "disk full") {
			t.Errorf("%q", h.status.Text)
		}
		r.w.openFile = func(chosen func(io.ReadCloser, error)) { chosen(nil, errors.New("gone")) }
		h.importProfile()
		if !strings.Contains(h.status.Text, "gone") {
			t.Errorf("%q", h.status.Text)
		}
		h.say("")
		r.w.openFile = func(chosen func(io.ReadCloser, error)) { chosen(nil, nil) } // cancelled
		h.importProfile()
		if h.statusBox.Visible() {
			t.Errorf("a cancel said %q", h.status.Text)
		}
	})
}

// TestStoredNoiseReductionAndOwnVoiceAreShownAndStay use the CLI's numbers and labels.
func TestStoredNoiseReductionAndOwnVoiceAreShownAndStay(t *testing.T) {
	r := readRig(t, sim.Demo())
	h := r.w.hearing
	r.main(func() {
		s := h.edit.Baseline()
		s.LeftAmbientNoiseReduction, s.RightAmbientNoiseReduction, s.OwnVoice = 0.5, 0.75, 0.625
		h.loadProfile(hearing.Profile{Settings: s, Editable: true})
		if h.storedANR.Text != "left 0.5, right 0.75" || h.ownVoice.Text != "0.625" {
			t.Errorf("%q %q", h.storedANR.Text, h.ownVoice.Text)
		}
		if i18n.T("extra.noiseReduction") != "Ambient noise reduction" || i18n.T("hearing.ownVoice") != "Own voice" {
			t.Error("labels differ from the CLI's")
		}
		h.loadFile([]byte(`{"tone": 0.5}`))
		if h.edit.Draft.OwnVoice != 0.625 || h.ownVoice.Text != "0.625" {
			t.Errorf("a file moved it: %v", h.edit.Draft.OwnVoice)
		}
		i18n.SetLanguage(i18n.German)
		defer i18n.SetLanguage(i18n.English)
		if h.storedANR.Text != "links 0.5, rechts 0.75" {
			t.Errorf("%q", h.storedANR.Text)
		}
	})
}

func TestAProfileThatIsNotEditableIsReadOnly(t *testing.T) {
	r := readRig(t, sim.Demo())
	h := r.w.hearing
	r.main(func() {
		h.loadProfile(hearing.Profile{Settings: h.edit.Baseline(), Editable: false})
		h.updateEnabled()
		if !h.bands[0][0].Disabled() || !h.amp.Slider.Disabled() || !h.boost.Disabled() || !h.apply.Disabled() {
			t.Error("the editor can still be used")
		}
		if h.editNote.Text != i18n.T("hearing.notEditable") || h.ownVoice.Text == dashText {
			t.Errorf("%q %q", h.editNote.Text, h.ownVoice.Text)
		}
	})
}

func TestCustomTransparencyShowsStoredNoiseReduction(t *testing.T) {
	r := experimentalRig(t, sim.Demo())
	x := r.w.extra
	r.main(x.readTransparency)
	r.until(func() bool { return x.transBase != nil && !r.w.hearingBusy }, "read")
	r.main(func() {
		base := *x.transBase
		want := "left " + bandText(base.LeftAmbientNoiseReduction) + ", right " + bandText(base.RightAmbientNoiseReduction)
		if x.transStored.Text != want {
			t.Errorf("%q, want %q", x.transStored.Text, want)
		}
		x.transNoise.Slider.SetValue(float64(base.LeftAmbientNoiseReduction) / 2)
		if x.transStored.Text != want {
			t.Errorf("an edit that was not applied moved what is stored: %q", x.transStored.Text)
		}
	})
}
