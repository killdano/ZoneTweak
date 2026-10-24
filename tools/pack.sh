#!/bin/bash
# Build the Windows VR build (ZoneTweak, with Contraband inside) and zip it with its runtime
# DLLs, the manual and the how-to-play.
set -e
cd "$(dirname "$0")/.."
DLLS="${DLLS:-/home/claude/boundary-vr/dist/ZoneTweak}" # openxr_loader.dll and openvr_api.dll
OUT=dist/ZoneTweak
rm -rf "$OUT" && mkdir -p "$OUT"
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w" -o "$OUT/ZoneTweak.exe" .
cp "$DLLS"/openxr_loader.dll "$DLLS"/openvr_api.dll "$DLLS"/openxr_loader_LICENSE.txt "$DLLS"/openvr_api_LICENSE.txt "$OUT"/
[ -f art/cover.png ] && cp art/cover.png "$OUT"/
cp docs/MANUAL.txt "$OUT"/README.txt
cp docs/HOW-TO-PLAY.txt LICENSE "$OUT"/
cp sim/data/tuning.json "$OUT"/contraband-tuning.sample.json
(cd dist && rm -f ZoneTweak.zip && zip -qr ZoneTweak.zip ZoneTweak)
ls -la "$OUT" dist/ZoneTweak.zip
