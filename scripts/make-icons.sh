#!/usr/bin/env bash
# Regenerates every logo-derived asset from build/logo.png, the one master.
# Needs ImageMagick 7 (`brew install imagemagick`). Run from anywhere:
#   scripts/make-icons.sh
set -euo pipefail

cd "$(dirname "$0")/.."

src=build/logo.png
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# The master is an opaque RGB image: a rounded square on a white backdrop with
# a faint drop shadow. Flood-fill the backdrop from the corners and edge middles
# (a generous fuzz also swallows the shadow), then trim to the artwork.
w=$(magick identify -format '%w' "$src")
h=$(magick identify -format '%h' "$src")
magick "$src" -alpha set -fuzz 30% -fill none \
  -draw "color 0,0 floodfill" -draw "color $((w - 1)),0 floodfill" \
  -draw "color 0,$((h - 1)) floodfill" -draw "color $((w - 1)),$((h - 1)) floodfill" \
  -draw "color $((w / 2)),$((h - 1)) floodfill" -draw "color 0,$((h / 2)) floodfill" \
  -trim +repage "$tmp/cut.png"

# square <size> <inner> <out>: fit the artwork inside <inner> px (no distortion)
# and centre it on a transparent <size> px canvas.
square() {
  magick "$tmp/cut.png" -resize "${2}x${2}" -background none -gravity center \
    -extent "${1}x${1}" "$3"
}

# macOS wants ~10% breathing room around the icon, so it matches other apps in
# the Dock. Wails builds the .icns from this file.
square 1024 840 build/appicon.png

# Windows draws the icon edge to edge, so no padding.
for s in 16 32 48 64 128 256; do square "$s" "$s" "$tmp/win-$s.png"; done
magick "$tmp"/win-{16,32,48,64,128,256}.png build/windows/icon.ico

# Desktop UI: title bar / onboarding (shown at <=128 px, so 256 covers 2x
# screens) and the window favicon.
square 256 256 frontend/src/assets/logo.png
square 64 64 frontend/public/favicon.png

# Device player page: favicon, touch icon and header, all from one file.
square 192 192 internal/adapters/httpapi/web/icon.png

# README banner.
square 256 256 docs/assets/logo.png
