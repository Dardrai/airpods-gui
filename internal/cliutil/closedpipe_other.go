//go:build !windows

package cliutil

import (
	"errors"
	"syscall"
)

// IsClosedPipe says that err is a write to a pipe whose reader has gone.
func IsClosedPipe(err error) bool {
	return errors.Is(err, syscall.EPIPE)
}
