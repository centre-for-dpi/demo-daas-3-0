#!/usr/bin/env bash
# Run the existing e2e suite against a K8s deployment.
#
# The suite is already parameterized via VERIFIABLY_URL (see e2e/*.mjs
# headers); this wrapper just discovers the right URL from the cluster
# and re-uses the existing tests verbatim.
#
# Usage:
#   ./run-against-cluster.sh                   # auto-detect from kubectl
#   ./run-against-cluster.sh --url=...         # explicit override

set -euo pipefail

BASE_URL=""
for arg in "$@"; do
  case "$arg" in
    --url=*) BASE_URL="${arg#--url=}" ;;
  esac
done

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"

if [[ -z "$BASE_URL" ]]; then
  HOST=$(kubectl -n waltid get ingress -o jsonpath='{.items[?(@.metadata.name=="waltid-verifiably-go")].spec.rules[0].host}')
  [[ -n "$HOST" ]] || { echo "could not discover ingress host"; exit 1; }
  BASE_URL="https://$HOST"
fi
echo "▶ Running e2e against $BASE_URL"

# Smoke set — runs in CI's 20-minute budget. Full suite is opt-in via
# the FULL=1 env var (most tests rely on Inji-* which is out of scope
# for the waltid scenario).
SMOKE_TESTS=(
  auth-test.mjs
  i18n-test.mjs
  walk-portal.mjs
  matrix-test.mjs
)

cd "$ROOT"
fail=0
for t in "${SMOKE_TESTS[@]}"; do
  echo "── $t ──"
  if ! VERIFIABLY_URL="$BASE_URL" node "e2e/$t"; then
    fail=$((fail+1))
  fi
done

[[ $fail -eq 0 ]] && { echo "all smoke e2e green"; exit 0; }
echo "$fail test(s) failed" >&2
exit 1
