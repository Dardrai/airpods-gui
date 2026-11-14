package cliutil

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

// IsClosedPipe says that err is a write to a pipe whose reader has gone.
func IsClosedPipe(err error) bool {
	return errors.Is(err, syscall.EPIPE) || errors.Is(err, windows.ERROR_BROKEN_PIPE) || errors.Is(err, windows.ERROR_NO_DATA)
}
