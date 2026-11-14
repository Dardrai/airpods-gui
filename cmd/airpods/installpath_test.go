package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallerPath(t *testing.T) {
	exe := filepath.Join("apps", "AirPods", "airpods.exe")
	type call struct {
		dir string
		add bool
	}
	for _, tc := range []struct {
		name    string
		args    []string
		handled bool
		want    *call
	}{
		{"add", []string{installerPathFlag, "add"}, true, &call{filepath.Dir(exe), true}},
		{"remove", []string{installerPathFlag, "remove"}, true, &call{filepath.Dir(exe), false}},
		{"another word", []string{installerPathFlag, "show"}, false, nil},
		{"a folder of the caller's choice", []string{installerPathFlag, "add", "C:\\x"}, false, nil},
		{"an ordinary command", []string{"status"}, false, nil},
		{"nothing", nil, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got *call
			var stderr bytes.Buffer
			code, handled := runInstallerPath(tc.args, &stderr, func() (string, error) { return exe, nil },
				func(dir string, add bool) error { got = &call{dir, add}; return nil })
			if handled != tc.handled || code != 0 || stderr.Len() != 0 {
				t.Errorf("handled %v, exit %d, stderr %q", handled, code, stderr.String())
			}
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Errorf("update got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestInstallerPathFailureIsReported(t *testing.T) {
	var stderr bytes.Buffer
	code, handled := runInstallerPath([]string{installerPathFlag, "add"}, &stderr, func() (string, error) { return "x", nil },
		func(string, bool) error { return errors.New("access denied") })
	if !handled || code != 1 || stderr.String() != "Error: access denied\n" {
		t.Errorf("handled %v, exit %d, stderr %q", handled, code, stderr.String())
	}
}

// The installer calls the switch by name, for both words.
func TestInstallerUsesThePathSwitch(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "build", "windows", "installer.nsi"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`!define CLI_PATH_SWITCH "` + installerPathFlag + `"`,
		`!define CLI_EXE "airpods.exe"`,
		"StrCpy $R5 add", "StrCpy $R5 remove",
		`${CLI_PATH_SWITCH} remove'`,
	} {
		if !strings.Contains(string(script), want) {
			t.Errorf("build/windows/installer.nsi has no %s", want)
		}
	}
}
