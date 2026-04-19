#!/usr/bin/env bash
# CI agnosticism guard: vendor-specific strings must not leak into the UI
# layer. The core (handlers, templates, vctypes, backend) talks in generic
# terms; vendor names live only in config/backends.json, internal/adapters/*,
# and internal/auth/*.
#
# Exit non-zero if any violating match is found.

set -euo pipefail

cd "$(dirname "$0")/../.."

PATTERN='waltid|walt\.id|walt community|inji_|inji certify|inji verify|inji web|keycloak|wso2is|mimoto|esignet'

# Directories the rule inspects. vctypes, backend, cmd, internal/handlers, and
# templates must stay vendor-agnostic. internal/adapters/* and
# internal/auth/* are intentionally vendor-aware and NOT scanned.
SCAN_DIRS=(
  "cmd"
  "backend"
  "vctypes"
  "internal/handlers"
  "internal/httpx"
  "internal/store"
  "templates"
)

violations=0
for dir in "${SCAN_DIRS[@]}"; do
  if [[ ! -d "$dir" ]]; then continue; fi
  # -I skips binary files; case-insensitive so "WaltID"/"waltid" both catch.
  if matches=$(grep -riEn "$PATTERN" "$dir" --include='*.go' --include='*.html' --include='*.js' --include='*.md' 2>/dev/null); then
    if [[ -n "$matches" ]]; then
      echo "FAIL: vendor-specific strings found in $dir" >&2
      echo "$matches" >&2
      violations=$((violations + 1))
    fi
  fi
done

if [[ $violations -gt 0 ]]; then
  echo "" >&2
  echo "The UI core must remain vendor-agnostic. Vendor-specific strings belong" >&2
  echo "in internal/adapters/* or config/backends.json only." >&2
  exit 1
fi

echo "agnosticism OK — no vendor leaks in the UI core."
