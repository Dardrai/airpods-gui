# Building and testing

One script builds and tests the tray app `airpods-tray`, the command line tool `airpods` and the library in `core/`
that both use. Everything runs in the fyne-cross images, so a host needs only Docker or podman: `CONTAINER_ENGINE` if set, else `docker`
if present, else `podman`. Rootless podman works, and `dist/` stays owned by you. Go and C toolchains come from the
images: `docker.io/fyneio/fyne-cross-images:v1.3.4-linux` and `-windows`, with Fyne 2.8.2. Set `FYNE_CROSS_TAG` to
use another image tag.

The binaries, tests and `govulncheck` use Go 1.26.11, not the images' own Go 1.26.10, which lacks the fix for
GO-2026-6859. `go_toolchain` in `build/fyne-cross.sh` names it and goes to the containers as `GOTOOLCHAIN`, so the go
command downloads that release into the module cache once and checks it against the checksum database. Every run prints
`go version` first and stops if it is not that release. To move to a newer Go, change `go_toolchain`, or set
`GO_TOOLCHAIN=go1.26.12` for one run; `go.mod` keeps `go 1.26.0`. Once a fyne-cross image ships that Go itself, the pin can
stay or go back to `local`.

```
build/fyne-cross.sh test      # gofmt, go vet, go test -race, for everything
build/fyne-cross.sh vulncheck # govulncheck
build/fyne-cross.sh cli       # dist/airpods-linux-amd64, -linux-arm64, -windows-amd64.exe, dist/completions/
build/fyne-cross.sh linux     # dist/airpods-tray, dist/airpods-tray.png, dist/icons/
build/fyne-cross.sh package   # linux and cli, then the .deb, .rpm and .pkg.tar.zst in dist/
build/fyne-cross.sh windows   # dist/airpods-tray.exe
build/fyne-cross.sh installer # windows and cli, then dist/airpods-gui-<version>-setup-x64.exe
build/fyne-cross.sh all
```

The command line tool is pure Go (`CGO_ENABLED=0`), so the Linux image cross-builds it for Linux x64 and arm64 and for
Windows. The tray app needs cgo and is built for x64 only. Both get the same version: `git describe --tags`, or
`VERSION` if set, which `airpods-tray --version` and `airpods --version` print.

`test` installs fish and zsh in its container if they are missing, because the completion tests ask each shell itself
and skip one that is not there.

The script keeps Go caches in the named volumes `airpods-gomod` and `airpods-gocache`. With `AIRPODS_CACHE_DIR` set they
are in `gomod/` and `gocache/` under that host directory instead, owned by you, which is how CI saves them. The Windows build uses
`-H=windowsgui` and embeds the icon from `cmd/genicon` and the manifest from `build/windows/winres.json`. Set `VERSION`
to override the version shown in the `...` panel and by `--version`.

To build on a remote Docker host, copy the tree over and run the script there:

```
tar -C . --exclude=.git --exclude=dist -cf - . | ssh HOST 'mkdir -p /tmp/airpods-gui && tar -C /tmp/airpods-gui -xf -'
ssh HOST 'cd /tmp/airpods-gui && build/fyne-cross.sh all'
```

## The library and vendor/

The dongle code (`hid`, `dongle`, `dongle/sim`, `disconnect`, `hearing`, `model`) is in `core/`, as
`github.com/Dardrai/airpods-gui/core/...`. It is the only code that talks to the dongle, and it imports nothing from
`cmd/` or `internal/`, so the app and the command cannot drift apart. [core/README.md](../core/README.md) describes it.
It was the separate module airpods-core until v0.6.0.

`vendor/` holds the other modules and is committed, so CI and the build script work offline: Go uses `-mod=vendor` by
default when `vendor/` exists. Do not edit `vendor/` by hand.

To move a dependency, such as `golang.org/x/image` when `govulncheck` reports it, run this one-shot container from the
repo root with the module and version in place of `MODULE@VERSION`, then commit `go.mod`, `go.sum` and `vendor/`. It
uses the image's own Go, which only resolves and copies modules:

```
docker run --rm --entrypoint sh -e GOTOOLCHAIN=local -e HOST_ID="$(id -u):$(id -g)" -v "$PWD":/src -w /src \
  fyneio/fyne-cross-images:v1.3.4-linux -c \
  'go get MODULE@VERSION && go mod tidy && go mod vendor && chown -R "$HOST_ID" vendor go.mod go.sum'
```

With podman, use `podman` in place of `docker`.

## Linux packages

`build/fyne-cross.sh package` builds the packages with `goreleaser/nfpm`:

| Package | From | Holds | Built for |
|---|---|---|---|
| `airpods-gui` | `build/nfpm.yaml` | The tray app and the `airpods` command | x64 |
| `airpods-cli` | `build/nfpm-cli.yaml` | The `airpods` command alone | x64 and arm64 |

`airpods-gui` is the one most people install. It installs both binaries, `airpods-tray.desktop`, the hicolor icons,
`70-airpods-hid.rules`, the completions for bash, zsh and fish, and the license. `airpods-cli` installs the command,
the rule, the completions and the license, and has no dependencies, since the binary is static:

| Path | Contents | In |
|---|---|---|
| `/usr/bin/airpods-tray`, the launcher and the icons | The app | `airpods-gui` |
| `/usr/bin/airpods` | The command | both |
| `/usr/lib/udev/rules.d/70-airpods-hid.rules` | The rule from `core/hid/70-airpods-hid.rules` | both |
| `/usr/share/bash-completion/completions/airpods` | bash completion | both |
| `/usr/share/fish/vendor_completions.d/airpods.fish` | fish completion | both |
| `/usr/share/zsh/vendor-completions/_airpods` (deb), `/usr/share/zsh/site-functions/_airpods` (rpm, Arch) | zsh completion | both |

The two hold the same files at the same paths, so a computer has one of them. `airpods-gui` provides, replaces and
conflicts with `airpods-cli`, and `airpods-cli` conflicts with `airpods-gui`: installing the app's package over the
command's removes the command's, and nothing of the command is lost. This also holds for the airpods-cli packages up to
0.3, which were a separate project, had `/usr/bin/airpods-diag` (now `airpods diag`) and named the rule
`71-airpods-cli-hid.rules`. One case needs a hand: `pacman -U --noconfirm` answers no when it asks whether to remove
`airpods-cli`, so on Arch with an old `airpods-cli` installed, the in-app update of an older app fails once. Run
`sudo pacman -U` on the new package yourself and answer yes.

A rule must sort before `73-seat-late.rules` for `uaccess` to work, so keep its number below 73.

Install a downloaded package, which also pulls in its dependencies:

```
sudo apt install ./airpods-gui_<version>_amd64.deb            # Debian 12 or newer, Ubuntu 22.04 or newer
sudo dnf install ./airpods-gui_<version>.x86_64.rpm           # Fedora
sudo pacman -U ./airpods-gui_<version>-x86_64.pkg.tar.zst     # Arch
```

Remove it with `sudo apt purge airpods-gui`, `sudo dnf remove airpods-gui` or `sudo pacman -R airpods-gui`. Replug the
dongle after installing if it does not open.

On GNOME the tray icon needs the AppIndicator extension, `gnome-shell-extension-appindicator` on all three. It is
optional: an RPM recommends it only when GNOME Shell is installed, Arch lists it as an optional dependency, and the
`.deb` only suggests it because a recommendation would pull GNOME Shell onto other desktops. Install it yourself on
Debian. Stock GNOME without it opens the window instead.

File names use the tag without its `v`, because GitHub rewrites a tilde in a release asset name. The version inside the
package follows each format's rules, so a pre-release sorts before its release:

| Tag | Debian, Fedora | Arch |
|---|---|---|
| `v0.0.2-alpha` | `0.0.2~alpha` | `0.0.2alpha` |
| `v0.0.2` | `0.0.2` | `0.0.2` |

`build/fyne-cross.sh pkg-version` prints the mapping for the current `VERSION`.

## Windows installer

`build/fyne-cross.sh installer` builds both exes, then runs `makensis` on `build/windows/installer.nsi` in a Debian
container, with warnings treated as errors. The installer is not signed, because the project has no certificate. See
[Windows SmartScreen](TROUBLESHOOTING.md#windows-smartscreen).

It installs for the current user, without admin rights, into `%LOCALAPPDATA%\Programs\AirPods Tray`, and registers under
`HKCU` in Add or Remove Programs. Start with Windows writes the same `Run` value as the App tab's option. An upgrade
installs in place and keeps the settings, which live in `%APPDATA%`.

Its components page has these boxes. At least one of the first and the fourth has to stay ticked:

| Box | Default | Effect |
|---|---|---|
| AirPods app | on | `airpods-tray.exe` and its Start Menu shortcut. Unticked, the command line tool is installed alone |
| Start the app with Windows | off | The `Run` value |
| Desktop shortcut for the app | off | |
| Command line tool | on | `airpods.exe`, in the same folder |
| Add the command line tool to PATH | on | Puts that folder on the user's PATH, so that a terminal opened afterwards finds `airpods` |

The installer records what it installed under its uninstall key, as `InstalledApp`, `InstalledCommandLine` and
`CommandLineOnPath`. An upgrade starts from those, so the in-app update of a command-line-only install stays that way,
and unticking a box on an upgrade removes what the earlier install set up. An install from before the command line
tool recorded nothing, so its first upgrade adds the tool and the PATH entry.

The PATH entry is written by `airpods.exe` itself, through the hidden switch `--installer-path add` or `remove`
(`internal/winpath`): it edits the user's `Path` value in the registry, keeps the other entries and the value's type as
they are, and announces the change. NSIS's own string limit would cut a long PATH short. If it fails, the installer says
so and installs the rest.

| Switch | Effect |
|---|---|
| `/S` | Installs silently. An upgrade keeps what the last install chose, and the current Start with Windows and desktop shortcut choices. |
| `/CLIONLY` | Installs the command line tool without the app, and removes an installed app. |
| `/NOCLI` | Installs the app without the command line tool, and removes an installed one. |
| `/NOPATH` | Leaves PATH without the tool's folder. |
| `/AUTOSTART`, `/DESKTOP` | Turns that option on, also with `/S`. |
| `/D=path` | Installs elsewhere. It must come last and has no quotes. |

For the uninstaller, `/S` removes the app silently and `/REMOVESETTINGS` also removes the settings. Without it they stay.

Before replacing or removing the exe, both ask a running app to close with `airpods-tray.exe --quit`, which goes through
the single-instance socket and waits. If the exe stays locked they use `taskkill`, and then give up with an error rather
than leave a partial install.

The installer has run under Wine only, not yet on real Windows. Wine has no Unix sockets, so the single-instance guard
and `--quit` do nothing there and only the `taskkill` path has run. The silent switches have run there: a default
install, `/CLIONLY`, `/NOCLI`, `/NOPATH`, an upgrade that keeps the last choice, an upgrade of an install from before the
command line tool, and the uninstaller, with the PATH entry added and removed among other entries. The components page
itself has not been clicked through.

## Linux runtime requirements

The binary is linked for glibc 2.31, which covers Debian 11 or newer and Ubuntu 20.04 or newer, though the packages
target the releases above. The fyne-cross image's libraries are newer than that, and only their sonames are
used at link time. The binary links `libGL` and `libwayland-client`, and GLFW loads the rest at run time, so `ldd` does
not list them:

| Library | Debian, Ubuntu | Fedora | Arch |
|---|---|---|---|
| `libGL.so.1` | `libgl1` | `libglvnd-glx` | `libglvnd` |
| `libEGL.so.1` | `libegl1` | `libglvnd-egl` | `libglvnd` |
| `libX11.so.6` | `libx11-6` | `libX11` | `libx11` |
| `libX11-xcb.so.1` | `libx11-xcb1` | `libX11-xcb` | `libx11` |
| `libXcursor.so.1` | `libxcursor1` | `libXcursor` | `libxcursor` |
| `libXext.so.6` | `libxext6` | `libXext` | `libxext` |
| `libXi.so.6` | `libxi6` | `libXi` | `libxi` |
| `libXinerama.so.1` | `libxinerama1` | `libXinerama` | `libxinerama` |
| `libXrandr.so.2` | `libxrandr2` | `libXrandr` | `libxrandr` |
| `libXrender.so.1` | `libxrender1` | `libXrender` | `libxrender` |
| `libXxf86vm.so.1` | `libxxf86vm1` | `libXxf86vm` | `libxxf86vm` |
| `libxkbcommon.so.0` | `libxkbcommon0` | `libxkbcommon` | `libxkbcommon` |
| `libwayland-client.so.0` | `libwayland-client0` | `libwayland-client` | `wayland` |
| `libwayland-cursor.so.0` | `libwayland-cursor0` | `libwayland-cursor` | `wayland` |
| `libwayland-egl.so.1` | `libwayland-egl1` | `libwayland-egl` | `wayland` |

The RPM requires these by soname. `libdecor` is optional and draws window decorations on GNOME Wayland.

For the plain binary, install the udev rule so the logged-in user can open the dongle without root, then replug it:

```
sudo install -m 644 core/hid/70-airpods-hid.rules /etc/udev/rules.d/
sudo udevadm control --reload
```

The rule tags the dongle's `hidraw` node with `uaccess`, which systemd turns into an ACL for the user with the active
session on the seat, so a shell over SSH alone does not get access. It has not yet been confirmed with a real dongle.

`build/linux/airpods-tray.desktop` is the launcher entry. `StartupWMClass` is the Wayland app ID, which Fyne sets from
the application ID. On X11 GLFW uses the window title, `AirPods`, as `WM_CLASS`, so the window may not group with its
launcher there.

## CI

`.github/workflows/build.yml` runs on a push to `main` and on `v*` tags. `test` and `govulncheck` run the script's
targets of those names, for the app, the command and the library together. `test-windows` runs the tests of the code
that needs no C compiler, the command and the library, on `windows-latest`. `build` builds the packages and the
installer and uploads them with the plain binaries. All actions are pinned to a commit, with the version in a comment.
Dependabot proposes updates to them and to the Go modules every week.

A tag starting with `v` also publishes a GitHub release with the binaries, packages, installer, `70-airpods-hid.rules`,
`airpods-tray.desktop` and `SHA256SUMS`, once every job has passed. A tag containing `alpha` or `rc` makes it a
prerelease. The release text starts with `build/release-notes.md`, which must not name a version, and GitHub generates
the list of changes after it.

Pushing a change to `.github/workflows/` with the `gh` token needs the `workflow` scope:
`gh auth refresh -h github.com -s workflow`.

## Trying the UI without a dongle

| Variable | Simulated dongle |
|---|---|
| `AIRPODS_GUI_SIM=1` | current firmware (0.1.66), with the routing handover, the reconnect policy, EN 352 protection, Gain swipe and an EQ report |
| `AIRPODS_GUI_SIM=disconnect` | the same, with a release that stays pending for about 10 s after Disconnect's 5 s wait, so the closing state shows |
| `AIRPODS_GUI_SIM=old` | firmware older than 0.1.59, with no routing handover: the Disconnect item shows the fallback message |
| `AIRPODS_GUI_SIM=legacy` | firmware 0.1.59 to 0.1.62: the handover, but no reconnect policy, no EN 352 protection or Gain swipe, and no EQ report |
| `AIRPODS_GUI_SIM=case` | both buds in the case, not streaming, so that the in-case check can start |
| `AIRPODS_GUI_SIM=autopause` | takes the buds out and puts them back every 15 s. Turn Auto-pause on in the `...` panel first |
| `AIRPODS_GUI_SIM=owner` | reports only some settings, so the page shows `–`, in Adaptive mode with no case battery |
| `AIRPODS_GUI_SIM=several` | two dongles to choose between in the `...` panel, and a third that cannot be opened |

The simulator's audio replies are fixed, so a codec change snaps back to what it reports.

The command has its own variable: `AIRPODS_CLI_SIM=1 airpods hearing status` uses a simulated dongle. See
[Try it without a dongle](CLI.md#try-it-without-a-dongle).

## Screenshots

`AIRPODS_SCREENS_DIR=dir go test -run 'Render.*Screens' ./internal/ui` renders the page and its dialogs to PNG with Fyne's
test driver. It needs no display. `AIRPODS_ICON_SHEET=file go test -run PreviewSheet ./internal/icon` renders every tray
icon state on both panels.

The images in `docs/images` come from a real window instead: the built binary with `AIRPODS_GUI_SIM`, under Xvfb with no
window manager, so that there is no title bar to crop. Use simulator values only in images that go in this repo.

## Hardware checklist

Run this on Windows 11 and on Linux, with one dongle plugged in unless a line says otherwise. The command line tool has
its own list in [CLI-TESTING.md](CLI-TESTING.md#hardware-checklist).

Tray:

- [ ] The tray icon appears and reads on light and dark taskbars, and the badge shows the lowest earbud percentage.
- [ ] The tooltip shows batteries and mode.
- [ ] The menu order and labels match [USER-GUIDE.md](USER-GUIDE.md#the-menu), and the current mode is checked.
- [ ] Each Noise Control row switches mode, and the check follows.
- [ ] Reconnect AirPods reconnects after the AirPods were away. On firmware 0.1.59 or newer it says `connected; the dongle has the AirPods`, or `taking over` with `Taking the AirPods over` on the card until they arrive. On older firmware it says `sent`.
- [ ] Reconnect while a phone is using the AirPods takes them, and the window says which outcome it got.
- [ ] Disconnect AirPods, on dongle firmware 0.1.59 or newer, shows `released` or `closing`, the menu shows `Not connected`, and the AirPods stay paired and do not come back by themselves under the Manual reconnect policy.
- [ ] After that, the card below About offers Reconnect, and Reconnect AirPods brings the AirPods back without pairing again.
- [ ] With the Automatic reconnect policy saved: after Disconnect, replugging the dongle brings the AirPods back by themselves, and with the AirPods away and not released, the dongle takes them back, also from a phone.
- [ ] Disconnect AirPods during a call is refused with the busy explanation, and the AirPods stay connected.
- [ ] On older dongle firmware, Disconnect AirPods shows the message as a notification and a dialog, and the AirPods stay connected and paired.
- [ ] Open settings and a click on the icon open the window. Quit ends the process.

Dongle handling:

- [ ] Unplugging shows `No dongle` with no error popups, and replugging recovers within a few seconds.
- [ ] Two dongles at once show `Choose a dongle` and a list in the `...` panel. The choice opens that one, the header and tray name it, and it is remembered after a restart.
- [ ] A dongle that cannot be opened shows its reason below the list, and is not offered.
- [ ] One dongle shows none of this, whatever was remembered. `--serial` with a wrong serial shows that it is not plugged in.
- [ ] With the CLI running next to the tray, idle polling does not disturb it.
- [ ] A setting changed while another client holds the channel shows `another app is controlling the dongle`.

Settings window:

- [ ] Status, peer, batteries and codec are correct.
- [ ] Turning a hearing mode on keeps the tray and the header up to date while it waits for the AirPods.
- [ ] Each setting shows the AirPods' value, and a change applies and is confirmed.
- [ ] Right after the AirPods connect, rows still waiting for their report say `Still arriving`, then fill in.
- [ ] AirPods Pro 3 with Allow Off on: right after they reconnect, Off from the page and from the tray works, with no `these AirPods did not accept this`.
- [ ] Hold actions apply to the correct ear.
- [ ] Pair AirPods asks first, and is blocked during USB audio, microphone or phone use. A busy answer, for example while the AirPods microphone is requested, shows the dialog that the pairing may be erased.
- [ ] Opening Connection reads the reconnect policy once, and the status poll never reads it (compare with `airpods reconnect-policy`). Read reads it again.
- [ ] Manual to Automatic asks first, then says `Saved and verified.`; the CLI reads the same policy, and it survives a replug. Choosing the saved policy sends nothing.
- [ ] The policy choice is disabled during playback or microphone use. While the CLI holds a settings session, a save says `Busy: ...` and nothing is saved.
- [ ] On dongle firmware older than 0.1.63 the policy says `Not available: ...`.
- [ ] Ear detection on, Auto-pause off, dongle firmware 0.1.65 or newer: taking a bud out during USB playback pauses the player once, and putting it back plays it once. Check on Windows and on Linux, with a browser and a desktop player.
- [ ] The same with Auto-pause on: the player is not paused twice, nothing plays with the buds out, and putting the bud back plays once.
- [ ] Windows with the system media controls unavailable and Auto-pause on: the app presses no media key.
- [ ] Linux: the dongle's Pause and Play keys reach the player through the desktop (they arrive as the separate Play and Pause keys, not Play/Pause).
- [ ] AirPods Pro 3 show EN 352 protection and Gain swipe with their values, AirPods Pro 2 only Gain swipe, and a change of each is confirmed.
- [ ] Pair phone opens the phone pairing window.
- [ ] A codec, bitrate and game mode change applies. Game mode status reads `Waiting for playback` without audio, and `On` once it is confirmed.
- [ ] The nearby finder shows signal, and stops when the window hides and when the app quits.
- [ ] Diagnostics counters and setting report ages update while the section is open.
- [ ] Copy status as JSON gives the same fields as `airpods --json status` and `settings`.
- [ ] Each Copy as JSON button gives the same fields as its command: `list`, `info --capabilities`, `hearing status`, `hardware status`, `hardware data`, `experimental transparency show` and `loud-sound-reduction get`.
- [ ] Copy as JSON Dongle list shows every attached dongle with two plugged in, including one that cannot be opened, and leaves the open one working.
- [ ] Copy as JSON Info keeps the serial `null`.
- [ ] Capture to file with each option writes the lines of `airpods diag`, the header and tray stay live during it, and Stop ends it.
- [ ] With Inspect signal on, inspection ends after finishing, stopping and quitting.
- [ ] Capture to file offers to replace an existing file only after asking, and a cancelled dialog starts nothing.
- [ ] The Model catalog in the `...` panel matches `airpods info --capabilities`, and scrolls within the window.
- [ ] Closing the window hides it to the tray.

Hearing, on AirPods Pro 2 or Pro 3, with **Experimental features** on. Keep the volume low and be ready to take the AirPods out:

- [ ] Hearing shows `set up` and report ages after Read that match `airpods hearing status`.
- [ ] Turning Hearing Aid or Media Assist on warns and asks first. Cancel sends nothing and the switch goes back. Turning one off asks nothing.
- [ ] A mode that is not set up is refused with its reason, and no question.
- [ ] Apply lists exactly what differs, next to the warning, and leaves Hearing Aid as it was, on or off.
- [ ] Amplification from -1.5 to 1.5 and Balance from -3 to 3: a value that would put one ear past 1.5 is refused with `Not changed: ...` and the slider goes back.
- [ ] Export writes a file `airpods hearing profile apply --file` accepts, and that applies no change.
- [ ] Import loads the editor and sends nothing until Apply. A file with an unknown field or a value out of range is refused, and the editor stays as it was.
- [ ] Ambient noise reduction and Own voice match `airpods hearing profile show`, and Custom Transparency's stored row matches `transparency show`.
- [ ] With Hearing Aid or Media Assist on, Custom Transparency can only be turned off. With both off, an amplification up to 2 applies and reads back.
- [ ] Loud Sound Reduction reads and changes on Pro 2 and Pro 3, and matches `airpods experimental loud-sound-reduction get`.
- [ ] EQ: Read shows On and the three bands as the AirPods report them. A change of one band keeps the others and says `Sent, and reported back by the AirPods.` Check that EQ on and off match what you hear and what an iPhone shows.

Starting:

- [ ] `--tray` starts hidden. A second plain start raises the window. A second `--tray` start does nothing visible.
- [ ] Start with the system survives a log out and log in, and starts hidden.
- [ ] Windows: the Run value reads `"<exe>" --tray`. Linux: the autostart file has the absolute path.
- [ ] Linux: the udev rule grants access without root.
- [ ] Linux: the tray works on KDE and on GNOME with the AppIndicator extension. Stock GNOME opens the window instead.
- [ ] Windows: the exe has its icon, and the window is sharp on a scaled display.

Windows installer:

- [ ] SmartScreen shows More info and Run anyway. No admin prompt appears, and files land in `%LOCALAPPDATA%\Programs\AirPods Tray`.
- [ ] The Start Menu shortcut and the entry in Add or Remove Programs have the icon, version, publisher and size.
- [ ] With Start with Windows ticked, the App tab shows the option on, not stale, and it survives a log out and log in.
- [ ] An upgrade while the app runs closes it through `--quit`, with no `taskkill`, and the settings stay.
- [ ] Uninstalling closes the app and removes files, shortcuts, the `Run` value and the PATH entry. Settings stay unless you choose to remove them.
- [ ] A default install has `airpods-tray.exe` and `airpods.exe`, and a terminal opened afterwards runs `airpods list` from any folder.
- [ ] With the app unticked, or `/S /CLIONLY`: only `airpods.exe`, no shortcuts, no Run to tick on the last page, and the entry in Add or Remove Programs still uninstalls it.
- [ ] `/S` over a command-line-only install keeps it that way. `/S` over an install from before the command line tool adds `airpods.exe` and the PATH entry.
- [ ] Unticking the command line tool on an upgrade removes `airpods.exe` and its PATH entry, and leaves the rest of PATH as it was, `%VARIABLES%` included.
- [ ] `/NOPATH` installs `airpods.exe` and leaves PATH alone.
