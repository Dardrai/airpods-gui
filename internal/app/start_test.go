package app

import (
	"testing"
	"time"

	"github.com/Dardrai/airpods-gui/internal/instance"
)

func TestShouldOpenWindow(t *testing.T) {
	cases := []struct {
		flag, tray, want bool
	}{
		{false, true, true},
		{false, false, true},
		{true, true, false},
		{true, false, true},
	}
	for _, c := range cases {
		if got := ShouldOpenWindow(c.flag, c.tray); got != c.want {
			t.Errorf("flag=%v tray=%v: %v", c.flag, c.tray, got)
		}
	}
}

func TestWaitForTray(t *testing.T) {
	var slept time.Duration
	sleep := func(d time.Duration) { slept += d }
	calls := 0
	if !WaitForTray(func() bool { calls++; return calls == 3 }, time.Second, 100*time.Millisecond, sleep) {
		t.Error("a late tray was missed")
	}
	if slept != 200*time.Millisecond {
		t.Errorf("slept %v", slept)
	}
	slept = 0
	if WaitForTray(func() bool { return false }, time.Second, 100*time.Millisecond, sleep) {
		t.Error("no tray reported as available")
	}
	if slept != time.Second {
		t.Errorf("slept %v", slept)
	}
}

func TestInstanceMessage(t *testing.T) {
	if InstanceMessage(false, instance.MessageShow, instance.MessageNone) != instance.MessageShow {
		t.Error("a plain second start must ask for the window")
	}
	if InstanceMessage(true, instance.MessageShow, instance.MessageNone) != instance.MessageNone {
		t.Error("a --tray second start must not open the window")
	}
}
