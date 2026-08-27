#!/usr/bin/env bash
# Build UtauTTS GUI + native library on macOS.
#
# The Go c-shared dylib ships with a bare install name, which dyld cannot
# resolve on macOS. We rewrite it to @rpath BEFORE cmake links the GUI binary
# so the recorded load command is correct, then add an @loader_path rpath.
#
# Requirements: Go (on PATH or pointed at via UTAUTTS_GO_BIN), Qt 6.5+,
# CMake, Ninja, and Xcode command line tools. Homebrew paths are detected
# automatically when Homebrew is installed.
#
# Optional environment overrides:
#   UTAUTTS_GO_BIN      path to the go binary       (default: `go` on PATH)
#   UTAUTTS_GOCACHE     Go build cache directory    (default: build/go-cache)
#   UTAUTTS_GOMODCACHE  Go module cache directory   (default: build/go-modcache)
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Locate the Go toolchain: PATH first, then common Homebrew/local installs.
go_bin="${UTAUTTS_GO_BIN:-}"
if [ -z "${go_bin}" ]; then
  for candidate in "$(command -v go || true)" \
      /usr/local/go/bin/go \
      /opt/homebrew/bin/go \
      "${HOME}/go/bin/go"; do
    if [ -n "${candidate}" ] && [ -x "${candidate}" ]; then
      go_bin="${candidate}"
      break
    fi
  done
fi
if [ -z "${go_bin}" ] || [ ! -x "${go_bin}" ]; then
  echo "Go toolchain not found. Install Go or set UTAUTTS_GO_BIN." >&2
  exit 1
fi
export PATH="$(dirname "${go_bin}"):/opt/homebrew/bin:/usr/local/bin:${PATH}"
export GOPATH="${GOPATH:-${root_dir}/build/go-home}"
export GOMODCACHE="${UTAUTTS_GOMODCACHE:-${root_dir}/build/go-modcache}"
export GOCACHE="${UTAUTTS_GOCACHE:-${root_dir}/build/go-cache}"
export CGO_ENABLED=1

cd "${root_dir}"

echo '=== Build native library ==='
go build -trimpath -buildmode=c-shared -o build/native/libutautts_native.dylib ./cmd/utautts-native
install_name_tool -id @rpath/libutautts_native.dylib build/native/libutautts_native.dylib

echo '=== Build server and CLI tools ==='
go build -trimpath -o build-test/utautts-server ./cmd/utautts-server
go build -trimpath -o build-test/utautts-cli ./cmd/utautts-cli
go build -trimpath -o build-test/utautts-ustx ./cmd/tools/utautts-ustx

echo '=== Build Qt GUI (links @rpath/libutautts_native.dylib) ==='
cmake --build build/qt-mac

echo '=== Fix runtime library lookup ==='
cp build/native/libutautts_native.dylib build/qt-mac/app/libutautts_native.dylib
install_name_tool -id @rpath/libutautts_native.dylib build/qt-mac/app/libutautts_native.dylib
install_name_tool -add_rpath @loader_path build/qt-mac/app/utautts 2>/dev/null || true

echo '=== Build UtauTTS.app bundle ==='
bundle="build/UtauTTS.app"
rm -rf "${bundle}" build/utautts.iconset
mkdir -p "${bundle}/Contents/MacOS" "${bundle}/Contents/Resources" build/utautts.iconset
cp build/qt-mac/app/utautts build/qt-mac/app/libutautts_native.dylib "${bundle}/Contents/MacOS/"
# Symlink the project's resource directories into Contents/Resources so the
# app keeps working when copied to /Applications (absolute symlinks follow
# the project wherever it lives).
for dir in voice models plugins runtime; do
  ln -s "${root_dir}/${dir}" "${bundle}/Contents/Resources/${dir}"
done
cat > "${bundle}/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key><string>UtauTTS</string>
    <key>CFBundleDisplayName</key><string>UtauTTS</string>
    <key>CFBundleIdentifier</key><string>local.utautts.desktop</string>
    <key>CFBundleVersion</key><string>0.0.12</string>
    <key>CFBundleShortVersionString</key><string>0.0.12</string>
    <key>CFBundlePackageType</key><string>APPL</string>
    <key>CFBundleExecutable</key><string>utautts</string>
    <key>CFBundleIconFile</key><string>utautts</string>
    <key>LSMinimumSystemVersion</key><string>13.0</string>
    <key>NSHighResolutionCapable</key><true/>
    <key>LSApplicationCategoryType</key><string>public.app-category.utilities</string>
</dict>
</plist>
PLIST
sips -z 16 16 icons/icon32.png --out build/utautts.iconset/icon_16x16.png >/dev/null
cp icons/icon32.png build/utautts.iconset/icon_16x16@2x.png
cp icons/icon32.png build/utautts.iconset/icon_32x32.png
cp icons/icon64.png build/utautts.iconset/icon_32x32@2x.png
cp icons/icon128.png build/utautts.iconset/icon_128x128.png
sips -z 256 256 icons/icon512.png --out build/utautts.iconset/icon_128x128@2x.png >/dev/null
sips -z 256 256 icons/icon512.png --out build/utautts.iconset/icon_256x256.png >/dev/null
cp icons/icon512.png build/utautts.iconset/icon_256x256@2x.png
cp icons/icon512.png build/utautts.iconset/icon_512x512.png
cp icons/icon512.png build/utautts.iconset/icon_512x512@2x.png
iconutil -c icns build/utautts.iconset -o "${bundle}/Contents/Resources/utautts.icns"
rm -rf build/utautts.iconset

echo '=== Done ==='
ls -lh build/qt-mac/app/utautts "${bundle}/Contents/MacOS/utautts" "${bundle}/Contents/Resources/utautts.icns"
