#!/bin/bash
# Runs the tests and builds the tray app and the command line tool in the fyne-cross images. Needs
# only Docker or podman: CONTAINER_ENGINE if set, else docker if present, else podman.
#   build/fyne-cross.sh test | vulncheck | cli | linux | package | windows | installer | all
# Binaries and packages land in dist/. Go caches live in named volumes, or with AIRPODS_CACHE_DIR
# set, in gomod/ and gocache/ under that host directory, which belong to the invoking user.
set -euo pipefail

target=${1:-all}
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
tag=${FYNE_CROSS_TAG:-v1.3.4}
# The Go that builds, tests and scans everything, newer than the image's own Go 1.26.10. The go
# command downloads it into the module cache once, checked against the checksum database.
go_toolchain=${GO_TOOLCHAIN:-go1.26.11}
winres=github.com/tc-hib/go-winres@v0.3.3
govulncheck=golang.org/x/vuln/cmd/govulncheck@v1.9.0
nfpm_image=docker.io/goreleaser/nfpm:v2.48.0
nsis_image=docker.io/library/debian:trixie-slim
version=${VERSION:-$(git -C "$root" describe --tags --always --dirty 2>/dev/null || echo dev)}

source "$root/build/pkg-version.sh"

engine=$(sh "$root/build/container-engine.sh" name)
read -r -a user_args <<< "$(sh "$root/build/container-engine.sh" user-args "$(id -u):$(id -g)")"

# The containers that install or build run as root, and hand what they write back with chown. In
# rootless podman, root in the container already is the invoking user, and that user's own id
# in there would map to a subordinate id on the host.
host_id="$(id -u):$(id -g)"
if [[ " ${user_args[*]} " == *" --userns=keep-id "* ]]; then
  host_id=0:0
fi

# Where the Go caches come from. Host directories can be saved and restored by CI.
cache_dir=${AIRPODS_CACHE_DIR:-}
if [ -n "$cache_dir" ]; then
  mkdir -p "$cache_dir/gomod" "$cache_dir/gocache"
  cache_dir=$(cd "$cache_dir" && pwd)
  cache_mounts=(-v "$cache_dir/gomod":/gomod -v "$cache_dir/gocache":/gocache)
  # On every way out, so that a failed run can still be saved and removed by the user. A
  # script that sets an EXIT trap of its own replaces this one, and has to call own_cache in it.
  # The downloaded Go toolchain is unpacked read-only despite -modcacherw.
  cache_prelude='own_cache() {
  chmod -R u+w /gomod/golang.org/toolchain@* 2>/dev/null || true
  chown -R "$HOST_ID" /gomod /gocache
}; trap own_cache EXIT'
else
  cache_mounts=(-v airpods-gomod:/gomod -v airpods-gocache:/gocache)
  cache_prelude='own_cache() { :; }'
fi

# Fails a run whose go command is not the pinned toolchain, and shows it in every log.
go_check='go version; [ "$(go env GOVERSION)" = "$GOTOOLCHAIN" ] || { echo "want $GOTOOLCHAIN" >&2; exit 1; }'

run() { # image-os cc-target-flags script
  local os=$1 target_flags=$2 script=$3
  "$engine" run --rm -i \
    "${cache_mounts[@]}" -v "$root":/src -w /src \
    -e GOMODCACHE=/gomod -e GOCACHE=/gocache -e ZIG_GLOBAL_CACHE_DIR=/gocache/zig \
    -e GOTOOLCHAIN="$go_toolchain" -e GOFLAGS="-buildvcs=false -modcacherw" -e CGO_ENABLED=1 -e GOARCH=amd64 -e GOOS="$os" \
    -e CC="zig cc $target_flags" -e CXX="zig c++ $target_flags" \
    -e VERSION="$version" -e HOST_ID="$host_id" -e WINRES="$winres" -e GOVULNCHECK="$govulncheck" \
    --entrypoint bash "docker.io/fyneio/fyne-cross-images:$tag-$os" -euo pipefail -c "$cache_prelude
$go_check
$script"
}

# The image's own shared libraries are newer than the target and are only used for their
# sonames at link time, so their symbol versions must not fail the link.
linux_flags='-target x86_64-linux-gnu.2.31 -isystem /usr/include -L/usr/lib/x86_64-linux-gnu -Wl,--allow-shlib-undefined'
windows_flags='-target x86_64-windows-gnu -Wdeprecated-non-prototype -Wl,--subsystem,windows'

do_test() {
  run linux "$linux_flags" '
    unformatted=$(gofmt -l cmd internal core)
    if [ -n "$unformatted" ]; then echo "gofmt: $unformatted"; exit 1; fi
    go vet ./...
    # The completion tests ask the shells themselves, and skip one that is missing. bash is here.
    if ! command -v fish >/dev/null || ! command -v zsh >/dev/null; then
      { apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends fish zsh >/dev/null; } ||
        echo "fish and zsh could not be installed: their completion tests are skipped" >&2
    fi
    # The window tests take minutes under the race detector, close to the default 10 min limit.
    go test -race -count=1 -timeout 30m ./...
  '
}

# The fyne image, since loading the packages needs the C headers of the libraries they link.
do_vulncheck() {
  run linux "$linux_flags" '
    GOBIN=/tmp/bin go install "$GOVULNCHECK"
    /tmp/bin/govulncheck ./...
    # The Windows side of the code that needs no C compiler: the library and the command line tool.
    GOOS=windows CGO_ENABLED=0 /tmp/bin/govulncheck ./cmd/airpods/... ./core/...
  '
}

# The command line tool is pure Go, so one image cross-builds it for every platform. The
# completion scripts come from the binary this image can run.
cli_targets="linux/amd64 windows/amd64"
do_cli() {
  run linux "$linux_flags" '
    mkdir -p dist/completions
    export CGO_ENABLED=0
    for target in '"$cli_targets"'; do
      os=${target%/*} arch=${target#*/} ext=
      if [ "$os" = windows ]; then ext=.exe; fi
      GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w -X main.version=$VERSION" \
        -o "dist/airpods-$os-$arch$ext" ./cmd/airpods
    done
    dist/airpods-linux-amd64 completion bash > dist/completions/airpods.bash
    dist/airpods-linux-amd64 completion fish > dist/completions/airpods.fish
    dist/airpods-linux-amd64 completion zsh > dist/completions/_airpods
    chown -R "$HOST_ID" dist
  '
}

do_linux() {
  run linux "$linux_flags" '
    mkdir -p dist
    # cgo adds -g after -s on the link line, so strip again through the linker.
    go build -trimpath -buildmode=pie -ldflags "-s -w -extldflags=-Wl,--strip-all -X main.version=$VERSION" \
      -o dist/airpods-tray ./cmd/airpods-tray
    rm -rf dist/icons
    CGO_ENABLED=0 go run ./cmd/genicon -png dist/airpods-tray.png -hicolor dist/icons/hicolor
    chown -R "$HOST_ID" dist
  '
}

# One package per format, with the tray app and the command line tool in it.
do_package() {
  do_linux
  do_cli
  # File names keep the tag's spelling: GitHub rewrites a tilde in a release asset name.
  local base="dist/airpods-gui_${version#v}"
  nfpm build/nfpm.yaml deb deb amd64 "${base}_amd64.deb"
  nfpm build/nfpm.yaml rpm rpm amd64 "$base.x86_64.rpm"
  nfpm build/nfpm.yaml archlinux arch amd64 "$base-x86_64.pkg.tar.zst"
  rm -rf "$root/dist/.stage"
}

# nfpm fills in variables in the package's name, version and architecture, not in file paths, so
# the command for that architecture goes to a fixed place first.
nfpm() { # config packager version-format architecture output
  mkdir -p "$root/dist/.stage"
  cp "$root/dist/airpods-linux-$4" "$root/dist/.stage/airpods"
  "$engine" run --rm "${user_args[@]}" -v "$root":/src -w /src \
    -e PKG_VERSION="$(pkg_version "$version" "$3")" -e PKG_ARCH="$4" \
    "$nfpm_image" package --config "$1" --packager "$2" --target "$5"
}

do_windows() {
  run windows "$windows_flags" '
    mkdir -p dist
    trap "rm -f cmd/airpods-tray/rsrc_windows_amd64.syso build/windows/app.ico; own_cache" EXIT
    GOOS=linux CGO_ENABLED=0 go run ./cmd/genicon -ico build/windows/app.ico
    GOOS=linux CGO_ENABLED=0 go run "$WINRES" make --in build/windows/winres.json \
      --arch amd64 --out cmd/airpods-tray/rsrc
    go vet ./...
    go build -trimpath -ldflags "-s -w -H=windowsgui -X main.version=$VERSION" \
      -o dist/airpods-tray.exe ./cmd/airpods-tray
    chown -R "$HOST_ID" dist
  '
}

do_installer() {
  do_windows
  do_cli
  # The Windows version resource takes four numbers; a pre-release shares its release's.
  local name=${version#v} numeric=0.0.0.0
  if [[ $name =~ ^([0-9]+\.[0-9]+\.[0-9]+) ]]; then numeric=${BASH_REMATCH[1]}.0; fi
  run windows "$windows_flags" '
    GOOS=linux CGO_ENABLED=0 go run ./cmd/genicon -ico build/windows/app.ico
    chown "$HOST_ID" build/windows/app.ico
  '
  "$engine" run --rm -i -v "$root":/src -w /src \
    -e DEBIAN_FRONTEND=noninteractive -e NAME="$name" -e NUMERIC="$numeric" -e HOST_ID="$host_id" \
    "$nsis_image" bash -euo pipefail -c '
      trap "rm -f build/windows/app.ico" EXIT
      apt-get update -qq
      apt-get install -y -qq --no-install-recommends nsis >/dev/null
      output="dist/airpods-gui-$NAME-setup-x64.exe"
      makensis -WX -V2 -DVERSION="$NAME" -DNUMERIC_VERSION="$NUMERIC" -DROOT=/src \
        -DICON=/src/build/windows/app.ico -DOUTFILE="/src/$output" build/windows/installer.nsi
      chown "$HOST_ID" "$output"
      ls -l "$output"
    '
}

case "$target" in
  test) do_test ;;
  vulncheck) do_vulncheck ;;
  cli) do_cli ;;
  linux) do_linux ;;
  package) do_package ;;
  pkg-version) for format in deb rpm arch; do echo "$format $(pkg_version "$version" "$format")"; done ;;
  windows) do_windows ;;
  installer) do_installer ;;
  all) do_test; do_package; do_installer ;;
  *) echo "usage: $0 test|vulncheck|cli|linux|package|pkg-version|windows|installer|all" >&2; exit 2 ;;
esac
