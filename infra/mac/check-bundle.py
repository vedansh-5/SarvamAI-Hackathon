import glob
import os
import re
import sys

repo = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
bundle = sys.argv[1]

instructions = []
for path in sorted(glob.glob(os.path.join(repo, "internal", "synth", "*.go"))):
    if path.endswith("_test.go"):
        continue
    source = open(path, encoding="utf-8").read()
    for name, text in re.findall(r"const (\w*Prompt\w*) = `([^`]+)`", source, re.S):
        for line in text.splitlines():
            line = line.strip()
            if len(line) >= 40 and "%" not in line[:40]:
                instructions.append((name, line[:40].encode()))

secrets = []
for env in (os.path.join(repo, ".env"),):
    if not os.path.exists(env):
        continue
    for raw in open(env, encoding="utf-8"):
        key, _, value = raw.strip().partition("=")
        value = value.strip().strip("\"'")
        if re.search(r"KEY|TOKEN|SECRET|PASS|TESTERS|INVITE|GATE", key) and len(value) >= 12:
            secrets.append((key, value.encode()))

shapes = [
    ("an AWS access key", re.compile(rb"AKIA[0-9A-Z]{16}")),
    ("a Google key", re.compile(rb"AIza[0-9A-Za-z_\-]{35}")),
    ("a Groq key", re.compile(rb"gsk_[0-9A-Za-z]{40,}")),
    ("an OpenAI-style key", re.compile(rb"sk-[0-9A-Za-z_\-]{32,}")),
    ("a private key", re.compile(rb"-----BEGIN [A-Z ]*PRIVATE KEY-----")),
]

if not instructions:
    sys.exit("check-bundle: found no instructions to look for; the pattern no longer matches internal/synth")

found = []
files = 0
for root, _, names in os.walk(bundle):
    for name in names:
        path = os.path.join(root, name)
        if os.path.islink(path):
            continue
        data = open(path, "rb").read()
        files += 1
        where = os.path.relpath(path, bundle)
        for label, probe in instructions:
            if probe in data:
                found.append(f"{where}: text from {label}")
                break
        for key, value in secrets:
            if value in data:
                found.append(f"{where}: the value of {key} from .env")
        for label, shape in shapes:
            if shape.search(data):
                found.append(f"{where}: something shaped like {label}")

if found:
    print("check-bundle: the app must not ship these:")
    for f in sorted(set(found)):
        print("  " + f)
    sys.exit(1)
print(f"check-bundle: {files} files, no instructions ({len(instructions)} lines looked for), no keys ({len(secrets)} from .env, {len(shapes)} shapes)")
