#!/usr/bin/env bash
# Builds the standalone desktop apps (Electron, with their own bundled browser engine):
#   dist/inferno-wings-linux-x86_64.AppImage, dist/inferno-wings-linux-arm64.AppImage,
#   dist/inferno-wings-windows-setup.exe (installer), dist/inferno-wings-windows-portable.exe
# Needs Node.js 18+ and npm. The Windows installer step also needs Wine when building on Linux
# (set SKIP_WIN_INSTALLER=1 to build only the portable .exe).
set -euo pipefail
cd "$(dirname "$0")/electron"
npm ci --no-audit --no-fund
rm -rf out
npx electron-builder --linux AppImage --x64 --arm64
npx electron-builder --win portable --x64
if [ -z "${SKIP_WIN_INSTALLER:-}" ]; then npx electron-builder --win nsis --x64; fi
mkdir -p ../dist
cp out/inferno-wings-linux-*.AppImage out/inferno-wings-windows-*.exe ../dist/
rm -f ../dist/*__uninstaller*
(cd ../dist && sha256sum inferno-wings-* > SHA256SUMS)
ls -lh ../dist
