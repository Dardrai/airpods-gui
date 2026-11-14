package cliutil

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"
)

// failAfter accepts the given number of writes.
type failAfter struct {
	writes int
	err    error
	got    bytes.Buffer
}

func (f *failAfter) Write(p []byte) (int, error) {
	if f.writes == 0 {
		return 0, f.err
	}
	f.writes--
	return f.got.Write(p)
}

func TestLatchWriter(t *testing.T) {
	full := errors.New("disk full")
	closed := &os.PathError{Op: "write", Path: "/dev/stdout", Err: syscall.EPIPE}
	for _, tc := range []struct {
		name         string
		err          error
		writes, code int
		want         int
		stderr       string
	}{
		{"all written", full, 2, 0, 0, ""},
		{"a line lost", full, 1, 0, 1, "Error: cannot write to stdout: disk full\n"},
		{"failure kept", full, 0, 1, 1, ""},
		{"uncertain kept", full, 0, 3, 3, ""},
		{"interrupted kept", full, 0, ExitInterrupted, ExitInterrupted, ""},
		// airpods diag --help | head -0: the reader left, which is not an error to describe.
		{"closed pipe is quiet", closed, 0, 0, ExitInterrupted, ""},
		{"closed pipe keeps a failure", closed, 0, 1, 1, ""},
		{"closed pipe keeps uncertain", closed, 0, 3, 3, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sink := &failAfter{writes: tc.writes, err: tc.err}
			latch := &LatchWriter{W: sink}
			fmt.Fprint(latch, "one\n")
			fmt.Fprint(latch, "two\n")
			var stderr bytes.Buffer
			if got := latch.Result(tc.code, &stderr); got != tc.want || stderr.String() != tc.stderr {
				t.Errorf("code %d, stderr %q", got, stderr.String())
			}
		})
	}
}

func TestLatchWriterStopsAfterAFailure(t *testing.T) {
	sink := &failAfter{writes: 0, err: errors.New("gone")}
	latch := &LatchWriter{W: sink}
	fmt.Fprint(latch, "one\n")
	sink.writes = 5 // Even if the sink would recover, the output already has a hole in it.
	fmt.Fprint(latch, "two\n")
	if sink.got.Len() != 0 {
		t.Errorf("wrote %q after the failure", sink.got.String())
	}
}

// TestLatchWriterWordsTheFailure: a command that reports the write error itself prints the
// same line as Result does, and a closed pipe is still recognised through the wording.
func TestLatchWriterWordsTheFailure(t *testing.T) {
	latch := &LatchWriter{W: &failAfter{err: errors.New("disk full")}}
	_, err := fmt.Fprint(latch, "one\n")
	if err == nil || err.Error() != "cannot write to stdout: disk full" {
		t.Errorf("err = %v", err)
	}
	if _, again := fmt.Fprint(latch, "two\n"); again != err {
		t.Errorf("the second write failed with %v", again)
	}
	latch = &LatchWriter{W: &failAfter{err: &os.PathError{Op: "write", Path: "/dev/stdout", Err: syscall.EPIPE}}}
	if _, err := fmt.Fprint(latch, "one\n"); !IsClosedPipe(err) {
		t.Errorf("closed pipe not recognised in %v", err)
	}
}
