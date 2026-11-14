package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/hid"
)

// writeWatch notes whether a request of one kind was handed to the dongle at all.
type writeWatch struct {
	hid.Device
	kind byte
	sent bool
}

func (w *writeWatch) Set(data [hid.ReportSize]byte) error {
	if data[0] == 3 && data[20] > 0 && data[22] == w.kind {
		w.sent = true
	}
	return w.Device.Set(data)
}

// TestAudioWriteNeverSentIsPlainFailure fails every read in turn, on a lost transport and on a
// changed session. "It may have applied" and exit 3 are only for a write that went out.
func TestAudioWriteNeverSentIsPlainFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind byte
		args []string
	}{
		{"audio set", 3, []string{"audio", "set", "--codec", "sbc"}},
		{"game", 4, []string{"game", "on"}},
	} {
		for _, fault := range []struct {
			name string
			set  func(*sim.Config, int)
		}{
			{"read fails", func(cfg *sim.Config, at int) { cfg.GetFailAt = at }},
			{"session changes", func(cfg *sim.Config, at int) { cfg.GenerationChangeAtGet = at }},
		} {
			t.Run(tc.name+", "+fault.name, func(t *testing.T) {
				plain, uncertain := 0, 0
				// Some reads in between can be lost without harm, so the loop ends only once
				// the fault has come after the command's last read several times running.
				for at, harmless := 1, 0; harmless < 8; at++ {
					cfg := sim.Demo()
					fault.set(&cfg, at)
					watch := &writeWatch{Device: sim.New(cfg), kind: tc.kind}
					var stdout, stderr bytes.Buffer
					code := runClock(tc.args, &stdout, &stderr, func(string) (hid.Device, error) { return watch, nil }, sim.NewVirtualClock())
					if code == 0 {
						if !watch.sent {
							t.Fatalf("fault at read %d: the command wrote nothing: %s", at, stdout.String())
						}
						harmless++
						continue
					}
					harmless = 0
					maybe := strings.Contains(stderr.String(), "It may have applied")
					switch {
					case !watch.sent && (code != 1 || maybe || !strings.HasPrefix(stderr.String(), "Error: ")):
						t.Errorf("fault at read %d, nothing sent: exit %d, stderr %q", at, code, stderr.String())
					case watch.sent && (code != 3 || !maybe):
						t.Errorf("fault at read %d, write sent: exit %d, stderr %q", at, code, stderr.String())
					}
					if watch.sent {
						uncertain++
					} else {
						plain++
					}
				}
				if plain == 0 || uncertain == 0 {
					t.Errorf("%d plain failures and %d uncertain ones: the loop did not reach both", plain, uncertain)
				}
			})
		}
	}
}
