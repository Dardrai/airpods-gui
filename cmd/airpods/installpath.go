package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Dardrai/airpods-gui/internal/winpath"
)

// installerPathFlag is for the Windows installer and its uninstaller, not for people, so it
// is in no help text: `airpods --installer-path add` puts the folder of this executable on
// the user's PATH, and `remove` takes it out. It takes no folder, so it cannot be made to add
// another one.
const installerPathFlag = "--installer-path"

// runInstallerPath handles the installer's call. handled is false for every other command
// line.
func runInstallerPath(args []string, stderr io.Writer, executable func() (string, error), update func(dir string, add bool) error) (code int, handled bool) {
	if len(args) != 2 || args[0] != installerPathFlag || (args[1] != "add" && args[1] != "remove") {
		return 0, false
	}
	exe, err := executable()
	if err == nil {
		err = update(filepath.Dir(exe), args[1] == "add")
	}
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1, true
	}
	return 0, true
}

func installerPath(args []string) (int, bool) {
	return runInstallerPath(args, os.Stderr, os.Executable, winpath.Update)
}
