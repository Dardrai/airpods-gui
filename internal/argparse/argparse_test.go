package argparse

import (
	"strings"
	"testing"
)

func TestRepr(t *testing.T) {
	for in, want := range map[string]string{
		"x": `'x'`, "it's": `"it's"`, `a"b`: `'a"b'`, `'"`: `'\'"'`, "t\tb": `'t\tb'`, `a\b`: `'a\\b'`, "é": `'é'`, "\x01": `'\x01'`, "\u200b": "'\\u200b'",
	} {
		if got := Repr(in); got != want {
			t.Errorf("Repr(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestParseInt(t *testing.T) {
	for in, want := range map[string]int{"5": 5, " 5 ": 5, "+5": 5, "-5": -5, "1_0": 10, "٣": 3, "007": 7} {
		if got, err := ParseInt(in); err != nil || got != want {
			t.Errorf("ParseInt(%q) = %d, %v", in, got, err)
		}
	}
	for _, in := range []string{"", " ", "+", "1__0", "_1", "1_", "1.5", "0x10", "abc", "--5"} {
		if _, err := ParseInt(in); err == nil {
			t.Errorf("ParseInt(%q) accepted", in)
		}
	}
	if got, err := ParseInt("99999999999999999999999"); err != nil || got < 1800 {
		t.Errorf("huge = %d, %v", got, err)
	}
}

func TestVersionActionExits(t *testing.T) {
	p := &Parser{Prog: "x", Usage: "usage: x\n", VersionMsg: "x v1\n", Actions: []*Action{
		{Kind: Version, Dest: "version", Flags: []string{"--version"}},
		{Kind: Store, Dest: "name", Required: true},
	}}
	_, exit := p.Parse([]string{"--version"})
	if exit == nil || exit.Code != 0 || exit.Stdout != "x v1\n" || exit.Stderr != "" {
		t.Fatalf("exit = %+v", exit)
	}
}

func parseTable() *Parser {
	sub := &Parser{Prog: "x sub", Usage: "usage: x sub\n", Actions: []*Action{
		{Kind: Help, Dest: "help", Flags: []string{"-h", "--help"}},
		{Kind: StoreTrue, Dest: "serials", Flags: []string{"--serials"}},
		{Kind: StoreTrue, Dest: "yes", Flags: []string{"--yes"}},
		{Kind: Store, Dest: "balance", Flags: []string{"--balance"}},
		{Kind: Store, Dest: "word", Nargs: NargsOptional},
	}}
	return &Parser{Prog: "x", Usage: "usage: x\n", HelpMsg: "usage: x\nhelp\n", VersionMsg: "x v1\n", Parsers: map[string]*Parser{"sub": sub},
		Actions: []*Action{
			{Kind: Help, Dest: "help", Flags: []string{"-h", "--help"}},
			{Kind: Version, Dest: "version", Flags: []string{"--version"}},
			{Kind: Store, Dest: "serial", Flags: []string{"--serial"}},
			{Kind: StoreTrue, Dest: "json", Flags: []string{"--json"}},
			{Kind: SubParsers, Dest: "command", Required: true, Choices: []string{"sub"}},
		}}
}

func TestParseTable(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want map[string]any // checked against the namespace when no error is expected
		err  string         // the last line of stderr when one is
	}{
		// "--": what follows it is never a flag.
		{"dashes make a positional", []string{"sub", "--", "--yes"}, map[string]any{"word": "--yes", "yes": false}, ""},
		{"dashes keep a negative", []string{"sub", "--", "-1"}, map[string]any{"word": "-1"}, ""},
		{"dashes before the command", []string{"--", "sub"}, map[string]any{"command": "sub"}, ""},
		{"dashes then an unknown command", []string{"--", "--json"}, nil, "x: error: argument command: invalid choice: '--json' (choose from sub)"},

		// An explicit value on a single-dash word.
		{"unknown short with a value", []string{"-x=val", "sub"}, nil, "x: error: unrecognized arguments: -x=val"},
		{"known short refuses a value", []string{"-h=val", "sub"}, nil, "x: error: argument -h/--help: ignored explicit argument 'val'"},
		{"long flag takes a value", []string{"--serial=abc", "sub"}, map[string]any{"serial": "abc"}, ""},
		{"long flag takes an empty value", []string{"--serial=", "sub"}, map[string]any{"serial": ""}, ""},
		{"store true refuses a value", []string{"sub", "--yes=1"}, nil, "x sub: error: argument --yes: ignored explicit argument '1'"},

		// Negative numbers.
		{"plain negative is a value", []string{"sub", "--balance", "-1"}, map[string]any{"balance": "-1"}, ""},
		{"negative decimal is a value", []string{"sub", "--balance", "-0.5"}, map[string]any{"balance": "-0.5"}, ""},
		{"negative without a leading digit", []string{"sub", "--balance", "-.5"}, map[string]any{"balance": "-.5"}, ""},
		{"negative positional", []string{"sub", "-5"}, map[string]any{"word": "-5"}, ""},
		{"exponent with an equals sign", []string{"sub", "--balance=-1e-1"}, map[string]any{"balance": "-1e-1"}, ""},
		{"list with an equals sign", []string{"sub", "--balance=-1,2,3"}, map[string]any{"balance": "-1,2,3"}, ""},
		{"a value with a space is a value", []string{"sub", "--balance", "-1 2"}, map[string]any{"balance": "-1 2"}, ""},

		// A value that reads as a flag gets a hint; nothing to hint at gets none.
		{"exponent reads as a flag", []string{"sub", "--balance", "-1e-1"}, nil, "x sub: error: argument --balance: expected one argument (use --balance=VALUE if the value starts with -)"},
		{"list reads as a flag", []string{"sub", "--balance", "-1,2,3"}, nil, "x sub: error: argument --balance: expected one argument (use --balance=VALUE if the value starts with -)"},
		{"an unknown flag follows", []string{"sub", "--balance", "--bogus"}, nil, "x sub: error: argument --balance: expected one argument (use --balance=VALUE if the value starts with -)"},
		// A known flag is not a value that starts with -: the value is missing.
		{"another flag follows", []string{"sub", "--balance", "--yes"}, nil, "x sub: error: argument --balance: expected one argument"},
		{"another flag with a value follows", []string{"sub", "--balance", "--balance=1"}, nil, "x sub: error: argument --balance: expected one argument"},
		{"a short flag follows", []string{"sub", "--balance", "-h"}, nil, "x sub: error: argument --balance: expected one argument"},
		{"another flag follows at the top level", []string{"--serial", "--json", "sub"}, nil, "x: error: argument --serial: expected one argument"},
		{"at the top level", []string{"--serial", "-x", "sub"}, nil, "x: error: argument --serial: expected one argument (use --serial=VALUE if the value starts with -)"},
		{"no word follows", []string{"sub", "--balance"}, nil, "x sub: error: argument --balance: expected one argument"},

		// Nothing is abbreviated any more.
		{"short of a store true", []string{"sub", "--y"}, nil, "x: error: unrecognized arguments: --y"},
		{"short of a word with a value", []string{"sub", "--bal", "1"}, nil, "x: error: unrecognized arguments: --bal"},
		{"short with an equals sign", []string{"sub", "--bal=1"}, nil, "x: error: unrecognized arguments: --bal=1"},
		{"short before the command", []string{"--ser", "abc", "sub"}, nil, "x: error: unrecognized arguments: --ser"},
		{"short of the help flag", []string{"--he"}, nil, "x: error: unrecognized arguments: --he"},
		{"a word fitting two flags", []string{"sub", "--se"}, nil, "x: error: unrecognized arguments: --se"},
		{"a single dash prefix", []string{"-=x", "sub"}, nil, "x: error: unrecognized arguments: -=x"},

		// A word that reads as a flag and is none is reported before a later error, which it often caused.
		{"mistyped flag whose value reads as the command", []string{"--seria", "X", "sub"}, nil, "x: error: unrecognized arguments: --seria"},
		{"mistyped flag and no command", []string{"--jsno"}, nil, "x: error: unrecognized arguments: --jsno"},
		{"mistyped flag with a value and no command", []string{"--seria=X"}, nil, "x: error: unrecognized arguments: --seria=X"},
		{"mistyped flag before a failing flag", []string{"--jsno", "--serial"}, nil, "x: error: unrecognized arguments: --jsno"},
		{"mistyped flag before a failing subcommand", []string{"--jsno", "sub", "--balance"}, nil, "x: error: unrecognized arguments: --jsno"},
		{"mistyped flags of both parsers", []string{"--jsno", "sub", "--bogus", "--balance"}, nil, "x: error: unrecognized arguments: --jsno --bogus"},
		{"mistyped flag of the subcommand", []string{"sub", "--bogus", "--yes=1"}, nil, "x: error: unrecognized arguments: --bogus"},
		{"parent flag before a failing subcommand flag", []string{"sub", "--json", "--balance"}, nil, "x: error: --json must come before the command"},
		{"an earlier error stays first", []string{"sub", "--yes=1", "--bogus"}, nil, "x sub: error: argument --yes: ignored explicit argument '1'"},
		{"mistyped flag after the failing word", []string{"nope", "--bogus"}, nil, "x: error: argument command: invalid choice: 'nope' (choose from sub)"},
		{"help wins over a mistyped flag", []string{"--bogus", "-h"}, nil, ""},
		{"a negative number is no flag", []string{"-5"}, nil, "x: error: argument command: invalid choice: '-5' (choose from sub)"},

		// A flag of the parent after the command.
		{"store flag after the command", []string{"sub", "--serial", "abc"}, nil, "x: error: --serial must come before the command"},
		{"store flag with an equals sign", []string{"sub", "--serial=abc"}, nil, "x: error: --serial must come before the command"},
		{"store true after the command", []string{"sub", "--json"}, nil, "x: error: --json must come before the command"},
		{"version after the command", []string{"sub", "--version"}, nil, "x: error: --version must come before the command"},
		{"before an unknown one", []string{"sub", "--bogus", "--json"}, nil, "x: error: --json must come before the command"},
		{"own flags still work", []string{"--json", "--serial", "abc", "sub", "--serials"}, map[string]any{"json": true, "serial": "abc", "serials": true}, ""},
		{"short help still works after the command", []string{"sub", "-h"}, nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ns, exit := parseTable().Parse(tc.args)
			if tc.err == "" && tc.want == nil {
				if exit == nil || exit.Code != 0 {
					t.Fatalf("exit = %+v", exit)
				}
				return
			}
			if tc.err != "" {
				if exit == nil || exit.Code != 2 {
					t.Fatalf("exit = %+v, ns %v", exit, ns)
				}
				lines := strings.Split(strings.TrimSpace(exit.Stderr), "\n")
				if got := lines[len(lines)-1]; got != tc.err {
					t.Fatalf("stderr %q, want %q", got, tc.err)
				}
				return
			}
			if exit != nil {
				t.Fatalf("exit = %+v", exit)
			}
			for key, want := range tc.want {
				if ns[key] != want {
					t.Errorf("%s = %v, want %v", key, ns[key], want)
				}
			}
		})
	}
}
