package main

import (
	"os"
	"syscall"
	"unsafe"
)

// isTerminal asks the kernel for terminal settings, which only a terminal has. A character
// device is not enough: /dev/null is one too.
func isTerminal(file *os.File) bool {
	var settings syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&settings)))
	return errno == 0
}
