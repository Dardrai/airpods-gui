# Disconnecting AirPods from the dongle

## Outcome

`airpods disconnect` ends the AirPods' connection to the dongle and keeps them paired, on dongle
firmware 0.1.59 or newer. It uses the routing handover that the
[dongle firmware](https://github.com/Ripthulhu/airpods-dongle) describes in its
`docs/CONTROL-EXTENSIONS.md`. Older firmware has no such command: there `disconnect` sends nothing and
fails with exit `1`. For the output and exit codes, see [disconnect](CLI.md#disconnect).

`airpods connect` is the way back through the same handover, and waits until the dongle owns the AirPods' audio.
`airpods reconnect` is the plain reconnect request every firmware has: it also ends a release, but says only that the
dongle took the request. See [connect](CLI.md#connect) and [reconnect](CLI.md#reconnect).

## Which firmware

| Dongle firmware | `disconnect` | `connect` | Connecting again |
|---|---|---|---|
| Before 0.1.58 | Unsupported | The plain reconnect request, not confirmed | The dongle reconnects by itself after a lost link |
| 0.1.58 | Unsupported | The plain reconnect request, not confirmed | The dongle no longer connects by itself; the AirPods, a button press, `connect` or `reconnect` do |
| 0.1.59 to 0.1.62 | Releases the AirPods | Takes them back and confirms it | As 0.1.58. `reconnect` is a takeover with a time limit, as a button press is, and ends a release |
| 0.1.63 and newer | As 0.1.59 | As 0.1.59 | As 0.1.59 under the `manual` [reconnect policy](CLI.md#reconnect-policy), the default. Under `auto` the dongle also reconnects by itself while the AirPods are not connected and not released |

`airpods status --details` lists `routing` under `Dongle capabilities` when the firmware has the
handover. The tool decides by that capability alone, not by the version number. It follows firmware
0.1.66.

## How the handover works

1. The tool reads the status report. Without the `routing` capability it stops there, having sent
   nothing.
2. It reads the routing state. That holds the current intent revision, flags for owned, released and
   pending, and the number of the last accepted request. If the AirPods are already released and nothing
   is connected or pending, it says so and sends nothing. If a release is still closing, it waits for that
   one.
3. Otherwise it sends one release that quotes the revision it read and a new random number, **even when the
   AirPods are not connected**: the release is also the hold that keeps the dongle from taking them back. The
   dongle refuses a request with an old revision or a number it has already accepted. So a release cannot
   apply twice, and one that something else overtook is refused rather than applied late.
4. The dongle stops accepting the AirPods' incoming connections, then closes their control and audio
   links. The pairing stays.
5. The tool reads the routing state for up to 5 seconds. It checks that each read still carries its own
   request: the revision one past the one it quoted, its action and its number. Once nothing is connected or
   pending, the state says released. If closing takes longer, the tool reports the release as accepted, not
   yet closed. That is not an error: the dongle goes on closing it.

The tool never sends a release again by itself, with one exception. When the dongle refuses the release
because the revision moved before it arrived, nothing was applied. If the move was a reconnect attempt that
has ended, such as an automatic one under the `auto` policy, the tool reads the state once and sends one
fresh release. It does not when another app's handover moved it, or while a takeover is still under way,
which the dongle would refuse anyway. If the reply is lost, it looks for its own request in the routing
state. When it cannot find it, it exits `3`, because the release may have applied.

The dongle refuses a release while the microphone or a call is in use, during a takeover, an audio
connection change, an audio recovery or a Game Mode change, and while any app holds the settings
session. The pair command (opcode 2) also drops the link, but it erases the saved pairing, so the tool
never uses it to disconnect.

## After a release

The dongle declines the AirPods' incoming connections, and does not reconnect by itself, until one of these:

- `airpods connect` or `airpods reconnect`,
- a press of the dongle's button,
- another app asks for the AirPods back through the same handover,
- **a restart of the dongle**, such as unplugging it. The hold is kept in memory only.

Under the default `manual` reconnect policy the dongle does not connect to the AirPods by itself, after a
release, a restart or otherwise. Before a release, the AirPods can still connect to it on their own, for
example when the case opens.

**Under the `auto` policy a restart ends the hold and the dongle then takes the AirPods back by itself**, even
from another device that is using them. Changing the policy does not end a hold, and `disconnect` cannot
make one last across a restart. To keep the AirPods away from a dongle that restarts, save `manual` first:
`airpods reconnect-policy manual`. See [reconnect-policy](CLI.md#reconnect-policy).

## Not yet verified on hardware

The tests run against a simulator that follows the firmware's documentation. These need a real
dongle with firmware 0.1.59 or newer:

- Whether the AirPods stay away after a release, or keep offering a connection that the dongle
  declines.
- How long closing takes, against the 5 second wait.
- Whether `airpods connect` and `airpods reconnect` after a release bring them back every time, and how long
  `connect` takes to confirm.
- Which refusals come up in daily use.
- On 0.1.63 or newer: whether a dongle restarted under `auto` takes the AirPods back, and from a phone that uses
  them.

The check, with the AirPods connected and out of the case:

1. `airpods status --details` lists `routing`.
2. `airpods disconnect` prints `disconnect: released; the AirPods stay paired`, or `release accepted,
   not yet closed`, and the line about declined connections. `airpods status` then shows
   `AirPods: disconnected`.
3. They stay disconnected for at least a minute.
4. `airpods connect` brings them back without pairing again and prints `audio ownership confirmed`.
5. `airpods disconnect`, then `airpods reconnect`: they come back as well.
6. `airpods disconnect` during a call or while recording is refused with exit `1`, and the AirPods stay
   connected.
7. With the AirPods in their case and disconnected, `airpods disconnect` prints `released`, and opening the case
   does not connect them.
8. On 0.1.63 or newer, `airpods reconnect-policy` reads `manual`. After `airpods disconnect` and a replug of the
   dongle, the AirPods stay away until `airpods connect`.
