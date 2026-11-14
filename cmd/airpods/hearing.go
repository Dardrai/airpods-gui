package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/hearing"
	"github.com/Dardrai/airpods-gui/internal/argparse"
)

// A hearing.Modes lives in memory, so this process is all it knows about. What it can keep from
// one run to the next is nothing, and that shapes two things here. A pending Recovery is known only
// to the run whose change went unconfirmed, so that run offers it at once when somebody is there to
// answer. A later run starts from the dongle's kept reports, and hearing recover --off then asks
// for an ordinary Modes.Set(mode, false), with every check Modes.StartRecover would make.

const hearingWarning = "Warning: this changes how sound is presented, including how loud it is, and has not been verified on real AirPods. If it sounds wrong or too loud, take the AirPods out, then turn it off."

// errUncertain ends a command with exit 3 when what it had to say is already printed.
var errUncertain = errors.New("outcome unknown")

func modeWord(mode hearing.Mode) string {
	if mode == hearing.MediaAssist {
		return "media-assist"
	}
	return "hearing-aid"
}

// requireHearingModel applies the model flag to reads too; the mutating calls enforce it again.
func requireHearingModel(client *dongle.Client) error {
	meta, _, err := readMetadata(client)
	if err != nil {
		return err
	}
	if !hearing.Supported(meta.Model) {
		return &hearing.Refused{Reason: "Hearing assistance is not supported by the model these AirPods report."}
	}
	return nil
}

func reportAge(client *dongle.Client, slot byte) *uint32 {
	report, err := client.Cached(slot)
	if err != nil || report == nil {
		return nil
	}
	age := report.AgeMs
	return &age
}

// hearingReports is how old each kept report is. A report that predates a change looks the same as
// one that follows it, so the age is the one clue a new process has.
type hearingReports struct {
	HearingAid *uint32 `json:"hearing_aid"`
	Assist     *uint32 `json:"assist"`
	Media      *uint32 `json:"media"`
}

func readHearingReports(client *dongle.Client) hearingReports {
	return hearingReports{
		HearingAid: reportAge(client, dongle.SlotHearingAid),
		Assist:     reportAge(client, dongle.SlotHearingMedia),
		Media:      reportAge(client, dongle.SlotHearingPME),
	}
}

func flagText(flag hearing.Flag) string {
	if flag == hearing.FlagUnknown {
		return "unknown (not reported; this does not mean off)"
	}
	return flag.String()
}

func enrolledText(flag hearing.Flag) string {
	switch flag {
	case hearing.FlagOn:
		return "yes"
	case hearing.FlagOff:
		return "no"
	}
	return "unknown (not reported)"
}

func ageText(age *uint32) string {
	if age == nil {
		return "no report"
	}
	return fmt.Sprintf("%d ms", *age)
}

func printHearingState(stdout io.Writer, state hearing.ModeState, ages hearingReports) {
	fmt.Fprintln(stdout, "Hearing Aid: "+flagText(state.HearingEnabled))
	fmt.Fprintln(stdout, "Media Assist: "+flagText(state.Media))
	fmt.Fprintln(stdout, "Hearing Aid set up: "+enrolledText(state.HearingEnrolled))
	fmt.Fprintln(stdout, "Media Assist set up: "+enrolledText(state.VoiceEnrolled))
	fmt.Fprintf(stdout, "Report ages: hearing aid %s, assist %s, media %s\n", ageText(ages.HearingAid), ageText(ages.Assist), ageText(ages.Media))
}

type jsonHearingStatus struct {
	Schema          int            `json:"schema"`
	HearingAid      string         `json:"hearing_aid"`
	MediaAssist     string         `json:"media_assist"`
	HearingEnrolled string         `json:"hearing_enrolled"`
	VoiceEnrolled   string         `json:"voice_enrolled"`
	AssistEnabled   string         `json:"assist_enabled"`
	VoiceEnabled    string         `json:"voice_enabled"`
	MediaEnabled    string         `json:"media_enabled"`
	ReportAgeMs     hearingReports `json:"report_age_ms"`
}

func hearingStatusJob(ctx jobContext) (jobFunc, *argparse.Exit) {
	asJSON := ctx.ns.Bool("json")
	return func(client *dongle.Client) error {
		modes := new(hearing.Modes)
		state, err := modes.Read(client)
		if err != nil {
			return err
		}
		if err := requireHearingModel(client); err != nil {
			return err
		}
		ages := readHearingReports(client)
		if asJSON {
			return writeJSON(ctx.stdout, jsonHearingStatus{
				Schema: jsonSchema, HearingAid: state.HearingEnabled.String(), MediaAssist: state.Media.String(),
				HearingEnrolled: state.HearingEnrolled.String(), VoiceEnrolled: state.VoiceEnrolled.String(),
				AssistEnabled: state.AssistEnabled.String(), VoiceEnabled: state.VoiceEnabled.String(),
				MediaEnabled: state.PMEMediaEnabled.String(), ReportAgeMs: ages,
			})
		}
		printHearingState(ctx.stdout, state, ages)
		return nil
	}, nil
}

// hearingFailure says what a failed change left behind. Exit 3 means the state is unknown, and
// that also covers an error that reads as a refusal when Hearing Aid may have stayed on behind it.
// When this process still holds the Recovery it offers it, since no later run can have it.
func hearingFailure(ctx jobContext, client *dongle.Client, modes *hearing.Modes, err error) error {
	var mode *hearing.ModeUnconfirmed
	var plain *dongle.Unconfirmed
	recovery := modes.Recovery()
	if !errors.As(err, &mode) && !errors.As(err, &plain) && recovery == nil {
		return err
	}
	fmt.Fprintln(ctx.stderr, err)
	fmt.Fprintln(ctx.stderr, "It may have applied. Read it again with airpods hearing status.")
	if recovery == nil {
		return errUncertain
	}
	word := modeWord(recovery.Mode)
	fmt.Fprintf(ctx.stderr, "%s Run: airpods hearing recover --off %s\n", recovery.Text(), word)
	if yes, _ := askUser(fmt.Sprintf("Turn %s off now?", recovery.Mode)); !yes {
		return errUncertain
	}
	// Only this process has the last good packets, so this can work where a later run cannot.
	if _, err := modes.Recover(client); err != nil {
		fmt.Fprintln(ctx.stderr, err)
		return errUncertain
	}
	fmt.Fprintf(ctx.stdout, "%s: off (confirmed by the AirPods)\n", word)
	return errUnreported // The change that was asked for did not happen.
}

// modeSet is hearing aid and hearing media-assist. It turns a mode on only after the warning and a
// yes, and those come after the plan so that a change that would be refused asks nothing.
func modeSet(ctx jobContext, mode hearing.Mode) (jobFunc, *argparse.Exit) {
	word, _ := ctx.ns.String("state")
	on, yes := word == "on", ctx.ns.Bool("yes")
	return func(client *dongle.Client) error {
		modes := new(hearing.Modes)
		if _, err := modes.Read(client); err != nil {
			return err
		}
		if err := requireHearingModel(client); err != nil {
			return err
		}
		if on {
			plan, err := modes.Plan(mode, true)
			if err != nil {
				return err
			}
			if len(plan.Packets()) > 0 {
				fmt.Fprintln(ctx.stderr, hearingWarning)
				if !yes {
					if err := confirmHearing(fmt.Sprintf("Turn %s on?", mode), fmt.Sprintf("turning %s on", mode)); err != nil {
						return err
					}
				}
			}
		}
		result, err := modes.Set(client, mode, on)
		if err != nil {
			return hearingFailure(ctx, client, modes, err)
		}
		if result.Sent {
			fmt.Fprintf(ctx.stdout, "%s: %s (confirmed by the AirPods)\n", modeWord(mode), word)
		} else {
			fmt.Fprintf(ctx.stdout, "%s: %s (already reported; no write sent)\n", modeWord(mode), word)
		}
		return nil
	}, nil
}

// confirmHearing asks a person. It never writes, and nobody being there is an answer of no.
func confirmHearing(question, what string) error {
	accepted, available := askUser(question)
	switch {
	case !available:
		return fmt.Errorf("Nothing was sent: %s needs --yes, or a terminal to confirm in.", what)
	case !accepted:
		return errors.New("Nothing was sent: not confirmed.")
	}
	return nil
}

func hearingRecoverJob(ctx jobContext) (jobFunc, *argparse.Exit) {
	off, given := ctx.ns.String("off")
	return func(client *dongle.Client) error {
		modes := new(hearing.Modes)
		state, err := modes.Read(client)
		if err != nil {
			return err
		}
		if err := requireHearingModel(client); err != nil {
			return err
		}
		ages := readHearingReports(client)
		if !given {
			printHearingState(ctx.stdout, state, ages)
			printRecoverAdvice(ctx.stdout, state)
			return nil
		}
		mode := hearing.HearingAid
		if off == "media-assist" {
			mode = hearing.MediaAssist
		}
		result, err := modes.Set(client, mode, false)
		if err != nil {
			return hearingFailure(ctx, client, modes, err)
		}
		if result.Sent {
			fmt.Fprintf(ctx.stdout, "%s: off (confirmed by the AirPods)\n", off)
			return nil
		}
		fmt.Fprintf(ctx.stdout, "%s: already reported off; no write sent\n", off)
		fmt.Fprintln(ctx.stdout, "That report may be older than the change it should show. If it still sounds on, take the AirPods out, run airpods reconnect, then airpods hearing status.")
		return nil
	}, nil
}

// printRecoverAdvice writes nothing to the AirPods: without --off this command only reads.
func printRecoverAdvice(stdout io.Writer, state hearing.ModeState) {
	any := false
	for _, row := range []struct {
		mode hearing.Mode
		flag hearing.Flag
	}{{hearing.HearingAid, state.HearingEnabled}, {hearing.MediaAssist, state.Media}} {
		if row.flag == hearing.FlagOff {
			continue
		}
		any = true
		fmt.Fprintf(stdout, "%s may be active. To turn it off: airpods hearing recover --off %s\n", row.mode, modeWord(row.mode))
	}
	if !any {
		fmt.Fprintln(stdout, "Both report off. A report can be older than a change that went unconfirmed: if a report is older than that change, run airpods reconnect, then read again.")
	}
}

func hearingJob(ctx jobContext) (jobFunc, *argparse.Exit) {
	subcommand, _ := ctx.ns.String("subcommand")
	switch subcommand {
	case "status":
		return hearingStatusJob(ctx)
	case "aid":
		return modeSet(ctx, hearing.HearingAid)
	case "media-assist":
		return modeSet(ctx, hearing.MediaAssist)
	case "profile":
		if action, _ := ctx.ns.String("action"); action == "show" {
			return profileShowJob(ctx)
		}
		return profileApplyJob(ctx)
	}
	return hearingRecoverJob(ctx)
}
