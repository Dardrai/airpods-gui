package cliutil

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Dardrai/airpods-gui/internal/argparse"
)

func TestNotifyStopOnEverySignalThatWouldSkipCleanup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no way to signal this process")
	}
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGPIPE} {
		t.Run(sig.String(), func(t *testing.T) {
			defer signal.Reset()
			stop := NotifyStop()
			self, err := os.FindProcess(os.Getpid())
			if err != nil {
				t.Fatal(err)
			}
			if err := self.Signal(sig); err != nil {
				t.Fatal(err)
			}
			select {
			case <-stop:
			case <-time.After(5 * time.Second):
				t.Fatal("not closed")
			}
		})
	}
}

func TestIsClosedPipe(t *testing.T) {
	if !IsClosedPipe(fmt.Errorf("write: %w", syscall.EPIPE)) {
		t.Error("EPIPE not recognised")
	}
	if IsClosedPipe(errors.New("disk full")) || IsClosedPipe(nil) {
		t.Error("recognised something else")
	}
}

func TestSecondsOption(t *testing.T) {
	p := &argparse.Parser{Prog: "x", Usage: "usage: x\n", Actions: []*argparse.Action{SecondsOption(30)}}
	ns, exit := p.Parse(nil)
	if exit != nil || ns["seconds"] != 30 {
		t.Fatalf("default: %v %+v", ns, exit)
	}
	if ns, exit = p.Parse([]string{"--seconds", "5"}); exit != nil || ns["seconds"] != 5 {
		t.Fatalf("given: %v %+v", ns, exit)
	}
	if _, exit = p.Parse([]string{"--seconds", "soon"}); exit == nil || !strings.Contains(exit.Stderr, "invalid int value: 'soon'") {
		t.Fatalf("word: %+v", exit)
	}
}

func TestSimDevice(t *testing.T) {
	for _, value := range []string{"", "0"} {
		var note bytes.Buffer
		if device, _ := SimDevice(value, &note); device != nil || note.Len() != 0 {
			t.Errorf("%q gave a device, or said so: %q", value, note.String())
		}
	}
	for _, value := range []string{"1", "case", "anything"} {
		var note bytes.Buffer
		device, cfg := SimDevice(value, &note)
		if device == nil || cfg.Serial == "" {
			t.Fatalf("%q gave no device", value)
		}
		if note.String() != "note: simulated dongle (AIRPODS_CLI_SIM)\n" {
			t.Errorf("%q: note %q", value, note.String())
		}
	}
}
