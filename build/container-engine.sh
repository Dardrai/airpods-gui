#!/bin/sh
# Picks the container engine for build/fyne-cross.sh.
#   container-engine.sh name           $CONTAINER_ENGINE, else docker if present, else podman
#   container-engine.sh user-args U:G  the flags that run a container as U:G, so that what it
#                                      writes to a mounted directory belongs to U:G on the host
set -eu

name() {
  if [ -n "${CONTAINER_ENGINE:-}" ]; then
    echo "$CONTAINER_ENGINE"
  elif command -v docker >/dev/null 2>&1; then
    echo docker
  else
    echo podman
  fi
}

# Rootless podman maps the container's users through the host user's own, so --user alone would
# leave the files with a subordinate id. keep-id maps the host user to the same id in the container.
rootless_podman() {
  "$1" --version 2>/dev/null | grep -qi podman || return 1
  [ "$("$1" info --format '{{.Host.Security.Rootless}}' 2>/dev/null)" = true ]
}

case "${1:-}" in
  name)
    name
    ;;
  user-args)
    user=${2:?usage: container-engine.sh user-args UID:GID}
    if rootless_podman "$(name)"; then
      echo "--userns=keep-id --user $user"
    else
      echo "--user $user"
    fi
    ;;
  *)
    echo "usage: container-engine.sh name | user-args UID:GID" >&2
    exit 2
    ;;
esac
