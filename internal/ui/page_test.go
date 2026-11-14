package ui

import (
	"encoding/binary"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/disconnect"
	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/i18n"
	"github.com/Dardrai/airpods-gui/internal/tray"
)

// noDongleSim as the sim's open error makes the rig find no dongle at all.
const noDongleSim = "no dongle"

// ownerConfig mirrors the state seen on real hardware: connected, Adaptive, AAC-ELD,
// both buds full, no case reading, and only some settings reported.
func ownerConfig() sim.Config {
	cfg := sim.Demo()
	cfg.ANC = 4
	cfg.Codec = 3
	cfg.Battery = [3]byte{100, 100, 255}
	cfg.Present = 3
	cfg.Settings = nil
	for _, s := range []struct {
		control byte
		value   uint32
	}{
		{0x0d, 4}, {0x2e, 50}, {0x28, 1}, {0x26, 2}, {0x1f, 0x5032}, {0x17, 0}, {0x18, 0}, {0x23, 0}, {0x24, 0x0300}, {0x25, 1}, {0x3b, 1}, {0x1b, 1}, {0x35, 1},
	} {
		cfg.Settings = append(cfg.Settings, sim.Setting{Control: s.control, Age: 900, Revision: 3, Value: s.value})
	}
	audio := append([]byte(nil), cfg.Replies[3]...)
	audio[1], audio[2] = 3, 3
	binary.LittleEndian.PutUint32(audio[4:], 256000)
	binary.LittleEndian.PutUint32(audio[8:], 256000)
	cfg.Replies[3], cfg.Replies[4], cfg.Replies[9] = audio, audio, audio
	return cfg
}

// waitingConfig is what the dongle reports for the first half minute after the AirPods
// connect: connected and ready, with no mode, no presence, no battery and no setting yet.
func waitingConfig() sim.Config {
	cfg := ownerConfig()
	cfg.Flags = 0x03
	cfg.ANC = 0
	cfg.Capabilities = 0x01fd
	cfg.Battery = [3]byte{255, 255, 255}
	cfg.Ages = [3]uint32{0xffffffff, 0xffffffff, 0xffffffff}
	cfg.Present = 0
	cfg.Settings = nil
	return cfg
}

// positionOf finds an object's position in the window, through containers and widgets.
func positionOf(root, target fyne.CanvasObject, at fyne.Position) (fyne.Position, bool) {
	here := at.Add(root.Position())
	if root == target {
		return here, true
	}
	var children []fyne.CanvasObject
	switch item := root.(type) {
	case *fyne.Container:
		children = item.Objects
	case fyne.Widget:
		children = test.WidgetRenderer(item).Objects()
	}
	for _, child := range children {
		if pos, ok := positionOf(child, target, here); ok {
			return pos, true
		}
	}
	return fyne.Position{}, false
}

// click taps the middle of an object through the canvas, as a pointer would: it hits only
// what is really on screen at that spot.
func (r *rig) click(target fyne.CanvasObject) {
	r.t.Helper()
	pos, ok := positionOf(r.w.Canvas().Content(), target, fyne.Position{})
	if !ok {
		r.t.Fatalf("%T is not on the page", target)
	}
	size := target.Size()
	test.TapCanvas(r.w.Canvas(), pos.Add(fyne.NewPos(size.Width/2, size.Height/2)))
}

// tall shows the whole page without scrolling, so that every row can be clicked.
func (r *rig) tall() {
	r.main(func() {
		r.w.relayout()
		r.w.win.Resize(fyne.NewSize(defaultWidth, r.w.page.MinSize().Height+40))
		r.w.relayout()
	})
}

func settled(r *rig) func() bool {
	return func() bool {
		for _, row := range r.w.settings.rows {
			if row.writing {
				return false
			}
		}
		return !r.w.settings.reading
	}
}

func TestDefaultWindowIsTheSinglePage(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.main(func() {
		if got := r.w.win.Canvas().Size(); got.Width != defaultWidth || got.Height != defaultHeight {
			t.Errorf("size %v", got)
		}
		// A wider minimum would push every window past the default.
		if got := r.w.win.Content().MinSize().Width; got > defaultWidth {
			t.Errorf("minimum width %v", got)
		}
		r.w.win.Resize(fyne.NewSize(1000, 700))
		r.w.relayout()
		if got := r.w.page.Size().Width; got != pageMaxWidth {
			t.Errorf("page width %v at a wide window", got)
		}
		pos, ok := positionOf(r.w.Canvas().Content(), r.w.page, fyne.Position{})
		if got := pos.X; !ok || got != (1000-pageMaxWidth)/2 {
			t.Errorf("page not centred: x %v", got)
		}
	})
}

func TestHeaderNamesTheModelAndTheState(t *testing.T) {
	r := newRig(t, ownerConfig())
	r.main(func() {
		h := r.w.header
		if h.name.Text != fallbackName || h.status.value != "Connected" {
			t.Errorf("%q %q", h.name.Text, h.status.value)
		}
		r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"})
		if h.name.Text != "AirPods Pro 3" {
			t.Errorf("name %q", h.name.Text)
		}
		r.w.SetInfo(dongle.Metadata{Model: "A9999"})
		if h.name.Text != fallbackName {
			t.Errorf("unknown model %q", h.name.Text)
		}
		if h.shown[0].percent != "100%" || h.shown[1].percent != "100%" {
			t.Errorf("buds %+v", h.shown)
		}
		if !h.shown[2].dim || h.shown[2].percent != dashText || h.shown[2].level != -1 {
			t.Errorf("unknown case %+v", h.shown[2])
		}
	})
	r.dev.SetFlags(0)
	r.until(func() bool { return r.w.header.status.value == "Not connected" }, "not connected")
	r.main(func() {
		if r.w.header.detail.Text != i18n.T("page.waiting") {
			t.Errorf("detail %q", r.w.header.detail.Text)
		}
	})
}

// Connected with nothing reported yet is a wait, and must not look like missing earbuds.
func TestHeaderKeepsTheEarbudsBrightWhileNothingIsReportedYet(t *testing.T) {
	r := newRig(t, waitingConfig())
	r.main(func() {
		h := r.w.header
		if h.status.value != "Connected" {
			t.Fatalf("status %q", h.status.value)
		}
		for i, name := range []string{"left", "right"} {
			if h.columns[i].artDim || h.columns[i].percent.Text != dashText || h.columns[i].level != -1 {
				t.Errorf("%s: art dim %v, %q, gauge %d", name, h.columns[i].artDim, h.columns[i].percent.Text, h.columns[i].level)
			}
		}
		if !h.columns[2].artDim {
			t.Error("the case is bright with no reading")
		}
	})
	// Everything arrives together.
	r.dev.Update(func(c *sim.Config) {
		full := ownerConfig()
		c.ANC, c.Capabilities, c.Battery, c.Ages, c.Present, c.Settings = 4, full.Capabilities, [3]byte{80, 79, 255}, [3]uint32{1000, 1000, 0xffffffff}, 3, full.Settings
	})
	r.until(func() bool { return r.w.header.shown[0].percent == "80%" }, "the readings")
	r.main(func() {
		h := r.w.header
		if h.columns[0].artDim || h.columns[1].artDim || h.shown[1].percent != "79%" || h.shown[0].dim {
			t.Errorf("after the report: %+v", h.shown)
		}
	})
}

// The dongle keeps the last levels with young ages once the AirPods are away.
func TestHeaderDimsEverythingWithoutAConnection(t *testing.T) {
	cfg := ownerConfig()
	cfg.Battery, cfg.Ages, cfg.Present = [3]byte{100, 100, 60}, [3]uint32{1000, 1000, 1000}, 7
	r := newRig(t, cfg)
	r.main(func() {
		for i, column := range r.w.header.columns {
			if column.artDim || column.percent.Text == dashText {
				t.Fatalf("column %d while connected: art dim %v, %q", i, column.artDim, column.percent.Text)
			}
		}
	})
	r.dev.SetFlags(0)
	r.until(func() bool { return !r.w.state.Connected() }, "away")
	r.main(func() {
		for i, column := range r.w.header.columns {
			if !column.artDim || !column.dim || column.percent.Text != dashText || column.level != -1 {
				t.Errorf("column %d while away: art dim %v, dim %v, %q, gauge %d", i, column.artDim, column.dim, column.percent.Text, column.level)
			}
		}
	})
}

func TestBatteryWording(t *testing.T) {
	for _, item := range []struct {
		in   tray.Reading
		want reading
	}{
		{tray.Reading{}, reading{level: -1, percent: dashText, dim: true}},
		{tray.Reading{Known: true, Level: 50, Current: true}, reading{level: 50, percent: "50%"}},
		{tray.Reading{Known: true, Level: 50, Current: true, Charging: true}, reading{level: 50, percent: "50%", note: "Charging"}},
		{tray.Reading{Known: true, Level: 50, HasAge: true, AgeMs: 180_000}, reading{level: 50, percent: "50%", note: "3 min ago", dim: true}},
		{tray.Reading{Known: true, Level: 50}, reading{level: 50, percent: "50%", note: "Last known", dim: true}},
	} {
		if got := batteryReading(item.in); got != item.want {
			t.Errorf("%+v: %+v", item.in, got)
		}
	}
}

func TestNoDongleHidesWhatCannotWork(t *testing.T) {
	cfg := sim.Demo()
	cfg.OpenError = noDongleSim
	r := startRig(t, cfg, "")
	r.until(func() bool { return r.w.header.status.value == "No dongle" }, "no dongle")
	r.main(r.w.Show)
	r.main(func() {
		if r.w.header.status.value != "No dongle" {
			t.Errorf("status %q", r.w.header.status.value)
		}
		for _, entry := range r.w.sections {
			if entry.object.Visible() {
				t.Errorf("%s shows with no dongle", entry.fold.Title.Text)
			}
		}
		for _, group := range r.w.settings.cards {
			if group.group.Visible() {
				t.Errorf("%s shows with no dongle", group.base)
			}
		}
		if r.w.device.about.Visible() || r.w.device.leave.Visible() {
			t.Error("About and Disconnect show with no dongle")
		}
		if r.w.header.batteries.Visible() || !r.w.header.noDongle.Visible() {
			t.Error("AirPods and battery rings show with no dongle")
		}
	})
}

func TestEveryRowIsOnThePageInItsCard(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.main(func() {
		for _, row := range r.w.settings.rows {
			if _, ok := positionOf(r.w.Canvas().Content(), row.body, fyne.Position{}); !ok {
				t.Errorf("%s is not on the page", row.setting.Name)
			}
		}
		want := map[string]string{
			"adaptive-strength": listeningGroup, "hold-modes": holdGroup, "call-controls": "Call Controls",
			"conversation-awareness": "Audio", "microphone": generalGroup, "connect-automatically": generalGroup,
			"one-bud-anc": "Accessibility", "tone-volume": "Accessibility", "allow-off": "Off Listening Mode",
			"en352-protection": "Audio", "gain-swipe": "Accessibility",
			"crown-direction": experimentGroup, "case-sounds": experimentGroup, "case-volume": experimentGroup, "allow-auto-connect": experimentGroup,
		}
		for _, group := range r.w.settings.cards {
			for _, row := range group.rows {
				if title, ok := want[row.setting.Name]; ok && title != group.base {
					t.Errorf("%s in %q", row.setting.Name, group.base)
				}
				delete(want, row.setting.Name)
			}
		}
		if len(want) != 0 {
			t.Errorf("in no card: %v", want)
		}
	})
}

// TestPageFollowsTheAirPodsSettingsPage pins the order: the cards and rows of the AirPods'
// own settings page first, then About and Disconnect, then what the experimental switch adds.
func TestPageFollowsTheAirPodsSettingsPage(t *testing.T) {
	want := []struct {
		title string
		names string
	}{
		{"Listening Mode", "mode adaptive-strength"},
		{"Press and Hold AirPods", "hold-actions hold-modes"},
		{"Call Controls", "call-controls"},
		{"Audio", "personalized-volume conversation-awareness en352-protection"},
		{"", "ear-detection connect-automatically microphone pause-when-asleep optimized-charging"},
		{"Accessibility", "tone-volume press-speed hold-duration one-bud-anc volume-swipe swipe-duration gain-swipe"},
		{"Off Listening Mode", "allow-off"},
		{"Experimental", "crown-direction case-sounds case-volume allow-auto-connect"},
	}
	if len(settingGroups) != len(want) {
		t.Fatalf("%d groups", len(settingGroups))
	}
	for i, group := range settingGroups {
		if group.title != want[i].title || strings.Join(group.names, " ") != want[i].names {
			t.Errorf("group %d is %q with %v", i, group.title, group.names)
		}
	}
	r := newRigWith(t, sim.Demo(), func(o *Options) { o.Disconnect = func() {} })
	r.main(func() { r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"}) })
	r.shown()
	r.experiments(true)
	r.tall()
	r.main(func() {
		s, d := r.w.settings, r.w.device
		if len(s.cards) != len(want) {
			t.Fatalf("%d cards", len(s.cards))
		}
		order := []fyne.CanvasObject{r.w.header.content}
		names := []string{"header"}
		for i, card := range s.cards[:len(want)-1] {
			if card.title.Text != want[i].title {
				t.Errorf("card %d is captioned %q", i, card.title.Text)
			}
			order, names = append(order, card.group), append(names, "card "+card.base)
		}
		order = append(order, d.about, d.leave, s.cards[len(want)-1].group, r.w.playbackRow.object, r.w.nearbyRow.object,
			r.w.connectionRow.object, r.w.diagnosticsRow.object, r.w.extras, r.w.hardwareRow.object)
		names = append(names, "About", "Disconnect", "experimental card", "Playback", "Find nearby", "Connection", "Diagnostics", "Hearing and experiments", "Hardware checks")
		last := float32(-1)
		for i, object := range order {
			pos, ok := positionOf(r.w.page, object, fyne.Position{})
			if !ok || !r.onScreen(object) {
				t.Errorf("%s is not showing", names[i])
				continue
			}
			if pos.Y <= last {
				t.Errorf("%s is at %v, above what should come before it", names[i], pos.Y)
			}
			last = pos.Y
		}
		// Inside a card the rows run in the listed order too.
		for _, card := range s.cards[1:] {
			last := float32(-1)
			for _, row := range card.rows {
				if !row.body.Visible() {
					continue
				}
				pos, _ := positionOf(r.w.page, row.body, fyne.Position{})
				if pos.Y <= last {
					t.Errorf("%s is out of order in %q", row.setting.Name, card.base)
				}
				last = pos.Y
			}
		}
	})
}

// TestExperimentalSwitchHidesWhatTheAirPodsPageLacks has everything the AirPods' own
// settings page does not have off the page until the switch is on.
func TestExperimentalSwitchHidesWhatTheAirPodsPageLacks(t *testing.T) {
	r := newRigWith(t, sim.Demo(), func(o *Options) { o.Disconnect = func() {} })
	r.main(func() { r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"}) })
	r.shown()
	s := r.w.settings
	extra := func() map[string]fyne.CanvasObject {
		out := map[string]fyne.CanvasObject{
			"the experimental card": s.experiments, "Playback": r.w.playbackRow.object, "Find nearby": r.w.nearbyRow.object,
			"Connection": r.w.connectionRow.object, "Diagnostics": r.w.diagnosticsRow.object, "Hardware checks": r.w.hardwareRow.object,
			"case-sounds": s.byName["case-sounds"].body, "case-volume": s.byName["case-volume"].body,
			"allow-auto-connect": s.byName["allow-auto-connect"].body,
		}
		for _, entry := range r.w.sections {
			out[entry.fold.Title.Text] = entry.object
		}
		return out
	}
	core := map[string]fyne.CanvasObject{
		"the pill": s.byName["mode"].body, "hold-modes": s.byName["hold-modes"].body, "call-controls": s.byName["call-controls"].body,
		"personalized-volume": s.byName["personalized-volume"].body, "ear-detection": s.byName["ear-detection"].body,
		"tone-volume": s.byName["tone-volume"].body, "the model": r.w.device.modelName, "the version": r.w.device.firmware,
		"the serial": r.w.device.airpodsSerial, "Disconnect": r.w.device.disconnect,
	}
	check := func(on bool) {
		t.Helper()
		for name, object := range extra() {
			if r.onScreen(object) != on {
				t.Errorf("experimental %v: %s showing %v", on, name, !on)
			}
		}
		for name, object := range core {
			if !r.onScreen(object) {
				t.Errorf("experimental %v: %s is not showing", on, name)
			}
		}
	}
	r.main(func() {
		check(false)
		before := r.w.page.MinSize().Height
		r.w.about.experimental.SetChecked(true)
		check(true)
		if r.w.page.MinSize().Height <= before {
			t.Error("the page did not grow")
		}
		// Open, the sections show what the page keeps nowhere else.
		for _, entry := range r.w.sections {
			entry.fold.Toggle()
		}
		for name, object := range map[string]fyne.CanvasObject{
			"the dongle's state": r.w.device.status, "Reconnect": r.w.device.reconnect, "Pair": r.w.device.pair, "Read again": s.rereadBt,
		} {
			if !r.onScreen(object) {
				t.Errorf("%s is not in an open section", name)
			}
		}
	})
	r.until(func() bool { return r.w.nearby.state.canStart() }, "identity known")
	r.main(func() { test.Tap(r.w.nearby.start) })
	r.until(func() bool { return r.w.nearby.state.mine && !r.w.nearby.state.starting }, "finder started")
	r.main(func() {
		r.w.about.experimental.SetChecked(false)
		check(false)
		for _, entry := range r.w.sections {
			if entry.fold.Open() {
				t.Errorf("%s stayed open", entry.fold.Title.Text)
			}
		}
		if r.w.nearby.state.mine {
			t.Error("the finder must end with its section")
		}
	})
}

func TestClickingASwitchWritesThroughTheOwner(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.tall()
	r.main(func() { r.click(r.w.settings.byName["optimized-charging"].toggle) })
	r.until(func() bool { return r.dev.Writes() == 1 }, "write")
	r.until(settled(r), "settled")
	if got := lastCommitValue(t, r.dev, 0x3b); got != 2 {
		t.Errorf("wrote %d", got)
	}
}

func TestClickingTheDashChoosesOn(t *testing.T) {
	cfg := ownerConfig()
	cfg.WriteMode = sim.WriteLost
	r := newRig(t, cfg)
	r.main(r.w.Show)
	r.until(func() bool { return r.w.settings.byName["mode"].reported }, "read")
	r.experiments(true) // the charging case's rows are in the experimental card
	r.tall()
	row := r.w.settings.byName["case-sounds"]
	r.main(func() {
		if row.note.Text != "" {
			t.Errorf("unknown row note %q", row.note.Text)
		}
		r.click(row.dash)
	})
	r.until(func() bool { return r.dev.Writes() == 1 }, "write")
	r.until(settled(r), "settled")
	r.main(func() {
		if !row.toggle.Visible() || row.dash.Visible() || !row.toggle.Checked {
			t.Errorf("after the choice: toggle %v dash %v checked %v", row.toggle.Visible(), row.dash.Visible(), row.toggle.Checked)
		}
		if row.note.Text != sentText() {
			t.Errorf("note %q", row.note.Text)
		}
	})
	if got := lastCommitValue(t, r.dev, 0x31); got != 1 {
		t.Errorf("wrote %d", got)
	}
}

func TestClickingASegmentChangesTheMode(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.tall()
	seg := r.w.settings.byName["mode"].segment
	r.main(func() {
		pos, ok := positionOf(r.w.Canvas().Content(), seg, fyne.Position{})
		if !ok {
			t.Fatal("no pill")
		}
		// Transparency is the second of four on a model with no Off report blocking it.
		index := -1
		for i, option := range seg.Options {
			if option == "Transparency" {
				index = i
			}
		}
		each := seg.Size().Width / float32(len(seg.Options))
		test.TapCanvas(r.w.Canvas(), pos.Add(fyne.NewPos(each*(float32(index)+0.5), 20)))
	})
	r.until(func() bool { return r.dev.Writes() == 1 }, "write")
	r.until(settled(r), "settled")
	if got := lastCommitValue(t, r.dev, 0x0d); got != 3 {
		t.Errorf("wrote %d", got)
	}
}

func TestStrengthSliderOnlyInsideAdaptive(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.main(func() {
		if r.w.settings.strength.Visible() {
			t.Error("shown outside Adaptive")
		}
		if r.w.settings.byName["mode"].segment.OnStrength != nil {
			t.Error("nothing to drag outside Adaptive")
		}
		r.w.settings.byName["mode"].segment.SetSelected("Adaptive")
	})
	r.until(settled(r), "write")
	r.until(func() bool { return r.w.settings.byName["mode"].decoded == "adaptive" }, "adaptive")
	r.main(func() {
		seg := r.w.settings.byName["mode"].segment
		if open := seg.lane(seg.Size().Width) >= 0; r.w.settings.strength.Visible() == open || r.rowDisabled("adaptive-strength") {
			t.Errorf("strength in Adaptive: lane %v, slider %v", open, r.w.settings.strength.Visible())
		}
		if r.w.settings.byName["mode"].segment.OnStrength == nil {
			t.Error("the capsule must drag in Adaptive")
		}
	})
}

func TestSectionsExpandByClickAndStartCollapsed(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.experiments(true)
	r.tall()
	r.main(func() {
		for _, entry := range r.w.sections {
			if entry.fold.Open() {
				t.Errorf("%s starts open", entry.fold.Title.Text)
			}
		}
		before := r.w.page.MinSize().Height
		r.click(r.w.playbackRow.fold.header)
		if !r.w.playbackRow.fold.Open() || !r.w.playbackRow.fold.well.Visible() {
			t.Error("did not open")
		}
		if r.w.page.MinSize().Height <= before {
			t.Error("the page did not grow")
		}
	})
}

func TestNearbyStopsWhenCollapsed(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.until(func() bool { return r.w.nearby.state.canStart() }, "identity known")
	r.experiments(true)
	r.tall()
	stops := func() int {
		count := 0
		for _, commit := range r.dev.Commits() {
			if commit[0] == 8 {
				count++
			}
		}
		return count
	}
	r.main(func() {
		r.click(r.w.nearbyRow.fold.header)
		test.Tap(r.w.nearby.start)
	})
	r.until(func() bool { return r.w.nearby.state.mine && !r.w.nearby.state.starting }, "started")
	r.main(func() { r.click(r.w.nearbyRow.fold.header) })
	r.until(func() bool { return stops() == 1 }, "stop sent")
	r.main(func() {
		if r.w.nearby.state.mine {
			t.Error("still searching")
		}
	})
}

func TestNearbyStopsWhenItsSectionIsGatedAway(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.until(func() bool { return r.w.nearby.state.canStart() }, "identity known")
	r.experiments(true)
	r.main(func() {
		r.w.nearbyRow.fold.Toggle()
		test.Tap(r.w.nearby.start)
	})
	r.until(func() bool { return r.w.nearby.state.mine && !r.w.nearby.state.starting }, "started")
	r.main(func() {
		st := r.w.state
		st.Status.Capabilities = uint16(dongle.CapControl)
		r.w.Update(st)
		if r.w.nearbyRow.object.Visible() || r.w.nearbyRow.fold.Open() || r.w.nearby.state.mine {
			t.Error("the finder must end with its section")
		}
	})
}

func TestPollUpdatesKeepScrollAndWidgets(t *testing.T) {
	r := newRig(t, ownerConfig())
	r.shown()
	r.main(func() {
		r.w.win.Resize(fyne.NewSize(defaultWidth, 500))
		r.w.relayout()
		r.w.scroll.ScrollToOffset(fyne.NewPos(0, 400))
		if r.w.scroll.Offset.Y != 400 {
			t.Fatalf("offset %v", r.w.scroll.Offset)
		}
		before := r.w.page.Objects[1]
		row := r.w.settings.byName["optimized-charging"]
		toggle, position := row.toggle, row.body.Position()
		size := r.w.page.Size()
		st := r.w.state
		for i := range 50 {
			st.Status.BatteryLeft = byte(50 + i%40)
			st.Status.AgeLeft = uint32(i * 100)
			st.Status.Streaming = i%2 == 0
			r.w.Update(st)
			r.w.tick()
		}
		if r.w.scroll.Offset.Y != 400 {
			t.Errorf("scroll moved to %v", r.w.scroll.Offset)
		}
		if r.w.page.Objects[1] != before || row.toggle != toggle || row.body.Position() != position || r.w.page.Size() != size {
			t.Error("a poll rebuilt or moved the rows")
		}
	})
}

func TestReadFailureLeavesThePageAliveWithOneLine(t *testing.T) {
	cfg := sim.Demo()
	cfg.Fail = map[byte]byte{2: 1}
	r := newRig(t, cfg)
	r.main(r.w.Show)
	r.until(func() bool { return r.w.settings.failures >= 1 && !r.w.settings.reading }, "failed read")
	r.main(func() {
		if !r.w.banner.Visible() || !strings.HasPrefix(r.w.banner.Text, readProblemPrefix()) {
			t.Errorf("banner %q", r.w.banner.Text)
		}
		for _, row := range r.w.settings.rows {
			if row.note.Text != "" {
				t.Errorf("%s has a note before any read: %q", row.setting.Name, row.note.Text)
			}
		}
		if r.rowDisabled("ear-detection") {
			t.Error("the page must stay usable")
		}
	})
}

func TestSuccessfulWriteClearsTheProblemLine(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.main(func() {
		r.w.say("Audio: something went wrong")
		r.w.settings.byName["optimized-charging"].toggle.SetChecked(false)
	})
	r.until(func() bool { return r.dev.Writes() == 1 }, "write")
	r.until(settled(r), "settled")
	r.main(func() {
		if r.w.banner.Text != "" || r.w.banner.Visible() {
			t.Errorf("banner %q", r.w.banner.Text)
		}
	})
}

func TestOverflowOpensAppSettingsAndConnection(t *testing.T) {
	r := newRig(t, sim.Demo())
	disconnects := 0
	r.main(func() {
		r.w.opts.Disconnect = func() { disconnects++ }
		r.click(r.w.header.overflow)
		if !r.w.menu.pop.Visible() {
			t.Fatal("the panel did not open")
		}
		for name, item := range map[string]fyne.CanvasObject{
			"theme": r.w.about.theme, "experimental": r.w.about.experimental, "reconnect": r.w.device.menuReconnect,
			"disconnect": r.w.device.menuDisconnect, "pair a phone": r.w.device.menuPairPhone, "pair": r.w.device.menuPair,
			"read again": r.w.settings.rereadItem, "app version": r.w.about.version,
		} {
			if _, ok := positionOf(r.w.menu.pop, item, fyne.Position{}); !ok {
				t.Errorf("%s not in the panel", name)
			}
		}
		r.w.Update(r.w.state)
		test.Tap(r.w.device.menuDisconnect)
		if disconnects != 1 || r.w.menu.pop.Visible() {
			t.Errorf("disconnects %d, panel %v", disconnects, r.w.menu.pop.Visible())
		}
	})
}

func TestDisconnectActionHiddenWithoutAHandlerAndEnabledForItsFallback(t *testing.T) {
	cfg := sim.Demo()
	cfg.Capabilities &^= dongle.CapRouting // firmware older than 0.1.59
	r := newRig(t, cfg)
	r.main(func() {
		if r.w.device.menuDisconnect.Visible() || r.w.device.leave.Visible() {
			t.Error("nothing to call")
		}
		if !r.onScreen(r.w.device.about) {
			t.Error("About does not need the handler")
		}
	})
	r.dev.SetFlags(0)
	r.until(func() bool { return !r.w.state.Connected() }, "away")
	disconnects := 0
	r.main(func() {
		r.w.opts.Disconnect = func() { disconnects++ }
		r.w.Update(r.w.state)
		// The page's own Disconnect, below About, with the experimental switch off.
		if !r.w.device.menuDisconnect.Visible() || !r.onScreen(r.w.device.disconnect) {
			t.Error("the handler shows the action in the panel and on the page")
		}
		if r.w.device.disconnect.Disabled() {
			t.Error("with no dongle command it stays enabled to explain itself")
		}
		test.Tap(r.w.device.disconnect)
		if disconnects != 1 {
			t.Errorf("disconnects %d", disconnects)
		}
	})
}

// With the routing handover the Disconnect card follows the release: closing while the
// connection is still up, then Reconnect, which the dongle needs before it takes the AirPods
// back.
func TestDisconnectCardOffersReconnectAfterARelease(t *testing.T) {
	r := newRigWith(t, sim.Demo(), func(o *Options) { o.Disconnect = func() {} })
	d := r.w.device
	r.main(func() {
		if d.disconnect.Text != i18n.T("device.disconnect") || d.disconnect.Disabled() || d.leaveRow.Hint.Text != i18n.T("device.disconnectHint") {
			t.Errorf("connected: %q disabled %v hint %q", d.disconnect.Text, d.disconnect.Disabled(), d.leaveRow.Hint.Text)
		}
		r.w.DisconnectSent()
		if !d.disconnect.Disabled() || !d.menuDisconnect.Disabled() {
			t.Error("a second disconnect while one is out")
		}
		r.w.DisconnectDone(disconnect.ReleasePending, "")
		if !d.disconnect.Disabled() || d.leaveRow.Hint.Text != i18n.T("device.closingHint") {
			t.Errorf("closing: disabled %v hint %q", d.disconnect.Disabled(), d.leaveRow.Hint.Text)
		}
		if !strings.HasSuffix(r.w.banner.Text, i18n.T("disconnect.pending")) {
			t.Errorf("banner %q", r.w.banner.Text)
		}
	})
	r.dev.SetFlags(0)
	r.until(func() bool { return !r.w.state.Connected() }, "away")
	r.main(func() {
		if r.w.Release() != disconnect.Released {
			t.Errorf("release %v once the connection closed", r.w.Release())
		}
		if d.leaveRow.Title.Text != i18n.T("tray.reconnect") || d.disconnect.Text != i18n.T("device.reconnect") ||
			d.disconnect.Disabled() || d.leaveRow.Hint.Text != i18n.T("device.releasedHint") {
			t.Errorf("released: %q %q disabled %v hint %q", d.leaveRow.Title.Text, d.disconnect.Text, d.disconnect.Disabled(), d.leaveRow.Hint.Text)
		}
		if !r.onScreen(d.disconnect) {
			t.Error("Reconnect is not on the page")
		}
		test.Tap(d.disconnect)
	})
	// The handover takes them back, and the dongle confirms it owns them.
	r.until(func() bool { return !r.w.reconnecting && r.w.Release() == 0 }, "reconnect done")
	if n := len(r.dev.Controls()); n != 0 {
		t.Errorf("%d control commands beside the handover", n)
	}
	r.until(func() bool { return r.w.state.Connected() }, "back")
	r.main(func() {
		if d.disconnect.Text != i18n.T("device.disconnect") || d.disconnect.Disabled() {
			t.Errorf("back: %q disabled %v", d.disconnect.Text, d.disconnect.Disabled())
		}
	})
}

func TestPairOnThePageAsksFirst(t *testing.T) {
	r := newRig(t, sim.Demo())
	asked := 0
	r.experiments(true) // the page's Pair is in the Connection section
	r.main(func() {
		r.w.confirm = func(_, _, _ string, _ func(bool)) { asked++ }
		if !r.w.connectionRow.object.Visible() {
			t.Error("no Connection section")
		}
		test.Tap(r.w.device.pair)
		test.Tap(r.w.device.menuPair)
	})
	if asked != 2 {
		t.Errorf("asked %d", asked)
	}
	if len(r.dev.Controls()) != 0 {
		t.Error("a command was sent")
	}
}

func TestAddedSectionFollowsItsGate(t *testing.T) {
	r := newRig(t, sim.Demo())
	wanted := false
	var opened []bool
	r.main(func() {
		body := container.NewVBox()
		r.w.AddSection("Hearing", "", body, func() bool { return wanted }, func(open bool) { opened = append(opened, open) })
		entry := r.w.sections[len(r.w.sections)-1]
		if entry.object.Visible() {
			t.Error("gated away, yet shown")
		}
		wanted = true
		r.w.gate()
		if !entry.object.Visible() {
			t.Error("gate open, yet hidden")
		}
		entry.fold.Toggle()
		wanted = false
		r.w.gate()
		if entry.object.Visible() || entry.fold.Open() {
			t.Error("must fold and go")
		}
		if fmt.Sprint(opened) != "[true false]" {
			t.Errorf("toggles %v", opened)
		}
	})
}

func TestRowsShowAtMostOneHint(t *testing.T) {
	cfg := ownerConfig()
	r := newRig(t, cfg)
	r.main(r.w.Show)
	r.until(func() bool { return r.w.settings.byName["mode"].reported && !r.w.settings.reading }, "read")
	r.main(func() {
		for _, row := range r.w.settings.rows {
			if row.help != nil && row.help.Visible() && row.note.Visible() {
				t.Errorf("%s shows two hints", row.setting.Name)
			}
			if strings.Contains(strings.ToLower(row.note.Text), "unknown") {
				t.Errorf("%s says %q", row.setting.Name, row.note.Text)
			}
		}
	})
}

func TestLinkChangeDropsTheNotReportedHints(t *testing.T) {
	r := newRig(t, ownerConfig())
	r.main(r.w.Show)
	r.until(func() bool { return r.w.settings.byName["mode"].reported && !r.w.settings.reading }, "read")
	r.settle()
	r.main(func() {
		if r.w.settings.byName["crown-direction"].note.Text != notReportedHint() {
			t.Errorf("note %q", r.w.settings.byName["crown-direction"].note.Text)
		}
	})
	r.dev.SetFlags(0)
	r.until(func() bool { return !r.w.state.Connected() }, "away")
	r.main(func() {
		if got := r.w.settings.byName["crown-direction"].note.Text; got != "" {
			t.Errorf("a disconnected page proves nothing: %q", got)
		}
	})
}

// TestRenderScreens saves the page as PNG when AIRPODS_SCREENS_DIR is set.
func TestRenderScreens(t *testing.T) {
	dir := os.Getenv("AIRPODS_SCREENS_DIR")
	if dir == "" {
		t.Skip("AIRPODS_SCREENS_DIR not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	sizes := []struct {
		name string
		size fyne.Size
	}{{"default", fyne.NewSize(defaultWidth, defaultHeight)}, {"wide", fyne.NewSize(1000, 700)}}
	for _, variant := range []string{ThemeDark, ThemeLight} {
		for _, shape := range sizes {
			prefix := filepath.Join(dir, variant+"-"+shape.name+"-")
			renderOwner(t, variant, shape.size, prefix)
			renderAway(t, variant, shape.size, prefix)
			renderWaiting(t, variant, shape.size, prefix)
		}
	}
}

func renderOwner(t *testing.T, variant string, size fyne.Size, prefix string) {
	t.Helper()
	cfg := ownerConfig()
	cfg.WriteMode = sim.WriteLost
	r := startRig(t, cfg, variant)
	r.until(func() bool { return r.w.state.Present }, "dongle present")
	r.main(func() {
		r.w.opts.Disconnect = func() {}
		r.w.Update(r.w.state)
		r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348", Serial: "SIMSERIAL123"})
		r.w.win.Resize(size)
		r.w.Show()
	})
	r.until(func() bool {
		return r.w.settings.byName["mode"].reported && r.w.audio.have && !r.w.settings.reading && r.w.nearby.state.snapshot != nil
	}, "first reads")
	shot := func(name string) {
		t.Helper()
		r.main(func() {
			r.w.relayout()
			savePNG(t, prefix+name+".png", r.w)
		})
	}
	shot("top")
	scroll := func(y float32) { r.main(func() { r.w.scroll.ScrollToOffset(fyne.NewPos(0, y)) }) }
	// The whole page as it is with the experimental switch off, a window's height at a time.
	var tall float32
	r.main(func() { tall = r.w.scroll.Content.MinSize().Height })
	for i, name := range []string{"scrolled", "scrolled2", "scrolled3", "scrolled4"} {
		y := float32(i+1) * (size.Height - 120)
		if y > tall-120 {
			break
		}
		scroll(y)
		shot(name)
	}
	r.experiments(true)
	r.main(func() {
		r.w.relayout()
		pos, _ := positionOf(r.w.page, r.w.device.about, fyne.Position{})
		r.w.scroll.ScrollToOffset(fyne.NewPos(0, pos.Y))
	})
	shot("experimental")
	r.main(func() {
		r.w.playbackRow.fold.Toggle()
		r.w.connectionRow.fold.Toggle()
		r.w.relayout()
		r.w.scroll.ScrollToOffset(fyne.NewPos(0, r.w.page.MinSize().Height))
	})
	shot("open-playback-connection")
	r.main(func() {
		r.w.playbackRow.fold.Toggle()
		r.w.connectionRow.fold.Toggle()
		r.w.nearbyRow.fold.Toggle()
		test.Tap(r.w.nearby.start)
	})
	r.until(func() bool { return !r.w.nearby.inFlight }, "finder started")
	r.main(r.w.tick)
	r.until(func() bool { return r.w.nearby.signalText == "-58 dBm" }, "signal")
	r.main(func() {
		r.w.diagnosticsRow.fold.Toggle()
		r.w.hardwareRow.fold.Toggle()
		r.w.relayout()
		r.w.scroll.ScrollToOffset(fyne.NewPos(0, r.w.nearbyRow.object.Position().Y))
	})
	r.until(func() bool { return r.w.diag.values["Uptime"].Text != notReported() }, "counters")
	shot("open-nearby-diagnostics")
	r.main(func() {
		r.w.scroll.ScrollToOffset(fyne.NewPos(0, r.w.page.MinSize().Height))
	})
	shot("open-hardware")
	r.main(func() {
		r.w.scroll.ScrollToOffset(fyne.NewPos(0, 0))
		r.w.relayout()
		r.w.menu.open()
	})
	shot("overflow")
	r.main(func() {
		r.w.menu.close()
		r.w.confirmDialog("Reset the AirPods settings?", "Every setting goes back to what the AirPods came with. This cannot be undone.", "Reset", func(bool) {})
	})
	shot("question")
	r.main(func() {
		r.w.win.Canvas().Overlays().Remove(r.w.win.Canvas().Overlays().Top())
		r.w.informDialog("Disconnect", "This dongle has no disconnect command.")
	})
	shot("notice")
	r.main(func() {
		r.w.win.Canvas().Overlays().Remove(r.w.win.Canvas().Overlays().Top())
		// The real Show batteries content, as the tray's popup sizes it.
		batteries := NewBatterySheet(func() {})
		batteries.Update(r.w.state, model.View{Model: "A3063"})
		win := test.NewWindow(batteries.Object())
		defer win.Close()
		win.SetPadded(false)
		win.Resize(fyne.NewSize(330, win.Content().MinSize().Height))
		file, err := os.Create(prefix + "batteries.png")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, win.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
	})
}

func renderAway(t *testing.T, variant string, size fyne.Size, prefix string) {
	t.Helper()
	cfg := sim.Demo()
	cfg.OpenError = noDongleSim
	none := startRig(t, cfg, variant)
	none.main(func() {
		none.w.win.Resize(size)
		none.w.Show()
	})
	none.flush()
	none.main(func() {
		none.w.relayout()
		savePNG(t, prefix+"no-dongle.png", none.w)
	})

	away := ownerConfig()
	r := startRig(t, away, variant)
	r.until(func() bool { return r.w.state.Present }, "dongle present")
	r.main(func() {
		r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"})
		r.w.win.Resize(size)
		r.w.Show()
	})
	r.until(func() bool { return r.w.settings.byName["mode"].reported && !r.w.settings.reading }, "first read")
	r.dev.SetFlags(0)
	r.until(func() bool { return !r.w.state.Connected() }, "disconnected")
	r.main(func() {
		r.w.relayout()
		savePNG(t, prefix+"disconnected.png", r.w)
	})
}

// renderWaiting is the first half minute of a connection, before the AirPods report anything.
func renderWaiting(t *testing.T, variant string, size fyne.Size, prefix string) {
	t.Helper()
	r := startRig(t, waitingConfig(), variant)
	r.until(func() bool { return r.w.state.Present }, "dongle present")
	r.main(func() {
		r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348"})
		r.w.win.Resize(size)
		r.w.Show()
	})
	r.until(func() bool { return r.w.settings.everRead && !r.w.settings.reading }, "first read")
	r.main(func() {
		r.w.relayout()
		savePNG(t, prefix+"waiting.png", r.w)
	})
}

func savePNG(t *testing.T, path string, w *Window) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, w.Canvas().Capture()); err != nil {
		t.Fatal(err)
	}
}

func TestOverflowStaysShutUntilAsked(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.shown()
	r.main(func() {
		r.w.SetDongles(nil)
		r.w.retext()
		if r.w.menu.pop.Visible() {
			t.Error("the panel opened by itself")
		}
	})
}

func TestQuestionsAndNoticesAreSheets(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.main(func() {
		overlays := r.w.win.Canvas().Overlays()
		var answers []bool
		r.w.confirmDialog("Reset", "Really?", "Reset", func(ok bool) { answers = append(answers, ok) })
		if overlays.Top() == nil {
			t.Fatal("the question did not open")
		}
		test.Tap(findButton(t, overlays.Top(), "Reset"))
		if len(answers) != 1 || !answers[0] || overlays.Top() != nil {
			t.Errorf("answers %v, still open %v", answers, overlays.Top() != nil)
		}
		r.w.informDialog("Done", "It is done.")
		test.Tap(findButton(t, overlays.Top(), "OK"))
		if overlays.Top() != nil {
			t.Error("the notice stayed open")
		}
	})
}

func findButton(t *testing.T, root fyne.CanvasObject, label string) *widget.Button {
	t.Helper()
	var found *widget.Button
	var walk func(fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		switch o := o.(type) {
		case *widget.Button:
			if o.Text == label {
				found = o
			}
		case *fyne.Container:
			for _, child := range o.Objects {
				walk(child)
			}
		case fyne.Widget:
			for _, child := range test.WidgetRenderer(o).Objects() {
				walk(child)
			}
		}
	}
	walk(root)
	if found == nil {
		t.Fatalf("no %q button", label)
	}
	return found
}

// Leaving Adaptive must not flash the separate slider while the new mode is still on its way.
func TestLeavingAdaptiveShowsNoStrengthSlider(t *testing.T) {
	cfg := sim.Demo()
	cfg.ANC = 4
	cfg.Settings[0].Value = 4
	r := newRig(t, cfg)
	r.shown()
	r.until(func() bool { return r.w.settings.byName["mode"].decoded == "adaptive" }, "adaptive")
	r.main(func() {
		r.w.settings.byName["mode"].segment.SetSelected("Transparency")
		r.w.settings.updateEnabled()
		if r.w.settings.strength.Visible() {
			t.Error("the slider showed while the mode was changing")
		}
	})
	r.until(settled(r), "write")
	r.main(func() {
		if r.w.settings.strength.Visible() {
			t.Error("the slider stayed")
		}
	})
}
