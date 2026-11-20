package ui

import (
	"errors"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
	"github.com/Dardrai/airpods-gui/internal/tray"
)

// gameModeFollowPeriod is how often the audio state is read again while Game Mode is on its
// way somewhere: waiting for playback, unconfirmed or turning off. Nothing else is polled.
const gameModeFollowPeriod = 5 * time.Second

// codecLabels lists the codecs by number, in the current language.
func codecLabels() []string { return []string{i18n.T("audio.automatic"), "SBC", "AAC", "AAC-ELD"} }

// bitratesFor lists what the library accepts for a codec; 0 is the default.
func bitratesFor(codec byte) []uint32 {
	switch codec {
	case 2:
		return []uint32{0, 96000, 128000, 174000, 192000, 220000}
	case 3:
		return []uint32{0, 128000, 174000, 192000, 220000, 256000}
	}
	return []uint32{0}
}

func bitrateLabel(bitrate uint32) string {
	if bitrate == 0 {
		return i18n.T("audio.default")
	}
	return fmt.Sprintf("%d kbps", bitrate/1000)
}

func codecName(codec byte) string {
	if labels := codecLabels(); codec != 0 && int(codec) < len(labels) {
		return labels[codec]
	}
	return i18n.T("value.unknown")
}

type audioTab struct {
	w       *Window
	content fyne.CanvasObject

	current, requested, delay *widget.Label
	flags, gameState          *widget.Label
	codec, bitrate            *widget.Select
	variable, gameMode        *Switch
	apply                     *widget.Button

	// gameModeCard is the Game Mode card on the default page, with its own switch and a
	// row whose hint names the phase. It shows only where Game Mode can be used.
	gameModeCard   fyne.CanvasObject
	gameModeRow    *optionRow
	gameModeOnPage *Switch
	// lastRead is when the last audio state read started, for following Game Mode.
	lastRead time.Time

	// codecChoice and bitrateChoice are what the pickers mean, whatever language they are in.
	codecChoice   byte
	bitrateChoice uint32
	last          *dongle.AudioState

	have     bool
	supports bool
	pending  bool
	reading  bool
	writing  bool
	applying bool
	// relabeling is set while a language change refills the pickers.
	relabeling bool
}

func newAudioTab(w *Window) *audioTab {
	t := &audioTab{w: w}
	t.current, t.requested, t.delay = newValue(dashText), newValue(dashText), newValue(dashText)
	t.flags, t.gameState = newValue(dashText), newValue(dashText)
	t.codec = widget.NewSelect(codecLabels(), func(label string) {
		if t.relabeling {
			return
		}
		for i, option := range codecLabels() {
			if option == label {
				t.codecChoice = byte(i)
			}
		}
		t.codecChanged()
	})
	t.bitrate = widget.NewSelect([]string{bitrateLabel(0)}, func(label string) {
		if t.relabeling {
			return
		}
		for _, bitrate := range bitratesFor(t.codecChoice) {
			if bitrateLabel(bitrate) == label {
				t.bitrateChoice = bitrate
			}
		}
	})
	t.variable = NewSwitch(nil)
	t.apply = w.button("audio.apply", t.applyProfile)
	t.gameMode = NewSwitch(func(on bool) {
		if !t.applying {
			t.setGameMode(on, nil)
		}
	})
	t.gameModeOnPage = NewSwitch(func(on bool) {
		if !t.applying {
			t.setGameMode(on, nil)
		}
	})
	t.gameModeRow = w.optionRow("audio.gameMode", "", t.gameModeOnPage)
	caption, head := captionText("")
	w.bind(func() { caption.SetText(i18n.T("section.playback")) })
	t.gameModeCard = container.New(tightLayout{}, head, card(newStack(t.gameModeRow)), gap(sectionGap))
	t.gameModeCard.Hide()
	t.showGameModePhase()
	t.content = newStack(
		w.valueRow("audio.active", t.current),
		w.valueRow("audio.requested", t.requested),
		w.valueRow("audio.delay", t.delay),
		w.valueRow("audio.state", t.flags),
		w.selectRow("audio.codec", "", t.codec),
		w.selectRow("audio.bitrate", "", t.bitrate),
		w.optionRow("audio.variable", "", t.variable),
		w.optionRow("audio.applyRow", "audio.applyHint", t.apply),
		w.optionRow("audio.gameMode", "", t.gameMode),
		w.valueRow("audio.gameStatus", t.gameState),
	)
	t.codecChanged()
	t.updateEnabled()
	w.texts = append(w.texts, t.relabel)
	return t
}

// relabel puts the current language on the pickers and the values on show.
func (t *audioTab) relabel() {
	t.relabeling = true
	codec, bitrate := t.codecChoice, t.bitrateChoice
	labels := codecLabels()
	t.codec.SetOptions(labels)
	t.codec.Selected = ""
	t.codec.SetSelected(labels[codec])
	options := make([]string, 0, 6)
	for _, value := range bitratesFor(codec) {
		options = append(options, bitrateLabel(value))
	}
	t.bitrate.SetOptions(options)
	t.bitrate.Selected = ""
	t.bitrate.SetSelected(bitrateLabel(bitrate))
	t.relabeling = false
	if t.last != nil {
		t.showValues(*t.last)
	}
	t.showGameModePhase()
}

func (t *audioTab) selectedCodec() byte { return t.codecChoice }

func (t *audioTab) selectedBitrate() uint32 { return t.bitrateChoice }

func (t *audioTab) codecChanged() {
	codec := t.selectedCodec()
	previous := t.bitrate.Selected
	var options []string
	keep := false
	for _, bitrate := range bitratesFor(codec) {
		label := bitrateLabel(bitrate)
		options = append(options, label)
		keep = keep || label == previous
	}
	t.bitrate.SetOptions(options)
	if !keep {
		t.bitrate.SetSelected(options[0])
		t.bitrateChoice = bitratesFor(codec)[0]
	}
	if codec != 3 {
		t.variable.SetChecked(false)
	}
	t.updateEnabled()
}

func (t *audioTab) updateEnabled() {
	// Not while recording: a profile change could restart the link under the microphone.
	usable := t.w.state.Present && t.w.state.Status.Has(dongle.CapTypedRPC) && t.have && !t.writing && !t.pending &&
		!t.w.state.Status.MicrophoneActive
	codec := t.selectedCodec()
	enableWidget(t.codec, usable)
	enableWidget(t.bitrate, usable && codec >= 2)
	enableWidget(t.variable, usable && codec == 3)
	setEnabled(t.apply, usable)
	enableWidget(t.gameMode, usable && t.supports)
	enableWidget(t.gameModeOnPage, usable && t.supports)
	t.showGameModeCard()
	if t.w.opts.OnGameMode != nil {
		t.w.opts.OnGameMode(t.gameModeForTray(usable))
	}
}

// gameModeAvailable is whether Game Mode can be offered at all: a dongle with typed requests
// that reported Game Mode usable over this connection.
func (t *audioTab) gameModeAvailable() bool {
	return t.w.state.Present && t.w.state.Status.Has(dongle.CapTypedRPC) && t.have && t.supports
}

// showGameModeCard puts the default page's Game Mode card on the page where Game Mode can
// be used, and takes it off where it cannot: it is never shown empty or for good disabled.
func (t *audioTab) showGameModeCard() {
	if on := t.gameModeAvailable(); on != t.gameModeCard.Visible() {
		showIf(t.gameModeCard, on)
		t.w.relayout()
	}
}

// showGameModePhase names the current phase in the default page's row.
func (t *audioTab) showGameModePhase() {
	phase := i18n.T("value.unknown")
	if t.last != nil {
		phase = gamePhaseText(t.last.GameModePhase)
	}
	t.gameModeRow.SetHint(fmt.Sprintf(i18n.T("audio.gameModeHint"), phase))
}

// gameModeForTray is what the tray menu shows of Game Mode. usable is whether a request
// could be sent now.
func (t *audioTab) gameModeForTray(usable bool) tray.GameMode {
	game := tray.GameMode{Supported: t.gameModeAvailable(), Busy: !usable}
	if t.last != nil {
		game.Requested, game.Phase = t.last.GameModeRequested, t.last.GameModePhase
	}
	return game
}

// followGameMode reads the audio state again, at most every gameModeFollowPeriod, while Game
// Mode is on its way to a phase it stays in. With the window hidden this is what moves the
// tray's Game Mode row on.
func (t *audioTab) followGameMode(now time.Time) {
	if t.last == nil || !t.gameModeAvailable() || t.writing || now.Sub(t.lastRead) < gameModeFollowPeriod {
		return
	}
	switch t.last.GameModePhase {
	case dongle.GameModePending, dongle.GameModeUnconfirmed, dongle.GameModeRestoring:
		t.read()
	}
}

func (t *audioTab) read() {
	if t.reading || !t.w.state.Present || !t.w.state.Status.Has(dongle.CapTypedRPC) {
		return
	}
	t.reading = true
	t.lastRead = t.w.now()
	var state dongle.AudioState
	t.w.owner.Submit(func(c *dongle.Client) (err error) {
		state, err = c.AudioState()
		return err
	}, func(err error) {
		t.w.do(func() {
			t.reading = false
			if err != nil {
				t.w.say(i18n.T("audio.title") + ": " + device.Explain(err))
				return
			}
			t.show(state)
		})
	})
}

func (t *audioTab) show(state dongle.AudioState) {
	t.have = true
	t.pending = state.Pending
	t.supports = state.GameModeSupported
	kept := state
	t.last = &kept
	t.showValues(state)
	t.applying = true
	t.codecChoice = state.RequestedCodec
	t.codec.SetSelected(codecLabels()[state.RequestedCodec])
	t.codecChanged()
	t.bitrateChoice = state.RequestedBitrate
	t.bitrate.SetSelected(bitrateLabel(state.RequestedBitrate))
	t.variable.SetChecked(state.Variable)
	t.gameMode.SetChecked(state.GameModeRequested)
	t.gameModeOnPage.SetChecked(state.GameModeRequested)
	t.applying = false
	t.showGameModePhase()
	t.updateEnabled()
}

// showValues writes the read-only rows.
func (t *audioTab) showValues(state dongle.AudioState) {
	t.current.SetText(fmt.Sprintf("%s, %s", codecName(state.Codec), bitrateLabel(state.Bitrate)))
	t.requested.SetText(fmt.Sprintf("%s, %s", codecLabels()[state.RequestedCodec], bitrateLabel(state.RequestedBitrate)))
	if state.ReportedDelayTenthsMs != nil {
		t.delay.SetText(fmt.Sprintf("%.1f ms", float64(*state.ReportedDelayTenthsMs)/10))
	} else {
		t.delay.SetText(notReported())
	}
	text := i18n.T("audio.idle")
	switch {
	case state.Pending:
		text = i18n.T("audio.pending")
	case state.Streaming:
		text = i18n.T("audio.streaming")
	}
	if state.MicrophoneActive {
		text += i18n.T("audio.micActive")
	}
	t.flags.SetText(text)
	if state.GameModeSupported {
		t.gameState.SetText(gamePhaseText(state.GameModePhase))
	} else {
		t.gameState.SetText(i18n.T("audio.notSupported"))
	}
}

// writeMessage: a refusal did not apply, and neither did a request that was never sent;
// anything else may have, because a profile change can restart the link before the reply
// arrives.
func writeMessage(err error) string {
	var rejected *dongle.RequestRejected
	var unsupported *dongle.Unsupported
	if errors.As(err, &rejected) || errors.As(err, &unsupported) || errors.Is(err, device.ErrNoDongle) ||
		errors.Is(err, dongle.ErrNotSent) {
		return device.Explain(err)
	}
	return device.UnconfirmedNote()
}

// submit sends one audio request. outcome words a request the dongle accepted, empty for
// plain "applied"; notify, which may be nil, also hears what went wrong, for a request made
// from the tray, where the window's line is out of sight.
func (t *audioTab) submit(name string, job func(*dongle.Client) (dongle.AudioState, error),
	outcome func(dongle.AudioState) (text string, problem bool), notify func(title, text string)) {
	t.writing = true
	t.updateEnabled()
	var state dongle.AudioState
	t.w.owner.SubmitCommand(func(c *dongle.Client) (err error) {
		state, err = job(c)
		return err
	}, func(err error) {
		t.w.do(func() {
			t.writing = false
			if err != nil {
				text := writeMessage(err)
				t.w.say(name + ": " + text)
				// No answer is not worth a notification: the next read shows what happened.
				if notify != nil && text != device.UnconfirmedNote() {
					notify(name, text)
				}
				t.updateEnabled()
				t.read()
				return
			}
			text, problem := "", false
			if outcome != nil {
				text, problem = outcome(state)
			}
			if text == "" {
				text = i18n.T("audio.applied")
			}
			t.w.say(name + ": " + text)
			if problem && notify != nil {
				notify(name, text)
			}
			t.show(state)
		})
	})
}

func (t *audioTab) applyProfile() {
	codec, variable, bitrate := t.selectedCodec(), t.variable.Checked, t.selectedBitrate()
	t.submit(i18n.T("audio.codecProfile"), func(c *dongle.Client) (dongle.AudioState, error) {
		return c.SetAudioProfile(codec, variable, bitrate)
	}, nil, nil)
}

// setGameMode asks for Game Mode on or off, from either switch or from the tray. As the
// command line tool does, it reads the audio state first and writes only when the request
// differs, and a dongle that answers with the failed or unsupported phase counts as a problem.
func (t *audioTab) setGameMode(on bool, notify func(title, text string)) {
	if t.writing {
		return
	}
	// Both switches show the request at once; the reply or the read after a failure corrects them.
	t.applying = true
	t.gameMode.SetChecked(on)
	t.gameModeOnPage.SetChecked(on)
	t.applying = false
	sent := false
	t.submit(i18n.T("audio.gameMode"), func(c *dongle.Client) (dongle.AudioState, error) {
		state, err := c.AudioState()
		if err != nil || !state.GameModeSupported || state.GameModeRequested == on {
			return state, err
		}
		sent = true
		return c.SetGameMode(on)
	}, func(state dongle.AudioState) (string, bool) {
		switch {
		case !state.GameModeSupported:
			return i18n.T("audio.notSupported"), true
		case !sent:
			return gamePhaseText(state.GameModePhase), false
		case state.GameModePhase == dongle.GameModeFailed || state.GameModePhase == dongle.GameModeUnsupported:
			return gamePhaseText(state.GameModePhase), true
		}
		return "", false
	}, notify)
}

// gamePhaseText names a Game Mode phase the way the firmware's own tool does: waiting for
// playback, unconfirmed, on, turning off.
func gamePhaseText(phase byte) string {
	label := dongle.GameModeLabel(phase)
	return translated("game."+label, titleCase(label))
}
