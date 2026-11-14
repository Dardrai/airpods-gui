package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

func TestReconnectButtonNeedsASavedPair(t *testing.T) {
	cfg := sim.Demo()
	cfg.Peer = sim.Hex{0, 0, 0, 0, 0, 0}
	cfg.Flags = 0
	r := newRig(t, cfg)
	r.main(func() {
		if !r.w.device.reconnect.Disabled() {
			t.Error("no saved pair")
		}
		if r.w.device.pair.Disabled() {
			t.Error("pairing is how to get one")
		}
	})
}

func TestDeviceInfoHidesTheSerialUntilAsked(t *testing.T) {
	r := newRig(t, sim.Demo())
	r.main(func() {
		r.w.SetInfo(dongle.Metadata{Model: "A3063", Firmware: "9A348", Serial: "ABC123"})
		d := r.w.device
		if d.modelName.Text != "AirPods Pro 3 (A3063)" || d.firmware.Text != "9A348" {
			t.Errorf("%q %q", d.modelName.Text, d.firmware.Text)
		}
		if d.airpodsSerial.Text != i18n.T("device.hidden") {
			t.Errorf("serial shown: %q", d.airpodsSerial.Text)
		}
		d.revealSerial.SetChecked(true)
		if d.airpodsSerial.Text != "ABC123" {
			t.Errorf("reveal: %q", d.airpodsSerial.Text)
		}
		d.revealSerial.SetChecked(false)
		if d.airpodsSerial.Text != i18n.T("device.hidden") {
			t.Errorf("hide: %q", d.airpodsSerial.Text)
		}
		r.w.SetInfo(dongle.Metadata{Model: "A9999"})
		if d.modelName.Text != "Unknown model (A9999)" || d.firmware.Text != notReported() || d.airpodsSerial.Text != notReported() {
			t.Errorf("%q %q %q", d.modelName.Text, d.firmware.Text, d.airpodsSerial.Text)
		}
	})
}

func TestPairNeedsConfirmation(t *testing.T) {
	r := newRig(t, sim.Demo())
	var text string
	var answer func(bool)
	r.main(func() {
		r.w.confirm = func(_, body, _ string, a func(bool)) { text, answer = body, a }
		r.w.device.requestPair()
	})
	if !strings.Contains(text, "replaces the saved pairing") {
		t.Errorf("confirmation text: %q", text)
	}
	r.main(func() { answer(false) })
	r.flush()
	if len(r.dev.Controls()) != 0 {
		t.Fatal("cancel sent a command")
	}
	r.main(func() { answer(true) })
	r.until(func() bool { return len(r.dev.Controls()) == 1 }, "pair sent")
	if op := r.dev.Controls()[0][7]; op != dongle.OpPair {
		t.Errorf("opcode %d", op)
	}
}

func TestPairBlockedWhileInUse(t *testing.T) {
	for name, flags := range map[string]byte{"playback": 0x43, "microphone": 0x13, "phone": 0x0b} {
		t.Run(name, func(t *testing.T) {
			cfg := sim.Demo()
			cfg.Flags = flags
			r := newRig(t, cfg)
			var message string
			r.main(func() {
				r.w.inform = func(_, body string) { message = body }
				r.w.confirm = func(_, _, _ string, _ func(bool)) { t.Error("offered to pair") }
				r.w.device.requestPair()
			})
			if message != i18n.T("device.pairBlocked") {
				t.Errorf("message: %q", message)
			}
			r.flush()
			if len(r.dev.Controls()) != 0 {
				t.Error("a command was sent")
			}
		})
	}
}

func TestReconnectAndPairPhoneButtons(t *testing.T) {
	r := newRig(t, sim.Demo())
	// The dongle already owns the AirPods, so the handover sends nothing.
	r.main(func() { test.Tap(r.w.device.reconnect) })
	r.until(func() bool { return strings.HasSuffix(r.w.banner.Text, i18n.T("reconnect.acquired")) }, "result")
	r.main(func() { test.Tap(r.w.device.pairPhone) })
	r.until(func() bool { return len(r.dev.Controls()) == 1 }, "commands")
	if op := r.dev.Controls()[0][7]; op != dongle.OpPairPhone {
		t.Errorf("opcode %d", op)
	}
}

func TestReconnectButtonSendsTheCommandOnOldFirmware(t *testing.T) {
	cfg := sim.Demo()
	cfg.Capabilities &^= dongle.CapRouting
	r := newRig(t, cfg)
	r.main(func() { test.Tap(r.w.device.reconnect) })
	r.until(func() bool { return strings.HasSuffix(r.w.banner.Text, i18n.T("device.sent")) }, "result")
	if c := r.dev.Controls(); len(c) != 1 || c[0][7] != dongle.OpReconnect {
		t.Errorf("controls %v", c)
	}
}
