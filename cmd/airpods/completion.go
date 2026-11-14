package main

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/Dardrai/airpods-gui/core/dongle"
)

var completionShells = []string{"bash", "zsh", "fish", "powershell"}

// completionRule offers words at a route: the command-line words so far without flags and
// their values, joined by spaces.
type completionRule struct {
	route string
	words []string
}

// completionTable is what every shell's script is written from.
type completionTable struct {
	positionals []completionRule
	flags       []completionRule
	flagValues  []completionRule // the route is the flag; --serial is added by each script
	allFlags    []string         // every flag that takes a value
}

func buildCompletionTable() completionTable {
	cli := buildParser()
	var commands []string
	for _, action := range cli.Actions {
		if len(action.Flags) == 0 {
			commands = action.Choices
		}
	}
	names := settingNames()
	table := completionTable{
		positionals: []completionRule{
			{"", commands},
			{"help", append(slices.Clone(commands), names...)},
			{"completion", completionShells},
			{"get", names},
			{"set", names},
			{"audio", []string{"set"}},
			{"game", []string{"on", "off"}},
			{"hearing", []string{"status", "aid", "media-assist", "recover", "profile"}},
			{"hearing profile", []string{"show", "apply"}},
			{"hearing aid", []string{"on", "off"}},
			{"experimental", []string{"transparency", "loud-sound-reduction", "eq", "rename"}},
			{"experimental transparency", []string{"show", "set"}},
			{"experimental loud-sound-reduction", []string{"get", "set"}},
			{"experimental loud-sound-reduction set", []string{"on", "off"}},
			{"experimental eq", []string{"show", "set"}},
			{"reconnect-policy", []string{"manual", "auto"}},
			{"hardware", []string{"status", "data", "check"}},
			{"hearing media-assist", []string{"on", "off"}},
		},
		flags: []completionRule{
			{"", []string{"-h", "--help", "--version", "--serial", "--json", "--debug"}},
			{"status", []string{"-h", "--help", "--details"}},
			{"list", []string{"-h", "--help", "--serials"}},
			{"audio set", []string{"-h", "--help", "--codec", "--bitrate", "--variable"}},
			{"nearby", []string{"-h", "--help", "--seconds"}},
			{"info", []string{"-h", "--help", "--show-serial", "--capabilities"}},
			{"pair", []string{"-h", "--help", "--yes"}},
			{"hearing aid", []string{"-h", "--help", "--yes"}},
			{"hearing media-assist", []string{"-h", "--help", "--yes"}},
			{"hearing recover", []string{"-h", "--help", "--off"}},
			{"experimental", []string{"-h", "--help", "--experimental"}},
			{"experimental transparency show", []string{"-h", "--help", "--experimental"}},
			{"experimental transparency set", []string{"-h", "--help", "--experimental", "--enabled", "--ear", "--amplification", "--tone", "--noise-reduction", "--conversation-boost"}},
			{"experimental loud-sound-reduction get", []string{"-h", "--help", "--experimental"}},
			{"experimental loud-sound-reduction set", []string{"-h", "--help", "--experimental"}},
			{"experimental eq show", []string{"-h", "--help", "--experimental"}},
			{"experimental eq set", []string{"-h", "--help", "--experimental", "--enabled", "--low", "--mid", "--high"}},
			{"experimental rename", []string{"-h", "--help", "--experimental"}},
			{"hardware check", []string{"-h", "--help", "--buds-in-case"}},
			{"diag", []string{"-h", "--help", "--version", "--seconds", "--serial", "--output", "--legacy", "--inspect", "--phone", "--radio", "--no-trace", "--replace"}},
			{"hearing profile apply", []string{"-h", "--help", "--file", "--left", "--right", "--amplification", "--balance", "--tone", "--conversation-boost", "--yes"}},
		},
		flagValues: []completionRule{
			{"--codec", codecWords()},
			{"--bitrate", allBitrateWords()},
			{"--variable", []string{"on", "off"}},
			{"--off", []string{"hearing-aid", "media-assist"}},
			{"--enabled", []string{"on", "off"}},
			{"--ear", []string{"left", "right", "both"}},
			{"--conversation-boost", []string{"on", "off"}},
		},
		allFlags: []string{"--serial", "--codec", "--bitrate", "--variable", "--seconds", "--off", "--file", "--left", "--right", "--amplification", "--balance", "--tone", "--conversation-boost", "--enabled", "--ear", "--noise-reduction", "--low", "--mid", "--high", "--output"},
	}
	for _, setting := range allSettings() {
		if words := settingValueWords(setting); len(words) > 0 {
			table.positionals = append(table.positionals, completionRule{"set " + setting.Name, words})
		}
	}
	return table
}

func allBitrateWords() []string {
	words := []string{"default"}
	for _, codec := range audioCodecs {
		for _, bitrate := range codec.bitrates {
			word := fmt.Sprint(bitrate)
			if !slices.Contains(words, word) {
				words = append(words, word)
			}
		}
	}
	return words
}

// settingValueWords lists what a setting accepts. Percentages have no useful list.
func settingValueWords(setting *dongle.Setting) []string {
	var words []string
	switch setting.Kind {
	case dongle.KindNamed:
		for _, value := range setting.Values {
			words = append(words, value.Word)
		}
	case dongle.KindModes:
		mode, _ := dongle.LookupSetting("mode")
		for mask := 1; mask < 1<<len(mode.Values); mask++ {
			var parts []string
			for i, value := range mode.Values {
				if mask&(1<<i) != 0 {
					parts = append(parts, value.Word)
				}
			}
			if len(parts) >= 2 {
				words = append(words, strings.Join(parts, ","))
			}
		}
	case dongle.KindActions:
		for _, left := range []string{"noise-control", "siri"} {
			for _, right := range []string{"noise-control", "siri"} {
				words = append(words, left+","+right)
			}
		}
	}
	return words
}

func writeCompletion(w io.Writer, shell string) {
	table := buildCompletionTable()
	switch shell {
	case "bash":
		writeBash(w, table)
	case "zsh":
		writeZsh(w, table)
	case "fish":
		writeFish(w, table)
	case "powershell":
		writePowerShell(w, table)
	}
}

// caseArms writes one case arm per rule, in the shape bash and zsh share.
func caseArms(w io.Writer, indent string, rules []completionRule) {
	for _, rule := range rules {
		fmt.Fprintf(w, "%s'%s') words='%s' ;;\n", indent, rule.route, strings.Join(rule.words, " "))
	}
}

// walkPOSIX splits the words before the cursor into a route and a flag waiting for a value.
func walkPOSIX(table completionTable, first, limit, word string) string {
	return fmt.Sprintf(`    local route="" pending="" word words="" i
    for ((i = %s; i < %s; i++)); do
        word=%s
        if [[ -n $pending ]]; then
            pending=""
            continue
        fi
        case $word in
            %s) pending=$word ;;
            -*) ;;
            *) route="${route:+$route }$word" ;;
        esac
    done
`, first, limit, word, strings.Join(table.allFlags, "|"))
}

func writeBash(w io.Writer, table completionTable) {
	fmt.Fprint(w, "# bash completion for airpods\n_airpods() {\n    local cur=${COMP_WORDS[COMP_CWORD]}\n    COMPREPLY=()\n")
	fmt.Fprint(w, walkPOSIX(table, "1", "COMP_CWORD", "${COMP_WORDS[i]}"))
	// Read serials line by line: compgen -W would expand whatever a USB device calls itself.
	fmt.Fprint(w, `    if [[ $pending == --serial ]]; then
        local serial
        while IFS= read -r serial; do
            if [[ $serial == "$cur"* ]]; then
                COMPREPLY+=("$serial")
            fi
        done < <("${COMP_WORDS[0]}" list --serials 2>/dev/null)
        return
    fi
    if [[ -n $pending ]]; then
        case $pending in
`)
	caseArms(w, "            ", table.flagValues)
	fmt.Fprint(w, `        esac
    elif [[ $cur == -* ]]; then
        words='-h --help'
        case $route in
`)
	caseArms(w, "            ", table.flags)
	fmt.Fprint(w, `        esac
    else
        case $route in
`)
	caseArms(w, "            ", table.positionals)
	fmt.Fprint(w, `        esac
    fi
    COMPREPLY=($(compgen -W "$words" -- "$cur"))
}
complete -F _airpods airpods airpods.exe
`)
}

func writeZsh(w io.Writer, table completionTable) {
	fmt.Fprint(w, "#compdef airpods\n# zsh completion for airpods\n_airpods() {\n    local cur=${words[CURRENT]} command=${words[1]}\n")
	walk := walkPOSIX(table, "2", "CURRENT", "${words[i]}")
	// words is zsh's own command line here, so the candidates need another name.
	walk = strings.Replace(walk, `word words="" i`, `word offers="" i`, 1)
	fmt.Fprint(w, walk)
	arms := func(indent string, rules []completionRule) {
		var buffer strings.Builder
		caseArms(&buffer, indent, rules)
		fmt.Fprint(w, strings.ReplaceAll(buffer.String(), "words=", "offers="))
	}
	fmt.Fprint(w, `    if [[ $pending == --serial ]]; then
        local -a serials
        serials=(${(f)"$($command list --serials 2>/dev/null)"})
        compadd -- $serials
        return
    fi
    if [[ -n $pending ]]; then
        case $pending in
`)
	arms("            ", table.flagValues)
	fmt.Fprint(w, `        esac
    elif [[ $cur == -* ]]; then
        offers='-h --help'
        case $route in
`)
	arms("            ", table.flags)
	fmt.Fprint(w, `        esac
    else
        case $route in
`)
	arms("            ", table.positionals)
	fmt.Fprint(w, `        esac
    fi
    compadd -- ${=offers}
}
if [[ ${funcstack[1]} == _airpods ]]; then
    _airpods "$@"
else
    compdef _airpods airpods
fi
`)
}

func writeFish(w io.Writer, table completionTable) {
	fmt.Fprintf(w, `# fish completion for airpods
function __airpods_route
    set -l tokens (commandline -opc)
    set -l route
    set -l pending 0
    for token in $tokens[2..-1]
        if test $pending = 1
            set pending 0
            continue
        end
        switch $token
            case %s
                set pending 1
            case '-*'
            case '*'
                set -a route $token
        end
    end
    echo (string join ' ' -- $route)
end
function __airpods_at
    test (__airpods_route) = "$argv"
end
function __airpods_serials
    set -l tokens (commandline -opc)
    $tokens[1] list --serials 2>/dev/null
end
complete -c airpods -f
`, strings.Join(table.allFlags, " "))
	condition := func(route string) string { return fmt.Sprintf("__airpods_at %s", route) }
	for _, rule := range table.positionals {
		fmt.Fprintf(w, "complete -c airpods -n '%s' -a '%s'\n", condition(rule.route), strings.Join(rule.words, " "))
	}
	fmt.Fprintln(w, "complete -c airpods -s h -l help")
	values := map[string][]string{}
	for _, rule := range table.flagValues {
		values[rule.route] = rule.words
	}
	for _, rule := range table.flags {
		for _, flag := range rule.words {
			if flag == "-h" || flag == "--help" {
				continue
			}
			fmt.Fprintf(w, "complete -c airpods -n '%s' -l %s%s\n", condition(rule.route), strings.TrimPrefix(flag, "--"), fishFlagValue(table, flag, values))
		}
	}
}

// fishFlagValue says what follows a flag. A flag with a value takes the next word, and only --file
// and --output offer file names, against the -f at the top of the script.
func fishFlagValue(table completionTable, flag string, values map[string][]string) string {
	switch words, known := values[flag]; {
	case flag == "--serial":
		return " -x -a '(__airpods_serials)'"
	case flag == "--file" || flag == "--output":
		return " -r -F"
	case known:
		return " -x -a '" + strings.Join(words, " ") + "'"
	case slices.Contains(table.allFlags, flag):
		return " -x"
	}
	return ""
}

func writePowerShell(w io.Writer, table completionTable) {
	arms := func(rules []completionRule) {
		for _, rule := range rules {
			fmt.Fprintf(w, "        '%s' { '%s' }\n", rule.route, strings.Join(rule.words, " "))
		}
	}
	quoted := make([]string, len(table.allFlags))
	for i, flag := range table.allFlags {
		quoted[i] = "'" + flag + "'"
	}
	fmt.Fprintf(w, `# PowerShell completion for airpods
Register-ArgumentCompleter -Native -CommandName airpods, airpods.exe -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $route = @()
    $pending = ''
    $before = @($commandAst.CommandElements | Select-Object -Skip 1 | Where-Object { $_.Extent.EndOffset -lt $cursorPosition })
    foreach ($element in $before) {
        $text = $element.Extent.Text
        if ($pending) { $pending = ''; continue }
        if (@(%s) -contains $text) { $pending = $text; continue }
        if ($text -like '-*') { continue }
        $route += $text
    }
    $route = $route -join ' '
    $offers = ''
    if ($pending -eq '--serial') {
        $offers = (& $commandAst.CommandElements[0].Extent.Text list --serials 2>$null) -join ' '
    } elseif ($pending) {
        $offers = switch -Exact ($pending) {
`, strings.Join(quoted, ", "))
	arms(table.flagValues)
	fmt.Fprint(w, `        default { '' }
        }
    } elseif ($wordToComplete -like '-*') {
        $offers = switch -Exact ($route) {
`)
	arms(table.flags)
	fmt.Fprint(w, `        default { '-h --help' }
        }
    } else {
        $offers = switch -Exact ($route) {
`)
	arms(table.positionals)
	fmt.Fprint(w, `        default { '' }
        }
    }
    @($offers -split ' ') | Where-Object { $_ -and $_ -like "$wordToComplete*" } | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}
`)
}
