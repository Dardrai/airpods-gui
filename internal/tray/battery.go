package tray

import (
	"fmt"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/model"
	"github.com/Dardrai/airpods-gui/internal/device"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

// Battery components, in the bit order of the status report's masks.
const (
	BatteryLeft = iota
	BatteryRight
	BatteryCase
)

const (
	freshEarbudMs = 30_000
	freshCaseMs   = 90_000
	unknownAge    = 0xffffffff
)

// Reading is one battery's presentation.
type Reading struct {
	Known    bool
	Level    int
	Current  bool // recent enough to rely on
	Charging bool
	AgeMs    uint32
	HasAge   bool
}

func raw(status dongle.Status, component int) (byte, uint32) {
	switch component {
	case BatteryLeft:
		return status.BatteryLeft, status.AgeLeft
	case BatteryRight:
		return status.BatteryRight, status.AgeRight
	}
	return status.BatteryCase, status.AgeCase
}

// ReadBattery applies the freshness rules: a reading is current while present and
// younger than 30 s, or under 90 s when it came from the case. Without a connection there
// is no reading at all: the dongle goes on reporting the last levels with young ages, and
// those say nothing about AirPods that are away.
func ReadBattery(st device.State, component int) Reading {
	if !st.Connected() {
		return Reading{}
	}
	level, age := raw(st.Status, component)
	if level > 100 {
		return Reading{}
	}
	bit := byte(1) << component
	r := Reading{Known: true, Level: int(level), AgeMs: age, HasAge: age != unknownAge}
	present := !st.Status.Has(dongle.CapPerComponent) || st.Status.PresentMask&bit != 0
	if r.HasAge && present {
		if st.Status.CaseSourceMask&bit != 0 {
			r.Current = age < freshCaseMs
		} else {
			r.Current = age <= freshEarbudMs
		}
	}
	r.Charging = r.Current && st.Status.ChargingMask&bit != 0
	return r
}

// ArtAvailable decides whether a battery's picture shows at full strength: an earbud
// while the AirPods are connected and it is not reported missing, the case only with a
// current reading. Nothing shows at full strength without a connection.
func ArtAvailable(st device.State, component int) bool {
	if !st.Connected() {
		return false
	}
	if component != BatteryCase {
		bit := byte(1) << component
		if !st.Status.Has(dongle.CapPerComponent) || st.Status.PresentMask&bit != 0 || awaitingReport(st) {
			return true
		}
	}
	return ReadBattery(st, component).Current
}

// awaitingReport is the first half minute of a connection: the dongle says connected at
// once, and the presence mask and the batteries arrive together later. An empty mask with
// no earbud reading is that wait, not two missing earbuds.
func awaitingReport(st device.State) bool {
	return st.Connected() && st.Status.PresentMask == 0 &&
		!ReadBattery(st, BatteryLeft).Known && !ReadBattery(st, BatteryRight).Known
}

func compact(r Reading) string {
	switch {
	case !r.Known:
		return "--"
	case r.Charging:
		return fmt.Sprintf("%d%%+", r.Level)
	case !r.Current:
		return fmt.Sprintf("%d%%?", r.Level)
	}
	return fmt.Sprintf("%d%%", r.Level)
}

// BatteryLine is the tray's battery row: + means charging, ? means an old reading.
func BatteryLine(st device.State) string {
	return fmt.Sprintf("%s %s · %s %s · %s %s",
		i18n.T("battery.shortL"), compact(ReadBattery(st, BatteryLeft)),
		i18n.T("battery.shortR"), compact(ReadBattery(st, BatteryRight)),
		i18n.T("battery.case"), compact(ReadBattery(st, BatteryCase)))
}

func spelled(name string, r Reading) string {
	switch {
	case !r.Known:
		return name + " " + i18n.T("battery.unknown")
	case r.Charging:
		return fmt.Sprintf("%s %d%% (%s)", name, r.Level, i18n.T("battery.charging"))
	case !r.Current && r.HasAge:
		return fmt.Sprintf("%s %d%% (%s)", name, r.Level, fmt.Sprintf(i18n.T("battery.age"), r.AgeMs/1000))
	case !r.Current:
		return fmt.Sprintf("%s %d%% (%s)", name, r.Level, i18n.T("battery.lastKnown"))
	}
	return fmt.Sprintf("%s %d%%", name, r.Level)
}

// batteryRow is the battery row for a model: over-ear models have one battery and no
// case, so the row shows that one reading on its own.
func batteryRow(st device.State, view model.View) string {
	if view.Headphone() {
		for _, component := range []int{BatteryLeft, BatteryRight} {
			if r := ReadBattery(st, component); r.Known {
				return compact(r)
			}
		}
		return compact(Reading{})
	}
	return BatteryLine(st)
}

// HasReading reports whether there is any battery reading to show.
func HasReading(st device.State) bool {
	for _, component := range []int{BatteryLeft, BatteryRight, BatteryCase} {
		if ReadBattery(st, component).Known {
			return true
		}
	}
	return false
}

// BatteryDetail spells the batteries out for the window.
func BatteryDetail(st device.State) string {
	return spelled(i18n.T("battery.left"), ReadBattery(st, BatteryLeft)) + " · " +
		spelled(i18n.T("battery.right"), ReadBattery(st, BatteryRight)) + " · " +
		spelled(i18n.T("battery.case"), ReadBattery(st, BatteryCase))
}

// LowestEarbud is the lower current earbud percentage. The case does not count.
func LowestEarbud(st device.State) (int, bool) {
	if !st.Connected() {
		return 0, false
	}
	lowest, found := 101, false
	for _, component := range []int{BatteryLeft, BatteryRight} {
		if r := ReadBattery(st, component); r.Known && r.Current {
			lowest, found = min(lowest, r.Level), true
		}
	}
	return lowest, found
}
