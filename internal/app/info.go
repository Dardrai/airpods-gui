package app

import (
	"errors"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/choices"
	"github.com/Dardrai/airpods-gui/internal/device"
)

const (
	infoRetry = 5 * time.Second
	// infoPreference, followed by the pair's key, keeps its model and firmware.
	infoPreference = "pair.info."
)

// infoState tracks the AirPods' own information, which is read once per connection:
// the model decides what the tray and the window offer.
type infoState struct {
	current    dongle.Metadata
	reports    map[byte]uint32
	generation uint32
	have       bool
	reading    bool
	now        func() time.Time

	// tryGeneration is the connection the failures below belong to. nextTry follows
	// device.ReadBackoff; failureLogged keeps the log to one line per connection.
	tryGeneration uint32
	failures      int
	nextTry       time.Time
	failureLogged bool

	keyFailureLogged bool

	// allowOffDone is the connection the remembered Allow Off was handled for.
	allowOffDone    uint32
	allowOffHandled bool
	allowOffSending bool
	allowOffTry     time.Time
}

func (a *App) clock() time.Time {
	if a.info.now != nil {
		return a.info.now()
	}
	return time.Now()
}

// refreshInfo reads the metadata report when a connection needs it. The dongle has it a moment
// after the AirPods connect, and until then the page cannot tell which model it is showing. A
// busy channel is normal next to another client, so failing is quiet. Nothing announces that
// the report has come, so the notes tick retries on the backoff schedule.
func (a *App) refreshInfo(st device.State) {
	if !st.Ready() || !st.Status.Has(dongle.CapRawTransport) {
		if !st.Present || !st.Status.Connected {
			a.info.have = false
			a.setInfo(a.rememberedInfo(st))
		}
		return
	}
	generation := st.Status.Generation
	if a.info.reading || (a.info.have && a.info.generation == generation) {
		return
	}
	if a.info.current == (dongle.Metadata{}) {
		// Until the AirPods say who they are, they are who this pair was last time.
		a.setInfo(a.rememberedInfo(st))
	}
	if a.info.tryGeneration != generation {
		a.info.tryGeneration, a.info.failures, a.info.nextTry, a.info.failureLogged = generation, 0, time.Time{}, false
	}
	if a.clock().Before(a.info.nextTry) {
		return
	}
	a.info.reading = true
	sameLink := func(status dongle.Status) bool {
		return status.Peer == st.Status.Peer && status.Generation == generation
	}
	var meta dongle.Metadata
	var found, stale bool
	var reports map[byte]uint32
	a.owner.Submit(func(c *dongle.Client) error {
		// The state in hand can be a poll behind: this must be the pair it will be saved under.
		status, err := c.Status()
		if err != nil {
			return err
		}
		if !sameLink(status) {
			stale = true
			return nil
		}
		report, err := c.Cached(dongle.SlotMetadata)
		if err != nil || report == nil {
			return err
		}
		meta, found = dongle.ParseMetadata(report.Packet)
		if found && st.Status.Has(dongle.CapTypedRPC) {
			// One snapshot per connection: Off is offered according to Allow Off.
			if settings, err := c.SettingsClient(); err == nil {
				if records, err := settings.Read(); err == nil {
					reports = make(map[byte]uint32, len(records))
					for control, record := range records {
						reports[control] = record.Value
					}
				}
			}
		}
		return nil
	}, func(err error) {
		a.do(func() {
			a.info.reading = false
			if stale || !sameLink(a.state.Status) {
				// Read for one pair or connection with another there now. No failure of
				// the read, and the next state or tick asks the right one.
				a.info.nextTry = time.Time{}
				return
			}
			if err != nil || !found {
				a.metadataFailed(err)
				return
			}
			a.info.have, a.info.generation = true, generation
			a.info.failures, a.info.nextTry = 0, time.Time{}
			if reports != nil {
				a.info.reports = reports
			}
			a.setInfo(meta)
			a.rememberInfo(st, meta)
			a.pushView()
			a.reapplyAllowOff(a.state)
		})
	})
}

// metadataFailed schedules the next try. An empty report, with no error, only means the
// AirPods have not sent it yet.
func (a *App) metadataFailed(err error) {
	a.info.failures++
	a.info.nextTry = a.clock().Add(device.ReadBackoff(a.info.failures))
	if err != nil && !a.info.failureLogged && !errors.Is(err, device.ErrStopped) {
		a.info.failureLogged = true
		a.log.Printf("metadata read failed: %v", err)
	}
}

// reapplyAllowOff sends the remembered Allow Off once per connection. The dongle forgets an
// Allow Off write whenever the AirPods connect again, and for models that apply it silently
// and never report it, it then refuses Off until allow-off on is sent again: these are the
// models model.View.NeedsAllowOffResend names. The value is the user's own choice and
// sending it again changes nothing if it did survive. An Off chosen before this has gone
// out sends allow-off on itself, first.
func (a *App) reapplyAllowOff(st device.State) {
	if !st.Connected() {
		a.info.allowOffHandled = false
		return
	}
	generation := st.Status.Generation
	if a.info.allowOffSending || (a.info.allowOffHandled && a.info.allowOffDone == generation) {
		return
	}
	if !st.Ready() || st.Status.MicrophoneActive || !st.Status.Has(dongle.CapTypedRPC) ||
		!a.info.have || a.info.generation != generation {
		return
	}
	now := a.clock()
	if !a.info.allowOffTry.IsZero() && now.Sub(a.info.allowOffTry) < infoRetry {
		return
	}
	_, reported := a.info.reports[model.ControlAllowOff]
	requested := a.win.AllowOffRequested()
	if !model.SilentAllowOff(a.info.current.Model) || reported || requested == nil {
		a.info.allowOffDone, a.info.allowOffHandled = generation, true
		return
	}
	value := uint32(2)
	if *requested {
		value = 1
	}
	a.info.allowOffSending, a.info.allowOffTry = true, now
	a.owner.Submit(func(c *dongle.Client) error {
		settings, err := c.SettingsClient()
		if err != nil {
			return err
		}
		_, err = settings.Send("allow-off", value)
		return err
	}, func(err error) {
		a.do(func() {
			a.info.allowOffSending = false
			if err != nil {
				a.log.Printf("remembered allow-off not sent: %v", err)
			}
			if err == nil || device.IsUnconfirmed(err) {
				a.win.AllowOffSent(generation, value == 1)
			}
			// Only a refused request is known not to have applied; nothing else is sent twice.
			if err == nil || !device.IsBusy(err) {
				a.info.allowOffDone, a.info.allowOffHandled = generation, true
				a.info.allowOffTry = time.Time{}
			}
		})
	})
}

// rememberedInfo is the model and firmware the dongle's selected pair last reported, so
// the page keeps its name and pictures while the AirPods are away or still connecting.
// The serial number is never kept.
func (a *App) rememberedInfo(st device.State) dongle.Metadata {
	if !st.Present || !st.Status.Selected() {
		return dongle.Metadata{}
	}
	key, ok := a.infoKey(st.Status.Peer)
	if !ok {
		return dongle.Metadata{}
	}
	saved := a.fyne.Preferences().String(key)
	number, firmware, _ := strings.Cut(saved, "|")
	if _, known := model.Find(number); !known {
		return dongle.Metadata{}
	}
	return dongle.Metadata{Model: number, Firmware: firmware}
}

func (a *App) rememberInfo(st device.State, meta dongle.Metadata) {
	if !st.Status.Selected() || meta.Model == "" {
		return
	}
	key, ok := a.infoKey(st.Status.Peer)
	if !ok {
		return
	}
	value := meta.Model + "|" + meta.Firmware
	if prefs := a.fyne.Preferences(); prefs.String(key) != value {
		prefs.SetString(key, value)
	}
}

// infoKey is the preference that keeps a pair's model and firmware, after moving what earlier
// versions kept under the address. Without a store nothing is kept.
func (a *App) infoKey(peer [6]byte) (string, bool) {
	if a.choices == nil {
		return "", false
	}
	a.migrateInfo(peer)
	key, err := a.choices.PairKey(peer)
	if err != nil {
		a.logInfoKey(err)
		return "", false
	}
	return infoPreference + key, true
}

func (a *App) logInfoKey(err error) {
	if !a.info.keyFailureLogged {
		a.info.keyFailureLogged = true
		a.log.Printf("pair key unavailable, model not remembered: %v", err)
	}
}

// preferenceValues is how Fyne's preferences show their keys. It is not part of the interface
// they are handed out as, so a driver may lack it; the pair in hand still moves over then.
type preferenceValues interface {
	ReadValues(func(map[string]any))
}

// migrateStored moves, once at startup, everything earlier versions kept under a Bluetooth
// address: each such name is an address to protect, whether or not its pair ever comes by
// again. Both moves leave alone what is already moved, so running them again changes nothing.
func (a *App) migrateStored() {
	if a.choices == nil {
		return
	}
	a.choices.Migrate()
	prefs := a.fyne.Preferences()
	values, ok := prefs.(preferenceValues)
	if !ok {
		return
	}
	var legacy [][6]byte
	values.ReadValues(func(all map[string]any) {
		for key := range all {
			if name, found := strings.CutPrefix(key, infoPreference); found {
				if old, ok := choices.LegacyPeer(name); ok {
					legacy = append(legacy, old)
				}
			}
		}
	})
	for _, old := range legacy {
		a.moveInfo(prefs, old)
	}
}

// migrateInfo moves the given pair's entry, which only finds anything where the startup
// pass could not: without ReadValues, or with no salt to be had then.
func (a *App) migrateInfo(peer [6]byte) {
	a.moveInfo(a.fyne.Preferences(), peer)
}

// moveInfo moves one pair's entry. A value already under the new key is newer and wins.
func (a *App) moveInfo(prefs fyne.Preferences, peer [6]byte) {
	legacy := infoPreference + choices.LegacyName(peer)
	saved := prefs.String(legacy)
	if saved == "" {
		return
	}
	key, err := a.choices.PairKey(peer)
	if err != nil {
		a.logInfoKey(err) // Stays where it is, and is tried again on the next read.
		return
	}
	if prefs.String(infoPreference+key) == "" {
		prefs.SetString(infoPreference+key, saved)
	}
	prefs.RemoveValue(legacy)
}

func (a *App) setInfo(meta dongle.Metadata) {
	if a.info.current == meta {
		return
	}
	a.info.current = meta
	a.win.SetInfo(meta)
	a.pushView()
}

// setReports takes the settings the AirPods reported, for the gates on the tray rows.
func (a *App) setReports(reports map[byte]uint32) {
	a.info.reports = reports
	a.pushView()
}

func (a *App) view() model.View {
	return model.View{
		Model: a.info.current.Model, Firmware: a.info.current.Firmware, Reported: a.info.reports,
		AllowOffRequested: a.win.AllowOffRequested(),
	}
}

func (a *App) pushView() { a.tray.SetView(a.view()) }
