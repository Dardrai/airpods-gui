// Package app wires the device owner, the tray and the window together.
package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"

	"github.com/Dardrai/airpods-gui/core/disconnect"
	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/autopause"
	"github.com/Dardrai/airpods-gui/internal/autostart"
	"github.com/Dardrai/airpods-gui/internal/choices"
	"github.com/Dardrai/airpods-gui/internal/debuglog"
	"github.com/Dardrai/airpods-gui/internal/desktopnote"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
	"github.com/Dardrai/airpods-gui/internal/notify"
	"github.com/Dardrai/airpods-gui/internal/tray"
	"github.com/Dardrai/airpods-gui/internal/ui"
	"github.com/Dardrai/airpods-gui/internal/updates"
)

const (
	shutdownWait  = 5 * time.Second
	pauseStopWait = time.Second
	notifyPeriod  = 2 * time.Second
	// noteQueue is how many notes can wait for a slow notification service; past that the
	// oldest goes, since a newer note says more about the present.
	noteQueue = 8
	// noteStopWait outlasts the notification call's own timeout.
	noteStopWait = 3 * time.Second
	// trayPeriod is how often the tray's presence is checked; it decides only what closing
	// the window does.
	trayPeriod = 5 * time.Second
)

// Config is what the app is built from.
type Config struct {
	// UpdateFetch and FirmwareFetch replace the calls to GitHub, for tests; nil asks GitHub.
	UpdateFetch   func(context.Context) (updates.Release, error)
	FirmwareFetch func(context.Context) (updates.Release, error)

	Fyne    fyne.App
	Host    tray.Host
	Open    device.OpenFunc
	Options device.Options
	// Do runs a function on the UI thread; nil means fyne.Do.
	Do func(func())
	// Version is shown in the window.
	Version string
	// TrayAvailable decides whether closing the window can hide it to a tray. It may block:
	// it runs off the main thread and the window sees the last answer, or that there is
	// none yet.
	TrayAvailable func() bool
	// Autostart is the start-with-OS entry; nil hides the option.
	Autostart autostart.Manager
	// Log is the debug log; nil keeps nothing.
	Log *debuglog.Log
	// Choices remembers per-pair values, and names the pairs for the model kept in the
	// preferences; nil keeps nothing.
	Choices *choices.Store
	// Tick overrides the period of the visible-window refresh.
	Tick time.Duration
	// Media is what auto-pause controls; nil means the system's players.
	Media autopause.Media
	// Picker chooses between several dongles; nil means Open is the only way to one.
	Picker *device.Picker
	// Notify shows a notification; nil means the desktop's own service. It may block: it
	// runs on the notes goroutine. An error sends the note through Fyne instead.
	Notify func(title, text string) error
}

// dongleListPeriod is how often the attached dongles are listed while one is open.
const dongleListPeriod = 3 * time.Second

// donglePreference remembers the chosen dongle by its USB serial.
const donglePreference = "dongleSerial"

// WantedDongle is the serial to prefer and whether it is required: the flag's, which is
// required, or else the remembered one, which is not.
func WantedDongle(prefs fyne.Preferences, flagSerial string) (string, bool) {
	if flagSerial != "" {
		return flagSerial, true
	}
	return prefs.String(donglePreference), false
}

// App is the running application. Its methods run on the Fyne main thread.
type App struct {
	fyne  fyne.App
	send  func(title, text string) error
	do    func(func())
	log   *debuglog.Log
	owner *device.Owner
	tray  *tray.Tray
	win   *ui.Window
	state device.State
	info  infoState
	// choices names the pairs in what is stored; nil keeps nothing about them.
	choices *choices.Store
	pause   *autopause.Controller
	popup   *tray.Popup

	// notes carries what notify was given to the goroutine that talks to the desktop.
	notes        chan desktopNote
	notesDone    chan struct{}
	notesRunning atomic.Bool
	trayCheck    *tray.Availability

	// releaseReading is set while followRelease has a read out.
	releaseReading bool

	// updates asks GitHub for a newer release, every two hours, only while turned on;
	// firmwareUpdates does the same for the dongle firmware.
	updates         *updates.Checker
	firmwareUpdates *updates.Checker
	firmware        firmwareState
	installing      atomic.Bool

	watcher    notify.Watcher
	stopped    chan struct{}
	stopOnce   sync.Once
	quitOnce   sync.Once
	startOnce  sync.Once
	notifyTick time.Duration
}

// New builds the app; the device is not touched until Start. What earlier versions stored
// under Bluetooth addresses is moved here, before anything reads it.
func New(cfg Config) *App {
	a := &App{
		fyne: cfg.Fyne, do: cfg.Do, log: cfg.Log, choices: cfg.Choices, stopped: make(chan struct{}), notifyTick: notifyPeriod,
		notes: make(chan desktopNote, noteQueue), notesDone: make(chan struct{}),
	}
	if a.do == nil {
		a.do = fyne.Do
	}
	if a.send = cfg.Notify; a.send == nil {
		a.send = desktopnote.Send
	}
	i18n.SetLanguage(cfg.Fyne.Preferences().StringWithFallback(i18n.Preference, i18n.System))
	a.owner = device.NewOwner(cfg.Open, cfg.Options, func(st device.State) {
		a.do(func() { a.apply(st) })
	})
	a.owner.SetLog(cfg.Log.Printf, cfg.Log.Protect)
	if cfg.Picker != nil {
		a.owner.SetPicker(cfg.Picker, dongleListPeriod, func(list []hid.Info) {
			a.do(func() { a.win.SetDongles(list) })
		})
	}
	ui.ApplyTheme(cfg.Fyne)
	media := cfg.Media
	if media == nil {
		media = autopause.SystemMedia()
	}
	a.pause = autopause.New(autopause.Options{
		Owner: a.owner, Media: media, Log: cfg.Log.Printf, Protect: cfg.Log.Protect,
		Enabled: cfg.Fyne.Preferences().BoolWithFallback(autopause.Preference, autopause.Default),
	})
	var trayAnswer func() tray.Answer
	var trayRecheck func()
	if cfg.TrayAvailable != nil {
		a.trayCheck = tray.NewAvailability(cfg.TrayAvailable, trayPeriod)
		// Off the checking goroutine, which a stop on the main thread waits for.
		a.trayCheck.OnChange = func(answer tray.Answer) {
			go a.do(func() { a.win.TrayAnswered(answer) })
		}
		trayAnswer, trayRecheck = a.trayCheck.Answer, a.trayCheck.Recheck
	}
	a.updates = &updates.Checker{
		Prefs: cfg.Fyne.Preferences(), Running: cfg.Version, Logf: cfg.Log.Printf,
		OnResult: func(r updates.Result) { a.do(func() { a.win.SetUpdate(r) }) },
		OnNew:    func(r updates.Release) { a.do(func() { a.announceUpdate(r) }) },
	}
	a.firmwareUpdates = &updates.Checker{
		Prefs: cfg.Fyne.Preferences(), Repo: FirmwareRepo, Prefix: firmwarePrefix, Logf: cfg.Log.Printf,
		OnResult: func(r updates.Result) {
			a.do(func() {
				a.win.SetFirmwareUpdate(r)
				a.announceFirmware()
			})
		},
	}
	if cfg.UpdateFetch != nil {
		a.updates.SetFetch(cfg.UpdateFetch)
	}
	if cfg.FirmwareFetch != nil {
		a.firmwareUpdates.SetFetch(cfg.FirmwareFetch)
	}
	a.win = ui.New(ui.Options{
		Fyne: cfg.Fyne, Owner: a.owner, Do: a.do, Version: cfg.Version,
		Quit: a.quit, Disconnect: a.disconnect, Tray: trayAnswer, TrayRecheck: trayRecheck, Tick: cfg.Tick, Autostart: cfg.Autostart,
		Log: cfg.Log, Choices: cfg.Choices, Picker: cfg.Picker,
		OnReports: func(reports map[byte]uint32) { a.setReports(reports) },
		Controls: &ui.AppControls{
			AutoPauseEnabled: a.AutoPauseEnabled, SetAutoPauseEnabled: a.SetAutoPauseEnabled,
			NotificationsEnabled: a.NotificationsEnabled, SetNotificationsEnabled: a.SetNotificationsEnabled,
			ModeNotificationsEnabled: a.ModeNotificationsEnabled, SetModeNotificationsEnabled: a.SetModeNotificationsEnabled,
			Language: a.Language, SetLanguage: a.SetLanguage, ShowBatteries: a.showBatteries, ChooseDongle: a.chooseDongle,
			UpdatesEnabled: func() bool { return a.updates.Enabled() }, SetUpdatesEnabled: a.setUpdatesEnabled,
			CheckUpdates: func() { a.updates.CheckNow(); a.firmwareUpdates.CheckNow() }, OpenRelease: a.openRelease,
			OpenFirmwareRelease: a.openFirmwareRelease, InstallUpdate: a.installUpdate,
			CanInstallUpdate: updates.CanInstall(),
		},
	})
	// What earlier checks found, on the goroutine that built the window.
	a.updates.Load()
	a.win.SetUpdate(a.updates.Result())
	a.firmwareUpdates.Load()
	a.win.SetFirmwareUpdate(a.firmwareUpdates.Result())
	a.tray = tray.New(cfg.Host, tray.Actions{
		SetMode:       a.setMode,
		ShowBatteries: a.showBatteries,
		Disconnect:    a.disconnect,
		Reconnect:     a.reconnect,
		OpenSettings:  a.win.Show,
		Quit:          a.quit,
	})
	if create := tray.PopupWindow(cfg.Fyne); create != nil {
		a.popup = tray.NewPopup(create, a.do)
		a.popup.Content = ui.NewBatterySheet
	}
	a.migrateStored()
	return a
}

// Start begins polling.
func (a *App) Start() {
	a.owner.Start()
	a.pause.Start()
	a.startOnce.Do(func() {
		a.notesRunning.Store(true)
		go a.log.Protect(a.sendNotes)
		if a.trayCheck != nil {
			a.trayCheck.Start()
		}
		go a.log.Protect(a.watchNotes)
		a.updates.Start()
		a.firmwareUpdates.Start()
		// The icon's tint follows the system's light or dark mode.
		a.fyne.Settings().AddListener(func(fyne.Settings) { go a.do(a.tray.Reapply) })
	})
}

// watchNotes keeps time passing for the notifications and the metadata retries: with no
// dongle, or nothing changing, no state arrives to carry a settled change or to read again.
func (a *App) watchNotes() {
	ticker := time.NewTicker(a.notifyTick)
	defer ticker.Stop()
	for {
		select {
		case <-a.stopped:
			return
		case <-ticker.C:
			a.do(func() {
				a.showNotes(a.state)
				a.refreshInfo(a.state)
				a.followRelease()
			})
		}
	}
}

// showNotes runs on the main thread. Turned off, the watcher still follows along, so
// turning it on does not announce the past.
func (a *App) showNotes(st device.State) {
	notes := a.watcher.Observe(st, a.clock())
	prefs := a.fyne.Preferences()
	if !prefs.BoolWithFallback(notify.Preference, notify.Default) {
		return
	}
	for _, note := range notes {
		if note.Kind == notify.ModeChanged && !prefs.BoolWithFallback(notify.ModePreference, notify.ModeDefault) {
			continue
		}
		title, body := note.Text()
		a.notify(title, body)
	}
}

// desktopNote is one notification on its way out.
type desktopNote struct{ title, text string }

// notify queues a notification and never waits, so the main thread can call it freely.
func (a *App) notify(title, text string) {
	note := desktopNote{title, text}
	for {
		select {
		case a.notes <- note:
			return
		default:
		}
		select {
		case <-a.notes:
			a.log.Printf("a notification was dropped: the service is not keeping up")
		default:
		}
	}
}

// sendNotes shows the queued notes one at a time, under the app's name. Fyne names the sender
// by the app id, so it is only the way out where this system has no sender of its own. A
// note the desktop's service did not take is dropped: Fyne's would ask the same service
// again, from the main thread and with no timeout, and leave a second note that never expires.
func (a *App) sendNotes() {
	defer close(a.notesDone)
	failure := ""
	for {
		select {
		case <-a.stopped:
			return
		case note := <-a.notes:
			err := a.send(note.title, note.text)
			switch {
			case err == nil:
				failure = ""
			case errors.Is(err, desktopnote.ErrUnsupported):
				a.do(func() { a.fyne.SendNotification(fyne.NewNotification(note.title, note.text)) })
			default:
				if text := err.Error(); text != failure {
					failure = text
					a.log.Printf("desktop notification failed, note dropped: %v", err)
				}
			}
		}
	}
}

// Language reports the saved language choice; i18n.Choices lists the options.
func (a *App) Language() string { return i18n.Language() }

// SetLanguage changes and saves the language. Everything the app has on screen follows,
// the window through i18n.OnChange. It runs on the main thread.
func (a *App) SetLanguage(code string) {
	a.fyne.Preferences().SetString(i18n.Preference, i18n.SetLanguage(code))
	a.tray.Refresh()
	if a.popup != nil {
		a.popup.Update(a.state, a.view())
	}
}

// chooseDongle remembers the choice and moves to that dongle.
func (a *App) chooseDongle(serial string) {
	a.fyne.Preferences().SetString(donglePreference, serial)
	a.owner.Choose(serial)
}

// NotificationsEnabled reports the saved notifications choice.
func (a *App) NotificationsEnabled() bool {
	return a.fyne.Preferences().BoolWithFallback(notify.Preference, notify.Default)
}

// SetNotificationsEnabled changes and saves the notifications choice.
func (a *App) SetNotificationsEnabled(on bool) { a.fyne.Preferences().SetBool(notify.Preference, on) }

// ModeNotificationsEnabled reports the saved choice for notes about mode changes made
// elsewhere, such as on the AirPods themselves.
func (a *App) ModeNotificationsEnabled() bool {
	return a.fyne.Preferences().BoolWithFallback(notify.ModePreference, notify.ModeDefault)
}

// SetModeNotificationsEnabled changes and saves that choice.
func (a *App) SetModeNotificationsEnabled(on bool) {
	a.fyne.Preferences().SetBool(notify.ModePreference, on)
}

func (a *App) stopBackground() {
	a.stopOnce.Do(func() { close(a.stopped) })
	a.updates.Stop()
	a.firmwareUpdates.Stop()
	a.pause.Stop(pauseStopWait)
	if a.trayCheck != nil {
		a.trayCheck.Stop(pauseStopWait)
	}
	if a.notesRunning.Load() {
		select {
		case <-a.notesDone:
		case <-time.After(noteStopWait):
			a.log.Printf("the notification service did not answer before quitting")
		}
	}
}

// AutoPauseEnabled reports the saved auto-pause choice.
func (a *App) AutoPauseEnabled() bool { return a.pause.Enabled() }

// SetAutoPauseEnabled changes and saves the auto-pause choice.
func (a *App) SetAutoPauseEnabled(on bool) {
	a.fyne.Preferences().SetBool(autopause.Preference, on)
	a.pause.SetEnabled(on)
}

// Window returns the settings window.
func (a *App) Window() *ui.Window { return a.win }

// ReapplyTray sends the icon and tooltip again once the tray has had time to start.
func (a *App) ReapplyTray() { a.tray.Reapply() }

// Stop ends everything on any way out of the app, including ones that bypass the
// menu. It runs on the main thread and is safe to call more than once.
func (a *App) Stop() {
	a.win.Shutdown()
	a.hidePopup()
	a.stopBackground()
	a.owner.Stop(shutdownWait)
}

func (a *App) hidePopup() {
	if a.popup != nil {
		a.popup.Hide()
	}
}

func (a *App) apply(st device.State) {
	// A mode or capability the status gained means the AirPods just reported something, so
	// the metadata they send at the same time need not wait for the backoff.
	// The backoff starts over as well: tries made before the AirPods had anything to say
	// must not stretch the wait for a report that is now on its way.
	if st.Status.ANC != a.state.Status.ANC || st.Status.Capabilities != a.state.Status.Capabilities {
		a.info.failures, a.info.nextTry = 0, time.Time{}
	}
	a.state = st
	a.pause.SetState(st)
	a.tray.Update(st)
	a.win.Update(st)
	if a.popup != nil {
		a.popup.Update(st, a.view())
	}
	a.showNotes(st)
	a.refreshInfo(st)
	a.refreshFirmware(st)
	a.reapplyAllowOff(st)
}

// Quit ends the app as the menu's Quit does, for a request from outside. Repeats do nothing.
func (a *App) Quit() { a.quitOnce.Do(a.quit) }

func (a *App) quit() {
	a.win.Shutdown()
	a.hidePopup()
	go func() {
		a.stopBackground()
		a.owner.Stop(shutdownWait)
		a.do(a.fyne.Quit)
	}()
}

// report shows a failed tray action; the menu has no place of its own for it.
func (a *App) report(title string, err error) {
	if err == nil {
		return
	}
	if device.IsUnconfirmed(err) {
		// Not something the user can act on: the next status report shows what happened.
		a.log.Printf("%s: sent, no answer: %v", title, err)
		return
	}
	text := device.Explain(err)
	a.do(func() { a.notify(title, text) })
}

// showBatteries answers the tray's Show batteries row: the small window where the
// driver has one, a notification otherwise.
func (a *App) showBatteries() {
	if a.popup != nil {
		a.popup.Show(a.state, a.view())
		return
	}
	a.notify(tray.DeviceName(a.view()), tray.BatteryDetail(a.state))
}

func (a *App) setMode(word string) {
	// Off needs allow-off on sent in this generation first, on AirPods that never report it.
	generation := a.state.Status.Generation
	allowOffFirst := word == "off" && ui.NeedsAllowOffFirst(a.view(), model.ControlMode, 1, a.win.AllowOffOnSent(generation))
	allowOffWent := false
	a.owner.SubmitCommand(func(c *dongle.Client) error {
		var err error
		allowOffWent, err = applyMode(c, word, allowOffFirst)
		return err
	}, func(err error) {
		if allowOffWent {
			a.do(func() { a.win.AllowOffSent(generation, true) })
		}
		a.report(i18n.T("tray.noiseControl"), err)
	})
}

// reconnect takes the AirPods back through the routing handover where the dongle has it, and
// with the plain reconnect command where not. It also ends a release. The window shows how it
// went; a problem also gets a notification, since the menu has no place for it.
func (a *App) reconnect() { a.win.StartReconnect(a.notify) }

// disconnect releases the AirPods through the routing handover when the dongle has it; the
// work happens on the owner goroutine, waits up to 5 s for the connection to close, and the
// state poll then updates the menu. Without the handover nothing goes near the device. The
// window shows the outcome; a release still closing and a failure also get a notification,
// since the menu has no place for them.
func (a *App) disconnect() {
	if disconnect.Pick(nil, a.state.Status).Kind() != disconnect.KindRouting {
		a.showDisconnectFallback()
		return
	}
	var outcome disconnect.Outcome
	a.win.DisconnectSent()
	a.owner.SubmitCommand(func(c *dongle.Client) error {
		status, err := c.Status()
		if err != nil {
			return err
		}
		outcome, err = disconnect.Pick(c, status).Disconnect()
		return err
	}, func(err error) {
		a.do(func() {
			if device.IsUnconfirmed(err) {
				// No answer is not a failure: follow it like a release still closing, and let
				// the routing state and the connection notification tell how it ended.
				a.log.Printf("disconnect: sent, no answer: %v", err)
				outcome, err = disconnect.ReleasePending, nil
			}
			// Only a failure is worth a notification of its own; a release that goes through
			// ends in the Disconnected one.
			problem := ""
			if err != nil {
				problem = disconnectProblem(err)
				a.notify(i18n.T("tray.disconnect"), problem)
			}
			a.win.DisconnectDone(outcome, problem)
		})
	})
}

// disconnectProblem words a failed release. The dongle refusing as busy has its own
// reasons; a channel held by another app is the shared wording, and so is an uncertain
// outcome, which comes first since it can wrap either.
func disconnectProblem(err error) string {
	switch {
	case device.IsUnconfirmed(err):
	case errors.Is(err, disconnect.ErrIntentChanged):
		return i18n.T("disconnect.intentChanged")
	case errors.Is(err, disconnect.ErrUnsupported):
		return i18n.T("disconnect.fallback")
	case device.IsBusy(err) && !errors.Is(err, dongle.ErrNotSent):
		return i18n.T("disconnect.busy")
	}
	return device.Explain(err)
}

// followRelease reads the routing state while a release is still closing, on the notes tick:
// one typed request every few seconds, and only then. The status report shows the connection
// closing as well; this also notices a release that something else ended.
func (a *App) followRelease() {
	if a.win.Release() != disconnect.ReleasePending || a.releaseReading || !a.state.Ready() {
		return
	}
	a.releaseReading = true
	var routing dongle.Routing
	a.owner.Submit(func(c *dongle.Client) error {
		var err error
		routing, err = c.Routing()
		return err
	}, func(err error) {
		a.do(func() {
			a.releaseReading = false
			switch {
			case err != nil || a.win.Release() != disconnect.ReleasePending:
			case routing.Has(dongle.RoutingReleased):
				a.win.SetRelease(disconnect.Released)
			case !routing.Has(dongle.RoutingReleasePending):
				a.win.SetRelease(0)
			}
		})
	})
}

func (a *App) showDisconnectFallback() {
	title, text := i18n.T("tray.disconnect"), i18n.T("disconnect.fallback")
	a.notify(title, text)
	a.win.Message(title, text)
}

// applyMode prefers the confirmed mode setting; dongles without typed settings can
// still switch between the two modes the control opcode knows. With allowOffFirst it sends
// allow-off on before the mode, and reports whether that may have gone out.
func applyMode(c *dongle.Client, word string, allowOffFirst bool) (bool, error) {
	status, err := c.Status()
	if err != nil {
		return false, err
	}
	if !status.Has(dongle.CapTypedRPC) {
		switch word {
		case "anc":
			return false, c.SetListeningMode(dongle.ListeningANC)
		case "transparency":
			return false, c.SetListeningMode(dongle.ListeningTransparency)
		}
		return false, errors.New("this dongle cannot select that mode")
	}
	setting, ok := dongle.LookupSetting("mode")
	if !ok {
		return false, errors.New("unknown setting: mode")
	}
	raw, err := setting.Encode(word)
	if err != nil {
		return false, err
	}
	settings, err := c.SettingsClient()
	if err != nil {
		return false, err
	}
	allowOffWent := false
	if allowOffFirst {
		if allowOffWent, err = ui.SendAllowOffOn(settings); !allowOffWent {
			return false, err
		}
	}
	_, err = settings.Set("mode", raw)
	return allowOffWent, err
}

// announceUpdate tells about a newer release once, with the other notifications.
func (a *App) announceUpdate(r updates.Release) {
	if !a.fyne.Preferences().BoolWithFallback(notify.Preference, notify.Default) {
		return
	}
	a.notify(i18n.T("update.noteTitle"), fmt.Sprintf(i18n.T("update.noteText"), r.Version))
}

// openFirmwareRelease shows the newest firmware release's page in the browser.
func (a *App) openFirmwareRelease() {
	page := a.firmwareUpdates.Result().Latest.Page
	if page == "" {
		page = "https://github.com/" + FirmwareRepo + "/releases/latest"
	}
	if u, err := url.Parse(page); err == nil {
		if err := a.fyne.OpenURL(u); err != nil {
			a.log.Printf("opening the firmware release page: %v", err)
		}
	}
}

// openRelease shows the newest release's page in the browser.
func (a *App) openRelease() {
	page := a.updates.Result().Latest.Page
	if page == "" {
		page = "https://github.com/" + updates.Repo + "/releases/latest"
	}
	if u, err := url.Parse(page); err == nil {
		if err := a.fyne.OpenURL(u); err != nil {
			a.log.Printf("opening the release page: %v", err)
		}
	}
}

// installUpdate downloads the newest release's installer or package, checks it against the
// release's SHA256SUMS and installs it: the Windows installer closes this app and starts the
// new one; on Linux the package manager asks for the password, and the app restarts itself.
func (a *App) installUpdate() {
	r := a.updates.Result()
	installer, ok := updates.Detect()
	if !r.Available || !ok || !a.installing.CompareAndSwap(false, true) {
		return
	}
	a.win.UpdateProgress(fmt.Sprintf(i18n.T("update.downloading"), r.Latest.Version), true)
	go a.log.Protect(func() {
		dir, err := updates.DownloadDir()
		path := ""
		if err == nil {
			path, err = updates.Download(context.Background(), &http.Client{}, r.Latest, dir, installer.Asset(r.Latest.Tag))
		}
		if err == nil {
			a.log.Printf("update: %s downloaded and verified, installing with %s", filepath.Base(path), installer.Kind)
			a.do(func() { a.win.UpdateProgress(i18n.T("update.starting"), true) })
			err = installer.Run(path)
		}
		if err == nil && !installer.Restarts {
			err = updates.Relaunch()
		}
		if !installer.Restarts && dir != "" {
			os.RemoveAll(dir)
		}
		a.do(func() {
			switch {
			case errors.Is(err, updates.ErrCancelled):
				a.installing.Store(false)
				a.win.UpdateProgress("", false)
			case err != nil:
				a.installing.Store(false)
				a.log.Printf("update failed: %v", err)
				a.win.UpdateProgress(fmt.Sprintf(i18n.T("update.installFailed"), err), false)
			case !installer.Restarts:
				a.log.Printf("update installed, restarting")
				a.quit()
			}
		})
	})
}

// setUpdatesEnabled turns both checks on or off; turned on, both ask GitHub right away.
func (a *App) setUpdatesEnabled(on bool) {
	a.updates.SetEnabled(on)
	if on {
		a.firmwareUpdates.CheckNow()
	}
	a.win.SetFirmwareUpdate(a.firmwareUpdates.Result())
}
