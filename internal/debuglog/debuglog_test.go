package debuglog

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
)

func TestLogsRequestsWithoutPersonalData(t *testing.T) {
	cfg := sim.Demo()
	cfg.Fail = map[byte]byte{2: 7}
	log := New(nil)
	client := dongle.NewClient(log.Wrap(sim.New(cfg)))
	settings, err := client.SettingsClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settings.Read(); err == nil {
		t.Fatal("expected a refusal")
	}
	if _, err := client.Cached(dongle.SlotMetadata); err != nil {
		t.Fatal(err)
	}
	text := log.Text()
	for _, want := range []string{"status flags=07", "capabilities=03ff", "-> op=7", "first=02", "phase=4 code=7", "-> op=3"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	for _, private := range []string{"SIMSERIAL", "4c:56", "4c569d", "A3063"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(private)) {
			t.Errorf("log contains %q", private)
		}
	}
}

func TestLogIsBounded(t *testing.T) {
	log := New(nil)
	for i := range maxLines + 10 {
		log.Printf("%d", i)
	}
	if got := strings.Count(log.Text(), "\n"); got >= maxLines {
		t.Errorf("%d lines", got)
	}
}

func TestTeeWritesWhatIsThereAndWhatComes(t *testing.T) {
	log := New(nil)
	log.Printf("before")
	var file bytes.Buffer
	log.Tee(&file)
	log.Printf("after")
	if got := file.String(); !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Errorf("%q", got)
	}
}

func TestProtectDumpsTheLogAndLetsThePanicGoOn(t *testing.T) {
	log := New(nil)
	var stderr bytes.Buffer
	log.stderr = &stderr
	crash := filepath.Join(t.TempDir(), "crash.log")
	if err := os.WriteFile(crash, []byte("an earlier crash\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	setCrashFile(t, log, crash)
	log.Printf("the last thing that happened")
	defer func() {
		if value := recover(); value != "boom" {
			t.Errorf("panic became %v", value)
		}
		data, err := os.ReadFile(crash)
		if err != nil {
			t.Fatal(err)
		}
		for name, text := range map[string]string{"stderr": stderr.String(), "file": string(data)} {
			if !strings.Contains(text, "panic: boom") || !strings.Contains(text, "the last thing that happened") || !strings.Contains(text, "goroutine") {
				t.Errorf("%s: %q", name, text)
			}
		}
		if !strings.HasPrefix(string(data), "an earlier crash\n--- ") {
			t.Errorf("the earlier crash is gone: %q", data)
		}
		info, err := os.Stat(crash)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("crash file: %v %v", info, err)
		}
	}()
	log.Protect(func() { panic("boom") })
	t.Error("the panic stopped")
}

// setCrashFile gives the test process its crash output back when the test ends.
func setCrashFile(t *testing.T, log *Log, path string) {
	t.Helper()
	if err := log.SetCrashFile(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		debug.SetCrashOutput(nil, debug.CrashOptions{})
		log.crash.Close()
	})
}

func TestCrashFileIsPrivateAndStartsOverWhenBig(t *testing.T) {
	crash := filepath.Join(t.TempDir(), "crash.log")
	if err := os.WriteFile(crash, make([]byte, maxCrashBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	setCrashFile(t, New(nil), crash)
	info, err := os.Stat(crash)
	if err != nil || info.Size() != 0 || info.Mode().Perm() != 0o600 {
		t.Errorf("crash file: %v %v", info, err)
	}
	if info, err := os.Stat(crash + ".1"); err != nil || info.Size() != maxCrashBytes+1 {
		t.Errorf("the one before: %v %v", info, err)
	}
	if err := New(nil).SetCrashFile(filepath.Join(crash, "below-a-file")); err == nil {
		t.Error("a crash file that cannot be opened was accepted")
	}
	if err := (*Log)(nil).SetCrashFile(crash); err != nil {
		t.Error(err)
	}
}

// crashChild is set in the copy of this test binary that TestAPanicInAnyGoroutineLeavesATrace
// starts: it names the crash file and tells the copy to die of a panic.
const crashChild = "AIRPODS_DEBUGLOG_CRASH_CHILD"

// A panic outside Protect's goroutine ends the process without passing through it, so the
// trace in the file is the runtime's. In a main-goroutine panic both write, one after the other.
func TestAPanicInAnyGoroutineLeavesATrace(t *testing.T) {
	if path := os.Getenv(crashChild); path != "" {
		log := New(nil)
		log.stderr = io.Discard
		if err := log.SetCrashFile(path); err != nil {
			t.Fatal(err)
		}
		log.Printf("the last thing that happened")
		if os.Getenv(crashChild+"_PROTECTED") == "1" {
			log.Protect(func() { panic("boom in the protected goroutine") })
		}
		go func() { panic("boom in another goroutine") }()
		select {}
	}
	exe, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	for _, test := range []struct {
		name, protected string
		want            []string
	}{
		{"another goroutine", "0", []string{"an earlier crash\npanic: boom in another goroutine", "goroutine "}},
		{"the protected goroutine", "1", []string{"an earlier crash\n--- ", "the last thing that happened\n", "panic: boom in the protected goroutine [recovered"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			crash := filepath.Join(t.TempDir(), "crash.log")
			if err := os.WriteFile(crash, []byte("an earlier crash\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(exe, "-test.run=^TestAPanicInAnyGoroutineLeavesATrace$")
			cmd.Env = append(os.Environ(), crashChild+"="+crash, crashChild+"_PROTECTED="+test.protected)
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("the child did not crash:\n%s", output)
			}
			data, err := os.ReadFile(crash)
			if err != nil {
				t.Fatal(err)
			}
			at := 0
			for _, want := range test.want {
				found := strings.Index(string(data)[at:], want)
				if found < 0 {
					t.Fatalf("missing %q after byte %d in:\n%s", want, at, data)
				}
				at += found + len(want)
			}
		})
	}
}

func TestProtectWithoutAPanicOrALog(t *testing.T) {
	ran := 0
	New(nil).Protect(func() { ran++ })
	(*Log)(nil).Protect(func() { ran++ })
	if ran != 2 {
		t.Errorf("ran %d times", ran)
	}
}

func TestRotatingFileStaysBounded(t *testing.T) {
	dir := t.TempDir()
	file, err := OpenRotating(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	line := strings.Repeat("x", 1<<16) + "\n"
	for range 100 {
		if _, err := file.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != keptFiles+1 {
		t.Errorf("%d files", len(entries))
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > maxFileBytes {
			t.Errorf("%s has %d bytes", entry.Name(), info.Size())
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %v", entry.Name(), info.Mode().Perm())
		}
	}
}

func TestRotatingFileKeepsWhatWasThere(t *testing.T) {
	dir := t.TempDir()
	for _, word := range []string{"first", "second"} {
		file, err := OpenRotating(dir)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(word + "\n")); err != nil {
			t.Fatal(err)
		}
		file.Close()
		if _, err := file.Write([]byte("late")); err == nil {
			t.Error("wrote to a closed file")
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if text := string(data); !strings.Contains(text, "first") || !strings.Contains(text, "second") || !strings.Contains(text, "--- started") {
		t.Errorf("%q", text)
	}
}
