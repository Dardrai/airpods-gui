#!/bin/sh
# Runs after install, upgrade and removal. A container has no udev daemon and udevadm may be
# missing, so nothing here may fail the package operation.
if command -v udevadm >/dev/null 2>&1 && [ -S /run/udev/control ]; then
    udevadm control --reload-rules >/dev/null 2>&1 || true
    udevadm trigger --subsystem-match=hidraw --action=change >/dev/null 2>&1 || true
fi
true
