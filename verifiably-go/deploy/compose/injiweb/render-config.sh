#!/usr/bin/env bash
# render-config.sh — renders docker/injiweb/config/mimoto-issuers-config.json
# from its .template by substituting PUBLIC_HOST, ESIGNET_PUBLIC_PORT, and
# INJIWEB_UI_PUBLIC_PORT. Docker Compose reads .env at compose-up time, but
# static JSON files mounted into containers aren't interpolated — this
# script fills that gap.
#
# Invoked by scripts/start-all.sh before `docker compose up -d`. Can also
# be run standalone after editing .env:
#
#     ./docker/injiweb/render-config.sh
#
# Idempotent — re-running overwrites the rendered file with current .env
# values. The .template file is the source of truth; never edit the
# rendered .json directly.

set -euo pipefail

cd "$(dirname "$0")"

ENV_FILE="../stack/.env"
if [[ -f "$ENV_FILE" ]]; then
    set -o allexport
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    set +o allexport
fi

: "${PUBLIC_HOST:=172.24.0.1}"
: "${ESIGNET_PUBLIC_PORT:=3005}"
: "${INJIWEB_UI_PUBLIC_PORT:=3004}"
: "${VERIFIABLY_HOSTS_PATTERN:=}"
: "${VERIFIABLY_PUBLIC_DOMAIN:=}"

# resolve_subdomain mirrors deploy.sh's resolve_slug — VERIFIABLY_SLUG_<NAME>
# overrides take precedence so the operator's custom labels (e.g.
# VERIFIABLY_SLUG_INJI_WEB=wallet → wallet.<domain>) flow through here too.
resolve_subdomain() {
    local default="$1"
    local upper var
    upper=$(printf '%s' "$default" | tr '[:lower:]-' '[:upper:]_')
    var="VERIFIABLY_SLUG_${upper}"
    if [[ -v "$var" ]] && [[ -n "${!var}" ]]; then
        printf '%s' "${!var}"
    else
        printf '%s' "$default"
    fi
}

# In subdomain mode the inji-web redirect_uri + esignet token endpoint
# point at the public subdomains (no port — Caddy fronts both on 443).
# In legacy mode they're the host:port form, unchanged from before.
# Without this fix Mimoto returns "No issuers found" because it can't
# reach the eSignet token endpoint at the legacy URL when the user is
# browsing via a public domain.
if [[ -n "$VERIFIABLY_HOSTS_PATTERN" && -n "$VERIFIABLY_PUBLIC_DOMAIN" ]]; then
    INJIWEB_REDIRECT_URI="https://$(resolve_subdomain inji-web).${VERIFIABLY_PUBLIC_DOMAIN}/redirect"
    ESIGNET_TOKEN_URL="https://$(resolve_subdomain esignet).${VERIFIABLY_PUBLIC_DOMAIN}/v1/esignet/oauth/v2/token"
else
    INJIWEB_REDIRECT_URI="http://${PUBLIC_HOST}:${INJIWEB_UI_PUBLIC_PORT}/redirect"
    ESIGNET_TOKEN_URL="http://${PUBLIC_HOST}:${ESIGNET_PUBLIC_PORT}/v1/esignet/oauth/v2/token"
fi
export PUBLIC_HOST ESIGNET_PUBLIC_PORT INJIWEB_UI_PUBLIC_PORT \
       INJIWEB_REDIRECT_URI ESIGNET_TOKEN_URL

TEMPLATE=config/mimoto-issuers-config.json.template
OUTPUT=config/mimoto-issuers-config.json

if [[ ! -f "$TEMPLATE" ]]; then
    echo "error: $TEMPLATE not found — run fetch-config.sh first?" >&2
    exit 1
fi

if ! command -v envsubst >/dev/null; then
    echo "error: envsubst not installed (apt install gettext-base)" >&2
    exit 1
fi

envsubst '${PUBLIC_HOST} ${ESIGNET_PUBLIC_PORT} ${INJIWEB_UI_PUBLIC_PORT} ${INJIWEB_REDIRECT_URI} ${ESIGNET_TOKEN_URL}' \
    < "$TEMPLATE" > "$OUTPUT"

echo "rendered $OUTPUT with:"
echo "  INJIWEB_REDIRECT_URI=$INJIWEB_REDIRECT_URI"
echo "  ESIGNET_TOKEN_URL=$ESIGNET_TOKEN_URL"
