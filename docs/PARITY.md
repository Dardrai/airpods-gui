# Parity between the CLI and the GUI

The rule for both programs: the `airpods` command and the `airpods-tray` app have the same
features, except what is inherently desktop-only: the tray icon, notifications, autostart, language, theme and
popup, and auto-pause, which needs a running background app.

This table was built by reading the GUI's `internal/ui`, `internal/tray` and `docs/USER-GUIDE.md`, against every
command of `airpods`. **Status** is one of:

- **Same**: both have it.
- **Same, CLI differs**: both have the feature. The note says how the CLI's form differs, and why.
- **Exception**: desktop-only under the rule, or inherent to a command line tool. The note gives the reason.
- **GUI gap**: the CLI has it and the GUI does not yet. Listed again [below](#gaps-on-the-gui-side).

The GUI's open gaps are listed [below](#gaps-on-the-gui-side) with the ones it closed. The CLI is also compared with the
[dongle firmware's own command line tool](#the-firmwares-own-command-line-tool).

## Every GUI control and action

### Tray

| GUI | CLI | Status |
|---|---|---|
| Icon: plain, number, check mark, red number, dimmed, outline; click opens the window | `airpods status`, `airpods info` give the same facts | Exception: tray icon |
| Tooltip with batteries and mode | `airpods status --details` | Exception: tray icon |
| Menu row: model and state | `airpods info`, `airpods status` | Same |
| Menu row: batteries with charging and old markers | `airpods status --details` | Same |
| Menu rows: Off, Noise Cancellation, Transparency, Adaptive | `airpods set mode VALUE` | Same, CLI differs: see [`set` and model gates](#set-and-model-gates) |
| Show batteries (popup, closes after 6 seconds) | `airpods status --details` | Exception: popup |
| Disconnect AirPods | `airpods disconnect` | Same: the same backend, the routing handover of firmware 0.1.59 and newer |
| Reconnect AirPods | `airpods reconnect` | Same |
| Open settings, Quit | None; a command ends by itself | Exception: tray icon |
| A failed tray action shows a notification | The error goes to stderr, with an exit code | Exception: notifications |

### Window, header and batteries

| GUI | CLI | Status |
|---|---|---|
| Closing hides the window to the tray | None | Exception: tray icon |
| Status line with the last result | stdout, stderr and the exit code | Same |
| Header: model name, state, dongle problem | `airpods info`, `airpods status`, `airpods list` | Same |
| Battery: level, charging, age, last known, no reading | `airpods status --details` | Same |
| Listening mode pill | `airpods set mode VALUE` | Same, CLI differs: see [`set` and model gates](#set-and-model-gates) |
| Adaptive strength lane and slider | `airpods set adaptive-strength 0..100` | Same, CLI differs: see [`set` and model gates](#set-and-model-gates) |

### Settings cards

Every row reads with `airpods get` and `airpods settings`, and writes with `airpods set`.

| GUI row | CLI setting | Status |
|---|---|---|
| Conversation Awareness | `conversation-awareness` | Same |
| Personalized Volume | `personalized-volume` | Same |
| Allow Off mode | `allow-off` | Same |
| Noise Cancellation with one AirPod | `one-bud-anc` | Same |
| Tone volume | `tone-volume` | Same |
| Press speed | `press-speed` | Same |
| Press and hold duration | `hold-duration` | Same |
| Volume swipe duration | `swipe-duration` | Same |
| Volume swipe | `volume-swipe` | Same |
| Call controls | `call-controls` | Same |
| Press and hold action | `hold-actions` | Same |
| Modes for press and hold | `hold-modes` | Same |
| Digital Crown direction | `crown-direction` | Same |
| Microphone | `microphone` | Same |
| Ear detection | `ear-detection` | Same |
| Pause when asleep | `pause-when-asleep` | Same |
| Charging case sounds | `case-sounds` | Same |
| Case volume | `case-volume` | Same |
| Optimized charging | `optimized-charging` | Same |
| Connect automatically | `connect-automatically` | Same |
| Allow automatic connection | `allow-auto-connect` | Same |
| Values remembered for a pair that does not report them back | None: exit `3`, or `unknown` on `get` | Same, CLI differs: the CLI keeps no files, and never shows a value it only sent |

### Playback

| GUI | CLI | Status |
|---|---|---|
| Active, Requested, Reported delay, State | `airpods audio` | Same |
| Codec, Bitrate, Variable bitrate, Apply | `airpods audio set --codec --bitrate --variable` | Same |
| Game mode, Game mode status | `airpods game on\|off`, and `Game mode` in `airpods audio` | Same |

### Find nearby

| GUI | CLI | Status |
|---|---|---|
| Start, Stop | `airpods nearby`, Ctrl-C | Same, CLI differs: a bounded search, up to 5 minutes, since nothing may keep the finder on after a process ends |
| Signal bar, Finder, Trend, Last heard | One `nearby:` line per second | Same |

### Connection

| GUI | CLI | Status |
|---|---|---|
| Reconnect AirPods | `airpods reconnect` | Same: both send the plain reconnect request |
| None | `airpods connect` | GUI gap: Reconnect does not wait for the dongle to own the AirPods' audio |
| Disconnect AirPods | `airpods disconnect` | Same |
| None | `airpods reconnect-policy` | GUI gap |
| Pair a phone | `airpods pair-phone` | Same |
| Pair AirPods, with its warning and confirmation | `airpods pair`, its warning, a question or `--yes` | Same |
| Read settings again | `airpods settings` | Same |

### Diagnostics

| GUI | CLI | Status |
|---|---|---|
| Model | `airpods info` | Same |
| Firmware | `airpods info` | Same |
| Serial, Hidden until Show | `airpods info`, hidden until `--show-serial` | Same |
| Dongle | `airpods list` | Same |
| Dongle serial | `airpods list`, `airpods status --details` | Same |
| AirPods, with the address | `airpods status --details` | Same |
| Controls | `airpods status` | Same |
| Batteries in words | `airpods status --details` | Same |
| Noise control | `airpods status --details`, `airpods get mode` | Same |
| Codec | `airpods status` | Same |
| In use | `airpods status --details` | Same |
| None | Dongle capabilities in `airpods status --details` | Gap on the GUI side: see [below](#gaps-on-the-gui-side) |
| Counters: Dongle, Audio buffer, Encoder, Memory | `airpods perf` | Same |
| Setting reports with ages | `airpods settings` | Same |
| Refresh | Run the command again | Same |
| Copy status as JSON | `airpods --json status`, `airpods --json settings` | Same, CLI differs: two documents, which together hold the same fields |
| Copy as JSON: Dongle list, Info, Hearing state, Hardware status, Hardware data, Custom Transparency, Loud Sound Reduction | `airpods --json list`, `info --capabilities`, `hearing status`, `hardware status`, `hardware data`, `experimental transparency show`, `loud-sound-reduction get` | Same: the same fields. Info keeps the serial `null`, as without `--show-serial`. The last two need the Experimental features switch, as the commands need `--experimental` |
| Capture: Duration | `airpods diag --seconds` | Same, CLI differs: any 1 to 1800 s, where the GUI offers seven lengths |
| Capture: Audio trace off | `airpods diag --no-trace` | Same |
| Capture: Inspect signal | `airpods diag --inspect` | Same |
| Capture: Phone input | `airpods diag --phone` | Same |
| Capture: Radio only | `airpods diag --radio` | Same |
| Capture: file dialog, replacing a file after asking | `airpods diag --output`, with `--replace` | Same |
| Stop capture | Ctrl-C | Same |

### The `...` panel

| GUI | CLI | Status |
|---|---|---|
| Dongle list with reasons | `airpods list`, `--serial` | Same, CLI differs: the choice is given per command, because the CLI keeps no files. Use a shell alias to remember it |
| Theme | None | Exception: theme |
| Language | None | Exception: language |
| Start with the system | None | Exception: autostart |
| Auto-pause | None | Exception: needs a running background app |
| Notifications | None | Exception: notifications |
| Mode change notifications | None | Exception: notifications |
| Experimental features switch | `--experimental` on each `airpods experimental` command | Same, CLI differs: see [Hearing and hardware are not behind `--experimental`](#hearing-and-hardware-are-not-behind---experimental) |
| Debug log, Copy | `airpods --debug COMMAND`, to stderr | Same |
| Connection actions, repeated | As under Connection | Same |
| Show batteries, repeated | `airpods status --details` | Exception: popup |
| About: version | `airpods --version` | Same |
| About: Model catalog | `airpods info --capabilities` | Same |

### Hearing

| GUI | CLI | Status |
|---|---|---|
| Read: the modes | `airpods hearing status` | Same |
| Set up state and report ages | `airpods hearing status` | Same |
| Read: the profile | `airpods hearing profile show` | Same |
| Hearing Aid switch, asking before on | `airpods hearing aid on\|off`, a question or `--yes` | Same |
| Media Assist switch, asking before on | `airpods hearing media-assist on\|off`, a question or `--yes` | Same |
| Button to turn a mode off after an uncertain change | The same run offers it; later, `airpods hearing recover --off MODE` | Same, CLI differs: see [Hearing recovery](CLI.md#hearing-recovery) |
| Audiogram | `airpods hearing profile apply --left --right` | Same |
| Amplification, Balance, Tone, Conversation Boost | `--amplification`, `--balance`, `--tone`, `--conversation-boost` | Same |
| Apply, with the list of changes, its warning and `Turn Hearing Aid on?` | `airpods hearing profile apply`, its list, its warnings, a question or `--yes` | Same |
| Export... | `airpods --json hearing profile show \| jq .profile` | Same: the file `apply --file` takes |
| Import..., then Apply | `airpods hearing profile apply --file` | Same, CLI differs: the GUI only loads the file into the editor, and sends nothing until Apply |
| Ambient noise reduction, Own voice, read-only | `airpods hearing profile show` | Same |
| Not editable note | `Editable: no` in `profile show`, and a refusal on apply | Same |

### More experiments

| GUI | CLI | Status |
|---|---|---|
| Custom Transparency: Read, with stored ambient noise reduction | `airpods experimental transparency show` | Same |
| Custom Transparency: on, Ear, Amplification, Tone, Noise reduction, Conversation Boost, Apply | `airpods experimental transparency set` | Same |
| Loud Sound Reduction: Read | `airpods experimental loud-sound-reduction get` | Same |
| Loud Sound Reduction: switch | `airpods experimental loud-sound-reduction set on\|off` | Same |
| EQ: Mode, Low, Mid, High, Send | `airpods experimental eq show`, `eq set` | GUI gap: the CLI reads the AirPods' EQ report and confirms a change by a new one; the GUI sends without reading |
| Rename: name, Send | `airpods experimental rename` | Same: says sent, never confirmed |

### Hardware checks

| GUI | CLI | Status |
|---|---|---|
| Refresh, results per ear, measured time | `airpods hardware status` | Same |
| Read device data | `airpods hardware data` | Same |
| Run check... with `Both are in the open case` | `airpods hardware check`, a question or `--buds-in-case` | Same |
| Progress, ears, seconds left | `check:` lines | Same |
| Stop waiting | Ctrl-C | Same |

### Starting the app

| GUI | CLI | Status |
|---|---|---|
| `airpods-tray` | None; each command is a run | Exception: tray icon |
| `--tray` | None | Exception: tray icon |
| `--serial SERIAL` | `--serial SERIAL` | Same |
| `--version` | `--version` | Same |
| One copy at a time, a second start shows the window | None | Exception: a command ends by itself. Commands share the channel, and a busy channel says so |
| `AIRPODS_GUI_DEBUG=1` | `--debug` | Same |
| `AIRPODS_GUI_SIM` | `AIRPODS_CLI_SIM` | Same, CLI differs: a simulated change lasts one run, and each run says on stderr that it is simulated |
| Preferences file | None | Exception: only desktop choices are in it, and the chosen dongle |

## Every CLI command

| CLI | GUI | Status |
|---|---|---|
| `airpods help` | The user guide | Exception: inherent to a command line tool |
| `airpods completion` | None | Exception: shells only |
| `airpods list` | Dongle list, shown with several dongles | Same |
| `airpods list --serials` | None | Exception: it exists for completion |
| `airpods --json list` | Copy as JSON: Dongle list | Same |
| `airpods status` | Header, Diagnostics | Same |
| `airpods status --details` | Diagnostics | Same |
| `airpods settings` | Cards, Setting reports | Same, except `en352-protection` and `gain-swipe`: GUI gap |
| `airpods get` | Cards, Setting reports | Same |
| `airpods --json get` | Copy status as JSON holds it among the rest | Same |
| `airpods set` | Cards, pill, tray menu | Same |
| `airpods audio`, `audio set` | Playback | Same |
| `airpods game` | Playback | Same |
| `airpods nearby --seconds` | Find nearby | Same |
| `airpods perf` | Diagnostics counters | Same |
| `airpods info` | Diagnostics, header | Same |
| `airpods info --capabilities` | About: Model catalog | Same |
| `airpods --json info` | Copy as JSON: Info | Same |
| `airpods hearing status` | Hearing, Read | Same |
| Set up state and report ages in `hearing status` | Hearing, after Read | Same |
| `airpods --json hearing status` | Copy as JSON: Hearing state | Same |
| `airpods hearing aid`, `media-assist` | Hearing switches | Same |
| `airpods hearing recover` | The button after an uncertain change | Same, CLI differs: see above |
| `airpods hearing profile show` | Stored hearing profile | Same |
| `airpods --json hearing profile show` | Export... writes its `profile` object, and Import... reads the whole document | Same |
| `airpods hearing profile apply` | Apply | Same |
| `airpods hearing profile apply --file` | Import..., then Apply | Same |
| List of what will change, before apply | Apply's question | Same |
| `airpods hardware status`, `data` | Hardware checks | Same |
| `airpods --json hardware data` | Copy as JSON: Hardware data | Same |
| `airpods --json hardware status` | Copy as JSON: Hardware status | Same |
| `airpods hardware check` | Run check... | Same |
| `airpods experimental ...` | More experiments | Same |
| `airpods --json experimental transparency show`, `loud-sound-reduction get` | Copy as JSON: Custom Transparency, Loud Sound Reduction | Same |
| `airpods --json experimental eq show` | None | GUI gap |
| `airpods reconnect`, `pair`, `pair-phone`, `disconnect` | Connection | Same |
| `airpods connect`, `reconnect-policy` | None | GUI gap |
| `--serial` | `--serial`, Dongle list | Same |
| `--debug` | Debug log | Same |
| `--version` | About | Same |
| `AIRPODS_CLI_SIM` | `AIRPODS_GUI_SIM` | Same |
| `airpods diag` and all its flags | Capture to file | Same; printing to stdout is inherent to a command line tool |

## Intended exceptions

| What | Reason |
|---|---|
| Tray icon, tooltip, tray menu frame, Open settings, Quit, closing to the tray, `--tray`, one copy at a time | There is no tray in a command line tool. What they show is in `status` and `info` |
| Notifications, mode change notifications | They watch the AirPods all the time, and a command ends by itself |
| Start with the system | Nothing runs between commands. A scheduled task or shell startup file can run a command |
| Language | The CLI speaks English. Scripts rely on the wording of its output |
| Theme | Nothing to colour |
| Popup, Show batteries | `airpods status --details` |
| Auto-pause | It needs a running background app, to follow the ears and control a media player |
| `help`, `completion`, `list --serials` | Inherent to a command line tool |

## Differences to know about

Both sides have these features, but the CLI's form differs on purpose.

### `set` and model gates

The GUI hides settings and values the reported model lacks, using `model.View`. `airpods set` does not look: it
sends what it is asked, and the dongle or the AirPods have the last word, as they always did. The result says so:
refused, not confirmed or exit `3`. Gating it would add a metadata read to each `set`, which changes the traffic of
every existing command. `airpods info --capabilities` shows what the catalog lists. The hearing, hardware and
experimental commands are gated, as the library enforces it.

### Allow Off after a reconnect

The dongle forgets Allow Off whenever the AirPods connect again, and on AirPods Pro 3 it then refuses Off. The GUI
remembers the user's Allow Off choice and sends it again once per connection, before anything needs it. `airpods`
keeps nothing between runs, so it waits for the dongle to refuse Off, then sends `allow-off on` and Off once more. Asking
first would add reads to every `set` of Off, for the same reason as above. See [allow-off](CLI.md#allow-off).

### Hearing and hardware are not behind `--experimental`

In the GUI, Hearing and Hardware checks sit behind the Experimental features switch. In the CLI they are their own
commands, gated on the model, with a warning and a confirmation where something changes. Only `airpods experimental`
needs `--experimental`. That is a deliberate choice for this repo.

## Gaps on the GUI side

Open:

- The GUI does not show the dongle's capabilities, which `airpods status --details` lists.
- No `connect`: Reconnect sends the plain request and does not wait for the routing handover to confirm the AirPods.
- No reconnect policy control.
- No `en352-protection` or `gain-swipe` rows.
- EQ sends without reading the AirPods' EQ report first, and cannot confirm a change.

Closed:

1. Turning Hearing Aid or Media Assist on with their switches warns and asks first.
2. Apply lists what will change, next to the warning.
3. Hearing profiles can be exported to, and imported from, a JSON file.
4. Hearing shows the set up state and how old the reports are.
5. Stored ambient noise reduction and own voice are shown, read-only.
6. About lists what the model catalog says, as `info --capabilities` prints.
7. Copy as JSON has the dongle list, `info`, the hearing state, the hardware status and data, and the two experimental reads.
8. The user guide no longer says that only the file dialog can replace a file, and names `--no-trace` for Audio trace.

## The firmware's own command line tool

The [dongle firmware](https://github.com/Ripthulhu/airpods-dongle) comes with its own Python tool, `tools/airpods.py`,
documented in its `docs/CLI.md`. This CLI follows its behaviour and wording where it has the same command, not its
command tree. Where they differ:

| The firmware's tool | This CLI | Status |
|---|---|---|
| `reconnect-policy [manual\|auto]` | `airpods reconnect-policy [manual\|auto]` | Same outputs: `Reconnect policy: X`, `(no change needed; no write sent)`, `(saved and verified)`. A save the dongle could not verify is exit `3`, never retried. This CLI adds a warning on stderr whenever `auto` is the result |
| `connect` | `airpods connect` | Same, CLI differs: the tool waits 12 seconds and then calls the takeover unconfirmed, exit `3`. This CLI waits 5 seconds and then reports it as accepted and still in progress, exit `0`, as `disconnect` does, since the dongle goes on with it. On firmware without the routing handover both send the plain reconnect request and say it is `requested; not yet confirmed` |
| `connect` replaces a reconnect command | `airpods reconnect` stays | Same, CLI differs: `reconnect` is the plain request and says only that the dongle took it; `connect` is the confirmed one |
| `disconnect` | `airpods disconnect` | Same: both send the release even when the AirPods are not connected, to set the hold. The tool waits 12 seconds and then calls it unconfirmed; this CLI waits 5 and reports `release accepted, not yet closed`, exit `0` |
| `pair airpods --replace`: refuses while AirPods are saved, unless `--replace` | `airpods pair`: a warning, then a question, or `--yes` | Not adopted. Both erase the pairing only on an explicit choice. `--replace` is not accepted as a synonym |
| `pair phone` | `airpods pair-phone` | Same, CLI differs in the name only |
| A busy answer to `pair airpods` is unconfirmed, exit `3` | The same | Same |
| `pair` and `pair phone` refuse before sending while streaming, a phone, the microphone or USB playback is active | `pair` refuses for USB playback, the microphone and a phone; `pair-phone` for USB playback and the microphone | Same, CLI differs: the dongle's own refusal covers the rest, with nothing sent |
| `set game-mode on`: waits up to 8 seconds for the AirPods to confirm, and says `waiting for playback (not yet active)` | `airpods game on` returns once the dongle accepts it, with the state in the tool's words, such as `now waiting for playback` | Not adopted: no other command of this CLI waits for an audio state. `airpods audio` follows it |
| `get game-mode` | `airpods audio`, its `Game mode` line | Same words: `off`, `waiting for playback`, `unconfirmed`, `on`, `failed`, `unsupported`, `turning off` |
| `settings`, `get`, `set` with `en352-protection` and `gain-swipe`, by the extended snapshot, falling back once on older firmware | The same | Same. This CLI uses the extended snapshot for every read, as the library does |
| `get/set eq`, `eq-low`, `eq-mid`, `eq-high` | `airpods experimental eq show`, `eq set` | Same, CLI differs: behind `--experimental`, as all of this CLI's EQ is |
| `get/set loud-sound-reduction`, hearing names | `airpods experimental loud-sound-reduction`, `airpods hearing ...` | Same, CLI differs: this CLI's own command tree, with the `--experimental` gate kept |
| Hearing accommodations and presets | None | Not adopted: the library does not support them |
