#!/bin/sh
set -eu

cd "$(dirname "$0")/../.."

profile=vans-deploy
region=ap-south-1
bucket=${VANS_RELEASES_BUCKET:-vans-releases}
name="VANS Developer"
updater="$HOME/.tauri/vans-updater.key"
target=universal-apple-darwin
notes=${1:-}

version=$(python3 -I -c 'import json;print(json.load(open("ui/src-tauri/tauri.conf.json"))["version"])')
case "$version" in
  [0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "$version is not a version like 0.2.0."; exit 1 ;;
esac
cargo=$(sed -n 's/^version = "\(.*\)"/\1/p' ui/src-tauri/Cargo.toml | head -1)
npm=$(python3 -I -c 'import json;print(json.load(open("ui/package.json"))["version"])')
if [ "$cargo" != "$version" ] || [ "$npm" != "$version" ]; then
  echo "The versions differ: tauri.conf.json $version, Cargo.toml $cargo, package.json $npm. Run infra/mac/bump.sh $version."
  exit 1
fi

if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
  echo "Commit or put away the changes first: a release is built from what is committed."
  exit 1
fi
git fetch -q origin main --tags
if ! git merge-base --is-ancestor HEAD origin/main; then
  echo "Release from main: this commit is not on origin/main."
  exit 1
fi
if git rev-parse -q --verify "refs/tags/v$version" >/dev/null; then
  echo "v$version is already tagged. Raise the version with infra/mac/bump.sh."
  exit 1
fi
if ! security find-identity -v -p codesigning | grep -q "\"$name\""; then
  echo "No \"$name\" certificate. Run infra/mac/signing-setup.sh first."
  exit 1
fi
if [ ! -f "$updater" ]; then
  echo "No update signing key. Run infra/mac/signing-setup.sh first."
  exit 1
fi
if aws s3api head-object --profile $profile --region $region --bucket "$bucket" --key "$version/VANS.app.tar.gz" >/dev/null 2>&1; then
  echo "$version is already released. Raise the version in ui/src-tauri/tauri.conf.json."
  exit 1
fi
rustup target list --installed | grep -q x86_64-apple-darwin || rustup target add x86_64-apple-darwin

sh infra/mac/basics.sh

server=$(aws lambda get-function-url-config --profile $profile --region $region --function-name vans-server --query FunctionUrl --output text)
server=${server%/}

out=$(mktemp -d)
engine="ui/vans-engine-$target"
arm="ui/vans-engine-aarch64-apple-darwin"
intel="ui/vans-engine-x86_64-apple-darwin"
trap 'rm -rf "$out" "$engine" "$arm" "$intel"' EXIT

flags="-s -w -X main.serverURL=$server"
CGO_ENABLED=1 GOARCH=arm64 go build -tags tester -trimpath -ldflags "$flags" -o "$out/arm64" ./cmd/vans-engine
CGO_ENABLED=1 GOARCH=amd64 CC="clang -arch x86_64" go build -tags tester -trimpath -ldflags "$flags" -o "$out/amd64" ./cmd/vans-engine
lipo -create -output "$engine" "$out/arm64" "$out/amd64"
cp "$out/arm64" "$arm"
cp "$out/amd64" "$intel"

(
  cd ui
  CI=true \
  VANS_UPDATER_PUBKEY="$(cat "$updater.pub")" \
  TAURI_SIGNING_PRIVATE_KEY="$(cat "$updater")" \
  TAURI_SIGNING_PRIVATE_KEY_PASSWORD="" \
  npx tauri build --target $target --config src-tauri/tauri.tester.conf.json \
    --config "{\"plugins\":{\"updater\":{\"pubkey\":\"$(cat "$updater.pub")\"}}}"
)

bundle="ui/src-tauri/target/$target/release/bundle"
app="$bundle/macos/VANS.app"
tarball="$bundle/macos/VANS.app.tar.gz"
dmg="$bundle/dmg/VANS_${version}_universal.dmg"

codesign --verify --deep --strict "$app"
codesign -dvv "$app" 2>&1 | grep -q "Authority=$name"
codesign -dvv "$app/Contents/MacOS/vans-engine" 2>&1 | grep -q "Authority=$name"
lipo -info "$app/Contents/MacOS/vans-engine" | grep -q "x86_64 arm64"
python3 -I infra/mac/check-bundle.py "$app"

latest="$out/latest.json"
python3 -I - "$version" "$notes" "$(cat "$tarball.sig")" > "$latest" <<'PY'
import datetime, json, sys
version, notes, signature = sys.argv[1:4]
build = {"key": f"{version}/VANS.app.tar.gz", "signature": signature}
print(json.dumps({
    "version": version,
    "notes": notes,
    "pub_date": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "platforms": {"darwin-aarch64": build, "darwin-x86_64": build},
}, indent=2))
PY

aws s3 cp --profile $profile --region $region "$tarball" "s3://$bucket/$version/VANS.app.tar.gz"
aws s3 cp --profile $profile --region $region "$dmg" "s3://$bucket/$version/VANS_$version.dmg"
aws s3 cp --profile $profile --region $region --content-type application/json "$latest" "s3://$bucket/latest.json"

git tag -a "v$version" -m "VANS $version${notes:+: $notes}"
git push -q origin "v$version"

echo "Released $version and tagged it v$version. Copies already out pick it up within the hour."
echo "For a new tester: add them in the console, then send summonvans.in/download and their code."
