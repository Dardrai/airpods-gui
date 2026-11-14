#!/bin/bash
# Run with no arguments.
set -euo pipefail
script="$(dirname "$0")/pkg-version.sh"
source "$script"

failed=0
check() { # version format want
  local got
  got=$(pkg_version "$1" "$2")
  if [ "$got" != "$3" ]; then
    echo "pkg_version $1 $2: got $got, want $3" >&2
    failed=1
  fi
  got=$(bash "$script" "$1" "$2")
  if [ "$got" != "$3" ]; then
    echo "pkg-version.sh $1 $2: got $got, want $3" >&2
    failed=1
  fi
}

# version               deb and rpm            arch
while read -r version tilde arch; do
  check "$version" deb "$tilde"
  check "$version" rpm "$tilde"
  check "$version" arch "$arch"
done <<'TABLE'
v0.0.2                  0.0.2                  0.0.2
0.0.2                   0.0.2                  0.0.2
v10.20.30               10.20.30               10.20.30
v0.0.2-alpha            0.0.2~alpha            0.0.2alpha
v0.1.0-beta             0.1.0~beta             0.1.0beta
v1.0.0-rc.1             1.0.0~rc.1             1.0.0rc.1
v1.0.0-rc_1             1.0.0~rc.1             1.0.0rc.1
v0.0.2-alpha-3-gabc     0.0.2~alpha.3.gabc     0.0.2alpha.3.gabc
v0.0.2-3-gabc           0.0.2+3.gabc           0.0.2+3.gabc
v0.0.2-3-gabc-dirty     0.0.2+3.gabc.dirty     0.0.2+3.gabc.dirty
abc1234                 0.0.0~git.abc1234      0.0.0git.abc1234
abc1234-dirty           0.0.0~git.abc1234.dirty 0.0.0git.abc1234.dirty
dev                     0.0.0~git.dev          0.0.0git.dev
v1.2                    0.0.0~git.1.2          0.0.0git.1.2
TABLE

if bash "$script" v1.0.0 2>/dev/null; then
  echo "pkg-version.sh with one argument should fail" >&2
  failed=1
fi

exit "$failed"
