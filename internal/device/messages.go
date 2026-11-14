package device

import (
	"errors"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/internal/i18n"
)

// UnconfirmedText is the English text shown when a write went out but no answer came back:
// just "sent", since the next read shows what happened; UnconfirmedNote is the same in the
// current language.
const UnconfirmedText = "sent"

// BusyText is the English text shown when another client holds the shared request channel.
const BusyText = "another app is controlling the dongle"

// UnconfirmedNote is UnconfirmedText in the current language.
func UnconfirmedNote() string { return i18n.T("device.sent") }

// IsBusy reports the channel being held by another client.
func IsBusy(err error) bool {
	var rejected *dongle.RequestRejected
	return errors.As(err, &rejected) && rejected.Code == 2
}

// maxReadBackoff is where ReadBackoff stops growing.
const maxReadBackoff = 30 * time.Second

// ReadBackoff is the wait after that many failed reads in a row: 1, 2, 4, 8 and 16 s, then 30 s.
func ReadBackoff(failures int) time.Duration {
	if failures > 5 {
		return maxReadBackoff
	}
	return min(maxReadBackoff, time.Second<<max(0, failures-1))
}

// IsUnconfirmed reports a write whose outcome is unknown.
func IsUnconfirmed(err error) bool {
	var unconfirmed *dongle.Unconfirmed
	return errors.As(err, &unconfirmed)
}

// Explain turns a device error into the text the UI shows. Unconfirmed comes first: it can
// wrap a busy error, and is shown as sent, since the next read settles it.
func Explain(err error) string {
	switch {
	case err == nil:
		return ""
	case IsUnconfirmed(err):
		return UnconfirmedNote()
	case IsBusy(err):
		return i18n.T("device.busy")
	}
	return err.Error()
}

// ExplainRead words a failed read of the settings snapshot. The dongle can refuse that
// while it is still connecting, so a refusal is not reported as missing support.
func ExplainRead(err error, ready bool) string {
	var unsupported *dongle.Unsupported
	switch {
	case IsBusy(err):
		return i18n.T("device.busy")
	case errors.As(err, &unsupported) && !ready:
		return NotReady()
	case errors.As(err, &unsupported):
		return i18n.T("device.refused")
	}
	return Explain(err)
}

// NotReady is NotReadyText in the current language.
func NotReady() string { return i18n.T("device.notReady") }

// English texts for a settings read the dongle would not serve.
const (
	NotReadyText = "the AirPods aren't ready yet"
	RefusedText  = "the dongle isn't serving settings right now; trying again"
)
