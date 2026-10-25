#!/bin/bash
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
root="$(dirname "$here")"

name=""
if [ -f "$root/brand.py" ]; then
  python="$root/.venv/bin/python"
  [ -x "$python" ] || python="python3"
  name="$(cd "$root" && "$python" -I -c 'import sys; sys.path.insert(0, "."); import brand; print(brand.APP_NAME)' 2>/dev/null || true)"
fi
name="$(printf '%s' "$name" | tr -d '/:\n\r' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
[ -n "$name" ] || name="Voice Guide"

ident="$(printf '%s' "$name" | tr '[:upper:]' '[:lower:]' | tr -cd 'a-z0-9')"
[ -n "$ident" ] || ident="voiceguide"

echo "Building release..."
swift build -c release --package-path "$here"
binary="$(swift build -c release --package-path "$here" --show-bin-path)/GuideApp"

app="$here/build/$name.app"
rm -rf "$here/build/"*.app
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp "$binary" "$app/Contents/MacOS/GuideApp"

plist="$app/Contents/Info.plist"
/usr/libexec/PlistBuddy \
  -c "Add :CFBundleName string $name" \
  -c "Add :CFBundleDisplayName string $name" \
  -c "Add :CFBundleIdentifier string local.guideapp.$ident" \
  -c "Add :CFBundleExecutable string GuideApp" \
  -c "Add :CFBundlePackageType string APPL" \
  -c "Add :CFBundleShortVersionString string 0.1" \
  -c "Add :CFBundleVersion string 1" \
  -c "Add :CFBundleInfoDictionaryVersion string 6.0" \
  -c "Add :LSMinimumSystemVersion string 14.0" \
  -c "Add :LSUIElement bool true" \
  -c "Add :NSHighResolutionCapable bool true" \
  -c "Add :NSMicrophoneUsageDescription string $name listens while you hold the talk key, so it can hear your question." \
  "$plist" >/dev/null

codesign --force -s - "$app"
codesign --verify --verbose "$app"
echo "Built $app"
