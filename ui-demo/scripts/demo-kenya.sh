#!/usr/bin/env bash
# Launch the app in Kenya configuration: Walt.id as all three DPGs.
set -e
cd "$(dirname "$0")/.."
set -a
. ./.env
set +a

pkill -f "./server -config" 2>/dev/null || true
sleep 1

go build -o server ./cmd/server/

ISSUER_DPG=waltid \
WALLET_DPG=waltid \
VERIFIER_DPG=waltid \
./server -config config/demo-kenya.json
