package statusdoc

import "github.com/Dardrai/airpods-gui/core/dongle"

// HearingReports is how old each kept report of the modes is, in milliseconds. A report that
// predates a change looks the same as one that follows it, so the age is the clue. Nil means no report.
type HearingReports struct {
	HearingAid *uint32 `json:"hearing_aid"`
	Assist     *uint32 `json:"assist"`
	Media      *uint32 `json:"media"`
}

func reportAge(client *dongle.Client, slot byte) *uint32 {
	report, err := client.Cached(slot)
	if err != nil || report == nil {
		return nil
	}
	age := report.AgeMs
	return &age
}

// ReadHearingReports asks the dongle for the ages. Only an owner job may call it.
func ReadHearingReports(client *dongle.Client) HearingReports {
	return HearingReports{
		HearingAid: reportAge(client, dongle.SlotHearingAid),
		Assist:     reportAge(client, dongle.SlotHearingMedia),
		Media:      reportAge(client, dongle.SlotHearingPME),
	}
}
