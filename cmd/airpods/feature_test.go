package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/core/dongle/sim"
	"github.com/Dardrai/airpods-gui/core/hid"
	"github.com/Dardrai/airpods-gui/internal/cliutil"
)

var update = flag.Bool("update", false, "rewrite testdata/features_golden.json from the current behaviour")

// fakeDongle is one attached dongle of a feature golden.
type fakeDongle struct {
	Serial   string          `json:"serial"`
	PID      uint16          `json:"pid"`
	Path     string          `json:"path"`
	Openable bool            `json:"openable"`
	Err      string          `json:"err,omitempty"`
	Sim      json.RawMessage `json:"sim,omitempty"`
}

// featureCase holds its inputs and, once recorded, the expected output and HID traffic.
type featureCase struct {
	Name    string          `json:"name"`
	Argv    []string        `json:"argv"`
	Sim     json.RawMessage `json:"sim,omitempty"`
	Dongles []fakeDongle    `json:"dongles"` // null means one simulated dongle from Sim
	Stdout  string          `json:"stdout"`
	Stderr  string          `json:"stderr"`
	Exit    int             `json:"exit"`
	Ops     int             `json:"ops"`
	Trace   string          `json:"trace"`
}

// bench is a set of simulated dongles behind the real open and enumerate contracts.
type bench struct {
	infos   []hid.Info
	devices []*sim.Device
}

func newBench(t *testing.T, dongles []fakeDongle) *bench {
	t.Helper()
	b := &bench{}
	for _, d := range dongles {
		cfg, err := sim.ParseConfig(d.Sim)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Serial = d.Serial
		info := hid.Info{Path: d.Path, Serial: d.Serial, PID: d.PID, Openable: d.Openable}
		if d.Err != "" {
			info.Err = errors.New(d.Err)
		}
		b.infos = append(b.infos, info)
		b.devices = append(b.devices, sim.New(cfg))
	}
	return b
}

func (b *bench) enumerate() ([]hid.Info, error) { return b.infos, nil }

// open applies the exactly-one rule of hid.Open to the openable dongles.
func (b *bench) open(serial string) (hid.Device, error) {
	var found []*sim.Device
	for i, info := range b.infos {
		if info.Openable && (serial == "" || info.Serial == serial) {
			found = append(found, b.devices[i])
		}
	}
	if len(found) != 1 {
		return nil, &hid.MatchError{Found: len(found), Text: "Expected exactly one matching Pico HID interface (CAFE:4015/4005)"}
	}
	return found[0].Open(serial)
}

// runFeature runs one case and returns what it printed and the HID traffic seen.
func runFeature(t *testing.T, tc featureCase) (stdout, stderr string, code, ops int, trace string) {
	t.Helper()
	b := newBench(t, tc.Dongles)
	env := environment{b.open, b.enumerate, sim.NewVirtualClock(), nil}
	var single *sim.Device
	if tc.Dongles == nil {
		cfg, err := sim.ParseConfig(tc.Sim)
		if err != nil {
			t.Fatal(err)
		}
		single = sim.New(cfg)
		b.devices = []*sim.Device{single}
		env.open = single.Open
	}
	var out, errOut bytes.Buffer
	code = runEnv(tc.Argv, &out, &errOut, env)
	summary := ""
	for i, device := range b.devices {
		count, hash := device.TraceSummary()
		if count > 0 && !device.Closed() {
			t.Errorf("dongle %d left open", i)
		}
		ops += count
		summary += hash[:16]
	}
	return out.String(), errOut.String(), code, ops, summary
}

// TestFeatureGolden checks stdout, stderr, the exit code and the exact HID traffic.
func TestFeatureGolden(t *testing.T) {
	const path = "testdata/features_golden.json"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cases []featureCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
	for i := range cases {
		tc := &cases[i]
		t.Run(tc.Name, func(t *testing.T) {
			stdout, stderr, code, ops, trace := runFeature(t, *tc)
			if *update {
				tc.Stdout, tc.Stderr, tc.Exit, tc.Ops, tc.Trace = stdout, stderr, code, ops, trace
				return
			}
			if code != tc.Exit {
				t.Errorf("exit = %d, want %d", code, tc.Exit)
			}
			if stdout != tc.Stdout {
				t.Errorf("stdout:\n%s\nwant:\n%s", stdout, tc.Stdout)
			}
			if stderr != tc.Stderr {
				t.Errorf("stderr:\n%s\nwant:\n%s", stderr, tc.Stderr)
			}
			if ops != tc.Ops || trace != tc.Trace {
				t.Errorf("traffic differs: %d ops, want %d", ops, tc.Ops)
			}
		})
	}
	if *update {
		var buffer bytes.Buffer
		encoder := json.NewEncoder(&buffer)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", " ")
		if err := encoder.Encode(cases); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, buffer.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestListTouchesOnlyStatusReport(t *testing.T) {
	b := newBench(t, []fakeDongle{
		{Serial: "A", PID: hid.ProductAudio, Path: "/dev/hidraw1", Openable: true},
		{Serial: "B", PID: hid.ProductControl, Path: "/dev/hidraw2", Openable: true},
		{Serial: "C", PID: hid.ProductAudio, Path: "/dev/hidraw3", Err: "denied"},
	})
	var out, errOut bytes.Buffer
	if code := runEnv([]string{"list"}, &out, &errOut, environment{b.open, b.enumerate, sim.NewVirtualClock(), nil}); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	for i, device := range b.devices[:2] {
		status, rpc, sets := device.Counts()
		if status != 1 || rpc != 0 || sets != 0 {
			t.Errorf("dongle %d: %d status reads, %d report 3 reads, %d writes", i, status, rpc, sets)
		}
		if !device.Closed() {
			t.Errorf("dongle %d left open", i)
		}
	}
	if ops, _ := b.devices[2].TraceSummary(); ops != 0 {
		t.Errorf("the dongle that cannot be opened saw %d operations", ops)
	}
}

// TestJSONShapes reads the documents as a script would: every documented field is present,
// and unknown values are null rather than missing.
func TestJSONShapes(t *testing.T) {
	read := func(cfg sim.Config, args ...string) map[string]any {
		t.Helper()
		_, stdout, stderr, _ := exec(t, cfg, args...)
		if stderr != "" {
			t.Fatalf("%v wrote to stderr: %s", args, stderr)
		}
		var document map[string]any
		if err := json.Unmarshal([]byte(stdout), &document); err != nil {
			t.Fatalf("%v: %v in %q", args, err, stdout)
		}
		if document["schema"] != float64(1) {
			t.Errorf("%v: schema = %v", args, document["schema"])
		}
		return document
	}
	status := read(sim.Default(), "--json", "status")
	batteries := status["batteries"].(map[string]any)
	if value, present := batteries["case"]; !present || value != nil {
		t.Errorf("case battery = %v, %v, want a null", value, present)
	}
	if batteries["left"] != float64(80) || status["state"] != "connected" || status["codec"] != "AAC" {
		t.Errorf("status = %v", status)
	}
	field := func(document map[string]any, key string) map[string]any { return document[key].(map[string]any) }
	mode := field(read(sim.Default(), "--json", "get", "mode"), "setting")
	if mode["raw"] != float64(2) || mode["decoded"] != "anc" || mode["age_ms"] != float64(1200) || mode["unknown"] != false {
		t.Errorf("mode = %v", mode)
	}
	missing := field(read(sim.Default(), "--json", "get", "microphone"), "setting")
	for _, key := range []string{"raw", "decoded", "age_ms"} {
		if value, present := missing[key]; !present || value != nil {
			t.Errorf("%s = %v, %v, want a null", key, value, present)
		}
	}
	if missing["unknown"] != true {
		t.Errorf("microphone = %v", missing)
	}
	strange := sim.Default()
	strange.Settings = []sim.Setting{{Control: 0x0d, Age: 5, Revision: 1, Value: 99}}
	odd := field(read(strange, "--json", "get", "mode"), "setting")
	if odd["raw"] != float64(99) || odd["decoded"] != nil || odd["age_ms"] != float64(5) || odd["unknown"] != true {
		t.Errorf("undecodable mode = %v", odd)
	}
}

func TestJSONKeepsWindowsPathReadable(t *testing.T) {
	path := `\\?\hid#vid_cafe&pid_4015#7&1`
	b := newBench(t, []fakeDongle{{Serial: "A", PID: hid.ProductAudio, Path: path, Openable: true}})
	var out, errOut bytes.Buffer
	if code := runEnv([]string{"--json", "list"}, &out, &errOut, environment{b.open, b.enumerate, sim.NewVirtualClock(), nil}); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "vid_cafe&pid_4015") {
		t.Errorf("escaped path: %s", out.String())
	}
}

func TestJSONFlagNeverOpensHardwareWhenRefused(t *testing.T) {
	open := func(string) (hid.Device, error) {
		t.Fatal("hardware opened")
		return nil, nil
	}
	for _, args := range [][]string{{"--json", "set", "mode", "off"}, {"--json", "reconnect"}, {"--json", "pair"}, {"--json", "pair-phone"}, {"--json", "disconnect"}, {"--json", "connect"}, {"--json", "reconnect-policy"}, {"--json", "help"}, {"status", "--json"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr, open); code != 2 || stdout.Len() != 0 {
			t.Errorf("%v: exit %d, stdout %q", args, code, stdout.String())
		}
	}
}

func TestListEnumerateFailure(t *testing.T) {
	env := environment{
		open:      func(string) (hid.Device, error) { t.Fatal("opened"); return nil, nil },
		enumerate: func() ([]hid.Info, error) { return nil, errors.New("boom") },
		clock:     sim.NewVirtualClock(),
	}
	var out, errOut bytes.Buffer
	if code := runEnv([]string{"list"}, &out, &errOut, env); code != 1 || out.Len() != 0 || !strings.Contains(errOut.String(), "Error: boom") {
		t.Errorf("got %d %q %q", code, out.String(), errOut.String())
	}
}

// nearbyReply is a verified, active finder snapshot at -58 dBm, heard 800 ms ago.
func nearbyReply(state byte) sim.Hex {
	reply := make([]byte, 24)
	reply[0], reply[1], reply[2], reply[3] = 1, state, 0xc6, 1
	binary.LittleEndian.PutUint32(reply[4:], 800)
	q8 := int32(-58 * 256)
	binary.LittleEndian.PutUint32(reply[20:], uint32(q8))
	return reply
}

// committedKinds returns the kinds of every typed request committed, in order.
func committedKinds(device *sim.Device) []byte {
	var kinds []byte
	for _, commit := range device.Commits() {
		if len(commit) > 0 {
			kinds = append(kinds, commit[0])
		}
	}
	return kinds
}

// stopClock is a virtual clock that closes a channel at its Nth sleep, like a Ctrl-C
// arriving while the finder runs.
type stopClock struct {
	*sim.VirtualClock
	at    int
	calls int
	stop  chan struct{}
}

func (c *stopClock) Sleep(d time.Duration) {
	c.calls++
	if c.calls == c.at {
		close(c.stop)
	}
	c.VirtualClock.Sleep(d)
}

func TestNearbyAlwaysStops(t *testing.T) {
	active := func() sim.Config {
		cfg := sim.Default()
		cfg.Replies = map[byte]sim.Hex{7: nearbyReply(0x81)}
		return cfg
	}
	for _, tc := range []struct {
		name      string
		cfg       func() sim.Config
		args      []string
		stopAt    int // the sleep that stops it; 0 never does
		wantExit  int
		wantStops int
		wantLine  string
	}{
		{name: "time bound", cfg: active, args: []string{"nearby", "--seconds", "2"}, wantExit: 0, wantStops: 1, wantLine: "nearby: stopped\n"},
		{name: "ctrl-c while searching", cfg: active, args: []string{"nearby"}, stopAt: 1, wantExit: 130, wantStops: 1, wantLine: "nearby: stopped\n"},
		{name: "snapshot fails", cfg: func() sim.Config {
			cfg := active()
			cfg.GetFailAt = 40
			cfg.FailMessage = "boom"
			return cfg
		}, args: []string{"nearby", "--seconds", "300"}, wantExit: 1, wantStops: 1},
		{name: "refused start sends no stop", cfg: func() sim.Config {
			cfg := active()
			cfg.Fail = map[byte]byte{6: 7}
			return cfg
		}, args: []string{"nearby"}, wantExit: 1, wantStops: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			device := sim.New(tc.cfg())
			stop := make(chan struct{})
			clock := &stopClock{VirtualClock: sim.NewVirtualClock(), at: tc.stopAt, stop: stop}
			env := environment{device.Open, nil, clock, func() <-chan struct{} { return stop }}
			var stdout, stderr bytes.Buffer
			code := runEnv(tc.args, &stdout, &stderr, env)
			if code != tc.wantExit {
				t.Errorf("exit = %d, want %d (%q %q)", code, tc.wantExit, stdout.String(), stderr.String())
			}
			stops := 0
			for _, kind := range committedKinds(device) {
				if kind == 8 {
					stops++
				}
			}
			if stops != tc.wantStops {
				t.Errorf("%d stops, want %d: kinds %v", stops, tc.wantStops, committedKinds(device))
			}
			if kinds := committedKinds(device); tc.wantStops > 0 && kinds[len(kinds)-1] != 8 {
				t.Errorf("the last request was %d, not the stop: %v", kinds[len(kinds)-1], kinds)
			}
			if !strings.HasSuffix(stdout.String(), tc.wantLine) {
				t.Errorf("stdout = %q, want suffix %q", stdout.String(), tc.wantLine)
			}
			if !device.Closed() {
				t.Error("device left open")
			}
		})
	}
}

// TestNearbyLeavesAnotherSearchAlone: a search this process did not start is not its to stop.
func TestNearbyLeavesAnotherSearchAlone(t *testing.T) {
	cfg := sim.Default()
	cfg.Replies = map[byte]sim.Hex{6: nearbyReply(0x81), 7: nearbyReply(0x81)}
	device := sim.New(cfg)
	// A first start makes the simulated finder active, as another app would have.
	dev, _ := device.Open("")
	client := dongle.NewClient(dev)
	client.SetClock(sim.NewVirtualClock())
	if _, err := client.NearbyStart(); err != nil {
		t.Fatal(err)
	}
	before := len(device.Commits())
	var stdout, stderr bytes.Buffer
	code := runEnv([]string{"nearby"}, &stdout, &stderr, environment{device.Open, nil, sim.NewVirtualClock(), nil})
	if code != 1 || !strings.Contains(stderr.String(), "Another app is already searching") {
		t.Errorf("got %d %q", code, stderr.String())
	}
	for _, kind := range committedKinds(device)[before:] {
		if kind == 6 || kind == 8 {
			t.Errorf("sent request kind %d", kind)
		}
	}
}

func TestNearbyInterruptedBeforeStartTouchesNothing(t *testing.T) {
	device := sim.New(sim.Default())
	stop := make(chan struct{})
	close(stop)
	var stdout, stderr bytes.Buffer
	code := runEnv([]string{"nearby"}, &stdout, &stderr, environment{device.Open, nil, sim.NewVirtualClock(), func() <-chan struct{} { return stop }})
	if code != cliutil.ExitInterrupted {
		t.Errorf("exit = %d", code)
	}
	for _, kind := range committedKinds(device) {
		if kind == 6 || kind == 8 {
			t.Errorf("sent request kind %d", kind)
		}
	}
}

// TestOnlyNearbyTakesOverSignals: hardware check is the one other command that asks; see its tests.
func TestOnlyNearbyTakesOverSignals(t *testing.T) {
	asked := 0
	env := environment{sim.New(sim.Default()).Open, nil, sim.NewVirtualClock(), func() <-chan struct{} { asked++; return nil }}
	for _, args := range [][]string{{"status"}, {"settings"}, {"get", "mode"}, {"set", "mode", "off"}, {"audio"}, {"perf"}, {"game", "on"}, {"reconnect"}, {"connect"}, {"reconnect-policy"}, {"info"}, {"hearing", "status"}, {"hardware", "status"}} {
		var stdout, stderr bytes.Buffer
		runEnv(args, &stdout, &stderr, env)
	}
	if asked != 0 {
		t.Errorf("%d commands took over signals", asked)
	}
}

// TestBitratesMatchCore keeps the table behind audio set's messages in step with the
// library that decides.
func TestBitratesMatchCore(t *testing.T) {
	candidates := []uint32{0, 1, 64000, 96000, 128000, 174000, 192000, 220000, 256000, 320000}
	for _, codec := range audioCodecs {
		for _, bitrate := range candidates {
			for _, variable := range []bool{false, true} {
				_, coreErr := dongle.AudioProfileRequest(codec.value, variable, bitrate)
				ourErr := checkProfile(codec.value, variable, bitrate)
				if (coreErr == nil) != (ourErr == nil) {
					t.Errorf("%s %d variable %v: core %v, ours %v", codec.word, bitrate, variable, coreErr, ourErr)
				}
			}
		}
	}
}

func TestCompletionNeverOpensHardware(t *testing.T) {
	open := func(string) (hid.Device, error) {
		t.Fatal("hardware opened")
		return nil, nil
	}
	for _, shell := range completionShells {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"completion", shell}, &stdout, &stderr, open); code != 0 || stdout.Len() == 0 || stderr.Len() != 0 {
			t.Errorf("%s: exit %d, stderr %q", shell, code, stderr.String())
		}
	}
}

// TestSerialsOpenNothing: completion runs this on every tab press.
func TestSerialsOpenNothing(t *testing.T) {
	b := newBench(t, []fakeDongle{{Serial: "A", PID: hid.ProductAudio, Path: "/dev/hidraw1", Openable: true}})
	env := environment{func(string) (hid.Device, error) { t.Fatal("hardware opened"); return nil, nil }, b.enumerate, sim.NewVirtualClock(), nil}
	var stdout, stderr bytes.Buffer
	if code := runEnv([]string{"list", "--serials"}, &stdout, &stderr, env); code != 0 || stdout.String() != "A\n" {
		t.Errorf("got %d %q %q", code, stdout.String(), stderr.String())
	}
}

// TestCompletionTableIsAccepted runs every offered word through what would parse it.
func TestCompletionTableIsAccepted(t *testing.T) {
	table := buildCompletionTable()
	sawSettings := 0
	for _, rule := range table.positionals {
		name, isSetting := strings.CutPrefix(rule.route, "set ")
		if !isSetting {
			continue
		}
		sawSettings++
		setting, ok := dongle.LookupAnySetting(name)
		if !ok {
			t.Fatalf("unknown setting %s", name)
		}
		for _, word := range rule.words {
			if _, err := setting.Encode(word); err != nil {
				t.Errorf("%s offers %q: %v", name, word, err)
			}
		}
	}
	if want := len(allSettings()) - 3; sawSettings != want { // the three percentages have no list
		t.Errorf("%d settings with values, want %d", sawSettings, want)
	}
	for _, word := range allBitrateWords() {
		if _, err := parseBitrate(word); err != nil {
			t.Errorf("bitrate %s: %v", word, err)
		}
	}
}

// TestBashCompletion sources the script in bash and asks it what a real tab press would.
func TestBashCompletion(t *testing.T) {
	bash, err := osexec.LookPath("bash")
	if err != nil || runtime.GOOS == "windows" {
		t.Skip("no bash here")
	}
	dir := t.TempDir()
	var script bytes.Buffer
	writeCompletion(&script, "bash")
	source := filepath.Join(dir, "completion.bash")
	if err := os.WriteFile(source, script.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	// A device naming itself like a command must not run when the serials are offered.
	fake := filepath.Join(dir, "airpods")
	body := "#!/bin/sh\necho SERIAL-A\necho SERIAL-B\necho '$(touch " + filepath.Join(dir, "ran") + ")'\n"
	if err := os.WriteFile(fake, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	complete := func(words ...string) []string {
		t.Helper()
		quoted := []string{"'" + fake + "'"}
		for _, word := range words {
			quoted = append(quoted, "'"+word+"'")
		}
		program := "source '" + source + "'; COMP_WORDS=(" + strings.Join(quoted, " ") + "); COMP_CWORD=" + strconv.Itoa(len(words)) + "; _airpods; printf '%s\\n' \"${COMPREPLY[@]}\""
		output, err := osexec.Command(bash, "-c", program).CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v: %s", words, err, output)
		}
		return strings.Fields(string(output))
	}
	for _, tc := range []struct {
		words []string
		want  string
	}{
		{[]string{"sta"}, "status"},
		{[]string{"com"}, "completion"},
		{[]string{"--j"}, "--json"},
		{[]string{"get", "connect-"}, "connect-automatically"},
		{[]string{"set", "mode", "tr"}, "transparency"},
		{[]string{"set", "allow-auto-connect", ""}, "on off"},
		{[]string{"set", "hold-actions", "siri"}, "siri,noise-control siri,siri"},
		{[]string{"set", "tone-volume", ""}, ""},
		{[]string{"--serial", "X", "set", "mode", "of"}, "off"},
		{[]string{"--serial", "SERIAL-"}, "SERIAL-A SERIAL-B"},
		{[]string{"--json", "get", "microph"}, "microphone"},
		{[]string{"help", "allow-"}, "allow-off allow-auto-connect"},
		{[]string{"audio", ""}, "set"},
		{[]string{"audio", "set", "--"}, "--help --codec --bitrate --variable"},
		{[]string{"audio", "set", "--codec", "aac"}, "aac aac-eld"},
		{[]string{"audio", "set", "--codec", "aac", "--bitrate", "25"}, "256000"},
		{[]string{"audio", "set", "--variable", "o"}, "on off"},
		{[]string{"game", ""}, "on off"},
		{[]string{"hearing", ""}, "status aid media-assist recover profile"},
		{[]string{"hearing", "profile", ""}, "show apply"},
		{[]string{"hearing", "profile", "apply", "--conv"}, "--conversation-boost"},
		{[]string{"hearing", "profile", "apply", "--tone", ""}, ""},
		{[]string{"hearing", "aid", ""}, "on off"},
		{[]string{"hardware", ""}, "status data check"},
		{[]string{"experimental", ""}, "transparency loud-sound-reduction eq rename"},
		{[]string{"experimental", "--experimental", "lou"}, "loud-sound-reduction"},
		{[]string{"experimental", "--experimental", "loud-sound-reduction", "set", ""}, "on off"},
		{[]string{"experimental", "eq", ""}, "show set"},
		{[]string{"experimental", "--experimental", "eq", "set", "--"}, "--help --experimental --enabled --low --mid --high"},
		{[]string{"experimental", "eq", "set", "--enabled", "o"}, "on off"},
		{[]string{"reconnect-policy", ""}, "manual auto"},
		{[]string{"reconnect-p"}, "reconnect-policy"},
		{[]string{"con"}, "connect"},
		{[]string{"get", "en352"}, "en352-protection"},
		{[]string{"set", "gain-swipe", ""}, "on off"},
		{[]string{"hardware", "check", "--b"}, "--buds-in-case"},
		{[]string{"hearing", "recover", "--off", "m"}, "media-assist"},
		{[]string{"nearby", "--s"}, "--seconds"},
		{[]string{"nearby", "--seconds", ""}, ""},
		{[]string{"list", "--s"}, "--serials"},
		{[]string{"completion", "p"}, "powershell"},
		{[]string{"status", "x"}, ""},
	} {
		if got := strings.Join(complete(tc.words...), " "); got != tc.want {
			t.Errorf("%v: %q, want %q", tc.words, got, tc.want)
		}
	}
	complete("--serial", "")
	if _, err := os.Stat(filepath.Join(dir, "ran")); err == nil {
		t.Error("a USB serial was run as a command")
	}
}
