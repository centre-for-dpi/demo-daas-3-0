#!/usr/bin/env bash
# demo-hybrid.sh — Cross-DPG demonstration.
#
# Issuer:   Walt.id (jwt_vc_json + sd-jwt + mso_mdoc via issuer-api; ldp_vc
#           via our in-process URDNA2015 signer)
# Wallet:   Walt.id Wallet (external wallet-api + shared local bag)
# Verifier: Verification Adapter (docker service vc-adapter — routes by DID
#           method to inji-verify / waltid-verifier; URDNA2015 canonicalization
#           for LDP_VC; x5c self-contained for SD-JWT)
#
# This proves the full cross-DPG matrix: credentials minted by any DPG
# can be verified by any compatible verifier through the adapter.
set -e
cd "$(dirname "$0")/.."
set -a
. ./.env
set +a

pkill -f "./server -config" 2>/dev/null || true
sleep 1

go build -o server ./cmd/server/

# The verification adapter runs as a docker-compose service (vc-adapter).
# Bring it up here if it isn't already running — it joins the waltid_default
# network so its backend URLs are internal docker hostnames.
if ! docker ps --format '{{.Names}}' | grep -q '^vc-adapter$'; then
  (cd docker/waltid && docker compose up -d vc-adapter)
fi

ISSUER_DPG=waltid \
WALLET_DPG=waltid \
VERIFIER_DPG=adapter \
VC_ADAPTER_URL=http://localhost:8085 \
./server -config config/demo-kenya.json
