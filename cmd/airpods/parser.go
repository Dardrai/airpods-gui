package main

import (
	"slices"

	"github.com/Dardrai/airpods-gui/core/dongle"
	"github.com/Dardrai/airpods-gui/internal/argparse"
	"github.com/Dardrai/airpods-gui/internal/cliutil"
)

// controlCommands maps control subcommands to their call and success line.
var controlCommands = map[string]struct {
	call func(*dongle.Client) error
	line string
}{
	"reconnect":  {(*dongle.Client).Reconnect, "reconnect: accepted by dongle"},
	"pair-phone": {(*dongle.Client).PairPhone, "pair-phone: accepted by dongle"},
}

func helpAction() *argparse.Action {
	return &argparse.Action{Kind: argparse.Help, Dest: "help", Flags: []string{"-h", "--help"}}
}

// allSettings is every setting the CLI exposes, in display order.
func allSettings() []*dongle.Setting {
	settings := make([]*dongle.Setting, 0, len(dongle.Settings)+len(dongle.ExtraSettings))
	for _, table := range [][]dongle.Setting{dongle.Settings, dongle.ExtraSettings} {
		for i := range table {
			settings = append(settings, &table[i])
		}
	}
	return settings
}

func settingNames() []string {
	var names []string
	for _, setting := range allSettings() {
		names = append(names, setting.Name)
	}
	return names
}

// extraCommands are the subcommands beyond get/set/status/settings/help.
var extraCommands = []string{"reconnect", "connect", "pair", "pair-phone", "disconnect", "reconnect-policy", "firmware"}

func buildParser() *argparse.Parser {
	sub := func(name string, positionals ...*argparse.Action) *argparse.Parser {
		text := helpTexts[name]
		return &argparse.Parser{
			Prog:    "airpods " + name,
			Usage:   text.usage,
			HelpMsg: text.usage + text.body,
			Actions: append([]*argparse.Action{helpAction()}, positionals...),
		}
	}
	setting := func() *argparse.Action {
		return &argparse.Action{Kind: argparse.Store, Dest: "setting", Required: true, Choices: settingNames()}
	}
	audio := sub("audio", &argparse.Action{Kind: argparse.SubParsers, Dest: "subcommand", Choices: []string{"set"}})
	audio.Parsers = map[string]*argparse.Parser{"set": sub("audio set",
		&argparse.Action{Kind: argparse.Store, Dest: "codec", Flags: []string{"--codec"}, Choices: codecWords()},
		&argparse.Action{Kind: argparse.Store, Dest: "bitrate", Flags: []string{"--bitrate"}},
		&argparse.Action{Kind: argparse.Store, Dest: "variable", Flags: []string{"--variable"}, Choices: []string{"on", "off"}},
	)}
	yes := func() *argparse.Action {
		return &argparse.Action{Kind: argparse.StoreTrue, Dest: "yes", Flags: []string{"--yes"}}
	}
	onOff := func() *argparse.Action {
		return &argparse.Action{Kind: argparse.Store, Dest: "state", Required: true, Choices: []string{"on", "off"}}
	}
	hearingGroup := sub("hearing", &argparse.Action{Kind: argparse.SubParsers, Dest: "subcommand", Required: true, Choices: []string{"status", "aid", "media-assist", "recover", "profile"}})
	hearingGroup.Parsers = map[string]*argparse.Parser{
		"status":       sub("hearing status"),
		"aid":          sub("hearing aid", onOff(), yes()),
		"media-assist": sub("hearing media-assist", onOff(), yes()),
		"recover":      sub("hearing recover", &argparse.Action{Kind: argparse.Store, Dest: "off", Flags: []string{"--off"}, Choices: []string{"hearing-aid", "media-assist"}}),
	}
	option := func(dest string, flag string) *argparse.Action {
		return &argparse.Action{Kind: argparse.Store, Dest: dest, Flags: []string{flag}}
	}
	profileGroup := sub("hearing profile", &argparse.Action{Kind: argparse.SubParsers, Dest: "action", Required: true, Choices: []string{"show", "apply"}})
	profileGroup.Parsers = map[string]*argparse.Parser{
		"show": sub("hearing profile show"),
		"apply": sub("hearing profile apply", option("file", "--file"), option("left", "--left"), option("right", "--right"),
			option("amplification", "--amplification"), option("balance", "--balance"), option("tone", "--tone"),
			&argparse.Action{Kind: argparse.Store, Dest: "boost", Flags: []string{"--conversation-boost"}, Choices: []string{"on", "off"}},
			yes()),
	}
	hearingGroup.Parsers["profile"] = profileGroup
	hardwareGroup := sub("hardware", &argparse.Action{Kind: argparse.SubParsers, Dest: "subcommand", Required: true, Choices: []string{"status", "data", "check"}})
	hardwareGroup.Parsers = map[string]*argparse.Parser{
		"status": sub("hardware status"),
		"data":   sub("hardware data"),
		"check":  sub("hardware check", &argparse.Action{Kind: argparse.StoreTrue, Dest: "buds_in_case", Flags: []string{"--buds-in-case"}}),
	}
	// The flag works before or after the subcommand; the two places cannot share a Dest, since a
	// subparser's default would overwrite what its parent found.
	leaf := func(name string, actions ...*argparse.Action) *argparse.Parser {
		flag := &argparse.Action{Kind: argparse.StoreTrue, Dest: "experimental_leaf", Flags: []string{"--experimental"}}
		return sub(name, append([]*argparse.Action{flag}, actions...)...)
	}
	transparency := sub("experimental transparency", &argparse.Action{Kind: argparse.SubParsers, Dest: "action", Required: true, Choices: []string{"show", "set"}})
	transparency.Parsers = map[string]*argparse.Parser{
		"show": leaf("experimental transparency show"),
		"set": leaf("experimental transparency set",
			&argparse.Action{Kind: argparse.Store, Dest: "enabled", Flags: []string{"--enabled"}, Choices: []string{"on", "off"}},
			&argparse.Action{Kind: argparse.Store, Dest: "ear", Flags: []string{"--ear"}, Choices: []string{"left", "right", "both"}},
			option("amplification", "--amplification"), option("tone", "--tone"), option("noise-reduction", "--noise-reduction"),
			&argparse.Action{Kind: argparse.Store, Dest: "boost", Flags: []string{"--conversation-boost"}, Choices: []string{"on", "off"}}),
	}
	loud := sub("experimental loud-sound-reduction", &argparse.Action{Kind: argparse.SubParsers, Dest: "action", Required: true, Choices: []string{"get", "set"}})
	loud.Parsers = map[string]*argparse.Parser{
		"get": leaf("experimental loud-sound-reduction get"),
		"set": leaf("experimental loud-sound-reduction set", onOff()),
	}
	eq := sub("experimental eq", &argparse.Action{Kind: argparse.SubParsers, Dest: "action", Required: true, Choices: []string{"show", "set"}})
	eq.Parsers = map[string]*argparse.Parser{
		"show": leaf("experimental eq show"),
		"set": leaf("experimental eq set",
			&argparse.Action{Kind: argparse.Store, Dest: "enabled", Flags: []string{"--enabled"}, Choices: []string{"on", "off"}},
			cliutil.IntOption("low", "--low"), cliutil.IntOption("mid", "--mid"), cliutil.IntOption("high", "--high")),
	}
	experimentalGroup := sub("experimental",
		&argparse.Action{Kind: argparse.StoreTrue, Dest: "experimental", Flags: []string{"--experimental"}},
		&argparse.Action{Kind: argparse.SubParsers, Dest: "subcommand", Required: true, Choices: []string{"transparency", "loud-sound-reduction", "eq", "rename"}})
	experimentalGroup.Parsers = map[string]*argparse.Parser{
		"transparency":         transparency,
		"loud-sound-reduction": loud,
		"eq":                   eq,
		"rename":               leaf("experimental rename", &argparse.Action{Kind: argparse.Store, Dest: "name", Required: true}),
	}
	parsers := map[string]*argparse.Parser{
		"experimental": experimentalGroup,
		"hearing":      hearingGroup,
		"hardware":     hardwareGroup,
		"audio":        audio,
		"game":         sub("game", &argparse.Action{Kind: argparse.Store, Dest: "state", Required: true, Choices: []string{"on", "off"}}),
		"nearby":       sub("nearby", cliutil.SecondsOption(nearbyDefaultSeconds)),
		"perf":         sub("perf"),
		"info": sub("info",
			&argparse.Action{Kind: argparse.StoreTrue, Dest: "show_serial", Flags: []string{"--show-serial"}},
			&argparse.Action{Kind: argparse.StoreTrue, Dest: "capabilities", Flags: []string{"--capabilities"}}),
		"completion": sub("completion", &argparse.Action{Kind: argparse.Store, Dest: "shell", Required: true, Choices: completionShells}),
		"help":       sub("help", &argparse.Action{Kind: argparse.Store, Dest: "topic", Nargs: argparse.NargsOptional}),
		"list":       sub("list", &argparse.Action{Kind: argparse.StoreTrue, Dest: "serials", Flags: []string{"--serials"}}),
		"status":     sub("status", &argparse.Action{Kind: argparse.StoreTrue, Dest: "details", Flags: []string{"--details"}}),
		"settings":   sub("settings"),
		"get":        sub("get", setting()),
		"set":        sub("set", setting(), &argparse.Action{Kind: argparse.Store, Dest: "value", Required: true}),
	}
	for _, name := range extraCommands {
		parsers[name] = sub(name)
	}
	parsers["diag"] = diagParser()
	parsers["pair"] = sub("pair", yes())
	parsers["reconnect-policy"] = sub("reconnect-policy", &argparse.Action{Kind: argparse.Store, Dest: "policy", Nargs: argparse.NargsOptional, Choices: []string{"manual", "auto"}})
	top := helpTexts[""]
	return &argparse.Parser{
		Prog:       "airpods",
		Usage:      top.usage,
		HelpMsg:    top.usage + top.body,
		VersionMsg: "airpods " + version + "\n",
		Parsers:    parsers,
		Actions: []*argparse.Action{
			helpAction(),
			{Kind: argparse.Version, Dest: "version", Flags: []string{"--version"}},
			{Kind: argparse.Store, Dest: "serial", Flags: []string{"--serial"}},
			{Kind: argparse.StoreTrue, Dest: "json", Flags: []string{"--json"}},
			{Kind: argparse.StoreTrue, Dest: "debug", Flags: []string{"--debug"}},
			{Kind: argparse.SubParsers, Dest: "command", Required: true,
				Choices: append([]string{"help", "completion", "list", "status", "settings", "get", "set", "audio", "game", "nearby", "perf", "info", "hearing", "hardware", "experimental"}, append(slices.Clone(extraCommands), "diag")...)},
		},
	}
}
