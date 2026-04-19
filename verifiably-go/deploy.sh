#!/usr/bin/env bash
# verifiably-go deploy script
#
# Single entry point for three scenarios, each of which brings up a curated
# subset of the shared compose stack and launches verifiably-go with a
# backends.json tailored to that subset.
#
#   ./deploy.sh up all     — every DPG (walt.id + inji stack + inji web)
#                            plus Keycloak + WSO2IS + LibreTranslate.
#   ./deploy.sh up waltid  — verifiably-go + walt.id + Keycloak + LibreTranslate.
#   ./deploy.sh up inji    — verifiably-go + Inji Certify (auth-code + pre-auth)
#                            + Inji Verify + Inji Web + WSO2IS + LibreTranslate.
#
# Other subcommands:
#   ./deploy.sh down [scenario]    — stop the services for a scenario (or all).
#   ./deploy.sh status             — summarise what's running.
#   ./deploy.sh config <scenario>  — print the backends.json that would be used.
#   ./deploy.sh run <scenario>     — generate backends.json + start verifiably-go
#                                    (without touching compose — for when the
#                                     stack is already up).
#
# The script does NOT modify the shared compose file; it selects services
# explicitly via `docker compose up <service> ...` and opts into the injiweb
# profile when the scenario needs it.

set -euo pipefail

# ------------------------------------------------------------------ config

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
: "${VERIFIABLY_COMPOSE_FILE:=$SCRIPT_DIR/../ui-demo/docker/stack/docker-compose.yml}"
: "${VERIFIABLY_COMPOSE_OVERRIDE:=$SCRIPT_DIR/deploy/docker-compose.injiweb-fix.yml}"
: "${VERIFIABLY_ADDR:=:8080}"
: "${VERIFIABLY_HOST_PORT:=8080}"
: "${VERIFIABLY_PUBLIC_URL:=http://localhost:$VERIFIABLY_HOST_PORT}"
: "${LIBRETRANSLATE_URL:=http://localhost:5000}"
: "${VERIFIABLY_IMAGE:=verifiably-go:local}"
: "${VERIFIABLY_CONTAINER:=verifiably-go}"
# The `waltid_` prefix on volumes + network is load-bearing in the shared
# compose (pinned via `name: waltid`). Every compose subcommand we issue
# must point at the same project name so we line up with the existing state.
: "${COMPOSE_PROJECT:=waltid}"

# Service lists per scenario. Kept here (not inside the compose file) so
# this script is the single source of truth for "what belongs to which
# scenario" and can evolve without touching the shared compose.

WALTID_SERVICES=(
  postgres caddy issuer-api verifier-api wallet-api
)
IDP_KEYCLOAK=( keycloak )
IDP_WSO2IS=( wso2is )
TRANSLATOR_SERVICES=( libretranslate )
INJI_CORE_SERVICES=(
  certify-postgres inji-certify
  certify-preauth-postgres inji-certify-preauth
  certify-nginx
  inji-verify-postgres inji-verify-service inji-verify-ui
  citizens-postgres vc-adapter
)
INJIWEB_SERVICES=(
  injiweb-postgres injiweb-redis
  injiweb-mock-identity injiweb-esignet injiweb-oidc-ui
  injiweb-minio injiweb-datashare injiweb-mimoto injiweb-ui
)

# ------------------------------------------------------------------ helpers

red()    { printf '\033[31m%s\033[0m\n' "$*" >&2; }
green()  { printf '\033[32m%s\033[0m\n' "$*"; }
bold()   { printf '\033[1m%s\033[0m\n' "$*"; }

require() {
  local cmd="$1"
  command -v "$cmd" >/dev/null 2>&1 || { red "missing dependency: $cmd"; exit 1; }
}

compose() {
  local extra=()
  if [[ -f "$VERIFIABLY_COMPOSE_OVERRIDE" ]]; then
    # When docker compose layers multiple files, relative paths inside
    # each file are resolved relative to the FIRST -f argument, not the
    # file that declared them. That breaks our override — it ends up
    # looking for the patched bootstrap under ui-demo/verifiably-go/…
    # which doesn't exist. Materialise a rendered override with an
    # absolute path instead.
    local rendered="$SCRIPT_DIR/config/docker-compose.injiweb-fix.rendered.yml"
    sed "s|{{VERIFIABLY_GO_DIR}}|$SCRIPT_DIR|g" "$VERIFIABLY_COMPOSE_OVERRIDE" > "$rendered"
    extra+=( -f "$rendered" )
  fi
  docker compose -p "$COMPOSE_PROJECT" -f "$VERIFIABLY_COMPOSE_FILE" "${extra[@]}" "$@"
}

scenario_services() {
  local scenario="$1"
  case "$scenario" in
    all)
      printf '%s\n' \
        "${WALTID_SERVICES[@]}" \
        "${IDP_KEYCLOAK[@]}" "${IDP_WSO2IS[@]}" \
        "${TRANSLATOR_SERVICES[@]}" \
        "${INJI_CORE_SERVICES[@]}" \
        "${INJIWEB_SERVICES[@]}"
      ;;
    waltid)
      printf '%s\n' \
        "${WALTID_SERVICES[@]}" \
        "${IDP_KEYCLOAK[@]}" \
        "${TRANSLATOR_SERVICES[@]}"
      ;;
    inji)
      printf '%s\n' \
        "${INJI_CORE_SERVICES[@]}" \
        "${INJIWEB_SERVICES[@]}" \
        "${IDP_WSO2IS[@]}" \
        "${TRANSLATOR_SERVICES[@]}"
      ;;
    *)
      red "unknown scenario: $scenario (want: all | waltid | inji)"; return 1;;
  esac
}

# scenario_needs_injiweb prints "yes" if the scenario includes any injiweb-*
# service — that decides whether we need to pass `--profile injiweb` to
# docker compose.
scenario_needs_injiweb() {
  scenario_services "$1" | grep -q '^injiweb-' && echo yes || echo no
}

# backends_for writes a scenario-specific config/backends.json. The content
# is assembled by including scenario-relevant stanzas; anything not in the
# scenario gets omitted so the UI never offers a DPG whose backend isn't up.
backends_for() {
  local scenario="$1"
  local out="$SCRIPT_DIR/config/backends.json"

  # Individual DPG stanzas — kept inline as HEREDOCs so the script is
  # self-contained (no per-scenario template files to manage).
  local waltid_stanza
  waltid_stanza=$(cat <<'JSON'
    {
      "vendor": "Walt Community Stack",
      "type": "walt_community",
      "roles": ["issuer", "holder", "verifier"],
      "dpg": {
        "Vendor": "Walt Community Stack",
        "Version": "v0.18.2",
        "Tag": "API-based",
        "Tagline": "Open-source, API-driven credentialing stack.",
        "FlowPreAuth": true,
        "FlowAuthCode": true,
        "FlowPlain": "OID4VCI with pre-authorized code flow and authorization code flow.",
        "Formats": ["w3c_vcdm_2", "sd_jwt_vc (IETF)", "mso_mdoc"],
        "FormatsPlain": "W3C VCDM 2.0 signed as JWT, SD-JWT VC (IETF), and ISO 18013-5 mdoc.",
        "DirectPDF": false,
        "DirectPDFPlain": "No documented QR-on-PDF export at v0.18.2.",
        "Caveats": "OID4VP v1.0 support in the wallet/demo apps is still rolling out.",
        "Capabilities": [
          {"Kind": "flow",  "Key": "pre_auth",      "Title": "Pre-authorized code flow", "Body": "Issuer stages the offer; wallet redeems at the token endpoint."},
          {"Kind": "flow",  "Key": "auth_code",     "Title": "Authorization code flow",  "Body": "Holder consents at the issuer; wallet exchanges the code."},
          {"Kind": "token", "Key": "issuer_signed", "Title": "Issuer-signed tokens",     "Body": "Tokens signed by this issuer's own keys."},
          {"Kind": "mode",  "Key": "wallet",        "Title": "Wallet delivery",          "Body": "Offer URI scanned or pasted into any OID4VCI wallet."}
        ]
      },
      "config": {
        "issuerBaseUrl": "http://localhost:7002",
        "verifierBaseUrl": "http://localhost:7003",
        "walletBaseUrl": "http://localhost:7001",
        "standardVersion": "draft13",
        "demoAccount": {
          "name": "Verifiably Demo",
          "email": "verifiably-demo@example.org",
          "password": "verifiably-demo-password"
        }
      }
    }
JSON
)
  local inji_authcode_stanza
  inji_authcode_stanza=$(cat <<'JSON'
    {
      "vendor": "Inji Certify · Auth-Code",
      "type": "inji_certify_authcode",
      "roles": ["issuer"],
      "dpg": {
        "Vendor": "Inji Certify",
        "Version": "v0.14.0 · Auth-Code via eSignet",
        "Tag": "MOSIP · Auth-Code",
        "Tagline": "Holder logs into eSignet; Inji Certify validates tokens as a resource server.",
        "FlowAuthCode": true,
        "FlowPresentationDuringIssue": true,
        "FlowPlain": "OID4VCI draft 13 Authorization Code flow via eSignet.",
        "Formats": ["w3c_vcdm_2", "sd_jwt_vc (IETF)"],
        "DirectPDF": false,
        "Caveats": "Holder wallet must be reachable by eSignet's redirect.",
        "Redirect": true,
        "UIURL": "http://172.24.0.1:3004",
        "Capabilities": [
          {"Kind": "flow",       "Key": "auth_code",       "Title": "Authorization Code flow",          "Body": "Wallet redirects holder to eSignet for login."},
          {"Kind": "data",       "Key": "identity_lookup", "Title": "Claims from MOSIP Identity Plugin", "Body": "Fills claims via UIN lookup against mock-identity."},
          {"Kind": "wallet",     "Key": "inji_web",        "Title": "Experience via Inji Web Wallet",    "Body": "Clicking the card opens Inji Web where the full eSignet auth-code flow plays out end-to-end."},
          {"Kind": "token",      "Key": "idp_signed",      "Title": "Tokens signed by the IdP",          "Body": "Credential endpoint validates eSignet-signed tokens."},
          {"Kind": "limitation", "Key": "needs_idp",       "Title": "Requires eSignet running",          "Body": "Fails closed if the IdP is unreachable."}
        ]
      },
      "config": {
        "mode": "auth_code",
        "baseUrl": "http://localhost:8091",
        "internalBaseUrl": "http://certify-nginx:80",
        "publicBaseUrl": "http://localhost:8091",
        "offerIssuerUrl": "http://certify-nginx:80",
        "authorizationServer": "http://172.24.0.1:3005"
      }
    }
JSON
)
  local inji_preauth_stanza
  inji_preauth_stanza=$(cat <<'JSON'
    {
      "vendor": "Inji Certify · Pre-Auth",
      "type": "inji_certify_preauth",
      "roles": ["issuer"],
      "dpg": {
        "Vendor": "Inji Certify",
        "Version": "v0.14.0 · Pre-Authorized Code",
        "Tag": "MOSIP · Pre-Auth",
        "Tagline": "Operator stages claims directly; wallet redeems pre-auth code at certify's own token endpoint.",
        "FlowPreAuth": true,
        "FlowPlain": "Self-contained — no external identity provider.",
        "Formats": ["w3c_vcdm_2", "sd_jwt_vc (IETF)"],
        "Caveats": "Not compatible with Inji Web Wallet. Demo-only — no user consent.",
        "Capabilities": [
          {"Kind": "flow",       "Key": "pre_auth",         "Title": "Pre-Authorized Code flow",           "Body": "POST /v1/certify/pre-authorized-data; wallet redeems code directly."},
          {"Kind": "data",       "Key": "operator_entered", "Title": "Claims entered by the operator",     "Body": "Operator types claims or loads a CSV row via the Pre-Auth plugin."},
          {"Kind": "wallet",     "Key": "paste",            "Title": "Works with paste-based wallets",     "Body": "Pasteable offer URI for any OID4VCI wallet."},
          {"Kind": "token",      "Key": "self_signed",      "Title": "Tokens signed by this instance",     "Body": "No external IdP; isolated JWKS validates its own tokens."},
          {"Kind": "limitation", "Key": "no_consent",       "Title": "No user consent screen",             "Body": "Demo only — no interactive approval."},
          {"Kind": "limitation", "Key": "not_inji_web",     "Title": "Not usable by Inji Web Wallet",      "Body": "Mimoto assumes Auth-Code; won't redeem pre-auth offers."}
        ]
      },
      "config": {
        "mode": "pre_auth",
        "baseUrl": "http://localhost:8094",
        "internalBaseUrl": "http://inji-certify-preauth:8090",
        "publicBaseUrl": "http://localhost:8094",
        "offerIssuerUrl": "http://inji-certify-preauth:8090"
      }
    }
JSON
)
  local inji_verify_stanza
  inji_verify_stanza=$(cat <<'JSON'
    {
      "vendor": "Inji Verify",
      "type": "inji_verify",
      "roles": ["verifier"],
      "dpg": {
        "Vendor": "Inji Verify",
        "Version": "v0.16.0",
        "Tag": "Redirect",
        "Tagline": "MOSIP verifier — operator runs presentation sessions through Inji Verify's own UI.",
        "FlowPlain": "Click the card to open the real Inji Verify UI in a new tab, or use the in-process direct-verify endpoints (paste/upload a JSON-LD VC).",
        "Formats": ["w3c_vcdm_1", "w3c_vcdm_2", "sd_jwt_vc (IETF)"],
        "Caveats": "INJIVER-1131: v0.16.0 cross-device flow can accept a wrong VC as valid — adapter applies a field-match guard.",
        "Redirect": true,
        "UIURL": "http://localhost:3001",
        "Capabilities": [
          {"Kind": "flow",       "Key": "direct_paste",  "Title": "Paste JSON-LD VC",         "Body": "POST /v1/verify/vc-verification returns SUCCESS/INVALID synchronously."},
          {"Kind": "flow",       "Key": "direct_upload", "Title": "Upload a QR image",        "Body": "Server decodes the QR with gozxing, then verifies the payload."},
          {"Kind": "flow",       "Key": "oid4vp",        "Title": "OID4VP via Inji Verify UI", "Body": "Full cross-device presentation flow lives in the Inji Verify SPA."},
          {"Kind": "limitation", "Key": "injiver_1131",  "Title": "INJIVER-1131 guard applied", "Body": "Adapter re-checks claims against requested fields."}
        ]
      },
      "config": {
        "baseUrl": "http://localhost:8082",
        "clientId": "verifiably-demo"
      }
    }
JSON
)
  local injiweb_stanza
  injiweb_stanza=$(cat <<'JSON'
    {
      "vendor": "Inji Web Wallet",
      "type": "inji_web",
      "roles": ["holder"],
      "dpg": {
        "Vendor": "Inji Web Wallet",
        "Version": "v0.16.0",
        "Tag": "Redirect",
        "Tagline": "MOSIP's browser-hosted wallet — credentials live inside the Inji Web SPA.",
        "FlowPlain": "Holder logs into Inji Web via eSignet. No server-to-server read-back API at v0.16.0.",
        "Formats": ["w3c_vcdm_1", "w3c_vcdm_2"],
        "Caveats": "Tested-compatible with Inji Certify v0.13.1 and Inji Verify v0.17.0 per the v0.16.0 matrix.",
        "Redirect": true,
        "UIURL": "http://172.24.0.1:3004",
        "Capabilities": [
          {"Kind": "flow",       "Key": "browser_hosted", "Title": "Browser-hosted wallet",        "Body": "Credentials live inside the Inji Web SPA."},
          {"Kind": "wallet",     "Key": "opens_tab",      "Title": "Opens in a new tab",            "Body": "Selecting this DPG hands off to the Inji Web app."},
          {"Kind": "limitation", "Key": "no_readback",    "Title": "No third-party read-back API",  "Body": "No way for an external service to list credentials at v0.16.0."}
        ]
      },
      "config": {
        "uiUrl": "http://172.24.0.1:3004",
        "mimotoUrl": "http://localhost:8099"
      }
    }
JSON
)

  # Assemble the backends array based on scenario.
  local entries=()
  case "$scenario" in
    all)
      entries=( "$waltid_stanza" "$inji_authcode_stanza" "$inji_preauth_stanza" "$inji_verify_stanza" "$injiweb_stanza" );;
    waltid)
      entries=( "$waltid_stanza" );;
    inji)
      entries=( "$inji_authcode_stanza" "$inji_preauth_stanza" "$inji_verify_stanza" "$injiweb_stanza" );;
    *)
      red "unknown scenario: $scenario"; return 1;;
  esac

  mkdir -p "$(dirname "$out")"
  {
    printf '{\n  "backends": [\n'
    local i
    for i in "${!entries[@]}"; do
      printf '%s' "${entries[$i]}"
      if [[ $i -lt $(( ${#entries[@]} - 1 )) ]]; then
        printf ',\n'
      else
        printf '\n'
      fi
    done
    printf '  ]\n}\n'
  } > "$out"

  green "wrote $out"
}

# auth_providers_for writes the scenario-specific auth-providers.json. Each
# scenario pairs with a primary IdP; we include both Keycloak and WSO2IS
# only in the `all` scenario, so the picker reflects what's actually up.
auth_providers_for() {
  local scenario="$1"
  local out="$SCRIPT_DIR/config/auth-providers.json"
  # clientId "vcplatform" matches the public client seeded by the shared
  # compose's keycloak-realm.json (realm: vcplatform, client: vcplatform,
  # redirectUris: http://localhost:8080/*). Keep these two in sync.
  local keycloak='{"id":"keycloak","type":"oidc","displayName":"Keycloak","kind":"OIDC","issuerUrl":"http://localhost:8180/realms/vcplatform","clientId":"vcplatform","scopes":["openid","profile","email"]}'

  # WSO2IS client_id + client_secret come from config/wso2is.env, written by
  # scripts/bootstrap-wso2is.sh (run automatically by `deploy.sh up` below).
  # Falls back to placeholder values if the bootstrap hasn't run; the provider
  # button will render but attempts to sign in will fail until it has.
  local wso2_id="verifiably_go_client"
  local wso2_secret=""
  if [[ -f "$SCRIPT_DIR/config/wso2is.env" ]]; then
    # shellcheck disable=SC1090
    source "$SCRIPT_DIR/config/wso2is.env"
    wso2_id="${WSO2_CLIENT_ID:-$wso2_id}"
    wso2_secret="${WSO2_CLIENT_SECRET:-}"
  fi
  local wso2is='{"id":"wso2is","type":"oidc","displayName":"WSO2 Identity Server","kind":"OIDC","issuerUrl":"https://localhost:9443/oauth2/token","clientId":"'"$wso2_id"'","clientSecret":"'"$wso2_secret"'","scopes":["openid","profile","email"],"insecureSkipVerify":true}'
  local items=()
  case "$scenario" in
    all)    items=( "$keycloak" "$wso2is" );;
    waltid) items=( "$keycloak" );;
    inji)   items=( "$wso2is" );;
  esac
  mkdir -p "$(dirname "$out")"
  {
    printf '['
    local i
    for i in "${!items[@]}"; do
      [[ $i -gt 0 ]] && printf ','
      printf '\n  %s' "${items[$i]}"
    done
    printf '\n]\n'
  } > "$out"
  green "wrote $out"
}

# ---------------------------------------------------------------- subcommands

cmd_up() {
  local scenario="${1:-}"
  [[ -n "$scenario" ]] || { red "usage: deploy.sh up <all|waltid|inji>"; exit 2; }
  scenario_services "$scenario" > /dev/null  # validate

  require docker

  bold "▶ Preparing config for scenario=$scenario"
  backends_for "$scenario"
  auth_providers_for "$scenario"

  # If injiweb-esignet or injiweb-mock-identity are in a restart loop, their
  # container writable layers have accumulated state that keeps the entrypoint
  # from completing (see docker-compose.injiweb-fix.yml for detail). Recreate
  # them so they start with a clean layer.
  if [[ "$(scenario_needs_injiweb "$scenario")" == "yes" ]]; then
    recover_injiweb
  fi

  bold "▶ Starting DPG services via docker compose"
  local -a services
  readarray -t services < <(scenario_services "$scenario")
  local profile_args=()
  if [[ "$(scenario_needs_injiweb "$scenario")" == "yes" ]]; then
    profile_args+=( --profile injiweb )
  fi
  compose "${profile_args[@]}" up -d "${services[@]}"

  bold "▶ Waiting for services to be reachable"
  wait_for_services "$scenario"

  # If WSO2IS is part of this scenario, register the OIDC client before the
  # UI starts. Idempotent — a second run reuses the existing registration.
  if [[ "$scenario" == "all" || "$scenario" == "inji" ]]; then
    bold "▶ Bootstrapping WSO2IS OIDC client"
    "$SCRIPT_DIR/scripts/bootstrap-wso2is.sh" || red "  WSO2IS bootstrap failed (proceeding — you can re-run it manually)"
    # Re-generate auth-providers.json now that wso2is.env exists.
    auth_providers_for "$scenario"
  fi

  # Seed the injiweb stack: register the wallet-demo-client keystore with
  # eSignet so private_key_jwt token exchange works, and stuff a test
  # identity into mock-identity so users can actually sign in.
  if [[ "$(scenario_needs_injiweb "$scenario")" == "yes" ]]; then
    bold "▶ Seeding Inji Web auth stack"
    local esignet_seed="$SCRIPT_DIR/../ui-demo/docker/injiweb/seed-esignet-client.sh"
    local mock_seed="$SCRIPT_DIR/../ui-demo/docker/injiweb/seed-mock-identity.sh"
    if [[ -x "$esignet_seed" ]]; then
      (cd "$(dirname "$esignet_seed")" && "$esignet_seed") \
        || red "  seed-esignet-client failed (retry manually: $esignet_seed)"
    else
      red "  $esignet_seed not found — OIDC login through Inji Web will fail"
    fi
    if [[ -x "$mock_seed" ]]; then
      "$mock_seed" || red "  seed-mock-identity failed (retry manually: $mock_seed)"
    else
      red "  $mock_seed not found — Inji Web login has no identities to authenticate"
    fi
    # The seed script returns OK on duplicate_client_id, but a previous deploy
    # could have registered the client with a different redirect_uri (e.g.
    # http://localhost:3004/redirect if UIURL was localhost before). eSignet
    # then rejects /authorize with invalid_redirect_uri. Repair the row in
    # place and flush the Redis client cache so the fix takes effect without
    # requiring a destructive re-seed.
    repair_injiweb_client_redirect_uri
  fi

  bold "▶ Building verifiably-go image ($VERIFIABLY_IMAGE)"
  docker build -q -t "$VERIFIABLY_IMAGE" "$SCRIPT_DIR" >/dev/null

  bold "▶ Starting verifiably-go container"
  start_container "$scenario"
  echo "    point your browser at $VERIFIABLY_PUBLIC_URL"
}

# repair_injiweb_client_redirect_uri ensures the wallet-demo-client row in
# eSignet's postgres has a redirect_uris array containing
# http://${PUBLIC_HOST}:3004/redirect. If it's missing, we rewrite the row and
# flush the Redis client cache (eSignet caches client_detail rows there with
# no invalidation on external DB writes).
#
# Idempotent — safe to run on every deploy. Only touches the wallet-demo-client
# row.
repair_injiweb_client_redirect_uri() {
  local public_host="${PUBLIC_HOST:-172.24.0.1}"
  local want="http://${public_host}:3004/redirect"
  local current
  current=$(docker exec injiweb-postgres \
    psql -U postgres -d mosip_esignet -tAX \
    -c "SELECT redirect_uris FROM client_detail WHERE id='wallet-demo-client'" 2>/dev/null || true)
  if [[ -z "$current" ]]; then
    red "  wallet-demo-client not found in eSignet DB — seed script may have failed"
    return
  fi
  if [[ "$current" == *"$want"* ]]; then
    return   # already has our redirect URI
  fi
  # Add the PUBLIC_HOST URI alongside whatever is already there. Keeping the
  # existing entries means old browser sessions don't break if a user has an
  # in-flight redirect URL in their history.
  local merged
  merged=$(python3 -c "
import json, sys
cur = json.loads('''$current''')
want = '$want'
if want not in cur:
    cur.append(want)
print(json.dumps(cur))
")
  docker exec injiweb-postgres psql -U postgres -d mosip_esignet -qc \
    "UPDATE client_detail SET redirect_uris='$merged' WHERE id='wallet-demo-client'" >/dev/null
  docker exec injiweb-redis redis-cli DEL 'clientdetails::wallet-demo-client' >/dev/null
  green "  repaired wallet-demo-client redirect_uris (+$want)"
}

cmd_down() {
  local scenario="${1:-all}"
  scenario_services "$scenario" > /dev/null  # validate

  bold "▶ Stopping verifiably-go container"
  stop_container

  bold "▶ Stopping compose services for scenario=$scenario"
  local -a services
  readarray -t services < <(scenario_services "$scenario")
  local profile_args=()
  if [[ "$(scenario_needs_injiweb "$scenario")" == "yes" ]]; then
    profile_args+=( --profile injiweb )
  fi
  compose "${profile_args[@]}" stop "${services[@]}"
}

cmd_status() {
  bold "▶ Running compose services"
  compose --profile injiweb ps --format '  {{.Service}}  {{.Status}}' 2>/dev/null | sort -u
  echo
  bold "▶ verifiably-go container"
  if docker ps --filter "name=^${VERIFIABLY_CONTAINER}$" --format '  {{.Names}}  {{.Status}}  {{.Ports}}' | grep -q .; then
    docker ps --filter "name=^${VERIFIABLY_CONTAINER}$" --format '  {{.Names}}  {{.Status}}  {{.Ports}}'
  else
    echo "  not running"
  fi
}

cmd_config() {
  local scenario="${1:-}"
  [[ -n "$scenario" ]] || { red "usage: deploy.sh config <all|waltid|inji>"; exit 2; }
  backends_for "$scenario"
  echo "---"
  cat "$SCRIPT_DIR/config/backends.json"
}

cmd_run() {
  local scenario="${1:-}"
  [[ -n "$scenario" ]] || { red "usage: deploy.sh run <all|waltid|inji>"; exit 2; }
  require docker
  backends_for "$scenario"
  auth_providers_for "$scenario"
  bold "▶ Building verifiably-go image ($VERIFIABLY_IMAGE)"
  docker build -q -t "$VERIFIABLY_IMAGE" "$SCRIPT_DIR" >/dev/null
  start_container "$scenario"
  echo "    point your browser at $VERIFIABLY_PUBLIC_URL"
}

# ---------------------------------------------------------------- helpers

# wait_for_services polls the TCP ports each scenario needs to be healthy
# before verifiably-go starts. Bounded — we don't block forever if a service
# is struggling; the app itself surfaces the failure on first use.
wait_for_services() {
  local scenario="$1"
  local -a ports=()
  case "$scenario" in
    all|waltid)    ports+=( 7001 7002 7003 8180 5000 );;
  esac
  case "$scenario" in
    all|inji)      ports+=( 8082 8091 8094 9443 5000 );;
  esac
  # De-dup; bash-ish.
  local seen="" p
  for p in "${ports[@]}"; do
    [[ ",$seen," == *",$p,"* ]] || { wait_port "$p"; seen="$seen,$p"; }
  done
}

# recover_injiweb force-recreates the three injiweb services that tend to
# get stuck in restart loops, so the next `docker compose up` gives them
# clean container state. Mimoto then picks up the patched bootstrap from the
# override compose; eSignet + mock-identity re-run their entrypoints with
# empty writable layers and the HSM unzip succeeds.
recover_injiweb() {
  local -a stuck=()
  for svc in injiweb-mimoto injiweb-esignet injiweb-mock-identity; do
    local state
    state=$(docker inspect "$svc" --format '{{.State.Status}}' 2>/dev/null || echo missing)
    if [[ "$state" == "restarting" ]]; then
      stuck+=( "$svc" )
    fi
  done
  if [[ ${#stuck[@]} -eq 0 ]]; then
    return 0
  fi
  bold "▶ Recovering injiweb services stuck in restart loop: ${stuck[*]}"
  for svc in "${stuck[@]}"; do
    docker rm -f "$svc" >/dev/null 2>&1 || true
    green "  removed $svc (container layer reset)"
  done
}

wait_port() {
  local port="$1"
  local tries=0
  while ! (exec 3<>/dev/tcp/localhost/"$port") 2>/dev/null; do
    tries=$((tries + 1))
    if [[ $tries -gt 60 ]]; then
      red "  port $port not reachable after 60s — continuing anyway"
      return 0
    fi
    sleep 1
  done
  exec 3<&-
  green "  port $port ready"
}

start_container() {
  local scenario="$1"
  stop_container

  # Regenerate backends.json in docker-internal form. The host-native version
  # (localhost:7002, etc.) is still in config/ but the container mounts an
  # override at /app/config/backends.json so it reaches DPGs via Docker DNS.
  backends_for_docker "$scenario"

  # --add-host=host.docker.internal:host-gateway makes the Docker host
  # reachable from inside this container as `host.docker.internal`. The OIDC
  # provider URLs stay on their browser-facing form (localhost:8180, etc.)
  # so the HX-Redirect we send to the browser is something the browser can
  # actually resolve. Container-side discovery + token exchange travel via
  # docker-internal DNS (wallet-api, issuer-api, ...) where the hostname
  # differs from the browser-facing one.
  docker run -d \
    --name "$VERIFIABLY_CONTAINER" \
    --network "${COMPOSE_PROJECT}_default" \
    --add-host=host.docker.internal:host-gateway \
    -p "${VERIFIABLY_HOST_PORT}:8080" \
    -v "$SCRIPT_DIR/config/backends.docker.json:/app/config/backends.json:ro" \
    -v "$SCRIPT_DIR/config/auth-providers.docker.json:/app/config/auth-providers.json:ro" \
    -v "${VERIFIABLY_CONTAINER}-locales:/app/locales" \
    -e VERIFIABLY_ADAPTER=registry \
    -e VERIFIABLY_ADDR=:8080 \
    -e VERIFIABLY_PUBLIC_URL="$VERIFIABLY_PUBLIC_URL" \
    -e LIBRETRANSLATE_URL="http://libretranslate:5000" \
    "$VERIFIABLY_IMAGE" >/dev/null

  sleep 1
  if docker ps --filter "name=^${VERIFIABLY_CONTAINER}$" --filter "status=running" -q | grep -q .; then
    green "  container $VERIFIABLY_CONTAINER running ($scenario)"
  else
    red "  container failed to start — last logs:"
    docker logs "$VERIFIABLY_CONTAINER" 2>&1 | tail -n 25 >&2 || true
    exit 1
  fi
}

stop_container() {
  if docker ps -a --filter "name=^${VERIFIABLY_CONTAINER}$" -q | grep -q .; then
    docker rm -f "$VERIFIABLY_CONTAINER" >/dev/null 2>&1 || true
  fi
}

# backends_for_docker writes a sibling config/backends.docker.json with
# docker-internal hostnames so the containerized verifiably-go can reach
# every DPG on the waltid_default network.
#
# Only rewrites fields the CONTAINER reads for backend-to-backend calls —
# "baseUrl", "issuerBaseUrl", "verifierBaseUrl", "walletBaseUrl",
# "mimotoUrl", "authorizationServer", "offerIssuerUrl", "issuerUrl".
# Browser-facing fields ("UIURL", "publicBaseUrl") are left on their
# localhost URLs so link-outs remain host-reachable.
#
# Uses Python instead of sed because sed can't scope rewrites by JSON key.
backends_for_docker() {
  local src="$SCRIPT_DIR/config/backends.json"
  local dst="$SCRIPT_DIR/config/backends.docker.json"
  local auth_src="$SCRIPT_DIR/config/auth-providers.json"
  local auth_dst="$SCRIPT_DIR/config/auth-providers.docker.json"

  python3 - "$src" "$dst" "$auth_src" "$auth_dst" <<'PY'
import json, sys
src, dst, auth_src, auth_dst = sys.argv[1:5]

# Fields in backends.json that hold a URL the CONTAINER needs to reach.
# UIURL and publicBaseUrl are intentionally excluded — they are shown to
# the browser, not used by the server, and must stay host-reachable.
internal_fields = {
    "baseUrl", "issuerBaseUrl", "verifierBaseUrl", "walletBaseUrl",
    "mimotoUrl", "authorizationServer", "offerIssuerUrl",
    "internalBaseUrl",  # the adapter writes this as the "from" half of the
                        # URL rewrite; it stays on the docker-internal host.
}

# localhost:PORT → docker-internal hostname:PORT
host_to_internal = {
    "http://localhost:7001": "http://wallet-api:7001",
    "http://localhost:7002": "http://issuer-api:7002",
    "http://localhost:7003": "http://verifier-api:7003",
    "http://localhost:8091": "http://certify-nginx:80",
    "http://localhost:8094": "http://inji-certify-preauth:8090",
    "http://localhost:8082": "http://inji-verify-service:8080",
    "http://localhost:3001": "http://inji-verify-ui:8000",
    "http://localhost:3004": "http://injiweb-ui:3004",
    "http://localhost:8099": "http://injiweb-mimoto:8099",
    "http://localhost:3005": "http://injiweb-oidc-ui:3000",
    "http://172.24.0.1:3005": "http://injiweb-oidc-ui:3000",
}

def rewrite_url(url):
    if not isinstance(url, str):
        return url
    for host, internal in host_to_internal.items():
        if url.startswith(host):
            return internal + url[len(host):]
    return url

def walk(obj, inside_internal_scope=False):
    if isinstance(obj, dict):
        for k, v in list(obj.items()):
            if k in internal_fields and isinstance(v, str):
                obj[k] = rewrite_url(v)
            elif isinstance(v, (dict, list)):
                walk(v, inside_internal_scope)
    elif isinstance(obj, list):
        for it in obj:
            walk(it, inside_internal_scope)

with open(src) as f:
    data = json.load(f)
walk(data)
with open(dst, "w") as f:
    json.dump(data, f, indent=2)

# Auth providers: the container-side issuerUrl is the docker-internal
# hostname (used for discovery + token exchange). The ORIGINAL localhost URL
# is preserved as publicIssuerUrl so the browser's authorize redirect points
# somewhere it can actually reach.
with open(auth_src) as f:
    auth = json.load(f)
for entry in auth:
    iu = entry.get("issuerUrl", "")
    entry["publicIssuerUrl"] = iu  # what the browser sees
    entry["issuerUrl"] = (iu
        .replace("http://localhost:8180", "http://keycloak:8180")
        .replace("https://localhost:9443", "https://wso2is:9443"))
with open(auth_dst, "w") as f:
    json.dump(auth, f, indent=2)

print(f"  wrote {dst} + {auth_dst} (docker-internal URLs, UIURL preserved)")
PY
}

# ---------------------------------------------------------------- main

usage() {
  cat >&2 <<EOF
usage: deploy.sh <command> [scenario]

commands:
  up <all|waltid|inji>     start compose services + build & run verifiably-go container
  down [all|waltid|inji]   stop them (default: all)
  run <all|waltid|inji>    rebuild + restart only the verifiably-go container
                           (use when the DPG stack is already up)
  config <all|waltid|inji> print the backends.json that would be generated
  status                   summarise what's running

scenarios:
  all     every DPG + both IdPs + LibreTranslate
  waltid  walt.id stack + Keycloak + LibreTranslate
  inji    Inji Certify (×2) + Inji Verify + Inji Web + WSO2IS + LibreTranslate

all three scenarios include a containerised verifiably-go on port $VERIFIABLY_HOST_PORT,
attached to the compose network (${COMPOSE_PROJECT}_default).
EOF
}

main() {
  local cmd="${1:-}"
  case "$cmd" in
    up)      shift; cmd_up "$@";;
    down)    shift; cmd_down "$@";;
    status)  cmd_status;;
    config)  shift; cmd_config "$@";;
    run)     shift; cmd_run "$@";;
    help|-h|--help|"") usage;;
    *)       red "unknown command: $cmd"; usage; exit 2;;
  esac
}

main "$@"
