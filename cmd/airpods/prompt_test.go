package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Dardrai/airpods-gui/core/dongle/sim"
)

func TestIsTerminal(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	if isTerminal(devNull) {
		t.Error("the null device counts as a terminal")
	}
	file, err := os.Create(filepath.Join(t.TempDir(), "plain"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if isTerminal(file) {
		t.Error("a file counts as a terminal")
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	if isTerminal(reader) {
		t.Error("a pipe counts as a terminal")
	}
	if runtime.GOOS != "linux" {
		return
	}
	// The master side of a new pseudo-terminal, so that the check also says yes to something.
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal here: %v", err)
	}
	defer master.Close()
	if !isTerminal(master) {
		t.Error("a pseudo-terminal does not count as a terminal")
	}
}

// TestNullDeviceAsksNobody: airpods pair </dev/null used to print the question, then the error on its line.
func TestNullDeviceAsksNobody(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	stderr := scratchFile(t)
	if yes, available := askOn(devNull, stderr, "Replace the saved pairing?"); yes || available || written(t, stderr) != "" {
		t.Errorf("yes %v, available %v, stderr %q", yes, available, written(t, stderr))
	}
}

// scratchFile stands in for a stderr that was redirected.
func scratchFile(t *testing.T) *os.File {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	return file
}

func written(t *testing.T, file *os.File) string {
	t.Helper()
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestHiddenQuestionAsksNobody: airpods pair 2>/dev/null used to wait for an answer to a question
// nobody could see. Before the fix this test waits on the terminal until it times out.
func TestHiddenQuestionAsksNobody(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs a pseudo-terminal")
	}
	terminal, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no pseudo-terminal here: %v", err)
	}
	defer terminal.Close()
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	for name, stderr := range map[string]*os.File{"the null device": devNull, "a file": scratchFile(t)} {
		if yes, available := askOn(terminal, stderr, "Replace the saved pairing?"); yes || available {
			t.Errorf("stderr to %s: yes %v, available %v", name, yes, available)
		}
		if name == "a file" && written(t, stderr) != "" {
			t.Errorf("the question went to the file: %q", written(t, stderr))
		}
	}
}

func TestAskLine(t *testing.T) {
	for _, tc := range []struct {
		input  string
		yes    bool
		stderr string
	}{
		{"y\n", true, "Q? [y/N] "},
		{"YES\n", true, "Q? [y/N] "},
		{"y", true, "Q? [y/N] "},
		{"n\n", false, "Q? [y/N] "},
		{"\n", false, "Q? [y/N] "},
		{"maybe\n", false, "Q? [y/N] "},
		{"", false, "Q? [y/N] \n"}, // Ctrl-D leaves the cursor after the question.
	} {
		var stderr bytes.Buffer
		if yes := askLine(strings.NewReader(tc.input), &stderr, "Q?"); yes != tc.yes || stderr.String() != tc.stderr {
			t.Errorf("%q: yes %v, stderr %q", tc.input, yes, stderr.String())
		}
	}
}

func TestPairWithNobodyToAsk(t *testing.T) {
	old := askUser
	defer func() { askUser = old }()
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	question := scratchFile(t)
	askUser = func(q string) (bool, bool) { return askOn(stdin, question, q) }
	device, stdout, stderr, code := exec(t, sim.Default(), "pair")
	if code != 1 || stdout != "" || written(t, question) != "" || len(device.Controls()) != 0 {
		t.Errorf("exit %d, stdout %q, question %q, sent %d", code, stdout, written(t, question), len(device.Controls()))
	}
	if want := "Error: Nothing was sent: pairing needs --yes, or a terminal to confirm in.\n"; !strings.HasSuffix(stderr, want) {
		t.Errorf("stderr %q", stderr)
	}
}
