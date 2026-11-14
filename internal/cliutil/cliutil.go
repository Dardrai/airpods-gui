// Package cliutil holds what the airpods command and its diag subcommand have in common.
package cliutil

import (
	"errors"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/internal/argparse"
)

// ExitInterrupted is the conventional status after Ctrl-C.
const ExitInterrupted = 130

// ErrInterrupted ends a command that was asked to stop.
var ErrInterrupted = errors.New("interrupted")

// OpenFunc opens a dongle; tests inject a simulator.
type OpenFunc func(serial string) (hid.Device, error)

// Finish prints what the parser decided and gives its exit code.
func Finish(exit *argparse.Exit, stdout, stderr io.Writer) int {
	io.WriteString(stdout, exit.Stdout)
	io.WriteString(stderr, exit.Stderr)
	return exit.Code
}

// NotifyStop returns a channel closed by the first signal that would otherwise end the
// process before it has cleaned up: Ctrl-C, termination, a closed terminal, or a closed pipe.
// A closed pipe then also makes the write fail with an error that IsClosedPipe recognises,
// instead of ending the process there.
func NotifyStop() <-chan struct{} {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGPIPE)
	stop := make(chan struct{})
	go func() {
		<-signals
		close(stop)
	}()
	return stop
}

// IntOption is an option holding a Python-style int.
func IntOption(dest, flag string) *argparse.Action {
	return &argparse.Action{Kind: argparse.Store, Dest: dest, Flags: []string{flag}, TypeName: "int",
		Convert: func(s string) (any, error) { return argparse.ParseInt(s) }}
}

// SecondsOption is the --seconds option, with its default.
func SecondsOption(defaultSeconds int) *argparse.Action {
	action := IntOption("seconds", "--seconds")
	action.Default = defaultSeconds
	return action
}
