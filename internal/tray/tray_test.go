package tray

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
	"github.com/Dardrai/airpods-gui/internal/icon"
)

type fakeHost struct {
	menuSets int
	icons    []icon.Variant
	tooltip  string
	tooltips int
}

func (f *fakeHost) SetSystemTrayMenu(*fyne.Menu) { f.menuSets++ }
func (f *fakeHost) SetTrayIcon(v icon.Variant)   { f.icons = append(f.icons, v) }
func (f *fakeHost) SetTrayTooltip(text string)   { f.tooltip = text; f.tooltips++ }

func connected() device.State {
	return device.State{Present: true, Status: dongle.Status{
		Connected: true, ControlsReady: true, ANC: 2, Peer: [6]byte{1, 2, 3, 4, 5, 6},
		BatteryLeft: 80, BatteryRight: 75, BatteryCase: 40,
		Capabilities: dongle.CapControl | dongle.CapListening | dongle.CapPairPhone | dongle.CapRawTransport | dongle.CapTypedRPC,
	}}
}

func labels(menu *fyne.Menu) []string {
	var out []string
	for _, item := range menu.Items {
		if item.IsSeparator {
			out = append(out, "---")
			continue
		}
		out = append(out, item.Label)
	}
	return out
}

func TestModelLabels(t *testing.T) {
	st := connected()
	m := BuildModel(st, model.View{})
	if m.Header.Label != "AirPods · Connected" || !m.Header.Disabled {
		t.Errorf("header: %+v", m.Header)
	}
	if m.Battery.Label != "L 80% · R 75% · Case 40%" || !m.Battery.Disabled {
		t.Errorf("battery: %+v", m.Battery)
	}
	for i, row := range m.Modes {
		if row.Disabled || row.Checked != (Modes[i].Value == 2) {
			t.Errorf("mode %d: %+v", i, row)
		}
	}
	if m.Reconnect.Disabled {
		t.Error("reconnect disabled while connected")
	}
	if m.Tooltip != "AirPods: L 80% · R 75% · Case 40% · Noise Cancellation" {
		t.Errorf("tooltip: %q", m.Tooltip)
	}
}

func TestModelUnknownBattery(t *testing.T) {
	st := connected()
	st.Status.BatteryCase = dongle.BatteryUnknown
	st.Status.BatteryRight = 101
	if got := BuildModel(st, model.View{}).Battery.Label; got != "L 80% · R -- · Case --" {
		t.Errorf("battery: %q", got)
	}
}

func TestModelNotConnected(t *testing.T) {
	st := connected()
	st.Status.Connected = false
	m := BuildModel(st, model.View{})
	if m.Header.Label != "AirPods · Not connected" {
		t.Errorf("header: %q", m.Header.Label)
	}
	if m.Battery.Label != "L -- · R -- · Case --" {
		t.Errorf("readings of AirPods that are away: %q", m.Battery.Label)
	}
	if !m.Batteries.Disabled {
		t.Error("Show batteries with nothing current to show")
	}
	for _, row := range m.Modes {
		if !row.Disabled || row.Checked {
			t.Errorf("mode row live while disconnected: %+v", row)
		}
	}
	if m.Reconnect.Disabled {
		t.Error("reconnect must stay usable while the AirPods are away")
	}
}

func TestModelNoDongleAndProblem(t *testing.T) {
	m := BuildModel(device.State{}, model.View{})
	if m.Header.Label != "AirPods · No dongle" || !m.Reconnect.Disabled {
		t.Errorf("no dongle: %+v", m)
	}
	m = BuildModel(device.State{Problem: "two dongles"}, model.View{})
	if m.Header.Label != "AirPods · Dongle problem" {
		t.Errorf("problem: %+v", m.Header)
	}
}

func TestModelWithoutTypedSettingsKeepsOnlyOpcodeModes(t *testing.T) {
	st := connected()
	st.Status.Capabilities = dongle.CapControl | dongle.CapListening
	m := BuildModel(st, model.View{})
	want := []bool{true, false, false, true}
	for i, row := range m.Modes {
		if row.Disabled != want[i] {
			t.Errorf("mode %s disabled=%v", Modes[i].Word, row.Disabled)
		}
	}
}

func newTray(t *testing.T) (*Tray, *fakeHost, *[]string) {
	t.Helper()
	test.NewTempApp(t)
	host := &fakeHost{}
	var calls []string
	tr := New(host, Actions{
		SetMode:       func(word string) { calls = append(calls, "mode:"+word) },
		Disconnect:    func() { calls = append(calls, "disconnect") },
		Reconnect:     func() { calls = append(calls, "reconnect") },
		ShowBatteries: func() { calls = append(calls, "batteries") },
		OpenSettings:  func() { calls = append(calls, "open") },
		Quit:          func() { calls = append(calls, "quit") },
	})
	return tr, host, &calls
}

func TestMenuOrderAndActions(t *testing.T) {
	tr, host, calls := newTray(t)
	if host.menuSets != 1 {
		t.Fatalf("menu installed %d times", host.menuSets)
	}
	want := []string{
		"AirPods · No dongle", "L -- · R -- · Case --",
		"Off", "Noise Cancellation", "Transparency", "Adaptive",
		"---", "Show batteries", "Disconnect AirPods", "Reconnect AirPods", "Open settings", "Quit",
	}
	got := labels(tr.Menu())
	if len(got) != len(want) {
		t.Fatalf("menu: %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d: %q want %q", i, got[i], want[i])
		}
	}
	items := tr.Menu().Items
	if !items[len(items)-1].IsQuit {
		t.Error("last row is not the quit item, Fyne would add a second one")
	}
	items[3].Action()
	for _, i := range []int{7, 8, 9, 10, 11} {
		items[i].Action()
	}
	if got := strings.Join(*calls, " "); got != "mode:anc batteries disconnect reconnect open quit" {
		t.Errorf("actions: %q", got)
	}
}

func TestUpdateChangesRowsAndSkipsNoOps(t *testing.T) {
	tr, host, _ := newTray(t)
	st := connected()
	tr.Update(st)
	if host.tooltips != 2 || host.tooltip != BuildModel(st, model.View{}).Tooltip {
		t.Fatalf("tooltip: %d %q", host.tooltips, host.tooltip)
	}
	if !tr.modes[1].Checked || tr.modes[0].Checked || tr.header.Label != "AirPods · Connected" {
		t.Error("rows not updated")
	}
	// Ages move on every report without changing anything visible.
	st.Status.AgeLeft += 2000
	tr.Update(st)
	if host.tooltips != 2 {
		t.Error("no-op state rebuilt the menu")
	}
	st.Status.ANC = 3
	tr.Update(st)
	if !tr.modes[2].Checked || tr.modes[1].Checked {
		t.Error("radio rows must move together")
	}
}

func TestIconFollowsLowestLiveEarbud(t *testing.T) {
	st := connected()
	if got := IconFor(st); got != (icon.Variant{Level: 75}) {
		t.Errorf("connected: %+v", got)
	}
	st.Status.BatteryRight = dongle.BatteryUnknown
	if got := IconFor(st); got != (icon.Variant{Level: 80}) {
		t.Errorf("one bud: %+v", got)
	}
	st.Status.BatteryLeft = 200
	st.Status.BatteryCase = 3
	if got := IconFor(st); got != (icon.Variant{Level: icon.NoBadge}) {
		t.Errorf("the case must not set the badge: %+v", got)
	}
	st.Status.Connected = false
	st.Status.BatteryLeft = 10
	if got := IconFor(st); got != (icon.Variant{Level: icon.NoBadge, Dim: true}) {
		t.Errorf("away: %+v", got)
	}
	if got := IconFor(device.State{}); got != (icon.Variant{Level: icon.NoBadge, NoDongle: true}) {
		t.Errorf("no dongle: %+v", got)
	}
}

func TestIconOnlyResentWhenItChanges(t *testing.T) {
	tr, host, _ := newTray(t)
	st := connected()
	tr.Update(st)
	sent := len(host.icons)
	if sent != 2 || host.icons[1] != (icon.Variant{Level: 75}) {
		t.Fatalf("icons: %+v", host.icons)
	}
	st.Status.ANC = 3 // The menu changes, the icon does not.
	tr.Update(st)
	if len(host.icons) != sent {
		t.Error("unchanged icon was sent again")
	}
	st.Status.BatteryRight = 60
	tr.Update(st)
	if host.icons[len(host.icons)-1] != (icon.Variant{Level: 60}) {
		t.Errorf("badge did not follow: %+v", host.icons)
	}
}

func TestDisconnectRowWithoutCommandStaysEnabled(t *testing.T) {
	st := connected()
	if BuildModel(st, model.View{}).Disconnect.Disabled {
		t.Error("connected")
	}
	st.Status.Connected = false
	if BuildModel(st, model.View{}).Disconnect.Disabled {
		t.Error("away")
	}
	if BuildModel(device.State{}, model.View{}).Disconnect.Disabled {
		t.Error("no dongle")
	}
}

func TestDisconnectRowWithRoutingNeedsAConnection(t *testing.T) {
	st := connected()
	st.Status.Capabilities |= dongle.CapRouting
	if BuildModel(st, model.View{}).Disconnect.Disabled {
		t.Error("connected")
	}
	st.Status.Connected = false
	m := BuildModel(st, model.View{})
	if !m.Disconnect.Disabled {
		t.Error("nothing to disconnect")
	}
	if m.Reconnect.Disabled {
		t.Error("reconnect must be offered after a disconnect")
	}
}

func TestReconnectNeedsASavedPair(t *testing.T) {
	st := connected()
	st.Status.Connected = false
	st.Status.Peer = [6]byte{}
	if !BuildModel(st, model.View{}).Reconnect.Disabled {
		t.Error("nothing to reconnect to")
	}
}

func TestMenuFollowsTheModel(t *testing.T) {
	tr, _, _ := newTray(t)
	tr.Update(connected())
	if got := len(tr.Menu().Items); got != 12 {
		t.Fatalf("items before identification: %d", got)
	}
	tr.SetView(model.View{Model: "A2564", Firmware: "6A1"}) // AirPods 3: no modes
	for _, label := range labels(tr.Menu()) {
		if label == "Off" || label == "Transparency" {
			t.Errorf("mode row on a model without modes: %q", label)
		}
	}
	tr.SetView(model.View{Model: "A2096", Firmware: "6A1"}) // original Max: no Adaptive
	got := labels(tr.Menu())
	want := []string{"Off", "Noise Cancellation", "Transparency"}
	for i, label := range want {
		if got[2+i] != label {
			t.Errorf("row %d: %q", i, got[2+i])
		}
	}
	if got[5] != "---" {
		t.Errorf("Adaptive row still there: %q", got[5])
	}
	tr.SetView(model.View{Model: "A3063", Firmware: "9A348", Reported: map[byte]uint32{0x34: 2}})
	got = labels(tr.Menu())
	if got[2] != "Noise Cancellation" || got[4] != "Adaptive" {
		t.Errorf("Pro 3 without Allow Off: %q", got)
	}
}

func TestBatteryFreshness(t *testing.T) {
	st := connected()
	st.Status.AgeLeft, st.Status.AgeRight, st.Status.AgeCase = 30_000, 30_001, 89_999
	st.Status.CaseSourceMask = 0b100
	if got := BatteryLine(st); got != "L 80% · R 75%? · Case 40%" {
		t.Errorf("line %q", got)
	}
	st.Status.AgeCase = 90_000
	if got := ReadBattery(st, BatteryCase); got.Current {
		t.Errorf("case at 90 s: %+v", got)
	}
	if got := BatteryDetail(st); got != "Left 80% · Right 75% (30 s old) · Case 40% (90 s old)" {
		t.Errorf("detail %q", got)
	}
	st.Status.AgeLeft = unknownAge
	if got := ReadBattery(st, BatteryLeft); got.Current || got.HasAge {
		t.Errorf("no age: %+v", got)
	}
	if got := spelled("Left", ReadBattery(st, BatteryLeft)); got != "Left 80% (last known)" {
		t.Errorf("spelled %q", got)
	}
}

func TestBatteryPresenceAndCharging(t *testing.T) {
	st := connected()
	st.Status.Capabilities |= dongle.CapPerComponent
	st.Status.PresentMask = 0b001
	st.Status.ChargingMask = 0b011
	if got := BatteryLine(st); got != "L 80%+ · R 75%? · Case 40%?" {
		t.Errorf("line %q", got)
	}
	if got := ReadBattery(st, BatteryRight); got.Charging {
		t.Error("an absent bud cannot show as charging")
	}
	level, ok := LowestEarbud(st)
	if !ok || level != 80 {
		t.Errorf("the badge ignores stale readings: %d %v", level, ok)
	}
	if got := ReadBattery(device.State{}, BatteryLeft); got.Known {
		t.Error("no dongle")
	}
}

// The dongle keeps the last levels and young ages after the AirPods leave.
func TestNoReadingAndNoBrightArtWithoutAConnection(t *testing.T) {
	st := connected()
	st.Status.Capabilities |= dongle.CapPerComponent
	st.Status.PresentMask = 0b111
	st.Status.AgeLeft, st.Status.AgeRight, st.Status.AgeCase = 1000, 1000, 1000
	st.Status.Connected = false
	for _, component := range []int{BatteryLeft, BatteryRight, BatteryCase} {
		if got := ReadBattery(st, component); got.Known || got.Current || got.Charging {
			t.Errorf("component %d reads %+v while away", component, got)
		}
		if ArtAvailable(st, component) {
			t.Errorf("component %d bright while away", component)
		}
	}
	if HasReading(st) {
		t.Error("a reading to show while away")
	}
	if got := Rows(st, model.View{}); got != "Left unknown\nRight unknown\nCase unknown" {
		t.Errorf("popup rows %q", got)
	}
	if got := BatteryDetail(st); got != "Left unknown · Right unknown · Case unknown" {
		t.Errorf("detail %q", got)
	}
	if _, ok := LowestEarbud(st); ok {
		t.Error("a badge while away")
	}
}

// For the first half minute of a connection the dongle reports no presence and no battery.
func TestEarbudArtIsBrightWhileNothingIsReportedYet(t *testing.T) {
	st := connected()
	st.Status.Capabilities = 0x01fd
	if !st.Status.Has(dongle.CapPerComponent) {
		t.Fatal("the capabilities seen on the dongle carry the per-component bit")
	}
	st.Status.ANC, st.Status.PresentMask = 0, 0
	st.Status.BatteryLeft, st.Status.BatteryRight, st.Status.BatteryCase = 255, 255, 255
	st.Status.AgeLeft, st.Status.AgeRight, st.Status.AgeCase = unknownAge, unknownAge, unknownAge
	if !ArtAvailable(st, BatteryLeft) || !ArtAvailable(st, BatteryRight) {
		t.Error("the earbuds are greyed out before anything is reported")
	}
	if ArtAvailable(st, BatteryCase) {
		t.Error("the case is bright with no reading")
	}
	// One earbud reported: the other one really is away.
	st.Status.PresentMask, st.Status.BatteryLeft, st.Status.AgeLeft = 0b001, 80, 1000
	if !ArtAvailable(st, BatteryLeft) || ArtAvailable(st, BatteryRight) {
		t.Errorf("left %v right %v with only the left one present", ArtAvailable(st, BatteryLeft), ArtAvailable(st, BatteryRight))
	}
	// A reading with an empty mask is a report too: both are out of reach.
	st.Status.PresentMask = 0
	if ArtAvailable(st, BatteryLeft) || ArtAvailable(st, BatteryRight) {
		t.Error("bright earbuds that are reported as not present")
	}
}

func TestHeaderAndTooltipNameTheModel(t *testing.T) {
	st := connected()
	view := model.View{Model: "A3063", Firmware: "9A348"}
	m := BuildModel(st, view)
	if m.Header.Label != "AirPods Pro 3 · Connected" {
		t.Errorf("header: %q", m.Header.Label)
	}
	if m.Tooltip != "AirPods Pro 3: L 80% · R 75% · Case 40% · Noise Cancellation" {
		t.Errorf("tooltip: %q", m.Tooltip)
	}
	if got := BuildModel(st, model.View{Model: "Z9999"}).Header.Label; got != "AirPods · Connected" {
		t.Errorf("unlisted model: %q", got)
	}
	st.Status.Connected = false
	if got := BuildModel(st, view).Header.Label; got != "AirPods Pro 3 · Not connected" {
		t.Errorf("after disconnect: %q", got)
	}
	if got := BuildModel(device.State{}, view).Header.Label; got != "AirPods · No dongle" {
		t.Errorf("no dongle says nothing about the model: %q", got)
	}
}

func TestBatteryRowFreshnessAndCharging(t *testing.T) {
	st := connected()
	st.Status.Capabilities |= dongle.CapPerComponent
	st.Status.PresentMask = 1 | 2 | 4
	st.Status.ChargingMask = 4
	st.Status.AgeLeft, st.Status.AgeRight, st.Status.AgeCase = 30_000, 30_001, 1000
	if got := BuildModel(st, model.View{}).Battery.Label; got != "L 80% · R 75%? · Case 40%+" {
		t.Errorf("earbud rule: %q", got)
	}
	st.Status.AgeRight = 1000
	st.Status.CaseSourceMask = 1
	st.Status.AgeLeft = 89_999
	st.Status.ChargingMask = 0
	st.Status.AgeCase = 31_000
	if got := BuildModel(st, model.View{}).Battery.Label; got != "L 80% · R 75% · Case 40%?" {
		t.Errorf("case-sourced rule: %q", got)
	}
	st.Status.PresentMask = 2 | 4
	st.Status.AgeCase = 1000
	if got := BuildModel(st, model.View{}).Battery.Label; got != "L 80%? · R 75% · Case 40%" {
		t.Errorf("absent bud: %q", got)
	}
}

func TestBatteryRowForOverEarModels(t *testing.T) {
	st := connected()
	st.Status.BatteryRight, st.Status.BatteryCase = dongle.BatteryUnknown, dongle.BatteryUnknown
	if got := BuildModel(st, model.View{Model: "A3184"}).Battery.Label; got != "80%" {
		t.Errorf("battery: %q", got)
	}
}

func TestShowBatteriesNeedsAReading(t *testing.T) {
	if !BuildModel(device.State{}, model.View{}).Batteries.Disabled {
		t.Error("enabled with no dongle")
	}
	st := connected()
	if BuildModel(st, model.View{}).Batteries.Disabled {
		t.Error("disabled with readings")
	}
	st.Status.BatteryLeft, st.Status.BatteryRight, st.Status.BatteryCase = 255, 255, 255
	if !BuildModel(st, model.View{}).Batteries.Disabled {
		t.Error("enabled with nothing to show")
	}
}

func TestTooltipFitsWindows(t *testing.T) {
	if got := clip(strings.Repeat("é", 200), tooltipLimit); len([]rune(got)) != tooltipLimit {
		t.Errorf("%d runes", len([]rune(got)))
	}
	if got := clip("short", tooltipLimit); got != "short" {
		t.Errorf("%q", got)
	}
}

func TestPopupShowsUpdatesAndHidesItself(t *testing.T) {
	test.NewTempApp(t)
	calls := make(chan func(), 1)
	var made int
	popup := NewPopup(func() fyne.Window { made++; return test.NewWindow(nil) }, func(f func()) { calls <- f })
	popup.wait = 5 * time.Millisecond
	view := model.View{Model: "A3063", Firmware: "9A348"}
	popup.Update(connected(), view) // closed: nothing to do, nothing to crash
	if made != 0 {
		t.Fatal("window made before it was asked for")
	}
	popup.Show(connected(), view)
	name, rows := popup.Text()
	if name != "AirPods Pro 3" || rows != "Left 80%\nRight 75%\nCase 40%\nNoise Cancellation" {
		t.Errorf("%q %q", name, rows)
	}
	st := connected()
	st.Status.BatteryLeft = 70
	popup.Update(st, view)
	if _, rows := popup.Text(); !strings.HasPrefix(rows, "Left 70%") {
		t.Errorf("%q", rows)
	}
	select {
	case hide := <-calls:
		hide()
	case <-time.After(5 * time.Second):
		t.Fatal("the timer never fired")
	}
	if popup.Visible() {
		t.Error("still open")
	}
	popup.Show(connected(), view)
	popup.Hide()
	popup.Hide()
	if made != 1 {
		t.Errorf("%d windows", made)
	}
}

func TestPopupRowsForOverEarModels(t *testing.T) {
	st := connected()
	st.Status.BatteryRight, st.Status.BatteryCase = 255, 255
	if got := Rows(st, model.View{Model: "A3184"}); got != "Battery 80%\nNoise Cancellation" {
		t.Errorf("%q", got)
	}
}

func TestPopupWindowOnlyWithADesktopDriver(t *testing.T) {
	if PopupWindow(test.NewTempApp(t)) != nil {
		t.Error("the test driver has no splash windows")
	}
}

func TestTranslatedMenu(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(i18n.English) })
	tr, host, _ := newTray(t)
	tr.Update(connected())
	before := host.tooltips
	i18n.SetLanguage(i18n.Spanish)
	tr.Refresh()
	got := labels(tr.Menu())
	for _, want := range []string{"AirPods · Conectados", "I 80% · D 75% · Estuche 40%", "Desactivado", "Cancelación de ruido",
		"Mostrar baterías", "Desconectar AirPods", "Abrir ajustes", "Salir"} {
		if !strings.Contains(strings.Join(got, "|"), want) {
			t.Errorf("no %q in %q", want, got)
		}
	}
	if host.tooltips == before || !strings.Contains(host.tooltip, "Cancelación") {
		t.Errorf("tooltip %q", host.tooltip)
	}
	i18n.SetLanguage(i18n.English)
	tr.Refresh()
	if got := labels(tr.Menu())[0]; got != "AirPods · Connected" {
		t.Errorf("back: %q", got)
	}
}

func TestHeadlineNamesTheDongleOnlyWithSeveral(t *testing.T) {
	st := device.State{Present: true, Serial: "SIM0001"}
	if got := Headline(st, model.View{}); strings.Contains(got, "SIM0001") {
		t.Errorf("one dongle: %q", got)
	}
	st.Several = true
	if got := Headline(st, model.View{}); !strings.Contains(got, "SIM0001") {
		t.Errorf("several: %q", got)
	}
	if got := Headline(device.State{Several: true, ChooseDongle: true}, model.View{}); !strings.Contains(got, i18n.T("state.chooseDongle")) {
		t.Errorf("no choice: %q", got)
	}
}

func TestAvailabilityAnswersWithoutBlockingAndFollowsTheCheck(t *testing.T) {
	var answer atomic.Bool
	answer.Store(true)
	a := NewAvailability(answer.Load, time.Millisecond)
	if a.Available() {
		t.Error("available before any answer")
	}
	a.Start()
	defer a.Stop(time.Second)
	deadline := time.Now().Add(5 * time.Second)
	for !a.Available() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !a.Available() {
		t.Fatal("the answer never arrived")
	}
	answer.Store(false)
	for a.Available() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if a.Available() {
		t.Error("a tray that went is still reported")
	}
}

func TestAvailabilityKeepsAnsweringDuringASlowCheck(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	a := NewAvailability(func() bool {
		close(entered)
		<-release
		return true
	}, time.Hour)
	a.Start()
	<-entered
	finished := make(chan bool)
	go func() { finished <- a.Available() }()
	select {
	case got := <-finished:
		if got {
			t.Error("an answer before the check finished")
		}
	case <-time.After(5 * time.Second):
		t.Error("Available waited for the check")
	}
	// Stop gives up on a check that hangs, then finishes once it returns.
	a.Stop(time.Millisecond)
	close(release)
	select {
	case <-a.done:
	case <-time.After(5 * time.Second):
		t.Error("the goroutine did not end")
	}
}

// Before the first answer nothing is known, which is not the same as no tray.
func TestAvailabilityIsUnknownUntilTheFirstAnswerAndTellsChanges(t *testing.T) {
	answers := make(chan bool)
	told := make(chan Answer, 8)
	a := NewAvailability(func() bool { return <-answers }, time.Hour)
	a.OnChange = func(answer Answer) { told <- answer }
	if a.Answer() != Unknown {
		t.Errorf("%v before the start", a.Answer())
	}
	a.Start()
	defer a.Stop(time.Millisecond)
	if a.Answer() != Unknown || a.Available() {
		t.Errorf("%v while the first check is out", a.Answer())
	}
	next := func(check bool) {
		t.Helper()
		select {
		case answers <- check:
		case <-time.After(5 * time.Second):
			t.Fatal("no check was made")
		}
	}
	heard := func(want Answer) {
		t.Helper()
		select {
		case got := <-told:
			if got != want || a.Answer() != want {
				t.Errorf("told %v, holds %v, want %v", got, a.Answer(), want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("never told %v", want)
		}
	}
	next(false)
	heard(No)
	// The period is an hour: only asking again brings the next check.
	a.Recheck()
	next(false)
	a.Recheck()
	next(true)
	heard(Yes)
	if !a.Available() {
		t.Error("not available with a tray")
	}
	select {
	case extra := <-told:
		t.Errorf("told %v for an answer that did not change", extra)
	default:
	}
}

func TestAvailabilityStopWithoutStart(t *testing.T) {
	a := NewAvailability(func() bool { return true }, time.Hour)
	a.Stop(time.Hour)
	a.Stop(time.Hour)
}
