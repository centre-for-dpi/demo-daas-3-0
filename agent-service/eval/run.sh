#!/usr/bin/env bash
# Run the eval set against the live CDPI Agent Chat webhook.
# First iteration is human-scored, not automated: we print the expected
# behaviour and the actual answer side by side so you can judge.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WEBHOOK="${WEBHOOK:-http://localhost:5678/webhook/agent-chat}"
OUT_DIR="${ROOT}/eval/out"
mkdir -p "$OUT_DIR"
TS=$(date +%Y%m%d-%H%M%S)
OUT_FILE="${OUT_DIR}/${TS}.md"

echo "# CDPI Agent Eval Run — $TS" > "$OUT_FILE"
echo "" >> "$OUT_FILE"
echo "Webhook: $WEBHOOK" >> "$OUT_FILE"
echo "" >> "$OUT_FILE"

while IFS= read -r line; do
  [ -z "$line" ] && continue
  id=$(echo "$line" | python3 -c "import json,sys; print(json.loads(sys.stdin.read())['id'])")
  expected_persona=$(echo "$line" | python3 -c "import json,sys; print(json.loads(sys.stdin.read())['expected_persona'])")
  message=$(echo "$line" | python3 -c "import json,sys; print(json.loads(sys.stdin.read())['message'])")
  expect=$(echo "$line" | python3 -c "import json,sys; print(json.loads(sys.stdin.read())['expect'])")

  echo "## $id — expected: $expected_persona" | tee -a "$OUT_FILE"
  echo "" >> "$OUT_FILE"
  echo "**Message:** $message" >> "$OUT_FILE"
  echo "" >> "$OUT_FILE"
  echo "**Expected behaviour:** $expect" >> "$OUT_FILE"
  echo "" >> "$OUT_FILE"

  body=$(python3 -c "import json,sys; print(json.dumps({'message': sys.argv[1]}))" "$message")
  response=$(curl -sS -X POST "$WEBHOOK" -H 'content-type: application/json' -d "$body" --max-time 120 || echo '{"error":"request failed"}')

  persona=$(echo "$response" | python3 -c "import json,sys; d=json.loads(sys.stdin.read() or '{}'); print(d.get('persona',''))" 2>/dev/null || echo "")
  answer=$(echo "$response" | python3 -c "import json,sys; d=json.loads(sys.stdin.read() or '{}'); print(d.get('answer',''))" 2>/dev/null || echo "$response")

  echo "**Routed to:** $persona" >> "$OUT_FILE"
  echo "" >> "$OUT_FILE"
  echo "**Answer:**" >> "$OUT_FILE"
  echo "" >> "$OUT_FILE"
  echo "$answer" >> "$OUT_FILE"
  echo "" >> "$OUT_FILE"
  echo "---" >> "$OUT_FILE"
  echo "" >> "$OUT_FILE"

  if [ "$persona" = "$expected_persona" ]; then
    echo "  routed: OK ($persona)"
  else
    echo "  routed: MISS (got '$persona', expected '$expected_persona')"
  fi
done < "${ROOT}/eval/prompts.jsonl"

echo ""
echo "Full report: $OUT_FILE"
