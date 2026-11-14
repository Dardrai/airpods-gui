//go:build !linux && !windows

package main

import "os"

// isTerminal is false where no dongle can be reached anyway, so that nothing asks.
func isTerminal(*os.File) bool { return false }
