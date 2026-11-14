// Package debuglog keeps a short, shareable record of what went over the dongle's
// request channel. It records sizes and codes, never payloads, so no AirPods address or
// serial ends up in it.
package debuglog

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/Dardrai/airpods-gui/core/hid"
)

const maxLines = 4000

// Log is a bounded list of lines, safe for concurrent use.
type Log struct {
	mu    sync.Mutex
	lines []string
	echo  io.Writer
	file  io.Writer
	now   func() time.Time

	// crash is where a panic leaves the log; nil means only stderr gets it.
	crash  *os.File
	stderr io.Writer
}

// New creates a log; echo, when not nil, also receives every line.
func New(echo io.Writer) *Log { return &Log{echo: echo, now: time.Now, stderr: os.Stderr} }

// Tee also writes the log to file: what it holds already, then every new line. Write errors
// are ignored, since the log has nowhere else to report them.
func (l *Log) Tee(file io.Writer) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, line := range l.lines {
		fmt.Fprintln(file, line)
	}
	l.file = file
}

// maxCrashBytes is the size past which the crash file starts over, keeping the one before.
const maxCrashBytes = 1 << 20

// SetCrashFile says where a crash leaves its trace, as well as on stderr. The Go runtime
// writes there itself when a panic in any goroutine, or a fatal error, ends the process;
// Protect adds the log when the panic is in its own goroutine. Both append through one
// file, so neither writes over the other nor over what an earlier run left.
func (l *Log) SetCrashFile(path string) error {
	if l == nil {
		return nil
	}
	if info, err := os.Stat(path); err == nil && info.Size() > maxCrashBytes {
		os.Rename(path, path+".1")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if err := debug.SetCrashOutput(file, debug.CrashOptions{}); err != nil {
		file.Close()
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.crash != nil {
		l.crash.Close()
	}
	l.crash = file
	return nil
}

// Protect runs fn. If fn panics, the log goes to stderr and the crash file, since the app is
// about to take it to the grave, and the panic carries on. It covers only fn's goroutine; a
// panic in another one leaves the runtime's trace in the crash file, without the log.
func (l *Log) Protect(fn func()) {
	if l == nil {
		fn()
		return
	}
	defer func() {
		if value := recover(); value != nil {
			l.dump(value, debug.Stack())
			panic(value)
		}
	}()
	fn()
}

func (l *Log) dump(value any, stack []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	text := fmt.Sprintf("panic: %v\n\n%s\n--- debug log\n%s\n", value, stack, strings.Join(l.lines, "\n"))
	fmt.Fprint(l.stderr, text)
	if l.crash == nil {
		return
	}
	// The runtime's own trace follows this one in the file and carries no date.
	if _, err := fmt.Fprintf(l.crash, "--- %s\n%s", l.now().Format(time.RFC3339), text); err != nil {
		fmt.Fprintln(l.stderr, "could not write the crash file:", err)
	}
}

// Printf adds a line.
func (l *Log) Printf(format string, args ...any) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	line := l.now().Format("15:04:05.000") + " " + fmt.Sprintf(format, args...)
	l.lines = append(l.lines, line)
	if len(l.lines) > maxLines {
		l.lines = append([]string(nil), l.lines[len(l.lines)-maxLines/2:]...)
	}
	if l.echo != nil {
		fmt.Fprintln(l.echo, line)
	}
	if l.file != nil {
		fmt.Fprintln(l.file, line)
	}
}

// Text returns the whole log.
func (l *Log) Text() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

type device struct {
	hid.Device
	log        *Log
	lastStatus string
	lastReply  string
	lastError  string
}

// Wrap records a device's traffic.
func (l *Log) Wrap(dev hid.Device) hid.Device { return &device{Device: dev, log: l} }

func (d *device) failed(what string, err error) {
	text := what + " failed: " + err.Error()
	if text != d.lastError {
		d.lastError = text
		d.log.Printf("%s", text)
	}
}

func (d *device) Get(report byte) ([hid.ReportSize]byte, error) {
	data, err := d.Device.Get(report)
	if err != nil {
		d.failed(fmt.Sprintf("get report %d", report), err)
		return data, err
	}
	d.lastError = ""
	le := binary.LittleEndian
	switch report {
	case 2:
		// Flags, generation, mode and capabilities only: bytes 12..17 hold the address.
		line := fmt.Sprintf("status flags=%02x generation=%d mode=%d codec=%d capabilities=%04x", data[7], le.Uint32(data[8:]), data[18], data[23], le.Uint16(data[42:]))
		if line != d.lastStatus {
			d.lastStatus = line
			d.log.Printf("%s", line)
		}
	case 3:
		code := byte(0)
		if data[21] == 4 {
			code = data[22]
		}
		line := fmt.Sprintf("<- op=%d generation=%d seq=%d total=%d offset=%d size=%d phase=%d code=%d", data[7], le.Uint32(data[8:]), le.Uint32(data[12:]), le.Uint16(data[16:]), le.Uint16(data[18:]), data[20], data[21], code)
		if line != d.lastReply {
			d.lastReply = line
			d.log.Printf("%s", line)
		}
	}
	return data, nil
}

func (d *device) Set(data [hid.ReportSize]byte) error {
	le := binary.LittleEndian
	switch data[0] {
	case 2:
		d.log.Printf("-> control opcode=%d arg=%d seq=%d", data[7], data[22], le.Uint32(data[18:]))
	case 3:
		d.log.Printf("-> op=%d generation=%d seq=%d total=%d offset=%d size=%d first=%02x", data[7], le.Uint32(data[8:]), le.Uint32(data[12:]), le.Uint16(data[16:]), le.Uint16(data[18:]), data[20], data[22])
	}
	err := d.Device.Set(data)
	if err != nil {
		d.failed("set", err)
	}
	return err
}
