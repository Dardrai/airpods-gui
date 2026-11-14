// Command airpods-tray is the tray app and settings window for the AirPods dongle.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/internal/app"
	"github.com/Dardrai/airpods-gui/internal/autostart"
	"github.com/Dardrai/airpods-gui/internal/choices"
	"github.com/Dardrai/airpods-gui/internal/debuglog"
	"github.com/Dardrai/airpods-gui/internal/desktopnote"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/icon"
	"github.com/Dardrai/airpods-gui/internal/instance"
	"github.com/Dardrai/airpods-gui/internal/tray"
)

// version is set at build time with -ldflags -X main.version=...
var version = ""

// quitWait covers the app's own shutdown, which waits for the device owner to stop.
const quitWait = 10 * time.Second

func displayVersion() string {
	if version == "" {
		return "development build"
	}
	return version
}

func main() {
	trayFlag := flag.Bool("tray", false, "start hidden in the system tray")
	versionFlag := flag.Bool("version", false, "print the version and exit")
	quitFlag := flag.Bool("quit", false, "ask a running instance to quit, wait for it, and exit")
	serialFlag := flag.String("serial", "", "use the dongle with this USB serial")
	flag.Parse()
	if *versionFlag {
		fmt.Println("airpods-tray", displayVersion())
		return
	}
	if *quitFlag {
		if err := quitRunning(); err != nil {
			fmt.Fprintln(os.Stderr, "airpods-tray:", err)
			os.Exit(1)
		}
		return
	}

	var echo io.Writer
	if os.Getenv("AIRPODS_GUI_DEBUG") == "1" {
		echo = os.Stderr
	}
	logger := debuglog.New(echo)
	logger.Protect(func() { run(logger, *trayFlag, *serialFlag) })
}

// run is the app, with a log that outlives it to report a panic.
func run(logger *debuglog.Log, trayFlag bool, serialFlag string) {
	// A failing guard must not keep the app from starting, so it just runs unguarded.
	var guard *instance.Guard
	path, err := instance.DefaultPath()
	if err == nil {
		message := app.InstanceMessage(trayFlag, instance.MessageShow, instance.MessageNone)
		var acquired bool
		guard, acquired, err = acquire(path, message)
		if err == nil && !acquired {
			return
		}
		if errors.Is(err, instance.ErrNoAnswer) {
			// Unlike a failing guard this is a running app, and two would fight over the dongle.
			fmt.Fprintln(os.Stderr, "airpods-tray:", err)
			os.Exit(1)
		}
	}
	if err != nil {
		logger.Printf("running without the single-instance guard: %v", err)
	}
	if guard != nil {
		defer guard.Close()
	}
	if closeLog := setUpLogFiles(logger); closeLog != nil {
		defer closeLog()
	}

	a := fyneapp.NewWithID(desktopnote.AppID)
	desk, ok := a.(desktop.App)
	if !ok {
		return
	}
	appIcon := icon.PNG(icon.Variant{Level: icon.NoBadge, Tile: true}, 256)
	a.SetIcon(fyne.NewStaticResource("airpods.png", appIcon))
	if err := desktopnote.Register(appIcon); err != nil {
		logger.Printf("toasts keep the app id as their sender: %v", err)
	}
	simValue := os.Getenv("AIRPODS_GUI_SIM")
	open := app.SimOpener(simValue)
	picker := app.SimPicker(simValue)
	if open == nil && picker == nil {
		wanted, required := app.WantedDongle(a.Preferences(), serialFlag)
		picker = device.NewPicker(hid.Enumerate, hid.Open, wanted, required)
	}
	if picker != nil {
		open = picker.Open
	}
	logger.Printf("airpods-tray %s %s", displayVersion(), runtime.GOOS)
	open = logged(open, logger)
	config := app.Config{
		Fyne: a, Host: tray.DesktopHost{App: desk}, Open: open,
		Options: device.DefaultOptions(), Version: version, TrayAvailable: tray.Available, Log: logger, Picker: picker,
	}
	if entry, err := autostart.New(); err == nil {
		config.Autostart = entry
	} else {
		logger.Printf("no autostart option: %v", err)
	}
	if dir, err := choices.DefaultDir(); err == nil {
		config.Choices = &choices.Store{Dir: dir, Logf: logger.Printf}
	} else {
		logger.Printf("nothing is remembered per pair: %v", err)
	}
	tr := app.New(config)
	a.Lifecycle().SetOnStarted(func() {
		tr.Start()
		tray.OnTapped(func() { fyne.Do(tr.Window().Show) })
		if guard != nil {
			guard.OnQuit(func() { fyne.Do(tr.Quit) })
			guard.Serve(func() { fyne.Do(tr.Window().Show) })
		}
		time.AfterFunc(2*time.Second, func() { fyne.Do(tr.ReapplyTray) })
		if !trayFlag {
			tr.Window().Show()
			return
		}
		go func() {
			available := app.WaitForTray(tray.Available, app.TrayWait, 250*time.Millisecond, time.Sleep)
			if app.ShouldOpenWindow(trayFlag, available) {
				fyne.Do(tr.Window().Show)
			}
		}()
	})
	a.Lifecycle().SetOnStopped(tr.Stop)
	a.Run()
}

// setUpLogFiles says where a crash leaves its trace and the log and, with AIRPODS_GUI_LOG_FILE=1, also keeps
// the log in a rotating file. It runs behind the guard, so that a second start touches neither.
// What it returns closes the file; nil when there is none.
func setUpLogFiles(logger *debuglog.Log) func() {
	cache, err := os.UserCacheDir()
	if err != nil {
		logger.Printf("no place for log files: %v", err)
		return nil
	}
	dir := filepath.Join(cache, "airpods-tray")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		logger.Printf("no place for log files: %v", err)
		return nil
	}
	// From here a panic in any goroutine leaves its trace in the file, not only one in run's.
	if err := logger.SetCrashFile(filepath.Join(dir, "crash.log")); err != nil {
		logger.Printf("no crash file: %v", err)
	}
	if os.Getenv("AIRPODS_GUI_LOG_FILE") != "1" {
		return nil
	}
	file, err := debuglog.OpenRotating(dir)
	if err != nil {
		logger.Printf("no log file: %v", err)
		return nil
	}
	logger.Tee(file)
	return func() { file.Close() }
}

func quitRunning() error {
	path, err := instance.DefaultPath()
	if err != nil {
		return err
	}
	return instance.Quit(path, quitWait, instance.OlderPaths()...)
}

func acquire(path, message string) (*instance.Guard, bool, error) {
	guard, err := instance.Acquire(path, message, instance.OlderPaths()...)
	return guard, guard != nil, err
}

// logged records each change in what opening the dongle does, and the traffic after it.
func logged(open device.OpenFunc, logger *debuglog.Log) device.OpenFunc {
	last := ""
	return func() (hid.Device, error) {
		dev, err := open()
		text := "open ok"
		if err != nil {
			text = "open: " + err.Error()
		}
		if text != last {
			last = text
			logger.Printf("%s", text)
		}
		if err != nil {
			return nil, err
		}
		return logger.Wrap(dev), nil
	}
}
