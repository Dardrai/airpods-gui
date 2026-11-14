package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/internal/argparse"
	"github.com/Dardrai/airpods-gui/internal/cliutil"
)

// diagParser is the diag subcommand, which was the separate airpods-diag tool: it captures
// read-only dongle audio diagnostics as JSON Lines.
func diagParser() *argparse.Parser {
	flag := func(name string) *argparse.Action {
		return &argparse.Action{Kind: argparse.StoreTrue, Dest: name, Flags: []string{"--" + name}}
	}
	text := helpTexts["diag"]
	return &argparse.Parser{
		Prog:       "airpods diag",
		Usage:      text.usage,
		HelpMsg:    text.usage + text.body,
		VersionMsg: "airpods " + version + "\n",
		Actions: []*argparse.Action{
			helpAction(),
			{Kind: argparse.Version, Dest: "version", Flags: []string{"--version"}},
			cliutil.SecondsOption(60),
			// The same Dest as the flag before the subcommand: given here, it replaces that one.
			{Kind: argparse.Store, Dest: "serial", Flags: []string{"--serial"}},
			{Kind: argparse.Store, Dest: "output", Flags: []string{"--output"}},
			flag("legacy"), flag("inspect"), flag("phone"), flag("radio"),
			{Kind: argparse.StoreTrue, Dest: "no_trace", Flags: []string{"--no-trace"}},
			flag("replace"),
		},
	}
}

// runDiag opens the dongle only once the options are known to be valid.
func runDiag(cli *argparse.Parser, ns argparse.Namespace, stdout, stderr io.Writer, env environment) int {
	finish := func(exit *argparse.Exit) int { return cliutil.Finish(exit, stdout, stderr) }
	seconds := ns["seconds"].(int)
	legacy, inspect, phone, radio := ns.Bool("legacy"), ns.Bool("inspect"), ns.Bool("phone"), ns.Bool("radio")
	noTrace, replace := ns.Bool("no_trace"), ns.Bool("replace")
	path, toFile := ns.String("output")
	toFile = toFile && path != ""
	switch {
	case seconds < 1 || seconds > 1800:
		return finish(cli.Error("--seconds must be 1..1800"))
	case legacy && (inspect || phone):
		return finish(cli.Error("--legacy cannot be combined with --inspect or --phone"))
	case radio && (legacy || inspect || phone || noTrace):
		return finish(cli.Error("--radio is a separate lightweight capture"))
	case replace && !toFile:
		return finish(cli.Error("--replace needs --output"))
	}
	serial, _ := ns.String("serial")
	device, err := env.open(serial)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	defer device.Close()
	output := stdout
	if toFile {
		mode := os.O_EXCL
		if replace {
			mode = os.O_TRUNC
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|mode, 0o666)
		if err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		defer file.Close()
		output = file
	}
	var stop <-chan struct{}
	if env.interrupt != nil {
		stop = env.interrupt()
	}
	client := dongle.NewClient(device)
	client.SetClock(env.clock)
	opts := dongle.SampleOptions{Trace: !legacy && !noTrace, Inspect: inspect, Phone: phone, Radio: radio}
	switch err := capture(client, env.clock, seconds, opts, output, stderr, stop); {
	case err == nil:
		return 0
	case errors.Is(err, cliutil.ErrInterrupted), cliutil.IsClosedPipe(err):
		return cliutil.ExitInterrupted
	default:
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
}
