#!/bin/sh
# Runs after install, upgrade and removal. Every step is optional: tools may be missing and
# a container has no udev daemon, so nothing here may fail the package operation.
set -e

if command -v udevadm >/dev/null 2>&1 && [ -S /run/udev/control ]; then
    udevadm control --reload-rules >/dev/null 2>&1 || true
    udevadm trigger --subsystem-match=hidraw --action=change >/dev/null 2>&1 || true
fi

if command -v gtk-update-icon-cache >/dev/null 2>&1 && [ -f /usr/share/icons/hicolor/index.theme ]; then
    gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor >/dev/null 2>&1 || true
fi

if command -v update-desktop-database >/dev/null 2>&1 && [ -d /usr/share/applications ]; then
    update-desktop-database -q /usr/share/applications >/dev/null 2>&1 || true
fi

true
