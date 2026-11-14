//go:build windows

package desktopnote

import (
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
)

// scriptTimeout ends a PowerShell that hangs; it usually takes about a second to start.
const scriptTimeout = 15 * time.Second

// toastScript shows one toast. It carries the same tag as the last, so a new state takes the
// old one's place in the notification center rather than stacking under it.
const toastScript = `$ErrorActionPreference = 'Stop'
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] > $null
$title = [Security.SecurityElement]::Escape(%s)
$text = [Security.SecurityElement]::Escape(%s)
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml("<toast><visual><binding template=""ToastGeneric""><text>$title</text><text>$text</text></binding></visual><audio silent=""true""/></toast>")
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
$toast.Tag = 'state'
$toast.Group = 'airpods'
$toast.ExpirationTime = [DateTimeOffset]::Now.AddMinutes(1)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier(%s).Show($toast)
`

// Send shows a toast under the registered sender. Before Register has worked it leaves the
// note to the toolkit, which is better than nothing.
func Send(title, text string) error {
	if !registered.Load() {
		return ErrUnsupported
	}
	ctx, cancel := context.WithTimeout(context.Background(), scriptTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive",
		"-ExecutionPolicy", "Bypass", "-EncodedCommand", encode(script(title, text)))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	if out, err := cmd.CombinedOutput(); err != nil {
		return &scriptError{err: err, output: strings.TrimSpace(string(out))}
	}
	return nil
}

func script(title, text string) string {
	return fmt.Sprintf(toastScript, quote(title), quote(text), quote(ToastID))
}

// encode is PowerShell's -EncodedCommand form: base64 of UTF-16LE, so no text needs escaping
// for the command line.
func encode(s string) string {
	units := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(units))
	for i, u := range units {
		b[2*i], b[2*i+1] = byte(u), byte(u>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

type scriptError struct {
	err    error
	output string
}

func (e *scriptError) Error() string {
	if e.output == "" {
		return "toast: " + e.err.Error()
	}
	return "toast: " + e.err.Error() + ": " + e.output
}

func (e *scriptError) Unwrap() error { return e.err }
