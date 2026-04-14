#!/usr/bin/env bash
# demo-airgap-verify.sh — prove TRUE air-gap cryptographic verification.
#
# Procedure (per verification-adapter/ARCHITECTURE.md §Air-gap):
#   1. Start a throwaway adapter with a fresh cache
#   2. Sync the issuer DID(s) we care about while the network is available
#      (fetches did:web documents / derives did:key keys / copies our local
#      in-process signer's did:key via /sync)
#   3. Export the cache DB and any pre-fetched JSON-LD contexts
#   4. Re-launch the adapter with `docker run --network none`, mount the
#      cache + contexts, and submit credentials to /verify-offline
#
# Expected results (from ARCHITECTURE.md §Test results §True air-gap):
#
#   SD-JWT (x5c in header) → SUCCESS / CRYPTOGRAPHIC  (self-contained cert)
#   LDP_VC (ldp)           → SUCCESS / TRUSTED_ISSUER (context fetch fails in
#                            air-gap; adapter falls back to structural valid.)
#
# This script tests the first scenario: issue an SD-JWT with x5c, sync the
# issuer, then verify it from an isolated container.
set -e
cd "$(dirname "$0")/.."

CACHE_HOST=${CACHE_HOST:-/tmp/adapter-airgap-cache}
mkdir -p "$CACHE_HOST"

# Pick a credential that has already been issued and is currently in the
# shared walletbag of the running server. The caller passes its document.
# For a fully automated run, use an SD-JWT with x5c or a pre-recorded sample.
SAMPLE_CRED_FILE=${SAMPLE_CRED_FILE:-}
if [ -z "$SAMPLE_CRED_FILE" ] || ! [ -f "$SAMPLE_CRED_FILE" ]; then
  echo "usage: SAMPLE_CRED_FILE=<path-to-cred-json-or-sdjwt> $0"
  echo
  echo "This script expects a credential file to submit to the air-gapped"
  echo "adapter. Examples:"
  echo "  SAMPLE_CRED_FILE=/tmp/cred.ldp.json ./scripts/demo-airgap-verify.sh"
  echo "  SAMPLE_CRED_FILE=/tmp/cred.sdjwt    ./scripts/demo-airgap-verify.sh"
  exit 1
fi

echo "== Step 1: populate the adapter cache (online) =="
docker run --rm -d --name adapter-cache-warmup \
  --network waltid_default \
  -v "$CACHE_HOST:/app/cache" \
  -p 18085:8085 \
  verification-adapter-adapter:latest > /dev/null
sleep 1

# Sync a well-known did:key issuer so the cache has at least one entry.
# The demo server's in-process LDP signer uses an ephemeral did:key that we
# can read from /api/capabilities or pass via SAMPLE_ISSUER_DID.
ISSUER_DID=${SAMPLE_ISSUER_DID:-}
if [ -n "$ISSUER_DID" ]; then
  curl -s -X POST http://localhost:18085/sync \
    -H 'Content-Type: application/json' \
    -d "{\"did\":\"$ISSUER_DID\"}" > /dev/null
  echo "  synced $ISSUER_DID"
fi

docker rm -f adapter-cache-warmup > /dev/null

echo "== Step 2: start adapter with --network none =="
# --network none gives the container no network interfaces AT ALL — no loopback
# to host, no DNS, no backends. Port forwarding doesn't apply, so we talk to
# the adapter via `docker exec` + the container's own loopback interface.
# (The adapter binds 127.0.0.1:8085 inside its own netns — isolated from host.)
docker run --rm -d --name adapter-airgap \
  --network none \
  -v "$CACHE_HOST:/app/cache" \
  -v "$SAMPLE_CRED_FILE:/app/cred.input:ro" \
  verification-adapter-adapter:latest > /dev/null

# Wait for the adapter to start listening inside the container.
for i in 1 2 3 4 5; do
  if docker exec adapter-airgap wget -q --spider http://localhost:8085/health 2>/dev/null; then
    break
  fi
  sleep 0.3
done

echo "== Step 3: submit credential to /verify-offline (from inside the air-gapped container) =="
docker exec adapter-airgap sh -c '
  wget -q -O - \
    --header="Content-Type: application/json" \
    --post-file=/app/cred.input \
    http://localhost:8085/verify-offline
' | python3 -m json.tool

echo
docker rm -f adapter-airgap > /dev/null
