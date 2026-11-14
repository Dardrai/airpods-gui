package ui

import (
	"errors"
	"fmt"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

// write sends one value once. It is never retried; an uncertain outcome only re-reads.
func (t *settingsTab) write(row *settingRow, word string) {
	raw, err := row.setting.Encode(word)
	if err != nil {
		t.w.say(fmt.Sprintf(i18n.T("settings.expected"), settingLabel(row.setting.Name), err))
		t.restore(row)
		return
	}
	if !t.w.view().ValueAllowed(row.setting.Control, raw) {
		t.w.say(settingLabel(row.setting.Name) + ": " + i18n.T("settings.notAvailable"))
		t.restore(row)
		return
	}
	row.writing = true
	t.updateEnabled()
	name := row.setting.Name
	silent := !row.reported || silentControl(row.setting.Control, t.w.model())
	sent := row.setting.Decode(raw)
	// The value belongs to the pair and connection it was sent to, whatever is there by the
	// time the answer comes back.
	peer, generation := t.w.state.Status.Peer, t.w.state.Status.Generation
	sameLink := func(status dongle.Status) bool { return status.Peer == peer && status.Generation == generation }
	var result string
	var landed dongle.Status
	landedKnown := false
	// Off needs allow-off on sent in this generation first, on AirPods that never report it.
	allowOffFirst := NeedsAllowOffFirst(t.w.view(), row.setting.Control, raw, t.w.AllowOffOnSent(generation))
	allowOffWent := false
	t.w.owner.SubmitCommand(func(c *dongle.Client) error {
		settings, err := c.SettingsClient()
		if err != nil {
			return err
		}
		if allowOffFirst {
			if allowOffWent, err = SendAllowOffOn(settings); !allowOffWent {
				return err
			}
		}
		if silent {
			result, err = settings.Send(name, raw)
		} else {
			result, err = settings.Set(name, raw)
		}
		// The state the window holds can be a poll behind what the job just talked to.
		if status, statusErr := c.Status(); statusErr == nil {
			landed, landedKnown = status, true
		}
		return err
	}, func(err error) {
		t.w.do(func() {
			switch {
			case allowOffWent:
				t.w.AllowOffSent(generation, true)
			case row.setting.Control == model.ControlAllowOff && (err == nil || device.IsUnconfirmed(err)):
				t.w.AllowOffSent(generation, raw == 1)
			}
			if keepsChoice(silent, row.setting.Control == model.ControlMode, err) {
				if sameLink(t.w.state.Status) && (!landedKnown || sameLink(landed)) {
					row.sent(sent)
					t.saveChoice(row, peer, raw)
				} else {
					t.w.logf("%s: the AirPods changed during the write, not remembered", name)
				}
			}
			t.finish(row, result, err)
		})
	})
}

// SendAllowOffOn sends allow-off on ahead of a write of Off, and reports whether it may have
// gone out: an uncertain send still lets the write follow, which the dongle refuses if it
// did not arrive. It runs on the owner goroutine.
func SendAllowOffOn(settings *dongle.SettingsClient) (bool, error) {
	_, err := settings.Send("allow-off", 1)
	return err == nil || device.IsUnconfirmed(err), err
}

func (t *settingsTab) finish(row *settingRow, result string, err error) {
	row.writeFinished(err)
	if row.unconfirmed {
		// Not a failure the user can act on: the row shows the choice (or, for the mode,
		// what the status reports) and the next read settles it. The log keeps the detail.
		t.w.logf("%s: sent, no report back: %v", row.setting.Name, err)
		setNote(row, "")
		t.w.say("")
		t.restore(row)
		t.updateEnabled()
		t.read()
		return
	}
	if err == nil {
		if row.requested != "" {
			setNote(row, sentText())
		} else {
			setNote(row, "")
		}
		t.w.logf("%s", result)
		t.w.say("")
		t.updateEnabled()
		t.read()
		return
	}
	setNote(row, explainWrite(err))
	t.restore(row)
	t.updateEnabled()
}

// allowOffRequested is a silently sent Allow Off value still waiting for a report.
func (t *settingsTab) allowOffRequested() *bool {
	row := t.byName["allow-off"]
	if row == nil || row.requested == "" {
		return nil
	}
	on := row.requested == "on"
	return &on
}

// loadChoices restores, for the current pair, what was sent earlier and never reported.
func (t *settingsTab) loadChoices() {
	store, peer := t.w.opts.Choices, t.w.state.Status.Peer
	for _, row := range t.rows {
		if !row.writing && !row.reported {
			row.requested = ""
		}
	}
	if store == nil || peer == [6]byte{} {
		t.restoreAll()
		return
	}
	for control, value := range store.Load(peer) {
		for _, row := range t.rows {
			if row.setting.Control == control && !row.reported && !row.writing {
				row.requested = row.setting.Decode(value)
			}
		}
	}
	if on := store.LoadAllowOff(peer); on != nil {
		if row := t.byName["allow-off"]; row != nil && !row.reported && !row.writing {
			row.requested = "off"
			if *on {
				row.requested = "on"
			}
		}
	}
	t.restoreAll()
}

func (t *settingsTab) restoreAll() {
	for _, row := range t.rows {
		if row.writing {
			continue
		}
		t.restore(row)
		if row.requested != "" {
			setNote(row, sentText())
		} else if !row.reported && !row.problem {
			setNote(row, t.unreportedNote(row.setting))
		}
	}
	t.fitModel()
}

func (t *settingsTab) saveChoice(row *settingRow, peer [6]byte, raw uint32) {
	store := t.w.opts.Choices
	if store == nil {
		return
	}
	var err error
	switch {
	case row.setting.Control == model.ControlAllowOff:
		err = store.SaveAllowOff(peer, row.requested == "on")
	case model.ExplicitControl(row.setting.Control):
		err = store.Save(peer, row.setting.Control, raw)
	}
	if err != nil {
		t.w.say(settingLabel(row.setting.Name) + ": " + i18n.T("settings.notRemembered") + ": " + err.Error())
	}
}

// explainWrite words a failed write; only Unsupported differs from the shared wording.
func explainWrite(err error) string {
	var unsupported *dongle.Unsupported
	if errors.As(err, &unsupported) {
		return i18n.T("settings.notAccepted")
	}
	return device.Explain(err)
}
