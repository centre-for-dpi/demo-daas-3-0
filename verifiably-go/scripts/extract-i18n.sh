#!/usr/bin/env bash
# Extract every `{{t "..." $.Lang}}` string from templates/ and pre-populate
# locales/{fr,es}.json via LibreTranslate. Running this at build time means
# the UI never blocks on a translate round-trip at render time — the t() func
# finds every string already cached.
#
# Usage:
#   LIBRETRANSLATE_URL=http://localhost:5000 ./scripts/extract-i18n.sh
#
# Re-run whenever you add new `{{t}}` calls. Idempotent: existing cached
# translations are kept, only new strings are fetched.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$SCRIPT_DIR/.."
TEMPLATES_DIR="$ROOT/templates"
LOCALES_DIR="$ROOT/locales"
: "${LIBRETRANSLATE_URL:=http://localhost:5000}"

mkdir -p "$LOCALES_DIR"

# Extract all `{{t "..." ...}}` strings. Uses a single python pass so it
# handles escaped quotes, multi-line calls, and duplicates correctly.
# Heredoc is quoted ('PY') so the shell doesn't expand $1, $2, etc. inside
# the Python regex.
TEMPLATES_DIR="$TEMPLATES_DIR" \
LOCALES_DIR="$LOCALES_DIR" \
LIBRETRANSLATE_URL="$LIBRETRANSLATE_URL" \
python3 - <<'PY'
import json
import os
import re
import sys
import urllib.request
import urllib.error

TEMPLATES_DIR = os.environ["TEMPLATES_DIR"]
LOCALES_DIR = os.environ["LOCALES_DIR"]
BASE = os.environ["LIBRETRANSLATE_URL"]
TARGETS = ["fr", "es"]

# Match {{t "..." ... }} — the first string literal is the message.
# Greedy in the middle is fine; the outer {{...}} is not nested here.
T_CALL = re.compile(r'\{\{\s*t\s+"((?:[^"\\]|\\.)*)"[^}]*\}\}', re.DOTALL)

messages = set()
for root, _, files in os.walk(TEMPLATES_DIR):
    for f in files:
        if not f.endswith(".html"):
            continue
        path = os.path.join(root, f)
        with open(path) as fh:
            body = fh.read()
        for m in T_CALL.findall(body):
            # Unescape Go template string literal sequences.
            msg = m.encode().decode('unicode_escape')
            messages.add(msg)
print(f"found {len(messages)} distinct translatable strings")

def translate(q, target):
    data = json.dumps({"q": q, "source": "en", "target": target, "format": "text"}).encode()
    req = urllib.request.Request(
        BASE.rstrip("/") + "/translate",
        data=data,
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            return json.loads(r.read()).get("translatedText", "")
    except urllib.error.URLError as e:
        return ""

for target in TARGETS:
    path = os.path.join(LOCALES_DIR, target + ".json")
    existing = {}
    if os.path.exists(path):
        with open(path) as f:
            try:
                existing = json.load(f)
            except json.JSONDecodeError:
                existing = {}
    added = 0
    for msg in sorted(messages):
        if msg in existing:
            continue
        tr = translate(msg, target)
        if tr:
            existing[msg] = tr
            added += 1
    with open(path, "w") as f:
        json.dump(existing, f, indent=2, ensure_ascii=False, sort_keys=True)
    print(f"  {target}: +{added} new, {len(existing)} total → {path}")
PY
