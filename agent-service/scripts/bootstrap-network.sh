#!/usr/bin/env bash
# Connect the existing n8n container to the cdpi-agents network so it can
# reach qdrant at http://qdrant:6333. Safe to re-run.
set -euo pipefail

if ! docker network inspect cdpi-agents >/dev/null 2>&1; then
  echo "cdpi-agents network does not exist yet — run 'docker compose -f docker-compose.agents.yml up -d qdrant' first."
  exit 1
fi

if docker network inspect cdpi-agents --format '{{range .Containers}}{{.Name}} {{end}}' | grep -qw n8n; then
  echo "n8n already attached to cdpi-agents."
else
  docker network connect cdpi-agents n8n
  echo "Attached n8n to cdpi-agents."
fi

echo "n8n can now reach: http://qdrant:6333"
