package ui

import (
	"strings"
	"testing"

	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/hearing"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

// TestTurningAModeOnAsksFirstAndOffDoesNot runs both switches through a no, a yes and an off.
func TestTurningAModeOnAsksFirstAndOffDoesNot(t *testing.T) {
	for _, row := range []struct {
		name string
		mode hearing.Mode
		pick func(*hearingTab) modeControl
		flag func(hearing.ModeState) hearing.Flag
	}{
		{"Hearing Aid", hearing.HearingAid, func(h *hearingTab) modeControl { return h.aid }, func(s hearing.ModeState) hearing.Flag { return s.HearingEnabled }},
		{"Media Assist", hearing.MediaAssist, func(h *hearingTab) modeControl { return h.media }, func(s hearing.ModeState) hearing.Flag { return s.Media }},
	} {
		t.Run(row.name, func(t *testing.T) {
			r := experimentalRig(t, sim.Demo())
			h := r.w.hearing
			asked, accept := 0, false
			r.main(func() {
				r.w.confirm = func(title, text, _ string, answer func(bool)) {
					asked++
					if !strings.Contains(title, row.name) || text != i18n.T("hearing.warning") {
						t.Errorf("asked %q %q", title, text)
					}
					answer(accept)
				}
				h.readAll()
			})
			r.until(func() bool { return h.have && !r.w.hearingBusy }, "read")
			sent := len(r.dev.AAPSent())
			r.main(func() { row.pick(h).toggle.SetChecked(true) })
			r.until(func() bool { return asked == 1 && !r.w.hearingBusy }, "declined")
			r.main(func() {
				if len(r.dev.AAPSent()) != sent {
					t.Error("a no must send nothing")
				}
				if row.pick(h).toggle.Checked || row.flag(h.state) != hearing.FlagOff {
					t.Errorf("declined, yet the switch is on: %+v", h.state)
				}
				accept = true
				row.pick(h).toggle.SetChecked(true)
			})
			r.until(func() bool { return asked == 2 && row.flag(h.state) == hearing.FlagOn && !r.w.hearingBusy }, "accepted")
			r.main(func() { row.pick(h).toggle.SetChecked(false) })
			r.until(func() bool { return row.flag(h.state) == hearing.FlagOff && !r.w.hearingBusy }, "off")
			if asked != 2 {
				t.Errorf("turning off asked: %d questions", asked)
			}
		})
	}
}

// TestARefusedChangeAsksNothing has a mode that is not set up: the reason shows, with no question.
func TestARefusedChangeAsksNothing(t *testing.T) {
	a := sim.NewHearingAirPods("A3063", "9A348")
	a.Slots[2] = sim.CachedReport{Age: 1000, Revision: 1, Packet: []byte{4, 0, 4, 0, 9, 0, 0x2c, 2, 2, 0, 0}}
	r := experimentalRig(t, a.Config())
	h := r.w.hearing
	r.main(func() {
		r.w.confirm = func(_, _, _ string, answer func(bool)) {
			t.Error("a change that would be refused must not ask")
			answer(false)
		}
		h.readAll()
	})
	r.until(func() bool { return h.have && !r.w.hearingBusy }, "read")
	sent := len(r.dev.AAPSent())
	r.main(func() { h.aid.toggle.SetChecked(true) })
	r.until(func() bool {
		return !r.w.hearingBusy && h.status.Text != i18n.T("hearing.checking") && h.status.Text != ""
	}, "refused")
	r.main(func() {
		if h.aid.toggle.Checked || len(r.dev.AAPSent()) != sent {
			t.Errorf("refused, yet on or sent: %q", h.status.Text)
		}
	})
}

// TestApplyListsWhatWillChange checks the list and the warnings on the sim's profile, and that
// a profile that needs no enable still asks.
func TestApplyListsWhatWillChange(t *testing.T) {
	r := experimentalRig(t, sim.Demo())
	h := r.w.hearing
	var text, accept string
	asks := 0
	r.main(func() {
		r.w.confirm = func(_, body, button string, answer func(bool)) {
			asks++
			text, accept = body, button
			answer(false)
		}
		h.readAll()
	})
	r.until(func() bool { return h.have && !r.w.hearingBusy }, "read")
	sent := len(r.dev.AAPSent())
	r.main(func() {
		h.bands[0][3].SetText("40")
		h.tone.Slider.SetValue(0.5)
		h.applyDraft()
	})
	r.until(func() bool { return asks == 1 && !r.w.hearingBusy }, "asked")
	r.main(func() {
		before := h.edit.Baseline()
		for _, want := range []string{
			"This will change:",
			"Left 2000 Hz: " + bandText(before.LeftEQ[3]) + " -> 40 dB HL",
			"Left tone: " + bandText(before.LeftTone) + " -> 0.5",
			"Right tone: " + bandText(before.RightTone) + " -> 0.5",
			i18n.T("hearing.warning"),
		} {
			if !strings.Contains(text, want) {
				t.Errorf("no %q in %q", want, text)
			}
		}
		if strings.Contains(text, "Left gain") {
			t.Errorf("what did not change is listed: %q", text)
		}
		if accept != i18n.T("hearing.apply") || len(r.dev.AAPSent()) != sent {
			t.Errorf("button %q, or a no sent something", accept)
		}
		r.w.confirm = func(_, _, _ string, answer func(bool)) { answer(true) }
		h.aid.toggle.SetChecked(true)
	})
	r.until(func() bool { return h.state.HearingEnabled == hearing.FlagOn && !r.w.hearingBusy }, "on")
	r.main(func() {
		r.w.confirm = func(_, body, button string, answer func(bool)) {
			asks++
			text, accept = body, button
			answer(false)
		}
		h.applyDraft()
	})
	r.until(func() bool { return asks == 2 && !r.w.hearingBusy }, "asked again")
	r.main(func() {
		if strings.Contains(text, i18n.T("hearing.enableText")) || accept != i18n.T("hearing.apply") || !strings.Contains(text, "This will change:") {
			t.Errorf("no enable needed: %q, %q", accept, text)
		}
	})
}

func TestApplyWithNoDifferenceSendsNothing(t *testing.T) {
	r := experimentalRig(t, sim.Demo())
	h := r.w.hearing
	r.main(func() {
		r.w.confirm = func(_, _, _ string, answer func(bool)) {
			t.Error("nothing to ask")
			answer(false)
		}
		h.readAll()
	})
	r.until(func() bool { return h.have && !r.w.hearingBusy }, "read")
	r.main(func() {
		first := h.bands[0][0].Text
		h.bands[0][0].SetText("44")
		h.bands[0][0].SetText(first)
		h.applyDraft()
		if h.status.Text != i18n.T("hearing.noChanges") {
			t.Errorf("%q", h.status.Text)
		}
	})
}

// TestHearingShowsSetUpAndReportAges has one mode not set up, one with no report at all behind its
// age, and the ages in the CLI's wording.
func TestHearingShowsSetUpAndReportAges(t *testing.T) {
	a := sim.NewHearingAirPods("A3063", "9A348")
	a.Slots[3] = sim.CachedReport{Age: 2500, Revision: 1, Packet: []byte{4, 0, 4, 0, 9, 0, 0x33, 2, 0, 0, 0}}
	a.Slots[4] = sim.CachedReport{Age: 700, Revision: 1, Packet: sim.PMERecord()}
	r := experimentalRig(t, a.Config())
	h := r.w.hearing
	r.main(func() {
		if h.reportBox.Visible() {
			t.Error("nothing was read yet")
		}
		h.readAll()
	})
	r.until(func() bool { return h.have && !r.w.hearingBusy }, "read")
	r.main(func() {
		if !h.reportBox.Visible() || h.aidSetUp.Text != "yes" {
			t.Errorf("%q", h.aidSetUp.Text)
		}
		if want := "Report ages: hearing aid 1000 ms, assist 2500 ms, media 700 ms"; h.ages.Text != want {
			t.Errorf("%q", h.ages.Text)
		}
		i18n.SetLanguage(i18n.German)
		defer i18n.SetLanguage(i18n.English)
		if h.aidSetUp.Text != "ja" || !strings.HasPrefix(h.ages.Text, "Alter der Meldungen:") {
			t.Errorf("the language did not follow: %q %q", h.aidSetUp.Text, h.ages.Text)
		}
	})
	for flag, want := range map[hearing.Flag]string{hearing.FlagOn: "yes", hearing.FlagOff: "no", hearing.FlagUnknown: "unknown (not reported)"} {
		if got := setUpText(flag); got != want {
			t.Errorf("%v: %q", flag, got)
		}
	}
	if got := reportAgeText(nil); got != "no report" {
		t.Errorf("%q", got)
	}
}

// TestReportAgesFollowAChange reads them again once the change ended.
func TestReportAgesFollowAChange(t *testing.T) {
	a := sim.NewHearingAirPods("A3063", "9A348")
	r := experimentalRig(t, a.Config())
	h := r.w.hearing
	r.main(func() {
		r.w.confirm = func(_, _, _ string, answer func(bool)) { answer(true) }
		h.readAll()
	})
	r.until(func() bool { return h.have && !r.w.hearingBusy }, "read")
	r.main(func() { h.aid.toggle.SetChecked(true) })
	r.until(func() bool {
		return h.state.HearingEnabled == hearing.FlagOn && !r.w.hearingBusy && h.reports != nil && h.reports.HearingAid != nil && *h.reports.HearingAid < 1000
	}, "fresh ages")
	r.main(func() {
		if strings.Contains(h.ages.Text, "hearing aid 1000 ms") {
			t.Errorf("%q", h.ages.Text)
		}
	})
}
