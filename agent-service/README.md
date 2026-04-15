# CDPI Agent Service

A chat-backed agent system for the **vc.infra** white-label platform. Three CDPI personas — **Senior Technical Architect**, **Programs & Operations Officer**, and **Platform Guide** — produce advisory notes, pitch decks, blog posts, country proposals, and technical scope documents, grounded in a RAG corpus over CDPI decks, advisory notes, scope templates, and conversation transcripts.

Outputs are saved as artifacts on the **Agents** page (`/agent-output`) where they can be previewed, edited inline, and downloaded as **real CDPI-branded `.pptx`** (via Marp with a custom theme) or raw `.md`.

The work lives across two directories:

- **`agent-service/`** — the agent backend: n8n workflow, persona prompts, corpus ingestion, Marp theme, eval harness, docker-compose for Qdrant + TEI embeddings.
- **`ui-demo/`** — the Go white-label app: chatbot partial, Outputs page, `/api/agent/chat` proxy, artifact store, PPTX export handler.

Plus **`references/`** (at the repo root) — the RAG corpus the ingest script reads.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│  Browser (chatbot partial on every vc.infra page)                   │
│    • sessionStorage persists chat state across refresh               │
│    • CustomEvent + BroadcastChannel sync with the Outputs page       │
│    • lastPersona sent back to the server for router stickiness       │
└────────────────────────────┬────────────────────────────────────────┘
                             │  POST /api/agent/chat
                             │  { message, history, lastPersona, context }
                             ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Go app (ui-demo/internal/handler/agent.go)                          │
│    Same-origin proxy — forwards the body to the n8n webhook.          │
│    Auth + session live here when we add them.                        │
└────────────────────────────┬────────────────────────────────────────┘
                             │  POST /webhook/agent-chat
                             ▼
┌─────────────────────────────────────────────────────────────────────┐
│  n8n workflow "CDPI Agent Chat" (9 nodes, all HTTP Request)          │
│                                                                      │
│  Webhook → Config → Router LLM → Parse Router → Embed Query →        │
│  Qdrant Search → Build Main Request → Main LLM → Respond             │
│                                                                      │
│  Router LLM   : Claude Haiku 4.5 (temperature 0, one-word output)     │
│  Main LLM     : Claude Sonnet 4.6                                     │
│  Embeddings   : local TEI (bge-small-en-v1.5, 384 dim)                │
│  Vector store : Qdrant (collection `cdpi_corpus`, 384 dim)            │
└────────────────────────────┬────────────────────────────────────────┘
                             │ { persona, answer, conversationId }
                             ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Back to browser                                                     │
│    Chatbot renders the reply, splits on <!-- CDPI-ARTIFACT-BREAK --> │
│    and POSTs each block to /api/agent/outputs if it looks like a     │
│    real document (Marp frontmatter, H1, or advisory-note template).  │
│    CustomEvent fires so the Outputs page refreshes its list inline.  │
└─────────────────────────────────────────────────────────────────────┘

                    ┌─────────────────────────────────┐
                    │  Go artifact store (file-backed) │
                    │  ui-demo/agent-outputs.json      │
                    │    create / list / get / update  │
                    │    delete / export?format=md|pptx│
                    │    survives server restart       │
                    └─────────────────────────────────┘

                    ┌─────────────────────────────────┐
                    │  PPTX export pipeline            │
                    │    marp --pptx --theme-set       │
                    │    agent-service/marp/cdpi.css   │
                    │    CDPI brand: #5B3FE4 accent,   │
                    │    #EDE8FD lavender, Outfit font,│
                    │    white cards, green tags       │
                    └─────────────────────────────────┘
```

### The three personas

| Persona | Voice | Produces | Signature rules |
|---|---|---|---|
| **Senior Technical Architect** | First-person plural ("we"), CDPI-neutral, standards-first | Technical advisory notes (strict 9–13-word Ask/Why/Scope/Context lines + Options + Advice tables), technical scope documents, feature specifications | Every reply starts with `# Technical Advisory Note — …` as the first line. No preamble. Never refuses. |
| **Programs & Operations Officer** | First-person plural, adoption-first, three CDPI principles baked in | Pitch decks (Marp), country proposals, inter-departmental briefs, outreach playbooks, blog posts, content packs (blog + deck in one response) | Decks start with `---\nmarp: true\ntheme: cdpi\npaginate: true\n---` frontmatter, title slide uses `<!-- _class: title -->`. Honors the three CDPI principles (inclusion=scale, minimalism, usage-over-enrolment). |
| **Platform Guide** | Warm, plain-English, patient | Orientation, decision support (tier picking, use-case framing), curiosity answers ("what is", "how does") | Never writes multi-turn handoff briefs to imaginary colleagues. If asked for a formal document, responds with one paragraph + one copy-pasteable prompt the user can send to route to the right persona. |

### Routing

Three layers, checked in order:

1. **Deterministic regex override** (Parse Router code node). If the message contains an explicit output-type signal — `pitch deck`, `slide deck`, `country proposal`, `advisory note`, `technical note`, `technical scope`, etc. — force the correct persona immediately, beating both the router LLM and stickiness.
2. **Router LLM classification** (Claude Haiku 4.5, `temperature: 0`). Handles the ambiguous middle ground using the rules in `prompts/router.md`.
3. **Persona stickiness**. If neither of the above gives a strong signal and the previous turn was architect or programs, keep the previous persona (so continuation phrases like "revise slide 3", "tighten this", "yes do it" don't drift to guide).

## File layout

```
repo root/
├── agent-service/                        ← agent backend
│   ├── README.md                         (this file)
│   ├── docker-compose.agents.yml         Qdrant + local TEI embeddings + ingest container
│   ├── .env.example / .env               ANTHROPIC_API_KEY, N8N_API_KEY, optional VOYAGE_API_KEY
│   │
│   ├── prompts/
│   │   ├── architect.md                  Senior Technical Architect persona
│   │   ├── programs.md                   Programs & Operations Officer persona
│   │   ├── guide.md                      Platform Guide persona
│   │   └── router.md                     Intent classifier prompt (three classes + examples)
│   │
│   ├── workflows/
│   │   ├── chat.template.json            Editable n8n workflow template (placeholder Config node)
│   │   └── chat.built.json               Generated by build_and_import.py — not committed
│   │
│   ├── scripts/
│   │   ├── build_and_import.py           Substitutes prompts into template, creates n8n credentials,
│   │   │                                 PUTs workflow, activates it. Idempotent. Run after every
│   │   │                                 prompt edit.
│   │   └── bootstrap-network.sh          Legacy — only needed if the n8n container is launched
│   │                                     outside the cdpi-agents docker network.
│   │
│   ├── marp/
│   │   └── cdpi.css                      CDPI-branded Marp theme. #5B3FE4 accent, #EDE8FD lavender
│   │                                     surface, Outfit font (Google Font with Calibri fallback),
│   │                                     white .card class, green .tag class, title-slide variant.
│   │
│   ├── ingest/
│   │   ├── Dockerfile                    Python 3.12 slim + python-docx/pptx + qdrant-client + tiktoken
│   │   ├── requirements.txt
│   │   └── ingest.py                     Walks ../references/, converts docx/pptx/md/txt to text,
│   │                                     chunks at 500 tokens with 80-token overlap, embeds via
│   │                                     local TEI (default) or Voyage (fallback), upserts to Qdrant
│   │                                     with deterministic UUIDs so re-runs upsert cleanly.
│   │
│   └── eval/
│       ├── prompts.jsonl                 10 seed prompts (5 architect, 5 programs) with expected behavior
│       ├── run.sh                        Runs prompts against live webhook, writes timestamped Markdown report
│       └── out/                          Report history (gitignored)
│
├── references/                           ← RAG corpus (your source docs)
│   ├── Architecture_By_building_blocks/  VC / Payments / Identity / Data Sharing source material
│   ├── Decks and conversations/          Sri Lanka SLUDI transcript + CDPI website screenshot
│   └── scope_docs/                       DaaS v2.0 template + Tiered Packages doc
│
└── ui-demo/                              ← Go white-label app that fronts the agents
    ├── internal/handler/
    │   ├── agent.go                      POST /api/agent/chat proxy → n8n webhook
    │   └── agent_outputs.go              Artifact store (file-backed), CRUD API,
    │                                     PPTX export via marp, type detection, title extraction
    │
    ├── web/templates/partials/
    │   └── chatbot.html                  Floating widget. JS: fetch, history persistence via
    │                                     sessionStorage, auto-save filter (looksLikeDocument),
    │                                     content-pack splitter, CustomEvent + BroadcastChannel sync.
    │
    ├── web/templates/landing/
    │   └── agent_output.html             Dynamic Outputs page: tabs (All / Pitch Decks / Blog Posts
    │                                     / Country Proposals / Advisory Notes / Technical Scopes),
    │                                     two-column list+preview, inline markdown editor,
    │                                     Print/PDF, Download PPTX, Download MD, Delete.
    │
    ├── web/static/css/landing.css        Chatbot styles, outputs layout, saved-badge styles
    └── agent-outputs.json                Runtime persistence file (created at first save).
                                          Survives server restart. Gitignored.
```

## Prerequisites

- **Docker** (for Qdrant, TEI, and the ingest container)
- **Node.js** (any recent LTS — for `marp-cli`, installed globally)
- **Python 3** (stdlib only — for `build_and_import.py` and `eval/run.sh`)
- **Go 1.23+** (for building the `ui-demo` server)
- **n8n** running locally on port 5678 with the public REST API enabled
- An **Anthropic API key** (for Claude Sonnet 4.6 main + Haiku 4.5 router)
- **Marp CLI** (`npm install -g @marp-team/marp-cli`) — required for PPTX export

No OpenAI key is needed. Embeddings run locally via TEI with `BAAI/bge-small-en-v1.5` (384 dim).

## First-time setup

**1. Fill in `agent-service/.env`:**
```
N8N_URL=http://localhost:5678
N8N_API_KEY=<your n8n public API key from Settings → n8n API>
ANTHROPIC_API_KEY=sk-ant-...
# Optional — Voyage is supported as a fallback embedding provider
VOYAGE_API_KEY=
```

**2. Start Qdrant and the TEI embeddings server:**
```bash
cd agent-service
docker compose -f docker-compose.agents.yml up -d qdrant embeddings
```
TEI downloads ~130 MB on first start (the bge-small model weights). Health check:
```bash
curl http://localhost:8081/health                    # TEI
curl http://localhost:6333/collections               # Qdrant
```

**3. Make sure the n8n container is on the `cdpi-agents` docker network** so it can reach `http://qdrant:6333` and `http://embeddings:80` by hostname. Either:
- Launch n8n with `--network cdpi-agents` (preferred — survives restart), or
- Run `bash scripts/bootstrap-network.sh` once (attaches the existing n8n container to the network; needs to be re-run if n8n is recreated).

**4. Ingest the corpus:**
```bash
docker compose -f docker-compose.agents.yml --profile ingest run --rm ingest
```
Walks `../references/` recursively, converts docx/pptx/md/txt, chunks, embeds via TEI, upserts to Qdrant. Idempotent — re-run any time you add or change files in `references/`. Deterministic UUIDs mean re-runs update existing chunks in place without duplicates.

**5. Build and import the n8n workflow:**
```bash
python3 scripts/build_and_import.py
```
This script:
- Reads `prompts/architect.md`, `programs.md`, `guide.md`, `router.md`.
- Loads `workflows/chat.template.json` and injects the prompts into the Config node's JavaScript.
- Writes `workflows/chat.built.json` (for audit).
- Creates or reuses the `CDPI Agent — Anthropic` credential in n8n.
- Creates or updates the "CDPI Agent Chat" workflow via the n8n public API.
- Activates it.

Re-run any time you edit a prompt file. **Idempotent** — it updates the existing workflow rather than duplicating it.

**6. Install `marp-cli` globally** (for PPTX export):
```bash
npm install -g @marp-team/marp-cli
```

**7. Build and start the Go server:**
```bash
cd ../ui-demo
go build -o ./server ./cmd/server
./server -config config/default.json
```
`default.json` is the vc.infra-branded baseline config (`brand.name = "vc.infra"`, exploration mode, chatbot enabled). Other configs in `ui-demo/config/` pin different brands/backends for country-specific deployments.

Open **http://localhost:8080/** — you should see the vc.infra landing page with the 💬 chatbot button in the bottom-right corner.

## Daily workflow

### Editing a persona
1. Edit `prompts/architect.md`, `programs.md`, `guide.md`, or `router.md`.
2. Re-run `python3 scripts/build_and_import.py`. No Go rebuild needed — prompts live in the n8n workflow.
3. Open the chat and test.

### Adding corpus documents
1. Drop `.docx`, `.pptx`, `.md`, or `.txt` files into `references/` (any subdirectory works).
2. Re-run the ingest command. Only new / changed files cost embedding calls; existing files upsert in place.

### Editing the Marp theme
1. Edit `marp/cdpi.css`.
2. No rebuild needed for the agent side — the Go handler loads the CSS fresh on every PPTX export request.
3. Test by downloading a PPTX from the Outputs page.

### Editing the chatbot UI / Outputs page
1. Edit `ui-demo/web/templates/partials/chatbot.html`, `ui-demo/web/templates/landing/agent_output.html`, or `ui-demo/web/static/css/landing.css`.
2. **Rebuild the Go server** — templates and static files are embedded via `//go:embed` at build time:
   ```bash
   cd ui-demo && go build -o ./server ./cmd/server
   ```
3. Restart the server process.

### Adding an endpoint to the agent API
Handlers live in `ui-demo/internal/handler/agent.go` and `agent_outputs.go`. Routes are registered in `handler.go` under the `exploration_mode` block:
```
POST   /api/agent/chat
POST   /api/agent/outputs
GET    /api/agent/outputs
GET    /api/agent/outputs/{id}
PUT    /api/agent/outputs/{id}
DELETE /api/agent/outputs/{id}
GET    /api/agent/outputs/{id}/export?format=md|pptx
```
All unauthenticated — the chatbot is on the public landing page.

## How a chat turn flows end-to-end

**User sends a message** via the chatbot widget:
```json
{
  "message": "Draft a pitch deck for Uganda on LC1 verifiable credentials",
  "context": { "currentPath": "/agent-output", "currentTitle": "Agent Outputs" },
  "history": [ ... prior turns ... ],
  "lastPersona": "architect"
}
```

**Go proxy** (`agent.go`) forwards this verbatim to `http://localhost:5678/webhook/agent-chat`.

**n8n pipeline** (in order):

1. **Webhook** receives the body.
2. **Config** (Code node) loads the four prompts as JS constants, extracts `userMessage`, `conversationId`, `context`, `history`, `lastPersona`, and the `architectPrompt` / `programsPrompt` / `guidePrompt` / `routerPrompt`.
3. **Router LLM** sends a classification request to Claude Haiku 4.5 with `temperature: 0` and the router prompt. Returns one word: `architect`, `programs`, or `guide`.
4. **Parse Router** (Code node):
   - Applies the **deterministic output-type override** first (forces persona based on regex match on explicit output signals).
   - Falls back to the router LLM output if no override fires.
   - Falls back to **stickiness** (previous persona) if the message is a continuation phrase.
   - Selects the matching `personaPrompt`.
5. **Embed Query** POSTs the user message to the local TEI `/v1/embeddings` endpoint.
6. **Qdrant Search** top-5 nearest chunks in the `cdpi_corpus` collection. `onError: continueRegularOutput` so a missing collection doesn't break the pipeline.
7. **Build Main Request** (Code node):
   - Assembles the system prompt: `personaPrompt` + current-context block (if `context` provided) + retrieved corpus chunks.
   - Builds the Anthropic messages array from `history` + current user message.
8. **Main LLM** sends the full request to Claude Sonnet 4.6.
9. **Respond to Webhook** returns `{ persona, answer, conversationId, output }` to the Go proxy, which forwards it to the browser.

**Browser** receives the reply:
- Updates `conversationHistory`, `lastBotPersona`, `conversationId` in JS and persists to `sessionStorage`.
- Renders the message via the minimal markdown-to-HTML converter (handles H1–H6, bold, italic, lists, tables, code, HR).
- **Auto-save filter** runs: if the reply is from `architect` or `programs` AND is longer than 400 chars AND starts with Marp frontmatter OR an H1 heading OR `## Ask`, it's a document.
- **Content-pack split**: if the reply contains `<!-- CDPI-ARTIFACT-BREAK -->`, split on it and save each block as a separate artifact.
- **POST to `/api/agent/outputs`** — the Go handler auto-detects the artifact type and title, stores in memory and persists to `agent-outputs.json`.
- **Fires `window.dispatchEvent(new CustomEvent('cdpi:artifact-saved'))`** — any open `/agent-output` page listens and calls `refresh()` inline (same tab), and BroadcastChannel notifies any other open tabs.
- Attaches a passive "✓ Saved as X · view and download from the Outputs tab" badge to the bot message.

## Auto-save rules (what counts as a document)

A chat reply is only saved as an artifact if **all** of these are true:

1. Persona is `architect` or `programs` (Guide replies are never saved).
2. Length of the block is > 400 characters.
3. Block passes `looksLikeDocument()`:
   - Starts with `---\nmarp: true` (Marp deck), OR
   - First non-blank line is a markdown H1 (`# `), OR
   - First non-blank line matches `^## Ask` (strict advisory-note template).

Content-pack replies (blog post + deck in one response) are split on `<!-- CDPI-ARTIFACT-BREAK -->` and each block is filtered independently.

### Artifact type detection

Server-side in `agent_outputs.go::detectArtifactType`:

| If content… | Type |
|---|---|
| Starts with `# Blog:` | `blog_post` |
| Starts with `---\nmarp: true` frontmatter | `pitch_deck` |
| Contains "slide 1" or "pitch deck" | `pitch_deck` |
| Contains "country adoption proposal" or "adoption proposal" | `country_proposal` |
| Has `## Ask` + `## Why` + `## Scope` | `advisory_note` |
| Contains "advisory note" | `advisory_note` |
| Contains "technical scope" or "scope document" | `technical_scope` |
| Persona is architect (fallback) | `advisory_note` |
| Persona is programs (fallback) | `pitch_deck` |
| Else | `other` |

### Title detection

First `# H1` → first `## H2` → truncated source message → "Untitled output".

## PPTX export

`GET /api/agent/outputs/{id}/export?format=pptx` pipes the stored markdown through `marp` with the CDPI theme:

```bash
marp <tmp>/input.md \
  --pptx \
  --theme-set agent-service/marp/cdpi.css \
  --output <tmp>/output.pptx \
  --allow-local-files
```

The theme mirrors **cdpi.dev**'s visual language:
- Accent `#5B3FE4` (brand violet) — h1 underline, bullets, tag borders
- Lavender surface `#EDE8FD` — table headers, blockquote backgrounds, title-slide gradient
- **Outfit** font (Google Font, imported in the theme CSS) with Calibri Light fallback
- Title-slide variant via `<!-- _class: title -->` — huge h1, italic second line in accent violet, small "CDPI · Country · Date" footer
- `.card` class for white cards with subtle borders
- `.tag` class for green taxonomy pills (`#059669` on `#ecfdf5`) + `.tag-violet` variant
- `.quote` class for full-screen pull-quotes on navy background

Marp uses Chromium internally to render each slide, which means Google Fonts load live during conversion. For offline deployments, you'd need to download the Outfit font files and reference them locally in the CSS.

If `marp` is not installed or the theme file is missing, `/export?format=pptx` returns 500 with a clear error message; other formats (`md`) still work.

## Outputs page features (`/agent-output`)

- **Tabs**: All · Pitch Decks · Blog Posts · Country Proposals · Advisory Notes · Technical Scopes · Other
- **List pane** (left): cards sorted newest-first with type label, title, persona, creation date
- **Preview pane** (right): markdown rendered to HTML via a local converter that handles H1–H6, tables, lists, code, hr, bold/italic, and strips the Marp directive comments
- **Edit**: inline `<textarea>` that PUTs to `/api/agent/outputs/{id}` and re-renders
- **Print / PDF**: opens a clean printable HTML window with inline CSS, calls `window.print()` — user prints to PDF from the browser
- **Download PPTX**: real CDPI-branded PowerPoint via the Marp pipeline above
- **Download MD**: raw markdown with a clean filename derived from the title
- **Delete**: with a confirm prompt
- **Live updates**: when the chatbot saves a new artifact, the list refreshes inline via `window.addEventListener('cdpi:artifact-saved')` — no polling, no manual refresh needed
- **Cross-tab sync**: `BroadcastChannel('cdpi-agent-outputs')` ensures a second tab viewing `/agent-output` also updates

## Eval harness

```bash
bash eval/run.sh
```

- Reads `eval/prompts.jsonl` (10 seed prompts: 5 architect, 5 programs, each with expected behavior).
- POSTs each to `http://localhost:5678/webhook/agent-chat` (direct, bypassing the Go proxy).
- Writes a timestamped Markdown report to `eval/out/` with the routed persona, the answer, and the expected behavior for side-by-side manual review.
- Prints routing-accuracy summary to stdout.

First iteration is human-scored on purpose — read the report, note where answers drift from the expected CDPI voice or miss a principle, then tighten the persona prompt and re-import via `build_and_import.py`.

Add more test cases by appending JSONL entries with the shape:
```json
{"id":"arch-06","expected_persona":"architect","message":"...","expect":"expected behavior description"}
```

## Debugging tips

- **No artifacts in Outputs after chatting?**
  - Check that the response actually looks like a document (Marp frontmatter or `# H1` at position 0).
  - Check the browser console for fetch errors on `POST /api/agent/outputs`.
  - Check `ui-demo/agent-outputs.json` on disk — if it's `{}` the save isn't firing; if it has the artifact the Outputs page listener isn't refreshing.
- **Wrong persona routed?**
  - Inspect recent n8n executions via API:
    ```bash
    KEY=$(grep N8N_API_KEY agent-service/.env | cut -d= -f2)
    curl -s -H "X-N8N-API-KEY: $KEY" "http://localhost:5678/api/v1/executions?workflowId=<id>&limit=3&includeData=true"
    ```
  - Check the Router LLM output. If it disagrees with the Parse Router's final persona, the deterministic override or stickiness kicked in — look at the `userMessage` for explicit output signals.
- **PPTX export returns 500?**
  - `marp --version` to confirm it's installed on the Go server's PATH.
  - `ls agent-service/marp/cdpi.css` to confirm the theme file is present.
  - Check `/tmp/vcserver.log` for the exact marp error (typically a syntax issue in the generated markdown).
- **Chat state lost on page refresh?**
  - Check browser DevTools → Application → Session Storage for `cdpi.chatbot.state.v1`. If it's there, `restoreChatState()` should be running on load.
- **Outputs page list stale?**
  - Hard-refresh. The server sets `Cache-Control: no-store` on GET `/api/agent/outputs` so there shouldn't be cache issues, but some proxies and browsers ignore it.
- **Server keeps losing artifacts on restart?**
  - The persistence file is `ui-demo/agent-outputs.json`. If it's missing, check `AGENT_OUTPUTS_FILE` env var and the process CWD (logs show `agentArtifactStore: loaded N artifacts from <path>` on startup).
- **n8n can't reach Qdrant or TEI?**
  - `docker network inspect cdpi-agents` and confirm the `n8n` container is listed under `Containers`. If not, re-launch n8n with `--network cdpi-agents` or run `bash scripts/bootstrap-network.sh`.

## Known gaps (honest)

- **No visual QA loop on slides.** The pipeline is text-to-PPTX one-shot. If the agent produces layout that looks awkward in the rendered deck, there's no feedback loop that lets it render-images-and-critique before returning. Addable by extending the Go export handler to `marp --images png`, feeding the images to Claude with vision, and letting the persona regenerate.
- **No Postgres-backed artifact store.** The file-based store is fine for single-server exploration mode, but it won't scale to multi-user deployments or survive a container restart without a persistent volume mount. A Postgres-backed implementation behind the same `agentArtifactStore` interface would be ~40 lines of Go.
- **No per-user artifact scoping.** All artifacts are globally visible in the current deployment. When we add real auth, key the store on user ID.
- **DOCX export is not wired.** The Outputs page shows a disabled DOCX button with a "coming soon" tooltip. Would need `pandoc` installed on the Go server's host + a new branch in `agent_outputs.go::exportDOCX`.
- **No streaming responses.** Chat replies land all-at-once after Claude finishes. Streaming would need SSE or WebSocket from the Go proxy through to the browser, plus a reshape of the n8n webhook (it doesn't natively stream).
- **Operations corpus is lighter than architecture corpus.** Programs persona falls back to general DPI principles when retrieval doesn't surface programme-specific case studies. Closes by adding more programme material to `references/`.
- **No scheduled re-ingestion.** Corpus ingestion is manual. A cron job or file-watcher could run `ingest` on changes.
- **No LLM-as-judge eval scoring.** Eval runs are human-scored. An automated pass could check structural rules (e.g., "does this advisory note follow the 9–13 word constraint on Ask/Why/Scope/Context?") and surface drift.

## At a glance: running checklist

Once set up, the ongoing workflow is:

```bash
# Start infrastructure (once per boot)
cd agent-service
docker compose -f docker-compose.agents.yml up -d qdrant embeddings

# Start the Go app (once per boot, or after any template/CSS/Go edit)
cd ../ui-demo
go build -o ./server ./cmd/server
./server -config config/default.json

# Edit a persona → re-import
cd ../agent-service
$EDITOR prompts/programs.md
python3 scripts/build_and_import.py

# Add corpus docs → re-ingest
cp /path/to/doc.docx ../references/scope_docs/
docker compose -f docker-compose.agents.yml --profile ingest run --rm ingest

# Run the eval
bash eval/run.sh
cat eval/out/<latest>.md
```

Browser: **http://localhost:8080/** → 💬 to open the chat → ask for a pitch deck, advisory note, blog post, or content pack → artifact appears live in the Outputs tab → Download PPTX / MD / Print-to-PDF.
