#!/bin/sh

name="VANS Developer"
engine=${1:?usage: sign-engine.sh path/to/vans-engine}

command -v security >/dev/null 2>&1 || exit 0
security find-identity -v -p codesigning 2>/dev/null | grep -q "\"$name\"" || exit 0
codesign -f -s "$name" -i vans-engine "$engine" 2>/dev/null ||
  echo "Could not sign $engine as \"$name\": macOS will ask for the login password when it reads the saved key." >&2
exit 0
