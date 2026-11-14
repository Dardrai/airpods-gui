package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
	"github.com/Dardrai/airpods-gui/internal/notify"
	"github.com/Dardrai/airpods-gui/internal/ui"
	"github.com/Dardrai/airpods-gui/internal/updates"
)

const (
	// FirmwareRepo is where the dongle firmware is released.
	FirmwareRepo = "Ripthulhu/airpods-dongle"
	// firmwarePrefix names what the firmware check saves.
	firmwarePrefix = "firmwareUpdates"
	// firmwareRetry is the pause after a failed read of the firmware version.
	firmwareRetry = 30 * time.Second
)

// firmwareState is the version the plugged-in dongle reported, read once per dongle.
type firmwareState struct {
	serial  string // the dongle it belongs to; empty while none was read
	reading bool
	nextTry time.Time
	shown   ui.DongleFirmware
}

// refreshFirmware reads the dongle's firmware version once for each dongle that is plugged in.
// Firmware without the version report says so, and is not asked again until replugged.
func (a *App) refreshFirmware(st device.State) {
	if !st.Present {
		a.firmware.serial, a.firmware.nextTry = "", time.Time{}
		a.showFirmware(ui.DongleFirmware{})
		return
	}
	if a.firmware.reading || a.firmware.serial == st.Serial || a.clock().Before(a.firmware.nextTry) {
		return
	}
	if !st.Status.Has(dongle.CapTypedRPC) {
		a.firmware.serial = st.Serial
		a.showFirmware(ui.DongleFirmware{Present: true, Unreported: true})
		return
	}
	a.firmware.reading = true
	serial := st.Serial
	var info dongle.FirmwareInfo
	a.owner.Submit(func(c *dongle.Client) error {
		var err error
		info, err = c.FirmwareInfo()
		return err
	}, func(err error) {
		a.do(func() {
			a.firmware.reading = false
			if !a.state.Present || a.state.Serial != serial {
				return
			}
			var unsupported *dongle.Unsupported
			switch {
			case errors.As(err, &unsupported):
				a.firmware.serial = serial
				a.showFirmware(ui.DongleFirmware{Present: true, Unreported: true})
			case err != nil:
				a.firmware.nextTry = a.clock().Add(firmwareRetry)
				a.log.Printf("dongle firmware version: %v", err)
			default:
				a.firmware.serial = serial
				a.log.Printf("dongle firmware %s", info)
				a.showFirmware(ui.DongleFirmware{Present: true, Known: true, Info: info})
			}
		})
	})
}

func (a *App) showFirmware(fw ui.DongleFirmware) {
	if fw == a.firmware.shown {
		return
	}
	a.firmware.shown = fw
	a.win.SetDongleFirmware(fw)
	a.announceFirmware()
}

// announceFirmware tells once about a firmware release newer than the dongle's own.
func (a *App) announceFirmware() {
	r := a.firmwareUpdates.Result()
	fw := a.firmware.shown
	if !r.Known || !fw.Known || !updates.Newer(r.Latest.Version, updates.Version{Major: fw.Info.Major, Minor: fw.Info.Minor, Patch: fw.Info.Patch}) {
		return
	}
	prefs := a.fyne.Preferences()
	key := firmwarePrefix + ".announced"
	if prefs.StringWithFallback(key, "") == r.Latest.Tag {
		return
	}
	prefs.SetString(key, r.Latest.Tag)
	if prefs.BoolWithFallback(notify.Preference, notify.Default) {
		a.notify(i18n.T("update.firmwareNoteTitle"), fmt.Sprintf(i18n.T("update.firmwareNoteText"), r.Latest.Version.Numbers()))
	}
}
