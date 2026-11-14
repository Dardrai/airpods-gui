package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

func dongleList() []hid.Info {
	return []hid.Info{
		{Path: "a", Serial: "SIM0001", Openable: true},
		{Path: "b", Serial: "SIM0002", Openable: true},
		{Path: "c", Serial: "SIM0003", Err: errors.New("no permission")},
		{Path: "d", Openable: true},
	}
}

func controlled(chosen *[]string) func(*Options) {
	return func(o *Options) {
		o.Controls = &AppControls{
			AutoPauseEnabled: func() bool { return false }, SetAutoPauseEnabled: func(bool) {},
			NotificationsEnabled: func() bool { return false }, SetNotificationsEnabled: func(bool) {},
			ModeNotificationsEnabled: func() bool { return false }, SetModeNotificationsEnabled: func(bool) {},
			Language: func() string { return i18n.English }, SetLanguage: func(string) {}, ShowBatteries: func() {},
			ChooseDongle: func(serial string) { *chosen = append(*chosen, serial) },
		}
	}
}

func TestPickerShowsOnlyWithSeveralDongles(t *testing.T) {
	var chosen []string
	r := newRigWith(t, sim.Demo(), controlled(&chosen))
	about := func() *appTab { return r.w.about }
	r.main(func() {
		if about().dongleBox.Visible() {
			t.Error("no list yet, yet the picker shows")
		}
		r.w.SetDongles(dongleList()[:1])
		if about().dongleBox.Visible() {
			t.Error("one dongle, yet the picker shows")
		}
		r.w.SetDongles(dongleList())
		if !about().dongleBox.Visible() {
			t.Fatal("several dongles, yet no picker")
		}
		if got := strings.Join(about().dongle.Options, " "); got != "SIM0001 SIM0002" {
			t.Errorf("options %q", got)
		}
		text := about().dongleReasons.Text
		if !strings.Contains(text, "SIM0003: no permission") || !strings.Contains(text, i18n.T("app.dongleNoSerial")) {
			t.Errorf("reasons %q", text)
		}
		about().dongle.SetSelected("SIM0002")
		if len(chosen) != 1 || chosen[0] != "SIM0002" {
			t.Errorf("chosen %q", chosen)
		}
		r.w.SetDongles(dongleList()[:1])
		if about().dongleBox.Visible() {
			t.Error("back to one, yet the picker stays")
		}
	})
}

func TestPickerHoldsStillWhileWorkRuns(t *testing.T) {
	var chosen []string
	r := newRigWith(t, sim.Demo(), controlled(&chosen))
	r.main(func() {
		r.w.SetDongles(dongleList())
		r.w.setHearingBusy(true)
		if !r.w.about.dongle.Disabled() {
			t.Error("the picker works during a hearing change")
		}
		r.w.setHearingBusy(false)
		if r.w.about.dongle.Disabled() {
			t.Error("the picker stays disabled")
		}
	})
}

func TestHeaderNamesTheDongleOnlyWithSeveral(t *testing.T) {
	st := device.State{Present: true, Serial: "SIM0001"}
	st.Status.Connected, st.Status.ControlsReady = true, true
	if _, detail := connectionText(st); detail != "" {
		t.Errorf("one dongle: %q", detail)
	}
	st.Several = true
	if _, detail := connectionText(st); !strings.Contains(detail, "SIM0001") {
		t.Errorf("several: %q", detail)
	}
	status, detail := connectionText(device.State{Several: true, ChooseDongle: true})
	if status != i18n.T("state.chooseDongle") || detail != i18n.T("page.chooseDongle") {
		t.Errorf("%q %q", status, detail)
	}
}
