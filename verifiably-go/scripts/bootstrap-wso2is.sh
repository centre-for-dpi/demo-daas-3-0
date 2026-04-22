#!/usr/bin/env bash
# Register an OIDC client in WSO2IS via its Dynamic Client Registration API
# and write the generated client_id + client_secret to config/wso2is.env
# so deploy.sh can inject them into auth-providers.json.
#
# Idempotent. Re-running reuses the existing client (WSO2IS DCR returns the
# same client on the second registration with the same ext_param_client_id).
#
# WSO2IS 7 DCR constraints we bake in:
#   - client_id must match /[a-zA-Z0-9_]{15,30}/ (hyphens forbidden).
#   - "none" token_endpoint_auth_method is rejected by default, so the client
#     uses the default basic auth + client_secret — not a pure public client.
#     That's OK; our OIDC provider package forwards client_secret when set.
#
# Usage:
#   ./scripts/bootstrap-wso2is.sh
#   WSO2_ADMIN_USER=... WSO2_ADMIN_PASS=... ./scripts/bootstrap-wso2is.sh

set -euo pipefail

: "${WSO2_BASE:=https://localhost:9443}"
: "${WSO2_ADMIN_USER:=admin}"
: "${WSO2_ADMIN_PASS:=admin}"
: "${CLIENT_ID:=verifiably_go_client}"
: "${CLIENT_NAME:=Verifiably Go}"
# Accept any of the hosts deploy.sh might bind to: localhost for laptop
# dev, PUBLIC_HOST (docker bridge IP or EC2 hostname) for published browser
# access, and 172.24.0.1 as a belt-and-braces bridge fallback. WSO2 collapses
# an array of redirect_uris into a "regexp=(...|...)" value on its side, so
# all three stay acceptable callbacks.
: "${PUBLIC_HOST:=localhost}"
: "${VERIFIABLY_HOST_PORT:=8080}"
_callback_local="http://localhost:${VERIFIABLY_HOST_PORT}/auth/callback"
_callback_public="http://${PUBLIC_HOST}:${VERIFIABLY_HOST_PORT}/auth/callback"
_callback_bridge="http://172.24.0.1:${VERIFIABLY_HOST_PORT}/auth/callback"
: "${REDIRECT_URIS_JSON:=[\"$_callback_local\",\"$_callback_public\",\"$_callback_bridge\"]}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT="$SCRIPT_DIR/../config/wso2is.env"

echo "waiting for WSO2IS at $WSO2_BASE…"
for i in $(seq 1 60); do
  if curl -sfk -o /dev/null "$WSO2_BASE/api/server/v1/applications?limit=1" \
       -u "$WSO2_ADMIN_USER:$WSO2_ADMIN_PASS"; then
    echo "  reachable"
    break
  fi
  sleep 2
  if [[ $i -eq 60 ]]; then
    echo "  WSO2IS admin API not reachable after 2 minutes — abort"
    exit 1
  fi
done

echo "registering OIDC client via DCR (client_id=$CLIENT_ID)…"
DCR=$(cat <<JSON
{
  "client_name": "$CLIENT_NAME",
  "grant_types": ["authorization_code", "refresh_token"],
  "redirect_uris": $REDIRECT_URIS_JSON,
  "ext_param_client_id": "$CLIENT_ID",
  "ext_pkce_mandatory": false
}
JSON
)
RESP=$(curl -sk -u "$WSO2_ADMIN_USER:$WSO2_ADMIN_PASS" \
  -H 'Content-Type: application/json' \
  -X POST "$WSO2_BASE/api/identity/oauth2/dcr/v1.1/register" \
  -d "$DCR")

CLIENT_SECRET=$(echo "$RESP" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("client_secret",""))' 2>/dev/null || echo '')

if [[ -z "$CLIENT_SECRET" ]]; then
  # Already registered? Look it up.
  echo "  no client_secret in response, looking up existing registration…"
  EXISTING=$(curl -sk -u "$WSO2_ADMIN_USER:$WSO2_ADMIN_PASS" \
    "$WSO2_BASE/api/identity/oauth2/dcr/v1.1/register/$CLIENT_ID")
  CLIENT_SECRET=$(echo "$EXISTING" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("client_secret",""))' 2>/dev/null || echo '')
  if [[ -z "$CLIENT_SECRET" ]]; then
    echo "  FAILED to register or fetch existing client:"
    echo "  register response: $RESP" >&2
    echo "  lookup response: $EXISTING" >&2
    exit 1
  fi
  echo "  existing client found"
  # Existing registration may have been created with a stale PUBLIC_HOST.
  # Push the current redirect_uris so a later deploy with a different
  # PUBLIC_HOST picks up the new callback.
  curl -sk -u "$WSO2_ADMIN_USER:$WSO2_ADMIN_PASS" \
    -H 'Content-Type: application/json' \
    -X PUT "$WSO2_BASE/api/identity/oauth2/dcr/v1.1/register/$CLIENT_ID" \
    -d "{\"client_name\":\"$CLIENT_NAME\",\"redirect_uris\":$REDIRECT_URIS_JSON,\"grant_types\":[\"authorization_code\",\"refresh_token\"]}" \
    > /dev/null && echo "  updated redirect_uris on existing client"
fi

mkdir -p "$(dirname "$OUT")"
cat > "$OUT" <<EOF
# Written by scripts/bootstrap-wso2is.sh. deploy.sh's auth-providers generator
# reads this to populate clientId + clientSecret on the wso2is entry.
WSO2_CLIENT_ID=$CLIENT_ID
WSO2_CLIENT_SECRET=$CLIENT_SECRET
EOF
chmod 600 "$OUT"
echo "  wrote $OUT (client_id=$CLIENT_ID, client_secret=***)"
