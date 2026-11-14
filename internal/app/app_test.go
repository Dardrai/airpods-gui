package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/disconnect"
	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/autopause"
	"github.com/Dardrai/airpods-gui/internal/choices"
	"github.com/Dardrai/airpods-gui/internal/debuglog"
	"github.com/Dardrai/airpods-gui/internal/desktopnote"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
	"github.com/Dardrai/airpods-gui/internal/icon"
	"github.com/Dardrai/airpods-gui/internal/notify"
	"github.com/Dardrai/airpods-gui/internal/tray"
)

type fakeHost struct{}

func (fakeHost) SetSystemTrayMenu(*fyne.Menu) {}
func (fakeHost) SetTrayIcon(icon.Variant)     {}
func (fakeHost) SetTrayTooltip(string)        {}

// disconnectRow finds the menu row by label.
func (r *rig) disconnectRow() *fyne.MenuItem {
	for _, item := range r.tray.Menu().Items {
		if item.Label == i18n.T("tray.disconnect") {
			return item
		}
	}
	panic("no disconnect row")
}

func fastOptions() device.Options {
	return device.Options{Idle: 5 * time.Millisecond, Pending: 2 * time.Millisecond, PendingWindow: 20 * time.Millisecond, Reopen: 2 * time.Millisecond}
}

// mainThread stands in for Fyne's main thread: everything that would run there,
// test code included, holds this lock.
type mainThread struct{ sync.Mutex }

func (m *mainThread) do(f func()) {
	m.Lock()
	defer m.Unlock()
	f()
}

type rig struct {
	*App
	dev      *sim.Device
	main     *mainThread
	lastNote string
	seen     []string
}

// notifyingApp records notifications; the test app keeps them to itself.
type notifyingApp struct {
	fyne.App
	r *rig
}

func (n notifyingApp) SendNotification(note *fyne.Notification) {
	n.r.lastNote = note.Content
	n.r.seen = append(n.r.seen, note.Content)
}

// delivered waits for the notes queued so far to go out, and returns them. The queue keeps its
// order, so a marker coming out means everything before it has. Not for inside main.do.
func (r *rig) delivered(t *testing.T) []string {
	t.Helper()
	const marker = "end of the notes so far"
	r.main.do(func() { r.notify("", marker) })
	r.eventually(t, func() bool { return r.lastNote == marker })
	var notes []string
	r.main.do(func() {
		notes = append(notes, r.seen[:len(r.seen)-1]...)
		r.seen, r.lastNote = nil, ""
	})
	return notes
}

// noService stands for a system without a sender of its own, which keeps the tests off
// the session bus and sends every note through the app above.
func noService(string, string) error { return desktopnote.ErrUnsupported }

func started(t *testing.T, cfg sim.Config) *rig {
	t.Helper()
	dev := sim.New(cfg)
	main := &mainThread{}
	r := &rig{dev: dev, main: main}
	base := test.NewTempApp(t)
	base.Preferences().SetString(i18n.Preference, i18n.English)
	a := New(Config{
		Fyne: notifyingApp{base, r}, Host: fakeHost{}, Options: fastOptions(), Do: main.do, Notify: noService,
		Choices: &choices.Store{Dir: t.TempDir()},
		Open:    func() (hid.Device, error) { return dev.Open("") },
	})
	r.App = a
	a.Start()
	t.Cleanup(func() {
		main.do(a.win.Shutdown)
		a.stopBackground()
		a.owner.Stop(shutdownWait)
	})
	r.eventually(t, func() bool { return a.state.Present })
	return r
}

// eventually evaluates cond on the main thread until it holds.
func (r *rig) eventually(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ok := false
		r.main.do(func() { ok = cond() })
		if ok {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not reached")
}

// flush is the owner barrier: it returns once every job queued so far has run and its callback
// has finished. A callback can queue the next job and the poll after each job can publish a
// state that queues one more, so it goes round more than once. Never call it from inside
// main.do, since the callbacks need that lock.
func (r *rig) flush(t *testing.T) {
	t.Helper()
	for range 3 {
		done := make(chan struct{})
		r.owner.Submit(func(*dongle.Client) error { return nil }, func(error) { close(done) })
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("the owner did not reach the barrier")
		}
	}
}

func TestMenuFollowsPoller(t *testing.T) {
	r := started(t, sim.Demo())
	r.eventually(t, func() bool { return strings.HasSuffix(r.tray.Menu().Items[0].Label, " · Connected") })
	r.dev.Update(func(c *sim.Config) { c.Flags = 0 })
	// The pair's model is remembered, so the header keeps its name while it is away.
	r.eventually(t, func() bool { return r.tray.Menu().Items[0].Label == "AirPods Pro 3 · Not connected" })
}

func TestNoiseControlRowWritesModeSetting(t *testing.T) {
	r := started(t, sim.Demo())
	r.main.do(func() { r.setMode("transparency") })
	r.eventually(t, func() bool { return r.state.Status.ANC == 3 })
	if r.dev.Writes() != 1 {
		t.Errorf("writes: %d", r.dev.Writes())
	}
}

// Older firmware gets the plain reconnect command, once.
func TestReconnectSendsOneCommandOnOldFirmware(t *testing.T) {
	r := started(t, SimConfig("old"))
	r.main.do(r.reconnect)
	r.eventually(t, func() bool { return len(r.dev.Controls()) == 1 })
	if op := r.dev.Controls()[0][7]; op != 1 {
		t.Errorf("opcode %d", op)
	}
}

// With the routing handover, Reconnect asks for the AirPods through it and waits until the
// dongle owns them; the plain command is not sent.
func TestReconnectTakesTheAirPodsOverThroughTheHandover(t *testing.T) {
	cfg := sim.Demo()
	cfg.Flags = 0
	r := started(t, cfg)
	r.eventually(t, func() bool { return r.state.Present && !r.state.Status.Connected })
	r.main.do(r.reconnect)
	r.eventually(t, func() bool { return r.state.Status.Connected })
	r.flush(t)
	if n := len(r.dev.Controls()); n != 0 {
		t.Errorf("%d control commands", n)
	}
}

func TestModeFallsBackToOpcodeWithoutTypedSettings(t *testing.T) {
	cfg := sim.Demo()
	cfg.Capabilities = 0x03
	r := started(t, cfg)
	r.main.do(func() { r.setMode("transparency") })
	r.eventually(t, func() bool { return r.state.Status.ANC == 3 })
	if r.dev.Writes() != 0 || len(r.dev.Controls()) != 1 || r.dev.Controls()[0][7] != 3 {
		t.Errorf("writes %d controls %d", r.dev.Writes(), len(r.dev.Controls()))
	}
}

func TestStockDisconnectShowsFallbackAndSendsNothing(t *testing.T) {
	r := started(t, SimConfig("old"))
	if !r.state.Status.Connected {
		t.Fatal("not connected")
	}
	var message string
	r.main.do(func() {
		r.win.SetInform(func(_, text string) { message = text })
		if r.disconnectRow().Disabled {
			t.Error("the row must stay enabled")
		}
		r.disconnectRow().Action()
	})
	if message != i18n.T("disconnect.fallback") || !strings.Contains(message, "0.1.59") {
		t.Errorf("dialog: %q", message)
	}
	if notes := r.delivered(t); len(notes) != 1 || notes[0] != message {
		t.Errorf("notifications: %q", notes)
	}
	r.flush(t)
	if len(r.dev.Controls()) != 0 {
		t.Errorf("sent %v", r.dev.Controls())
	}
	for _, commit := range r.dev.Commits() {
		if (commit[0] == 1 && len(commit) == 6) || commit[0] == 21 {
			t.Errorf("sent %x", commit)
		}
	}
}

// handovers lists the routing handover requests the simulator took.
func handovers(dev *sim.Device) [][]byte {
	var out [][]byte
	for _, commit := range dev.Commits() {
		if len(commit) == 10 && commit[0] == 21 {
			out = append(out, commit)
		}
	}
	return out
}

func TestSupportedDisconnectGoesThroughTheMenu(t *testing.T) {
	r := started(t, SimConfig("1"))
	r.main.do(func() {
		if r.disconnectRow().Disabled {
			t.Error("row disabled while connected")
		}
		r.win.SetInform(func(_, text string) { t.Errorf("dialog: %s", text) })
		r.disconnectRow().Action()
	})
	r.eventually(t, func() bool { return !r.state.Status.Connected && r.win.Release() == disconnect.Released })
	r.main.do(func() {
		items := r.tray.Menu().Items
		if items[0].Label != "AirPods Pro 3 · Not connected" {
			t.Errorf("header %q", items[0].Label)
		}
		if !r.disconnectRow().Disabled {
			t.Error("disconnect stays enabled with nothing to disconnect")
		}
		// The remembered model decides which mode rows show, so the row is found by name.
		reconnect := false
		for _, item := range items {
			if item.Label == "Reconnect AirPods" {
				reconnect = !item.Disabled
			}
		}
		if !reconnect {
			t.Error("no enabled Reconnect row")
		}
	})
	if sent := handovers(r.dev); len(sent) != 1 || sent[0][1] != dongle.HandoverRelease {
		t.Fatalf("handovers: %x", sent)
	}
	for _, control := range r.dev.Controls() {
		if control[7] == dongle.OpPair {
			t.Error("the pair opcode would erase the pairing")
		}
	}
	r.main.do(r.reconnect)
	r.eventually(t, func() bool { return r.state.Status.Connected })
	r.main.do(func() {
		if r.disconnectRow().Disabled {
			t.Error("row did not come back")
		}
		if r.win.Release() != 0 {
			t.Error("the reconnect ended the release")
		}
	})
}

// A release that outlasts Disconnect's own wait is shown as closing in the window, without a
// notification, and followed on the notes tick until the dongle reports it closed.
func TestPendingReleaseIsShownAndFollowedUntilClosed(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the 5 s release wait")
	}
	r := started(t, SimConfig("disconnect"))
	r.main.do(r.disconnect)
	deadline := time.Now().Add(15 * time.Second)
	for {
		pending := false
		r.main.do(func() { pending = r.win.Release() == disconnect.ReleasePending })
		if pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never pending")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if notes := r.delivered(t); len(notes) != 0 {
		t.Errorf("a release still closing is no news, got notifications: %q", notes)
	}
	r.main.do(func() {
		if !r.state.Status.Connected {
			t.Error("closed already: the simulator's delay is too short to show the pending state")
		}
	})
	for range simCloseDelay {
		r.main.do(r.followRelease)
		r.flush(t)
		done := false
		r.main.do(func() { done = r.win.Release() == disconnect.Released })
		if done {
			break
		}
	}
	r.eventually(t, func() bool { return r.win.Release() == disconnect.Released && !r.state.Status.Connected })
	if sent := handovers(r.dev); len(sent) != 1 {
		t.Errorf("handovers: %x", sent)
	}
}

func TestSupportAppearsWithoutRestart(t *testing.T) {
	r := started(t, SimConfig("old"))
	r.main.do(func() {
		if r.disconnectRow().Disabled {
			t.Error("stock row must be enabled")
		}
	})
	r.dev.Update(func(c *sim.Config) {
		c.Capabilities |= dongle.CapRouting
		c.Flags = 0
	})
	r.eventually(t, func() bool { return r.disconnectRow().Disabled })
}

func TestRefusedDisconnectNotifiesWhyAndLeavesTheAirPods(t *testing.T) {
	cfg := SimConfig("1")
	cfg.RoutingBusy = true
	r := started(t, cfg)
	r.main.do(func() { r.disconnectRow().Action() })
	r.flush(t)
	want := i18n.T("disconnect.busy")
	if notes := r.delivered(t); len(notes) != 1 || notes[0] != want {
		t.Errorf("notifications %q want %q", notes, want)
	}
	r.main.do(func() {
		if !r.state.Status.Connected {
			t.Error("a refused disconnect must not look disconnected")
		}
		if r.win.Release() != 0 {
			t.Error("nothing was released")
		}
	})
	if sent := handovers(r.dev); len(sent) != 1 {
		t.Errorf("sent %d times: a refusal is never retried", len(sent))
	}
}

func TestDisconnectProblems(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("%w (now released)", disconnect.ErrIntentChanged), i18n.T("disconnect.intentChanged")},
		{disconnect.ErrUnsupported, i18n.T("disconnect.fallback")},
		{dongle.NotSent(&dongle.RequestRejected{Code: 2}), i18n.T("device.busy")},
		{fmt.Errorf("refused: %w", &dongle.RequestRejected{Code: 2}), i18n.T("disconnect.busy")},
		{&dongle.Unconfirmed{Message: "release: not confirmed", Err: &dongle.RequestRejected{Code: 2}}, device.UnconfirmedNote()},
	} {
		if got := disconnectProblem(tc.err); got != tc.want {
			t.Errorf("%v: %q want %q", tc.err, got, tc.want)
		}
	}
}

func TestSimConfigs(t *testing.T) {
	if SimConfig("1").Capabilities&dongle.CapRouting == 0 {
		t.Error("the plain simulator must look like current firmware, with the routing handover")
	}
	if SimConfig("old").Capabilities&dongle.CapRouting != 0 {
		t.Error("the old simulator must lack the routing handover")
	}
	if SimConfig("disconnect").RoutingCloseDelay == 0 {
		t.Error("the disconnect simulator must keep a release pending for a while")
	}
	if SimOpener("") != nil || SimOpener("0") != nil || SimOpener("1") == nil {
		t.Error("simulator selection")
	}
	if _, err := SimOpener("disconnect")(); err != nil {
		t.Error(err)
	}
}

func TestStockDisconnectWithoutDongleStillExplains(t *testing.T) {
	main := &mainThread{}
	a := New(Config{
		Fyne: test.NewTempApp(t), Host: fakeHost{}, Options: fastOptions(), Do: main.do, Notify: noService,
		Open: func() (hid.Device, error) { return nil, &hid.MatchError{} },
	})
	t.Cleanup(func() { main.do(a.win.Shutdown) })
	var message string
	main.do(func() {
		a.win.SetInform(func(_, text string) { message = text })
		a.disconnect()
	})
	if message == "" {
		t.Error("no message")
	}
}

func TestMetadataIsReadOncePerConnection(t *testing.T) {
	r := started(t, sim.Demo())
	r.eventually(t, func() bool { return r.info.current.Model == "A3063" })
	cached := func() int {
		count := 0
		for _, commit := range r.dev.Commits() {
			if len(commit) == 1 && commit[0] == dongle.SlotMetadata {
				count++
			}
		}
		return count
	}
	r.flush(t)
	if cached() != 1 {
		t.Fatalf("read %d times", cached())
	}
	r.dev.BumpGeneration()
	r.eventually(t, func() bool { return cached() == 2 })
}

func TestMetadataFailureIsQuietAndRetriedSlowly(t *testing.T) {
	cfg := sim.Demo()
	cfg.InitialPhase = 1 // another client holds the channel
	r := started(t, cfg)
	r.main.do(func() { r.info.now = func() time.Time { return time.Unix(1000, 0) } })
	r.flush(t)
	r.main.do(func() {
		if r.lastNote != "" {
			t.Errorf("notified: %q", r.lastNote)
		}
		if r.info.current.Model != "" {
			t.Errorf("info: %+v", r.info.current)
		}
	})
	if len(r.dev.Commits()) != 0 {
		t.Error("sent a request on a busy channel")
	}
}

type noMedia struct{}

func (noMedia) PauseActive() (autopause.Paused, error) { return nil, nil }

func TestAutoPauseChoiceIsSaved(t *testing.T) {
	app := test.NewTempApp(t)
	open := func() *App {
		a := New(Config{Fyne: app, Host: fakeHost{}, Options: fastOptions(), Do: (&mainThread{}).do,
			Media: noMedia{}, Open: func() (hid.Device, error) { return nil, device.ErrNoDongle }})
		t.Cleanup(a.win.Shutdown)
		return a
	}
	a := open()
	if a.AutoPauseEnabled() {
		t.Error("auto-pause must start off: dongle firmware 0.1.65 and newer pause by themselves")
	}
	a.SetAutoPauseEnabled(true)
	if !a.AutoPauseEnabled() || !app.Preferences().Bool(autopause.Preference) {
		t.Error("not saved")
	}
	if again := open(); !again.AutoPauseEnabled() {
		t.Error("a saved choice to keep it on must survive the new default")
	}
	a.SetAutoPauseEnabled(false)
	if again := open(); again.AutoPauseEnabled() {
		t.Error("not read back")
	}
}

// An install from before the default changed that saved its choice keeps it.
func TestAnEarlierSavedAutoPauseChoiceIsKept(t *testing.T) {
	app := test.NewTempApp(t)
	app.Preferences().SetBool(autopause.Preference, true)
	a := New(Config{Fyne: app, Host: fakeHost{}, Options: fastOptions(), Do: (&mainThread{}).do,
		Media: noMedia{}, Open: func() (hid.Device, error) { return nil, device.ErrNoDongle }})
	t.Cleanup(a.win.Shutdown)
	if !a.AutoPauseEnabled() {
		t.Error("the saved choice was overridden")
	}
}

func TestNotesRespectTheSwitch(t *testing.T) {
	r := started(t, sim.Demo())
	var now time.Time
	var off device.State
	r.main.do(func() {
		now = time.Now()
		r.info.now = func() time.Time { return now }
		r.watcher = notify.Watcher{}
		r.showNotes(r.state)
		now = now.Add(time.Minute)
		r.showNotes(r.state)
	})
	r.delivered(t)
	r.main.do(func() {
		off = r.state
		off.Status.Connected = false
		r.SetNotificationsEnabled(false)
		if r.NotificationsEnabled() {
			t.Error("not saved")
		}
		r.showNotes(off)
		now = now.Add(time.Minute)
		r.showNotes(off)
	})
	if notes := r.delivered(t); len(notes) != 0 {
		t.Errorf("notes while off: %q", notes)
	}
	r.main.do(func() {
		r.SetNotificationsEnabled(true)
		on := off
		on.Status.Connected = true
		r.showNotes(on)
		now = now.Add(time.Minute)
		r.showNotes(on)
	})
	if notes := r.delivered(t); len(notes) != 1 || notes[0] != "Connected" {
		t.Errorf("notes: %q", notes)
	}
}

func TestShowBatteriesFallsBackToANotification(t *testing.T) {
	r := started(t, sim.Demo())
	r.eventually(t, func() bool { return r.state.Connected() })
	r.main.do(func() {
		if r.popup != nil {
			t.Fatal("the test driver has no popup window")
		}
		r.showBatteries()
	})
	if notes := r.delivered(t); len(notes) != 1 || !strings.Contains(notes[0], "Left") {
		t.Errorf("notes: %q", notes)
	}
}

func TestLanguageSwitchIsSavedAndReachesTheMenu(t *testing.T) {
	r := started(t, sim.Demo())
	t.Cleanup(func() { r.main.do(func() { i18n.SetLanguage(i18n.English) }) })
	r.eventually(t, func() bool { return r.state.Connected() })
	r.main.do(func() {
		r.SetLanguage(i18n.French)
		if r.Language() != i18n.French || r.fyne.Preferences().String(i18n.Preference) != i18n.French {
			t.Error("not saved")
		}
		items := r.tray.Menu().Items
		if last := items[len(items)-1]; last.Label != "Quitter" {
			t.Errorf("quit row: %q", last.Label)
		}
		if !strings.Contains(items[0].Label, "Connectés") {
			t.Errorf("header: %q", items[0].Label)
		}
		r.SetLanguage(i18n.English)
		if last := items[len(items)-1]; last.Label != "Quit" {
			t.Errorf("quit row: %q", last.Label)
		}
	})
}

// findButton walks the window for a button by its text, as a user would look for it.
func findButton(root fyne.CanvasObject, label string) *widget.Button {
	if button, ok := root.(*widget.Button); ok && button.Text == label {
		return button
	}
	var children []fyne.CanvasObject
	switch item := root.(type) {
	case *fyne.Container:
		children = item.Objects
	case fyne.Widget:
		children = test.WidgetRenderer(item).Objects()
	}
	for _, child := range children {
		if found := findButton(child, label); found != nil {
			return found
		}
	}
	return nil
}

func TestDisconnectOnThePageShowsTheFallback(t *testing.T) {
	r := started(t, SimConfig("old"))
	r.eventually(t, func() bool { return r.state.Status.Connected })
	var message string
	before := 0
	r.main.do(func() {
		before = r.dev.Writes()
		r.win.SetInform(func(_, text string) { message = text })
		button := findButton(r.win.Canvas().Content(), "Disconnect")
		if button == nil || button.Disabled() {
			t.Fatalf("button %v", button)
		}
		test.Tap(button)
		if message != i18n.T("disconnect.fallback") {
			t.Errorf("message %q", message)
		}
	})
	if notes := r.delivered(t); len(notes) == 0 {
		t.Error("no notification")
	}
	if after := r.dev.Writes(); after != before {
		t.Errorf("%d writes", after-before)
	}
}

func TestAutoPauseFollowsTheDemoSlot(t *testing.T) {
	if cfg := SimConfig("1"); cfg.Cached[dongle.SlotEarStatus].Packet == nil {
		t.Error("the demo has no ear status for auto-pause")
	}
}

func TestSimPickerOffersTwoAndOneThatCannotOpen(t *testing.T) {
	if SimPicker("1") != nil {
		t.Error("a plain simulator has no picker")
	}
	picker := SimPicker("several")
	list, err := picker.List()
	if err != nil || len(list) != 3 || list[2].Err == nil || list[2].Openable {
		t.Fatalf("%+v %v", list, err)
	}
	if _, err := picker.Open(); !errors.Is(err, device.ErrChooseDongle) {
		t.Errorf("no choice yet: %v", err)
	}
	picker.Choose("SIM0002")
	dev, err := picker.Open()
	if err != nil || dev.Serial() != "SIM0002" {
		t.Errorf("%v %v", dev, err)
	}
}

func TestWantedDongle(t *testing.T) {
	prefs := test.NewTempApp(t).Preferences()
	if serial, required := WantedDongle(prefs, ""); serial != "" || required {
		t.Errorf("%q %v", serial, required)
	}
	prefs.SetString(donglePreference, "SIM0001")
	if serial, required := WantedDongle(prefs, ""); serial != "SIM0001" || required {
		t.Errorf("%q %v", serial, required)
	}
	if serial, required := WantedDongle(prefs, "SIM0002"); serial != "SIM0002" || !required {
		t.Errorf("%q %v", serial, required)
	}
}

func allowOffSends(dev *sim.Device) (count int, last byte) {
	for _, commit := range dev.Commits() {
		if len(commit) == 6 && commit[0] == 1 && commit[1] == model.ControlAllowOff {
			count++
			last = commit[2]
		}
	}
	return count, last
}

// Pro 3 never reports Allow Off, so the remembered choice goes out on every connection.
func TestRememberedAllowOffIsSentOncePerConnection(t *testing.T) {
	cfg := SimConfig("1")
	cfg.Settings = []sim.Setting{{Control: 0x0d, Age: 1, Revision: 1, Value: 2}}
	cfg.WriteMode = sim.WriteLost // Accepted and never reported, as on real Pro 3.
	store := &choices.Store{Dir: t.TempDir()}
	if err := store.SaveAllowOff([6]byte(cfg.Peer), true); err != nil {
		t.Fatal(err)
	}
	dev := sim.New(cfg)
	main := &mainThread{}
	r := &rig{dev: dev, main: main}
	base := test.NewTempApp(t)
	base.Preferences().SetString(i18n.Preference, i18n.English)
	a := New(Config{
		Fyne: notifyingApp{base, r}, Host: fakeHost{}, Options: fastOptions(), Do: main.do, Choices: store, Notify: noService,
		Open: func() (hid.Device, error) { return dev.Open("") },
	})
	r.App = a
	a.Start()
	t.Cleanup(func() {
		main.do(a.win.Shutdown)
		a.stopBackground()
		a.owner.Stop(shutdownWait)
	})
	r.eventually(t, func() bool { count, _ := allowOffSends(dev); return count == 1 })
	if _, last := allowOffSends(dev); last != 1 {
		t.Errorf("sent %d, want on", last)
	}
	r.eventually(t, func() bool { return a.view().OffAvailable() })
	r.flush(t)
	if count, _ := allowOffSends(dev); count != 1 {
		t.Errorf("sent %d times on one connection", count)
	}
	dev.BumpGeneration()
	r.eventually(t, func() bool { count, _ := allowOffSends(dev); return count == 2 })
}

func TestNoAllowOffIsSentWithoutARememberedChoice(t *testing.T) {
	cfg := SimConfig("1")
	cfg.Settings = []sim.Setting{{Control: 0x0d, Age: 1, Revision: 1, Value: 2}}
	r := started(t, cfg)
	r.eventually(t, func() bool { return r.info.allowOffHandled })
	if count, _ := allowOffSends(r.dev); count != 0 {
		t.Errorf("sent %d times", count)
	}
}

// quietApp builds an app with a stubbed sender and no dongle, not started.
func quietApp(t *testing.T, send func(string, string) error, change func(*Config)) (*App, *rig) {
	t.Helper()
	main := &mainThread{}
	r := &rig{main: main}
	cfg := Config{
		Fyne: notifyingApp{test.NewTempApp(t), r}, Host: fakeHost{}, Options: fastOptions(), Do: main.do, Notify: send,
		Media: noMedia{}, Open: func() (hid.Device, error) { return nil, &hid.MatchError{} },
	}
	if change != nil {
		change(&cfg)
	}
	a := New(cfg)
	r.App = a
	t.Cleanup(func() {
		main.do(a.win.Shutdown)
		a.stopBackground()
		a.owner.Stop(shutdownWait)
	})
	return a, r
}

// A notification service that hangs must not hold the main thread, and quitting ends the
// goroutine once its call returns.
func TestNotesLeaveTheMainThreadAndStopOnQuit(t *testing.T) {
	entered := make(chan struct{}, noteQueue*2)
	release := make(chan struct{})
	a, r := quietApp(t, func(string, string) error {
		entered <- struct{}{}
		<-release
		return nil
	}, nil)
	a.Start()
	returned := make(chan struct{})
	go func() {
		// More than the queue holds, with the first one stuck in the service.
		for range noteQueue * 2 {
			r.main.do(func() { a.notify("title", "text") })
		}
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("notify waited for the service")
	}
	<-entered
	close(release)
	a.stopBackground()
	select {
	case <-a.notesDone:
	default:
		t.Error("the notes goroutine outlived the stop")
	}
}

func TestStopDoesNotWaitForNotesThatNeverStarted(t *testing.T) {
	a, _ := quietApp(t, noService, nil)
	began := time.Now()
	a.stopBackground()
	if time.Since(began) > noteStopWait/2 {
		t.Error("waited for a goroutine that was never started")
	}
}

func TestNotesFallBackThroughTheMainThreadWhereThereIsNoSender(t *testing.T) {
	log := debuglog.New(nil)
	a, r := quietApp(t, noService, func(cfg *Config) { cfg.Log = log })
	a.Start()
	r.main.do(func() {
		a.notify("title", "one")
		a.notify("title", "two")
	})
	if notes := r.delivered(t); len(notes) != 2 || notes[0] != "one" || notes[1] != "two" {
		t.Errorf("notes: %q", notes)
	}
	if strings.Contains(log.Text(), "desktop notification failed") {
		t.Errorf("no sender is no failure:\n%s", log.Text())
	}
}

// A service that fails or times out must not be asked again through Fyne, which would block
// the main thread on the same bus and leave a second note that never expires.
func TestFailedNoteIsLoggedOnceAndDropped(t *testing.T) {
	log := debuglog.New(nil)
	var mu sync.Mutex
	failing, sent := true, 0
	a, r := quietApp(t, func(string, string) error {
		mu.Lock()
		defer mu.Unlock()
		sent++
		if failing {
			return fmt.Errorf("notify: %w", context.DeadlineExceeded)
		}
		return desktopnote.ErrUnsupported
	}, func(cfg *Config) { cfg.Log = log })
	a.Start()
	r.main.do(func() {
		a.notify("title", "one")
		a.notify("title", "two")
	})
	r.eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return sent == 2
	})
	mu.Lock()
	failing = false
	mu.Unlock()
	// Everything queued before the marker has been through the sender by the time it is out.
	if notes := r.delivered(t); len(notes) != 0 {
		t.Errorf("notes that reached Fyne: %q", notes)
	}
	if got := strings.Count(log.Text(), "desktop notification failed"); got != 1 {
		t.Errorf("logged %d times:\n%s", got, log.Text())
	}
}

func TestClosingTheWindowUsesTheCachedTrayAnswer(t *testing.T) {
	checked := make(chan struct{}, 1)
	release := make(chan struct{})
	defer close(release)
	a, r := quietApp(t, noService, func(cfg *Config) {
		cfg.TrayAvailable = func() bool {
			select {
			case checked <- struct{}{}:
			default:
			}
			<-release
			return true
		}
	})
	a.Start()
	<-checked
	// The check is stuck on the bus, and the window can still ask at once.
	done := make(chan tray.Answer, 1)
	r.main.do(func() { done <- a.trayCheck.Answer() })
	if got := <-done; got != tray.Unknown {
		t.Errorf("answer %v before the check finished", got)
	}
}

// Closed while the tray check hangs, the window hides and the app stays; when the answer is
// that there is no tray, the app quits as that close would have with the answer known.
func TestWindowClosedBeforeTheTrayAnswerQuitsWithoutATray(t *testing.T) {
	checked := make(chan struct{}, 1)
	release := make(chan struct{})
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	defer free()
	a, r := quietApp(t, noService, func(cfg *Config) {
		cfg.TrayAvailable = func() bool {
			select {
			case checked <- struct{}{}:
			default:
			}
			<-release
			return false
		}
	})
	a.Start()
	<-checked
	r.main.do(func() {
		a.win.Show()
		a.win.CloseRequested()
		if a.win.Visible() {
			t.Error("the window stayed open")
		}
	})
	select {
	case <-a.stopped:
		t.Fatal("the app quit on a guess")
	default:
	}
	free()
	select {
	case <-a.stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("the app stayed, hidden, with no tray to reach it from")
	}
	if a.win.Visible() {
		t.Error("the window came back")
	}
}

// metadataRig starts an app whose dongle has no metadata yet, on a clock the test moves. The
// notes tick is fast, so that a retry depends only on the clock.
func metadataRig(t *testing.T, change func(*sim.Config)) (*rig, *time.Time, sim.CachedReport, *debuglog.Log) {
	t.Helper()
	cfg := sim.Demo()
	report := cfg.Cached[dongle.SlotMetadata]
	delete(cfg.Cached, dongle.SlotMetadata)
	if change != nil {
		change(&cfg)
	}
	dev := sim.New(cfg)
	main := &mainThread{}
	r := &rig{dev: dev, main: main}
	log := debuglog.New(nil)
	base := test.NewTempApp(t)
	now := time.Unix(1000, 0)
	a := New(Config{
		Fyne: notifyingApp{base, r}, Host: fakeHost{}, Options: fastOptions(), Do: main.do, Notify: noService, Log: log,
		Choices: &choices.Store{Dir: t.TempDir()},
		Open:    func() (hid.Device, error) { return dev.Open("") },
	})
	a.notifyTick = time.Millisecond
	a.info.now = func() time.Time { return now }
	r.App = a
	a.Start()
	t.Cleanup(func() {
		main.do(a.win.Shutdown)
		a.stopBackground()
		a.owner.Stop(shutdownWait)
	})
	r.eventually(t, func() bool { return a.state.Ready() })
	return r, &now, report, log
}

func TestMetadataRetryFollowsTheBackoffFromTheTick(t *testing.T) {
	r, now, _, _ := metadataRig(t, nil)
	r.eventually(t, func() bool { return r.info.failures == 1 && !r.info.reading })
	// Ticks go by; the clock has not reached the next try.
	r.flush(t)
	r.main.do(func() {
		if r.info.failures != 1 {
			t.Errorf("tried early: %d failures", r.info.failures)
		}
		*now = now.Add(time.Second)
	})
	r.eventually(t, func() bool { return r.info.failures == 2 && !r.info.reading })
	r.main.do(func() {
		if got := r.info.nextTry.Sub(*now); got != 2*time.Second {
			t.Errorf("next try in %v", got)
		}
		*now = now.Add(time.Second)
	})
	r.flush(t)
	r.main.do(func() {
		if r.info.failures != 2 {
			t.Errorf("tried a second early: %d failures", r.info.failures)
		}
	})
}

func TestMetadataArrivingLaterIsFoundWithNoStateChange(t *testing.T) {
	r, now, report, _ := metadataRig(t, nil)
	r.eventually(t, func() bool { return r.info.failures >= 1 && !r.info.reading })
	r.dev.Update(func(c *sim.Config) { c.Cached[dongle.SlotMetadata] = report })
	r.main.do(func() { *now = now.Add(time.Minute) })
	r.eventually(t, func() bool { return r.info.current.Model == "A3063" })
	r.main.do(func() {
		if r.info.failures != 0 || !r.info.nextTry.IsZero() {
			t.Errorf("failures %d next %v", r.info.failures, r.info.nextTry)
		}
	})
}

func TestStatusMovingCutsTheMetadataWaitShort(t *testing.T) {
	r, _, report, _ := metadataRig(t, nil)
	r.eventually(t, func() bool { return r.info.failures >= 1 && !r.info.reading })
	r.dev.Update(func(c *sim.Config) {
		c.Cached[dongle.SlotMetadata] = report
		c.ANC = 3
	})
	r.eventually(t, func() bool { return r.info.current.Model == "A3063" })
}

// The data arrives after half a minute of failed tries, and the metadata a moment after it.
func TestStatusMovingStartsTheMetadataBackoffOver(t *testing.T) {
	r, now, _, _ := metadataRig(t, nil)
	for want := 1; want <= 4; want++ {
		r.eventually(t, func() bool { return r.info.failures == want && !r.info.reading })
		r.main.do(func() { *now = now.Add(time.Minute) })
	}
	r.eventually(t, func() bool { return r.info.failures == 5 && !r.info.reading })
	r.main.do(func() {
		if got := r.info.nextTry.Sub(*now); got <= device.ReadBackoff(1) {
			t.Fatalf("the backoff did not grow: next try in %v", got)
		}
	})
	// The status moves and the metadata is not there yet: one more try, counted from the start.
	r.dev.Update(func(c *sim.Config) { c.ANC = 3 })
	r.eventually(t, func() bool { return r.state.Status.ANC == 3 && r.info.failures == 1 && !r.info.reading })
	r.main.do(func() {
		if got := r.info.nextTry.Sub(*now); got != device.ReadBackoff(1) {
			t.Errorf("next try in %v after the status moved", got)
		}
	})
}

func TestMetadataBackoffStartsOverOnANewConnection(t *testing.T) {
	r, now, _, _ := metadataRig(t, nil)
	r.eventually(t, func() bool { return r.info.failures == 1 && !r.info.reading })
	r.main.do(func() { *now = now.Add(time.Minute) })
	r.eventually(t, func() bool { return r.info.failures == 2 && !r.info.reading })
	r.dev.BumpGeneration()
	r.eventually(t, func() bool { return r.info.tryGeneration == r.state.Status.Generation && r.info.failures == 1 })
}

func TestFirstMetadataFailureIsLoggedOncePerConnection(t *testing.T) {
	// A failing read in every connection; a busy channel would not do, since a request a
	// connection left on it is dropped once that connection is over.
	r, now, _, log := metadataRig(t, func(c *sim.Config) { c.Fail = map[byte]byte{0: 6} })
	count := func() int { return strings.Count(log.Text(), "metadata read failed") }
	for range 3 {
		before := 0
		r.main.do(func() {
			before = r.info.failures
			*now = now.Add(time.Minute)
		})
		r.eventually(t, func() bool { return r.info.failures > before && !r.info.reading })
	}
	if got := count(); got != 1 {
		t.Errorf("logged %d times", got)
	}
	r.dev.BumpGeneration()
	r.main.do(func() { *now = now.Add(time.Minute) })
	r.eventually(t, func() bool { return count() == 2 })
}

// Metadata read while another pair took over must not land under the first pair's key.
func TestMetadataIsNotSavedForAPairThatLeft(t *testing.T) {
	r, _, report, _ := metadataRig(t, nil)
	r.eventually(t, func() bool { return r.info.failures >= 1 && !r.info.reading })
	first := [6]byte{}
	r.main.do(func() {
		first = r.state.Status.Peer
		r.info.nextTry = time.Time{}
		r.dev.Update(func(c *sim.Config) {
			c.Cached[dongle.SlotMetadata] = report
			c.Peer = sim.Hex{9, 9, 9, 9, 9, 9}
		})
		r.refreshInfo(r.state) // still the first pair, as far as the main thread knows
	})
	r.flush(t)
	r.eventually(t, func() bool { return r.state.Status.Peer != first && r.info.have })
	key, err := r.choices.PairKey(first)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.fyne.Preferences().String(infoPreference + key); got != "" {
		t.Errorf("saved %q under the pair that left", got)
	}
}

// infoKeys lists the pair entries in the preferences.
func infoKeys(prefs fyne.Preferences) []string {
	var keys []string
	prefs.(preferenceValues).ReadValues(func(all map[string]any) {
		for key := range all {
			if strings.HasPrefix(key, infoPreference) {
				keys = append(keys, key)
			}
		}
	})
	return keys
}

func TestEarlierPreferenceKeysMoveOverOnFirstRead(t *testing.T) {
	cfg := sim.Demo()
	cfg.Flags = 0 // Away, so that the page goes by the remembered model.
	seen := fmt.Sprintf("%x", []byte(cfg.Peer))
	other := "0a0b0c0d0e0f"
	base := test.NewTempApp(t)
	prefs := base.Preferences()
	prefs.SetString(infoPreference+seen, "A3063|9A348")
	prefs.SetString(infoPreference+other, "A3184|7A1")
	dev := sim.New(cfg)
	main := &mainThread{}
	r := &rig{dev: dev, main: main}
	store := &choices.Store{Dir: t.TempDir()}
	a := New(Config{
		Fyne: notifyingApp{base, r}, Host: fakeHost{}, Options: fastOptions(), Do: main.do, Notify: noService, Choices: store,
		Open: func() (hid.Device, error) { return dev.Open("") },
	})
	r.App = a
	a.Start()
	t.Cleanup(func() {
		main.do(a.win.Shutdown)
		a.stopBackground()
		a.owner.Stop(shutdownWait)
	})
	r.eventually(t, func() bool { return a.info.current.Model == "A3063" })
	if got := a.info.current.Firmware; got != "9A348" {
		t.Errorf("firmware %q", got)
	}
	keys := infoKeys(prefs)
	if len(keys) != 2 {
		t.Errorf("keys %q", keys)
	}
	for _, key := range keys {
		if strings.Contains(key, seen) || strings.Contains(key, other) {
			t.Errorf("%s still shows the address", key)
		}
	}
	// The pair not connected now kept its entry too.
	otherKey, err := store.PairKey([6]byte{0xa, 0xb, 0xc, 0xd, 0xe, 0xf})
	if err != nil {
		t.Fatal(err)
	}
	if got := prefs.String(infoPreference + otherKey); got != "A3184|7A1" {
		t.Errorf("the other pair has %q", got)
	}
}

// The move happens at startup, for every pair, with no dongle and no pair in sight.
func TestEarlierKeysAndFilesMoveAtStartupWithoutAPair(t *testing.T) {
	base := test.NewTempApp(t)
	prefs := base.Preferences()
	addresses := []string{"010203040506", "0a0b0c0d0e0f"}
	store := &choices.Store{Dir: t.TempDir()}
	for i, address := range addresses {
		prefs.SetString(infoPreference+address, []string{"A3063|9A348", "A3184|7A1"}[i])
		if err := os.WriteFile(filepath.Join(store.Dir, "allow-off-"+address), []byte("true"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	quietApp(t, noService, func(cfg *Config) {
		cfg.Fyne = base
		cfg.Choices = store
	})
	check := func() {
		t.Helper()
		keys := infoKeys(prefs)
		if len(keys) != 2 {
			t.Errorf("keys %q", keys)
		}
		entries, err := os.ReadDir(store.Dir)
		if err != nil || len(entries) != 3 { // two pairs and the salt
			t.Errorf("%d files (%v)", len(entries), err)
		}
		for _, address := range addresses {
			for _, key := range keys {
				if strings.Contains(key, address) {
					t.Errorf("%s still shows the address", key)
				}
			}
			for _, entry := range entries {
				if strings.Contains(entry.Name(), address) {
					t.Errorf("%s still shows the address", entry.Name())
				}
			}
		}
		for peer, want := range map[[6]byte]string{{1, 2, 3, 4, 5, 6}: "A3063|9A348", {0xa, 0xb, 0xc, 0xd, 0xe, 0xf}: "A3184|7A1"} {
			key, err := store.PairKey(peer)
			if err != nil {
				t.Fatal(err)
			}
			if got := prefs.String(infoPreference + key); got != want {
				t.Errorf("%x has %q", peer, got)
			}
			if on := store.LoadAllowOff(peer); on == nil || !*on {
				t.Errorf("%x lost its allow off", peer)
			}
		}
	}
	check()
	// The next start finds nothing left to move and changes nothing.
	quietApp(t, noService, func(cfg *Config) {
		cfg.Fyne = base
		cfg.Choices = &choices.Store{Dir: store.Dir}
	})
	check()
}

func TestNewInfoIsKeptUnderTheHash(t *testing.T) {
	r := started(t, sim.Demo())
	r.eventually(t, func() bool { return r.info.have })
	peer := ""
	r.main.do(func() { peer = fmt.Sprintf("%x", r.state.Status.Peer) })
	keys := infoKeys(r.fyne.Preferences())
	if len(keys) != 1 || strings.Contains(keys[0], peer) {
		t.Errorf("keys %q", keys)
	}
}

func TestWithoutAStoreNoPairIsRemembered(t *testing.T) {
	cfg := sim.Demo()
	dev := sim.New(cfg)
	main := &mainThread{}
	base := test.NewTempApp(t)
	r := &rig{dev: dev, main: main}
	a := New(Config{
		Fyne: notifyingApp{base, r}, Host: fakeHost{}, Options: fastOptions(), Do: main.do, Notify: noService,
		Open: func() (hid.Device, error) { return dev.Open("") },
	})
	r.App = a
	a.Start()
	t.Cleanup(func() {
		main.do(a.win.Shutdown)
		a.stopBackground()
		a.owner.Stop(shutdownWait)
	})
	r.eventually(t, func() bool { return a.info.have })
	if keys := infoKeys(base.Preferences()); len(keys) != 0 {
		t.Errorf("keys %q", keys)
	}
}
