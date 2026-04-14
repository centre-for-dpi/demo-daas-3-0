#!/usr/bin/env bash
# Launch the app in Trinidad configuration: Inji Certify as issuer,
# Walt.id as wallet + verifier (cross-DPG demonstration).
set -e
cd "$(dirname "$0")/.."
set -a
. ./.env
set +a

pkill -f "./server -config" 2>/dev/null || true
sleep 1

go build -o server ./cmd/server/

ISSUER_DPG=inji \
WALLET_DPG=inji \
VERIFIER_DPG=inji \
INJI_CERTIFY_URL=http://localhost:8090 \
INJI_CERTIFY_UPSTREAM_URL=http://localhost:8090 \
INJI_CERTIFY_PUBLIC_URL=http://certify-nginx:80 \
INJI_VERIFY_URL=http://localhost:8082 \
./server -config config/demo-trinidad.json
