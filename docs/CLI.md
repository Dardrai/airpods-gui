# Command line reference

This covers `airpods`, the command line tool, and its `airpods diag` capture. For error messages, see
[CLI-TROUBLESHOOTING.md](CLI-TROUBLESHOOTING.md). To build it, see [BUILD.md](BUILD.md). The tray app has its own
[user guide](USER-GUIDE.md).

- [Conventions](#conventions)
- [airpods](#airpods)
- [Commands](#commands): [help](#help), [list](#list), [status](#status), [settings](#settings),
  [get](#get), [set](#set), [audio](#audio), [audio set](#audio-set), [game](#game),
  [nearby](#nearby), [perf](#perf), [info](#info), [hearing](#hearing), [hardware](#hardware),
  [experimental](#experimental), [reconnect](#reconnect), [connect](#connect), [pair](#pair),
  [pair-phone](#pair-phone), [disconnect](#disconnect), [reconnect-policy](#reconnect-policy), [firmware](#firmware),
  [completion](#completion)
- [JSON output](#json-output)
- [What the results mean](#what-the-results-mean)
- [Settings](#settings-1)
- [Multiple dongles](#multiple-dongles)
- [airpods diag](#airpods-diag)

## Conventions

The examples call the tool `airpods`. Release files have longer names, such as
`airpods-linux-amd64` or `airpods-windows-amd64.exe`. Rename the one you use, as the
[README](../README.md#download) shows. To run a tool from the current folder rather than your
`PATH`, write `./airpods` on Linux or `.\airpods` in PowerShell.

Commands work the same on Windows and Linux unless noted. Where a platform needs its own command,
Windows commands are for PowerShell. To see an exit code, use `$LASTEXITCODE` in PowerShell or
`$?` in a Linux shell.

Output samples come from the test data in this repo, which uses a simulated dongle. The format is
exact, but the values are only examples. `SIM0001` is the simulator's serial.

Success output goes to stdout. Errors go to stderr, except for the `unknown` line of `get`, the
`unknown` setting of `airpods --json get`, and the `not reported` values of `info`.

## airpods

```text
airpods [-h] [--version] [--serial SERIAL] [--json] [--debug] COMMAND ...
```

| Option | Meaning |
|---|---|
| `-h`, `--help` | Show help and exit. Works after a command too, as in `airpods set -h` |
| `--version` | Show the version and exit, as in `airpods v0.0.2-alpha`. A build with no tag to read shows `dev`. Must come before the command |
| `--serial SERIAL` | Use the dongle with this USB serial. Needed when several are attached. See [Multiple dongles](#multiple-dongles) |
| `--json` | Print `status`, `settings`, `get`, `list`, `info` and the read commands of `hearing`, `hardware` and `experimental` as JSON. See [JSON output](#json-output). With any other command it fails with exit `2` |
| `--debug` | Also print what goes over the request channel to stderr: times, sizes and result codes, never payloads, so no address or serial. Paste it into a bug report |

`--serial`, `--json` and `--debug` must come before the command. `airpods status --serial X` and
`airpods status --json` fail with exit `2` and a message naming the flag: `--serial must come before the command`
and `--json must come before the command`.

Write every flag in full. A shortened flag such as `--ser` is an error, for `airpods diag` too.

A value starting with `-` needs an equals sign, unless it is a plain decimal such as `-1` or `-0.5`:
write `--balance=-1e-1` or `--serial=-x`. Without it the value reads as a flag, and the error says so.

### Exit codes

| Code | Meaning |
|---|---|
| `0` | Done: help or a read worked, a change was confirmed or accepted, or the value was already reported and no write was sent |
| `1` | Failed: no dongle, a connection or protocol failure, a refused request, `get` on an unreported setting, `disconnect` on dongle firmware before 0.1.59, `reconnect-policy` on dongle firmware before 0.1.63, or output that could not be written to stdout. A code that already says more, such as `3`, is kept |
| `2` | Bad command line: unknown command, unknown setting or invalid value. The dongle is not opened |
| `3` | A write went out and may have applied, but it was not confirmed. A failure before anything was sent is `1`. This includes a `pair` the dongle answered busy and a reconnect policy save the dongle could not verify. See [After exit 3](#after-exit-3), and [Hearing recovery](#hearing-recovery) for the hearing modes |
| `130` | `nearby` was interrupted, for example with Ctrl-C, and the search was stopped first. Or `hardware check`: only the waiting stopped |

## Commands

### help

```text
airpods help [TOPIC]
```

Shows commands, a command's options, or a setting's values. It never opens the dongle.

`TOPIC` is a command or setting name. `airpods --help` shows the same text as `airpods help`,
without the settings and examples at the end.

Examples:

```powershell
airpods help
airpods help set
airpods help mode
```

Output of `airpods help`:

```text
usage: airpods [-h] [--version] [--serial SERIAL] [--json] [--debug]
               {help,completion,list,status,settings,get,set,audio,game,nearby,perf,info,hearing,hardware,experimental,reconnect,connect,pair,pair-phone,disconnect,reconnect-policy,firmware} ...

Control AirPods through the dongle's existing USB HID interface.

positional arguments:
  {help,completion,list,status,settings,get,set,audio,game,nearby,perf,info,hearing,hardware,experimental,reconnect,connect,pair,pair-phone,disconnect,reconnect-policy,firmware}
    help                show commands or help for a setting
    completion          print a shell completion script
    list                list attached dongles; works with none attached
    status              show connection, codec and batteries without changing
                        settings
    settings            list readable setting values and their report ages
    get                 read one reported setting
    set                 change one setting and wait for device confirmation
    audio               show the audio profile, or change it with audio set
    game                turn game mode on or off
    nearby              search for the AirPods by signal strength for a
                        bounded time
    perf                show the dongle performance counters
    info                show model, firmware and serial, and what the model can
                        do
    hearing             read Hearing Aid and Media Assist, or change them
    hardware            read saved hardware results and data, or run the in-case
                        check
    experimental        transparency, loud sound reduction, EQ and rename; needs
                        --experimental
    reconnect           ask the dongle to reconnect to its AirPods; confirms
                        only that the dongle took the request
    connect             connect the AirPods and confirm that the dongle owns
                        their audio
    pair                start pairing new AirPods; refused while audio or a
                        phone is in use
    pair-phone          open the dongle's phone pairing window
    disconnect          disconnect AirPods but keep them paired; needs dongle
                        firmware 0.1.59 or newer
    reconnect-policy    read or save whether the dongle reconnects by itself;
                        needs dongle firmware 0.1.63 or newer
    firmware            show the version of the dongle firmware; needs firmware
                        with the version report

options:
  -h, --help            show this help message and exit
  --version             show the version and exit
  --serial SERIAL       select one Pico by its USB serial; required when
                        multiple dongles are attached
  --json                print status, settings, get, list, info and other reads
                        as JSON
  --debug               print request channel traffic to stderr: sizes and codes,
                        no addresses or serials

Close NTPods or other HID control clients before using settings commands.

Settings:
  mode: off, anc, transparency, adaptive
  microphone: auto, right, left
  ear-detection: on, off
  one-bud-anc: on, off
  conversation-awareness: on, off
  personalized-volume: on, off
  allow-off: on, off
  pause-when-asleep: on, off
  case-sounds: on, off
  optimized-charging: on, off
  volume-swipe: on, off
  press-speed: default, slower, slowest
  hold-duration: default, slower, slowest
  swipe-duration: default, longer, longest
  call-controls: default, swapped
  crown-direction: default, reversed
  tone-volume: integer 0..100
  case-volume: integer 0..100
  adaptive-strength: integer 0..100
  hold-modes: at least two distinct modes, comma-separated: off, anc, transparency, adaptive
  hold-actions: left,right actions, comma-separated: noise-control or siri for each ear
  connect-automatically: on, off
  allow-auto-connect: on, off
  en352-protection: on, off
  gain-swipe: on, off

Examples:
  airpods get mode
  airpods set mode transparency
  airpods set ear-detection on
```

Output of `airpods help mode`:

```text
mode: off, anc, transparency, adaptive
Get: airpods get mode
Set: airpods set mode VALUE
```

Exit codes: `0` on success, `2` for an unknown topic.

### list

```text
airpods list [--serials]
```

Shows every attached dongle, one per line, whether or not it can be used. It reads only the status
report of each dongle and changes nothing, so it is the first thing to run when a dongle is not
found. It works with no dongle attached.

Example with several dongles:

```powershell
airpods list
```

Output:

```text
SIM0001  4015  /dev/hidraw3   connected
SIM0002  4005  /dev/hidraw12  not connected
Several dongles attached. Pick one with: airpods --serial SERIAL COMMAND
```

The columns line up to the longest serial and path. With nothing attached, the output is:

```text
No dongles found.
```

Each line has four columns:

| Column | Values | Notes |
|---|---|---|
| Serial | The USB serial, or `-` | `-` means the system gave no serial. Use this value with `--serial` |
| USB ID | `4015`, `4005` | The product ID. See [PID 4005 and 4015](CLI-TROUBLESHOOTING.md#pid-4005-and-4015) |
| Path | `/dev/hidrawN` on Linux, a device interface path on Windows | |
| State | `connected`, `not connected`, or a reason | Whether the dongle has its AirPods connected, or why it could not be asked |

The reasons are:

| State | Meaning |
|---|---|
| `cannot be opened: REASON` | The system or the dongle refused. On Linux this is often a [missing udev rule](CLI-TROUBLESHOOTING.md#linux-found-node-but-no-permission) |
| `cannot be selected: it has no USB serial` | Several dongles can be opened and this one has no serial to pick it by |
| `cannot be selected: its USB serial is not unique` | Two attached dongles report the same serial, so neither can be picked |

With several dongles, a last line reminds you of `--serial`. With `--serial`, `list` shows only that
dongle, or `No dongle with serial X found.` if it is not attached.

`--serials` prints only the serials, one per line, without opening any dongle for its status.
Shell completion uses it. It leaves out an empty serial, and any serial with characters other than
letters, digits, `-`, `_` and `.`, because completion would put it on a command line. It cannot be
combined with `--json`.

Exit codes: `0` when the list was made, including with no dongles and with dongles that cannot be
opened. `1` if the system could not list devices. `2` for a bad command line.

### status

```text
airpods status
```

Shows the connection, settings readiness, codec and last known batteries. It changes nothing.

Example:

```powershell
airpods status
```

Output:

```text
AirPods: connected
Settings: ready
Codec: AAC
Battery left: 80%
Battery right: 70%
Battery case: unknown
```

| Line | Values | Notes |
|---|---|---|
| `AirPods` | `connected`, `disconnected` | The link between the dongle and the AirPods |
| `Settings` | `ready`, `not ready` | Whether `set` can work right now |
| `Codec` | `SBC`, `AAC`, `AAC-ELD`, `unknown` | `unknown` means the dongle reported no codec this tool knows |
| `Battery left`, `right`, `case` | `0%` to `100%`, `unknown` | Last values the dongle has. They can be old, and they can still show while the AirPods are disconnected. `unknown` means no reading |

Exit codes: `0` when the dongle answered, even if the AirPods are disconnected. `1` if the dongle
could not be read.

`airpods status --details` adds what the tray app's Diagnostics section lists. It also reads only the status report. The
AirPods' address is in it, so check it before you share it:

```text
AirPods: connected
Settings: ready
Codec: AAC
Battery left: 80% (charging)
Battery right: 70% (45 s old)
Battery case: unknown
Dongle serial: SIM0001
AirPods address: 01:02:03:04:05:06
Noise control: anc
In use: USB playback, microphone
Dongle capabilities: control, listening, pair-phone, raw-transport, typed-rpc, lease, per-component, host-volume, runtime-bootsel, routing
```

A battery reads `(charging)`, `(N s old)` for one that is no longer current, or `(last known)` with no known age. A reading counts
as current for 30 seconds, or 90 seconds when it came from the case, and only for a bud that is present. `In use` lists any of
`streaming`, `USB playback`, `microphone`, `phone connected` and `phone pairing open`, or says `nothing`.

`Dongle capabilities` names what the dongle firmware says it can do, or says `none`. A bit this tool has no name for shows as
its hex value, such as `0x400`:

| Name | Bit | Meaning |
|---|---|---|
| `control` | `0x001` | `reconnect`, `pair`, and `connect` on firmware without `routing` |
| `listening` | `0x002` | Noise control through the dongle, while the AirPods report a mode |
| `pair-phone` | `0x004` | `pair-phone` |
| `raw-transport` | `0x008` | Requests passed on to the AirPods, which `info`, `hearing`, `hardware` and `experimental` need |
| `typed-rpc` | `0x010` | The dongle's own requests, settings, `audio` and `reconnect-policy` among them |
| `lease` | `0x020` | The settings session that `hearing` and `hardware` hold while they work |
| `per-component` | `0x040` | Which buds are present, for the battery markers |
| `host-volume` | `0x080` | Volume control for phone hosts. This tool does not use it |
| `runtime-bootsel` | `0x100` | Firmware updates may interrupt playing audio. This tool does not use it |
| `routing` | `0x200` | `disconnect`, and `connect` with confirmation, on firmware 0.1.59 or newer |

### settings

```text
airpods settings
```

Lists all 25 settings with their last reported values. It changes nothing.

Example:

```powershell
airpods settings
```

Output:

```text
mode: anc (report age 1200 ms)
microphone: unknown (not reported; may be unsupported)
ear-detection: on (report age 300 ms)
one-bud-anc: unknown (not reported; may be unsupported)
conversation-awareness: unknown (not reported; may be unsupported)
personalized-volume: unknown (not reported; may be unsupported)
allow-off: unknown (not reported; may be unsupported)
pause-when-asleep: unknown (not reported; may be unsupported)
case-sounds: unknown (not reported; may be unsupported)
optimized-charging: unknown (not reported; may be unsupported)
volume-swipe: unknown (not reported; may be unsupported)
press-speed: unknown (not reported; may be unsupported)
hold-duration: unknown (not reported; may be unsupported)
swipe-duration: unknown (not reported; may be unsupported)
call-controls: unknown (not reported; may be unsupported)
crown-direction: unknown (not reported; may be unsupported)
tone-volume: 50 (report age 40 ms)
case-volume: unknown (not reported; may be unsupported)
adaptive-strength: unknown (not reported; may be unsupported)
hold-modes: unknown (not reported; may be unsupported)
hold-actions: noise-control,siri (report age 7 ms)
connect-automatically: unknown (not reported; may be unsupported)
allow-auto-connect: unknown (not reported; may be unsupported)
en352-protection: unknown (not reported; may be unsupported)
gain-swipe: unknown (not reported; may be unsupported)
```

See [Reading values](#reading-values) for what these lines mean.

`settings`, `get` and `set` ask the dongle for its extended list of settings, which dongle firmware 0.1.64 added for
`en352-protection` and `gain-swipe`. Older firmware calls that request malformed; the tool then reads the original list
once more, in the same run, and those two settings read `unknown`. Nothing else changes, and no error is shown.

The AirPods report their settings one by one after they connect, so a read in the first seconds can list fewer than
usual. Run it again a moment later. Before the AirPods controls are ready, the dongle refuses the read:

```text
Error: AirPods controls are not ready yet; read the settings again in a moment
```

Exit codes: `0` when the list was read, even if every line is `unknown`. `1` on failure.

### get

```text
airpods get SETTING
```

Shows one setting's last reported value. It changes nothing.

Examples:

```powershell
airpods get mode
airpods get tone-volume
```

Output:

```text
mode: anc (report age 1200 ms)
```

If the setting has not been reported:

```text
microphone: unknown (not reported; may be unsupported)
```

Exit codes: `0` if the setting has been reported. `1` if it has not, in which case the line above
goes to stdout and nothing goes to stderr. `1` on other failures, `2` for an unknown setting.

### set

```text
airpods set SETTING VALUE
```

Changes one setting and waits for the AirPods to confirm it. It sends the write at most once and
never retries by itself.

Examples:

```powershell
airpods set mode transparency
airpods set ear-detection on
airpods set microphone auto
airpods set tone-volume 75
airpods set hold-modes anc,transparency,adaptive
airpods set hold-actions noise-control,siri
```

Output when the AirPods confirm the change:

```text
mode: transparency (confirmed by AirPods)
```

Output when the value was already reported, so nothing was sent:

```text
mode: anc (already reported, 1200 ms old; no write sent)
```

Output on exit `3`, on stderr:

```text
mode: sent, but no fresh matching device report. It may have applied; use get mode before retrying.
```

On AirPods Pro 3, `set mode off` and `set hold-modes` with `off` can print a line first:

```text
allow-off: on (sent first; the dongle forgets it whenever the AirPods connect again)
mode: off (confirmed by AirPods)
```

See [allow-off](#allow-off) for why.

Exit codes: `0`, `1`, `2` and `3`. See [What the results mean](#what-the-results-mean).

### audio

```text
airpods audio
```

Shows the audio profile the dongle is using and the one that was requested. It changes nothing.

Output:

```text
Codec: AAC-ELD
Bitrate: 256 kbps
Requested codec: AAC-ELD
Requested bitrate: 256 kbps
Variable bitrate: on
Activity: streaming
Microphone: not active
Input: 1
Reported delay: 12.5 ms
Game mode: not supported
```

| Line | Values | Notes |
|---|---|---|
| `Codec`, `Bitrate` | `SBC`, `AAC`, `AAC-ELD`, `unknown`; `N kbps`, `default` | What is in use now |
| `Requested codec`, `Requested bitrate` | `automatic`, `SBC`, `AAC`, `AAC-ELD`; `N kbps`, `default` | What was asked for. It can differ from what is in use |
| `Variable bitrate` | `on`, `off` | AAC-ELD only |
| `Activity` | `idle`, `streaming`, `change pending` | |
| `Microphone` | `active`, `not active` | |
| `Input` | A number | The dongle's audio input, as it reports it |
| `Reported delay` | `N ms`, `not reported` | |
| `Game mode` | `off`, `waiting for playback`, `unconfirmed`, `on`, `failed`, `unsupported`, `turning off`, `not supported` | The dongle's own words for the state, as its firmware's tool shows them. `not supported` means the dongle does not offer game mode at all |

Exit codes: `0` when the dongle answered. `1` on failure.

### audio set

```text
airpods audio set [--codec CODEC] [--bitrate BITRATE] [--variable on|off]
```

Changes the codec, bitrate or variable bitrate. Give at least one flag. **A change can restart the
audio link.**

| Codec | `--codec` | `--bitrate` | `--variable` |
|---|---|---|---|
| Automatic | `auto` | `default` only | `off` only |
| SBC | `sbc` | `default` only | `off` only |
| AAC | `aac` | `default`, `96000`, `128000`, `174000`, `192000`, `220000` | `off` only |
| AAC-ELD | `aac-eld` | `default`, `128000`, `174000`, `192000`, `220000`, `256000` | `on`, `off` |

What you leave out follows these rules:

- With `--codec`, a bitrate you leave out becomes `default` and variable bitrate becomes `off`,
  because the old bitrate may not exist for the new codec.
- Without `--codec`, the requested codec stays, and so does whichever of the other two you leave out.

If what you asked for is already the requested profile, nothing is sent.

Examples:

```powershell
airpods audio set --codec aac --bitrate 192000
airpods audio set --codec aac-eld --bitrate 256000 --variable on
airpods audio set --bitrate 128000
airpods audio set --codec auto
```

Output starts with `audio set: accepted by dongle` or `audio set: already requested; no write sent`,
followed by the lines of [audio](#audio). `accepted` means the dongle took the request. Check the
`Codec` and `Bitrate` lines to see what is in use.

Exit codes: `0` when accepted or already requested. `1` when refused, which means it did not apply.
Also `1`, with `Error:`, when the flags do not fit the codec that stays requested, as in `audio set --bitrate 256000`
with `aac` requested: that depends on the dongle's state, so the dongle has to be asked first, and nothing is sent. `2` for a
value no codec accepts or a combination that does not fit `--codec`; the dongle is not opened. `3` if the outcome is unknown:
run `airpods audio` before you try again.

### game

```text
airpods game on|off
```

Turns game mode on or off, on dongles that offer it.

Output:

```text
game: on (accepted by dongle, now waiting for playback)
game: on (already requested, now on; no write sent)
```

After `now` comes the state in the `Game mode` line of [audio](#audio). `waiting for playback` means the dongle sends
game mode only once audio plays. `waiting for playback` and `unconfirmed` are not failures, but they are not confirmation
either: game mode is `on` only once the AirPods confirm it. `game` returns as soon as the dongle accepts the request and
does not wait for that. Run `airpods audio` to follow it.

Exit codes: `0` when accepted or already requested. `1` if the dongle has no game mode, refuses, or
replies `failed` or `unsupported`. `2` for a bad command line. `3` if the outcome is unknown.

### nearby

```text
airpods nearby [--seconds SECONDS]
```

Searches for the AirPods by signal strength, printing one line a second. It stops by itself after
`--seconds`, from `1` to `300`, default `30`. The dongle searches until told to stop, so `airpods`
tells it to on every way out: at the time limit, on Ctrl-C, when the process is asked to end, when
the terminal closes, when the program reading its output goes away, and after a failure. Only a
forced kill can skip this. If the dongle then keeps searching, replug it.

Output:

```text
nearby: waiting for the first signal
nearby: -58 dBm, strong signal, trend stronger, heard 800 ms ago
nearby: -58 dBm, strong signal, trend stronger, heard 800 ms ago
nearby: stopped
```

| Line | Meaning |
|---|---|
| `nearby: -58 dBm, strong signal, trend stronger, heard 800 ms ago` | A reading. Stronger than -60 dBm is `strong`, stronger than -80 dBm `moderate`, the rest `weak`. The trend is `stronger`, `steady`, `weaker` or `unknown` |
| `nearby: waiting for the first signal` | Nothing heard yet |
| `nearby: waiting for another signal; last heard N ms ago` | The last reading is more than 10 seconds old |
| `nearby: paused while audio is playing` | The dongle does not search while audio plays |
| `nearby: the search ended on the dongle` | The dongle stopped the search itself |
| `nearby: stopped` | The stop request was accepted |

A search that another app started is left alone, and `airpods nearby` fails until it ends.

Exit codes: `0` at the time limit, or if the dongle ended the search. `1` on failure, including a stop
request that failed. `2` for a bad command line. `130` if interrupted.

### perf

```text
airpods perf
```

Shows the dongle's performance counters once. It changes nothing. For a capture over time, use
[airpods diag](#airpods-diag).

Output:

```text
Uptime: 1h2m5s
Input: 1
Codec: AAC
Flags: 0x03
Buffer used: 960 of 4000
Buffer high water: 2210 of 4000
Underflow frames: 3
Dropped frames: 0
Encoded units: 182400
Encode time: 412 us
Encode time max: 1890 us
Send failures: 0
Recoveries: 1
Heap capacity: 256.0 KiB
Heap allocated: 118.0 KiB
Heap remaining: 138.0 KiB
Heap arena: 192.0 KiB
```

A counter the dongle does not report reads `not reported`.

Exit codes: `0` when the dongle answered. `1` on failure.

### info

```text
airpods info [--show-serial] [--capabilities]
```

Shows what the AirPods say about themselves, from the report the dongle keeps. It changes nothing, and it needs the
AirPods connected. The AirPods' serial number stays hidden until you ask for it, so that this output is safe to paste.

Output:

```text
Model: AirPods Pro 3
Model number: A3063
Firmware: 9A348
Serial: hidden; use --show-serial
```

| Line | Values | Notes |
|---|---|---|
| `Model` | A name, `unknown (not in the model catalog)`, `not reported` | The name comes from the catalog in this tool |
| `Model number` | Such as `A3063`, `not reported` | As the AirPods report it |
| `Firmware` | Such as `9A348`, `not reported` | The AirPods' firmware, not the dongle's |
| `Serial` | `hidden; use --show-serial`, the serial, `not reported` | Check it before you share it |

`--capabilities` adds what the model catalog lists: a flag for each feature, and each control with the setting that reads
it. This is the catalog, not a report from these AirPods. Firmware can narrow it further: Adaptive needs firmware 6, Pause
when asleep needs 8, and custom EQ needs 9. For an unlisted model it says `this model is not in the catalog`.

Exit codes: `0`. `1` if nothing was reported, or the AirPods are disconnected.

### hearing

> **Warning:** Hearing Aid and Media Assist change how sound is presented, including how loud it is. **None of this has
> run on real AirPods. It has only run against a simulator.** If a change sounds wrong or too loud, take the AirPods out,
> then turn the mode off. This is not a medical device or a medical tool. Both must first be set up on an iPhone or iPad,
> because setup needs a hearing test.

These commands work on AirPods Pro 2 and Pro 3, and refuse on any other model. They need the AirPods connected and ready,
and the microphone not in use. Each change is sent once. Nothing is ever repeated by itself.

| Result | Exit | Meaning |
|---|---|---|
| `... (confirmed by the AirPods)` | `0` | The AirPods reported the state that was asked for |
| `... (already reported; no write sent)` | `0` | Nothing was sent. See [Hearing recovery](#hearing-recovery) about old reports |
| `Error: REASON` | `1` | Refused, and nothing was sent. `REASON` says why |
| The reason, then `It may have applied. Read it again with airpods hearing status.` | `3` | Sent, but not confirmed. The state is unknown |

#### hearing status

```text
airpods hearing status
```

Reads the modes from the reports the dongle keeps. It sends nothing to the AirPods.

```text
Hearing Aid: off
Media Assist: off
Hearing Aid set up: yes
Media Assist set up: yes
Report ages: hearing aid 1000 ms, assist 1000 ms, media 1000 ms
```

A mode can read `unknown (not reported; this does not mean off)`. A report age says how long ago the dongle received that
report, which is how you tell whether it could show a change you just made.

#### hearing aid and hearing media-assist

```text
airpods hearing aid [--yes] {on,off}
airpods hearing media-assist [--yes] {on,off}
```

Turning a mode on shows a warning, then asks. Without a terminal to ask in, it needs `--yes`. Saying no, or having nobody
to ask, sends nothing and exits `1`. Turning a mode off asks nothing. A change that would be refused anyway is refused
before the question.

Turning on is refused unless the mode is set up, both modes have been reported and agree, the AirPods hold a writable saved
hearing profile, and Custom Transparency is known to be off.

#### hearing recover

```text
airpods hearing recover [--off {hearing-aid,media-assist}]
```

Without `--off` this only reads. It prints the state, and says which mode may be active and how to turn it off. It writes
nothing. With `--off`, it turns that mode off, with the same checks as `hearing aid off`.

#### Hearing recovery

After a change goes unconfirmed, the library holds a *recovery*: the mode may be active, so offer to turn it off. It keeps
this, and the last good packets, **in memory**. A command ends after each run, so a later run cannot have them. This is how
the CLI handles that, with no state written to disk and nothing weakened:

1. **The same run offers it.** If a person is at the terminal, the run that sent the change asks `Turn Hearing Aid off now?`.
   This is the strongest recovery, because this process still holds the last good packets. A yes that is confirmed exits
   `1`: the change you asked for did not happen, but the state is known. Anything else exits `3`.
2. **A later run starts from what is reported.** `hearing recover` reads the reports afresh. `--off MODE` is an explicit
   request, and runs an ordinary turn off with all of its checks. It is never automatic.

What a later run cannot do, and what to do instead:

- **It needs reports.** If the dongle holds no report of the modes, `--off` is refused, where the same run could still have
  used its packets. Reconnect the AirPods, so that they report again, then try again.
- **An old report can look current.** A later run cannot tell whether a kept report came before or after the change. If the
  report still says off, `--off` sends nothing and says so, even though the mode may be on. Compare the report ages with the
  time of your change. If in doubt, take the AirPods out, run `airpods reconnect`, then `airpods hearing status`.

#### hearing profile show

```text
airpods hearing profile show
```

Reads the profile stored on the AirPods. It writes nothing, and it does not turn anything on. The audiogram is in dB HL.

```text
Editable: yes
Bands: 250 500 1000 2000 3000 4000 6000 8000 Hz
Left: 10 11 12 13 14 15 16 17 dB HL
Right: 20 21 22 23 24 25 26 27 dB HL
Amplification: 0
Balance: -0.5
Tone: left 0.125, right -0.125
Conversation Boost: left off, right on
Stored gain: left 0.25, right -0.25
Ambient noise reduction: left 0.5, right 0.75
Own voice: 0.625
```

`Amplification` is the mean of the two ears' stored gains, and `Balance` is the right ear's gain minus the left's.
`Editable: no` means that it can be read, but this tool does not know how to write that record back.

#### hearing profile apply

```text
airpods hearing profile apply [--file FILE] [--left LEFT] [--right RIGHT] [--amplification A] [--balance B]
                              [--tone T] [--conversation-boost {on,off}] [--yes]
```

| Flag | Values | Notes |
|---|---|---|
| `--left`, `--right` | 8 comma separated values, 0 to 90 | dB HL at 250, 500, 1000, 2000, 3000, 4000, 6000 and 8000 Hz |
| `--amplification` | -1.5 to 1.5 | The mean of both ears' stored gains. Keeps the balance |
| `--balance` | -3 to 3 | The right ear's gain minus the left's. Keeps the amplification |
| `--tone` | -1 to 1 | For both ears |
| `--conversation-boost` | `on`, `off` | For both ears |
| `--file FILE` | A JSON file | Flags replace what the file says |
| `--yes` | | Do not ask |

Each ear's stored gain is the amplification minus half the balance for the left ear, plus half for the right, and each must
stay within -1.5 to 1.5. A value that puts an ear outside is rejected, never clamped. With both flags given that is a bad
command line, exit `2`. With one of them, the other is the stored value, so the profile is read first, and the error is
exit `1` with `Nothing was sent`:

```text
Error: Nothing was sent: amplification 1.4 with balance -0.5 puts the ears' gains at left 1.65, right 1.15; each must be -1.5 to 1.5.
```

A negative value in another form needs an equals sign, as in `--balance=-1e-1`. See [airpods](#airpods).

What neither gives stays as stored, byte for byte. A value equal to what `show` prints does not move, so that the stored gains
and each ear's tone survive. The file takes the names of the `profile` object of `--json hearing profile show`, and refuses
any other name, so that a misspelt field cannot quietly do nothing:

```sh
airpods --json hearing profile show | jq .profile > profile.json
# edit profile.json
airpods hearing profile apply --file profile.json
```

```json
{"left_db_hl": [10, 11, 12, 30, 14, 15, 16, 17], "amplification": 0.75, "tone": 0.25, "conversation_boost": true}
```

It reads the profile, lists each change, warns, and asks, unless `--yes` is given. It then checks on a fresh read that the
profile is still the one it read, and writes it once. The whole record must read back. Bad values exit `2`, without opening
the dongle. If nothing differs, it says `already stored; no write sent`.

It needs hearing assistance set up on the AirPods, and their Hearing Aid report. Hearing Aid may be on or off, and applying
leaves it as it is: it never turns a mode on or off. If the modes change around the write, the result is exit `3`.

### hardware

> **Warning:** these are results from the AirPods' own self-check. They are not a medical or repair diagnosis. **None of this
> has run on real AirPods.**

These commands work on AirPods Pro 2 and Pro 3, and need the AirPods connected and the microphone not in use.

#### hardware status

```text
airpods hardware status
```

Reads the saved result for each ear, and when it was measured. The time is the AirPods' own, shown as it is.

```text
Left: Passed
Left measured: 2025-06-15 15:06
Right: Passed (speaker)
Right measured: 2025-06-15 15:11
Read at: 22:13:20
```

A result is `Passed`, `Saved result reports a failure`, `Could not reach the test unit`, `No saved result`, or `Stopped:` with a
reason. Names in brackets are parts with a failure flag. If there is no record at all, it says `The AirPods have no saved
hardware results.`

#### hardware data

```text
airpods hardware data
```

Shows `Hearing test mode`, `Dosimetry sensitivity` and `Volume curve`, as raw values. They do not establish a sound level.

#### hardware check

```text
airpods hardware check [--buds-in-case]
```

Asks the AirPods for one in-case self-check, and waits up to 2 minutes for both ears' results to change. It only runs on
AirPods Pro 3 (A3063, A3064 or A3065) with firmware 9A348, with both buds reported in the case and no audio, recording or phone
in use.

1. Put both AirPods in the case and leave the lid open. Stop all audio, and disconnect any phone.
2. Run the command, and confirm that both are in the open case. Without a terminal, `--buds-in-case` confirms it. Without
   either, it exits `1` and sends nothing.
3. When it says `close the case now`, close it and wait. The AirPods disconnect during the check, which is expected.

It prints a `check:` line when something changes, and otherwise every 15 seconds:

```text
check: close the case now; left waiting, right waiting; 120 s left
check: new results arrived; left new result, right new result; 118 s left
```

**Ctrl-C stops the waiting, not the check.** No cancel command is known, so the AirPods may still finish. Read the results
later with `airpods hardware status`.

Exit codes: `0` new results. `1` refused, or no new results in time. `3` no reply to the request, so it may or may not have
started: do not start another yet. `130` Ctrl-C.

### experimental

> **Warning:** none of these has run on real AirPods, and they may not work through the dongle. **They have only run against a
> simulator.** The dongle or the AirPods can refuse a request or drop it silently. An error can mean that the dongle does
> not pass the request on, not that your AirPods lack the feature.

Every command here needs `--experimental`, before or after its subcommand. Without it, the command exits `2` and does not open
the dongle. With it, a one-line warning goes to stderr. Each one is sent once.

| Command | What it does | Models | The result |
|---|---|---|---|
| `transparency show` | Reads Custom Transparency | Pro 1, Pro 2, Pro 3 | The values |
| `transparency set` | Changes it, and reads it back | Pro 1, Pro 2, Pro 3 | `written and read back`, or exit `3` |
| `loud-sound-reduction get` | Reads it | Pro 2, Pro 3 | `on`, `off` |
| `loud-sound-reduction set {on,off}` | Changes it, and reads it back | Pro 2, Pro 3 | `written and read back`, or exit `3` |
| `eq show` | Reads the AirPods' last EQ report, as the dongle keeps it | Those that report EQ, on dongle firmware 0.1.64 or newer | The values |
| `eq set` | Changes it, and waits for a new EQ report | The same | `confirmed by the AirPods`, or exit `3` |
| `rename NAME` | Sends a new name | All | `sent`, and never more |

```text
airpods experimental --experimental transparency show
airpods experimental --experimental transparency set [--enabled {on,off}] [--ear {left,right,both}] [--amplification A]
                                                     [--tone T] [--noise-reduction N] [--conversation-boost {on,off}]
airpods experimental --experimental loud-sound-reduction get
airpods experimental --experimental loud-sound-reduction set {on,off}
airpods experimental --experimental eq show
airpods experimental --experimental eq set [--enabled {on,off}] [--low L] [--mid M] [--high H]
airpods experimental --experimental rename NAME
```

`transparency set` changes only what you give, for the ear in `--ear`, by default both. Amplification is -1 to 2, tone -1 to
1, and noise reduction 0 to 1. The AirPods keep the record in 104 bytes, or in 100 without Own Voice; both are read and
written back in the length they came in. On AirPods with hearing assistance, any change other than turning it off is refused,
before anything is sent, unless Hearing Aid and Media Assist are both reported off.

Loud Sound Reduction is looked up by its identifier on the AirPods every time, never at a fixed place, and a lookup that
finds it missing, twice or malformed is refused before anything is written.

`eq show` prints the EQ as the AirPods last reported it, which the dongle keeps from firmware 0.1.64 on. It sends nothing
to the AirPods:

```text
EQ: off
Low: 30
Mid: 50
High: 70
```

`eq set` changes only what you give: `--enabled`, and the bands from 0 to 100. It reads the report again just before it
sends, and the packet is that report with your values in it. It goes out once, and the change counts only when a new EQ report
from the AirPods carries exactly those values, within 8 seconds:

```text
eq: confirmed by the AirPods
EQ: on
Low: 10
Mid: 50
High: 70
```

If the report already holds the values, it says `eq: already reported; no write sent`. If no new report comes, or a different
one, the result is exit `3`. Whether `on` and `off` mean what the dongle firmware documents, and whether the AirPods apply the
change, has not been seen on real AirPods.

The dongle passes an EQ change on only while it knows the AirPods' model and holds a current EQ report from them. Otherwise
it refuses, nothing is sent, and the command fails with exit `1`:

```text
Error: These AirPods have not reported their EQ on this connection, so it cannot be read or changed.
Error: The dongle did not pass the EQ change on: it has no current EQ report from these AirPods, the report changed, or it does not know their model. Nothing was sent. Read the EQ again.
```

The first means there is no report to start from: the AirPods have not sent one since they connected, or the dongle firmware
is older than 0.1.64. The second means the dongle declined it: read the EQ again, and reconnect the AirPods if it keeps
happening.

`rename` takes 1 to 64 bytes of UTF-8 with no control characters, and the old name may show until the AirPods reconnect. Any
other name exits `2`, without opening the dongle. **`sent` means the dongle took it. The AirPods never answer a rename, so
nothing can say whether it worked.** Dongle firmware up to at least 0.1.66 passes on only a fixed list of AirPods requests,
and a rename is not on it. It refuses it before anything is sent, and the command fails with exit `1`:

```text
Error: The dongle refused to pass the name on to the AirPods: its firmware only sends a fixed list of AirPods requests. Nothing was sent.
```

### reconnect

```text
airpods reconnect
```

Asks the dongle to reconnect to its stored AirPods. If they are already connected, the dongle does
nothing. This is the plain reconnect request of every firmware: it says only that the dongle took the request.
[connect](#connect) also waits until the dongle owns the AirPods' audio.

On dongle firmware 0.1.59 or newer this is the same takeover as a press of the dongle's button. It has a time limit,
and it ends a release by [disconnect](#disconnect). It is refused as busy while the microphone is in use or another
takeover is pending.

Since firmware 0.1.58 the dongle no longer connects to the AirPods by itself, at startup or after a lost link, under the
`manual` [reconnect policy](#reconnect-policy), the default. It waits for the AirPods to connect, for a button press, for
`airpods connect` or for `airpods reconnect`. Under `auto` it also reconnects by itself.

Example:

```powershell
airpods reconnect
```

Output:

```text
reconnect: accepted by dongle
```

This means the dongle took the request, not that the AirPods are connected yet. Run
`airpods status` to see the result.

Exit codes: `0` accepted, `1` refused, `3` no receipt within 2 seconds.

### connect

```text
airpods connect
```

Connects the saved AirPods and waits up to 5 seconds until the dongle owns their audio. It also ends a release by
[disconnect](#disconnect). It never pairs.

On dongle firmware 0.1.59 or newer, whose `status --details` lists the `routing` capability, it reads the routing state and
sends nothing if the dongle already has the AirPods, with audio and controls up. Otherwise it asks for them through the routing
handover once, and waits for the dongle to report them owned:

```text
connect: AirPods connected; audio ownership confirmed.
connect: takeover accepted, still in progress. Use airpods status to follow it.
```

The second line means that the dongle accepted the request and goes on with it, but the AirPods had not confirmed it within 5
seconds. It is not an error.

On older firmware it sends the same request as [reconnect](#reconnect) and says only that:

```text
connect: AirPods connection requested; not yet confirmed. Use airpods status to check the connection.
```

The dongle refuses while the microphone or a call is in use, while a takeover, an audio change or a Game Mode change is in
flight, and while another app holds the settings session:

```text
Error: the dongle cannot take the AirPods back right now: the microphone or a call is in use, a takeover or audio change is pending, audio is recovering, a Game Mode change is in flight, or another app holds the settings lease
```

If something else changed the connection first, such as a button press, another app or an automatic reconnect under the
`auto` [reconnect policy](#reconnect-policy), the request is not sent again and the error says where it stands now. A
stale request is one exception: when the routing state moved before the request arrived, nothing was applied, and if the move
was an automatic reconnect attempt the tool reads the state once more and sends one fresh request.

Exit codes: `0` connected, already connected, still in progress, or requested on older firmware. `1` refused, or the takeover
ended without the AirPods confirming the dongle: they may be in their case, out of range or in use elsewhere. `3` no answer, so
the request may have applied: run `airpods status` before you try again.

### pair

```text
airpods pair [--yes]
```

Starts pairing new AirPods.

**This erases the stored pairing.** It disconnects the current AirPods, clears the dongle's pairing
slot and stops auto-reconnect. Do not use it as a way to disconnect. You will have to pair the
AirPods again.

It shows a warning, then asks. Without a terminal to ask in, it needs `--yes`. Saying no, or having
nobody to ask, sends nothing and exits `1`.

It is refused before the question, and nothing is sent, while USB audio is playing, the microphone
is in use or a phone is connected:

```text
Error: Control failure: busy; another companion request or settings session is running, or USB audio, the microphone or a phone is in use
```

The dongle can also answer busy after it has sent the request, for example while the AirPods microphone is about to start.
It may already have disconnected the AirPods and erased their pairing by then, so this is exit `3`, not a refusal:

```text
pair: the dongle answered busy, but it may already have disconnected the AirPods and erased their pairing; check status before retrying.
```

Run `airpods status`. If the AirPods are still connected, nothing happened and you can try again once the audio has stopped.
If not, pair them again.

Example:

```powershell
airpods pair
```

Output:

```text
Warning: this erases the saved pairing on the dongle. The AirPods paired now will have to be paired again to come back.
Replace the saved pairing? [y/N] y
pair: accepted by dongle
```

Then put the AirPods in pairing mode, as you would for a new device, and check with
`airpods status`.

Exit codes: `0` accepted, `1` refused before anything was sent, or not confirmed at the question. `3` no receipt within 2
seconds, or a busy answer after the request was sent.

### pair-phone

```text
airpods pair-phone
```

Opens the dongle's phone pairing window, so that a phone can pair with the dongle. The AirPods
pairing is not affected.

It is refused, and nothing is sent, while USB audio is playing or the microphone is in use, with the same `Control failure:
busy` error as [pair](#pair).

Example:

```powershell
airpods pair-phone
```

Output:

```text
pair-phone: accepted by dongle
```

Exit codes: `0` accepted, `1` refused, `3` no receipt within 2 seconds. `airpods status` does not
show the pairing window. After exit `3`, look for the dongle in the phone's Bluetooth list.

### disconnect

```text
airpods disconnect
```

Ends the AirPods' connection to the dongle and keeps them paired. It needs dongle firmware 0.1.59 or newer, whose
`status --details` lists the `routing` capability. [CLI-DISCONNECT.md](CLI-DISCONNECT.md) describes how it works.

Output:

```text
disconnect: released; the AirPods stay paired
The dongle now declines the AirPods' incoming connections and does not reconnect by itself until airpods connect, airpods reconnect, a press of its button or a restart of the dongle.
```

The first line is one of:

| Line | Meaning |
|---|---|
| `disconnect: released; the AirPods stay paired` | The connection is closed and the hold is set, or both already were |
| `disconnect: release accepted, not yet closed` | The dongle accepted the release, but it had not closed the connection within 5 seconds. It goes on closing it |

The release is also a hold: the dongle declines the AirPods' connections, even with them out of the case, and does not
reconnect by itself. So `disconnect` sends it even when the AirPods are not connected, unless a hold is already set. To
connect them again, run `airpods connect` or `airpods reconnect`, or press the dongle's button. Do not use `airpods pair`: it
erases the pairing.

The hold is not saved. **A restart of the dongle, such as unplugging it, ends it.** Under the default `manual`
[reconnect policy](#reconnect-policy) the dongle then still waits for the AirPods or for a request. Under `auto` it takes the
AirPods back by itself after the restart.

The dongle refuses a release while the microphone or a call is in use, while a takeover, an audio change or a Game
Mode change is in flight, and while another app, such as the tray app, holds the settings session:

```text
Error: the dongle cannot release the AirPods right now: the microphone or a call is in use, a takeover or audio connection is pending, audio is recovering, a Game Mode change is in flight, or another app holds the settings lease
```

If something else changed the connection first, such as a button press, another app or an automatic reconnect under the
`auto` policy, nothing is sent again. The brackets say where it stands now:

```text
Error: routing intent changed; read status again. The request was not retried (now owned by the dongle). Something else changed the connection first: a press of the dongle's button, another app, or an automatic reconnect under the auto reconnect policy. Run airpods status before you disconnect again
```

When the routing state moved before the release arrived, the dongle applied nothing. If the move was a reconnect attempt that
has ended, the tool reads the state once more and sends one fresh release. If that one is overtaken as well, the error starts
with `routing intent changed again during one fresh request`. While a takeover is still under way it is not sent, since the
dongle would refuse it: run `airpods status`, then disconnect again.

On older firmware the command sends nothing and fails:

```text
Error: this dongle firmware has no command to disconnect AirPods while keeping them paired; update it to 0.1.59 or newer, or put the AirPods in their case to disconnect
```

Exit codes: `0` released or accepted, `1` refused or unsupported, `3` no answer, so the release may have applied. After
exit `3`, run `airpods status` before you try again.

### reconnect-policy

```text
airpods reconnect-policy [manual|auto]
```

Without an argument it reads the reconnect policy the dongle keeps across restarts. With one it saves that policy. It needs
dongle firmware 0.1.63 or newer.

| Policy | What the dongle does |
|---|---|
| `manual` | The default. The dongle connects to the AirPods only for a press of its button, `airpods connect` or `airpods reconnect`. The AirPods can still connect to it |
| `auto` | It also reconnects by itself, with growing pauses, whenever the AirPods are not connected and not released by `disconnect`. **That can take them from another device that is using them** |

Output:

```text
Reconnect policy: manual
Reconnect policy: auto (saved and verified)
Reconnect policy: manual (no change needed; no write sent)
```

`saved and verified` means the dongle saved the policy and read it back. When the policy is already saved, nothing is sent.
Whenever `auto` is the result, a warning follows on stderr:

```text
Warning: under the auto policy the dongle connects to the AirPods by itself whenever they are not connected and not released, and can take them from another device using them. airpods disconnect holds it off only until the dongle restarts.
```

The dongle saves the policy only while it is idle: no playback, no microphone use or call, no connection or Game Mode change
under way, and no settings session open, the tray app's included. Otherwise it refuses, and nothing is saved:

```text
Error: The dongle saves the reconnect policy only while idle: pause playback, stop microphone use and calls, wait for connecting or a Game Mode change to finish, and close settings sessions; any settings lease blocks it, this app's own included
```

Older firmware, or a saved policy the dongle cannot read, gives this, and the dongle then acts as `manual`:

```text
Error: Reconnect policy unavailable: the dongle firmware is older than 0.1.63, or its saved configuration is unreadable
```

If the dongle could not verify a save, it may still have saved it. This is exit `3`, and the policy then reads as unavailable
until the dongle restarts or a later save verifies. **Do not repeat the command straight away.** Restart the dongle, read the
policy, then decide:

```text
reconnect policy: the dongle could not verify the save, so it may have been saved. Do not retry it automatically; the policy reads as unavailable until the dongle restarts or a later save verifies.
```

`status` never reads the policy.

Exit codes: `0` read, saved and verified, or already saved. `1` refused or unavailable, and nothing was saved. `2` a word other
than `manual` or `auto`. `3` the save may have happened.

### firmware

```text
airpods firmware
```

Shows the version of the firmware the dongle runs. It changes nothing.

Output:

```text
Firmware: 0.1.66 (experimental radio0076)
```

`(experimental radio0076)` marks the experimental radio build, which the firmware's releases ship. Dongle firmware without
the version report refuses the request, and the command fails with `the dongle firmware does not report its version`.

Exit codes: `0` when the dongle answered. `1` on failure, including firmware without the version report.

### completion

```text
airpods completion bash|zsh|fish|powershell
```

Prints a completion script to stdout. It covers commands, flags, setting names, setting values, and
serials from [`airpods list --serials`](#list), including the subcommands and flags of `hearing`, `hardware` and
`experimental`. It never opens a dongle. Percentages have no list of values to offer.

The [packages](../README.md#linux-packages) install it for bash, zsh and fish. Otherwise:

```sh
# bash: add to ~/.bashrc
source <(airpods completion bash)

# zsh: in a directory on your fpath, then start a new shell
airpods completion zsh > "${fpath[1]}/_airpods"

# fish
airpods completion fish > ~/.config/fish/completions/airpods.fish
```

In PowerShell, add this line to your `$PROFILE`:

```powershell
airpods completion powershell | Out-String | Invoke-Expression
```

Exit codes: `0` on success, `2` for an unknown shell.

## JSON output

`airpods --json status`, `settings`, `get` and `list`, and the newer read commands at the end of this section, print one JSON document to stdout. The text
output of those commands does not change without the flag. Errors stay on stderr as plain text, with
the same exit codes and nothing on stdout. There are two exceptions, where the document is printed
and the exit code is `1`, as in text mode: `get` on a setting the device has not reported, and `info`
when the AirPods have reported nothing about themselves.

Every document has `schema`, now `1`. Fields may be added within a schema version. A change that
would break a reader raises the number. Every field listed here is always present, with `null` where
the table says so.

### status document

```json
{
  "schema": 1,
  "state": "connected",
  "settings_ready": true,
  "codec": "AAC",
  "batteries": {
    "left": 80,
    "right": 70,
    "case": null
  }
}
```

| Field | Type | Values |
|---|---|---|
| `state` | string | `connected`, `disconnected` |
| `settings_ready` | boolean | Whether `set` can work right now |
| `codec` | string | `SBC`, `AAC`, `AAC-ELD`, `unknown` |
| `batteries.left`, `.right`, `.case` | number or null | `0` to `100`. `null` means no reading |

### settings and get documents

`settings` has a `settings` array with one object for each of the 25 settings, in the order of
[Settings](#settings-1). `get` has a `setting` object of the same shape:

```json
{
  "schema": 1,
  "setting": {
    "name": "tone-volume",
    "raw": 20530,
    "decoded": "50",
    "age_ms": 40,
    "unknown": false
  }
}
```

| Field | Type | Meaning |
|---|---|---|
| `name` | string | The setting's name |
| `raw` | number or null | The device value. `null` if the setting was not reported |
| `decoded` | string or null | The value as `get` shows it, always as a string. `null` if not reported, or if this tool cannot name the value |
| `age_ms` | number or null | The age of the report in milliseconds. `null` if not reported |
| `unknown` | boolean | `true` when `decoded` is `null` |

A setting with `unknown` true and a `raw` number was reported, with a value this tool has no name for.
`tone-volume` shows why `raw` and `decoded` differ.

### list document

```json
{
  "schema": 1,
  "dongles": [
    {
      "serial": "SIM0002",
      "usb_id": "4005",
      "path": "/dev/hidraw12",
      "connected": false,
      "error": null
    }
  ]
}
```

`dongles` is an empty array with nothing attached.

| Field | Type | Meaning |
|---|---|---|
| `serial` | string or null | The USB serial. `null` if the system gave none |
| `usb_id` | string | `4015`, `4005` |
| `path` | string | The node or device interface path |
| `connected` | boolean or null | Whether the AirPods are connected. `null` if the dongle could not be asked |
| `error` | string or null | The state text from [list](#list) when `connected` is `null` |

### Documents of the newer commands

Each has `schema`, and `null` for what was not reported.

`airpods --json info`: `model_name`, `model_number`, `firmware`, `serial`, `serial_hidden`. `serial` stays `null` without
`--show-serial`. With `--capabilities` it adds a `capabilities` object: `listed`, `family`, `over_ear`, `listening`,
`adaptive`, `allow_off_option`, `high_quality_mic`, `custom_eq`, `hearing_aid`, `custom_transparency`,
`loud_sound_reduction`, and `controls`, a list of `id` and `name`.

`airpods --json status --details`: the status document, and a `details` object with `dongle_serial`, `airpods_address`,
`noise_control`, `in_use`, `dongle_capabilities`, a list of the names in [status](#status), and `battery_left`,
`battery_right` and `battery_case`, each with `level`, `current`, `charging` and `age_ms`. It holds the AirPods' address,
so check it before you share it.

`airpods --json hearing status`: `hearing_aid`, `media_assist`, `hearing_enrolled`, `voice_enrolled`, `assist_enabled`,
`voice_enabled` and `media_enabled`, each `on`, `off` or `unknown`, and `report_age_ms` with `hearing_aid`, `assist` and
`media`.

`airpods --json hearing profile show`: `editable`, `bands_hz`, a `profile` object with `left_db_hl`, `right_db_hl`,
`amplification`, `balance`, `tone` and `conversation_boost`, which is what `hearing profile apply --file` takes, and a `stored`
object with `left_amplification`, `right_amplification`, `left_tone`, `right_tone`, `left_conversation_boost`,
`right_conversation_boost`, `left_ambient_noise_reduction`, `right_ambient_noise_reduction` and `own_voice`. In `profile`, `tone`
is the mean of the two ears and `conversation_boost` is true only if both ears have it.

`airpods --json hardware status`: `saved`, `read_at`, and `left` and `right`, each with `result`, `measured_at_seconds`,
`measured_at` and `failures`. Without a saved record, `saved` is `false` and the rest is `null`.

`airpods --json hardware data`: `available`, `read_at`, `test_mode`, `sensitivity` and `volume_curve`.

`airpods --json experimental --experimental transparency show`: `enabled`, then `left_` and `right_` for `amplification`, `tone`,
`conversation_boost` and `ambient_noise_reduction`. `airpods --json experimental --experimental loud-sound-reduction get`:
`state`. `airpods --json experimental --experimental eq show`: `enabled`, a boolean, and `low`, `mid` and `high`, numbers from 0
to 100.

## What the results mean

### Reading values

`get` and `settings` show what the AirPods have reported to the dongle. They do not ask the AirPods
for a new report, and they do not guess.

| Output | Meaning |
|---|---|
| `mode: anc (report age 1200 ms)` | The AirPods reported `anc`, and the dongle received that report 1200 ms ago |
| `microphone: unknown (not reported; may be unsupported)` | No report so far. The AirPods may not support this setting, or they may simply not have sent it yet. It does not say the setting is off |
| `mode: unknown device value (0x0) (report age 1 ms)` | The AirPods reported a value this tool has no word for |

An age of `4294967295 ms` is the largest number the field can hold. Treat it as "age not known".

### Results of set

`set` works in this order:

1. Check the value. A bad value is exit `2`, before the dongle is opened.
2. Open the dongle. Stop if settings are not ready or the microphone is in use.
3. Read the current report. If it already has the requested value, stop with `already reported`.
   Any failure up to here is exit `1` with `Error: ...`: nothing was sent, so it cannot have applied.
4. Send the write once.
5. For up to about 5 seconds, wait for a new report with the requested value.

A successful USB transfer alone does not count as confirmation. The tool needs a report with the
requested value and a new revision number, on the same connection. If the AirPods reconnect during
this time, the operation ends without confirmation.

| Result | Output contains | Exit | Did the AirPods change? |
|---|---|---|---|
| Confirmed | `confirmed by AirPods` | `0` | Yes. The AirPods reported the new value after the write |
| Already reported | `already reported, N ms old; no write sent` | `0` | Nothing was sent. The last report already had this value. Check `N` if it may be old |
| Not confirmed | `sent, but no fresh matching device report` or `change not confirmed` | `3` | Not known. It may have applied |
| Refused | `Error: ...` | `1` | No, for the refusals listed in [CLI-TROUBLESHOOTING.md](CLI-TROUBLESHOOTING.md#results-from-set) |
| Failed before sending | `Error: ...` | `1` | No. Nothing was sent |
| Invalid | `airpods set: error: ...` | `2` | No. Nothing was sent |

### After exit 3

Exit `3` means the write may have applied. Follow these steps:

1. Do not repeat the command straight away.
2. Run `airpods get SETTING`.
3. If it shows the value you wanted and a small report age, you are done.
4. If it shows the old value, or `unknown`, run `airpods status`.
5. If status shows `AirPods: connected` and `Settings: ready`, run the `set` once more. This is safe:
   if the value did apply and was reported, `set` prints `already reported` and sends nothing.
6. If not, fix the connection first. See [CLI-TROUBLESHOOTING.md](CLI-TROUBLESHOOTING.md).

Some settings may change without a new report. NTPods saw this with `press-speed` on first
generation AirPods Pro. If a setting keeps giving exit `3`, test the behaviour on the AirPods.

For `reconnect`, `pair` and `pair-phone`, exit `3` means no receipt came from the dongle, or for `pair` a busy answer
after the request went out. For `connect` and `disconnect` it means no answer to the handover. Run `airpods status` before
you try again. For `reconnect-policy`, see [reconnect-policy](#reconnect-policy).

The tool never retries a write that may have applied, and never reconnects, takes over audio or changes playback volume
by itself: only `connect` and `reconnect` ask for the AirPods, when you run them. The one write it repeats is Off after the dongle refused it for want of Allow Off: see [allow-off](#allow-off).

## Settings

The notes below apply to all settings:

- `get` shows what the AirPods have reported. `unknown` can mean not reported yet, not only
  unsupported.
- The dongle and the AirPods have the last word. A setting listed here may still be refused.
- **Models** comes from the
  [NTPods model catalog](https://github.com/Ripthulhu/ntpods/blob/main/docs/WINDOWS-MODEL-SUPPORT.md).
  It says which models usually have the setting. Most of these combinations have not been tested
  with this tool, and AirPods firmware can change them.
- **Usual default** is the factory setting as far as known. These tools do not read or restore
  defaults. "Not known" means we have no reliable value.
- Values are exact and case-sensitive. Lists use commas with no spaces.

The tool does not offer raw packets, hearing profile edits, or changes to music volume.

### mode

The listening mode: how much outside sound you hear.

- **Values:** `off`, `anc` (noise cancellation), `transparency`, `adaptive`
- **Usual default:** not known. The AirPods keep the last mode used
- **Models:** AirPods 4 with ANC, AirPods 5, AirPods Pro (all), AirPods Max (all), Powerbeats Pro 2.
  `adaptive` needs AirPods 4 with ANC, AirPods 5, Pro 2, Pro 3 or Max 2
- **Notes:** on Pro 2 and Pro 3, whether `off` is available depends on [allow-off](#allow-off).

### microphone

Which earbud's microphone is used.

- **Values:** `auto`, `right`, `left`
- **Usual default:** `auto`
- **Models:** AirPods 1 to 5, AirPods Pro (all). Not AirPods Max

### ear-detection

Whether the AirPods notice when you put them in or take them out, for example to pause playback.

- **Values:** `on`, `off`
- **Usual default:** `on`
- **Models:** all AirPods models, and Powerbeats Pro 2
- **Notes:** since dongle firmware 0.1.65 the dongle itself pauses music played from USB when a bud comes out, and plays it
  again when it goes back in, through its media keys. This setting is the only way to switch that off.

### one-bud-anc

Whether noise cancellation stays available when you wear only one earbud.

- **Values:** `on`, `off`
- **Usual default:** `off`
- **Models:** AirPods 4 with ANC, AirPods 5, AirPods Pro (all)

### conversation-awareness

Whether the AirPods lower media and let in voices when you start talking.

- **Values:** `on`, `off`
- **Usual default:** not known
- **Models:** AirPods 4 with ANC, AirPods 5, Pro 2, Pro 3, Max 2
- **Notes:** since dongle firmware 0.1.65 the dongle also lowers the music it sends while this is on and you talk.

### personalized-volume

Whether the AirPods adjust media volume by themselves, based on your surroundings and habits.

- **Values:** `on`, `off`
- **Usual default:** not known
- **Models:** AirPods 4 (both versions), AirPods 5, Pro 2, Pro 3, Max 2

### allow-off

Whether the `off` listening mode is allowed.

- **Values:** `on`, `off`
- **Usual default:** not known
- **Models:** Pro 2, Pro 3
- **Notes:** the dongle forgets this whenever the AirPods connect again. AirPods Pro 3 never report it,
  so the dongle then refuses `set mode off` and `set hold-modes` with `off` until it is sent again.
  When that happens on AirPods that never report it, `set` sends `allow-off on`, prints that line, and
  sends the refused write once more. The dongle refused that write before passing it on, so the AirPods
  receive it once. Each run of the tool is new and cannot know what an earlier run sent, so it
  sends `allow-off on` whenever the dongle refuses Off this way. On other models a refused Off is
  reported as it is: set this first.

### pause-when-asleep

Whether media pauses when the AirPods detect that you fell asleep.

- **Values:** `on`, `off`
- **Usual default:** not known
- **Models:** AirPods 4 (both versions), AirPods 5, Pro 2, Pro 3, Max 2. Needs recent AirPods firmware

### case-sounds

Whether the charging case plays sounds, for example when charging starts.

- **Values:** `on`, `off`
- **Usual default:** `on`
- **Models:** cases with a speaker: AirPods 4 with ANC, AirPods 5 with the wireless charging case,
  Pro 2, Pro 3

### optimized-charging

Whether the AirPods limit charging to reduce battery wear.

- **Values:** `on`, `off`
- **Usual default:** `on`
- **Models:** AirPods 3, 4 and 5, AirPods Pro (all), AirPods Max (all). Not AirPods 1 or 2

### volume-swipe

Whether swiping on the stem changes the volume.

- **Values:** `on`, `off`
- **Usual default:** `on`
- **Models:** AirPods 5 with the wireless charging case, Pro 2, Pro 3

### press-speed

How much time you get between the presses of a double or triple press.

- **Values:** `default`, `slower`, `slowest`
- **Usual default:** `default`
- **Models:** AirPods 3, 4 and 5, AirPods Pro (all)
- **Notes:** on first generation AirPods Pro, a change may apply without a new report, which gives
  exit `3`. See [After exit 3](#after-exit-3).

### hold-duration

How long a press and hold on the stem takes to register.

- **Values:** `default`, `slower`, `slowest`
- **Usual default:** `default`
- **Models:** AirPods 3, 4 and 5, AirPods Pro (all)

### swipe-duration

How much time a volume swipe on the stem gets.

- **Values:** `default`, `longer`, `longest`
- **Usual default:** `default`
- **Models:** AirPods 5 with the wireless charging case, Pro 2, Pro 3

### call-controls

How you end a call with a press.

- **Values:** `default` (press twice to end a call), `swapped` (press once to end a call)
- **Usual default:** `default`
- **Models:** AirPods 3, 4 and 5, AirPods Pro (all), AirPods Max (all)
- **Notes:** what this does to the mute gesture has not been established. Test it on your AirPods.

### crown-direction

Which way you turn the Digital Crown to raise the volume.

- **Values:** `default`, `reversed`
- **Usual default:** `default`
- **Models:** AirPods Max (all)

### tone-volume

The volume of notification sounds from the AirPods, such as connection tones. This is not music
volume.

- **Values:** whole number from `0` to `100`
- **Usual default:** not known
- **Models:** all AirPods models. Not Powerbeats Pro 2

### case-volume

The volume of sounds from the charging case. This is not music volume.

- **Values:** whole number from `0` to `100`
- **Usual default:** not known
- **Models:** cases with a speaker: AirPods 4 with ANC, AirPods 5 with the wireless charging case,
  Pro 2, Pro 3

### adaptive-strength

How strongly `adaptive` mode reduces outside sound.

- **Values:** whole number from `0` to `100`
- **Usual default:** not known
- **Models:** AirPods 4 with ANC, AirPods 5, Pro 2, Pro 3, Max 2
- **Notes:** which end of the scale lets in more sound has not been checked here.

### hold-modes

Which listening modes a press and hold cycles through.

- **Values:** at least two different modes from `off`, `anc`, `transparency`, `adaptive`, separated
  by commas, such as `anc,transparency,adaptive`
- **Usual default:** not known
- **Models:** AirPods 4 with ANC, AirPods 5, AirPods Pro (all), AirPods Max (all)
- **Notes:** the choice is shared by both ears. It is a selection, not an ordered cycle:
  `get` always lists modes in the order `off,anc,transparency,adaptive`. Including `off` may need
  [allow-off](#allow-off).

### hold-actions

What a press and hold does on each stem.

- **Values:** two actions, left then right, separated by a comma. Each is `noise-control` or `siri`,
  such as `noise-control,siri`
- **Usual default:** `noise-control,noise-control`
- **Models:** AirPods 3, 4 and 5, AirPods Pro (all)
- **Notes:** this always sets both sides. The left then right order has not been confirmed on real
  AirPods, because sources disagree. Test each stem after setting it. See the
  [hardware checklist](CLI-TESTING.md#hardware-checklist).

### connect-automatically

The AirPods' "connect automatically" option.

- **Values:** `on`, `off`
- **Usual default:** not known
- **Models:** not known
- **Notes:** its exact effect has not been tested on real AirPods. The AirPods may not report a change,
  in which case `set` exits `3`; check with `get`.

### allow-auto-connect

The AirPods' "allow automatic connection" option.

- **Values:** `on`, `off`
- **Usual default:** not known
- **Models:** not known
- **Notes:** its exact effect has not been tested on real AirPods. The AirPods may not report a change,
  in which case `set` exits `3`; check with `get`.

### en352-protection

The AirPods' EN 352 hearing protection option.

- **Values:** `on`, `off`
- **Usual default:** not known
- **Models:** AirPods Pro 3
- **Notes:** needs dongle firmware 0.1.64 or newer, and reads `unknown` on older firmware. The dongle writes it only once
  the AirPods have reported it since they connected; before that, `set` is refused with exit `1` and nothing is sent. Its
  effect has not been tested on real AirPods.

### gain-swipe

The AirPods' Gain Swipe option.

- **Values:** `on`, `off`
- **Usual default:** not known
- **Models:** AirPods Pro 2, Pro 3
- **Notes:** as for [en352-protection](#en352-protection): dongle firmware 0.1.64 or newer, and written only once the
  AirPods have reported it. Its effect has not been tested on real AirPods.

## Try it without a dongle

`AIRPODS_CLI_SIM=1` uses a simulated dongle with demo AirPods Pro 3, for trying the commands with no hardware. Any value
other than empty or `0` does this. With `AIRPODS_CLI_SIM=case`, both buds are in the case, so that `hardware check` can start.
It lasts one run, so a change is gone by the next command. `airpods diag` uses the same simulated dongle.

Every such run first prints `note: simulated dongle (AIRPODS_CLI_SIM)` to stderr, so that its results are not taken for those
of real AirPods. stdout stays as it is, so JSON output can still be parsed.

## Multiple dongles

With one dongle attached you do not need `--serial`. With several, the tool stops with
`Expected exactly one matching Pico ...` unless you pick one. [`airpods list`](#list) shows the
serials:

```powershell
airpods --serial YOUR_PICO_USB_SERIAL get mode
airpods diag --serial YOUR_PICO_USB_SERIAL --seconds 30 --output capture.jsonl
```

The serial is the Pico's USB serial number. It does not identify the AirPods or a COM port. It must
match exactly, including upper and lower case. A wrong serial gives the same `Expected exactly one`
error.

### Find the serial without airpods list

These commands work without this tool.

### Find the serial on Linux

```sh
grep -lE '^HID_ID=0003:0000CAFE:0000(4015|4005)$' /sys/class/hidraw/hidraw*/device/uevent |
  xargs grep -H '^HID_UNIQ='
```

Each output line is one matching HID node. The serial follows `HID_UNIQ=`. No root is needed.

### Find the serial on Windows

```powershell
Get-PnpDevice -PresentOnly |
  Where-Object InstanceId -Match '^USB\\VID_CAFE&PID_(4015|4005)\\' |
  ForEach-Object { ($_.InstanceId -split '\\')[-1] }
```

Each output line is the serial of one dongle. This command has not yet been run against a real
dongle.

On Linux, a dongle you have no permission to open is skipped when another one can be opened. If the
tool picks an unexpected dongle, check the [udev rule](CLI-TROUBLESHOOTING.md#linux-found-node-but-no-permission).

## airpods diag

Captures audio diagnostics as JSON Lines, one object per line. It never changes settings. No
companion app is needed. Up to airpods-cli 0.3 this was the separate program `airpods-diag`, with the
same flags and the same output: write `airpods diag` where a script said `airpods-diag`.

`--serial` and `--debug` also work before `diag`, as for every command. `--json` does not apply: the
output is JSON Lines already.

```text
airpods diag [-h] [--version] [--seconds SECONDS] [--serial SERIAL]
             [--output OUTPUT] [--legacy] [--inspect] [--phone] [--radio]
             [--no-trace] [--replace]
```

| Flag | Meaning |
|---|---|
| `-h`, `--help` | Show help and exit |
| `--version` | Show the version and exit, as in `airpods v0.0.2-alpha`. A build with no tag to read shows `dev` |
| `--seconds N` | Capture length, `1` to `1800`. Default `60` |
| `--serial SERIAL` | Use the dongle with this USB serial. Works on Windows and Linux |
| `--output FILE` | Write to this file instead of stdout. The file must not already exist unless `--replace` is given, and it is created only once the dongle is open |
| `--replace` | Overwrite the `--output` file if it exists. Needs `--output` |
| `--legacy` | Only the basic audio and performance fields, without `trace`. Cannot be combined with `--inspect`, `--phone` or `--radio` |
| `--inspect` | Add signal levels and the latest submitted AAC-ELD frame. Cannot be combined with `--legacy` or `--radio` |
| `--phone` | Add phone input queue and decoder counters. Cannot be combined with `--legacy` or `--radio` |
| `--no-trace` | Leave out `trace`. Unlike `--legacy`, this combines with `--inspect` and `--phone`, as the tray app's Audio trace option does |
| `--radio` | Only cached radio and adaptive bitrate readings. Cannot be combined with the others |

A sample is written every second, or every two seconds with `--radio`.

`--inspect` asks the firmware to turn on inspection. The tool renews this every 10 seconds and turns
it off at the end. If turning it off fails, the tool prints a warning, and the firmware ends
inspection by itself after 120 seconds.

Examples:

```powershell
airpods diag --seconds 30
airpods diag --seconds 30 --inspect --phone --output capture.jsonl
airpods diag --seconds 120 --radio --output radio.jsonl
```

On Windows, use `--output` rather than `>`. Windows PowerShell 5.1 saves redirected output as
UTF-16, which is harder to work with.

If the dongle cannot be opened, `--output` creates no file. If the capture fails later, the file
stays, possibly empty, and the next run with that name fails with `file exists`. Delete the file, use
a new name, or add `--replace`. To end a capture early, press Ctrl-C. Lines already written are kept.
The same goes when the process is asked to end, when its terminal closes, and when whatever reads its
stdout goes away, as with `airpods diag --inspect | head`: `--inspect` is turned off first on every one of
those ways out.

### Exit codes

| Code | Meaning |
|---|---|
| `0` | Capture finished |
| `1` | Failure. The file may still hold the lines written so far |
| `2` | Invalid arguments |
| `130` | Stopped early: Ctrl-C, a request to end, a closed terminal, or a closed pipe on stdout |

### Output samples

As with `airpods`, these samples are from the simulator. The numbers have no real meaning.

`airpods diag --seconds 2 --legacy`:

```json
{"time": 1700000000.2499998, "game_requested": false, "game_phase": 0, "audio_flags": 0, "delay_tenths_ms": 65535, "performance": {"uptime_ms": 4, "pcm_used": 8, "pcm_capacity": 4000, "pcm_peak": 16, "underflows": 20, "drops": 24, "encoded_units": 28, "encode_us": 32, "encode_max_us": 36, "send_failures": 40, "recoveries": 44, "heap_capacity": 9000, "heap_allocated": 52, "heap_remaining": 56, "heap_arena": 60}}
```

`airpods diag --seconds 25 --inspect --phone`:

```json
{"time": 1700000000.2499998, "game_requested": false, "game_phase": 0, "audio_flags": 0, "delay_tenths_ms": 65535, "performance": {"uptime_ms": 4, "pcm_used": 8, "pcm_capacity": 4000, "pcm_peak": 16, "underflows": 20, "drops": 24, "encoded_units": 28, "encode_us": 32, "encode_max_us": 36, "send_failures": 40, "recoveries": 44, "heap_capacity": 9000, "heap_allocated": 52, "heap_remaining": 56, "heap_arena": 60}, "trace": {"time_us_wrapped": 1000, "event_serial": 1001, "pcm_used": 1002, "sample_debt": 1003, "game_phase": 1004, "rtp_timestamp": 1005, "reserved": 1006, "stats": {"usb_packets": 2000, "usb_frames": 2001, "usb_gap_max_us": 2002, "usb_bad_packets": 2003, "usb_short_reads": 2004, "startup_dropped_frames": 2005, "active_dropped_frames": 2006, "startup_zero_frames": 2007, "active_zero_frames": 2008, "timer_late_max_us": 2009, "encode_max_us": 2010, "send_wait_max_us": 2011, "send_gap_max_us": 2012, "packet_age_max_us": 2013, "controller_flushes": 2014, "controller_completed_packets": 2015, "discarded_debt_frames": 2016, "timeline_recoveries": 2017, "pcm_min": 2018, "pcm_max": 2019, "clock_rate_q16": 2020, "ble_scan_requested": 2021, "usb_alternate_setting": 2022, "encode_unit_max_us": 2023}, "events": [{"time_us_wrapped": 3000, "kind": "drop", "a": 4000, "b": 5000}, {"time_us_wrapped": 3001, "kind": "game", "a": 4001, "b": 5001}, {"time_us_wrapped": 3002, "kind": 77, "a": 4002, "b": 5002}]}, "inspect": {"usb_samples": 100, "usb_nonzero_samples": 101, "usb_peak_q15": 102, "usb_last_nonzero_us": 103, "pcm_samples": 104, "pcm_nonzero_samples": 105, "pcm_peak_q15": 106, "pcm_last_nonzero_us": 107, "submitted_au_count": 108, "submitted_au_us": 109, "submitted_au_bytes": 4, "encoder_generation": 111, "submitted_au_sequence": 112, "usb_last_packet_us": 113, "snapshot_us": 111, "rtp_timestamp": 222, "bad_eld_packets": 333, "truncated": false, "active": true, "asc_hex": "1112", "au_hex": "030a1118"}, "phone": {"packets": 10, "malformed": 11, "sequence_gaps": 12, "submit_failures": 13, "decode_failures": 14, "buffer_resets": 15, "decoded_frames": 16, "last_packet_ms": 17, "queued_packets": 18, "output_ready": 1, "last_decode_us": 20, "max_decode_us": 21, "codec": 2, "flags": 5}}
```

`airpods diag --seconds 5 --radio`:

```json
{"time": 1700000000.2499998, "radio": {"snapshot_ms": 900, "epoch": 901, "handle": 2748, "connected": true, "sampling": true, "pending": true, "valid_mask": 31, "failed_mask": 3, "unavailable_mask": 4, "rssi_relative_db": -42, "quality": 200, "tx_power_dbm": -3, "max_tx_power_dbm": 12, "afh_mode": 1, "afh_map": "a0a1a2a3a4a5a6a7a8a9", "age_ms": {"rssi": 10, "quality": 20, "tx_power": 30, "max_tx_power": 40, "afh_map": 50}, "flush_events": 77, "completed_packets": 88, "adaptive": {"current_bps": 1000, "ceiling_bps": 1010, "target_bps": 1020, "changes": 1030, "errors": 1040, "last_change_ms": 1050, "window_flush_events": 1060, "window_pressure_events": 1070, "window_elapsed_ms": 1080, "floor_bps": 1090, "clean_ms": 1100, "total_pressure_events": 1120, "flush_events": 1130, "radio_epoch": 1140, "active": true, "frozen": true, "fault": true, "variable": true, "reason": "pressure"}}}
```

### JSON Lines fields

These fields are debugging data, and they can change between versions. Where this table gives no
meaning, the field name is all we can say for certain. Suffixes give the unit: `_us` is
microseconds, `_ms` milliseconds, `_bps` bits per second.

**Top level, in every line**

| Field | Meaning |
|---|---|
| `time` | Time of the sample on your computer, in Unix seconds |

**Top level, without `--radio`**

| Field | Meaning |
|---|---|
| `game_requested` | Whether game mode was requested |
| `game_phase` | Game mode state: `0` off, `1` pending, `2` unconfirmed, `3` confirmed, `4` failed, `5` unsupported, `6` restoring |
| `audio_flags` | Sum of: `1` change pending, `2` streaming, `4` microphone active, `8` game mode supported |
| `delay_tenths_ms` | Reported delay, in tenths of a millisecond. `65535` means not reported |
| `performance` | See below |

**`performance`**

A value of `4294967295` means not reported.

| Group | Fields |
|---|---|
| Uptime | `uptime_ms` |
| PCM buffer | `pcm_used`, `pcm_capacity`, `pcm_peak` |
| Audio faults | `underflows`, `drops`, `send_failures`, `recoveries` |
| Encoder | `encoded_units`, `encode_us`, `encode_max_us` |
| Memory | `heap_capacity`, `heap_allocated`, `heap_remaining`, `heap_arena` |

**`trace`**

Present unless `--legacy` or `--radio` is used. If the firmware does not support it, the line has
`"trace_supported": false` instead.

| Group | Fields |
|---|---|
| Snapshot | `time_us_wrapped`, `event_serial`, `pcm_used`, `sample_debt`, `game_phase`, `rtp_timestamp`, `reserved` |
| `stats`: USB input | `usb_packets`, `usb_frames`, `usb_gap_max_us`, `usb_bad_packets`, `usb_short_reads`, `usb_alternate_setting` |
| `stats`: lost or filled audio | `startup_dropped_frames`, `active_dropped_frames`, `startup_zero_frames`, `active_zero_frames`, `discarded_debt_frames` |
| `stats`: timing | `timer_late_max_us`, `encode_max_us`, `encode_unit_max_us`, `send_wait_max_us`, `send_gap_max_us`, `packet_age_max_us`, `timeline_recoveries`, `clock_rate_q16` |
| `stats`: controller | `controller_flushes`, `controller_completed_packets`, `ble_scan_requested` |
| `stats`: PCM range | `pcm_min`, `pcm_max` |
| `events` | Recent events, up to 24. Each has `time_us_wrapped`, `kind`, `a`, `b` |

`kind` is one of `drop`, `zero_fill`, `timer_late`, `send_wait`, `timeline_recovery`,
`controller_flush`, `usb_error`, `format` or `game`. An event type this tool does not know appears
as a number. `a` and `b` are values that depend on the event.

**`inspect`**

Only with `--inspect`. If the firmware does not support it, the capture fails.

| Group | Fields |
|---|---|
| USB signal | `usb_samples`, `usb_nonzero_samples`, `usb_peak_q15`, `usb_last_nonzero_us`, `usb_last_packet_us` |
| PCM signal | `pcm_samples`, `pcm_nonzero_samples`, `pcm_peak_q15`, `pcm_last_nonzero_us` |
| Latest audio frame | `submitted_au_count`, `submitted_au_us`, `submitted_au_bytes`, `submitted_au_sequence`, `encoder_generation`, `rtp_timestamp`, `bad_eld_packets` |
| Frame data | `au_hex` (up to 320 bytes of the frame), `truncated` (the frame was longer), `asc_hex` (codec configuration) |
| State | `snapshot_us`, `active` |

**`phone`**

Only with `--phone`. If the firmware does not support it, the line has `"phone_supported": false`
instead.

| Group | Fields |
|---|---|
| Input | `packets`, `malformed`, `sequence_gaps`, `queued_packets`, `last_packet_ms`, `buffer_resets` |
| Decoder | `submit_failures`, `decode_failures`, `decoded_frames`, `last_decode_us`, `max_decode_us`, `codec` |
| Output | `output_ready` (`0` or `1`), `flags` |
| Newer firmware only | `max_media_gap_ms`, `max_pcm_gap_ms`, `timestamp_discontinuities` |

**`radio`**

Only with `--radio`.

| Group | Fields |
|---|---|
| Snapshot | `snapshot_ms`, `epoch`, `handle`, `connected`, `sampling`, `pending` |
| Readings | `rssi_relative_db`, `quality`, `tx_power_dbm`, `max_tx_power_dbm`, `afh_mode`, `afh_map`. Each is `null` when there is no valid reading |
| Reading state | `valid_mask`, with bits `1` RSSI, `2` quality, `4` TX power, `8` max TX power, `16` AFH. `failed_mask` and `unavailable_mask` are similar masks |
| `age_ms` | `rssi`, `quality`, `tx_power`, `max_tx_power`, `afh_map`. Each is `null` when there is no reading |
| Link counters | `flush_events`, `completed_packets` |
| `adaptive`: bitrate | `current_bps`, `ceiling_bps`, `target_bps`, `floor_bps`, `changes`, `errors`, `last_change_ms` |
| `adaptive`: pressure | `window_flush_events`, `window_pressure_events`, `window_elapsed_ms`, `clean_ms`, `total_pressure_events`, `flush_events`, `radio_epoch` |
| `adaptive`: state | `active`, `frozen`, `fault`, `variable`, `reason` |

`reason` is one of `off`, `startup`, `clean`, `pressure`, `recover`, `frozen` or `fault`.

### Send a capture for debugging

1. Close NTPods, the tray app and any other `airpods` or `airpods diag` that is running.
2. Start a capture just before you reproduce the problem, with a new file name.

   ```powershell
   airpods diag --seconds 120 --phone --output capture.jsonl
   ```

   For Bluetooth dropouts or bitrate problems, also run a separate capture with `--radio` in place
   of `--phone`. Add `--inspect` only if asked.
3. Reproduce the problem. Note what you did and when.
4. Save the output of `airpods status` and `airpods settings`.
5. Compress the capture.

   ```powershell
   Compress-Archive capture.jsonl capture.zip
   ```

   ```sh
   gzip capture.jsonl
   ```

6. Attach the file to a GitHub issue, with these details:
   - Windows or Linux, and the output of `airpods --version`. v0.0.2-alpha has no such flag, so for
     that release give the release name.
   - Your AirPods model.
   - What you did, what you expected and what happened.
   - The exit code and any error message from the capture.

What a capture contains: counters, timing and radio readings. It has no USB serial, AirPods address
or file paths. With `--inspect`, `au_hex` also holds up to 320 bytes of one encoded audio frame, so
stop any private audio before you use that flag.
