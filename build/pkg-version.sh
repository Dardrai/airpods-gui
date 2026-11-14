#!/bin/bash
#
# pkg-version.sh VERSION FORMAT prints the package version of VERSION for FORMAT: deb, rpm or arch.
# Source this file to call pkg_version VERSION FORMAT instead. A pre-release sorts before its release,
# and a build after a release tag sorts after it:
#   v0.0.2                 0.0.2
#   v0.0.2-alpha           0.0.2~alpha          (arch: 0.0.2alpha)
#   v0.0.2-alpha-3-gabc    0.0.2~alpha.3.gabc   (arch: 0.0.2alpha.3.gabc)
#   v0.0.2-3-gabc          0.0.2+3.gabc
#   abc1234                0.0.0~git.abc1234    (arch: 0.0.0git.abc1234)
pkg_version() {
  local name=${1#v} format=$2 core pre
  if [[ $name =~ ^([0-9]+\.[0-9]+\.[0-9]+)(-(.+))?$ ]]; then
    core=${BASH_REMATCH[1]} pre=${BASH_REMATCH[3]}
  else
    core=0.0.0 pre=git.$name
  fi
  pre=${pre//[^A-Za-z0-9.]/.}
  if [ -z "$pre" ]; then
    echo "$core"
  elif [[ $pre =~ ^[0-9]+\.g[0-9a-f]+(\.dirty)?$ ]]; then
    echo "$core+$pre"
  elif [ "$format" = arch ]; then
    echo "$core$pre" # pacman has no tilde; a trailing letter already sorts first.
  else
    echo "$core~$pre"
  fi
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  if [ $# -ne 2 ]; then
    echo "usage: pkg-version.sh VERSION FORMAT" >&2
    exit 2
  fi
  pkg_version "$1" "$2"
fi
