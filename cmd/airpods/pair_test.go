package main

import (
	"strings"
	"testing"

	"github.com/Dardrai/airpods-gui/core/dongle/sim"
)

func TestPairAsksFirst(t *testing.T) {
	for _, tc := range []struct {
		name, answer string
		args         []string
		code, sent   int
		out, err     string
		asked        int
	}{
		{"yes", "y", []string{"pair"}, 0, 1, "pair: accepted by dongle\n", pairWarning + "\n", 1},
		{"no", "n", []string{"pair"}, 1, 0, "", pairWarning + "\nError: Nothing was sent: not confirmed.\n", 1},
		{"nobody", "", []string{"pair"}, 1, 0, "", pairWarning + "\nError: Nothing was sent: pairing needs --yes, or a terminal to confirm in.\n", 0},
		{"flag", "", []string{"pair", "--yes"}, 0, 1, "pair: accepted by dongle\n", pairWarning + "\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			device, stdout, stderr, code, asked := runParity(t, sim.Default(), tc.answer, tc.args...)
			if code != tc.code || stdout != tc.out || stderr != tc.err || asked != tc.asked {
				t.Errorf("exit %d, %q, %q, asked %d", code, stdout, stderr, asked)
			}
			if got := len(device.Controls()); got != tc.sent {
				t.Errorf("sent %d", got)
			}
		})
	}
}

func TestPairThatWouldBeRefusedAsksNothing(t *testing.T) {
	for name, mutate := range map[string]func(*sim.Config){
		"phone": func(c *sim.Config) { c.Flags = 3 | 8 },
		"mic":   func(c *sim.Config) { c.Flags = 3 | 16 },
		"usb":   func(c *sim.Config) { c.Flags = 3 | 64 },
		"none":  func(c *sim.Config) { c.Capabilities = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := sim.Default()
			mutate(&cfg)
			device, _, stderr, code, asked := runParity(t, cfg, "y", "pair")
			if code != 1 || asked != 0 || len(device.Controls()) != 0 || strings.Contains(stderr, "Warning") || !strings.HasPrefix(stderr, "Error: Control failure:") {
				t.Errorf("exit %d, asked %d, %q", code, asked, stderr)
			}
		})
	}
}

func TestPairRefusesAFlagItDoesNotKnow(t *testing.T) {
	device, _, stderr, code := exec(t, sim.Default(), "pair", "--force")
	if code != 2 || len(device.Controls()) != 0 || !strings.Contains(stderr, "--force") {
		t.Errorf("exit %d, %q", code, stderr)
	}
}

// TestPairFlagMustBeWrittenInFull: --y used to count as --yes, and erase the pairing unasked.
func TestPairFlagMustBeWrittenInFull(t *testing.T) {
	for _, flag := range []string{"--y", "--ye"} {
		device, _, stderr, code := exec(t, sim.Default(), "pair", flag)
		if code != 2 || len(device.Controls()) != 0 || !strings.Contains(stderr, "unrecognized arguments: "+flag) {
			t.Errorf("%s: exit %d, %q", flag, code, stderr)
		}
	}
}

// TestGlobalFlagAfterTheCommand: list --serial used to become list --serials.
func TestGlobalFlagAfterTheCommand(t *testing.T) {
	for _, args := range [][]string{
		{"list", "--serial"}, {"list", "--serial", "X"}, {"status", "--serial=X"}, {"status", "--json"}, {"status", "--debug"},
		{"pair", "--json"}, {"set", "mode", "off", "--debug"}, {"status", "--version"},
	} {
		device, stdout, stderr, code := exec(t, sim.Default(), args...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, "airpods: error: --") || !strings.HasSuffix(stderr, " must come before the command\n") {
			t.Errorf("%v: exit %d, %q, %q", args, code, stdout, stderr)
		}
		if ops, _ := device.TraceSummary(); ops != 0 {
			t.Errorf("%v: touched the dongle", args)
		}
	}
}
