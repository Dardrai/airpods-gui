// Package argparse is a port of the parts of Python 3.13's argparse that the two
// command-line tools use, so "--", negative numbers, message wording and error order
// behave the same. It acts as allow_abbrev=False does: a flag must be written in full,
// because a guessed one can do something the user never asked for. Usage and help text
// are supplied by the caller.
package argparse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type actionKind int

// Action kinds.
const (
	Help actionKind = iota
	Version
	Store
	StoreTrue
	SubParsers
)

// Nargs values for positionals and options.
const (
	NargsOne      = ""
	NargsOptional = "?"
)

// Action is one option or positional.
type Action struct {
	Kind     actionKind
	Dest     string
	Flags    []string // empty for positionals
	Nargs    string
	Required bool
	Choices  []string
	// Convert, when set, converts the string; TypeName names it in the error.
	Convert  func(string) (any, error)
	TypeName string
	Default  any
}

// Parser is one parser or subparser.
type Parser struct {
	Prog    string
	Usage   string
	HelpMsg string
	// VersionMsg is printed by the Version action.
	VersionMsg string
	Actions    []*Action
	// Parsers maps subcommand names for the SubParsers action.
	Parsers map[string]*Parser
}

// Namespace holds parsed values. A nil or absent value is Python's None.
type Namespace map[string]any

// String returns a string value and whether it is set.
func (n Namespace) String(dest string) (string, bool) {
	value, ok := n[dest].(string)
	return value, ok
}

// Bool returns a store_true value.
func (n Namespace) Bool(dest string) bool {
	value, _ := n[dest].(bool)
	return value
}

// Exit is argparse deciding to end the process.
type Exit struct {
	Code   int
	Stdout string
	Stderr string
}

// Error implements Parser.error: usage on stderr, the message, exit 2.
func (p *Parser) Error(message string) *Exit {
	return &Exit{Code: 2, Stderr: p.Usage + p.Prog + ": error: " + message + "\n"}
}

// Parse is parse_args. It returns a non-nil Exit when parsing ended the program.
func (p *Parser) Parse(args []string) (ns Namespace, exit *Exit) {
	defer func() {
		switch r := recover().(type) {
		case nil:
		case *Exit:
			ns, exit = nil, r
		case unknownFlags:
			ns, exit = nil, p.leftOver(r)
		default:
			panic(r)
		}
	}()
	ns, extras := p.parseKnown(args)
	if len(extras) > 0 {
		return nil, p.leftOver(extras)
	}
	return ns, nil
}

// leftOver is the error for words no parser took.
func (p *Parser) leftOver(extras []string) *Exit {
	// Only a subcommand leaves this parser's own flag over, so it came after the command.
	for _, extra := range extras {
		if option, _, _ := partition(extra); p.flag(option) != nil {
			return p.Error(option + " must come before the command")
		}
	}
	return p.Error("unrecognized arguments: " + strings.Join(extras, " "))
}

// unknownFlags ends parsing at an error that came after words that read as flags and are
// none. They are reported instead: a mistyped flag is the likelier mistake, and it is often
// what caused the later error, as when its value is then read as the command.
type unknownFlags []string

func (a *Action) name() string {
	if len(a.Flags) > 0 {
		return strings.Join(a.Flags, "/")
	}
	return a.Dest
}

func (p *Parser) fail(message string) { panic(p.Error(message)) }

func (p *Parser) failAction(a *Action, message string) {
	p.fail("argument " + a.name() + ": " + message)
}

func (p *Parser) flag(option string) *Action {
	for _, a := range p.Actions {
		for _, f := range a.Flags {
			if f == option {
				return a
			}
		}
	}
	return nil
}

type optionTuple struct {
	action   *Action
	option   string
	sep      *string
	explicit *string
}

func strp(s string) *string { return &s }

func partition(s string) (string, *string, *string) {
	before, after, found := strings.Cut(s, "=")
	if !found {
		return s, nil, nil
	}
	return before, strp("="), &after
}

var negativeNumber = regexp.MustCompile(`^-\p{Nd}+$|^-\p{Nd}*\.\p{Nd}+$`)

// parseOptional returns nil when the argument is meant to be positional.
func (p *Parser) parseOptional(arg string) *optionTuple {
	if arg == "" || arg[0] != '-' {
		return nil
	}
	if a := p.flag(arg); a != nil {
		return &optionTuple{a, arg, nil, nil}
	}
	if len([]rune(arg)) == 1 {
		return nil
	}
	if option, sep, explicit := partition(arg); sep != nil {
		if a := p.flag(option); a != nil {
			return &optionTuple{a, option, sep, explicit}
		}
	}
	if t := p.shortWithValue(arg); t != nil {
		return t
	}
	if negativeNumber.MatchString(arg) {
		return nil
	}
	if strings.Contains(arg, " ") {
		return nil
	}
	return &optionTuple{nil, arg, nil, nil}
}

// shortWithValue reads -xVALUE, and -xy as -x -y: only a one-letter flag joins what follows it.
func (p *Parser) shortWithValue(arg string) *optionTuple {
	runes := []rune(arg)
	if runes[1] == '-' {
		return nil
	}
	short := string(runes[:2])
	a := p.flag(short)
	if a == nil {
		return nil
	}
	rest := string(runes[2:])
	return &optionTuple{a, short, strp(""), &rest}
}

func nargsPattern(a *Action) string {
	option := len(a.Flags) > 0
	switch a.Kind {
	case Help, Version, StoreTrue:
		return "()"
	case SubParsers:
		return "(-*A[-AO]*)"
	}
	if a.Nargs == NargsOptional {
		if option {
			return "(A?)"
		}
		return "(-*A?-*)"
	}
	if option {
		return "([A])"
	}
	return "(-*A-*)"
}

// matchArgument counts the words a takes from pattern. next is the word after the flag, if any.
func (p *Parser) matchArgument(a *Action, pattern, next string) int {
	match := regexp.MustCompile("^" + nargsPattern(a)).FindStringSubmatch(pattern)
	if match == nil {
		msg := "expected one argument"
		if a.Nargs == NargsOptional {
			msg = "expected at most one argument"
		} else if t := p.parseOptional(next); t != nil && t.action == nil {
			// A value such as -1e-1 reads as a flag, since only plain decimals read as negative
			// numbers. A flag of this parser is no value, so the value is simply missing.
			msg += fmt.Sprintf(" (use %s=VALUE if the value starts with -)", a.Flags[0])
		}
		p.failAction(a, msg)
	}
	return len(match[1])
}

func matchPartial(actions []*Action, pattern string) []int {
	for i := len(actions); i > 0; i-- {
		joined := ""
		for _, a := range actions[:i] {
			joined += nargsPattern(a)
		}
		match := regexp.MustCompile("^" + joined).FindStringSubmatchIndex(pattern)
		if match == nil {
			continue
		}
		var result []int
		for g := 1; g*2 < len(match); g++ {
			result = append(result, match[g*2+1]-match[g*2])
		}
		if match[1] < len(pattern) && pattern[match[1]] == 'O' {
			for len(result) > 0 && result[len(result)-1] == 0 {
				result = result[:len(result)-1]
			}
		}
		return result
	}
	return nil
}

func (p *Parser) checkChoice(a *Action, value string) {
	if a.Kind == SubParsers {
		if _, ok := p.Parsers[value]; ok {
			return
		}
	} else if a.Choices == nil {
		return
	}
	for _, c := range a.Choices {
		if c == value {
			return
		}
	}
	p.failAction(a, fmt.Sprintf("invalid choice: %s (choose from %s)", Repr(value), strings.Join(a.Choices, ", ")))
}

func (p *Parser) value(a *Action, arg string) any {
	if a.Convert == nil {
		return arg
	}
	value, err := a.Convert(arg)
	if err != nil {
		p.failAction(a, fmt.Sprintf("invalid %s value: %s", a.TypeName, Repr(arg)))
	}
	return value
}

func (p *Parser) takeAction(a *Action, args []string, ns Namespace, unrecognized *[]string) {
	switch a.Kind {
	case Help:
		panic(&Exit{Code: 0, Stdout: p.HelpMsg})
	case Version:
		panic(&Exit{Code: 0, Stdout: p.VersionMsg})
	case StoreTrue:
		ns[a.Dest] = true
	case SubParsers:
		p.checkChoice(a, args[0])
		ns[a.Dest] = args[0]
		sub := p.Parsers[args[0]]
		subNS, extras := sub.parseKnown(args[1:])
		for k, v := range subNS {
			ns[k] = v
		}
		*unrecognized = append(*unrecognized, extras...)
	default:
		if len(args) == 0 {
			ns[a.Dest] = a.Default
			return
		}
		value := p.value(a, args[0])
		p.checkChoice(a, args[0])
		ns[a.Dest] = value
	}
}

func removeFirstDashes(args []string) []string {
	for i, s := range args {
		if s == "--" {
			return append(append([]string{}, args[:i]...), args[i+1:]...)
		}
	}
	return args
}

func (p *Parser) parseKnown(argv []string) (Namespace, []string) {
	// unknown holds the words taken so far that read as flags and are none.
	var unknown []string
	defer func() {
		switch r := recover().(type) {
		case nil:
		case *Exit:
			if r.Code == 0 || len(unknown) == 0 {
				panic(r)
			}
			panic(unknownFlags(unknown))
		case unknownFlags: // From a subcommand, whose words came after this parser's.
			panic(append(unknownFlags(unknown), r...))
		default:
			panic(r)
		}
	}()
	ns := Namespace{}
	for _, a := range p.Actions {
		if a.Kind != Help && a.Default != nil {
			ns[a.Dest] = a.Default
		}
		if a.Kind == StoreTrue {
			ns[a.Dest] = false
		}
	}

	indices := map[int]*optionTuple{}
	var patternBuilder strings.Builder
	maxOption := -1
	for i := 0; i < len(argv); i++ {
		if argv[i] == "--" {
			patternBuilder.WriteByte('-')
			for i++; i < len(argv); i++ {
				patternBuilder.WriteByte('A')
			}
			break
		}
		if t := p.parseOptional(argv[i]); t == nil {
			patternBuilder.WriteByte('A')
		} else {
			indices[i] = t
			patternBuilder.WriteByte('O')
			maxOption = i
		}
	}
	pattern := patternBuilder.String()

	seen := map[*Action]bool{}
	var extras, unrecognized []string
	take := func(a *Action, args []string) {
		seen[a] = true
		p.takeAction(a, args, ns, &unrecognized)
	}

	consumeOptional := func(start int) int {
		t := indices[start]
		action, option, sep, explicit := t.action, t.option, t.sep, t.explicit
		type taken struct {
			a    *Action
			args []string
		}
		var actions []taken
		stop := 0
	loop:
		for {
			if action == nil {
				extras = append(extras, argv[start])
				unknown = append(unknown, argv[start])
				return start + 1
			}
			if explicit != nil {
				count := p.matchArgument(action, "A", "")
				switch {
				case count == 0 && option[1] != '-' && *explicit != "":
					if (sep != nil && *sep != "") || (*explicit)[0] == '-' {
						p.failAction(action, "ignored explicit argument "+Repr(*explicit))
					}
					actions = append(actions, taken{action, nil})
					first := []rune(*explicit)[0]
					option = string([]rune{'-', first})
					if next := p.flag(option); next != nil {
						action = next
						rest := string([]rune(*explicit)[1:])
						switch {
						case rest == "":
							sep, explicit = nil, nil
						case rest[0] == '=':
							sep, explicit = strp("="), strp(rest[1:])
						default:
							sep, explicit = strp(""), &rest
						}
					} else {
						extras = append(extras, "-"+*explicit)
						unknown = append(unknown, "-"+*explicit)
						stop = start + 1
						break loop
					}
				case count == 1:
					stop = start + 1
					actions = append(actions, taken{action, []string{*explicit}})
					break loop
				default:
					p.failAction(action, "ignored explicit argument "+Repr(*explicit))
				}
			} else {
				from := start + 1
				next := ""
				if from < len(argv) {
					next = argv[from]
				}
				count := p.matchArgument(action, pattern[min(from, len(pattern)):], next)
				stop = from + count
				actions = append(actions, taken{action, argv[from:stop]})
				break loop
			}
		}
		for _, item := range actions {
			take(item.a, item.args)
		}
		return stop
	}

	var positionals []*Action
	for _, a := range p.Actions {
		if len(a.Flags) == 0 {
			positionals = append(positionals, a)
		}
	}
	consumePositionals := func(start int) int {
		counts := matchPartial(positionals, pattern[min(start, len(pattern)):])
		for i, count := range counts {
			a := positionals[i]
			args := argv[start : start+count]
			if a.Kind == SubParsers {
				if pattern[start] == '-' {
					args = removeFirstDashes(args)
				}
			} else if strings.Contains(pattern[start:start+count], "-") {
				args = removeFirstDashes(args)
			}
			start += count
			take(a, args)
		}
		positionals = positionals[len(counts):]
		return start
	}

	start := 0
	for start <= maxOption {
		next := start
		for next <= maxOption {
			if _, ok := indices[next]; ok {
				break
			}
			next++
		}
		if start != next {
			end := consumePositionals(start)
			if end > start {
				start = end
				continue
			}
			start = end
		}
		if _, ok := indices[start]; !ok {
			extras = append(extras, argv[start:next]...)
			start = next
		}
		start = consumeOptional(start)
	}
	stop := consumePositionals(start)
	extras = append(extras, argv[stop:]...)

	var required []string
	for _, a := range p.Actions {
		if !seen[a] && a.Required {
			required = append(required, a.name())
		}
	}
	if len(required) > 0 {
		p.fail("the following arguments are required: " + strings.Join(required, ", "))
	}
	return ns, append(extras, unrecognized...)
}

// Repr is Python's repr() of a string.
func Repr(s string) string {
	quote := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		quote = '"'
	}
	var b strings.Builder
	b.WriteByte(quote)
	for _, r := range s {
		switch {
		case r == rune(quote) || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r >= 0x80 && !unicode.IsPrint(r):
			switch {
			case r <= 0xff:
				fmt.Fprintf(&b, `\x%02x`, r)
			case r <= 0xffff:
				fmt.Fprintf(&b, `\u%04x`, r)
			default:
				fmt.Fprintf(&b, `\U%08x`, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte(quote)
	return b.String()
}

// ParseInt is Python's int(str): optional whitespace and sign, Unicode decimal digits
// and single underscores between digits. Huge values saturate instead of failing.
func ParseInt(s string) (int, error) {
	invalid := fmt.Errorf("invalid literal for int(): %s", strconv.Quote(s))
	s = strings.TrimFunc(s, unicode.IsSpace)
	negative := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		negative = s[0] == '-'
		s = s[1:]
	}
	if s == "" {
		return 0, invalid
	}
	const saturate = 1 << 40
	value, previousUnderscore := 0, true
	for _, r := range s {
		if r == '_' {
			if previousUnderscore {
				return 0, invalid
			}
			previousUnderscore = true
			continue
		}
		digit := -1
		if r >= '0' && r <= '9' {
			digit = int(r - '0')
		} else if unicode.Is(unicode.Nd, r) {
			digit = decimalDigit(r)
		}
		if digit < 0 {
			return 0, invalid
		}
		previousUnderscore = false
		if value < saturate {
			value = value*10 + digit
		}
	}
	if previousUnderscore {
		return 0, invalid
	}
	if negative {
		value = -value
	}
	return value, nil
}

// decimalDigit relies on Unicode Nd ranges being consecutive runs of ten from zero.
func decimalDigit(r rune) int {
	for _, rng := range unicode.Nd.R16 {
		if rune(rng.Lo) <= r && r <= rune(rng.Hi) {
			return int(r-rune(rng.Lo)) % 10
		}
	}
	for _, rng := range unicode.Nd.R32 {
		if rune(rng.Lo) <= r && r <= rune(rng.Hi) {
			return int(r-rune(rng.Lo)) % 10
		}
	}
	return -1
}
