package statusdoc

import (
	"fmt"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/model"
)

// Control is one control the catalog lists, with the setting that reads it. Name is nil when no
// setting does.
type Control struct {
	ID   string  `json:"id"`
	Name *string `json:"name"`
}

// Capabilities is what the catalog lists for a model, the capabilities object of
// `airpods --json info --capabilities`. It is not a report from the AirPods.
type Capabilities struct {
	Listed             bool      `json:"listed"`
	Family             *string   `json:"family"`
	OverEar            bool      `json:"over_ear"`
	Listening          bool      `json:"listening"`
	Adaptive           bool      `json:"adaptive"`
	AllowOffOption     bool      `json:"allow_off_option"`
	HighQualityMic     bool      `json:"high_quality_mic"`
	CustomEQ           bool      `json:"custom_eq"`
	HearingAid         bool      `json:"hearing_aid"`
	CustomTransparency bool      `json:"custom_transparency"`
	LoudSoundReduction bool      `json:"loud_sound_reduction"`
	Controls           []Control `json:"controls"`
}

func settingForControl(control byte) *string {
	for _, table := range [][]dongle.Setting{dongle.Settings, dongle.ExtraSettings} {
		for i := range table {
			if table[i].Control == control {
				name := table[i].Name
				return &name
			}
		}
	}
	return nil
}

// CapabilitiesOf looks a model number up.
func CapabilitiesOf(number string) *Capabilities {
	profile, ok := model.Find(number)
	if !ok {
		return &Capabilities{Controls: []Control{}}
	}
	result := &Capabilities{
		Listed: true, Family: &profile.Family, OverEar: profile.Headphone, Listening: profile.Listening,
		Adaptive: profile.Adaptive, AllowOffOption: profile.AllowOffOption, HighQualityMic: profile.HighQualityMic,
		CustomEQ: profile.CustomEQ, HearingAid: profile.HearingAid, CustomTransparency: profile.CustomTransparency,
		LoudSoundReduction: profile.LoudSoundReduction, Controls: []Control{},
	}
	for _, control := range profile.Controls {
		result.Controls = append(result.Controls, Control{fmt.Sprintf("0x%02x", control), settingForControl(control)})
	}
	return result
}
