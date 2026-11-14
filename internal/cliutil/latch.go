package cliutil

import (
	"fmt"
	"io"
)

// LatchWriter remembers the first failed write, so that a command which prints without
// checking every line can still fail at the end. After a failure it writes nothing more.
type LatchWriter struct {
	W   io.Writer
	err error
}

func (l *LatchWriter) Write(p []byte) (int, error) {
	if l.err != nil {
		return 0, l.err
	}
	n, err := l.W.Write(p)
	if err != nil {
		l.err = &stdoutError{err}
	}
	return n, l.err
}

// stdoutError words a failed write the same way whether the command reports it or Result does.
type stdoutError struct{ err error }

func (e *stdoutError) Error() string { return "cannot write to stdout: " + e.err.Error() }
func (e *stdoutError) Unwrap() error { return e.err }

// Result is the exit code of a command that printed through this writer. A code that already
// says something failed is kept: 3 still has to mean that a write may have applied. A reader
// that closed the pipe has stopped the command and is owed no message, as after Ctrl-C.
func (l *LatchWriter) Result(code int, stderr io.Writer) int {
	if code != 0 || l.err == nil {
		return code
	}
	if IsClosedPipe(l.err) {
		return ExitInterrupted
	}
	fmt.Fprintf(stderr, "Error: %v\n", l.err)
	return 1
}
