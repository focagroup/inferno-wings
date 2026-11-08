#!/usr/bin/env bash
# Builds the Inferno Wings launcher for Linux x86_64, Linux arm64 and Windows x86_64.
# Needs only a Go toolchain (1.22+). Pure Go, no cgo, so everything cross-compiles from any OS.
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p dist

build() {
  local goos=$1 goarch=$2 out=$3 extra=${4:-}
  echo "-> $out"
  CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath -ldflags "-s -w $extra" -o "dist/$out" .
}

build linux   amd64 inferno-wings-linux-x86_64
build linux   arm64 inferno-wings-linux-arm64
build windows amd64 inferno-wings-windows-x86_64.exe "-H=windowsgui"

(cd dist && sha256sum inferno-wings-* > SHA256SUMS)
ls -lh dist
