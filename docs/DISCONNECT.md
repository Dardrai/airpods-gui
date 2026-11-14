# Disconnect AirPods

Disconnect AirPods ends the dongle's connection to the AirPods and keeps them paired. It uses the routing handover of the
[dongle firmware](https://github.com/Ripthulhu/airpods-dongle), which needs firmware 0.1.59 or newer. Pairing is not a
substitute: opcode 2 erases the saved pairing, so the app never uses it for this.

This has run only against the simulated dongle. It has not been confirmed with real AirPods yet.

## What the app does

The dongle lists the handover as the `routing` capability in its status report. With it, the item is in the tray menu, the
`...` panel and on the page below About, and it is enabled while the AirPods are connected. A click:

1. Reads the routing state, then sends one release that quotes its revision and a fresh number. It is never sent twice.
2. Waits up to 5 seconds for the dongle to close the connection. The status and the tray stay as they were meanwhile.
3. Shows the outcome at the bottom of the window:

| Outcome | Meaning |
|---|---|
| `Disconnect: released; the AirPods stay paired` | The connection is closed |
| `Disconnect: release accepted, closing the connection` | The dongle took the release and is still closing. Also a notification. The page says `Closing the connection` until it has closed |
| `Disconnect: the dongle can't release the AirPods right now: ...` | The dongle refused. Also a notification. See [TROUBLESHOOTING.md](TROUBLESHOOTING.md#disconnect-airpods-refused) |
| `Disconnect: something else changed the connection first, ...` | A button press, a reconnect or another app came first. Nothing was sent again |

While a release is closing, the app reads the routing state every 2 seconds until it has closed. It reads nothing else for
it: the status report shows the connection go and come back.

Without the `routing` capability the item stays enabled, sends nothing, and shows this as a notification and in a dialog:

> this dongle firmware has no command to disconnect AirPods while keeping them paired; update it to 0.1.59 or newer, or put
> the AirPods in their case to disconnect

## After a release

The dongle declines the AirPods' incoming connections until one of these:

- Reconnect AirPods, in the tray, the `...` panel or on the page.
- A button press.
- A restart of the dongle, including unplugging it. The release is kept in memory only.

While the AirPods are away, the card below About offers **Reconnect** instead of Disconnect, and says why. On a dongle with
the `routing` capability, Reconnect asks for the AirPods through the same handover: it reads the routing state, sends one
acquire with a fresh number, and waits up to 5 seconds for the dongle to have them, with audio and controls up. The window
then says `Reconnect: connected; the dongle has the AirPods`, or `Reconnect: taking over; the AirPods are on their way`
while the dongle is still connecting, and the card says `Taking the AirPods over` until they arrive or 20 seconds pass.
It is never sent twice. Without the capability, Reconnect sends opcode 1, which ends a release too.

## The reconnect policy

Dongle firmware 0.1.63 and newer keep a reconnect policy, which the Connection section shows and changes. See
[Reconnect policy](USER-GUIDE.md#reconnect-policy).

- Under **Manual**, the default, the dongle does not connect to the AirPods on its own, as since firmware 0.1.58. It still
  accepts them when they connect to it, except after a release.
- Under **Automatic** the dongle retries the saved AirPods by itself while they are not connected and not released. That
  can take them from a phone or another computer using them. A release still holds while the dongle runs, but it is lost
  when the dongle restarts or is plugged in again, and the dongle then takes the AirPods back by itself. Changing the
  policy does not end a release.

So under Automatic, Disconnect AirPods is not a lasting way to hand the AirPods to another device. Choose Manual for that,
or put the AirPods in their case.

Once the app has read the policy, the card says which applies while the AirPods are away.

Not yet confirmed on hardware: whether the AirPods stay away after a release, or keep offering a connection that the dongle
declines, and whether Automatic attempts win the AirPods from a phone.

## Trying it

`AIRPODS_GUI_SIM=disconnect` starts a simulated dongle whose release stays pending for longer than the 5-second wait, so the
closing state shows. `AIRPODS_GUI_SIM=old` has no `routing` capability and shows the message. `AIRPODS_GUI_SIM=legacy` has
the handover but no reconnect policy. See
[BUILD.md](BUILD.md#trying-the-ui-without-a-dongle).
