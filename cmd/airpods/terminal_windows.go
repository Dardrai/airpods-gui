package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// isTerminal asks for the console mode, which only a console has. NUL is a character device too.
func isTerminal(file *os.File) bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(file.Fd()), &mode) == nil
}
