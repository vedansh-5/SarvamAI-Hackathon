#!/bin/sh
set -eu

name="VANS Developer"
dir="$HOME/.vans-signing"
keychain="$HOME/Library/Keychains/login.keychain-db"
updater="$HOME/.tauri/vans-updater.key"

cd "$(dirname "$0")/../.."
mkdir -p "$dir" "$HOME/.tauri"
chmod 700 "$dir"

if security find-certificate -c "$name" "$keychain" >/dev/null 2>&1; then
  echo "The \"$name\" certificate is already in the login keychain."
else
  pass=$(/usr/bin/openssl rand -hex 16)
  /usr/bin/openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
    -subj "/CN=$name" \
    -addext "basicConstraints=critical,CA:false" \
    -addext "keyUsage=critical,digitalSignature" \
    -addext "extendedKeyUsage=critical,codeSigning" \
    -keyout "$dir/key.pem" -out "$dir/cert.pem" 2>/dev/null
  /usr/bin/openssl pkcs12 -export -inkey "$dir/key.pem" -in "$dir/cert.pem" \
    -name "$name" -out "$dir/vans-signing.p12" -passout "pass:$pass"
  printf '%s\n' "$pass" > "$dir/p12-password"
  chmod 600 "$dir"/*
  security import "$dir/vans-signing.p12" -k "$keychain" -P "$pass" -T /usr/bin/codesign
  echo "macOS will ask for your password once, to trust \"$name\" for signing code."
  security add-trusted-cert -p codeSign -k "$keychain" "$dir/cert.pem"
  rm -f "$dir/key.pem"
  echo "Made \"$name\". Back up $dir: without it, testers grant permissions again after the next update."
fi

if [ -f "$dir/codesign-allowed" ]; then
  echo "codesign already uses \"$name\" without asking."
else
  echo "Type your login password once more, so codesign can use \"$name\" at every build without asking."
  security set-key-partition-list -S apple-tool:,apple:,codesign: -s -l "$name" "$keychain" >/dev/null
  touch "$dir/codesign-allowed"
fi

if [ -f "$updater" ]; then
  echo "The update signing key is already at $updater."
else
  (cd ui && npx tauri signer generate --ci -p "" -w "$updater")
  chmod 600 "$updater"
  echo "Made the update signing key at $updater. Back it up too: without it, no copy already out can be updated."
fi

security find-identity -v -p codesigning | grep "$name"
