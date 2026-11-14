package autostart

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopPath(t *testing.T) {
	cases := []struct{ xdg, want string }{
		{"", "/home/me/.config/autostart/airpods-tray.desktop"},
		{"/custom/cfg", "/custom/cfg/autostart/airpods-tray.desktop"},
		{"relative/cfg", "/home/me/.config/autostart/airpods-tray.desktop"},
	}
	for _, c := range cases {
		if got := DesktopPath(c.xdg, "/home/me"); got != c.want {
			t.Errorf("xdg %q: %q", c.xdg, got)
		}
	}
}

func execLine(t *testing.T, entry string) string {
	t.Helper()
	for _, line := range strings.Split(entry, "\n") {
		if value, ok := strings.CutPrefix(line, "Exec="); ok {
			return value
		}
	}
	t.Fatal("no Exec line")
	return ""
}

func TestDesktopExecQuoting(t *testing.T) {
	cases := []struct{ exe, want string }{
		{"/opt/airpods/airpods-tray", "/opt/airpods/airpods-tray --tray"},
		{"/opt/air pods/tray", `"/opt/air pods/tray" --tray`},
		{`/opt/a$b/tray`, `"/opt/a\\$b/tray" --tray`},
		{`/opt/100%/tray`, `"/opt/100%%/tray" --tray`},
	}
	for _, c := range cases {
		if got := execLine(t, DesktopEntry(c.exe)); got != c.want {
			t.Errorf("%q: %q want %q", c.exe, got, c.want)
		}
	}
}

func TestDesktopFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg", "autostart", DesktopFile)
	entry := DesktopEntryFile{Path: path, Exe: "/opt/airpods-tray"}
	if on, err := entry.Enabled(); on || err != nil {
		t.Fatalf("initially: %v %v", on, err)
	}
	if err := entry.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Exec=/opt/airpods-tray --tray\n", "Type=Application\n"} {
		if !strings.Contains(string(content), want) {
			t.Errorf("missing %q in:\n%s", want, content)
		}
	}
	if on, err := entry.Enabled(); !on || err != nil {
		t.Fatalf("after enabling: %v %v", on, err)
	}
	moved := DesktopEntryFile{Path: path, Exe: "/somewhere/else"}
	if on, _ := moved.Enabled(); on {
		t.Error("an entry for another path must read as off")
	}
	if stale, err := moved.Stale(); !stale || err != nil {
		t.Errorf("stale %v %v", stale, err)
	}
	if stale, _ := entry.Stale(); stale {
		t.Error("the current entry is not stale")
	}
	if err := entry.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if err := entry.SetEnabled(false); err != nil {
		t.Errorf("disabling twice: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("file left behind: %v", err)
	}
}

type fakeStore struct {
	value    string
	present  bool
	ignore   bool
	setError error
}

func (f *fakeStore) Get() (string, bool, error) { return f.value, f.present, nil }
func (f *fakeStore) Set(value string) error {
	if f.setError != nil {
		return f.setError
	}
	if !f.ignore {
		f.value, f.present = value, true
	}
	return nil
}
func (f *fakeStore) Delete() error {
	if !f.ignore {
		f.value, f.present = "", false
	}
	return nil
}

func TestRunEntry(t *testing.T) {
	store := &fakeStore{}
	entry := RunEntry{Store: store, Exe: `C:\Program Files\AirPods\airpods-tray.exe`}
	if err := entry.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	if store.value != `"C:\Program Files\AirPods\airpods-tray.exe" --tray` {
		t.Errorf("value %q", store.value)
	}
	if on, err := entry.Enabled(); !on || err != nil {
		t.Errorf("enabled: %v %v", on, err)
	}
	moved := RunEntry{Store: store, Exe: `C:\new\airpods-tray.exe`}
	if on, _ := moved.Enabled(); on {
		t.Error("an entry for another path must read as off")
	}
	if stale, err := moved.Stale(); !stale || err != nil {
		t.Errorf("stale %v %v", stale, err)
	}
	if stale, _ := entry.Stale(); stale {
		t.Error("the current entry is not stale")
	}
	if err := entry.SetEnabled(false); err != nil || store.present {
		t.Errorf("disable: %v present=%v", err, store.present)
	}
}

func TestRunEntryReadBackCatchesFailure(t *testing.T) {
	store := &fakeStore{ignore: true}
	entry := RunEntry{Store: store, Exe: "x"}
	if err := entry.SetEnabled(true); err == nil {
		t.Error("a write that did not stick must fail")
	}
	store.value, store.present = "x", true
	if err := entry.SetEnabled(false); err == nil {
		t.Error("a delete that did not stick must fail")
	}
	broken := errors.New("access denied")
	if err := (RunEntry{Store: &fakeStore{setError: broken}, Exe: "x"}).SetEnabled(true); !errors.Is(err, broken) {
		t.Errorf("err %v", err)
	}
}

// The installer writes the same value as this package, or the App tab would call it stale.
func TestInstallerWritesTheSameRunValue(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "build", "windows", "installer.nsi"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)
	for _, want := range []string{
		`!define RUN_KEY "` + RunKey + `"`,
		`!define RUN_VALUE "` + RunValue + `"`,
		WindowsCommand(`$INSTDIR\${EXE}`),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("build/windows/installer.nsi has no %s", want)
		}
	}
}
