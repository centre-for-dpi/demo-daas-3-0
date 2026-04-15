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
export PUBLIC_HOST ESIGNET_PUBLIC_PORT INJIWEB_UI_PUBLIC_PORT

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

envsubst '${PUBLIC_HOST} ${ESIGNET_PUBLIC_PORT} ${INJIWEB_UI_PUBLIC_PORT}' \
    < "$TEMPLATE" > "$OUTPUT"

echo "rendered $OUTPUT with:"
echo "  PUBLIC_HOST=$PUBLIC_HOST"
echo "  ESIGNET_PUBLIC_PORT=$ESIGNET_PUBLIC_PORT"
echo "  INJIWEB_UI_PUBLIC_PORT=$INJIWEB_UI_PUBLIC_PORT"
