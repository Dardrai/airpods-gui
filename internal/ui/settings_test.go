package ui

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/choices"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

func TestEverySettingHasARowOnScreen(t *testing.T) {
	r := newRig(t, sim.Demo())
	want := len(dongle.Settings) + len(dongle.ExtraSettings)
	if len(r.w.settings.rows) != want {
		t.Fatalf("rows %d, want %d", len(r.w.settings.rows), want)
	}
	placed := map[string]bool{}
	for _, group := range settingGroups {
		for _, name := range group.names {
			if _, ok := dongle.LookupAnySetting(name); !ok {
				t.Errorf("group lists unknown setting %s", name)
			}
			if placed[name] {
				t.Errorf("%s is in two groups", name)
			}
			placed[name] = true
		}
	}
	if len(placed) != want {
		t.Errorf("groups place %d of %d", len(placed), want)
	}
	// Every row is in a card that is on the page, the experimental one included.
	r.main(func() {
		for _, row := range r.w.settings.rows {
			if _, ok := positionOf(r.w.page, row.body, fyne.Position{}); !ok {
				t.Errorf("%s is not on the page", row.setting.Name)
			}
		}
	})
}

func TestWidgetKinds(t *testing.T) {
	r := newRig(t, sim.Demo())
	rows := r.w.settings.byName
	if rows["ear-detection"].toggle == nil {
		t.Errorf("ear-detection is %T", rows["ear-detection"].control)
	}
	if rows["microphone"].choice == nil {
		t.Errorf("microphone is %T", rows["microphone"].control)
	}
	if rows["call-controls"].choice == nil {
		t.Errorf("call-controls is %T", rows["call-controls"].control)
	}
	if rows["mode"].segment == nil {
		t.Errorf("mode is %T", rows["mode"].control)
	}
}

func TestHoldActionWordsMatchTheLibrary(t *testing.T) {
	setting, _ := dongle.LookupSetting("hold-actions")
	for _, left := range holdActionWords {
		for _, right := range holdActionWords {
			raw, err := setting.Encode(left + "," + right)
			if err != nil {
				t.Fatalf("%s,%s: %v", left, right, err)
			}
			if got := setting.Decode(raw); got != left+","+right {
				t.Errorf("round trip %q", got)
			}
		}
	}
}

func TestUnreportedSettingsAreUsableOrDimWithOneHint(t *testing.T) {
	cfg := sim.Demo()
	cfg.Settings = []sim.Setting{{Control: 0x0d, Age: 1, Revision: 1, Value: 2}}
	r := newRig(t, cfg)
	r.main(r.w.Show)
	r.until(func() bool { return r.w.settings.byName["mode"].reported }, "read")
	r.settle()
	r.main(func() {
		row := r.w.settings.byName["ear-detection"]
		if row.note.Text != "" || row.reported {
			t.Errorf("a usable row needs no note: %q reported %v", row.note.Text, row.reported)
		}
		if !row.dash.Visible() || row.toggle.Visible() {
			t.Error("an unknown toggle shows the dash")
		}
		for _, name := range []string{"ear-detection", "microphone", "case-sounds", "case-volume", "hold-modes", "hold-actions", "swipe-duration"} {
			if r.rowDisabled(name) {
				t.Errorf("%s has no report but must stay usable", name)
			}
		}
		for _, name := range []string{"tone-volume", "crown-direction", "press-speed"} {
			if r.w.settings.byName[name].note.Text != notReportedHint() || !r.rowDisabled(name) {
				t.Errorf("%s without a report must stay disabled with its one hint", name)
			}
		}
		if r.rowDisabled("mode") {
			t.Error("reported row is disabled")
		}
		if r.w.settings.byName["mode"].segment.Selected != "Noise Cancellation" {
			t.Error("mode not shown")
		}
	})
}

func TestUnreportedHoldModesBuildUpASelection(t *testing.T) {
	cfg := sim.Demo()
	cfg.Settings = []sim.Setting{{Control: 0x0d, Age: 1, Revision: 1, Value: 2}}
	r := newRig(t, cfg)
	r.main(r.w.Show)
	r.until(func() bool { return r.w.settings.byName["mode"].reported }, "read")
	r.main(func() {
		chips := r.w.settings.byName["hold-modes"].chips
		chips[1].SetChecked(true)
		if !chips[1].Checked {
			t.Error("the first choice must stay ticked")
		}
	})
	if r.dev.Writes() != 0 {
		t.Fatal("one mode must not be written")
	}
	r.main(func() {
		r.w.settings.byName["hold-modes"].chips[2].SetChecked(true)
	})
	r.until(func() bool { return r.dev.Writes() == 1 }, "write")
	if got := lastCommitValue(t, r.dev, 0x1a); got != 0b0110 {
		t.Errorf("hold-modes wrote 0b%b", got)
	}
}

func TestSilentAndUnreportedWritesSayTheyWereSent(t *testing.T) {
	cfg := sim.Demo()
	cfg.WriteMode = sim.WriteLost
	cfg.Settings = []sim.Setting{{Control: 0x0d, Age: 1, Revision: 1, Value: 2}, {Control: 0x28, Age: 1, Revision: 4, Value: 2}}
	r := newRig(t, cfg)
	r.main(r.w.Show)
	r.until(func() bool { return r.w.settings.byName["mode"].reported }, "read")
	started := time.Now()
	for _, name := range []string{"conversation-awareness", "ear-detection"} {
		r.main(func() { r.w.settings.byName[name].toggle.SetChecked(true) })
		r.until(func() bool {
			row := r.w.settings.byName[name]
			return !row.writing && !r.w.settings.reading && row.requested == "on"
		}, name+" sent")
		r.main(func() {
			row := r.w.settings.byName[name]
			if !row.toggle.Checked || row.note.Text != sentText() {
				t.Errorf("%s must keep showing the value sent, with no note: %q", name, row.note.Text)
			}
			if r.w.banner.Text != "" {
				t.Errorf("banner %q", r.w.banner.Text)
			}
		})
	}
	if time.Since(started) > 4*time.Second {
		t.Error("a silent write must not wait for a report")
	}
	if r.dev.Writes() != 2 {
		t.Errorf("writes %d", r.dev.Writes())
	}
}

func TestSilentValueGivesWayToANewerReport(t *testing.T) {
	cfg := sim.Demo()
	cfg.WriteMode = sim.WriteLost
	r := newRig(t, cfg)
	r.shown()
	r.main(func() { r.w.settings.byName["conversation-awareness"].toggle.SetChecked(false) })
	r.until(func() bool {
		row := r.w.settings.byName["conversation-awareness"]
		return !row.writing && row.requested == "off"
	}, "sent")
	r.dev.Update(func(c *sim.Config) {
		for i := range c.Settings {
			if c.Settings[i].Control == 0x28 {
				c.Settings[i].Revision += 5
			}
		}
	})
	r.until(func() bool {
		r.w.settings.read() // Does nothing while an earlier read is still out.
		row := r.w.settings.byName["conversation-awareness"]
		return row.requested == "" && row.note.Text == "" && row.toggle.Checked
	}, "newer report")
}

func TestAdaptiveStrengthNeedsConfirmedAdaptiveAndAReport(t *testing.T) {
	r := newRig(t, sim.Demo()) // mode is Noise Cancellation
	r.shown()
	r.main(func() {
		row := r.w.settings.byName["adaptive-strength"]
		if !r.rowDisabled("adaptive-strength") || row.note.Text != adaptiveOnlyText() {
			t.Errorf("disabled=%v note=%q", r.rowDisabled("adaptive-strength"), row.note.Text)
		}
		r.w.settings.byName["mode"].segment.SetSelected("Adaptive")
		if !r.rowDisabled("adaptive-strength") {
			t.Error("enabled while the mode change is in flight")
		}
	})
	r.until(func() bool {
		return !r.w.settings.byName["mode"].writing && !r.w.settings.reading && r.w.settings.byName["mode"].decoded == "adaptive"
	}, "adaptive reported")
	r.main(func() {
		if r.rowDisabled("adaptive-strength") || r.w.settings.byName["adaptive-strength"].note.Text != "" {
			t.Errorf("note %q", r.w.settings.byName["adaptive-strength"].note.Text)
		}
	})
}

// strengthDragWrites drags the capsule in the pill's Adaptive lane and reports whether that
// sent a write.
func strengthDragWrites(r *rig) bool {
	r.t.Helper()
	before := r.dev.Writes()
	r.main(func() {
		seg := r.w.settings.byName["mode"].segment
		seg.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(seg.Size().Width*0.55, 20)}})
		seg.DragEnd()
	})
	r.flush()
	return r.dev.Writes() > before
}

// A mode change the AirPods never take: the pill goes back to Adaptive, and the strength
// must be usable again, not wait for a report that has no reason to change.
func TestLostModeChangeGivesTheStrengthDragBack(t *testing.T) {
	cfg := ownerConfig()
	cfg.WriteMode = sim.WriteLost
	r := newRig(t, cfg)
	r.main(func() {
		r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"})
		r.w.win.Resize(fyne.NewSize(640, 900))
	})
	r.shown()
	mode, strength := r.w.settings.byName["mode"], r.w.settings.byName["adaptive-strength"]
	r.until(func() bool { return strength.reported && mode.segment.OnStrength != nil }, "draggable")
	r.main(func() { mode.segment.SetSelected(wordLabel("anc")) })
	r.until(func() bool { return mode.writing }, "writing")
	r.main(func() {
		if mode.segment.OnStrength != nil {
			t.Error("the strength can be dragged while the mode is changing")
		}
	})
	r.untilWithin(9*time.Second, func() bool { return !mode.writing }, "write given up")
	r.until(func() bool {
		return !r.w.settings.reading && mode.segment.Selected == wordLabel("adaptive") && mode.segment.OnStrength != nil
	}, "the drag is back after the read")
	r.main(func() {
		if mode.unconfirmed || r.rowDisabled("adaptive-strength") {
			t.Errorf("unconfirmed %v, strength disabled %v", mode.unconfirmed, r.rowDisabled("adaptive-strength"))
		}
	})
	r.dev.Update(func(c *sim.Config) { c.WriteMode = sim.WriteApply })
	if !strengthDragWrites(r) {
		t.Error("the drag wrote nothing")
	}
}

// Whatever a write left behind belongs to the link it went to.
func TestLinkChangeForgetsWriteOutcomes(t *testing.T) {
	r := newRig(t, ownerConfig())
	r.main(func() {
		r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"})
		r.w.win.Resize(fyne.NewSize(640, 900))
	})
	r.shown()
	mode, convo := r.w.settings.byName["mode"], r.w.settings.byName["conversation-awareness"]
	r.until(func() bool { return mode.segment.OnStrength != nil }, "draggable")
	r.whenIdle(func() {
		mode.unconfirmed, convo.unconfirmed, convo.problem = true, true, true
		r.w.settings.updateEnabled()
		if mode.segment.OnStrength != nil {
			t.Fatal("draggable with the mode unconfirmed")
		}
		r.w.settings.linkChanged()
		for _, row := range r.w.settings.rows {
			if row.unconfirmed || row.problem || row.revision != 0 {
				t.Errorf("%s kept unconfirmed %v, problem %v, revision %d", row.setting.Name, row.unconfirmed, row.problem, row.revision)
			}
		}
	})
	// The same over a real reconnect on the same dongle.
	r.whenIdle(func() { mode.unconfirmed = true })
	r.dev.BumpGeneration()
	r.until(func() bool { return !mode.unconfirmed && mode.segment.OnStrength != nil }, "a new link clears it")
	if !strengthDragWrites(r) {
		t.Error("the drag wrote nothing after the reconnect")
	}
}

// An unconfirmed write is quiet when it happens, and a change of language must not turn it
// into a note afterwards.
func TestLanguageChangeAddsNoNoteAboutAnUnconfirmedWrite(t *testing.T) {
	r := newRig(t, ownerConfig())
	t.Cleanup(func() { r.main(func() { i18n.SetLanguage(i18n.English) }) })
	r.shown()
	convo, mode := r.w.settings.byName["conversation-awareness"], r.w.settings.byName["mode"]
	r.whenIdle(func() {
		if !convo.reported || convo.note.Text != "" {
			t.Fatalf("reported %v, note %q", convo.reported, convo.note.Text)
		}
		convo.unconfirmed, mode.unconfirmed = true, true
		i18n.SetLanguage(i18n.Portuguese)
		for _, row := range []*settingRow{convo, mode} {
			if row.note.Text != "" || row.note.Visible() {
				t.Errorf("%s says %q after the language changed", row.setting.Name, row.note.Text)
			}
		}
		for _, row := range r.w.settings.rows {
			if row.note.Text == device.UnconfirmedNote() {
				t.Errorf("%s shows the note", row.setting.Name)
			}
		}
	})
}

func TestAdaptiveStrengthWithoutAReportStaysDisabled(t *testing.T) {
	cfg := sim.Demo()
	var kept []sim.Setting
	for _, s := range cfg.Settings {
		if s.Control == 0x0d {
			s.Value = 4
		}
		if s.Control != 0x2e {
			kept = append(kept, s)
		}
	}
	cfg.Settings = kept
	r := newRig(t, cfg)
	r.shown()
	r.main(func() {
		if !r.rowDisabled("adaptive-strength") {
			t.Error("no report to start from")
		}
	})
}

func TestSettingsFollowTheModel(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.main(func() {
		s := r.w.settings
		r.w.SetInfo(dongle.Metadata{Model: "A2564", Firmware: "6A1"}) // AirPods 3
		if s.byName["mode"].body.Visible() {
			t.Error("no modes on AirPods 3")
		}
		if !s.byName["ear-detection"].body.Visible() {
			t.Error("ear detection belongs here")
		}
		left := s.byName["hold-actions"].actions[0]
		if len(left.Options) != 1 || left.Options[0] != "Siri" {
			t.Errorf("hold options %q", left.Options)
		}
		r.w.SetInfo(dongle.Metadata{Model: "A3184", Firmware: "7A1"}) // Max
		if got := s.byName["ear-detection"].label.Text; got != "Head detection" {
			t.Errorf("label %q", got)
		}
		if got := s.byName["hold-modes"].label.Text; got != "Modes for the noise control button" {
			t.Errorf("label %q", got)
		}
		if !s.byName["crown-direction"].body.Visible() || s.byName["microphone"].body.Visible() {
			t.Error("Max has a crown and no microphone choice")
		}
		if r.onScreen(s.byName["crown-direction"].body) {
			t.Error("the crown's direction needs the experimental switch")
		}
		r.w.about.experimental.SetChecked(true)
		if !r.onScreen(s.byName["crown-direction"].body) {
			t.Error("the switch shows the crown's direction")
		}
		r.w.about.experimental.SetChecked(false)
		titles := map[string]bool{}
		for _, group := range s.cards {
			titles[group.title.Text] = true
		}
		if !titles["Buttons"] || titles["Press and Hold AirPods"] {
			t.Errorf("titles %v", titles)
		}
		choice := s.byName["mode"].segment
		for _, option := range choice.Options {
			if option == "Adaptive" {
				t.Error("Adaptive offered on original Max")
			}
		}
		r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"})
		titles = map[string]bool{}
		for _, group := range s.cards {
			titles[group.title.Text] = true
		}
		if !titles["Press and Hold AirPods"] || titles["Buttons"] {
			t.Errorf("titles %v", titles)
		}
		if s.byName["crown-direction"].body.Visible() {
			t.Error("no crown on Pro 3")
		}
	})
}

func TestModeBlockedBeforeSendWhenTheModelLacksIt(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	before := r.dev.Writes()
	r.main(func() {
		r.w.SetInfo(dongle.Metadata{Model: "A3184", Firmware: "7A1"})
		s := r.w.settings
		s.write(s.byName["mode"], "adaptive")
		if !strings.Contains(r.w.banner.Text, "not available") {
			t.Errorf("banner %q", r.w.banner.Text)
		}
	})
	r.flush()
	if r.dev.Writes() != before {
		t.Error("written")
	}
}

func TestAllowOffChosenWithoutReportOnProThree(t *testing.T) {
	cfg := sim.Demo()
	cfg.Settings = []sim.Setting{{Control: 0x0d, Age: 1, Revision: 1, Value: 2}}
	cfg.WriteMode = sim.WriteLost
	r := newRig(t, cfg)
	r.main(func() { r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"}) })
	r.main(r.w.Show)
	r.until(func() bool { return r.w.settings.byName["mode"].reported }, "read")
	r.main(func() {
		if r.rowDisabled("allow-off") {
			t.Fatal("Allow Off must be usable on Pro 3")
		}
		r.w.settings.byName["allow-off"].toggle.SetChecked(true)
	})
	r.until(func() bool { return r.w.settings.byName["allow-off"].requested != "" }, "sent")
	r.main(func() {
		if got := r.w.view().AllowOffRequested; got == nil || !*got {
			t.Errorf("requested %v", got)
		}
	})
}

// A value sent to one pair must not land under another that arrived in the meantime.
func TestWriteIsNotRememberedUnderAnotherPair(t *testing.T) {
	other := [6]byte{9, 9, 9, 9, 9, 9}
	for name, change := range map[string]func(*rig){
		"the state moved on": func(r *rig) {
			st := r.w.state
			st.Status.Peer = other
			r.w.Update(st)
		},
		"the dongle moved on": func(r *rig) {
			r.dev.Update(func(c *sim.Config) { c.Peer = sim.Hex(other[:]) })
		},
		"a reconnect of the same pair": func(r *rig) {
			r.dev.BumpGeneration()
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := sim.Demo()
			cfg.Settings = []sim.Setting{{Control: 0x0d, Age: 1, Revision: 1, Value: 2}}
			cfg.WriteMode = sim.WriteLost
			store := &choices.Store{Dir: t.TempDir()}
			r := newRigWith(t, cfg, func(o *Options) { o.Choices = store })
			r.main(func() { r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"}) })
			r.main(r.w.Show)
			r.until(func() bool { return r.w.settings.byName["mode"].reported }, "read")
			row := r.w.settings.byName["allow-off"]
			var peer [6]byte
			r.main(func() {
				peer = r.w.state.Status.Peer
				row.toggle.SetChecked(true) // submits the write
				change(r)                   // before the job can have come back
			})
			r.flush()
			r.until(func() bool { return !row.writing }, "write finished")
			for _, pair := range [][6]byte{peer, other} {
				if got := store.LoadAllowOff(pair); got != nil {
					t.Errorf("remembered %v under %x", *got, pair)
				}
			}
			r.main(func() {
				if row.requested != "" {
					t.Errorf("requested %q", row.requested)
				}
			})
		})
	}
}

func TestWriteIsRememberedForThePairItWentTo(t *testing.T) {
	cfg := sim.Demo()
	cfg.Settings = []sim.Setting{{Control: 0x0d, Age: 1, Revision: 1, Value: 2}}
	cfg.WriteMode = sim.WriteLost
	store := &choices.Store{Dir: t.TempDir()}
	r := newRigWith(t, cfg, func(o *Options) { o.Choices = store })
	r.main(func() { r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"}) })
	r.main(r.w.Show)
	r.until(func() bool { return r.w.settings.byName["mode"].reported }, "read")
	var peer [6]byte
	r.main(func() {
		peer = r.w.state.Status.Peer
		r.w.settings.byName["allow-off"].toggle.SetChecked(true)
	})
	r.until(func() bool { return r.w.settings.byName["allow-off"].requested != "" }, "sent")
	if got := store.LoadAllowOff(peer); got == nil || !*got {
		t.Errorf("saved %v", got)
	}
}

// Changing pair is a change of everything the status says, and must be judged against the old
// state, with no side effect on the window or the strength row.
func TestPeerChangeIsComparedWithTheOldState(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.whenIdle(func() {
		st := r.w.state
		st.Status.Peer = [6]byte{9, 9, 9, 9, 9, 9}
		st.Status.ANC = 3
		strengthBefore := r.w.settings.strength.Visible()
		r.w.Update(st)
		if !r.w.settings.reading {
			t.Error("a moved mode on the new pair was not read")
		}
		if r.w.settings.strength.Visible() != strengthBefore {
			t.Error("the strength row changed with the pair")
		}
	})
	r.flush()
}

func TestPeerChangeDoesNotOpenTheWindowOrReadWhileHidden(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.whenIdle(func() {
		st := r.w.state
		st.Status.Peer = [6]byte{9, 9, 9, 9, 9, 9}
		st.Status.ANC = 3
		r.w.Update(st)
		if r.w.Visible() {
			t.Error("the window opened")
		}
		if r.w.settings.reading {
			t.Error("read while hidden")
		}
		if r.w.settings.strength.Visible() {
			t.Error("the strength row flashed")
		}
	})
}

// With a dongle showing up, what the page offers follows at once.
func TestPresentChangeWithAPeerRefitsThePage(t *testing.T) {
	cfg := sim.Demo()
	cfg.OpenError = noDongleSim
	r := startRig(t, cfg, "")
	r.flush()
	demo := sim.Demo()
	r.main(func() {
		group := r.w.settings.cards[0].group
		if group.Visible() {
			t.Fatal("the card shows without a dongle")
		}
		st := device.State{Present: true, Status: dongle.Status{Capabilities: demo.Capabilities, Peer: [6]byte{1, 2, 3, 4, 5, 6}}}
		r.w.Update(st)
		if !group.Visible() {
			t.Error("the card did not come with the dongle")
		}
	})
}

func TestRefreshCooldown(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.main(func() {
		r.w.settings.cooldown = 50 * time.Millisecond
		test.Tap(r.w.settings.rereadBt)
		if !r.w.settings.rereadBt.Disabled() || !r.w.settings.rereadItem.Disabled() {
			t.Error("not cooling down")
		}
	})
	r.until(func() bool { return !r.w.settings.rereadBt.Disabled() && !r.w.settings.rereadItem.Disabled() }, "cooled down")
}

// TestReadAgainFromThePanel is the way to it with the experimental switch off, when the
// Connection section and its button are not on the page.
func TestReadAgainFromThePanel(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.until(settled(r), "settled")
	item := r.w.settings.rereadItem
	r.main(func() {
		r.w.settings.cooldown = 50 * time.Millisecond
		if r.onScreen(r.w.settings.rereadBt) {
			t.Error("the page's button needs the experimental switch")
		}
		r.w.menu.open()
		if _, ok := positionOf(r.w.menu.pop, item, fyne.Position{}); !ok || item.Disabled() {
			t.Fatalf("in the panel %v, disabled %v", ok, item.Disabled())
		}
	})
	r.dev.Update(func(c *sim.Config) {
		for i := range c.Settings {
			if c.Settings[i].Control == 0x0d {
				c.Settings[i].Value, c.Settings[i].Revision = 3, c.Settings[i].Revision+1
			}
		}
	})
	r.main(func() {
		test.Tap(item)
		if r.w.menu.pop.Visible() || !item.Disabled() {
			t.Errorf("panel open %v, cooling down %v", r.w.menu.pop.Visible(), item.Disabled())
		}
	})
	r.until(func() bool { return r.w.settings.byName["mode"].decoded == "transparency" }, "read again")
	r.until(func() bool { return !item.Disabled() }, "cooled down")
	r.main(func() {
		st := r.w.state
		st.Status.MicrophoneActive = true
		r.w.Update(st)
		if !item.Disabled() || !r.w.settings.rereadBt.Disabled() {
			t.Error("no read while the microphone is in use")
		}
	})
}

func TestHelpTextOnRows(t *testing.T) {
	r := newRig(t, sim.Demo())
	for _, name := range []string{"call-controls", "microphone"} {
		if settingHelp(name) == "" {
			t.Errorf("no help for %s", name)
		}
	}
	r.main(func() {
		if r.w.settings.byName["call-controls"].help == nil {
			t.Error("no help")
		}
	})
}

func TestRefusedReadRecoversWhenTheAirPodsBecomeReady(t *testing.T) {
	cfg := sim.Demo()
	cfg.Flags = 0x01 // connected, controls not ready yet
	cfg.SettingsNeedReady = true
	r := newRig(t, cfg)
	r.main(r.w.Show)
	r.until(func() bool { return r.w.header.detail.Text == i18n.T("page.waiting") && r.w.header.detailBox.Visible() }, "waiting shown")
	r.main(func() {
		if r.w.banner.Text != "" {
			t.Errorf("not being ready is no error: %q", r.w.banner.Text)
		}
	})
	if len(r.dev.Commits()) > 3 {
		t.Errorf("hammered the channel: %d", len(r.dev.Commits()))
	}
	r.dev.SetFlags(0x03)
	r.until(func() bool { return r.w.settings.byName["mode"].reported }, "read after ready")
	r.main(func() {
		if r.w.banner.Text != "" || r.w.header.detailBox.Visible() {
			t.Errorf("stale text left: %q %q", r.w.banner.Text, r.w.header.detail.Text)
		}
		if r.rowDisabled("mode") {
			t.Error("row still dead")
		}
	})
}

// The dongle refuses the snapshot while the AirPods controls come up, even when the status
// already says they are ready. That is part of connecting: the header waits, the banner stays
// quiet until it has gone on for a while, and the reads back off.
func TestRefusedReadWhileReadyIsRetriedWithBackoff(t *testing.T) {
	cfg := sim.Demo()
	cfg.FailTimes = map[byte]int{2: quietReadFailures}
	r := newRig(t, cfg)
	r.main(r.w.Show)
	r.until(func() bool { return r.w.settings.failures == 1 && !r.w.settings.reading }, "first failure")
	r.main(func() {
		if r.w.banner.Text != "" {
			t.Errorf("banner %q while the AirPods connect", r.w.banner.Text)
		}
		if !r.w.header.detailBox.Visible() || !strings.HasPrefix(r.w.header.detail.Text, i18n.T("page.waiting")) {
			t.Errorf("header %q", r.w.header.detail.Text)
		}
		r.w.tick() // Too early: the backoff holds.
		if r.w.settings.reading {
			t.Error("retried inside the backoff")
		}
		r.w.settings.nextRetry = time.Time{}
		r.w.tick()
	})
	r.until(func() bool { return r.w.settings.failures == 2 && !r.w.settings.reading }, "second failure")
	r.main(func() {
		if r.w.banner.Text != "" {
			t.Errorf("banner %q", r.w.banner.Text)
		}
		r.w.settings.nextRetry = time.Time{}
		r.w.tick()
	})
	r.until(func() bool { return r.w.settings.failures == 3 && !r.w.settings.reading }, "third failure")
	r.main(func() {
		if !strings.Contains(r.w.banner.Text, device.RefusedText) || strings.Contains(r.w.banner.Text, "Firmware") {
			t.Errorf("banner %q after it went on", r.w.banner.Text)
		}
		r.w.settings.nextRetry = time.Time{}
		r.w.tick()
	})
	r.until(func() bool { return r.w.settings.byName["mode"].reported }, "recovered")
	r.main(func() {
		if r.w.settings.failures != 0 || r.w.banner.Text != "" || r.w.settings.loading {
			t.Errorf("failures %d banner %q loading %v", r.w.settings.failures, r.w.banner.Text, r.w.settings.loading)
		}
	})
}

// Right after the AirPods connect the dongle lists only the settings they have sent so far.
// The rows still waiting for theirs say so, and only once the reports stop growing do they
// say the AirPods do not report them.
func TestSettingsStillArrivingAreNotCalledUnreported(t *testing.T) {
	cfg := sim.Demo()
	cfg.SettingsPerRead = 2
	r := newRig(t, cfg)
	r.main(r.w.Show)
	r.until(func() bool { return r.w.settings.everRead && !r.w.settings.reading }, "first read")
	unreported := func() []*settingRow {
		var rows []*settingRow
		for _, row := range r.w.settings.rows {
			if !row.reported && row.body.Visible() && !r.w.view().CanChoose(row.setting.Control) {
				rows = append(rows, row)
			}
		}
		return rows
	}
	r.main(func() {
		if len(r.w.settings.reported) != 2 {
			t.Fatalf("first read has %d reports", len(r.w.settings.reported))
		}
		rows := unreported()
		if len(rows) == 0 {
			t.Fatal("the first read left nothing to wait for")
		}
		for _, row := range rows {
			if row.note.Text == notReportedHint() || strings.Contains(strings.ToLower(row.note.Text), "unknown") {
				t.Errorf("%s says %q while its report may still come", row.setting.Name, row.note.Text)
			}
		}
		if !r.w.settings.arriving() {
			t.Error("not arriving")
		}
	})
	// Each tick reads again and finds more, which keeps the rows waiting.
	for range len(cfg.Settings) {
		r.main(r.w.tick)
		r.until(func() bool { return !r.w.settings.reading }, "read")
	}
	r.main(func() {
		if len(r.w.settings.reported) != len(cfg.Settings) {
			t.Errorf("%d of %d reported", len(r.w.settings.reported), len(cfg.Settings))
		}
	})
	// Then the reports stop growing, and what is still missing is not reported.
	for range eagerGrowTicks + 1 {
		r.main(r.w.tick)
		r.until(func() bool { return !r.w.settings.reading }, "read")
	}
	r.main(func() {
		if r.w.settings.arriving() {
			t.Error("still arriving after the reports stopped")
		}
		for _, row := range unreported() {
			if row.note.Text == arrivingHint() {
				t.Errorf("%s still says it is arriving", row.setting.Name)
			}
		}
	})
}

func TestUnreportedRowsStayWritableWhenTheReadFails(t *testing.T) {
	cfg := sim.Demo()
	cfg.Fail = map[byte]byte{2: 7}
	r := newRig(t, cfg)
	r.main(r.w.Show)
	r.until(func() bool { return r.w.settings.failures >= 1 }, "failed read")
	r.main(func() {
		if r.rowDisabled("ear-detection") || r.rowDisabled("hold-actions") {
			t.Error("explicit rows must stay usable")
		}
	})
}

func TestChangeOfConnectionDuringAReadIsNotMissed(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.main(func() {
		r.w.settings.reading = true // A read is out.
		r.w.settings.read()
		if !r.w.settings.again {
			t.Error("the request was dropped")
		}
		r.w.settings.readDone(nil, errors.New("gone"))
		if r.w.settings.again {
			t.Error("the follow-up did not run")
		}
	})
}

func TestProgrammaticUpdatesDoNotWrite(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.main(func() { r.w.settings.read() })
	r.until(func() bool { return !r.w.settings.reading }, "second read")
	if r.dev.Writes() != 0 {
		t.Errorf("showing values caused %d writes", r.dev.Writes())
	}
}

func lastCommitValue(t *testing.T, dev *sim.Device, control byte) uint32 {
	t.Helper()
	for _, commit := range reverse(dev.Commits()) {
		if len(commit) == 6 && commit[0] == 1 && commit[1] == control {
			return uint32(commit[2]) | uint32(commit[3])<<8 | uint32(commit[4])<<16 | uint32(commit[5])<<24
		}
	}
	t.Fatalf("no write to 0x%02x", control)
	return 0
}

func reverse(in [][]byte) [][]byte {
	out := make([][]byte, len(in))
	for i, item := range in {
		out[len(in)-1-i] = item
	}
	return out
}

func TestWritesUseLibraryEncoding(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	settle := func() {
		t.Helper()
		r.until(func() bool {
			for _, row := range r.w.settings.rows {
				if row.writing {
					return false
				}
			}
			return !r.w.settings.reading
		}, "write finished")
	}

	r.main(func() { r.w.settings.byName["mode"].segment.SetSelected("Transparency") })
	settle()
	if got := lastCommitValue(t, r.dev, 0x0d); got != 3 {
		t.Errorf("mode wrote %d", got)
	}
	// ear-detection has inverted raw values: off is 1.
	r.main(func() { r.w.settings.byName["ear-detection"].toggle.SetChecked(false) })
	settle()
	if got := lastCommitValue(t, r.dev, 0x0a); got != 1 {
		t.Errorf("ear-detection wrote %d", got)
	}
	// tone-volume carries a marker in its encoding; the widget must go through Encode.
	r.main(func() {
		r.w.settings.byName["tone-volume"].slider.OnChangeEnded(30)
	})
	settle()
	if got := lastCommitValue(t, r.dev, 0x1f); got != 0x5000|30 {
		t.Errorf("tone-volume wrote 0x%x", got)
	}
	r.main(func() {
		actions := r.w.settings.byName["hold-actions"].actions
		actions[0].SetSelected("Noise control") // same value, no change expected
		actions[1].SetSelected("Noise control")
	})
	settle()
	if got := lastCommitValue(t, r.dev, 0x16); got != 0x0505 { // 5 is noise control
		t.Errorf("hold-actions wrote 0x%x", got)
	}
	r.main(func() {
		r.w.settings.byName["hold-actions"].actions[0].SetSelected("Siri")
	})
	settle()
	if got := lastCommitValue(t, r.dev, 0x16); got != 0x0105 { // left Siri (1) high, right noise control (5) low
		t.Errorf("left action must be the high byte, wrote 0x%x", got)
	}
}

func TestHoldModesNeedTwo(t *testing.T) {
	r := newRig(t, sim.Demo()) // anc and transparency are on
	r.shown()
	before := r.dev.Writes()
	r.main(func() {
		r.w.settings.byName["hold-modes"].chips[1].SetChecked(false)
	})
	r.main(func() {
		if !r.w.settings.byName["hold-modes"].chips[1].Checked {
			t.Error("the rejected change was not undone")
		}
		if !strings.Contains(r.w.banner.Text, "at least two") {
			t.Errorf("banner: %q", r.w.banner.Text)
		}
	})
	if r.dev.Writes() != before {
		t.Error("an invalid selection was written")
	}
	r.main(func() {
		r.w.settings.byName["hold-modes"].chips[3].SetChecked(true)
	})
	r.until(func() bool { return !r.w.settings.byName["hold-modes"].writing && !r.w.settings.reading }, "write")
	if got := lastCommitValue(t, r.dev, 0x1a); got != 0b1110 {
		t.Errorf("hold-modes wrote 0b%b", got)
	}
}

// A reported toggle whose write the AirPods never echo, as Personalized Volume does on
// some models: the switch keeps the choice and the row says nothing.
func TestUnechoedToggleKeepsTheChoiceWithoutANote(t *testing.T) {
	r := newRig(t, writes(sim.Demo(), func(c *sim.Config) { c.WriteMode = sim.WriteLost }))
	r.shown()
	row := r.w.settings.byName["personalized-volume"]
	var before bool
	r.main(func() {
		before = row.toggle.Checked
		row.toggle.SetChecked(!before)
	})
	r.until(func() bool { return row.writing }, "writing")
	r.untilWithin(8*time.Second, func() bool { return !row.writing }, "write finished")
	r.until(func() bool { return !row.writing && !r.w.settings.reading }, "re-read")
	r.main(func() {
		if row.toggle.Checked == before || row.requested == "" {
			t.Errorf("the choice was dropped: checked %v requested %q", row.toggle.Checked, row.requested)
		}
		if row.note.Visible() && row.note.Text != "" {
			t.Errorf("note %q", row.note.Text)
		}
		if r.w.banner.Text != "" {
			t.Errorf("banner %q", r.w.banner.Text)
		}
	})
	if r.dev.Writes() != 1 {
		t.Errorf("writes: %d", r.dev.Writes())
	}
}

func TestUnconfirmedWriteIsQuietAndNotRetried(t *testing.T) {
	// The write is accepted but never shows up, and the confirm loop runs its whole
	// 5 s course, which is the realistic path.
	r := newRig(t, writes(sim.Demo(), func(c *sim.Config) { c.WriteMode = sim.WriteLost }))
	r.shown()
	r.main(func() { r.w.settings.byName["mode"].segment.SetSelected("Transparency") })
	r.until(func() bool { return r.w.settings.byName["mode"].writing }, "writing")
	r.untilWithin(8*time.Second, func() bool { return !r.w.settings.byName["mode"].writing }, "write finished")
	r.until(func() bool { return !r.w.settings.reading }, "re-read")
	r.main(func() {
		row := r.w.settings.byName["mode"]
		if row.note.Visible() && row.note.Text != "" {
			t.Errorf("an unconfirmed write is not the user's problem to read about: %q", row.note.Text)
		}
		if row.segment.Selected != "Noise Cancellation" {
			t.Errorf("the widget must show what the dongle reports: %q", row.segment.Selected)
		}
	})
	if r.dev.Writes() != 1 {
		t.Errorf("writes: %d", r.dev.Writes())
	}
}

func TestBusyShowsAnotherAppAndIsNotRetried(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.dev.Update(func(c *sim.Config) { c.Fail = map[byte]byte{1: 2} })
	r.main(func() { r.w.settings.byName["mode"].segment.SetSelected("Transparency") })
	r.until(func() bool { return r.w.settings.byName["mode"].note.Text == device.BusyText }, "busy text")
	r.until(func() bool { return !r.w.settings.byName["mode"].writing }, "finished")
	r.flush()
	r.main(func() {
		row := r.w.settings.byName["mode"]
		if row.note.Text != device.BusyText {
			t.Errorf("note: %q", row.note.Text)
		}
		if row.segment.Selected != "Noise Cancellation" {
			t.Error("the widget did not go back")
		}
	})
	count := 0
	for _, commit := range r.dev.Commits() {
		if commit[0] == 1 {
			count++
		}
	}
	if count != 1 {
		t.Errorf("write attempts: %d", count)
	}
}

func TestBusyChannelOnReadShowsAnotherApp(t *testing.T) {
	cfg := sim.Demo()
	cfg.InitialPhase = 1
	r := newRig(t, cfg)
	r.main(r.w.Show)
	r.until(func() bool { return strings.Contains(r.w.banner.Text, device.BusyText) }, "busy text")
	if len(r.dev.Commits()) != 0 {
		t.Error("a request was sent on a busy channel")
	}
}

func TestHiddenWindowSendsNoRPC(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.main(func() {
		r.w.Update(device.State{Present: true, Status: dongle.Status{Capabilities: 0x7f, Connected: true, ControlsReady: true, Generation: 99}})
		r.w.tick()
	})
	r.flush()
	if _, rpc, sets := r.dev.Counts(); rpc != 0 || sets != 0 {
		t.Errorf("rpc %d sets %d while hidden", rpc, sets)
	}
}

func TestReconnectIsRereadWhileVisible(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	// A reconnect in the middle of a request leaves that request staged in the simulator,
	// which then reads as another app holding the channel; that is not what is tested here.
	r.whenIdle(func() {
		r.dev.Update(func(c *sim.Config) { c.Settings[0].Value = 3 })
		r.dev.BumpGeneration()
	})
	r.until(func() bool {
		return r.w.settings.byName["mode"].segment.Selected == "Transparency"
	}, "new generation re-read")
}

func TestDisconnectedDisablesRows(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.dev.SetFlags(0)
	r.until(func() bool { return !r.w.state.Connected() }, "away")
	r.main(func() {
		if !r.rowDisabled("mode") {
			t.Error("row still enabled")
		}
		if r.w.device.reconnect.Disabled() {
			t.Error("reconnect must stay available")
		}
		if r.w.device.peer.Text != "Not connected" {
			t.Errorf("peer %q", r.w.device.peer.Text)
		}
	})
}

func TestChoicesAreRestoredPerPair(t *testing.T) {
	store := &choices.Store{Dir: t.TempDir()}
	cfg := sim.Demo()
	cfg.Settings = []sim.Setting{{Control: 0x0d, Age: 1, Revision: 1, Value: 2}}
	peer := [6]byte(cfg.Peer)
	if err := store.Save(peer, 0x0a, 1); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAllowOff(peer, true); err != nil {
		t.Fatal(err)
	}
	r := newRig(t, cfg)
	r.main(func() {
		r.w.opts.Choices = store
		r.w.settings.loadChoices()
		r.w.Show()
	})
	r.until(func() bool { return r.w.settings.byName["mode"].reported }, "read")
	r.main(func() {
		row := r.w.settings.byName["ear-detection"]
		if row.requested != "off" || row.note.Text != sentText() || row.toggle.Checked {
			t.Errorf("restored: %q %q", row.requested, row.note.Text)
		}
		if got := r.w.view().AllowOffRequested; got == nil || !*got {
			t.Errorf("allow off %v", got)
		}
	})
	r.main(func() { r.w.settings.byName["case-sounds"].toggle.SetChecked(true) })
	r.until(func() bool { return !r.w.settings.byName["case-sounds"].writing && r.dev.Writes() == 1 }, "write")
	r.until(func() bool { return store.Load(peer)[0x31] == 1 }, "saved")
}

// The dongle's reports can arrive after the first read, which then finds nothing.
func TestSettingsFillInAfterAnEmptyFirstRead(t *testing.T) {
	cfg := sim.Demo()
	cfg.Settings = nil
	cfg.ANC = 0 // the status has no mode either until the AirPods speak
	r := newRig(t, cfg)
	r.main(func() { r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"}) })
	r.main(r.w.Show)
	r.until(func() bool { return r.w.settings.everRead && !r.w.settings.reading }, "empty read")
	r.main(func() {
		if !r.rowDisabled("mode") {
			t.Error("the pill cannot work before a mode is reported")
		}
		if got := r.w.header.detail.Text; got != i18n.T("page.waiting") {
			t.Errorf("the header must say what an empty page waits for: %q", got)
		}
		if got := r.w.settings.byName["crown-direction"].note.Text; got != "" {
			t.Errorf("nothing reported at all is not a row's fault: %q", got)
		}
	})
	r.dev.Update(func(c *sim.Config) {
		c.Settings = []sim.Setting{{Control: 0x0d, Age: 1, Revision: 1, Value: 2}}
	})
	// One tick is enough on a new link; later it takes two.
	r.main(r.w.tick)
	r.until(func() bool { return r.w.settings.byName["mode"].reported }, "second read")
	r.main(func() {
		if r.rowDisabled("mode") {
			t.Error("the pill stayed locked after the report came in")
		}
		if got := r.w.header.detail.Text; got != "" {
			t.Errorf("header still waiting: %q", got)
		}
	})
	if r.dev.Writes() != 0 {
		t.Errorf("reading caused %d writes", r.dev.Writes())
	}
}

func TestRepeatedReadsKeepChoicesAndWriteProblems(t *testing.T) {
	cfg := sim.Demo()
	cfg.Settings = []sim.Setting{{Control: 0x0d, Age: 1, Revision: 1, Value: 2}, {Control: 0x28, Age: 1, Revision: 1, Value: 1}}
	r := newRig(t, cfg)
	r.main(r.w.Show)
	r.until(func() bool { return r.w.settings.byName["mode"].reported }, "first read")
	hold, convo := r.w.settings.byName["hold-actions"], r.w.settings.byName["conversation-awareness"]
	r.main(func() {
		hold.actions[0].SetSelected(hold.actions[0].Options[0])
		r.w.settings.finish(convo, "", errors.New("refused"))
		r.w.tick()
		r.w.tick()
	})
	r.until(func() bool { return !r.w.settings.reading && r.w.ticks >= 2 }, "second read")
	r.main(func() {
		if hold.actions[0].Selected == "" {
			t.Error("a read cleared a half-made hold action choice")
		}
		if convo.note.Text == "" {
			t.Error("a read cleared the note about a failed write")
		}
	})
}

func TestEarbudArtStaysBrightWithAnOldReading(t *testing.T) {
	cfg := sim.Demo()
	cfg.Ages = [3]uint32{45_000, 45_000, 200_000}
	r := newRig(t, cfg)
	r.until(func() bool { return r.w.state.Present }, "status")
	r.main(func() {
		columns := r.w.header.columns
		if !r.w.header.shown[0].dim || columns[0].artDim || columns[1].artDim {
			t.Errorf("earbuds: numbers dim %v, art dim %v %v", r.w.header.shown[0].dim, columns[0].artDim, columns[1].artDim)
		}
		if !columns[2].artDim {
			t.Error("an old case reading must dim the case")
		}
	})
}

func TestSettingsAreReadEveryTickOnlyWhileALinkIsNew(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.until(func() bool { return !r.w.settings.reading }, "read")
	r.main(func() {
		s := r.w.settings
		s.linkChanged()
		for i := 0; i < eagerLinkTicks; i++ {
			if !s.due(1) {
				t.Fatalf("tick %d of a new link skipped", i+1)
			}
		}
		if s.due(1) {
			t.Error("a settled link is still read on every tick")
		}
		if !s.due(settingsPollTicks) {
			t.Error("the regular read is gone")
		}
		// Reports that grew are followed closely for a moment.
		s.applyRecords(map[byte]dongle.Record{0x0d: {Value: 2, Revision: 9}})
		if !s.due(1) {
			t.Error("growing reports are not followed")
		}
		// A link that reports nothing is read every tick for longer, not for ever.
		s.linkChanged()
		s.applyRecords(nil)
		for i := 0; i < eagerEmptyTicks; i++ {
			if !s.due(1) {
				t.Fatalf("tick %d of an empty link skipped", i+1)
			}
		}
		if s.due(1) {
			t.Error("an empty link is read on every tick for ever")
		}
	})
}

func TestListeningModeComesFromTheStatusUntilItIsReported(t *testing.T) {
	cfg := sim.Demo()
	cfg.Settings = nil
	cfg.ANC = 3
	cfg.WriteMode = sim.WriteLost
	r := newRig(t, cfg)
	r.main(func() { r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"}) })
	r.main(r.w.Show)
	r.until(func() bool { return r.w.settings.everRead && !r.w.settings.reading }, "empty read")
	mode := r.w.settings.byName["mode"]
	r.main(func() {
		if mode.reported || r.rowDisabled("mode") || mode.segment.Selected != "Transparency" {
			t.Fatalf("reported=%v disabled=%v selected=%q", mode.reported, r.rowDisabled("mode"), mode.segment.Selected)
		}
		for _, option := range mode.segment.Options {
			if option == "Off" {
				t.Error("Off offered with no word that it is allowed")
			}
		}
		mode.segment.SetSelected("Noise Cancellation")
	})
	r.main(func() {
		if mode.segment.Selected != "Noise Cancellation" {
			t.Errorf("selected %q while the tap is on its way", mode.segment.Selected)
		}
	})
	// The write is lost: after it and one read the status still says Transparency.
	r.until(func() bool {
		return !mode.writing && !r.w.settings.reading && mode.requested == "" && mode.segment.Selected == "Transparency"
	}, "the ignored tap left the pill")
	if r.dev.Writes() != 1 {
		t.Fatalf("%d writes", r.dev.Writes())
	}
	r.main(func() {
		if r.rowDisabled("mode") || (mode.note.Visible() && mode.note.Text != "") {
			t.Errorf("disabled %v note %q", r.rowDisabled("mode"), mode.note.Text)
		}
	})
	// The stem, or a write taking effect, shows in the status first.
	r.dev.Update(func(c *sim.Config) { c.ANC = 4 })
	r.until(func() bool { return mode.segment.Selected == "Adaptive" && mode.requested == "" }, "status followed")
	r.main(func() {
		if !r.rowDisabled("adaptive-strength") {
			t.Error("the strength has no report to start from")
		}
	})
	// Off as the current mode proves that Off exists.
	r.dev.Update(func(c *sim.Config) { c.ANC = 1 })
	r.until(func() bool { return mode.segment.Selected == "Off" && mode.segment.Options[0] == "Off" }, "off shown")
	// A report takes over, even when it disagrees with the status for a moment.
	r.dev.Update(func(c *sim.Config) {
		c.Settings = []sim.Setting{{Control: 0x0d, Age: 1, Revision: 1, Value: 2}}
	})
	r.main(r.w.tick)
	r.until(func() bool { return mode.reported && mode.segment.Selected == "Noise Cancellation" }, "report shown")
	r.main(func() {
		if mode.status != "" || r.rowDisabled("mode") {
			t.Errorf("status %q disabled %v", mode.status, r.rowDisabled("mode"))
		}
	})
	if r.dev.Writes() != 1 {
		t.Errorf("following the status wrote: %d writes", r.dev.Writes())
	}
}

// With no report for the mode the status is all there is to go by.
func TestModeTapTheAirPodsTakeStaysWhenOnlyTheStatusTellsTheMode(t *testing.T) {
	cfg := ownerConfig()
	cfg.Settings = cfg.Settings[1:] // no mode report
	r := newRig(t, cfg)
	r.main(func() { r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"}) })
	r.main(r.w.Show)
	r.until(func() bool { return r.w.settings.everRead && !r.w.settings.reading }, "first read")
	mode := r.w.settings.byName["mode"]
	r.main(func() {
		if mode.reported || mode.segment.Selected != wordLabel("adaptive") {
			t.Fatalf("reported %v, selected %q", mode.reported, mode.segment.Selected)
		}
		mode.segment.SetSelected(wordLabel("anc"))
	})
	// The AirPods take it: the status moves, whenever that is against the write and the read.
	r.dev.Update(func(c *sim.Config) { c.ANC = 2 })
	r.until(func() bool { return !mode.writing && !r.w.settings.reading && r.w.state.Status.ANC == 2 }, "written and read")
	r.flush()
	r.main(func() {
		if mode.segment.Selected != wordLabel("anc") {
			t.Errorf("the pill shows %q", mode.segment.Selected)
		}
	})
}

// The same tap when the AirPods ignore it must not stay on the pill for ever.
func TestIgnoredModeTapLeavesThePillWhenOnlyTheStatusTellsTheMode(t *testing.T) {
	cfg := ownerConfig()
	cfg.Settings = cfg.Settings[1:] // no mode report
	cfg.WriteMode = sim.WriteLost
	r := newRig(t, cfg)
	r.main(func() { r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"}) })
	r.main(r.w.Show)
	r.until(func() bool { return r.w.settings.everRead && !r.w.settings.reading }, "first read")
	mode := r.w.settings.byName["mode"]
	r.main(func() {
		if mode.reported || mode.status != "adaptive" || mode.segment.Selected != wordLabel("adaptive") {
			t.Fatalf("reported %v, status %q, selected %q", mode.reported, mode.status, mode.segment.Selected)
		}
		mode.segment.SetSelected(wordLabel("anc"))
		if mode.segment.Selected != wordLabel("anc") {
			t.Errorf("selected %q right after the tap", mode.segment.Selected)
		}
	})
	r.until(func() bool { return !mode.writing && !r.w.settings.reading && mode.requested == "" }, "the tap dropped after a read")
	r.main(func() {
		if r.w.state.Status.ANC != 4 || mode.segment.Selected != wordLabel("adaptive") {
			t.Errorf("status mode %d, the pill says %q", r.w.state.Status.ANC, mode.segment.Selected)
		}
		if r.rowDisabled("mode") {
			t.Error("the pill is disabled")
		}
	})
	if r.dev.Writes() != 1 {
		t.Errorf("%d writes", r.dev.Writes())
	}
}

func TestDisconnectedShowsNoValuesAndReconnectBringsThemBack(t *testing.T) {
	cfg := ownerConfig()
	r := newRig(t, cfg)
	r.main(func() { r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"}) })
	r.shown()
	r.until(func() bool { return !r.w.settings.reading }, "read")
	s := r.w.settings
	mode, toggle, slider, choice, chosen := s.byName["mode"], s.byName["conversation-awareness"], s.byName["tone-volume"], s.byName["press-speed"], s.byName["ear-detection"]
	var cards []bool
	r.main(func() {
		// A remembered choice that the AirPods never report.
		chosen.requested = "on"
		s.restore(chosen)
		if mode.segment.Selected != "Adaptive" || !toggle.toggle.Visible() || !toggle.toggle.Checked || slider.dimmed || choice.choice.Selected == "" || !chosen.toggle.Checked {
			t.Fatal("values not shown while connected")
		}
		for _, group := range s.cards {
			cards = append(cards, group.group.Visible())
		}
	})
	r.dev.SetFlags(0)
	r.until(func() bool { return !r.w.state.Connected() }, "away")
	r.main(func() {
		if mode.segment.Selected != "" {
			t.Errorf("pill still selects %q", mode.segment.Selected)
		}
		if toggle.toggle.Visible() || !toggle.dash.Visible() || chosen.toggle.Visible() {
			t.Error("a switch still shows a value")
		}
		if !slider.dimmed || slider.number.Text != unknownValue {
			t.Errorf("slider still shows %q", slider.number.Text)
		}
		if choice.choice.Selected != "" {
			t.Errorf("select still shows %q", choice.choice.Selected)
		}
		for _, name := range []string{"mode", "conversation-awareness", "tone-volume", "press-speed", "ear-detection"} {
			if !r.rowDisabled(name) {
				t.Errorf("%s usable without AirPods", name)
			}
		}
		for i, group := range s.cards {
			if group.group.Visible() != cards[i] {
				t.Errorf("card %s came or went", group.base)
			}
		}
		if chosen.requested != "on" {
			t.Error("the remembered choice was dropped")
		}
	})
	r.dev.SetFlags(cfg.Flags)
	r.until(func() bool {
		return r.w.state.Ready() && s.everRead && !s.reading && mode.segment.Selected == "Adaptive"
	}, "back")
	r.main(func() {
		if !toggle.toggle.Visible() || !toggle.toggle.Checked || slider.dimmed || choice.choice.Selected == "" {
			t.Error("values did not come back")
		}
		if !chosen.toggle.Visible() || !chosen.toggle.Checked {
			t.Error("the remembered choice did not come back")
		}
		if r.rowDisabled("mode") || r.rowDisabled("conversation-awareness") {
			t.Error("rows stayed disabled")
		}
	})
}

// writesOf lists the setting writes the simulator took, as control and value.
func writesOf(dev *sim.Device) [][2]uint32 {
	var out [][2]uint32
	for _, commit := range dev.Commits() {
		if len(commit) == 6 && commit[0] == 1 {
			out = append(out, [2]uint32{uint32(commit[1]), binary.LittleEndian.Uint32(commit[2:])})
		}
	}
	return out
}

// The dongle refuses Off on AirPods Pro 3 until allow-off on was sent in this connection.
// With the user's Allow Off on, choosing Off sends it first, once per connection.
func TestOffIsPrecededByAllowOffOnceAConnection(t *testing.T) {
	cfg := sim.Demo()
	cfg.AllowOffGate = true
	cfg.Settings = []sim.Setting{{Control: 0x0d, Age: 1, Revision: 1, Value: 2}} // Pro 3 never reports Allow Off.
	store := &choices.Store{Dir: t.TempDir()}
	if err := store.SaveAllowOff([6]byte(cfg.Peer), true); err != nil {
		t.Fatal(err)
	}
	r := newRigWith(t, cfg, func(o *Options) { o.Choices = store })
	r.main(func() { r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"}) })
	r.shown()
	// The gate is real: Off on its own is refused.
	refused := make(chan error, 1)
	r.owner.Submit(func(c *dongle.Client) error {
		settings, err := c.SettingsClient()
		if err == nil {
			_, err = settings.Send("mode", 1)
		}
		return err
	}, func(err error) { refused <- err })
	var unsupported *dongle.Unsupported
	if err := <-refused; !errors.As(err, &unsupported) {
		t.Fatalf("Off without allow-off: %v", err)
	}
	mode := r.w.settings.byName["mode"]
	var generation uint32
	r.main(func() {
		generation = r.w.state.Status.Generation
		if !r.w.view().ValueAllowed(model.ControlMode, 1) {
			t.Fatal("Off is not offered with Allow Off on")
		}
		r.w.settings.write(mode, "off")
	})
	r.until(func() bool { return !mode.writing && r.w.state.Status.ANC == 1 }, "off")
	r.main(func() {
		if mode.problem || !r.w.AllowOffOnSent(generation) {
			t.Errorf("problem %v note %q, allow-off recorded %v", mode.problem, mode.note.Text, r.w.AllowOffOnSent(generation))
		}
		r.w.settings.write(mode, "anc")
	})
	r.until(func() bool { return !mode.writing && r.w.state.Status.ANC == 2 }, "anc")
	r.main(func() { r.w.settings.write(mode, "off") })
	r.until(func() bool { return !mode.writing && r.w.state.Status.ANC == 1 }, "off again")
	want := [][2]uint32{{0x0d, 1}, {uint32(model.ControlAllowOff), 1}, {0x0d, 1}, {0x0d, 2}, {0x0d, 1}}
	if got := writesOf(r.dev); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("writes %v, want %v", got, want)
	}
}

// withNewSettings is the demo pair also reporting EN 352 protection (off) and Gain Swipe (on),
// which dongle firmware 0.1.64 and newer list in the extended snapshot only.
func withNewSettings(cfg sim.Config) sim.Config {
	cfg.Settings = append(cfg.Settings, sim.Setting{Control: 0x37, Age: 900, Revision: 3, Value: 2},
		sim.Setting{Control: 0x2f, Age: 900, Revision: 3, Value: 1})
	return cfg
}

func TestEN352AndGainSwipeFollowTheModelAndTheExtendedSnapshot(t *testing.T) {
	r := newRig(t, withNewSettings(sim.Demo()))
	r.main(func() { r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"}) })
	r.shown()
	s := r.w.settings
	r.until(func() bool { return s.byName["gain-swipe"].reported && s.byName["en352-protection"].reported }, "reported")
	r.main(func() {
		en352, swipe := s.byName["en352-protection"], s.byName["gain-swipe"]
		if !r.onScreen(en352.body) || !r.onScreen(swipe.body) {
			t.Fatal("Pro 3 has both")
		}
		if en352.toggle.Checked || !swipe.toggle.Checked || r.rowDisabled("gain-swipe") {
			t.Errorf("en352 %v, gain swipe %v", en352.toggle.Checked, swipe.toggle.Checked)
		}
		if en352.label.Text != "EN 352 protection" || swipe.label.Text != "Gain swipe" {
			t.Errorf("labels %q %q", en352.label.Text, swipe.label.Text)
		}
		r.w.SetInfo(dongle.Metadata{Model: "A2931", Firmware: "6A1"}) // Pro 2
		if en352.body.Visible() || !swipe.body.Visible() {
			t.Error("Pro 2 has Gain Swipe and no EN 352 protection")
		}
		r.w.SetInfo(dongle.Metadata{Model: "A2564", Firmware: "6A1"}) // AirPods 3
		if en352.body.Visible() || swipe.body.Visible() {
			t.Error("AirPods 3 have neither")
		}
		r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"})
		swipe.toggle.SetChecked(false)
	})
	r.until(func() bool { return !s.byName["gain-swipe"].writing && r.dev.Writes() == 1 }, "the write")
	if got := lastCommitValue(t, r.dev, 0x2f); got != 2 {
		t.Errorf("wrote %d", got)
	}
}

// Firmware before 0.1.64 calls the extended snapshot malformed: the page reads the original
// one and the new rows stay unreported, never wrong.
func TestNewSettingsOnFirmwareWithTheLegacySnapshotOnly(t *testing.T) {
	cfg := withNewSettings(sim.Demo())
	cfg.LegacySnapshotOnly = true
	r := newRig(t, cfg)
	r.main(func() { r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"}) })
	r.shown()
	r.until(func() bool { return !r.w.settings.reading && r.w.settings.byName["mode"].reported }, "read")
	r.settle()
	r.main(func() {
		for _, name := range []string{"en352-protection", "gain-swipe"} {
			if row := r.w.settings.byName[name]; row.reported || row.known {
				t.Errorf("%s reported on legacy firmware", name)
			}
		}
		if !r.w.settings.byName["ear-detection"].reported {
			t.Error("the legacy snapshot was not read")
		}
	})
}
