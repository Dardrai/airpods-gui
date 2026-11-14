package desktopnote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The uninstaller removes what Register leaves behind.
func TestUninstallerRemovesTheToastSender(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "build", "windows", "installer.nsi"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)
	for _, want := range []string{
		`!define TOAST_KEY "` + RegistryKey + `"`,
		`Delete "${CACHE_DIR}\` + iconFile + `"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("build/windows/installer.nsi has no %s", want)
		}
	}
}

func TestIconPathIsBesideTheLock(t *testing.T) {
	got, err := IconPath(func() (string, error) { return "base", nil })
	if err != nil || got != filepath.Join("base", "airpods-tray", "notify.png") {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestQuoteDoublesEveryQuote(t *testing.T) {
	for in, want := range map[string]string{
		"AirPods":     "'AirPods'",
		"Dave's":      "'Dave''s'",
		"a’b":         "'a’’b'",
		"$(rm -rf /)": "'$(rm -rf /)'",
	} {
		if got := quote(in); got != want {
			t.Errorf("quote(%q) = %q, want %q", in, got, want)
		}
	}
}
