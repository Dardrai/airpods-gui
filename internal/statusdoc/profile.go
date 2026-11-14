package statusdoc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Dardrai/airpods-gui/core/hearing"
)

// ProfileValues is the profile object of `airpods --json hearing profile show`, which is also the
// file `airpods hearing profile apply --file` takes.
type ProfileValues struct {
	Left              []float32 `json:"left_db_hl"`
	Right             []float32 `json:"right_db_hl"`
	Amplification     float32   `json:"amplification"`
	Balance           float32   `json:"balance"`
	Tone              float32   `json:"tone"`
	ConversationBoost bool      `json:"conversation_boost"`
}

// ProfileOf is what a draft holds, in that shape.
func ProfileOf(edit *hearing.ProfileEdit) ProfileValues {
	s := edit.Draft
	return ProfileValues{
		Left: s.LeftEQ[:], Right: s.RightEQ[:], Amplification: edit.Amplification, Balance: edit.Balance,
		Tone: (s.LeftTone + s.RightTone) / 2, ConversationBoost: s.LeftConversationBoost && s.RightConversationBoost,
	}
}

// ProfileInput is what a file can change. A nil field stays as stored.
type ProfileInput struct {
	Left              *[]float64 `json:"left_db_hl"`
	Right             *[]float64 `json:"right_db_hl"`
	Amplification     *float64   `json:"amplification"`
	Balance           *float64   `json:"balance"`
	Tone              *float64   `json:"tone"`
	ConversationBoost *bool      `json:"conversation_boost"`
}

// showDocument is all of `airpods --json hearing profile show`. Only its profile is read.
type showDocument struct {
	Schema   *int            `json:"schema"`
	Editable *bool           `json:"editable"`
	BandsHz  []int           `json:"bands_hz"`
	Profile  *ProfileInput   `json:"profile"`
	Stored   json.RawMessage `json:"stored"`
}

func strictDecode(data []byte, into any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields() // A misspelt field must not quietly leave a value as it is.
	if err := decoder.Decode(into); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("expected one JSON object")
	}
	return nil
}

// ParseProfile reads a profile file: the apply file's object, or the whole show document. Any
// other field, a value out of range or a file with nothing in it is an error, as in the CLI.
func ParseProfile(data []byte) (ProfileInput, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return ProfileInput{}, err
	}
	var input ProfileInput
	if _, whole := probe["profile"]; whole {
		var doc showDocument
		if err := strictDecode(data, &doc); err != nil {
			return input, err
		}
		if doc.Schema != nil && *doc.Schema != Schema {
			return input, fmt.Errorf("schema %d is not one this app reads", *doc.Schema)
		}
		if doc.Profile != nil {
			input = *doc.Profile
		}
	} else if err := strictDecode(data, &input); err != nil {
		return input, err
	}
	if input.empty() {
		return input, errors.New("it has no profile values")
	}
	return input, input.check()
}

func (p ProfileInput) empty() bool {
	return p.Left == nil && p.Right == nil && p.Amplification == nil && p.Balance == nil && p.Tone == nil && p.ConversationBoost == nil
}

func checkRange(name string, value *float64, low, high float64) error {
	if value != nil && !(*value >= low && *value <= high) { // Also catches NaN.
		return fmt.Errorf("%s must be %g to %g", name, low, high)
	}
	return nil
}

func bandNames() string {
	names := make([]string, len(hearing.BandHz))
	for i, hz := range hearing.BandHz {
		names[i] = strconv.Itoa(hz)
	}
	return strings.Join(names, ", ")
}

func (p ProfileInput) check() error {
	for _, ear := range []struct {
		name  string
		bands *[]float64
	}{{"left", p.Left}, {"right", p.Right}} {
		if ear.bands == nil {
			continue
		}
		if len(*ear.bands) != hearing.Bands {
			return fmt.Errorf("the %s ear needs %d values, for %s Hz", ear.name, hearing.Bands, bandNames())
		}
		for _, value := range *ear.bands {
			if !(value >= 0 && value <= 90) {
				return fmt.Errorf("every %s ear value must be 0 to 90 dB HL", ear.name)
			}
		}
	}
	return errors.Join(checkRange("amplification", p.Amplification, -1.5, 1.5), checkRange("balance", p.Balance, -3, 3), checkRange("tone", p.Tone, -1, 1))
}

func curveOf(stored [hearing.Bands]float32, given *[]float64) ([]float64, bool) {
	current := make([]float64, hearing.Bands)
	for i, value := range stored {
		current[i] = float64(value)
	}
	if given == nil {
		return current, false
	}
	changed := false
	for i, value := range *given {
		if float32(value) != stored[i] {
			changed = true
		}
	}
	return *given, changed
}

// ApplyTo moves the draft only for values that differ from what the edit shows, because a combined
// slider that moves rewrites both ears' stored gains or tones. So a file made from a profile
// changes nothing in it. The result is false if the draft refused a value.
func (p ProfileInput) ApplyTo(edit *hearing.ProfileEdit) bool {
	draft := edit.Draft
	left, leftChanged := curveOf(draft.LeftEQ, p.Left)
	right, rightChanged := curveOf(draft.RightEQ, p.Right)
	ok := true
	if leftChanged || rightChanged {
		ok = edit.EditCurve(left, right)
	}
	if p.Amplification != nil && float32(*p.Amplification) != edit.Amplification {
		ok = edit.EditAmplification(float32(*p.Amplification)) && ok
	}
	if p.Balance != nil && float32(*p.Balance) != edit.Balance {
		ok = edit.EditBalance(float32(*p.Balance)) && ok
	}
	if p.Tone != nil && float32(*p.Tone) != (draft.LeftTone+draft.RightTone)/2 {
		ok = edit.EditTone(float32(*p.Tone)) && ok
	}
	if p.ConversationBoost != nil && *p.ConversationBoost != (draft.LeftConversationBoost && draft.RightConversationBoost) {
		ok = edit.EditConversationBoost(*p.ConversationBoost) && ok
	}
	return ok
}
