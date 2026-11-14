package main

import (
	"bytes"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Dardrai/airpods-gui/internal/argparse"
)

// walkParsers visits every parser of the tree with its route, the words that lead to it.
func walkParsers(route string, parser *argparse.Parser, visit func(route string, parser *argparse.Parser)) {
	visit(route, parser)
	for name, sub := range parser.Parsers {
		walkParsers(strings.TrimSpace(route+" "+name), sub, visit)
	}
}

// allParserFlags gives each flag of the tree once, per route.
func allParserFlags() map[string][]*argparse.Action {
	flags := map[string][]*argparse.Action{}
	walkParsers("", buildParser(), func(route string, parser *argparse.Parser) {
		for _, action := range parser.Actions {
			if len(action.Flags) > 0 {
				flags[route] = append(flags[route], action)
			}
		}
	})
	return flags
}

func wholeWord(flag string) *regexp.Regexp {
	return regexp.MustCompile(`(^|[^-\w])` + regexp.QuoteMeta(flag) + `($|[^-\w])`)
}

// TestEveryFlagIsInItsHelp: the parser and the hand-kept help text must not drift apart.
func TestEveryFlagIsInItsHelp(t *testing.T) {
	seen := 0
	for route, actions := range allParserFlags() {
		text, ok := helpTexts[route]
		if !ok {
			t.Errorf("%q has no help text", route)
			continue
		}
		for _, action := range actions {
			for _, flag := range action.Flags {
				seen++
				if !wholeWord(flag).MatchString(text.body) {
					t.Errorf("%q: %s is not in the help body", route, flag)
				}
				if flag != "--help" && !wholeWord(flag).MatchString(text.usage) {
					t.Errorf("%q: %s is not in the usage", route, flag)
				}
			}
		}
	}
	if seen < 40 {
		t.Fatalf("walked only %d flags", seen)
	}
	routes := map[string]bool{}
	walkParsers("", buildParser(), func(route string, _ *argparse.Parser) { routes[route] = true })
	for route := range helpTexts {
		if !routes[route] {
			t.Errorf("help text for %q, which is no command", route)
		}
	}
}

// completionHas says whether a shell's script offers a flag at a route, in that shell's own syntax.
func completionHas(shell, script, route, flag string) bool {
	if shell == "fish" {
		switch flag {
		case "-h", "--help":
			return strings.Contains(script, "complete -c airpods -s h -l help\n")
		}
		pattern := "complete -c airpods -n '__airpods_at " + route + "' -l " + strings.TrimPrefix(flag, "--")
		for _, line := range strings.Split(script, "\n") {
			if line == pattern || strings.HasPrefix(line, pattern+" ") {
				return true
			}
		}
		return false
	}
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		var isArm bool
		if shell == "powershell" {
			isArm = strings.HasPrefix(trimmed, "'"+route+"' { ")
		} else {
			isArm = strings.HasPrefix(trimmed, "'"+route+"') ")
		}
		if isArm && wholeWord(flag).MatchString(trimmed) {
			return true
		}
	}
	// Every route without an arm of its own falls back to these two.
	return (flag == "-h" || flag == "--help") && wholeWord(flag).MatchString(script)
}

// TestEveryFlagIsInEveryCompletion: every parser flag, at its own route, in every shell's script.
func TestEveryFlagIsInEveryCompletion(t *testing.T) {
	for _, shell := range completionShells {
		var script bytes.Buffer
		writeCompletion(&script, shell)
		for route, actions := range allParserFlags() {
			for _, action := range actions {
				for _, flag := range action.Flags {
					if !completionHas(shell, script.String(), route, flag) {
						t.Errorf("%s: %s is not offered at %q", shell, flag, route)
					}
				}
			}
		}
	}
}

// TestFishFlagsStayAtTheirRoute: --mode used to be offered for audio set, and nowhere it worked.
func TestFishFlagsStayAtTheirRoute(t *testing.T) {
	var script bytes.Buffer
	writeCompletion(&script, "fish")
	audioSet := "complete -c airpods -n '__airpods_at audio set' -l "
	for _, line := range strings.Split(script.String(), "\n") {
		if name, found := strings.CutPrefix(line, audioSet); found {
			name, _, _ = strings.Cut(name, " ")
			if !slices.Contains([]string{"codec", "bitrate", "variable"}, name) {
				t.Errorf("audio set offers --%s", name)
			}
		}
	}
}

// TestCompletionTableMatchesParser: a flag that takes a value must be known as one, or the walk in
// each script would read its value as a command word, and its offered values must be its choices.
func TestCompletionTableMatchesParser(t *testing.T) {
	table := buildCompletionTable()
	values := map[string][]string{}
	for _, rule := range table.flagValues {
		values[rule.route] = rule.words
	}
	takesValue := map[string]bool{}
	for route, actions := range allParserFlags() {
		for _, action := range actions {
			if action.Kind != argparse.Store {
				continue
			}
			for _, flag := range action.Flags {
				takesValue[flag] = true
				if !slices.Contains(table.allFlags, flag) {
					t.Errorf("%s of %q is not in allFlags", flag, route)
				}
				if action.Choices != nil && !slices.Equal(values[flag], action.Choices) {
					t.Errorf("%s of %q offers %v, not %v", flag, route, values[flag], action.Choices)
				}
			}
		}
	}
	for _, flag := range table.allFlags {
		if !takesValue[flag] {
			t.Errorf("%s is in allFlags, but no parser gives it a value", flag)
		}
	}
	routes := allParserFlags()
	for _, rule := range table.flags {
		for _, flag := range rule.words {
			if !slices.ContainsFunc(routes[rule.route], func(a *argparse.Action) bool { return slices.Contains(a.Flags, flag) }) {
				t.Errorf("%q offers %s, which its parser does not know", rule.route, flag)
			}
		}
	}
}

// TestCompletionScriptsParse leaves out a shell that is not here. PowerShell has no such check.
func TestCompletionScriptsParse(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		path, err := osexec.LookPath(shell)
		if err != nil || runtime.GOOS == "windows" {
			t.Logf("no %s here", shell)
			continue
		}
		file := filepath.Join(t.TempDir(), "completion")
		var script bytes.Buffer
		writeCompletion(&script, shell)
		if err := os.WriteFile(file, script.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		if output, err := osexec.Command(path, "-n", file).CombinedOutput(); err != nil {
			t.Errorf("%s: %v: %s", shell, err, output)
		}
	}
}

// TestFishCompletion asks fish itself, as TestBashCompletion asks bash.
func TestFishCompletion(t *testing.T) {
	fish, err := osexec.LookPath("fish")
	if err != nil || runtime.GOOS == "windows" {
		t.Skip("no fish here")
	}
	source := filepath.Join(t.TempDir(), "airpods.fish")
	var script bytes.Buffer
	writeCompletion(&script, "fish")
	if err := os.WriteFile(source, script.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		line string
		want string
	}{
		{"airpods sta", "status"},
		{"airpods --", "--help --version --serial --json --debug"},
		{"airpods status --", "--help --details"},
		{"airpods info --", "--help --show-serial --capabilities"},
		{"airpods list --ser", "--serials"},
		{"airpods pair --", "--help --yes"},
		{"airpods hearing aid --", "--help --yes"},
		{"airpods hearing media-assist --", "--help --yes"},
		{"airpods hardware check --", "--help --buds-in-case"},
		{"airpods experimental --", "--help --experimental"},
		{"airpods experimental eq set --", "--help --experimental --enabled --low --mid --high"},
		{"airpods experimental eq ", "show set"},
		{"airpods reconnect-policy ", "manual auto"},
		{"airpods get gain", "gain-swipe"},
		{"airpods hearing profile apply --", "--help --file --left --right --amplification --balance --tone --conversation-boost --yes"},
		{"airpods audio set --", "--help --codec --bitrate --variable"},
		{"airpods status --cod", ""},
		{"airpods audio set --codec a", "auto aac aac-eld"},
		{"airpods audio set --codec aac --variable o", "on off"},
		{"airpods experimental eq set --enabled o", "on off"},
		{"airpods hearing recover --off m", "media-assist"},
		{"airpods nearby --seconds ", ""},
		{"airpods hearing profile apply --tone 1 --yes --conv", "--conversation-boost"},
		{"airpods --serial X get connect-", "connect-automatically"},
		{"airpods set mode tr", "transparency"},
	} {
		program := "source '" + source + "'; complete -C '" + tc.line + "'"
		output, err := osexec.Command(fish, "--no-config", "-c", program).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v: %s", tc.line, err, output)
		}
		var got []string
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			if word, _, _ := strings.Cut(line, "\t"); word != "" {
				got = append(got, word)
			}
		}
		slices.Sort(got)
		want := strings.Fields(tc.want)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%q: %q, want %q", tc.line, got, want)
		}
	}
}
