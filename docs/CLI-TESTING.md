# Testing the command line tool

[BUILD.md](BUILD.md) builds and tests everything in this repository, the `airpods` command included. This page has
what is particular to the command: its goldens and its hardware checklist. The app's checklist is in
[BUILD.md](BUILD.md#hardware-checklist).

## Goldens

The command and diagnostics goldens in `cmd/airpods/testdata/` hold the expected output, exit codes and HID traffic.
`diag_cli_golden.json` is the one for `airpods diag`.
A change to them should be a deliberate change in behaviour.

`cmd/airpods/testdata/parity_golden.json` holds `info`, `status --details`, `hearing`, `hardware` and `experimental`, on the
scenarios in `cmd/airpods/parity_test.go`. Its inputs are the name, scenario, arguments and what a person would answer. To
add a case, add them and rewrite the expected values as below, with `-run TestParityGolden`, then read the diff.

`cmd/airpods/testdata/features_golden.json` holds the commands added after the first release. To add
a case, add its name, arguments and simulator settings, then rewrite the expected values and read the
diff:

    go test ./cmd/airpods -run TestFeatureGolden -update

## Trying the commands without a dongle

`AIRPODS_CLI_SIM=1 airpods hearing status` uses a simulated dongle. See
[Try it without a dongle](CLI.md#try-it-without-a-dongle).

## Hardware checklist

The tests use a protocol simulator. On a real dongle, `airpods status` and reading settings have
run on Linux. Writes are being tested. Nothing has been run on Windows yet. Run this list on a
Windows 11 PC and on a Linux box with the dongle attached.

General:

- [ ] `airpods status`
- [ ] `airpods settings`
- [ ] `airpods get mode`
- [ ] `airpods set mode transparency`: confirmed by the AirPods
- [ ] The same `set` again: already reported, no write sent
- [ ] `set` during microphone use: refused, exit 1
- [ ] `airpods --serial YOUR_PICO_USB_SERIAL status`, and a wrong serial
- [ ] `airpods list` with no dongle, one dongle and two dongles, and with one the user cannot open
- [ ] `airpods --json status`, `--json settings`, `--json get mode`, `--json list`
- [ ] `airpods audio`
- [ ] `airpods audio set --bitrate 128000`, then `airpods audio`: the link may restart, then the bitrate
      is in use. Restore it afterwards
- [ ] `airpods game on` and `airpods game off` on a dongle that offers game mode, then `airpods audio`
- [ ] `airpods nearby --seconds 10` with the AirPods out of their case, near and far: readings change
- [ ] Ctrl-C during `airpods nearby`: `nearby: stopped`, exit `130`, and a following `airpods nearby`
      can start
- [ ] `airpods perf`
- [ ] `airpods get connect-automatically` and `airpods get allow-auto-connect`, then `set` for each:
      note whether the AirPods confirm it or `set` exits `3`
- [ ] `airpods diag --seconds 30 --inspect --phone`, and `airpods diag --seconds 30 --radio`
- [ ] `airpods disconnect` on firmware before 0.1.59: the error from [CLI.md](CLI.md), exit 1, and the
      AirPods stay connected. The check for 0.1.59 and newer is in [CLI-DISCONNECT.md](CLI-DISCONNECT.md).
- [ ] `airpods connect` with the AirPods disconnected and out of the case: `audio ownership confirmed`, and how long
      it took. On firmware before 0.1.59: `requested; not yet confirmed`
- [ ] On firmware 0.1.63 or newer: `airpods reconnect-policy` reads `manual`. `reconnect-policy auto` while idle:
      `saved and verified` and the warning. The same during playback: refused, exit 1. Replug the dongle: the policy
      is kept. Set it back with `reconnect-policy manual`
- [ ] On firmware 0.1.62 or older: `airpods reconnect-policy` gives the `unavailable` error, exit 1
- [ ] On firmware 0.1.64 or newer: `airpods get gain-swipe` on AirPods Pro 2 or 3, and `get en352-protection` on
      Pro 3, then `set` each: confirmed or exit 3. On older firmware both read `unknown`, and `settings` still works
- [ ] `airpods game on` without playback: `now waiting for playback`; start playback, then `airpods audio` shows `on`
- [ ] `airpods pair-phone` during USB playback: refused before sending, exit 1
- [ ] On firmware 0.1.65 or newer: take a bud out during USB playback. The player pauses, and plays again when it goes
      back in. `airpods set ear-detection off` stops that
- [ ] On AirPods Pro 3, after taking them out of the case: `airpods set mode off` prints the `allow-off` line,
      then `confirmed by AirPods`

- [ ] `airpods info`, with `--show-serial` and `--capabilities`, and `airpods status --details`: the dongle
      capabilities include `routing` on firmware 0.1.59 or newer
- [ ] `airpods hearing status` and `airpods hearing profile show` on AirPods Pro 2 or Pro 3 set up with an iPhone, and the
      refusal on another model
- [ ] `airpods hearing aid on`, with the AirPods out of your ears first: the warning, the question, then the mode. Then
      `airpods hearing aid off`. Note the report ages
- [ ] `airpods hearing recover` with no change pending: reads only
- [ ] `airpods hearing profile apply --tone VALUE` with Hearing Aid off: it is written and Hearing Aid stays off. Restore it
      afterwards
- [ ] `airpods hearing profile apply --amplification VALUE`: the gains that `profile show` prints afterwards match the
      request, each ear within -1.5 to 1.5
- [ ] `airpods hardware status`, `airpods hardware data`
- [ ] `airpods hardware check` on a Pro 3 with firmware 9A348, and Ctrl-C during another one: exit `130`
- [ ] Each `airpods experimental --experimental` command: note whether it works through the dongle at all
- [ ] `airpods experimental --experimental eq show` on AirPods Pro 2 or 3 and dongle firmware 0.1.64 or newer: the
      values match the EQ an iPhone shows. Then `eq set --low VALUE`: `confirmed by the AirPods`, and the sound changes.
      Note whether `--enabled on` turns EQ on or off
- [ ] `airpods experimental --experimental loud-sound-reduction get` and `set` on AirPods Pro 2 or 3: found by lookup,
      and `written and read back`
- [ ] `airpods experimental --experimental transparency show` on AirPods whose amplification was set above 1 on an
      iPhone
- [ ] `airpods diag --seconds 10 --no-trace --phone`, and `--replace` over an existing file

Linux:

- [ ] The udev rule grants hidraw access without root, for both 4015 and 4005
- [ ] The same after installing the package instead of copying the rule
- [ ] Installing the package over an old airpods-cli package removes airpods-cli, and `airpods` still runs
- [ ] Close the terminal during `airpods nearby`, and `airpods nearby | head -n 1`: a following
      `airpods nearby` can start

Windows:

- [ ] `airpods list` shows the expected path and serial, and the reason for a dongle held by another app
- [ ] Close the console window during `airpods nearby`: a following `airpods nearby` can start
- [ ] `airpods completion powershell | Out-String | Invoke-Expression`, then press Tab after `airpods set `
- [ ] After the installer, with Add to PATH ticked: a terminal opened afterwards runs `airpods status` from any folder

Open questions only hardware can settle:

- [ ] `airpods set hold-actions siri,siri`, then hold a stem: the listening mode must not change.
      Then `airpods set hold-actions noise-control,siri`: noise control should be on the left stem
      and Siri on the right. The encoding follows the LibrePods protocol notes (1 is Siri, 5 is
      noise control, the right ear is the low byte) and has not been confirmed on hardware; if
      either result is the opposite, the encoding is wrong.
- [ ] Confirm control command receipts on a real dongle: `reconnect` and `pair-phone` should print
      `accepted by dongle`, not exit 3. Test `pair` last: it erases the stored pairing, so you will
      have to pair the AirPods again. Note whether `pair` ever answers busy after sending (exit 3), and whether the
      pairing was erased then.
