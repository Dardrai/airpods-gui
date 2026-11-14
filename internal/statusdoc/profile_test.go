package statusdoc

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Dardrai/airpods-gui/core/hearing"
)

func sampleProfile() hearing.Profile {
	return hearing.Profile{Editable: true, Settings: hearing.Settings{
		LeftEQ: [hearing.Bands]float32{10, 11, 12, 13, 14, 15, 16, 17}, RightEQ: [hearing.Bands]float32{20, 21, 22, 23, 24, 25, 26, 27},
		LeftAmplification: 0.25, RightAmplification: -0.25, LeftTone: 0.125, RightTone: -0.125,
		RightConversationBoost: true, LeftAmbientNoiseReduction: 0.5, RightAmbientNoiseReduction: 0.75, OwnVoice: 0.625,
	}}
}

// TestExportHasTheNamesOfTheCLI compares the exact bytes with the profile object of the CLI's show.
func TestExportHasTheNamesOfTheCLI(t *testing.T) {
	want := `{
  "left_db_hl": [
    10,
    11,
    12,
    13,
    14,
    15,
    16,
    17
  ],
  "right_db_hl": [
    20,
    21,
    22,
    23,
    24,
    25,
    26,
    27
  ],
  "amplification": 0,
  "balance": -0.5,
  "tone": 0,
  "conversation_boost": false
}
`
	if got := Encode(ProfileOf(hearing.NewProfileEdit(sampleProfile()))); got != want {
		t.Errorf("%s", got)
	}
}

// TestAFileMadeFromAProfileChangesNothing keeps each ear's gain and tone, which a moved slider
// would overwrite.
func TestAFileMadeFromAProfileChangesNothing(t *testing.T) {
	profile := sampleProfile()
	profile.Settings.LeftAmplification, profile.Settings.RightAmplification = 0.1, 0.3 // no short binary form
	source := hearing.NewProfileEdit(profile)
	input, err := ParseProfile([]byte(Encode(ProfileOf(source))))
	if err != nil {
		t.Fatal(err)
	}
	edit := hearing.NewProfileEdit(profile)
	if !input.ApplyTo(edit) || edit.Dirty || edit.Draft != profile.Settings {
		t.Errorf("moved: dirty %v, %+v", edit.Dirty, edit.Draft)
	}
}

func TestPartOfAFileMovesOnlyThatPart(t *testing.T) {
	input, err := ParseProfile([]byte(`{"left_db_hl": [10, 11, 12, 30, 14, 15, 16, 17], "tone": 0.25, "conversation_boost": true}`))
	if err != nil {
		t.Fatal(err)
	}
	profile := sampleProfile()
	edit := hearing.NewProfileEdit(profile)
	if !input.ApplyTo(edit) || !edit.Dirty {
		t.Fatal("not applied")
	}
	s := edit.Draft
	if s.LeftEQ[3] != 30 || s.RightEQ != profile.Settings.RightEQ || s.LeftTone != 0.25 || s.RightTone != 0.25 || !s.LeftConversationBoost || !s.RightConversationBoost {
		t.Errorf("%+v", s)
	}
	if s.LeftAmplification != profile.Settings.LeftAmplification || s.OwnVoice != profile.Settings.OwnVoice {
		t.Errorf("what the file does not name moved: %+v", s)
	}
}

func TestTheWholeShowDocumentIsRead(t *testing.T) {
	doc := `{"schema": 1, "editable": true, "bands_hz": [250, 500, 1000, 2000, 3000, 4000, 6000, 8000],
		"profile": {"amplification": 0.75}, "stored": {"own_voice": 0.5}}`
	input, err := ParseProfile([]byte(doc))
	if err != nil || input.Amplification == nil || *input.Amplification != 0.75 {
		t.Fatalf("%v %+v", err, input)
	}
}

func TestBadFilesAreRefused(t *testing.T) {
	for _, tc := range []struct{ name, text, want string }{
		{"not JSON", `{`, "unexpected"},
		{"a list", `[1]`, "cannot unmarshal"},
		{"misspelt", `{"tones": 0.5}`, "unknown field"},
		{"misspelt in a document", `{"schema": 1, "profile": {"tones": 0.5}}`, "unknown field"},
		{"another field beside a profile", `{"profile": {"tone": 0.5}, "x": 1}`, "unknown field"},
		{"newer schema", `{"schema": 2, "profile": {"tone": 0.5}}`, "schema 2"},
		{"empty", `{}`, "no profile values"},
		{"empty document", `{"schema": 1, "profile": null}`, "no profile values"},
		{"a second value", `{"tone": 0.5} {"tone": 0.25}`, "invalid character"},
		{"few bands", `{"left_db_hl": [1, 2]}`, "needs 8 values"},
		{"band too high", `{"right_db_hl": [1, 2, 3, 4, 5, 6, 7, 91]}`, "0 to 90"},
		{"band below zero", `{"left_db_hl": [-1, 2, 3, 4, 5, 6, 7, 8]}`, "0 to 90"},
		{"amplification", `{"amplification": 1.6}`, "amplification must be"},
		{"balance", `{"balance": -3.5}`, "balance must be"},
		{"tone", `{"tone": 2}`, "tone must be"},
		{"too large a number", `{"tone": 1e999}`, "1e999"},
		{"wrong type", `{"conversation_boost": "on"}`, "cannot unmarshal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseProfile([]byte(tc.text))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%v, want %q", err, tc.want)
			}
		})
	}
}

// TestTheNamesInTheDocumentAreTheCLIs stops a renamed field from passing as an unknown one.
func TestTheNamesInTheDocumentAreTheCLIs(t *testing.T) {
	var fields map[string]any
	if err := json.Unmarshal([]byte(Encode(ProfileValues{})), &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"left_db_hl", "right_db_hl", "amplification", "balance", "tone", "conversation_boost"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("no %s", name)
		}
	}
	if len(fields) != 6 {
		t.Errorf("%v", fields)
	}
}
