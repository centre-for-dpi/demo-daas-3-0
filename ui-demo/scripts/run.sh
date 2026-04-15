#!/usr/bin/env bash
# run.sh — start the white-label server.
#
# One server, one brand (vc.infra), all DPG backends available simultaneously.
# Users pick their own issuer / wallet / verifier backend per-role during the
# onboarding wizard. No server-side mode switching.
#
# To swap the brand skin for a demo (e.g. show a Kenyan ministry what the app
# looks like in ke.vc colors), pass --config config/demo-kenya.json. The DPG
# backends are still all available — only the brand + theme changes.
set -e
cd "$(dirname "$0")/.."
set -a
. ./.env
set +a

CONFIG=${1:-config/default.json}

pkill -f "./server -config" 2>/dev/null || true
sleep 1

go build -o server ./cmd/server/

# Bring up the verification-adapter sidecar if not already running.
if ! docker ps --format '{{.Names}}' | grep -q '^vc-adapter$'; then
  (cd docker/waltid && docker compose up -d vc-adapter) || true
fi

# No DPG env var overrides: the handler picks the right backend per-request
# from each user's onboarding choice via h.issuerFor(user) / walletFor /
# verifierFor. The server instantiates ALL supported DPGs at startup.
INJI_CERTIFY_URL=http://localhost:8090 \
INJI_CERTIFY_UPSTREAM_URL=http://localhost:8090 \
INJI_CERTIFY_PUBLIC_URL=http://certify-nginx:80 \
INJI_VERIFY_URL=http://localhost:8082 \
VC_ADAPTER_URL=http://localhost:8085 \
./server -config "$CONFIG"
