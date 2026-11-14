package instance

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func socketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ap")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "sub", "instance.sock")
}

func TestFirstWinsAndSecondSignals(t *testing.T) {
	path := socketPath(t)
	first, err := Acquire(path, MessageShow)
	if err != nil || first == nil {
		t.Fatalf("first: %v %v", first, err)
	}
	defer first.Close()
	shown := make(chan struct{}, 1)
	first.Serve(func() { shown <- struct{}{} })
	second, err := Acquire(path, MessageShow)
	if err != nil || second != nil {
		t.Fatalf("second: %v %v", second, err)
	}
	select {
	case <-shown:
	case <-time.After(3 * time.Second):
		t.Fatal("the first instance was not told")
	}
}

func TestQuietSecondInstanceDoesNotShow(t *testing.T) {
	path := socketPath(t)
	first, err := Acquire(path, MessageShow)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	shown := make(chan struct{}, 1)
	first.Serve(func() { shown <- struct{}{} })
	if second, err := Acquire(path, MessageNone); second != nil || err != nil {
		t.Fatalf("second: %v %v", second, err)
	}
	select {
	case <-shown:
		t.Fatal("a tray-only start opened the window")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestStaleSocketIsReplaced(t *testing.T) {
	path := socketPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	// A socket left by a crash: bound, no listener behind it.
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	listener.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stale socket missing: %v", err)
	}
	guard, err := Acquire(path, MessageShow)
	if err != nil || guard == nil {
		t.Fatalf("over stale socket: %v %v", guard, err)
	}
	guard.Close()
}

func TestStaleRegularFileIsReplaced(t *testing.T) {
	path := socketPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	guard, err := Acquire(path, MessageShow)
	if err != nil || guard == nil {
		t.Fatalf("over a file: %v %v", guard, err)
	}
	guard.Close()
}

func TestCloseAllowsANewOwner(t *testing.T) {
	path := socketPath(t)
	first, err := Acquire(path, MessageShow)
	if err != nil {
		t.Fatal(err)
	}
	first.Serve(func() {})
	first.Close()
	second, err := Acquire(path, MessageShow)
	if err != nil || second == nil {
		t.Fatalf("after close: %v %v", second, err)
	}
	second.Close()
}

func TestDirectoryIsPrivate(t *testing.T) {
	path := socketPath(t)
	guard, err := Acquire(path, MessageShow)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Errorf("mode %v", info.Mode().Perm())
	}
}

func TestQuitWithNothingRunningSucceeds(t *testing.T) {
	path := socketPath(t)
	if err := Quit(path, time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestQuitWaitsForTheInstanceToGo(t *testing.T) {
	path := socketPath(t)
	first, err := Acquire(path, MessageShow)
	if err != nil {
		t.Fatal(err)
	}
	shown := make(chan struct{}, 1)
	first.OnQuit(func() { go first.Close() })
	first.Serve(func() { shown <- struct{}{} })
	if err := Quit(path, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case <-shown:
		t.Fatal("a quit request opened the window")
	default:
	}
	second, err := Acquire(path, MessageNone)
	if err != nil || second == nil {
		t.Fatalf("after quit: %v %v", second, err)
	}
	second.Close()
}

func TestQuitReportsAnInstanceThatStays(t *testing.T) {
	path := socketPath(t)
	first, err := Acquire(path, MessageShow)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	first.Serve(func() {}) // no OnQuit, like a version from before the request existed
	if err := Quit(path, 300*time.Millisecond); err == nil {
		t.Fatal("no error for an instance that stayed")
	}
}

func TestOnlyOneOfManyStartingTogetherWins(t *testing.T) {
	path := socketPath(t)
	var winners atomic.Int32
	var guards sync.Map
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			guard, err := Acquire(path, MessageNone)
			if err != nil {
				t.Errorf("%d: %v", i, err)
			}
			if guard != nil {
				winners.Add(1)
				guards.Store(i, guard)
			}
		}()
	}
	close(start)
	wg.Wait()
	if got := winners.Load(); got != 1 {
		t.Errorf("%d winners", got)
	}
	guards.Range(func(_, guard any) bool {
		guard.(*Guard).Close()
		return true
	})
}

// A process with the lock is running even if its socket was deleted: starting another would
// make two apps fight over the dongle.
func TestALockedInstanceThatDoesNotAnswerIsNotReplaced(t *testing.T) {
	path := socketPath(t)
	first, err := Acquire(path, MessageShow)
	if err != nil || first == nil {
		t.Fatalf("first: %v %v", first, err)
	}
	defer first.Close()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	second, err := Acquire(path, MessageShow)
	if second != nil || !errors.Is(err, ErrNoAnswer) {
		t.Fatalf("second: %v %v", second, err)
	}
}

func TestLockIsPrivateAndOutlivesItsHolder(t *testing.T) {
	path := socketPath(t)
	guard, err := Acquire(path, MessageShow)
	if err != nil {
		t.Fatal(err)
	}
	guard.Close()
	info, err := os.Stat(path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Errorf("mode %v", info.Mode().Perm())
	}
}

func TestAnOlderInstanceOnItsOldSocketIsFound(t *testing.T) {
	older := socketPath(t)
	running, err := Acquire(older, MessageShow)
	if err != nil {
		t.Fatal(err)
	}
	defer running.Close()
	shown := make(chan struct{}, 1)
	running.Serve(func() { shown <- struct{}{} })
	guard, err := Acquire(socketPath(t), MessageShow, older)
	if guard != nil || err != nil {
		t.Fatalf("%v %v", guard, err)
	}
	select {
	case <-shown:
	case <-time.After(3 * time.Second):
		t.Fatal("the older instance was not told")
	}
}

func TestStaleOlderSocketIsCleared(t *testing.T) {
	older := socketPath(t)
	if err := os.MkdirAll(filepath.Dir(older), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(older, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	guard, err := Acquire(socketPath(t), MessageShow, older)
	if err != nil || guard == nil {
		t.Fatalf("%v %v", guard, err)
	}
	defer guard.Close()
	if _, err := os.Stat(older); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("still there: %v", err)
	}
}

func TestQuitReachesAnOlderInstance(t *testing.T) {
	older := socketPath(t)
	running, err := Acquire(older, MessageShow)
	if err != nil {
		t.Fatal(err)
	}
	running.OnQuit(func() { go running.Close() })
	running.Serve(func() {})
	if err := Quit(socketPath(t), 3*time.Second, older); err != nil {
		t.Fatal(err)
	}
	if err := signal(older, MessageNone); !errors.Is(err, errNotRunning) {
		t.Errorf("still answering: %v", err)
	}
}

func TestSignalReportsAFailedWrite(t *testing.T) {
	// Nothing listens, so the dial fails and says nothing is running.
	if err := signal(socketPath(t), MessageShow); !errors.Is(err, errNotRunning) {
		t.Errorf("%v", err)
	}
	// A listener that closes at once takes the connection; a message that long cannot be
	// written after that, and it must not count as delivered.
	path := socketPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	huge := make([]byte, 1<<24)
	err = nil
	for range 20 {
		if err = signal(path, string(huge)); err != nil {
			break
		}
	}
	if err == nil {
		t.Fatal("a write to a closed peer reported success")
	}
	if errors.Is(err, errNotRunning) {
		t.Errorf("reported as nothing running: %v", err)
	}
}

func TestDefaultPath(t *testing.T) {
	cache := func() (string, error) { return "/home/u/.cache", nil }
	for _, c := range []struct{ system, runtimeDir, want string }{
		{"linux", "/run/user/1000", "/run/user/1000/airpods-tray/instance.sock"},
		{"linux", "", "/home/u/.cache/airpods-tray/instance.sock"},
		{"linux", "relative", "/home/u/.cache/airpods-tray/instance.sock"},
		{"windows", "/run/user/1000", "/home/u/.cache/airpods-tray/instance.sock"},
	} {
		got, err := defaultPath(c.system, c.runtimeDir, cache)
		if err != nil || got != filepath.FromSlash(c.want) {
			t.Errorf("%+v: %q %v", c, got, err)
		}
	}
	if _, err := defaultPath("linux", "", func() (string, error) { return "", errors.New("no home") }); err == nil {
		t.Error("no error without a cache directory")
	}
}

func TestOlderPathsWithARuntimeDirectory(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	current, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	for _, older := range OlderPaths() {
		if older == current {
			t.Errorf("the current socket is listed as older: %s", older)
		}
	}
}
