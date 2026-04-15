#!/usr/bin/env bash
# start-all.sh — bring up the full vc.infra stack after a host reboot.
#
# Starts three independent things in the right order:
#
#   1. DPG stack    — walt.id + Inji Certify + Inji Verify + verification
#                     adapter + citizens DB + (optional) full Inji Web
#                     via --profile injiweb. Lives in
#                     ui-demo/docker/stack/docker-compose.yml.
#
#   2. Agent stack  — Qdrant + embeddings for the chatbot RAG corpus.
#                     Lives in agent-service/docker-compose.agents.yml.
#                     n8n auto-starts via its own container-level
#                     restart: unless-stopped policy — this script
#                     only kicks it if it's missing.
#
#   3. vc.infra Go  — the white-label app. Runs on the host, NOT in
#      server        docker. Launched with nohup so it survives this
#                     shell exiting.
#
# Before first run: copy ui-demo/docker/stack/.env.example to .env and
# edit PUBLIC_HOST for your deployment. The defaults work for a laptop
# with a browser on the same host.
#
# Usage:
#   ./scripts/start-all.sh               # full stack including Inji Web
#   ./scripts/start-all.sh --no-injiweb  # skip the 9 Inji Web containers
#   ./scripts/start-all.sh --no-agent    # skip the agent stack
#   ./scripts/start-all.sh --no-server   # don't start the Go app
#
# Idempotent — re-running only touches services that aren't already up.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STACK_DIR="$REPO_ROOT/ui-demo/docker/stack"
INJIWEB_DIR="$REPO_ROOT/ui-demo/docker/injiweb"
AGENT_DIR="$REPO_ROOT/agent-service"
APP_DIR="$REPO_ROOT/ui-demo"

WITH_INJIWEB=1
WITH_AGENT=1
WITH_SERVER=1

for arg in "$@"; do
    case "$arg" in
        --no-injiweb) WITH_INJIWEB=0 ;;
        --no-agent)   WITH_AGENT=0 ;;
        --no-server)  WITH_SERVER=0 ;;
        -h|--help)
            sed -n '2,30p' "$0"
            exit 0
            ;;
        *)
            echo "unknown flag: $arg" >&2
            exit 1
            ;;
    esac
done

step() { echo; echo "==> $*"; }

# -----------------------------------------------------------------------------
# Pre-flight: .env must exist for the DPG stack. Copy from the example
# on first run so docker-compose has something to interpolate.
# -----------------------------------------------------------------------------
if [[ ! -f "$STACK_DIR/.env" ]]; then
    step "first-run: copying $STACK_DIR/.env.example → $STACK_DIR/.env"
    cp "$STACK_DIR/.env.example" "$STACK_DIR/.env"
    echo "   edit $STACK_DIR/.env if you need non-default PUBLIC_HOST etc."
fi

# -----------------------------------------------------------------------------
# Render mimoto-issuers-config.json from its template using the current
# .env values. Must run before bringing Inji Web up so the UI container's
# mounted file reflects the target PUBLIC_HOST.
# -----------------------------------------------------------------------------
if [[ $WITH_INJIWEB -eq 1 ]]; then
    if [[ ! -f "$INJIWEB_DIR/config/mimoto-issuers-config.json.template" ]]; then
        step "Inji Web config missing — running fetch-config.sh"
        (cd "$INJIWEB_DIR" && ./fetch-config.sh)
    fi
    step "rendering mimoto-issuers-config.json from .env"
    (cd "$INJIWEB_DIR" && ./render-config.sh)
fi

# -----------------------------------------------------------------------------
# 1. DPG stack
# -----------------------------------------------------------------------------
step "starting DPG stack (docker/stack/docker-compose.yml)"
cd "$STACK_DIR"
if [[ $WITH_INJIWEB -eq 1 ]]; then
    docker compose --profile injiweb up -d
else
    docker compose up -d
fi

# -----------------------------------------------------------------------------
# 2. Agent stack (Qdrant + embeddings). n8n is a standalone container
#    with its own restart: unless-stopped policy — only kick it if it's
#    missing.
# -----------------------------------------------------------------------------
if [[ $WITH_AGENT -eq 1 ]]; then
    step "starting agent stack (Qdrant + embeddings)"
    cd "$AGENT_DIR"
    docker compose -f docker-compose.agents.yml up -d

    if ! docker ps --filter "name=n8n" --format '{{.Names}}' | grep -q '^n8n$'; then
        if docker ps -a --filter "name=n8n" --format '{{.Names}}' | grep -q '^n8n$'; then
            step "starting existing n8n container"
            docker start n8n
        else
            echo "   n8n container not found — start it manually (it lives outside"
            echo "   any compose file; see agent-service/README.md for setup)"
        fi
    fi
fi

# -----------------------------------------------------------------------------
# 3. vc.infra Go server. Binds on :8080, runs on the host.
# -----------------------------------------------------------------------------
if [[ $WITH_SERVER -eq 1 ]]; then
    if pgrep -fa "./server -config" >/dev/null 2>&1; then
        step "vc.infra server already running (pgrep matched)"
    else
        cd "$APP_DIR"
        # Source ui-demo/.env so $KEYCLOAK_URL / $WSO2_URL / $WALTID_*
        # placeholders in config/*.json get expanded by the Go config
        # loader's os.Expand pass at startup. Without this the SSO
        # discovery URLs come out as literal "$KEYCLOAK_URL/..." and the
        # /login flow fails. Mirrors what the Makefile's run-sso target
        # does.
        if [[ -f .env ]]; then
            set -o allexport
            # shellcheck disable=SC1091
            source .env
            set +o allexport
        fi

        # Provision the WSO2 OIDC application and write its client_id to
        # /tmp/wso2-client-id (the file the Go server reads via
        # `auto:$WSO2_CLIENT_ID_FILE` in config/default.json). Idempotent —
        # checks for an existing app first. This MUST run before the Go
        # server boots, otherwise `sso: 2 provider(s) configured` becomes
        # `1 provider(s)` and WSO2 login is unavailable. The script's own
        # boot-wait loop tolerates a still-warming WSO2 (up to 5 min).
        if [[ -f docker/stack/wso2-init.sh ]]; then
            step "provisioning WSO2 OIDC app (writes /tmp/wso2-client-id)"
            bash docker/stack/wso2-init.sh || \
                echo "   wso2-init.sh failed — WSO2 SSO will be disabled until you re-run it"
        fi

        step "starting vc.infra server on :8080"
        if [[ ! -x ./server ]]; then
            echo "   ./server not built — running 'go build -o server ./cmd/server'"
            go build -o server ./cmd/server
        fi
        nohup ./server -config config/default.json \
            > /tmp/vcplatform.log 2>&1 &
        disown
        echo "   logs: /tmp/vcplatform.log"
    fi
fi

# -----------------------------------------------------------------------------
# Report
# -----------------------------------------------------------------------------
step "stack up — waiting ~60-90s for Java services to finish booting"
echo
echo "  vc.infra          http://localhost:8080"
if [[ $WITH_INJIWEB -eq 1 ]]; then
    set -o allexport
    # shellcheck disable=SC1091
    source "$STACK_DIR/.env"
    set +o allexport
    : "${PUBLIC_HOST:=172.24.0.1}"
    : "${ESIGNET_PUBLIC_PORT:=3005}"
    : "${INJIWEB_UI_PUBLIC_PORT:=3004}"
    echo "  Inji Web catalog  http://${PUBLIC_HOST}:${INJIWEB_UI_PUBLIC_PORT}/issuers"
    echo "  eSignet authorize http://${PUBLIC_HOST}:${ESIGNET_PUBLIC_PORT}/authorize"
fi
if [[ $WITH_AGENT -eq 1 ]]; then
    echo "  n8n               http://localhost:5678"
fi
echo
echo "  First-time seeding (idempotent — safe to re-run):"
echo "    $INJIWEB_DIR/seed-esignet-client.sh"
echo "    $INJIWEB_DIR/seed-mock-identity.sh"
