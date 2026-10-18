#!/bin/sh
set -eu

cd "$(dirname "$0")/../.."

version=${1:-}
case "$version" in
  [0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "Usage: infra/mac/bump.sh 0.2.0"; exit 1 ;;
esac

python3 -I - "$version" <<'PY'
import json, re, sys
version = sys.argv[1]
for path in ["ui/src-tauri/tauri.conf.json", "ui/package.json"]:
    with open(path) as f:
        text = f.read()
    text = re.sub(r'^(  "version": )"[^"]*"', r'\1"' + version + '"', text, count=1, flags=re.M)
    with open(path, "w") as f:
        f.write(text)
with open("ui/package-lock.json") as f:
    lock = json.load(f)
lock["version"] = version
lock["packages"][""]["version"] = version
with open("ui/package-lock.json", "w") as f:
    f.write(json.dumps(lock, indent=2) + "\n")
with open("ui/src-tauri/Cargo.toml") as f:
    text = f.read()
text = re.sub(r'^version = "[^"]*"', 'version = "' + version + '"', text, count=1, flags=re.M)
with open("ui/src-tauri/Cargo.toml", "w") as f:
    f.write(text)
PY
(cd ui/src-tauri && cargo update -q -p vans --precise "$version" 2>/dev/null || cargo metadata -q --format-version 1 >/dev/null)
git diff --stat
