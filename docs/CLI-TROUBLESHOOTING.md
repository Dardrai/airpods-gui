# Troubleshooting

Find your message below, or search this page for it. For commands and settings, see
[CLI.md](CLI.md).

Windows has not been tested on real hardware yet. Windows causes and commands here come from the
code and general Windows behaviour. Messages that start with `[WinError` or `[Errno` come from the
operating system.

## First checks

Run `airpods list` to see each attached dongle and whether it can be used, then `airpods status`,
and work from the result:

| Result | Go to |
|---|---|
| `Expected exactly one matching Pico ...` | [Dongle not found](#dongle-not-found) |
| `found /dev/hidrawN but no permission` | [Linux: found node but no permission](#linux-found-node-but-no-permission) |
| `AirPods: disconnected` | [AirPods disconnected](#airpods-disconnected) |
| `Settings: not ready` | [Settings not ready](#settings-not-ready) |
| A hearing, hardware or experimental message | [Hearing, hardware and experimental commands](#hearing-hardware-and-experimental-commands) |
| `AirPods: connected` and `Settings: ready` | The link is fine. Look up the message from your other command |

A line of `airpods list` that says `cannot be opened: REASON` has the same `REASON` as these messages.

## Dongle not found

```text
Error: Expected exactly one matching Pico HID interface (CAFE:4015/4005)
```

On Windows the same problem reads:

```text
Error: Expected exactly one matching Pico vendor HID collection
```

The tool needs exactly one dongle that it can open. Exit code `1`. `airpods list` shows what the tool
can see: `No dongles found.` means the system does not show a dongle at all. This has four usual
causes.

**The dongle is absent.** It is not plugged in, the cable only carries power, or the Pico is not
running compatible firmware. A Pico in bootloader mode shows up as a USB drive and not as a dongle.
Replug it and use another data cable.

Check that the system sees it. On Linux:

```sh
lsusb -d cafe:
```

On Windows:

```powershell
Get-PnpDevice -PresentOnly | Where-Object InstanceId -Match 'VID_CAFE&PID_(4015|4005)'
```

If nothing is listed, the problem is the USB connection or the firmware, not this tool.

**Several dongles are attached.** Pick one with `--serial`, or unplug the others. See
[Multiple dongles](CLI.md#multiple-dongles).

**The `--serial` value is wrong.** It must match exactly, including upper and lower case, and it
must come before the command.

**The dongle is not accessible.** On Linux, this is a permissions problem. Builds since
`v0.0.2-alpha` report it with the message in the next section. Older builds show only the
`Expected exactly one` message, so update, or try the [udev rule](#install-the-udev-rule). You can
confirm it once with `sudo airpods status`. If that works, permissions are the cause. Do not keep
running the tool with `sudo`.

## Linux: found node but no permission

```text
Error: found /dev/hidraw3 but no permission to open it; install the AirPods udev rule; see the docs
```

The dongle is there, but your user may not open its device node. Exit code `1`. The number in
`/dev/hidraw3` varies.

The [packages](../README.md#download-and-install)
install the rule file as `/usr/lib/udev/rules.d/70-airpods-hid.rules`. After installing a package, replug the dongle. If the message
stays, your session has no seat: see [SSH and other sessions](#ssh-and-other-sessions-without-seat-access).

### Install the udev rule

Without a package, run this in the folder that holds the rule file. In a clone
of this repo the file is [core/hid/70-airpods-hid.rules](../core/hid/70-airpods-hid.rules).

```sh
sudo cp 70-airpods-hid.rules /etc/udev/rules.d/
sudo udevadm control --reload
sudo udevadm trigger --subsystem-match=hidraw --action=add
```

Then unplug the dongle and plug it back in, and run `airpods status`.

The rule covers both product IDs, `4015` and `4005`. It gives access to the user at the active local
seat, which is the person logged in at the machine.

### SSH and other sessions without seat access

The standard rule does not help if your session has no seat. That includes SSH logins, and most
headless machines and containers. In those cases, make your user the owner of the node instead:

```sh
sudo tee /etc/udev/rules.d/70-airpods-hid-owner.rules >/dev/null <<EOF
SUBSYSTEM=="hidraw", ATTRS{idVendor}=="cafe", ATTRS{idProduct}=="4015", OWNER="$(id -un)", MODE="0600"
SUBSYSTEM=="hidraw", ATTRS{idVendor}=="cafe", ATTRS{idProduct}=="4005", OWNER="$(id -un)", MODE="0600"
EOF
sudo udevadm control --reload
sudo udevadm trigger --subsystem-match=hidraw --action=add
```

Run it as the user who will use the dongle, then replug. This gives that one user access, in any
session.

To check the result:

```sh
ls -l /dev/hidraw*
getfacl /dev/hidraw3
```

Use the node number from your error message. With the owner rule, your user name appears in the
`ls` output. With the standard rule, `getfacl` shows your user while you are at the local seat. A
container also needs the node passed in.

## AirPods disconnected

`airpods status` shows `AirPods: disconnected`, or a command fails with one of these:

```text
Error: RPC failure: AirPods disconnected
Error: Control failure: AirPods disconnected
```

The dongle is working, but it has no link to the AirPods. Batteries in `status` can still show old
values.

Since firmware 0.1.58 the dongle does not reconnect by itself, at startup or after a lost link, under the default
`manual` [reconnect policy](CLI.md#reconnect-policy). It waits for the AirPods to connect, for a press of its button, or
for `airpods connect` or `airpods reconnect`. Under `auto` it also retries by itself. After `airpods disconnect` it
declines the AirPods' own connections and does not retry until one of those requests, or until it restarts. To fix it:

1. Take the AirPods out of the case and keep them close to the dongle.
2. Disconnect them from any phone or computer that may have taken them over.
3. Run `airpods connect`. It waits until the dongle owns the AirPods' audio. On firmware before 0.1.59, wait a few
   seconds, then run `airpods status`.
4. If they never connect, the pairing may be lost. `airpods pair` pairs them again, but it erases
   the stored pairing. Use it last. See [pair](CLI.md#pair).

## Settings not ready

`airpods status` shows `Settings: not ready`, or `set` fails with this:

```text
Error: AirPods settings connection is not ready
```

The dongle's settings connection to the AirPods is not up. Nothing was sent. This can happen right
after connecting, or when the AirPods are disconnected. To fix it:

1. Wait a few seconds and run `airpods status` again.
2. If it stays `not ready` while the AirPods are connected, put them in the case for a moment and
   take them out again.

`reconnect` may do nothing here, because the dongle ignores it while the AirPods are connected.

`settings` and `get` fail this way while the controls are not ready:

```text
Error: AirPods controls are not ready yet; read the settings again in a moment
```

Nothing was sent. Run the command again after a few seconds. Right after connecting, the AirPods also
report their settings one by one, so a first `settings` can show more `unknown` lines than a later one.

## unknown lines

```text
microphone: unknown (not reported; may be unsupported)
```

The AirPods have not reported this setting to the dongle. This is not an error, and it does not mean
the setting is off. `get` exits with `1` and `settings` exits with `0`.

- Your model may not have the setting. See [Settings](CLI.md#settings-1).
- The report may not have arrived. Check that `airpods status` shows `Settings: ready`, wait, and try
  again. The tool cannot ask for a report.

```text
mode: unknown device value (0x0) (report age 1 ms)
```

The AirPods reported a value this tool has no word for. Exit code `0`. Please report it with your
AirPods model.

```text
Battery case: unknown
```

The dongle has no reading for this part. This is common for the case. `Codec: unknown` likewise
means the dongle reported no codec this tool knows.

## Busy: close other apps

```text
Error: RPC failure: busy; close competing companion controls
```

Another program is using the dongle's control channel. Clients share this one channel, and this tool
does not cancel another client's work. To fix it, close these and run the command again:

- NTPods
- The tray app (`airpods-tray`). Use Quit in its
  menu, because closing the window may leave it running in the tray
- Another `airpods` or `airpods diag` that is running, including a capture in another window

Serial diagnostics can stay open, because they use a different interface.

From `set`, this message with exit `1` means nothing was applied. If it appears inside a
`change not confirmed` message with exit `3`, see [After exit 3](CLI.md#after-exit-3).

```text
Error: Control failure: busy; another companion request or settings session is running, or USB audio, the microphone or a phone is in use
```

The dongle refused a control command. Nothing changed. It does this while another app has a request
running or holds the settings session, so close the apps above first. For `pair`, which is also
refused this way before anything is sent, stop playback to the dongle, close apps using its
microphone, and disconnect any phone from it. On firmware 0.1.59 or newer, `reconnect` is refused
this way while the microphone is in use or another takeover is pending.

`disconnect` has its own message for this. See [Disconnect](#disconnect).

## Microphone in use

```text
Error: Stop microphone use before changing AirPods settings
```

An app is using the dongle's microphone, and settings cannot be changed during that time. Nothing
was sent. `get`, `settings` and `status` still work.

Close the call or recording app, such as Teams, Zoom or Discord. To see what is using the microphone:

- Windows: Settings > Privacy & security > Microphone shows recent use.
- Linux with PipeWire or PulseAudio: run `pactl list short source-outputs`.

## Dongle dropping off USB

The dongle disappears during use, or commands fail with operating system errors such as these:

```text
Error: [Errno 19] No such device
Error: [Errno 5] Input/output error
Error: [Errno 110] Connection timed out
Error: [WinError 1167] The device is not connected.
Error: USB feature transfer exceeded 400 ms
Error: Incomplete USB feature report
```

Exit code `1`. If the failure came once a write had started to go out, or right after one, the message is
`change not confirmed` or `not confirmed` with exit `3`, because the write may have applied. A failure
before anything was sent stays exit `1`.
The last two messages above are Windows only.

To fix it:

1. Replug the dongle, then run `airpods status`.
2. Plug it directly into the computer, not through a hub, and use a short data cable. A Pico can
   reset if the port power is poor.
3. See what the system logged. On Linux, run `journalctl -k -g usb` or `sudo dmesg | tail -n 30`.
   On Windows, run the `Get-PnpDevice` check from [Dongle not found](#dongle-not-found), and look at
   the `Status` column.
4. If it keeps happening, the firmware may be restarting. Send a capture. See
   [Send a capture for debugging](CLI.md#send-a-capture-for-debugging).

After a replug the `/dev/hidrawN` number may change, which does no harm. Any `airpods diag` capture
that was running has ended, so start a new one with a new file name.

## PID 4005 and 4015

The dongle appears with USB vendor ID `CAFE` and one of two product IDs. The tools accept both.

| Product ID | Likely meaning |
|---|---|
| `4015` | Audio mode: USB audio plus control |
| `4005` | Control only: no USB audio |

**This mapping is inferred, not confirmed.** It comes from names in this tool's code. If you see
`4005` and expect sound through the dongle, replug it and check the firmware. The udev rule covers
both IDs, and `airpods` treats them the same, so a command that fails on one may be a firmware
limit.

To see which one you have, run `lsusb -d cafe:` on Linux. On Windows, look at the `InstanceId` from
the `Get-PnpDevice` command in [Dongle not found](#dongle-not-found).

## All other messages

### Command line errors

These come with a usage text and exit code `2`. The dongle is not opened, so these also appear
without a dongle.

| Message | Cause | Fix |
|---|---|---|
| `the following arguments are required: command` | No command given | Run `airpods help` |
| `the following arguments are required: setting`, or `setting, value`, or `value` | `get` or `set` is missing something | Run `airpods help set` |
| `argument command: invalid choice: 'x' (choose from ...)` | Unknown command | Use one from the list |
| `argument setting: invalid choice: 'x' (choose from ...)` | Unknown setting | Use one from the list |
| `unknown help topic: x` | `help` was given something that is not a command or setting | Run `airpods help` |
| `unrecognized arguments: x` | Extra words, an unknown flag, or a flag that was shortened: flags must be written in full. An unknown flag is reported before any later mistake on the line, as in `airpods --seria X status` | Remove the extra words, or write the flag in full |
| `--serial must come before the command`, or the same naming `--json`, `--debug` or `--version` | The flag the message names came after the command | Write it first, as in `airpods --serial X status` or `airpods --json status` |
| `argument --serial: expected one argument` | `--serial` has no value: nothing follows it, or another flag does, as in `airpods --serial --json status` | Give the serial |
| `argument --balance: expected one argument (use --balance=VALUE if the value starts with -)` | The next word starts with `-` and is neither a plain decimal nor a flag of the command, as in `--balance -1e-1`, so it reads as a flag. This goes for every flag that takes a value | Write `--balance=-1e-1`, or give the missing value |
| `argument -h/--help: ignored explicit argument 'x'` | A value was given to a flag that takes none | Remove the value |
| `mode: expected off, anc, transparency, adaptive` | Invalid value for a named setting. Other settings name their own values | Use a listed value, with exact case |
| `tone-volume: expected integer 0..100` | Not a whole number from 0 to 100. This also covers `case-volume` and `adaptive-strength`. Signs such as `+5` are refused | Use digits only |
| `hold-modes: expected at least two distinct modes, comma-separated: off, anc, transparency, adaptive` | Fewer than two modes, a repeat, a space or an unknown word | Use a value such as `anc,transparency` |
| `hold-actions: expected left,right actions, comma-separated: noise-control or siri for each ear` | Not exactly two valid actions | Use a value such as `noise-control,siri` |

These are specific to `list`, `--json`, `audio set`, `game`, `nearby` and `completion`:

| Message | Cause | Fix |
|---|---|---|
| `--json works only with status, settings, get, list, info and the read commands of hearing, hardware and experimental` | `--json` was given to another command, or to `help` | Remove it |
| `--serials cannot be combined with --json` | Both were given to `list` | Use `airpods --json list` and read `serial` |
| `give at least one of --codec, --bitrate and --variable` | `audio set` had nothing to change | Run `airpods audio set -h` |
| `--bitrate: expected default or a bitrate of aac (...) or aac-eld (...)` | No codec has that bitrate | Use a listed value |
| `--bitrate: aac expects default, 96000, ...` | That codec does not have that bitrate. Exit `1` with `Error:` if the codec is the one already requested | See [audio set](CLI.md#audio-set) |
| `--bitrate needs codec aac or aac-eld, not sbc` | `auto` and `sbc` have no bitrate choice. Exit `1` with `Error:` if the codec is the one already requested | Add `--codec aac` or `--codec aac-eld` |
| `--variable on needs codec aac-eld, not aac` | Only AAC-ELD has variable bitrate. Exit `1` with `Error:` if the codec is the one already requested | Add `--codec aac-eld` |
| `Use a name of 1 to 64 UTF-8 bytes with no control characters.` | `experimental rename` was given a name that cannot be sent | Use a shorter name, or one that is not blank |
| `--seconds must be 1..300` | `nearby` is limited to 5 minutes | Use a shorter time and run it again |
| `argument state: invalid choice: 'x' (choose from on, off)` | `game` needs `on` or `off` | Use one of them |
| `argument shell: invalid choice: 'x' (choose from bash, zsh, fish, powershell)` | No completion for that shell | Use one from the list |

### airpods diag command line errors

Also with usage text and exit code `2`.

| Message | Cause | Fix |
|---|---|---|
| `--seconds must be 1..1800` | Length out of range | Use `1` to `1800` |
| `argument --seconds: invalid int value: 'x'` | Not a whole number | Use a whole number |
| `argument --seconds: expected one argument` | No value given. `--output` and `--serial` have the same message | Give a value |
| `argument --legacy: ignored explicit argument '1'` | A value was given to an on or off flag | Remove the value |
| `--legacy cannot be combined with --inspect or --phone` | Flags conflict | Drop one |
| `--radio is a separate lightweight capture` | `--radio` was combined with another capture flag | Run it as a separate capture |
| `--replace needs --output` | There is no file to replace | Add `--output` |

### Results from set

With `mode` as the example setting. Refusals with exit `1` were not applied, unless the message says
otherwise.

| Message | Exit | Cause | Fix |
|---|---|---|---|
| `mode: sent, but no fresh matching device report. It may have applied; use get mode before retrying.` | `3` | No matching report within about 5 seconds | [After exit 3](CLI.md#after-exit-3) |
| `mode: change not confirmed: REASON. It may have applied; read it again before retrying.` | `3` | The write or the read after it failed. `REASON` is one of the other messages on this page. If the failure came before anything was sent, it is plain `Error: REASON` with exit `1` instead, and nothing changed | [After exit 3](CLI.md#after-exit-3), then look up `REASON` |
| `mode: change not confirmed: RPC failure: REASON. Read it again before retrying.` | `3` | The dongle returned a failure that does not rule out a change | [After exit 3](CLI.md#after-exit-3) |
| `mode: change not confirmed: Unexpected setting-write response. It may have applied; read it again before retrying.` | `3` | The dongle answered a write in an unexpected way. Firmware may not match | [After exit 3](CLI.md#after-exit-3), and check the firmware |
| `Error: RPC failure: malformed request` | `1` | The dongle did not accept the request. Firmware may not match | Update the tool or check the firmware |
| `Error: Firmware does not support this request for this device` | `1` | The firmware or the AirPods model does not support the setting. `airpods diag --inspect` can also get this. For `mode off` or `hold-modes` with `off`, Allow Off may be missing | Check [Settings](CLI.md#settings-1). For Off, run `airpods set allow-off on` first. Without `--inspect`, the capture may work |
| `Error: AirPods controls are not ready yet; read the settings again in a moment` | `1` | The AirPods have only just connected. Nothing was sent | Wait a few seconds, then run it again |
| `allow-off: on (sent first; the dongle forgets it whenever the AirPods connect again)` | `0` | Not an error. The dongle refused Off for want of Allow Off, so the tool sent it, then Off once more. See [allow-off](CLI.md#allow-off) | None needed |
| `Error: Firmware does not advertise typed settings support` | `1` | The firmware has no settings interface, so `settings`, `get` and `set` cannot work | Use compatible firmware |

### Results from audio, game, nearby and perf

| Message | Exit | Cause | Fix |
|---|---|---|---|
| `audio set: not confirmed: REASON. It may have applied; run airpods audio before retrying.` | `3` | A profile change can restart the link before the reply arrives, so the outcome is unknown. `game` has the same message. If the failure came before anything was sent, it is plain `Error: REASON` with exit `1` instead, and nothing changed | Run `airpods audio`, and try again only if needed |
| `Error: RPC failure: busy; close competing companion controls` | `1` | Another app is using the control channel. Nothing changed | See [Busy](#busy-close-other-apps) |
| `Error: Unsupported dongle audio profile.` | `1` | The dongle answered in a form this tool does not know. Firmware may not match | Check the firmware |
| `Error: Unsupported dongle performance snapshot.`, `Error: Inconsistent dongle performance counters.` | `1` | The same, for `perf` | Check the firmware |
| `Error: Game mode is not supported by this dongle` | `1` | The dongle does not offer it. Nothing was sent | None needed |
| `Error: game: the dongle reports failed`, or `unsupported` | `1` | The dongle took the request and could not do it | Run `airpods audio` to see the state |
| `Error: Searching is not available for this pair of AirPods yet` | `1` | The dongle has no verified identity for these AirPods, so it cannot recognise them | Connect the AirPods, then try again |
| `Error: Another app is already searching; stop the search there first` | `1` | A search is running that this command did not start. It is left alone | Stop it in that app. If no app is running, replug the dongle |
| `Error: The finder did not start` | `1` | The dongle accepted the request, but no search began within 5 seconds. A stop was sent | Try again, and replug if it repeats |
| `Error: Unsupported dongle nearby-finder snapshot.`, `Error: Invalid dongle signal strength.`, `Error: Unverified dongle signal reading.` | `1` | The dongle's reply was not usable. A stop was sent | Check the firmware |
| `nearby: could not stop the finder: REASON. Replug the dongle if it keeps searching.` | `1` | The stop request failed, for example because the dongle was unplugged | Replug the dongle |

### Hearing, hardware and experimental commands

These have only run against a simulator. Messages that start with `Error:` exit `1`, and nothing was sent.

| Message | Exit | Cause | Fix |
|---|---|---|---|
| `Error: Connect the dongle's AirPods before reading or changing hearing data.` | `1` | Not connected, or not ready | See [AirPods disconnected](#airpods-disconnected) |
| `Error: Connect the dongle's AirPods first.` | `1` | The same, from `info` or `hardware` | The same |
| `Error: Update the dongle firmware before reading or changing hearing data.` | `1` | The firmware has no typed requests | Update it |
| `Error: Update the dongle firmware before reading AirPods information.` | `1` | The same, from `info` | Update it |
| `Error: Update the dongle firmware first; it cannot serve these reads.` | `1` | The same, from `hardware` | Update it |
| `Error: Stop recording and wait for the microphone to recover before reading or changing hearing data.` | `1` | The microphone is in use | See [Microphone in use](#microphone-in-use) |
| `Error: Stop recording first; these reads wait until the microphone is free.` | `1` | The same, from `hardware` | The same |
| `Error: Hearing assistance is not supported by the model these AirPods report.` | `1` | Not a Pro 2 or Pro 3, or the model is unknown | Check `airpods info --capabilities` |
| `Error: Not available on these AirPods.` | `1` | `hardware` on another model | The same |
| `Error: Hearing assistance has not been set up on these AirPods. Set it up from an iPhone or iPad.` | `1` | Not set up, or set up state not reported | Set it up, then reconnect. Check `airpods hearing status` |
| `Error: Wait for the AirPods to report the other hearing mode before changing this one.` | `1` | A report is missing | Reconnect the AirPods, then read again |
| `Error: Wait for the AirPods to report Hearing Aid before editing.` | `1` | The same, for a profile | The same |
| `Error: The AirPods' hearing modes disagree. Reconnect the AirPods before changing them.` | `1` | The reports contradict each other | Reconnect |
| `Error: The hearing modes moved while the request was being checked. Read them again.` | `1` | They changed during the checks | Read again |
| `Error: The hearing modes changed before the profile write. Read the settings again.` | `1` | They changed between the checks and the profile write. Nothing was written | Read again |
| `The profile was written, but the hearing modes changed. Read the settings again.` | `3` | The modes moved around the write | `airpods hearing status`, then `airpods hearing profile show` |
| `Error: Custom Transparency is on. Turn it off before enabling hearing assistance.` | `1` | The two exclude each other | `airpods experimental --experimental transparency set --enabled off` |
| `Error: Could not confirm Custom Transparency is off, so hearing assistance stays as it is.` | `1` | The AirPods do not expose it | Nothing to fix |
| `Error: These AirPods have no writable saved hearing profile, so hearing assistance stays as it is. Set it up from an iPhone or iPad.` | `1` | Turning on needs a hearing profile the AirPods can use | Set it up from an iPhone or iPad |
| `Error: Nothing was sent: turning Hearing Aid on needs --yes, or a terminal to confirm in.` | `1` | Nobody could be asked: the input is not a terminal, or stderr, where the question goes, is sent elsewhere as with `2>/dev/null`. Media Assist, `pair` and `hearing profile apply` say the same | Read the warning, then add `--yes` |
| `Error: Nothing was sent: not confirmed.` | `1` | You said no | Nothing to fix |
| `Error: Nothing was sent: confirm with --buds-in-case that both AirPods are in the open case, or run this in a terminal.` | `1` | `hardware check` could not ask | Add `--buds-in-case` when they are |
| The reason, then `It may have applied. Read it again with airpods hearing status.` | `3` | Sent, but not confirmed | Read it again. Do not simply repeat it |
| `Hearing Aid may still be active. Turn it off to be sure. Run: airpods hearing recover --off hearing-aid` | `3` | An unconfirmed turn on | Do what it says. See [Hearing recovery](CLI.md#hearing-recovery) |
| `hearing-aid: already reported off; no write sent` | `0` | A later run read off, perhaps from a report older than your change | If it sounds on: AirPods out, `airpods reconnect`, `airpods hearing status` |
| `Error: The hearing profile changed on the AirPods. Read it again before editing.` | `1` | Something else changed it | Run it again |
| `Error: This hearing profile can be read but not edited.` | `1` | A format this tool cannot write | Nothing to fix |
| `Error: Nothing was sent: amplification A with balance B puts the ears' gains at left L, right R; each must be -1.5 to 1.5.` | `1` | `hearing profile apply` with one of the two values, which together with the stored other one puts an ear out of range | Give both values, or a smaller one |
| `Error: The in-case check is only verified on AirPods Pro 3 (A3063, A3064 or A3065) with firmware 9A348.` | `1` | Another model or firmware | Nothing to fix |
| `Error: Both AirPods must be in the open case, not in your ears, and the case report must be recent.` | `1` | Not in the case, or the report is over 30 s old | Put them in the open case, wait a moment, and try again |
| `Error: The in-case check needs the audio idle. Pause playback and recording and disconnect phone audio first.` | `1` | Audio or a phone is in use | Stop it |
| `Error: No new results in 2 minutes. ...` | `1` | No new results in time. The case may not have closed | `airpods hardware status` later |
| `Stopped waiting. The AirPods may still finish the check; ...` | `130` | Ctrl-C | `airpods hardware status` later |
| `... Do not start another check yet; read the results with airpods hardware status.` | `3` | No reply to the request | Do what it says |
| `Error: These AirPods have not reported their EQ on this connection, so it cannot be read or changed.` | `1` | The dongle holds no EQ report from them: they have not sent one since they connected, they have no EQ, or the dongle firmware is older than 0.1.64 | Reconnect the AirPods and try again. Older firmware cannot do this |
| `Error: The AirPods reported an EQ in a format this app does not know.` | `1` | An unexpected report | Nothing to fix |
| `Error: The EQ changed on the AirPods. Read it again before changing it.` | `1` | A new EQ report came between the read and the send. Nothing was sent | Run it again |
| `Error: The dongle did not pass the EQ change on: it has no current EQ report from these AirPods, the report changed, or it does not know their model. Nothing was sent. Read the EQ again.` | `1` | The dongle declined it before sending: a refusal, not a failure. It passes EQ on only while it knows the AirPods' model and holds a current, unchanged EQ report | `airpods experimental --experimental eq show`, then try once more. If it repeats, reconnect the AirPods |
| `The EQ change was sent, but the AirPods did not report it. Read the EQ again before changing it.` | `3` | No new EQ report within 8 seconds. The AirPods may have ignored it | `eq show`. Do not simply repeat it |
| `A newer EQ report from the AirPods differs from the EQ sent. Read it again before changing it.` | `3` | Something else changed the EQ, or the AirPods applied other values | `eq show` |
| `Could not confirm EQ was sent. It may or may not have arrived.` | `3` | The send failed partway. A rename says the same about the name | `eq show` before you try again |
| `Error: Loud Sound Reduction is not available on these AirPods.` | `1` | The lookup did not find it on the AirPods | Nothing to fix |
| `Error: Loud Sound Reduction is not supported by the model these AirPods report.` | `1` | Not a Pro 2 or Pro 3 | Nothing to fix |
| `Error: The dongle refused to pass the name on to the AirPods: its firmware only sends a fixed list of AirPods requests. Nothing was sent.` | `1` | Dongle firmware up to at least 0.1.66 does not pass a rename on | None. Nothing reached the AirPods |

Command line errors from these commands exit `2`, without opening the dongle:

| Message | Cause | Fix |
|---|---|---|
| `these commands need --experimental: they may not work through the dongle` | `airpods experimental` without its flag | Add `--experimental`, if you accept that |
| `nothing to apply: give --file, or at least one of ...` | `hearing profile apply` had nothing to do | Give a value |
| `nothing to set: give at least one of ...` | `transparency set` had nothing to do | Give a value |
| `the left ear needs 8 values, for 250, 500, ... Hz` | Not 8 values | Give all 8 |
| `every left ear value must be 0 to 90 dB HL` | A value out of range | Fix it |
| `amplification must be -1.5 to 1.5`, `balance must be -3 to 3`, and the like | A value out of range | Fix it |
| `amplification A with balance B puts the ears' gains at left L, right R; each must be -1.5 to 1.5` | Together they put an ear out of range | Change one of them |
| `--low must be 0 to 100`, and the like | An EQ band out of range | Fix it |
| `FILE: json: unknown field "x"` | A name the file does not take | See [hearing profile apply](CLI.md#hearing-profile-apply) |

### Control commands

These are the results from `reconnect`, `pair` and `pair-phone`, and from `connect` on dongle firmware before 0.1.59.
`reconnect` is the example.

| Message | Exit | Cause | Fix |
|---|---|---|---|
| `reconnect: sent, but no receipt from the dongle. It may have applied; check status before retrying.` | `3` | No receipt within 2 seconds | Run `airpods status` before you try again |
| `reconnect: not confirmed: REASON. It may have applied; check status before retrying.` | `3` | Sending failed or the dongle stopped answering | Run `airpods status`, and look up `REASON` |
| `Error: Control failure: not supported by this dongle` | `1` | The firmware does not offer this command | Use compatible firmware |
| `Error: Control failure: stale session` | `1` | The connection changed while the command was sent | Run the command again |
| `Error: Control failure: malformed` | `1` | The dongle did not accept the request. Firmware may not match | Check the firmware |
| `Error: Control failure: busy; another companion request or settings session is running, or USB audio, the microphone or a phone is in use` | `1` | `pair` or `pair-phone` before anything was sent: USB audio is playing or the microphone is in use, and for `pair` a phone is connected | Stop the audio, wait for the microphone, disconnect the phone, and try again |
| `pair: the dongle answered busy, but it may already have disconnected the AirPods and erased their pairing; check status before retrying.` | `3` | The dongle answers busy for a pair it could not finish, for example while the AirPods microphone is about to start, and it may have dropped the AirPods and their pairing first | `airpods status`. If they are still connected, nothing changed: stop audio and try again. If not, pair them again |

### connect

On dongle firmware without the routing handover, `connect` gives the results of [Control commands](#control-commands).

| Message | Exit | Cause | Fix |
|---|---|---|---|
| `connect: takeover accepted, still in progress. Use airpods status to follow it.` | `0` | Not an error. The AirPods had not confirmed the dongle within 5 seconds | Run `airpods status` |
| `Error: the dongle cannot take the AirPods back right now: ...` | `1` | The microphone or a call is in use, a takeover, audio change or Game Mode change is in flight, or another app holds the settings session. Nothing changed | End the call or recording, close the tray app and other clients, and try again |
| `Error: the dongle has no saved AirPods pair to takeover` | `1` | No AirPods are paired with the dongle | Pair them: [pair](CLI.md#pair) |
| `Error: the takeover ended without the AirPods confirming the dongle; they may be in their case, out of range or in use elsewhere` | `1` | The dongle tried and gave up | Take them out, bring them close, disconnect them from other devices, and try again |
| `Error: routing intent changed; ...` | `1` | As for [Disconnect](#disconnect) | Run `airpods status`, then `connect` again if they are not connected |
| `takeover: not confirmed: REASON. It may have applied; read status before trying again.` | `3` | No answer to the request, and no sign of it in the dongle's state | Run `airpods status` before you try again |

### reconnect-policy

| Message | Exit | Cause | Fix |
|---|---|---|---|
| `Error: Reconnect policy unavailable: the dongle firmware is older than 0.1.63, or its saved configuration is unreadable` | `1` | Older firmware, or the dongle cannot read its saved policy and acts as `manual`. After a save that exited `3`, it reads like this until the dongle restarts. With `; nothing was saved` at the end, a save was refused | Update the firmware. Otherwise restart the dongle and read again |
| `Error: The dongle saves the reconnect policy only while idle: ...` | `1` | Busy: playback, the microphone or a call, a connection or Game Mode change under way, or a settings session open in any app. Nothing was saved | Pause playback, end the call, close the tray app and other clients, wait for the connection to settle, and try again |
| `reconnect policy: the dongle could not verify the save, so it may have been saved. Do not retry it automatically; ...` | `3` | The dongle wrote the policy but could not read it back. Not confirmed | Do not repeat it at once. Restart the dongle, run `airpods reconnect-policy`, then decide |
| `reconnect policy: not confirmed: REASON. It may have been saved; read it again before retrying.` | `3` | No answer, or an answer with another policy | Run `airpods reconnect-policy` before you try again |
| `Error: Firmware does not advertise typed settings support` | `1` | The firmware has no typed requests at all | Update the firmware |
| `Warning: under the auto policy the dongle connects to the AirPods by itself ...` | `0` | Not an error: `auto` is saved | `airpods reconnect-policy manual` if you did not mean it |

### Disconnect

| Message | Exit | Cause | Fix |
|---|---|---|---|
| `Error: this dongle firmware has no command to disconnect AirPods while keeping them paired; update it to 0.1.59 or newer, or put the AirPods in their case to disconnect` | `1` | Firmware before 0.1.59 has no such command. Nothing was sent | Update the firmware, or put the AirPods in their case. See [disconnect](CLI.md#disconnect) |
| `Error: the dongle cannot release the AirPods right now: ...` | `1` | The microphone or a call is in use, a takeover, audio change or Game Mode change is in flight, or another app holds the settings session. Nothing changed | End the call or recording, close the tray app and other clients, and try again |
| `Error: the dongle has no saved AirPods pair to release` | `1` | No AirPods are paired with the dongle | Nothing to disconnect |
| `Error: routing intent changed; read status again. The request was not retried (now ...). ...` | `1` | A button press, a reconnect, another app or an automatic reconnect under the `auto` policy changed the connection first. Nothing was sent again | Run `airpods status`, then `disconnect` again |
| `Error: routing intent changed again during one fresh request; read status again (now ...). ...` | `1` | The release was stale, so one fresh release went out, and that was overtaken as well. Under `auto` the dongle may keep retrying | Run `airpods status`, then `disconnect` again. If it keeps happening, `airpods reconnect-policy manual` first |
| `release: not confirmed: REASON. It may have applied; read status before trying again.` | `3` | No answer to the release, and no sign of it in the dongle's state | Run `airpods status` before you try again |

### Connection and protocol failures

All have exit code `1` when shown as `Error: ...`, which means nothing was sent. Once a write has gone out they can
appear inside `change not confirmed` with exit `3`.

| Message | Cause | Fix |
|---|---|---|
| `Pico session changed; run the command again` | The dongle's connection changed since the command began, for example because the AirPods reconnected | Run the command again. You do not need `airpods reconnect`, because each command starts a new session |
| `Pico session changed during request`, `Pico session changed during read` | As above, during the exchange | Run the command again |
| `RPC failure: stale session` | The dongle considered the request out of date | Run the command again |
| `RPC failure: device timeout` | The AirPods did not answer in time | Check `airpods status`, keep them close, try again |
| `RPC failure: transport failure` | The dongle could not reach the AirPods | Check `airpods status`, keep them close, try again |
| `RPC failure: cancelled` | The request was cancelled on the dongle | Close other clients and try again |
| `RPC failure: N` | The dongle returned failure code `N`, which this tool does not know | Update the tool, and report it if it continues |
| `Pico diagnostic read timed out` | The dongle did not answer within 4 seconds. Despite the name, this also applies to settings | Close other clients, try again, then replug |
| `Unsupported HID report identity/version` | The device has a matching ID but speaks another protocol version | Use compatible firmware, or update the tool |
| `Malformed settings snapshot`, `Malformed settings record` | The dongle's answer did not fit the expected format | Replug and try again. If it continues, check that firmware and tool versions match |
| `Oversized HID page`, `Invalid diagnostic length`, `Invalid diagnostic pagination`, `Diagnostic page exceeds declared length` | Same | Same |
| `[Errno N] TEXT` | Linux error from the device. See [Dongle dropping off USB](#dongle-dropping-off-usb) | Replug and try again |
| `[WinError N] TEXT` | Windows error while finding or using the device. `[WinError 5] Access is denied.` means another program or a policy is blocking access | Close other clients, replug, try again |
| `Invalid HID interface detail size` | Windows returned unexpected data while listing devices | Replug and try again. If it continues, please report it |
| `HID transport is only implemented for Windows and Linux` | A build for another operating system | Use Windows or Linux |

### airpods diag failures

| Message | Exit | Cause | Fix |
|---|---|---|---|
| `Error: open capture.jsonl: file exists` | `1` | `--output` never overwrites. A capture that fails after the dongle opens also leaves its file, which may be empty. If the dongle cannot be opened, no file is created. The wording can differ on Windows | Use a new name, or delete the old file |
| `Error: open ...` with other text, or `Error: write ...` | `1` | The file could not be created or written, for example a missing folder or a full disk | Fix the path or free up space |
| `Inspection stop failed; firmware expires it after 120 seconds: REASON` | Unchanged | A warning. Turning off `--inspect` failed | Nothing is needed. Inspection ends by itself |
| `Error: Unsupported audio/performance layout`, `Unsupported audio trace layout`, `Unsupported audio inspection layout`, `Unsupported phone diagnostic layout`, `Unsupported radio/adaptation layout` | `1` | The firmware's data has a format this tool does not know | Use matching firmware and tool versions. Leaving out the flag concerned may work |
| `Error: Invalid inspected AAC-ELD length`, `Invalid phone output state` | `1` | The firmware reported values out of range | Try again. Report it if it continues |

The capture also fails with the messages in the sections above, such as `busy` or
`AirPods disconnected`.

### Messages you should not see

The tool has these strings in shared code, but no path in `airpods` or `airpods diag` shows
them today. If you do see one, please report it.

| Message | Where it belongs |
|---|---|
| `Typed requests must fit one 1..42 byte HID fragment` | Internal size check |
| `Only diagnostic reads and explicit audio profiling controls are allowed` | Internal safety check |
| `Collector permits audio/performance/trace reads only` | Internal safety check |
| `unknown setting: x` | Internal check. The command line refuses unknown settings first |
| `Unsupported dongle audio profile.`, `Unsupported dongle performance snapshot.`, `Inconsistent dongle performance counters.` | Tray features |
| `Unsupported dongle nearby-finder snapshot.`, `Invalid dongle signal strength.`, `Unverified dongle signal reading.`, `Unsupported finder response.` | Tray features |
| `Unsupported dongle settings lease.`, `Dongle settings lease is unavailable.`, `Unsupported dongle settings lease renewal.` | Tray features |

## Shell completion does nothing

- Check that the script loads. Run `airpods completion bash`, or your shell's name, and look for errors.
- bash needs the `bash-completion` package for the packaged script to load by itself.
- zsh needs `compinit` in your `.zshrc`, and picks up new files only in a new shell. If an old copy
  is cached, delete `~/.zcompdump`.
- No serials offered after `--serial`: run `airpods list --serials`. A serial is left out if it has
  characters other than letters, digits, `-`, `_` and `.`. Type it by hand, quoted.
- In PowerShell, completion works only for `airpods` and `airpods.exe`. If you kept the long file name,
  rename it.

## Still stuck

Open an issue with the command, its full output and exit code, your OS, the output of `airpods --version`,
and your AirPods model. For audio problems, add a capture. See
[Send a capture for debugging](CLI.md#send-a-capture-for-debugging).
