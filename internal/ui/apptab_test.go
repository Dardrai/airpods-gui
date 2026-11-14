package ui

import (
	"errors"
	"strings"
	"testing"

	"fyne.io/fyne/v2/theme"

	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/internal/i18n"
	"github.com/Dardrai/airpods-gui/internal/tray"
)

func TestCloseHidesWithTrayAndQuitsWithout(t *testing.T) {
	r := newRig(t, sim.Demo())
	answer := tray.Yes
	r.main(func() {
		r.w.opts.Tray = func() tray.Answer { return answer }
		r.w.Show()
		r.w.CloseRequested()
		if r.w.Visible() || r.quits != 0 {
			t.Errorf("with tray: visible %v quits %d", r.w.Visible(), r.quits)
		}
		answer = tray.No
		r.w.Show()
		r.w.CloseRequested()
		if r.quits != 1 {
			t.Errorf("without tray: quits %d", r.quits)
		}
	})
}

// Before the tray check has answered, and for ever with a session bus that hangs, closing
// must not quit the app on a guess; the answer, when it comes, does what the close meant.
func TestCloseBeforeTheTrayAnswerHidesAndTheAnswerDecides(t *testing.T) {
	r := newRig(t, sim.Demo())
	answer, rechecks := tray.Unknown, 0
	r.main(func() {
		r.w.opts.Tray = func() tray.Answer { return answer }
		r.w.opts.TrayRecheck = func() { rechecks++ }
		r.w.Show()
		if rechecks != 1 {
			t.Errorf("%d checks asked for on show", rechecks)
		}
		r.w.CloseRequested()
		if r.w.Visible() || r.quits != 0 {
			t.Fatalf("closed while unknown: visible %v quits %d", r.w.Visible(), r.quits)
		}
		// There is a tray: the window stays where the user put it.
		answer = tray.Yes
		r.w.TrayAnswered(answer)
		if r.w.Visible() || r.quits != 0 {
			t.Errorf("a tray answered: visible %v quits %d", r.w.Visible(), r.quits)
		}
		// A later change of answer has no close left to put right.
		r.w.TrayAnswered(tray.No)
		if r.w.Visible() || r.quits != 0 {
			t.Errorf("an answer with no blind close: visible %v quits %d", r.w.Visible(), r.quits)
		}

		answer = tray.Unknown
		r.w.Show()
		r.w.CloseRequested()
		if r.w.Visible() || r.quits != 0 {
			t.Fatalf("closed while unknown again: visible %v quits %d", r.w.Visible(), r.quits)
		}
		// No tray: the close meant quitting, and now it does.
		answer = tray.No
		r.w.TrayAnswered(answer)
		if r.w.Visible() || r.quits != 1 {
			t.Errorf("no tray answered: visible %v quits %d", r.w.Visible(), r.quits)
		}
		if rechecks != 2 {
			t.Errorf("%d checks asked for over two shows", rechecks)
		}
		// The answer is applied once.
		r.w.TrayAnswered(answer)
		if r.quits != 1 {
			t.Errorf("answered again: quits %d", r.quits)
		}
	})
}

// A window the user opened again before the answer is not opened or closed by it.
func TestTrayAnswerLeavesAReopenedWindowAlone(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.main(func() {
		r.w.opts.Tray = func() tray.Answer { return tray.Unknown }
		r.w.Show()
		r.w.CloseRequested()
		r.w.Show()
		r.w.Hide() // as the tray's own row would, were there one
		r.w.TrayAnswered(tray.No)
		if r.w.Visible() || r.quits != 0 {
			t.Errorf("visible %v quits %d", r.w.Visible(), r.quits)
		}
	})
}

func TestStartupRepairWarning(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.main(func() {
		entry := &fakeAutostart{stale: true}
		r.w.opts.Autostart = entry
		tab := newAppTab(r.w)
		if !tab.repair.Visible() || tab.startup.Checked {
			t.Error("a stale entry must warn and read as off")
		}
		entry.stale = false
		tab.startup.SetChecked(true)
		if tab.repair.Visible() || !entry.on {
			t.Error("turning it on repairs it")
		}
	})
}

func TestExperimentalIsOffByDefaultAndStored(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.main(func() {
		if r.w.Experimental() || r.w.about.experimental.Checked {
			t.Error("must start off")
		}
		r.w.about.experimental.SetChecked(true)
		if !r.w.Experimental() {
			t.Error("not stored")
		}
	})
}

func TestThemeDefaultsToDarkInThePickerToo(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.main(func() {
		if got := r.w.about.theme.Selected; got != i18n.T("app.themeDark") {
			t.Errorf("selected %q", got)
		}
		if current, ok := r.w.opts.Fyne.Settings().Theme().(pageTheme); !ok || !current.forced || current.variant != theme.VariantDark {
			t.Errorf("window theme %#v", r.w.opts.Fyne.Settings().Theme())
		}
	})
}

func TestThemeChoiceIsStored(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.main(func() {
		r.w.about.theme.SetSelected("Light")
		if got := r.w.opts.Fyne.Preferences().String(themePreference); got != ThemeLight {
			t.Errorf("stored %q", got)
		}
		r.w.about.theme.SetSelected("Dark")
	})
}

type fakeAutostart struct {
	on    bool
	stale bool
	err   error
}

func (f *fakeAutostart) Enabled() (bool, error) { return f.on, nil }

func (f *fakeAutostart) Stale() (bool, error) { return f.stale, nil }

func (f *fakeAutostart) SetEnabled(on bool) error {
	if f.err != nil {
		return f.err
	}
	f.on = on
	return nil
}

func TestAutostartCheckbox(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.main(func() {
		if r.w.about.startup != nil {
			t.Error("shown without a manager")
		}
		entry := &fakeAutostart{on: true}
		r.w.opts.Autostart = entry
		tab := newAppTab(r.w)
		if !tab.startup.Checked {
			t.Error("does not read the current state")
		}
		tab.startup.SetChecked(false)
		if entry.on {
			t.Error("not turned off")
		}
		entry.err = errors.New("access denied")
		tab.startup.SetChecked(true)
		if tab.startup.Checked {
			t.Error("a failed change must show the actual state")
		}
		if !strings.Contains(r.w.banner.Text, "access denied") {
			t.Errorf("banner %q", r.w.banner.Text)
		}
	})
}
