# Troubleshooting

Find your message below, or search this page for it. For what each part of the app does, see
[USER-GUIDE.md](USER-GUIDE.md).

The messages are shown in English here. If you use another language, most of them are translated. Windows has not been run
on real hardware yet, and the real tray has not been confirmed, so Windows and tray causes here come from the code and
general behaviour of those systems.

If you use the command line tool too, its
[troubleshooting page](CLI-TROUBLESHOOTING.md) has more on the USB side,
such as listing the dongle with `lsusb` and `Get-PnpDevice`.

## First checks

Look at the header of the window, or the first row of the tray menu:

| You see | Go to |
|---|---|
| `No dongle` | [No dongle](#no-dongle) |
| `Dongle problem` | [Dongle problem](#dongle-problem) |
| `Not connected`, or `Waiting for AirPods` | [Waiting for AirPods](#waiting-for-airpods) |
| `Connected`, and a message at the bottom of the window | [Settings read failing](#settings-read-failing), or [All other messages](#all-other-messages) |
| No tray icon | [Tray icon missing on GNOME](#tray-icon-missing-on-gnome) |

## No dongle

The header says `No dongle` and `Plug in the AirPods dongle`. The tray icon is an outline.

The app sees no dongle, and tries again every second. It recovers by itself once the dongle appears. This is not an error,
so nothing pops up.

- Plug it in, and use a cable that carries data. A cable that only carries power is a common cause.
- Check that the Pico runs compatible firmware. A Pico in bootloader mode shows up as a USB drive and not as a dongle. This
  repo has no firmware.

## Dongle problem

The header says `Dongle problem`, with the reason on the line below. The tray says `AirPods · Dongle problem`.

### Several dongles

The header says `Choose a dongle` and the tray says `AirPods · Choose a dongle`. Open the `...` panel and pick one in the
[Dongle](USER-GUIDE.md#dongle) list. The app remembers it by its USB serial.

```text
Several dongles are plugged in. Keep one plugged in.
```

This means two dongles report the same USB serial, so that they cannot be told apart. Unplug one.

```text
The dongle chosen with --serial is not plugged in.
```

The app was started with `--serial`, and no dongle has that serial. It must match exactly, including upper and lower case.
Plug it in, or start the app without the flag.

### No permission

```text
found /dev/hidraw3 but no permission to open it; install the AirPods udev rule; see the docs
```

Linux only. The dongle is there, but your user may not open it. The number in `/dev/hidraw3` varies.

The packages install this rule, so replug the dongle first. With a binary you built yourself, run this in the folder that holds the rule
file, then replug:

```sh
sudo install -m 644 70-airpods-hid.rules /etc/udev/rules.d/
sudo udevadm control --reload
```

In a clone of this repo the file is
[core/hid/70-airpods-hid.rules](../core/hid/70-airpods-hid.rules).

The rule only gives access to the user logged in at the machine. It does not help an SSH session, and the app has a window,
so this rarely matters. The command line tool's page has
[a rule for such sessions](CLI-TROUBLESHOOTING.md#ssh-and-other-sessions-without-seat-access).

Do not run the app with `sudo` to get around this.

### Other reasons

| Message | Cause | Fix |
|---|---|---|
| `Unsupported HID report identity/version` | The device has a matching ID, but it speaks another protocol version | Use compatible firmware, or update the app |
| `USB feature transfer exceeded 400 ms`, `Incomplete USB feature report`, `[WinError N] ...` | Windows could not talk to the dongle | Close other programs that use the dongle, replug it, and try another port |
| `Invalid HID interface detail size` | Windows returned unexpected data while listing devices | Replug. If it continues, please report it |
| `input/output error`, `no such device` or `connection timed out` | The dongle dropped off USB | Replug it, directly into the computer, with a short data cable. A Pico can reset if the port's power is poor |

## Waiting for AirPods

The header says `Not connected` and `Waiting for AirPods`, and the tray icon is dimmed. The dongle works, but it has no link to
the AirPods.

1. Take the AirPods out of the case, and keep them close to the dongle.
2. Disconnect them from any phone or computer that may have taken them over.
3. Click **Reconnect AirPods** in the tray or the `...` panel, and wait a few seconds.
4. If they never connect, the pairing may be lost. **Pair AirPods** pairs them again, but it
   [replaces the saved pairing](USER-GUIDE.md#pair-airpods), so use it last.

`Reconnect AirPods` is greyed out if the dongle has no saved pair. In that case, use **Pair AirPods**.

The header says `Connected` and `Waiting for AirPods` when the AirPods are connected but not yet ready for settings. This
can happen right after connecting. Wait a few seconds. If it stays, put them in the case for a moment and take them out
again. Rows and tray modes stay greyed out until then.

The batteries can keep showing old values while the AirPods are away. They are dimmed, with their age.

## Settings read failing

The window shows a message at the bottom that starts with `Settings:`, or the setting rows stay on `–`.

| Message | Cause | Fix |
|---|---|---|
| `Settings: the dongle isn't serving settings right now; trying again` | The dongle refused the read three times in a row. It refuses while the AirPods controls come up after connecting, and the header says `Waiting for AirPods` until then | Nothing, at first. The app retries after 1, 2, 4, 8 and 16 seconds, and then every 30 seconds. If it stays, replug the dongle and send the [debug log](USER-GUIDE.md#debug-log) |
| `Settings: another app is controlling the dongle` | Another program holds the request channel | [See below](#another-app-is-controlling-the-dongle). The app retries by itself |
| `Settings: Pico diagnostic read timed out` | The dongle did not answer in time. Despite the name, this also covers settings | Close other programs that use the dongle, then replug it if it continues |
| `Settings: Malformed settings snapshot`, `Settings: Malformed settings record` | The dongle's answer did not fit the expected format | Replug. If it continues, firmware and app may not match. Send the debug log |
| `Settings: Pico session changed; reconnect before issuing another command` | The connection changed during the read | Nothing. The app reads again |

A failed read with the AirPods not ready shows no message. The header already says what the page is waiting for.

With firmware that has no settings support, Playback and Find nearby are not on the page at all, and the tray can only switch
between Noise Cancellation and Transparency.

Rows that stay on `–` with no message are normal. It means the AirPods have not reported them. See
[What the values mean](USER-GUIDE.md#what-the-values-mean).

## Another app is controlling the dongle

```text
another app is controlling the dongle
```

Another program holds the dongle's request channel. Close it and try again:

- The `airpods` command, including a running `airpods diag` capture
- NTPods

Nothing was applied. See [Sharing the dongle](USER-GUIDE.md#sharing-the-dongle).

From the other side: if the `airpods` tool reports that it is busy, close this app's window. If that is not enough, turn
Auto-pause off, or use Quit in the tray menu.

## A change shows only as sent

When a change goes out and no answer comes back, the app shows `sent` and never sends it again. The next read shows
what really happened: the settings, the header and the connection notifications follow the AirPods' own reports. The
log keeps the detail. If a value still looks wrong after a few seconds, click **Read settings again** in
[Connection](USER-GUIDE.md#connection).

`Sent; not reported back` is different. The dongle took the change, and the AirPods just do not report that setting. It is
normal for some settings. See [What the values mean](USER-GUIDE.md#what-the-values-mean).

## Disconnect AirPods only shows a message

```text
this dongle firmware has no command to disconnect AirPods while keeping them paired; update it to 0.1.59 or newer, or put the AirPods in their case to disconnect
```

The dongle firmware is older than 0.1.59, and nothing is wrong. Update the firmware, or put the AirPods in their case. See
[DISCONNECT.md](DISCONNECT.md). Do not use Pair AirPods as a way to disconnect, as that replaces the saved pairing.

## Disconnect AirPods refused

```text
the dongle can't release the AirPods right now: the microphone or a call is in use, an audio or game mode change is pending, or another app holds the settings; try again in a moment
```

The dongle refuses to let the AirPods go while something needs them. Nothing was sent again.

1. End the call or recording, and wait a few seconds.
2. Close other apps that use the dongle, such as the CLI in a settings session.
3. Click Disconnect AirPods again.

`something else changed the connection first, such as a button press or another app; nothing was sent again` means another
choice came first. Look at the header, and try again if the AirPods are still connected.

## The AirPods do not come back

Under the Manual reconnect policy, the default, the dongle does not connect to the AirPods by itself, and after Disconnect
AirPods it turns them away. Click **Reconnect AirPods** in the tray, the `...` panel or the card below About.

`Taking the AirPods over` on the card means the dongle accepted Reconnect and is still connecting. Wait a few seconds. If the
card offers Reconnect again after that, the AirPods may be out of range or in their closed case.

## The AirPods come back by themselves

The dongle's reconnect policy is Automatic. It then retries the AirPods whenever they are not connected and not released,
and can take them from a phone. A Disconnect also ends when the dongle restarts or is plugged in again. Open
[Connection](USER-GUIDE.md#reconnect-policy) and choose Manual. See [DISCONNECT.md](DISCONNECT.md#the-reconnect-policy).

## Music pauses or plays by itself when an AirPod comes out or goes in

Dongle firmware 0.1.65 and newer pause USB playback when a bud comes out, and play it again when it goes back in, while the
AirPods' **Ear detection** is on. They also lower the music during Conversation Awareness. This is the dongle, not the app,
and the app cannot see it or turn it off on the dongle.

- To stop it, turn **Ear detection** off in the Device card. That also stops the AirPods' own ear detection. To stop the
  lowering, turn **Conversation Awareness** off.
- If the app's **Auto-pause** is on as well, both act on the same removal. Turn Auto-pause off in the `...` panel. It is off
  by default since this version, unless you turned it on yourself earlier.
- A call through the computer's own microphone counts as music for the dongle, so it can be lowered or paused too.

## Auto-pause does nothing

- It is off by default. Turn it on in the `...` panel, if your dongle firmware is older than 0.1.65.
- It acts only while USB audio plays to the dongle, and only on a player that reports it is playing. After the dongle's own
  pause there is nothing left for it to do.
- On Windows it needs the system media controls. Where they are not available it does nothing, and the
  [debug log](USER-GUIDE.md#debug-log) says `auto-pause: pause: media controls are not available`. It never falls back to the
  Play/Pause key.

## Tray icon missing on GNOME

Linux trays need a StatusNotifier host. KDE has one. Stock GNOME does not, unless the AppIndicator extension is installed.

With no host, the app opens the window, even with `--tray`, after waiting 5 seconds for a host that may still be starting.
Closing the window then ends the app, because a hidden window could not be reached again. If you start the app from a terminal
it may also print this, which you can ignore:

```text
systray error: failed to register: The name org.kde.StatusNotifierWatcher was not provided by any .service files
```

To get the icon, install the extension, then log out and back in:

```sh
sudo apt install gnome-shell-extension-appindicator    # Debian, Ubuntu
sudo dnf install gnome-shell-extension-appindicator    # Fedora
sudo pacman -S gnome-shell-extension-appindicator      # Arch
```

If the icon still does not show, enable the extension in the Extensions app. Other desktops may need their own tray plugin
that supports StatusNotifier.

If you added the app to your startup items while no tray was available, the window opens at each login. That stops once a tray
is there.

## Window does not group with the launcher

On X11 the window may show as a separate icon in your dock or taskbar instead of grouping with the `AirPods` launcher. It may
also show a generic icon.

On X11 the app's window class is its title, `AirPods`, while the launcher names the Wayland ID,
`io.github.dardrai.airpods-gui`. It does no harm. To fix it for your user, copy the launcher and change that one line:

```sh
mkdir -p ~/.local/share/applications
cp /usr/share/applications/airpods-tray.desktop ~/.local/share/applications/
sed -i 's/^StartupWMClass=.*/StartupWMClass=AirPods/' ~/.local/share/applications/airpods-tray.desktop
```

Log out and in if your desktop does not pick it up.

## Wayland: popups in odd places

The **Show batteries** window asks to stay on top. Windows and X11 honour that. On Wayland the compositor decides, so the small
window may open behind others or somewhere you do not expect. It closes by itself after 6 seconds. The tray menu's battery row
and the window's header show the same readings.

On GNOME under Wayland, the window may have no title bar unless `libdecor` is installed. The packages recommend it.

No Wayland run is recorded yet. Reports are welcome.

## Windows SmartScreen

```text
Windows protected your PC
```

The installer and the exe are not signed, so SmartScreen does not know them. Check the file against `SHA256SUMS` from the
same release:

```powershell
Get-FileHash .\airpods-gui-<version>-setup-x64.exe -Algorithm SHA256
```

If it matches, click **More info**, then **Run anyway**. If you would rather not, you can [build it yourself](BUILD.md).

If your browser marked the file as downloaded from the internet, you can clear that with
`Unblock-File` followed by the file name.

## All other messages

### Rows in the settings cards

| Message | Cause | Fix |
|---|---|---|
| `Still arriving` | The AirPods have just connected and are still sending their settings | Nothing. The row fills in within a few seconds |
| `Not reported` | The AirPods have not reported this setting, and the app cannot safely change it blind | Nothing. It may report later |
| `Available once Adaptive is confirmed` | Strength can only change once the AirPods have confirmed Adaptive mode | Select Adaptive and wait |
| `these AirPods did not accept this` | The AirPods or the dongle refused the setting | Nothing was applied. Your model or firmware may not have it |
| `not available on these AirPods` | This value does not exist on your model. For example, Off with Allow Off mode off | Pick another value, or turn on Allow Off mode |
| `pick at least two modes` | Modes for press and hold needs two or more | Select another mode |
| `could not remember this choice` | The app could not save the choice to its settings folder. The change itself was sent | Check that your config folder is writable |
| `unknown device value (0x…)` | The AirPods reported a value this app has no word for | Please report it, with your AirPods model |
| `AirPods settings connection is not ready` | The AirPods became unready as you changed the setting. Nothing was sent | Wait for the header to say `Connected`, and try again |
| `Stop microphone use before changing AirPods settings` | An app is using the dongle's microphone. Nothing was sent | Close the call or recording app |
| `Firmware does not advertise typed settings support` | The firmware has no settings interface | Use compatible firmware |
| `Firmware does not support this request for this device` | The firmware or the AirPods do not support it | Nothing was applied |

### Connection actions

These show at the bottom of the window, or as a notification when they come from the tray. They start with the action's name,
such as `Reconnect: sent`.

| Message | Cause | Fix |
|---|---|---|
| `sending`, `sent` | The action went to the dongle, and it took it. This does not yet mean the AirPods acted on it | Watch the header |
| `connected; the dongle has the AirPods` | Reconnect finished: the dongle owns the AirPods' audio and controls | Nothing |
| `taking over; the AirPods are on their way` | The dongle accepted Reconnect and is still connecting | Wait a few seconds. See [The AirPods do not come back](#the-airpods-do-not-come-back) |
| `the dongle can't take the AirPods back right now: ...` | The dongle refused Reconnect during a call, microphone use, an audio or game mode change, or another app's settings session | End what applies, close other apps that use the dongle, and try again |
| `something else changed the connection first, such as a button press or another app; nothing was sent again` | Another choice came first, possibly an automatic reconnect | Look at the header, and try again if needed |
| `Pair AirPods: sent` | Pair AirPods went out. A late busy answer can still follow an erased pairing; it is never sent again | Check the header. If the AirPods do not come back, pair them again |
| `released; the AirPods stay paired`, `release accepted, closing the connection` | Disconnect worked, or the dongle is still closing the connection | Nothing. Click Reconnect AirPods to bring them back |
| `the dongle can't release the AirPods right now: ...` | The dongle refused Disconnect | See [Disconnect AirPods refused](#disconnect-airpods-refused) |
| `Pairing is blocked while the dongle is in use. Stop USB audio and microphone use and disconnect the phone first.` | Pairing is refused during use | Do what it says |
| `Control failure: busy; another companion request or settings session is running, or USB audio, the microphone or a phone is in use` | The dongle refused, for one of these reasons | Stop what applies, close other apps that use the dongle, and try again |
| `Control failure: not supported by this dongle` | The firmware does not offer this command | Use compatible firmware |
| `Control failure: AirPods disconnected` | The AirPods are not connected | See [Waiting for AirPods](#waiting-for-airpods) |
| `Control failure: stale session`, `Control failure: malformed` | The connection changed during the command, or the firmware may not match | Try again, and then check the firmware |
| `no dongle` | The dongle was gone when the action ran | See [No dongle](#no-dongle) |
| `too many actions are waiting, try again` | The queue is full | Wait a moment and try again |
| `this dongle cannot select that mode` | The tray asked an older firmware for a mode it cannot set | Use Noise Cancellation or Transparency, or update the firmware |

The reconnect policy shows its messages below its row in Connection:

| Message | Cause | Fix |
|---|---|---|
| `Saved and verified.`, `Already saved; nothing was sent.` | The change worked, or there was nothing to change | Nothing |
| `Busy: the dongle saves the policy only while idle. ...` | Playback, the microphone, a call, connecting, a Game Mode change, or a settings session of any app, this one included, was in the way. Nothing was saved | Pause playback, close the CLI and wait for this app's hearing requests to end, then choose again |
| `Not available: the dongle firmware is older than 0.1.63, or it cannot read its saved policy. It acts as Manual.` | Older firmware, or an unreadable saved policy | Update the firmware. The dongle acts as Manual |
| `sent` | The save went out but the dongle could not verify it. It was not sent again | Click **Read**. If it says not available, replug the dongle, read again, then choose again |
| `the app is shutting down` | The app is quitting | Nothing |

### Audio and Playback

| Message | Cause | Fix |
|---|---|---|
| `Codec profile: applied`, `Game mode: applied` | The dongle confirmed the change | Nothing |
| `Codec profile: sent`, `Game mode: sent` | The link can restart before the reply arrives | Check Active and Requested after a few seconds |
| `Audio: ...`, `Unsupported dongle audio profile.` | The read failed, or the firmware has a format this app does not know | Use matching firmware. The app tries again |

### Find nearby

| Message | Cause | Fix |
|---|---|---|
| `Finder not available right now` | The dongle has not answered about the finder yet | Wait. It tries again every 5 seconds |
| `Searching isn't available for this pair yet` | The dongle cannot search for these AirPods at the moment | Connect the AirPods, and try again |
| `Another app is already searching` | Another program started the finder | Close it |
| `Paused while audio is playing` | The dongle pauses the finder during playback | Stop the audio |
| `Waiting for the first signal`, `Waiting for another signal` | Nothing heard, or nothing recent | Move around, and open the case |
| `Nearby: the finder did not start` | The dongle did not start within 5 seconds | Try again |
| `Unsupported dongle nearby-finder snapshot.`, `Invalid dongle signal strength.`, `Unverified dongle signal reading.`, `Unsupported finder response.` | The firmware has a format this app does not know | Use matching firmware |

### Diagnostics

| Message | Cause | Fix |
|---|---|---|
| `Unsupported dongle performance snapshot.`, `Inconsistent dongle performance counters.` | The firmware has a format this app does not know | Use matching firmware |

### Hearing

| Message | Cause | Fix |
|---|---|---|
| `Hearing assistance has not been set up on these AirPods. Set it up from an iPhone or iPad.` | Setup needs a hearing test this app cannot run | Set it up on an iPhone or iPad |
| `Hearing assistance is not supported by the model these AirPods report.`, `Hearing assistance is not available on these AirPods.` | The model or its firmware does not have it | Nothing |
| `Connect the dongle's AirPods before reading or changing hearing data.` | The AirPods are not connected and ready | See [Waiting for AirPods](#waiting-for-airpods) |
| `Update the dongle firmware before reading or changing hearing data.` | The firmware lacks a feature this needs | Update the firmware |
| `Stop recording and wait for the microphone to recover before reading or changing hearing data.` | The microphone is in use | Close the call or recording app |
| `sent`, with a `Turn Hearing Aid off` button | The change went out with no answer. It was not sent again | Click the button if you want it off, then **Read** |
| `Wait for the AirPods to report the other hearing mode before changing this one.`, `Wait for the AirPods to report Hearing Aid before editing.`, `The current hearing modes are not known yet. Read them first.` | A report has not arrived | Wait, then click **Read** |
| `The AirPods' hearing modes disagree. Reconnect the AirPods before changing them.` | Their reports contradict each other | Reconnect the AirPods |
| `The hearing modes moved while the request was being checked. Read them again.`, `The hearing modes changed before the profile write. Read the settings again.`, `The hearing modes changed before the request finished. Read them again.`, `The stored hearing profile is no longer the one read at the start. Read it again.`, `The hearing profile changed on the AirPods. Read it again before editing.`, `The hearing profile changed or cannot be edited. Read it again before editing.` | Something changed, possibly on another device | Click **Read** |
| `This profile can be read but not edited.`, `This hearing profile can be read but not edited.` | The profile on the AirPods is in a form the app will not write | Change it on an iPhone or iPad |
| `Each audiogram value must be a number from 0 to 90.` | A cell is empty or out of range | Fix the cell |
| `Not changed: each ear's gain has to stay between -1.5 and 1.5.` | Amplification or Balance would put one ear out of range. Nothing is trimmed | Lower the other control first |
| `No reply to the hearing profile write. ...`, `The AirPods did not apply the hearing profile. ...`, `The profile was written, but the hearing modes changed. ...` | The write's outcome is unknown | Click **Read** before you change it again |
| `A complete, valid hearing profile is required.` | A value is out of range | Click **Read** to start again |
| `The AirPods returned a hearing profile in a format this app does not know.`, `The AirPods returned hearing profile values this app does not know.` | Other firmware may store it differently | Please report it, with your model and firmware |
| `Custom Transparency is on. Turn it off before enabling hearing assistance.`, `Could not confirm Custom Transparency is off, so hearing assistance stays as it is.` | The two cannot be on together | Turn Custom Transparency off in More experiments |
| `There is no hearing mode change to recover from.`, `Unknown hearing mode.` | The app has nothing to undo | Click **Read** |
| `A hearing change is still being confirmed. Wait for it to finish.` | An earlier change has not ended yet | Wait up to 15 seconds |

### More experiments

These may not work through the dongle at all. An error here can mean that the dongle does not pass the request on.

| Message | Cause | Fix |
|---|---|---|
| `Custom Transparency is not supported by the model these AirPods report.`, `Custom Transparency is not available on these AirPods.`, `Custom Transparency is read-only on these AirPods.` | The model or firmware does not offer it | Nothing |
| `Read Custom Transparency before changing it.`, `Custom Transparency changed on the AirPods. Read it again before changing it.`, `Loud Sound Reduction changed on the AirPods. Read it again before changing it.` | The app needs a fresh value first | Click **Read** |
| `Hearing Aid or Media Assist is on, or not reported off. Until both are off, Custom Transparency can only be turned off.` | The editor is locked while hearing assistance may be active | Turn Hearing Aid and Media Assist off in Hearing, then **Read** again |
| `Hearing Aid and Media Assist must be reported off before Custom Transparency can be changed, except to turn it off.` (with `If they are off, reconnect the AirPods.` when not reported) | As above, from the request itself | As above, or reconnect |
| `Loud Sound Reduction is not available on these AirPods.` | The AirPods did not offer it when the app looked it up | Nothing |
| `Custom Transparency values are out of range.`, `The AirPods returned a Custom Transparency record in a format this app does not know.`, `The AirPods returned Custom Transparency values this app does not know.` | Unexpected values | Click **Read**. Report it if it continues |
| `Keep each EQ band between 0 and 100.` | A value is out of range | Fix the value |
| `These AirPods have not reported their EQ on this connection, so it cannot be read or changed.` | No EQ report from these AirPods yet | Reconnect the AirPods, then **Read** |
| `The dongle firmware does not keep the AirPods' EQ. EQ needs dongle firmware 0.1.64 or newer.` | Older firmware | Update the firmware |
| `Not sent: the dongle has no EQ report from these AirPods yet, or does not know their model. Read the EQ again.` | The dongle declined to pass the change on. Nothing was sent | Click **Read**. If it continues, reconnect the AirPods |
| `The EQ changed on the AirPods. Read it again before changing it.` | Another device changed it | Click **Read** |
| `Sent, and reported back by the AirPods.` | The AirPods reported exactly the values sent | Nothing |
| `The EQ change was sent, but the AirPods did not report it. ...`, `A newer EQ report from the AirPods differs from the EQ sent. ...` | The outcome is unknown. It was not sent again | Click **Read** |
| `Use a name of 1 to 64 UTF-8 bytes with no control characters.` | The name is empty or too long. Some letters take more than one byte | Use a shorter name |
| `Sent. Its effect is unknown.` | The AirPods give no answer to Rename | Reconnect to see the name. This is not an error |
| `The dongle refused to pass the name on to the AirPods: its firmware only sends a fixed list of AirPods requests. Nothing was sent.` | The dongle firmware does not pass Rename on | Nothing. It needs a dongle firmware that sends it |

### Hardware checks

| Message | Cause | Fix |
|---|---|---|
| `Connect the dongle's AirPods first.` | The AirPods are not ready | See [Waiting for AirPods](#waiting-for-airpods) |
| `Not available on these AirPods.`, `Saved hardware checks are not available on these AirPods.`, `Hardware diagnostics are not available on these AirPods.`, `These AirPods do not expose this data.` | The model does not offer it | Nothing |
| `Stop recording first.` | The microphone is in use | Close the call or recording app |
| `Update the dongle firmware to read this.` | The firmware lacks a feature this needs | Update the firmware |
| `The in-case check is only verified on AirPods Pro 3 (A3063, A3064 or A3065) with firmware 9A348.` | The app refuses it elsewhere, on purpose | Nothing |
| `Both AirPods must be in the open case, not in your ears, and the case report must be recent.` | The AirPods did not report both buds in the case | Put them in, leave the lid open, and try again shortly |
| `The in-case check needs the audio idle. Pause playback and recording and disconnect phone audio first.` | Audio is in use | Do what it says |
| `Read the saved hardware results before starting a check.` | The AirPods returned no earlier result to compare with | Click **Refresh** first. The AirPods may not offer the check |
| `These AirPods do not offer the hardware check request.`, `The hardware check request does not support a response.` | The AirPods do not offer the check | Nothing |
| `No new results after 2 minutes. Nothing was sent again.` | The AirPods did not finish in time | Click **Refresh** later. They may still finish |
| `Stopped waiting. The AirPods may still finish, since a check cannot be cancelled.` | You clicked Stop waiting | Click **Refresh** later |
| `Unsupported hardware diagnostic record; expected version 8 and 71 bytes`, `AirPods returned an unsupported dosimetry value length.`, `AirPods returned a non-finite dosimetry value.`, `AirPods returned an unsupported hearing-test mode length.` | Other firmware may store this differently | Please report it, with your model and firmware |

### Requests in general

These can show in any section.

| Message | Cause | Fix |
|---|---|---|
| `The dongle request ran out of time and was not retried.` | A request took more than 60 seconds | Try again |
| `The dongle connection changed. Read the settings again.`, `Dongle settings lease ended. Read the settings again.` | The AirPods reconnected or the connection changed during the request | Read again |
| `Pico session changed; reconnect before issuing another command` | As above | Try again. You do not need to reconnect by hand |
| `Unsupported dongle settings lease.`, `Dongle settings lease is unavailable.`, `Unsupported dongle settings lease renewal.`, `Invalid dongle settings payload length.`, `The previous dongle request has not finished cancelling.` | The firmware could not hold the channel for this request, or it has a format this app does not know | Try again, then replug. Use matching firmware |
| `Oversized HID page`, `Invalid diagnostic length`, `Invalid diagnostic pagination`, `Diagnostic page exceeds declared length`, `Dongle returned an incomplete response.`, `Incomplete dongle report.`, `Dongle report has no reception revision.`, `Unknown dongle report.`, `Unexpected setting-write response` | The dongle's answer did not fit the expected format | Replug. If it continues, firmware and app may not match |
| `RPC failure: malformed request`, `RPC failure: stale session`, `RPC failure: AirPods disconnected`, `RPC failure: device timeout`, `RPC failure: transport failure`, `RPC failure: cancelled`, `RPC failure: N` | The dongle returned a failure | Check that the AirPods are connected and close, then try again. See the [same messages for the CLI](CLI-TROUBLESHOOTING.md#connection-and-protocol-failures) |

### The `...` panel

| Message | Cause | Fix |
|---|---|---|
| `The startup entry points to another copy of this app. Turn the option on to repair it.` | You moved the program, or another copy made the entry | Turn Start with the system on |
| `Start with the system: ...` | The entry could not be written or read back | Check that you can write to the autostart folder, or on Windows to your Run key |
| `Debug log copied` | It worked | Paste it where you need it |
| `No debug log in this run.` | The app kept no log | Nothing |

## Still stuck

Open an issue with:

- what you did, and the message you saw
- the [debug log](USER-GUIDE.md#debug-log)
- the version, from `airpods-tray --version` or the `...` panel
- your OS and desktop, and whether it runs X11 or Wayland
- your AirPods model

Check any screenshot of Diagnostics first, because it shows your AirPods' Bluetooth address.
