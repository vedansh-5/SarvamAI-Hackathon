#!/bin/sh
set -eu

cd "$(dirname "$0")/../.."
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT

step() { printf '\n== %s\n' "$1"; }

step "The Mac-only code builds and its tests pass"
CGO_ENABLED=1 go vet ./...
CGO_ENABLED=1 go test -count=1 ./...

step "The tester engine carries no instructions"
mkdir -p "$out/tester.app"
CGO_ENABLED=1 go build -tags tester -trimpath -ldflags "-s -w" -o "$out/tester.app/vans-engine" ./cmd/vans-engine
python3 -I infra/mac/check-bundle.py "$out/tester.app"

step "The panel type-checks"
(cd ui && npx tsc --noEmit)

if [ "${1:-}" != "--screen" ]; then
  printf '\nDone. Run with --screen to also check what needs a window (it takes the screen for a few seconds).\n'
  exit 0
fi

step "Selected text is read, from a text box that only gives it through Copy too (a test window opens)"
(cd internal/uiact && CGO_ENABLED=1 go test -tags integration -count=1 -v -run TestLiveTheSelectionIsReadInEveryKindOfTextBox .)

step "Only the panel, the coach and the dot are on screen, nothing wide in the menu bar strip"
cat > "$out/wins.swift" <<'S'
import CoreGraphics
for w in CGWindowListCopyWindowInfo([.optionOnScreenOnly], kCGNullWindowID) as! [[String: Any]]
where (w[kCGWindowOwnerName as String] as? String ?? "").lowercased().contains("vans") {
  print(w[kCGWindowLayer as String] ?? "", w[kCGWindowBounds as String] ?? "")
}
S
swift "$out/wins.swift"

printf '\nDone. The rest of AGY.md G11 is a look, not a command: go down the list.\n'
