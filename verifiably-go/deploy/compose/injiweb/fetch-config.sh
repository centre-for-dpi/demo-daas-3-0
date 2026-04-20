#!/usr/bin/env bash
# fetch-config.sh — pulls the canonical Inji Web / Mimoto config files from
# the upstream inji/inji-web repository into docker/injiweb/config/.
#
# We deliberately don't vendor these files into this repo because:
#   - they contain several file paths + URLs keyed to MOSIP's collab env,
#     which change between releases, and
#   - the upstream docker-compose layout is the authoritative reference
#     for Mimoto's Spring boot config.
#
# After this script runs you'll have:
#   mimoto_init.sql                     — Postgres schema for the
#                                         inji_mimoto database
#   mimoto-default.properties           — Spring config for Mimoto
#   mimoto-bootstrap.properties         — Spring cloud bootstrap
#   mimoto-issuers-config.json          — issuer catalog Inji Web loads
#   mimoto-trusted-verifiers.json       — OID4VP verifier allowlist
#   credential-template.html            — PDF rendering template
#   certs/oidckeystore.p12              — OIDC client keystore (empty
#                                         placeholder; you MUST replace
#                                         this with a real keystore
#                                         issued by whichever esignet
#                                         you front Mimoto with)
#
# Usage:
#   cd docker/injiweb
#   ./fetch-config.sh
#
# Requires: curl, install to PATH.

set -euo pipefail
cd "$(dirname "$0")"
mkdir -p config/certs

INJI_UPSTREAM="https://raw.githubusercontent.com/inji/inji-web/master/docker-compose"
ESIGNET_UPSTREAM="https://raw.githubusercontent.com/mosip/esignet/release-1.5.x/docker-compose"

fetch() {
    local base=$1 src=$2 dst=$3
    if [[ -f "$dst" ]]; then
        echo "  skip  $dst (already present — delete first to overwrite)"
        return
    fi
    echo "  fetch $base/$src"
    curl -fsSL "$base/$src" -o "$dst"
}

# --- Inji Web / Mimoto config ---
fetch "$INJI_UPSTREAM" mimoto_init.sql                          config/mimoto_init.sql
fetch "$INJI_UPSTREAM" config/mimoto-default.properties         config/mimoto-default.properties
fetch "$INJI_UPSTREAM" config/mimoto-bootstrap.properties       config/mimoto-bootstrap.properties
fetch "$INJI_UPSTREAM" config/mimoto-issuers-config.json        config/mimoto-issuers-config.json
fetch "$INJI_UPSTREAM" config/mimoto-trusted-verifiers.json     config/mimoto-trusted-verifiers.json
fetch "$INJI_UPSTREAM" config/credential-template.html          config/credential-template.html
# --- data-share-service config (required by Mimoto for credential delivery) ---
fetch "$INJI_UPSTREAM" config/data-share-inji-default.properties config/data-share-inji-default.properties
fetch "$INJI_UPSTREAM" config/data-share-standalone.properties   config/data-share-standalone.properties

# --- esignet + mock-identity-system schema ---
# esignet's init.sql creates mosip_esignet + mosip_mockidentitysystem
# databases alongside our existing inji_mimoto, all in the same Postgres
# server. The init scripts run in lexical order, so this file is
# mounted as 02-esignet_init.sql (after 01-mimoto_init.sql).
fetch "$ESIGNET_UPSTREAM" init.sql                            config/esignet_init.sql

# The p12 keystore is NOT distributed by upstream (and can't be — it holds
# the private key for Mimoto's OIDC client). Create an empty placeholder so
# the volume mount doesn't fail and print a loud warning.
if [[ ! -f config/certs/oidckeystore.p12 ]]; then
    touch config/certs/oidckeystore.p12
    cat <<'WARN'

------------------------------------------------------------------------
WARNING: docker/injiweb/config/certs/oidckeystore.p12 is an empty file.

Mimoto will start but cannot complete OIDC token exchanges with any
configured issuer until you replace it with a real PKCS12 keystore
whose alias matches client_alias in mimoto-issuers-config.json.

See https://docs.mosip.io/inji/inji-web/developer-setup for how to
generate one against your esignet.
------------------------------------------------------------------------

WARN
fi

# Mimoto's kernel keymanager creates its own master key aliases inside
# the mounted p12 on first boot. If we bind-mount the user's original
# p12 read-write, the original file gets rewritten. To keep the real
# oidckeystore.p12 pristine, copy it into a sibling `certs-runtime/`
# directory that Mimoto can write to. Re-running fetch-config.sh refreshes
# this copy from the original.
mkdir -p config/certs-runtime
cp -f config/certs/oidckeystore.p12 config/certs-runtime/oidckeystore.p12
echo "  copied config/certs/oidckeystore.p12 -> config/certs-runtime/oidckeystore.p12 (writable runtime copy)"

echo
echo "Done. Review config/mimoto-issuers-config.json and edit the issuer"
echo "entries to point at your local esignet + issuer endpoints (walt.id"
echo "issuer-api at http://localhost:7002 or inji-certify at"
echo "http://certify-nginx:80) before starting the container."
