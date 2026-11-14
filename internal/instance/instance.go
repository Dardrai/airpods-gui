// Package instance keeps the app to one process per user. The first process holds a lock
// file and listens on a local socket; later ones ask it to show its window and exit.
package instance

import (
	"bufio"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	dialTimeout = time.Second
	readTimeout = time.Second
	quitPoll    = 100 * time.Millisecond
	// answerWait covers the moment between a first process taking the lock and listening,
	// and between it closing the socket and exiting.
	answerWait = 3 * time.Second
	answerPoll = 20 * time.Millisecond

	// MessageShow asks the running instance to show its window.
	MessageShow = "show"
	// MessageNone only checks that an instance is running.
	MessageNone = "none"
	// MessageQuit asks the running instance to quit, as its Quit menu item does.
	MessageQuit = "quit"
)

// ErrNoAnswer means another process holds the lock and does not answer on the socket. Starting
// anyway would make two apps fight over the dongle.
var ErrNoAnswer = errors.New("another instance is running and does not answer")

// errNotRunning means nothing accepted a connection on the socket.
var errNotRunning = errors.New("no instance is listening")

// Guard is the primary instance's lock and socket.
type Guard struct {
	lock     *os.File
	listener net.Listener
	wg       sync.WaitGroup
	onQuit   func()
}

// DefaultPath is where the socket goes: under XDG_RUNTIME_DIR on Linux, which is private to
// the user, on local storage and emptied at logout. Elsewhere, and on a Linux session without
// one, it is under the user cache directory.
func DefaultPath() (string, error) {
	return defaultPath(runtime.GOOS, os.Getenv("XDG_RUNTIME_DIR"), os.UserCacheDir)
}

func defaultPath(system, runtimeDir string, cacheDir func() (string, error)) (string, error) {
	if system == "linux" && filepath.IsAbs(runtimeDir) {
		return filepath.Join(runtimeDir, "airpods-tray", "instance.sock"), nil
	}
	return cachePath(cacheDir)
}

func cachePath(cacheDir func() (string, error)) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "airpods-tray", "instance.sock"), nil
}

// OlderPaths are the sockets a running instance of an earlier version may listen on, which
// knows nothing of the lock. A new version must still find it, to ask it to show or quit.
func OlderPaths() []string {
	current, err := DefaultPath()
	if err != nil {
		return nil
	}
	older, err := cachePath(os.UserCacheDir)
	if err != nil || older == current {
		return nil
	}
	return []string{older}
}

// signal delivers one message. An error wrapping errNotRunning means nothing is there; any
// other error means something is there and the message did not reach it.
func signal(path, message string) error {
	conn, err := net.DialTimeout("unix", path, dialTimeout)
	if err != nil {
		return errors.Join(errNotRunning, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(dialTimeout)); err != nil {
		return err
	}
	_, err = conn.Write([]byte(message + "\n"))
	return err
}

// Acquire returns a Guard when this process is first. When another instance answers on the
// socket, or on an older one, it gets the message and Acquire returns nil and no error. The
// lock decides who is first, so two processes starting together cannot both win, and a socket
// found while holding the lock is stale and replaced. The lock is held until Close.
func Acquire(path, message string, older ...string) (*Guard, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	for _, old := range older {
		if signal(old, message) == nil {
			return nil, nil
		}
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(answerWait)
	for {
		held, err := tryLock(lock)
		if err != nil {
			lock.Close()
			return nil, err
		}
		if held {
			break
		}
		if signal(path, message) == nil {
			lock.Close()
			return nil, nil
		}
		// Another process has the lock and is not listening yet, or is on its way out.
		if time.Now().After(deadline) {
			lock.Close()
			return nil, ErrNoAnswer
		}
		time.Sleep(answerPoll)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		lock.Close()
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		lock.Close()
		return nil, err
	}
	for _, old := range older {
		os.Remove(old) // Nobody answered it, so it is left from an earlier version's crash.
	}
	return &Guard{lock: lock, listener: listener}, nil
}

// Quit asks the running instance, or one on an older socket, to quit and waits for it to stop
// answering. Nothing running is a success. The installer uses this to free the executable
// before replacing it.
func Quit(path string, wait time.Duration, older ...string) error {
	deadline := time.Now().Add(wait)
	for _, socket := range append([]string{path}, older...) {
		if err := quitOne(socket, deadline); err != nil {
			return err
		}
	}
	return nil
}

func quitOne(path string, deadline time.Time) error {
	if err := signal(path, MessageQuit); err != nil {
		if errors.Is(err, errNotRunning) {
			return nil
		}
		return err
	}
	for ; time.Now().Before(deadline); time.Sleep(quitPoll) {
		if errors.Is(signal(path, MessageNone), errNotRunning) {
			return nil
		}
	}
	return errors.New("the running instance did not quit")
}

// OnQuit sets what a quit request runs. Call it before Serve.
func (g *Guard) OnQuit(quit func()) { g.onQuit = quit }

// Serve calls onShow, on its own goroutine, for each show request until Close.
func (g *Guard) Serve(onShow func()) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		for {
			conn, err := g.listener.Accept()
			if err != nil {
				return
			}
			conn.SetReadDeadline(time.Now().Add(readTimeout))
			line, _ := bufio.NewReader(conn).ReadString('\n')
			conn.Close()
			switch strings.TrimSpace(line) {
			case MessageShow:
				onShow()
			case MessageQuit:
				if g.onQuit != nil {
					g.onQuit()
				}
			}
		}
	}()
}

// Close stops serving, removes the socket and lets go of the lock. The lock file stays:
// removing it could split two later processes between the old file and a new one.
func (g *Guard) Close() error {
	err := g.listener.Close()
	g.wg.Wait()
	g.lock.Close()
	return err
}
