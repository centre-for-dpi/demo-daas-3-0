#!/usr/bin/env bash
# Ensure the vcplatform OIDC client in Keycloak has a redirect URI matching
# the current ${PUBLIC_HOST} on every ./deploy.sh up. Mirrors the role of
# bootstrap-wso2is.sh.
#
# Why this exists: Keycloak imports realms from /opt/keycloak/data/import/
# only when the realm doesn't already exist (Strategy: IGNORE_EXISTING).
# Once vcplatform is in the H2 DB, edits to keycloak-realm.json are stranded.
# Worse: Keycloak does NOT support `*` as a hostname wildcard in
# redirectUris — `*` only matches a path component. So the wildcards we
# inherited from upstream (http://*:8080/*, https://*/*) are functionally
# inert; only literal hostnames work. When the operator changes
# VERIFIABLY_PUBLIC_HOST and re-deploys, the new host's callback URL has
# to land in the client's redirectUris list explicitly or every login
# fails with "Invalid parameter: redirect_uri".
#
# This script reaches Keycloak's Admin REST API (auth via the master-realm
# admin user) and adds — set-union, not replace — three callback URIs:
#   - http://${PUBLIC_HOST}:${VERIFIABLY_HOST_PORT}/* (browser-direct on EC2)
#   - http://localhost:${VERIFIABLY_HOST_PORT}/*      (laptop dev fallback)
#   - https://${PUBLIC_HOST}/*                        (Caddy/HTTPS-fronted)
# Existing entries (literal localhost, 172.24.0.1, etc.) survive — set union
# means a host change is additive, not destructive.
#
# Idempotent: a second run finds the URIs already present and is a no-op.
#
# Required env (with defaults):
#   KEYCLOAK_BASE             http://localhost:8180
#   KEYCLOAK_REALM            vcplatform
#   KEYCLOAK_CLIENT_ID        vcplatform
#   KEYCLOAK_ADMIN_USER       admin
#   KEYCLOAK_ADMIN_PASS       admin
#   PUBLIC_HOST               (no default — script aborts if unset and
#                              VERIFIABLY_PUBLIC_HOST also unset)
#   VERIFIABLY_HOST_PORT      8080
#
# Usage:
#   ./scripts/bootstrap-keycloak.sh
#   PUBLIC_HOST=ec2-1-2-3-4.compute.amazonaws.com ./scripts/bootstrap-keycloak.sh

set -euo pipefail

: "${KEYCLOAK_BASE:=http://localhost:8180}"
: "${KEYCLOAK_REALM:=vcplatform}"
: "${KEYCLOAK_CLIENT_ID:=vcplatform}"
: "${KEYCLOAK_ADMIN_USER:=admin}"
: "${KEYCLOAK_ADMIN_PASS:=admin}"
: "${VERIFIABLY_HOST_PORT:=8080}"
: "${PUBLIC_HOST:=${VERIFIABLY_PUBLIC_HOST:-}}"

if [[ -z "$PUBLIC_HOST" ]]; then
  echo "  bootstrap-keycloak: PUBLIC_HOST / VERIFIABLY_PUBLIC_HOST unset — abort"
  exit 1
fi

echo "waiting for Keycloak at $KEYCLOAK_BASE…"
for i in $(seq 1 60); do
  if curl -sf -o /dev/null "$KEYCLOAK_BASE/realms/$KEYCLOAK_REALM/.well-known/openid-configuration"; then
    echo "  reachable"
    break
  fi
  sleep 2
  if [[ $i -eq 60 ]]; then
    echo "  Keycloak not reachable after 2 minutes — abort"
    exit 1
  fi
done

# Master-realm admin token. admin-cli is a built-in public client, no
# secret needed.
TOKEN=$(curl -sS -X POST \
  "$KEYCLOAK_BASE/realms/master/protocol/openid-connect/token" \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d "grant_type=password&client_id=admin-cli&username=$KEYCLOAK_ADMIN_USER&password=$KEYCLOAK_ADMIN_PASS" \
  | python3 -c 'import json,sys; print(json.load(sys.stdin).get("access_token",""))')

if [[ -z "$TOKEN" ]]; then
  echo "  failed to obtain master-realm admin token (user/pass wrong?)"
  exit 1
fi

# Look up the vcplatform client by clientId (different from the UUID
# Keycloak uses internally — Admin API needs the UUID for PUT).
CLIENT_UUID=$(curl -sS -H "Authorization: Bearer $TOKEN" \
  "$KEYCLOAK_BASE/admin/realms/$KEYCLOAK_REALM/clients?clientId=$KEYCLOAK_CLIENT_ID" \
  | python3 -c 'import json,sys; arr=json.load(sys.stdin); print(arr[0]["id"]) if arr else print("")')

if [[ -z "$CLIENT_UUID" ]]; then
  echo "  client $KEYCLOAK_CLIENT_ID not found in realm $KEYCLOAK_REALM — was the realm imported?"
  exit 1
fi

# Pull current client config, fold new URIs into redirectUris + webOrigins,
# PUT back. Set-union via Python so re-runs don't keep growing the list.
CURRENT=$(curl -sS -H "Authorization: Bearer $TOKEN" \
  "$KEYCLOAK_BASE/admin/realms/$KEYCLOAK_REALM/clients/$CLIENT_UUID")

UPDATED=$(PUBLIC_HOST="$PUBLIC_HOST" VERIFIABLY_HOST_PORT="$VERIFIABLY_HOST_PORT" \
  python3 -c '
import json, os, sys
host = os.environ["PUBLIC_HOST"]
port = os.environ["VERIFIABLY_HOST_PORT"]

client = json.load(sys.stdin)

want_redirect = {
    f"http://{host}:{port}/*",
    f"https://{host}/*",
    f"http://localhost:{port}/*",
}
want_origins = {
    f"http://{host}:{port}",
    f"https://{host}",
    f"http://localhost:{port}",
}

existing_redirect = set(client.get("redirectUris") or [])
existing_origins  = set(client.get("webOrigins") or [])

client["redirectUris"] = sorted(existing_redirect | want_redirect)
client["webOrigins"]   = sorted(existing_origins  | want_origins)

print(json.dumps(client))
' <<<"$CURRENT")

curl -sS -X PUT -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  "$KEYCLOAK_BASE/admin/realms/$KEYCLOAK_REALM/clients/$CLIENT_UUID" \
  -d "$UPDATED" > /dev/null

echo "  $KEYCLOAK_CLIENT_ID redirectUris now include http://$PUBLIC_HOST:$VERIFIABLY_HOST_PORT/* + https://$PUBLIC_HOST/*"
