#!/usr/bin/env python3
"""Build the CDPI Agent Chat workflow from the template + prompts, POST to n8n.

Reads:
  workflows/chat.template.json
  prompts/router.md
  prompts/architect.md
  prompts/programs.md
  .env (for N8N_API_KEY, N8N_URL)

Writes:
  workflows/chat.built.json     — the final workflow JSON, for audit
  (POSTs to $N8N_URL/api/v1/workflows)

Usage:
  python3 scripts/build_and_import.py          # create or update
  python3 scripts/build_and_import.py --dry    # build only, no HTTP
"""
import json
import os
import sys
import urllib.error
import urllib.request
from pathlib import Path


BASE = Path(__file__).resolve().parent.parent
DRY = "--dry" in sys.argv


def load_env() -> None:
    env_path = BASE / ".env"
    if not env_path.exists():
        return
    for raw in env_path.read_text().splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, _, val = line.partition("=")
        os.environ.setdefault(key.strip(), val.strip())


def read(path: str) -> str:
    return (BASE / path).read_text(encoding="utf-8")


def build_config_js(architect: str, programs: str, guide: str, router: str) -> str:
    return (
        "const architectPrompt = " + json.dumps(architect) + ";\n"
        "const programsPrompt = " + json.dumps(programs) + ";\n"
        "const guidePrompt = " + json.dumps(guide) + ";\n"
        "const routerPrompt = " + json.dumps(router) + ";\n"
        "const input = $input.first().json;\n"
        "const body = (input && input.body) ? input.body : input;\n"
        "// Handle both webhook ({message, conversation_id, context, history}) and Chat Trigger ({chatInput, sessionId}) shapes\n"
        "const userMessage = (body && (body.message || body.chatInput)) || input.chatInput || '';\n"
        "const conversationId = (body && (body.conversation_id || body.conversationId || body.sessionId)) || input.sessionId || null;\n"
        "const context = (body && body.context) || null;\n"
        "const history = (body && Array.isArray(body.history)) ? body.history : [];\n"
        "const lastPersona = (body && typeof body.lastPersona === 'string') ? body.lastPersona : null;\n"
        "return [{ json: { architectPrompt, programsPrompt, guidePrompt, routerPrompt, userMessage, conversationId, context, history, lastPersona } }];"
    )


def api(method: str, path: str, body: dict | None = None) -> dict:
    url = os.environ["N8N_URL"].rstrip("/") + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(
        url,
        data=data,
        headers={
            "X-N8N-API-KEY": os.environ["N8N_API_KEY"],
            "Content-Type": "application/json",
            "Accept": "application/json",
        },
        method=method,
    )
    try:
        with urllib.request.urlopen(req) as r:
            raw = r.read()
            return json.loads(raw) if raw else {}
    except urllib.error.HTTPError as e:
        print(f"HTTP {e.code} {e.reason} on {method} {path}", file=sys.stderr)
        print(e.read().decode(errors="ignore"), file=sys.stderr)
        raise


def find_existing(name: str) -> dict | None:
    resp = api("GET", "/api/v1/workflows")
    for wf in resp.get("data", []):
        if wf.get("name") == name:
            return wf
    return None


CRED_NAME_ANTHROPIC = "CDPI Agent — Anthropic"
CRED_NAME_VOYAGE = "CDPI Agent — Voyage"


def find_credential(name: str) -> dict | None:
    try:
        resp = api("GET", "/api/v1/credentials")
        for c in resp.get("data", []):
            if c.get("name") == name:
                return c
    except Exception:
        pass
    return None


def ensure_anthropic_credential(api_key: str) -> str:
    existing = find_credential(CRED_NAME_ANTHROPIC)
    if existing:
        print(f"Credential '{CRED_NAME_ANTHROPIC}' already exists (id={existing['id']})")
        return existing["id"]
    # header:false is a workaround for a quirk in the anthropicApi schema where
    # the if-clause always matches unless header is explicitly false.
    payload = {
        "name": CRED_NAME_ANTHROPIC,
        "type": "anthropicApi",
        "data": {"apiKey": api_key, "header": False},
    }
    result = api("POST", "/api/v1/credentials", payload)
    print(f"Created credential '{CRED_NAME_ANTHROPIC}' (id={result['id']})")
    return result["id"]


def ensure_voyage_credential(api_key: str) -> str:
    existing = find_credential(CRED_NAME_VOYAGE)
    if existing:
        print(f"Credential '{CRED_NAME_VOYAGE}' already exists (id={existing['id']})")
        return existing["id"]
    payload = {
        "name": CRED_NAME_VOYAGE,
        "type": "httpHeaderAuth",
        "data": {"name": "Authorization", "value": f"Bearer {api_key}"},
    }
    result = api("POST", "/api/v1/credentials", payload)
    print(f"Created credential '{CRED_NAME_VOYAGE}' (id={result['id']})")
    return result["id"]


def bind_credentials(workflow: dict, anthropic_id: str | None, voyage_id: str | None) -> None:
    for node in workflow["nodes"]:
        name = node.get("name")
        if name in ("Router LLM", "Main LLM") and anthropic_id:
            node.setdefault("credentials", {})["anthropicApi"] = {
                "id": anthropic_id,
                "name": CRED_NAME_ANTHROPIC,
            }
        elif name == "Embed Query" and voyage_id:
            node.setdefault("credentials", {})["httpHeaderAuth"] = {
                "id": voyage_id,
                "name": CRED_NAME_VOYAGE,
            }


def strip_readonly(workflow: dict) -> dict:
    # n8n public API rejects unknown fields on POST — keep only the writable set.
    return {k: workflow[k] for k in ("name", "nodes", "connections", "settings") if k in workflow}


def main() -> None:
    load_env()

    template = json.loads(read("workflows/chat.template.json"))
    architect = read("prompts/architect.md")
    programs = read("prompts/programs.md")
    guide = read("prompts/guide.md")
    router = read("prompts/router.md")

    config_js = build_config_js(architect, programs, guide, router)
    for node in template["nodes"]:
        if node.get("name") == "Config":
            node["parameters"]["jsCode"] = config_js
            break
    else:
        print("ERROR: Config node not found in template", file=sys.stderr)
        sys.exit(1)

    built_path = BASE / "workflows" / "chat.built.json"
    built_path.write_text(json.dumps(template, indent=2))
    print(f"Built: {built_path}")

    if DRY:
        print("Dry run — not importing.")
        return

    if not os.environ.get("N8N_API_KEY"):
        print("ERROR: N8N_API_KEY not set (in .env or env).", file=sys.stderr)
        sys.exit(1)
    os.environ.setdefault("N8N_URL", "http://localhost:5678")

    anthropic_id = None
    if os.environ.get("ANTHROPIC_API_KEY"):
        anthropic_id = ensure_anthropic_credential(os.environ["ANTHROPIC_API_KEY"])
    else:
        print("WARN: ANTHROPIC_API_KEY not set — Router LLM and Main LLM will fail until it is.")

    # Embed Query now hits local TEI (no auth). Voyage credential is left in n8n
    # as a warm fallback but not bound to any node by default.
    bind_credentials(template, anthropic_id, None)
    # Re-write built file with bindings attached so it's audit-friendly.
    built_path.write_text(json.dumps(template, indent=2))

    payload = strip_readonly(template)

    existing = find_existing(template["name"])
    if existing:
        wf_id = existing["id"]
        print(f"Updating existing workflow id={wf_id}")
        result = api("PUT", f"/api/v1/workflows/{wf_id}", payload)
    else:
        print("Creating new workflow")
        result = api("POST", "/api/v1/workflows", payload)
        wf_id = result.get("id")
        print(f"Created workflow id={wf_id}")

    active = result.get("active", False)
    if not active and wf_id:
        print("Activating workflow...")
        api("POST", f"/api/v1/workflows/{wf_id}/activate")
        print("Activated.")

    base = os.environ["N8N_URL"].rstrip("/")
    print(f"Webhook URL: {base}/webhook/agent-chat")
    print("Send test: curl -X POST $URL -H 'content-type: application/json' -d '{\"message\":\"hello\"}'")


if __name__ == "__main__":
    main()
