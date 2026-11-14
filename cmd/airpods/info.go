package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/argparse"
)

// jobContext is what a command needs before it has a device.
type jobContext struct {
	cli            *argparse.Parser
	ns             argparse.Namespace
	env            environment
	stdout, stderr io.Writer
}

// jobFunc is a command's work on an opened dongle.
type jobFunc func(*dongle.Client) error

// jobCommands check their arguments before any device is opened, and return the work to do.
var jobCommands = map[string]func(jobContext) (jobFunc, *argparse.Exit){
	"info":         infoJob,
	"hearing":      hearingJob,
	"hardware":     hardwareJob,
	"experimental": experimentalJob,
	"pair":         pairJob,
}

// readMetadata reads what the AirPods say about themselves. false means they said nothing.
func readMetadata(client *dongle.Client) (dongle.Metadata, bool, error) {
	status, err := client.Status()
	if err != nil {
		return dongle.Metadata{}, false, err
	}
	switch {
	case !status.Connected:
		return dongle.Metadata{}, false, errors.New("Connect the dongle's AirPods first.")
	case !status.Has(dongle.CapRawTransport):
		return dongle.Metadata{}, false, errors.New("Update the dongle firmware before reading AirPods information.")
	}
	report, err := client.Cached(dongle.SlotMetadata)
	if err != nil || report == nil {
		return dongle.Metadata{}, false, err
	}
	meta, ok := dongle.ParseMetadata(report.Packet)
	return meta, ok, nil
}

// reportedOr gives a value's text, or says that it was not reported.
func reportedOr(value string) string {
	if value == "" {
		return "not reported"
	}
	return value
}

func nullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// modelName is the catalog's name; an unlisted model has none.
func modelName(number string) string {
	if profile, ok := model.Find(number); ok {
		return profile.Name
	}
	return ""
}

type jsonControl struct {
	ID   string  `json:"id"`
	Name *string `json:"name"`
}

type jsonCapabilities struct {
	Listed             bool          `json:"listed"`
	Family             *string       `json:"family"`
	OverEar            bool          `json:"over_ear"`
	Listening          bool          `json:"listening"`
	Adaptive           bool          `json:"adaptive"`
	AllowOffOption     bool          `json:"allow_off_option"`
	HighQualityMic     bool          `json:"high_quality_mic"`
	CustomEQ           bool          `json:"custom_eq"`
	HearingAid         bool          `json:"hearing_aid"`
	CustomTransparency bool          `json:"custom_transparency"`
	LoudSoundReduction bool          `json:"loud_sound_reduction"`
	Controls           []jsonControl `json:"controls"`
}

type jsonInfo struct {
	Schema       int               `json:"schema"`
	ModelName    *string           `json:"model_name"`
	ModelNumber  *string           `json:"model_number"`
	Firmware     *string           `json:"firmware"`
	Serial       *string           `json:"serial"`
	SerialHidden bool              `json:"serial_hidden"`
	Capabilities *jsonCapabilities `json:"capabilities,omitempty"`
}

// settingForControl names a control when a setting of this tool reads it.
func settingForControl(control byte) string {
	for _, setting := range allSettings() {
		if setting.Control == control {
			return setting.Name
		}
	}
	return ""
}

func capabilitiesOf(number string) *jsonCapabilities {
	profile, ok := model.Find(number)
	if !ok {
		return &jsonCapabilities{Controls: []jsonControl{}}
	}
	result := &jsonCapabilities{
		Listed: true, Family: &profile.Family, OverEar: profile.Headphone, Listening: profile.Listening,
		Adaptive: profile.Adaptive, AllowOffOption: profile.AllowOffOption, HighQualityMic: profile.HighQualityMic,
		CustomEQ: profile.CustomEQ, HearingAid: profile.HearingAid, CustomTransparency: profile.CustomTransparency,
		LoudSoundReduction: profile.LoudSoundReduction, Controls: []jsonControl{},
	}
	for _, control := range profile.Controls {
		result.Controls = append(result.Controls, jsonControl{fmt.Sprintf("0x%02x", control), nullable(settingForControl(control))})
	}
	return result
}

func printCapabilities(stdout io.Writer, c *jsonCapabilities) {
	if !c.Listed {
		fmt.Fprintln(stdout, "Capabilities: this model is not in the catalog")
		return
	}
	fmt.Fprintln(stdout, "Capabilities (from the model catalog, not a report from these AirPods):")
	for _, flag := range []struct {
		name string
		on   bool
	}{
		{"over-ear", c.OverEar}, {"listening modes", c.Listening}, {"adaptive", c.Adaptive},
		{"allow-off option", c.AllowOffOption}, {"high quality microphone", c.HighQualityMic},
		{"custom EQ", c.CustomEQ}, {"hearing assistance", c.HearingAid},
		{"custom transparency", c.CustomTransparency}, {"loud sound reduction", c.LoudSoundReduction},
	} {
		fmt.Fprintf(stdout, "  %s: %s\n", flag.name, pick(flag.on, "yes", "no"))
	}
	fmt.Fprintln(stdout, "Controls:")
	for _, control := range c.Controls {
		name := "no setting in this tool"
		if control.Name != nil {
			name = *control.Name
		}
		fmt.Fprintf(stdout, "  %s: %s\n", control.ID, name)
	}
}

func infoJob(ctx jobContext) (jobFunc, *argparse.Exit) {
	showSerial, capabilities, asJSON := ctx.ns.Bool("show_serial"), ctx.ns.Bool("capabilities"), ctx.ns.Bool("json")
	return func(client *dongle.Client) error {
		meta, found, err := readMetadata(client)
		if err != nil {
			return err
		}
		name := modelName(meta.Model)
		var listed *jsonCapabilities
		if capabilities {
			listed = capabilitiesOf(meta.Model)
		}
		if asJSON {
			doc := jsonInfo{
				Schema: jsonSchema, ModelName: nullable(name), ModelNumber: nullable(meta.Model),
				Firmware: nullable(meta.Firmware), SerialHidden: !showSerial && meta.Serial != "", Capabilities: listed,
			}
			if showSerial {
				doc.Serial = nullable(meta.Serial)
			}
			if err := writeJSON(ctx.stdout, doc); err != nil {
				return err
			}
		} else {
			serial := reportedOr(meta.Serial)
			if !showSerial && meta.Serial != "" {
				serial = "hidden; use --show-serial"
			}
			if name == "" && meta.Model != "" {
				name = "unknown (not in the model catalog)"
			}
			fmt.Fprintln(ctx.stdout, "Model: "+reportedOr(name))
			fmt.Fprintln(ctx.stdout, "Model number: "+reportedOr(meta.Model))
			fmt.Fprintln(ctx.stdout, "Firmware: "+reportedOr(meta.Firmware))
			fmt.Fprintln(ctx.stdout, "Serial: "+serial)
			if listed != nil {
				printCapabilities(ctx.stdout, listed)
			}
		}
		if !found {
			return errUnreported // The same exit code as get on an unreported setting.
		}
		return nil
	}, nil
}
