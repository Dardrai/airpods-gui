package choices

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var peer = [6]byte{1, 2, 3, 4, 5, 6}

func TestRoundTrip(t *testing.T) {
	s := Store{Dir: t.TempDir() + "/sub"}
	if got := s.Load(peer); len(got) != 0 {
		t.Errorf("empty: %v", got)
	}
	if err := s.Save(peer, 0x0a, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(peer, 0x1a, 6); err != nil {
		t.Fatal(err)
	}
	got := s.Load(peer)
	if got[0x0a] != 2 || got[0x1a] != 6 || len(got) != 2 {
		t.Errorf("%v", got)
	}
	if other := s.Load([6]byte{9}); len(other) != 0 {
		t.Errorf("another pair sees %v", other)
	}
}

func TestSaveRefusesWhatCannotBeChosen(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if s.Save(peer, 0x0d, 2) == nil {
		t.Error("mode is not an explicit choice")
	}
	if s.Save(peer, 0x0a, 9) == nil {
		t.Error("invalid value")
	}
	if s.Save([6]byte{}, 0x0a, 2) == nil {
		t.Error("no pair")
	}
}

func TestDamagedFilesAreIgnored(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	for name, content := range map[string]string{
		"not json":  "{",
		"bad key":   `{"0a":2}`,
		"leading":   `{"010":2}`,
		"bad value": `{"10":9}`,
		"not ours":  `{"13":2}`,
		"too long":  `{"10":2,` + strings.Repeat(" ", 1100) + `"26":6}`,
	} {
		path, err := s.path(peer)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := s.Load(peer); len(got) != 0 {
			t.Errorf("%s: %v", name, got)
		}
	}
}

func TestAllowOff(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if s.LoadAllowOff(peer) != nil {
		t.Error("nothing saved")
	}
	if err := s.SaveAllowOff(peer, true); err != nil {
		t.Fatal(err)
	}
	if got := s.LoadAllowOff(peer); got == nil || !*got {
		t.Errorf("%v", got)
	}
	if err := s.SaveAllowOff(peer, false); err != nil {
		t.Fatal(err)
	}
	if got := s.LoadAllowOff(peer); got == nil || *got {
		t.Errorf("%v", got)
	}
	path, err := s.allowOffPath(peer)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("maybe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s.LoadAllowOff(peer) != nil {
		t.Error("garbage read as a value")
	}
}

const peerText = "010203040506"

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, entry := range entries {
		out = append(out, entry.Name())
	}
	return out
}

func TestNoFileNameShowsTheAddress(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.Save(peer, 0x0a, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAllowOff(peer, true); err != nil {
		t.Fatal(err)
	}
	for _, name := range names(t, s.Dir) {
		if strings.Contains(name, peerText) {
			t.Errorf("%s shows the address", name)
		}
	}
}

func TestSaltIsPrivateStableAndPerInstall(t *testing.T) {
	a, b := Store{Dir: t.TempDir()}, Store{Dir: t.TempDir()}
	first, err := a.PairKey(peer)
	if err != nil {
		t.Fatal(err)
	}
	again, err := a.PairKey(peer)
	if err != nil || again != first {
		t.Errorf("%q then %q (%v)", first, again, err)
	}
	if other, _ := b.PairKey(peer); other == first {
		t.Error("two installs name a pair alike")
	}
	if other, _ := a.PairKey([6]byte{9}); other == first {
		t.Error("two pairs share a name")
	}
	if strings.Contains(first, peerText) {
		t.Errorf("%s shows the address", first)
	}
	path := filepath.Join(a.Dir, saltFile)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", info.Mode().Perm())
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	// The next start reads the same salt and tightens the file.
	restarted := Store{Dir: a.Dir}
	if again, _ := restarted.PairKey(peer); again != first {
		t.Error("the salt changed")
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("mode not tightened: %v", info.Mode().Perm())
	}
}

// The salt is read once: a pair is named many times a second while the app runs.
func TestSaltIsReadFromDiskOnce(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	first, err := s.PairKey(peer)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.Dir, saltFile)); err != nil {
		t.Fatal(err)
	}
	again, err := s.PairKey(peer)
	if err != nil || again != first {
		t.Errorf("%q then %q (%v)", first, again, err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, saltFile)); !os.IsNotExist(err) {
		t.Errorf("the disk was touched again: %v", err)
	}
}

// A salt of the wrong length still names what was saved under the right one, wherever the
// rest of it went. Replacing it would orphan all of that.
func TestADamagedSaltIsAnErrorAndStaysAsItIs(t *testing.T) {
	var logged []string
	s := Store{Dir: t.TempDir(), Logf: func(format string, args ...any) { logged = append(logged, fmt.Sprintf(format, args...)) }}
	path := filepath.Join(s.Dir, saltFile)
	if err := os.WriteFile(path, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if key, err := s.PairKey(peer); err == nil {
		t.Errorf("a pair was named %q", key)
	}
	if s.Save(peer, 0x0a, 2) == nil || s.SaveAllowOff(peer, true) == nil {
		t.Error("saved under a damaged salt")
	}
	if got := s.Load(peer); len(got) != 0 {
		t.Errorf("%v", got)
	}
	if s.LoadAllowOff(peer) != nil {
		t.Error("read under a damaged salt")
	}
	if salt, err := os.ReadFile(path); err != nil || string(salt) != "short" {
		t.Errorf("the salt file now holds %q (%v)", salt, err)
	}
	if got := names(t, s.Dir); len(got) != 1 {
		t.Errorf("something was written: %v", got)
	}
	if len(logged) != 1 || !strings.Contains(logged[0], saltFile) {
		t.Errorf("logged %q", logged)
	}
}

// A salt that cannot be read for now is still the salt: a new one would orphan every choice.
func TestAnUnreadableSaltIsAnErrorAndIsNotReplaced(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	first, err := s.PairKey(peer)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(peer, 0x0a, 2); err != nil {
		t.Fatal(err)
	}
	// A directory in the file's place fails to read on every system, whoever runs the test.
	path := filepath.Join(s.Dir, saltFile)
	if err := os.Rename(path, path+".kept"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	broken := Store{Dir: s.Dir}
	if key, err := broken.PairKey(peer); err == nil {
		t.Errorf("a pair was named %q", key)
	}
	if got := broken.Load(peer); len(got) != 0 {
		t.Errorf("%v", got)
	}
	if broken.Save(peer, 0x1a, 6) == nil {
		t.Error("saved without the salt")
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Errorf("what was in the way was replaced: %v", err)
	}
	// Readable again, everything is still there under the same name.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".kept", path); err != nil {
		t.Fatal(err)
	}
	back := Store{Dir: s.Dir}
	if again, err := back.PairKey(peer); err != nil || again != first {
		t.Errorf("%q then %q (%v)", first, again, err)
	}
	if got := back.Load(peer); got[0x0a] != 2 || len(got) != 1 {
		t.Errorf("the choice was orphaned: %v", got)
	}
}

// With no salt to be had nothing can be stored, and above all not in the clear.
func TestWithoutASaltNothingIsStored(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s := Store{Dir: filepath.Join(blocker, "sub")}
	if s.Save(peer, 0x0a, 2) == nil || s.SaveAllowOff(peer, true) == nil {
		t.Error("saved without a salt")
	}
	if got := s.Load(peer); len(got) != 0 {
		t.Errorf("%v", got)
	}
	if s.LoadAllowOff(peer) != nil {
		t.Error("read without a salt")
	}
}

func TestEarlierFilesMoveOverOnFirstRead(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	other := [6]byte{0xa, 0xb, 0xc, 0xd, 0xe, 0xf}
	for name, content := range map[string]string{
		"device-choices-" + peerText + ".json": `{"10":2}`,
		"allow-off-" + peerText:                "true",
		// Another pair, not connected now, whose address is in a file name just the same.
		"device-choices-0a0b0c0d0e0f.json": `{"26":6}`,
		"allow-off-0a0b0c0d0e0f":           "false",
		// Not ours to touch.
		"device-choices-notapeer.json": `{}`,
	} {
		if err := os.WriteFile(filepath.Join(s.Dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.Load(peer); got[0x0a] != 2 || len(got) != 1 {
		t.Errorf("choices did not survive: %v", got)
	}
	if got := s.LoadAllowOff(peer); got == nil || !*got {
		t.Errorf("allow off did not survive: %v", got)
	}
	if got := s.Load(other); got[0x1a] != 6 {
		t.Errorf("the other pair's choices did not survive: %v", got)
	}
	if got := s.LoadAllowOff(other); got == nil || *got {
		t.Errorf("the other pair's allow off did not survive: %v", got)
	}
	found := false
	for _, name := range names(t, s.Dir) {
		if strings.Contains(name, peerText) || strings.Contains(name, "0a0b0c0d0e0f") {
			t.Errorf("%s still shows an address", name)
		}
		found = found || name == "device-choices-notapeer.json"
	}
	if !found {
		t.Error("a file that is not ours went")
	}
	// Later reads and writes work on the moved files.
	if err := s.Save(peer, 0x1a, 6); err != nil {
		t.Fatal(err)
	}
	if got := s.Load(peer); got[0x0a] != 2 || got[0x1a] != 6 {
		t.Errorf("%v", got)
	}
}

func TestSavingFirstKeepsAnEarlierAllowOffAndChoices(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := os.WriteFile(filepath.Join(s.Dir, "device-choices-"+peerText+".json"), []byte(`{"10":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(peer, 0x1a, 6); err != nil {
		t.Fatal(err)
	}
	if got := s.Load(peer); got[0x0a] != 2 || got[0x1a] != 6 {
		t.Errorf("the earlier choice was lost: %v", got)
	}
}

func TestANewerFileWinsOverAnEarlierOne(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.Save(peer, 0x0a, 2); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(s.Dir, "device-choices-"+peerText+".json")
	if err := os.WriteFile(legacy, []byte(`{"10":3}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// Found by the next start.
	restarted := Store{Dir: s.Dir}
	restarted.Migrate()
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("the address stayed on disk: %v", err)
	}
	if got := restarted.Load(peer); got[0x0a] != 2 {
		t.Errorf("%v", got)
	}
}

// At startup every pair's files move, with no pair in sight, and a second pass changes nothing.
func TestMigrateMovesEveryPairAtOnceAndOnlyOnce(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	for name, content := range map[string]string{
		"device-choices-" + peerText + ".json": `{"10":2}`,
		"allow-off-" + peerText:                "true",
		"device-choices-0a0b0c0d0e0f.json":     `{"26":6}`,
		"allow-off-0a0b0c0d0e0f":               "false",
	} {
		if err := os.WriteFile(filepath.Join(s.Dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s.Migrate()
	moved := names(t, s.Dir)
	if len(moved) != 5 { // the four files and the salt
		t.Errorf("files %v", moved)
	}
	for _, name := range moved {
		if strings.Contains(name, peerText) || strings.Contains(name, "0a0b0c0d0e0f") {
			t.Errorf("%s still shows an address", name)
		}
	}
	s.Migrate()
	restarted := Store{Dir: s.Dir}
	restarted.Migrate()
	if again := names(t, s.Dir); strings.Join(again, " ") != strings.Join(moved, " ") {
		t.Errorf("a second pass changed %v into %v", moved, again)
	}
	if got := restarted.Load(peer); got[0x0a] != 2 || len(got) != 1 {
		t.Errorf("%v", got)
	}
	if got := restarted.LoadAllowOff([6]byte{0xa, 0xb, 0xc, 0xd, 0xe, 0xf}); got == nil || *got {
		t.Errorf("the other pair's allow off: %v", got)
	}
}

// A pass that could not name the pairs moves nothing and is made again by the next read.
func TestMigrateIsTriedAgainAfterAFailure(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	legacy := filepath.Join(s.Dir, "device-choices-"+peerText+".json")
	if err := os.WriteFile(legacy, []byte(`{"10":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	salt := filepath.Join(s.Dir, saltFile)
	if err := os.Mkdir(salt, 0o700); err != nil {
		t.Fatal(err)
	}
	s.Migrate()
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("moved or removed without a salt: %v", err)
	}
	if err := os.Remove(salt); err != nil {
		t.Fatal(err)
	}
	if got := s.Load(peer); got[0x0a] != 2 {
		t.Errorf("not moved by the next read: %v", got)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("the address stayed on disk: %v", err)
	}
}

func TestLegacyNames(t *testing.T) {
	if got := LegacyName(peer); got != peerText {
		t.Errorf("%q", got)
	}
	if got, ok := LegacyPeer(peerText); !ok || got != peer {
		t.Errorf("%x %v", got, ok)
	}
	for _, name := range []string{"", "0102030405", "01020304050607", "ABCDEF012345", "zz0203040506"} {
		if _, ok := LegacyPeer(name); ok {
			t.Errorf("%q read as an address", name)
		}
	}
}
