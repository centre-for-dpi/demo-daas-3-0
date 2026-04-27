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

# WSO2 7's full startup (Carbon kernel + identity-server module + admin
# API) can take 5+ minutes on a small VPS even after port 9443 goes
# ready. Probe the admin API itself with a generous timeout — it returns
# 200 only after WSO2 is genuinely usable, so this is also a usability
# signal, not just a TCP-ready signal.
echo "waiting for WSO2IS at $WSO2_BASE…"
for i in $(seq 1 240); do
  if curl -sfk -o /dev/null "$WSO2_BASE/api/server/v1/applications?limit=1" \
       -u "$WSO2_ADMIN_USER:$WSO2_ADMIN_PASS"; then
    echo "  reachable"
    break
  fi
  sleep 2
  if [[ $i -eq 240 ]]; then
    echo "  WSO2IS admin API not reachable after 8 minutes — abort"
    echo "  hint: check 'docker logs waltid-wso2is-1 --tail 40' for boot errors"
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

# Self-heal check: confirm the client_secret in our hands actually
# authenticates against WSO2's token endpoint. WSO2 7's DCR can leave the
# OAuth-app side missing while the SP record persists (observed when an
# earlier registration partially failed and was followed by a GET-fallback
# branch); subsequent token requests then return:
#
#   {"error":"invalid_client",
#    "error_description":"A valid OAuth client could not be found for client_id"}
#   …or via the AS:
#   "no application could be found associated with the given consumer key"
#
# We probe with a client_credentials request — succeeds with 200 if WSO2
# recognises the client (even if scope is empty), 401 if not. On failure we
# DELETE the orphan SP and POST a fresh DCR registration so the OAuth app is
# created cleanly. Idempotent: a healthy client passes the probe and the
# heal branch is skipped.
echo "verifying client_secret authenticates with WSO2 token endpoint…"
TOKEN_PROBE_HTTP=$(curl -sk -o /dev/null -w '%{http_code}' \
  -u "$CLIENT_ID:$CLIENT_SECRET" \
  -X POST "$WSO2_BASE/oauth2/token" \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d 'grant_type=client_credentials' || echo '000')

if [[ "$TOKEN_PROBE_HTTP" != "200" && "$TOKEN_PROBE_HTTP" != "400" ]]; then
  # 200 = success. 400 = "unsupported_grant_type" or similar AFTER auth
  # succeeded — the client_id+secret pair was accepted, just the grant
  # isn't allowed. Anything else (401, 404) means auth failed.
  echo "  client_secret rejected (HTTP $TOKEN_PROBE_HTTP) — re-registering from scratch"
  curl -sk -u "$WSO2_ADMIN_USER:$WSO2_ADMIN_PASS" \
    -X DELETE "$WSO2_BASE/api/identity/oauth2/dcr/v1.1/register/$CLIENT_ID" \
    > /dev/null || true
  RESP=$(curl -sk -u "$WSO2_ADMIN_USER:$WSO2_ADMIN_PASS" \
    -H 'Content-Type: application/json' \
    -X POST "$WSO2_BASE/api/identity/oauth2/dcr/v1.1/register" \
    -d "$DCR")
  CLIENT_SECRET=$(echo "$RESP" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("client_secret",""))' 2>/dev/null || echo '')
  if [[ -z "$CLIENT_SECRET" ]]; then
    echo "  re-registration failed:" >&2
    echo "  $RESP" >&2
    exit 1
  fi
  echo "  re-registered with fresh client_secret"
else
  echo "  client_secret OK (HTTP $TOKEN_PROBE_HTTP)"
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
