Tray app, settings window and command line tool for the AirPods USB dongle, for Linux and Windows. This release follows dongle firmware 0.1.66.

What changed since the last beta:

- **The command line tool is part of this project now.** `airpods-cli` and the library `airpods-core` moved into this repository, and this release has both programs: `airpods-tray`, the app, and `airpods`, the command.
- **The installer and the `airpods-gui` packages install both.** On Windows, untick the app to install the command line tool alone, or run the installer with `/S /CLIONLY`; it also puts `airpods` on your PATH unless you untick that. On Linux, the `airpods-cli` packages hold the command alone, also for arm64.
- **`airpods-diag` is now `airpods diag`**, with the same flags and the same output. Change `airpods-diag` to `airpods diag` in scripts.
- On Linux, the `airpods-gui` package replaces an installed `airpods-cli` package, since it holds the same command. On Arch with `airpods-cli` installed, the in-app update cannot answer pacman's question: run `sudo pacman -U` on the new package once and answer yes.

What has been tested:

- **Real hardware so far: only `airpods status` and settings reads on Linux**, and an older build of the app on Arch Linux.
- **Everything else has run only on a simulator or in containers.** That includes all writes, control commands, the hearing, hardware and experimental features, `airpods diag` and all of Windows. The installer's new choices and its PATH entry have run under Wine only.

Before you install:

- Linux needs the udev rule for access without root. The Arch, Fedora and Debian/Ubuntu packages install it; see docs/BUILD.md. The plain binaries need `70-airpods-hid.rules` installed by hand.
- GNOME needs the AppIndicator extension for the tray icon.
- The Windows installer is unsigned, so Windows SmartScreen may warn; choose "More info", then "Run anyway".
- Hearing Aid and Media Assist change how sound is presented, including how loud it is, and have never run on real AirPods. Read the warnings in the user guide and in `docs/CLI.md` first. If a change sounds wrong or too loud, take the AirPods out, then turn the mode off.
- `experimental` commands may not work through the dongle, and need `--experimental`.

Run `airpods help` to get started with the command. `docs/PARITY.md` compares the commands with the app. Check downloads against `SHA256SUMS`.
