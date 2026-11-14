Tray app, settings window and command line tool for the AirPods USB dongle, for Linux and Windows. This release follows dongle firmware 0.1.66.

One download per system, and each installs both programs: `airpods-tray`, the app, and `airpods`, the command line tool.

- **Linux:** the package for your distribution. It also installs the udev rule, the launcher and shell completions.
- **Windows:** the installer. Untick the app to install the command line tool alone, or run it with `/S /CLIONLY`. It puts `airpods` on your PATH unless you untick that.

Coming from the betas:

- `airpods-diag` is now `airpods diag`, with the same flags and the same output.
- The package replaces an installed `airpods-cli` package. On Arch with `airpods-cli` installed, the in-app update cannot answer pacman's question: run `sudo pacman -U` on the new package once and answer yes.

What has been tested:

- **Real hardware so far: only `airpods status` and settings reads on Linux**, and an older build of the app on Arch Linux.
- **Everything else has run only on a simulator or in containers.** That includes all writes, control commands, the hearing, hardware and experimental features, `airpods diag` and all of Windows. The installer has run under Wine only.

Before you install:

- GNOME needs the AppIndicator extension for the tray icon.
- The Windows installer is unsigned, so Windows SmartScreen may warn; choose "More info", then "Run anyway".
- Hearing Aid and Media Assist change how sound is presented, including how loud it is, and have never run on real AirPods. Read the warnings in the user guide and in `docs/CLI.md` first. If a change sounds wrong or too loud, take the AirPods out, then turn the mode off.

Run `airpods help` to get started with the command. Check downloads against `SHA256SUMS`.
