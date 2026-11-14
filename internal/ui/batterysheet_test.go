package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/device"
)

func sheetState() device.State {
	return device.State{Present: true, Status: dongle.Status{
		Connected: true, ControlsReady: true, ANC: 2, Peer: [6]byte{1, 2, 3, 4, 5, 6},
		BatteryLeft: 80, BatteryRight: 75, BatteryCase: 255,
		Capabilities: dongle.CapControl | dongle.CapListening | dongle.CapTypedRPC,
	}}
}

func TestBatterySheetShowsThePageReadings(t *testing.T) {
	test.NewTempApp(t)
	closed := 0
	s := newBatterySheet(func() { closed++ })
	s.Update(sheetState(), model.View{Model: "A3063"})
	if s.title.Text != "AirPods Pro 3" {
		t.Errorf("title %q", s.title.Text)
	}
	for i, want := range []struct{ percent, label string }{{"80%", "Left"}, {"75%", "Right"}, {dashText, "Case"}} {
		c := s.columns[i]
		if c.percent.Text != want.percent || c.label.Text != want.label || !c.box.Visible() {
			t.Errorf("column %d: %q %q visible %v", i, c.percent.Text, c.label.Text, c.box.Visible())
		}
	}
	if s.columns[0].level != 80 || s.columns[2].level != -1 {
		t.Errorf("gauges %d %d", s.columns[0].level, s.columns[2].level)
	}
	if !s.modeRow.Visible() || s.mode.Text != "Noise Cancellation" || s.modeKind != iconCancellation {
		t.Errorf("mode %v %q %d", s.modeRow.Visible(), s.mode.Text, s.modeKind)
	}
	withMode := s.Object().MinSize().Height
	test.Tap(s.close)
	if closed != 1 {
		t.Errorf("closed %d times", closed)
	}

	// Disconnected: no mode to show, so no line and a shorter window.
	away := device.State{Present: true, Status: dongle.Status{BatteryLeft: 255, BatteryRight: 255, BatteryCase: 255}}
	s.Update(away, model.View{Model: "A3063"})
	if s.modeRow.Visible() {
		t.Error("mode line without a mode")
	}
	if got := s.Object().MinSize().Height; got >= withMode {
		t.Errorf("height %v with no mode line, %v with one", got, withMode)
	}
	if s.columns[0].percent.Text != dashText {
		t.Errorf("left %q", s.columns[0].percent.Text)
	}
}

// The dongle keeps the last levels with young ages once the AirPods are away.
func TestBatterySheetShowsNothingCurrentWithoutAConnection(t *testing.T) {
	test.NewTempApp(t)
	s := newBatterySheet(nil)
	st := sheetState()
	st.Status.Capabilities |= dongle.CapPerComponent
	st.Status.PresentMask, st.Status.BatteryCase = 7, 60
	st.Status.AgeLeft, st.Status.AgeRight, st.Status.AgeCase = 1000, 1000, 1000
	s.Update(st, model.View{Model: "A3063"})
	for i, c := range s.columns {
		if c.percent.Text == dashText || c.artDim {
			t.Fatalf("column %d while connected: %q, art dim %v", i, c.percent.Text, c.artDim)
		}
	}
	st.Status.Connected = false
	s.Update(st, model.View{Model: "A3063"})
	for i, c := range s.columns {
		if c.percent.Text != dashText || c.level != -1 || !c.artDim {
			t.Errorf("column %d while away: %q, gauge %d, art dim %v", i, c.percent.Text, c.level, c.artDim)
		}
	}
}

func TestBatterySheetHasOneColumnForOverEarModels(t *testing.T) {
	test.NewTempApp(t)
	s := newBatterySheet(nil)
	st := sheetState()
	st.Status.BatteryLeft, st.Status.BatteryRight = 255, 60
	s.Update(st, model.View{Model: "A3184"})
	if s.columns[1].box.Visible() || s.columns[2].box.Visible() {
		t.Error("more than one column")
	}
	if c := s.columns[0]; c.percent.Text != "60%" || c.label.Text != "Battery" || c.kind != artHeadphones {
		t.Errorf("%q %q kind %d", c.percent.Text, c.label.Text, c.kind)
	}
	// Back to earbuds: three columns, each with its own battery again.
	s.Update(sheetState(), model.View{Model: "A3063"})
	if !s.columns[1].box.Visible() || !s.columns[2].box.Visible() || s.columns[0].percent.Text != "80%" || s.columns[0].label.Text != "Left" {
		t.Errorf("after the headphones: %q %q", s.columns[0].percent.Text, s.columns[0].label.Text)
	}
}
