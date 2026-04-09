# VC White-Label App — Implementation Plan

**Version:** 1.0  
**Date:** 06 April 2026  
**Author:** Adam Mwaniki  
**Status:** Draft — pending approval before build

---

## 1. Purpose

This document defines the full UI/UX architecture for a white-label verifiable credentialing application. The system is backend-agnostic, standards-compliant (W3C VC 2.0, OID4VCI, OID4VP, DIDComm v2), and designed to serve every process in the credential lifecycle across all ecosystem roles.

It is aligned to the CDPI DaaS Scope Document v2.0 requirements matrix and designed for interoperable, country-scale deployment.

The application is structured as a product that teaches, demonstrates, and delivers. It serves users who don't yet know what verifiable credentials are (exploration), users evaluating whether to adopt (decision support), and users operating live infrastructure (production). A single codebase serves all three stages — the exploratory mockups *are* the production UI running against sandbox or live backends.

---

## 2. Product Framework: Inputs → Processes → Outputs → Feedback Loop

Every interaction in the application follows a four-step cycle:

| Step | What it means | Example (Issuer) | Example (Explorer) |
|---|---|---|---|
| **Inputs** | What enters the system — data, decisions, context | Attested CSV of graduates | "I'm a ministry official exploring digital credentials for land registry" |
| **Processes** | What the system does with the input | Schema validation → credential signing → batch dispatch | Chatbot triage → role identification → guided mockup walkthrough |
| **Outputs** | What the system produces | Signed credentials in multiple formats, delivery confirmations | Tailored pitch deck, technical scope document, or interactive demo |
| **Feedback Loop** | What comes back to improve the next cycle | Delivery rates, verification volumes, holder acceptance rates | User satisfaction, adoption funnel metrics, content gap identification |

This framework applies at every level: a single credential issuance, an issuer onboarding, an entire country rollout, and the exploration journey of a first-time visitor.

---

## 3. Landing Page & Exploration Layer

### 3.1 Entry Point

The application opens with a single question:

> *"One ecosystem, many journeys. What can verifiable credentials do for you? What can you do for verifiable credentials infrastructure?"*

The landing page serves seven audience types: governments, institutions, citizens, funders, advisors, program managers, and developers. Each arrives with different knowledge levels and intent. The page does not assume prior knowledge of VCs.

### 3.2 Exploration Mode

For users who are learning, evaluating, or deciding — before any commitment to deploy.

| Feature | What it does |
|---|---|
| **Explanatory screens** | Progressive disclosure: what VCs are → how they work → what they enable → what it costs → who's done it. Written for non-technical readers. Optional AI chatbot toggle for conversational exploration. |
| **Interactive mockups** | Live UI components running against a sandbox backend with sample data. Users can customize these for their specific flows — select a sector (education, land, health, identity), choose roles (issuer, holder, verifier), and walk through a complete credential lifecycle with their own scenario. The mockups *are* the production UI — same codebase, sandbox data. |
| **Backend explainers** | Expandable guides attached to each mockup screen explaining what's happening technically — which standards and protocols are in play (W3C-VCDM 2.0, OID4VCI, Data Integrity, etc.). Two depth levels: executive summary (one sentence) and technical detail (protocol-level). |
| **Requirements mapping** | Each mockup screen maps to specific requirement IDs in the DaaS Scope v2.0 (CI-01, CV-02, etc.) — visible as an overlay toggle. Shows which DaaS requirements a given screen satisfies. |
| **Tiered package mapping** | Each mockup maps to the tier structure in Tiered Packages 1.0 — showing which capabilities are included in which package tier. Users can see exactly what they get at each level. |

**Screens (new):**

| ID | Screen | What it does |
|---|---|---|
| 78 | **Landing Page** | Entry point with audience selector; "What brings you here?" prompt; routes to exploration or production portal |
| 79 | **VC Explainer Journey** | Progressive disclosure screens: What are VCs → How they work → Use cases by sector → Global adoption evidence → Cost/benefit framing |
| 80 | **Interactive Mockup Launcher** | Select sector + role → launch sandbox-backed mockup of the full flow; customizable scenario parameters |
| 81 | **Standards & Protocol Explorer** | Expandable overlays on mockup screens; executive and technical depth levels; links to W3C specs |
| 82 | **Requirements Matrix Mapper** | Overlay showing DaaS v2.0 requirement IDs satisfied by each screen/feature; filterable by component (CI, CV, CS) |
| 83 | **Tiered Package Mapper** | Shows which package tier includes which capabilities; interactive comparison; "what do I need?" guided selector |

### 3.3 Unified Production Portal

For users operating live infrastructure. The production portal is the same UI as the mockups, connected to live backends instead of sandbox.

| Entry | What it does |
|---|---|
| **Role selector** | "Select your role: Issuer, Holder, Verifier, Auditor, Admin." Routes to the appropriate workspace. Default is Holder/Wallet — the most common user. |
| **Issuer workspace** | Screens 9–32 from this plan |
| **Holder/Wallet workspace** | Screens 33–42 from this plan |
| **Verifier workspace** | Screens 43–51 from this plan |
| **Audit portal** | Dedicated auditor view — read-only access to cross-workspace audit logs, compliance reports, credential lifecycle trails, and ecosystem-wide verification statistics. Extends Screen 7 (Audit Log) with auditor-specific dashboards. |

**Screens (new):**

| ID | Screen | What it does |
|---|---|---|
| 84 | **Role Selector** | Production portal entry; role-based routing; remembers last role |
| 85 | **Auditor Dashboard** | Cross-workspace read-only view; compliance reports; credential lifecycle trails; ecosystem verification stats; exportable audit packages |

### 3.4 AI Chatbot & Agent Router

A chatbot is available throughout the application — on the landing page, in exploration mode, and in the production portal. It serves three functions:

**Function 1: Guided exploration.** Walks users through mockups, answers "what is this?" and "why would I need this?" questions, suggests which screens to visit based on stated needs.

**Function 2: Decision support.** Helps users determine which package tier they need, which use cases to prioritize, which stakeholders to involve. Asks clarifying questions to narrow scope.

**Function 3: Document generation.** When a user is ready for outputs, the chatbot performs a quick triage to route to the appropriate agent:

| Triage question | What it determines |
|---|---|
| "What output do you need?" | The routing signal. Not "who are you?" but "what do you want to walk away with?" |
| Pitch deck, adoption proposal, adoption brief, outreach material → | **Operations & Program Management Agent** |
| Technical scope document, feature spec, implementation advice, advisory note → | **Technical Architect Agent** |

The triage routes on **desired output**, not user role. A government CTO might want a pitch deck (→ Program agent) or a technical integration spec (→ Technical agent) depending on whether they're persuading a minister or briefing an engineering team.

**Agent details:**

| Agent | Tone | Outputs | Context |
|---|---|---|---|
| **Operations & Program Management** | CDPI operations advisory — authoritative, accessible, policy-aware, outcome-focused. Leads with problems and measurable outcomes, not technology. | Pitch decks, country adoption proposals, inter-departmental adoption briefs, outreach playbooks | Implementation plan + DaaS scope + VerifyTT learnings + country-specific context from user |
| **Technical Architect** | CDPI technical advisory — precise, standards-aware, implementation-oriented. References requirement IDs, screen numbers, protocol names. | Technical scope documents, feature specifications, implementation advice, technical advisory notes | Implementation plan + DaaS scope + VerifyTT learnings + technical context from user |

Both agents have access to the full document corpus (this implementation plan, DaaS scope v2.0, VerifyTT learnings, VerifyTT user manual) as persistent context. They generate documents that can be exported as PPTX, DOCX, PDF, or markdown.

**Screens (new):**

| ID | Screen | What it does |
|---|---|---|
| 86 | **Chatbot Interface** | Persistent side panel or modal; available on every screen; context-aware (knows which screen the user is viewing); conversation history persisted per session |
| 87 | **Agent Output Viewer** | Renders generated documents (pitch deck preview, scope doc preview); export controls (PPTX, DOCX, PDF, MD); revision request flow ("make it shorter," "add budget section," "change tone") |

### 3.5 Exploration-to-Production Continuity

The critical architectural decision: **the mockups and the production UI are the same codebase.** The difference is the backend connection:

| Mode | Backend | Data | Auth | Visual indicator |
|---|---|---|---|---|
| **Exploration / Sandbox** | Sandbox backend with sample data | Synthetic — clearly labeled "SAMPLE DATA" | Optional (guest access or lightweight signup) | Persistent banner: "You are exploring with sample data" |
| **Production** | Live backend (Inji, Credebl, Walt.id, Quark.id, custom) | Real credentials | Full SSO auth per deployment | No banner; live status indicators |

A user who has explored the system in sandbox mode can transition to production by connecting a live backend — the UI they learned on is the UI they operate. Zero retraining cost.

---

## 4. Design Philosophy

Derived from the mwaniki.dev brand identity:

- **Minimal by default.** No decoration for decoration's sake. Every element earns its place.
- **Typographic precision.** Information hierarchy driven by type weight and spacing, not color or ornament. Josefin Sans for headings, emphasis, and display text. Geist for body copy, labels, and data. This pairing gives the UI a distinctive editorial voice (Josefin Sans) grounded by a clean, functional reading experience (Geist).
- **Light-first, theme-switchable.** Default light palette; full dark mode; all colors config-driven for white-label deployments. Each deployment can override the default theme direction.
- **Silent competence.** "Great software doesn't announce itself; it just belongs."

The UI must work for a government ministry deploying national ID credentials and a rural field agent verifying a land certificate on a low-bandwidth connection. Inclusivity is a design constraint, not an afterthought.

---

## 5. Learnings from VerifyTT Implementation

The Government of Trinidad and Tobago deployed VerifyTT — a national credentialing platform built on the INJI DPG stack — for education credential issuance and verification. The implementation surfaced operational, architectural, and ecosystem-level challenges that directly inform this white-label plan. Key actors included iGovTT (platform operator), University of the West Indies, University of Trinidad and Tobago, and WeLearnTT (education registry).

These learnings are incorporated as explicit design constraints throughout the plan.

### 5.1 Issuer Onboarding Was the Primary Bottleneck

**Problem:** Onboarding a new issuer in the baseline INJI framework required 1–2 days of engineering effort. It involved manual creation of issuer identities, key/certificate configuration, Kubernetes deployments, credential registry updates, and DevOps-driven service restarts. This required specialized knowledge of platform architecture and was not sustainable at national scale.

**Solution deployed:** A centralized Admin Portal with self-service onboarding reduced issuer onboarding from 1–2 days to minutes. The portal captures issuer organization info, identifiers, access settings, and registry configs, then triggers an automated backend pipeline for infrastructure provisioning — including service deployment, key generation, API configuration, and IAM setup.

**Design constraint for this plan:** Screens 9–10 (Issuer Registration Form, Onboarding Queue) must trigger fully automated provisioning pipelines. Zero engineering intervention for standard onboarding. The Admin Portal must also support issuer lifecycle management — update, suspend, remove — without DevOps involvement (Screen 28).

### 5.2 Credential Lifecycle Operations Were API-Only

**Problem:** Schema creation, template configuration, credential issuance, and revocation were all API-driven in the baseline platform. Non-technical issuer institutions (universities, training orgs, certification bodies) could not perform routine operations without engineering support.

**Solution deployed:** A dedicated Issuance Portal with UI-driven schema management, visual certificate template design (HTML-based with institutional branding, dynamic field mapping, QR code integration), single and bulk issuance, and revocation — all without API interaction.

**Design constraint for this plan:** The schema builder (Screen 13) and template editor (Screen 15) must be fully visual, zero-code tools. The template editor must support HTML/SVG customization with live preview, institutional branding (logo, colors), and dynamic field mapping from schema attributes. Bulk issuance (Screen 18) must support CSV upload with status monitoring per batch job.

### 5.3 External Registry Integration Was Not Native

**Problem:** Credential issuance needed validation against authoritative external registries (e.g., WeLearnTT education registry). Native integrations between eSignet (identity), Certify (issuance), and external data registries were not available out of the box. Custom integration plugins had to be built per registry.

**Solution deployed:** Custom plugin architecture — each external registry gets a dedicated integration plugin (e.g., welearn-tt-certify-integration-impl) that bridges eSignet identity validation, Certify issuance workflows, and external data validation.

**Design constraint for this plan:** The Adaptor Registry (Screen 58) and Adaptor Configuration (Screen 59) must support a plugin model for external registry integrations. Each adaptor must be configurable without code changes — endpoint URLs, auth credentials, data mapping rules, validation logic. The UI must expose a replicable pattern: "configure once for WeLearnTT, replicate for any other registry." This also reinforces the wallet-initiated flow — the Issuer Review Queue (Screen 24) must be able to validate holder requests against external registries before approving issuance.

### 5.4 Ecosystem Visibility Was Absent

**Problem:** The platform initially lacked consolidated dashboards for ecosystem-level metrics. Administrators could not see credential issuance trends, verification volumes, issuer participation levels, or platform usage patterns. Without this, governance and adoption monitoring were impossible.

**Solution deployed:** Centralized operational dashboards showing total credentials issued, verification volumes, active issuers, revocation statistics, and growth trends. Also per-issuer analytics: credentials issued, downloaded, verified, revoked, and verification trends over time.

**Design constraint for this plan:** The Platform Health Dashboard (Screen 77) must provide ecosystem-wide metrics. The Issuer Integration Status (Screen 32) must include per-issuer analytics. The Reporting Dashboard (Screen 75) must serve multiple stakeholder tiers — the VerifyTT experience confirmed that funders, DPI owners, and DPG owners need fundamentally different views of the same data.

### 5.5 Multi-Tenancy Is Required for Scale

**Problem:** Each issuer required dedicated infrastructure resources (separate service deployments, isolated compute). This provided strong isolation but created escalating infrastructure costs, operational complexity, and scalability limits as issuer count grew.

**Future constraint for this plan:** The deployment architecture must support both single-tenant (strong isolation, current INJI model) and multi-tenant (shared infrastructure, logical isolation) modes. The Deployment Configuration (Screen 73) must expose this as a toggle. Multi-tenant mode must ensure logical data isolation while sharing compute — this is a backend architecture decision, but the UI must surface tenant boundaries clearly (e.g., an issuer admin never sees another issuer's data, even on shared infrastructure).

### 5.6 Capacity Building Was Critical for Adoption

**Problem:** Digital credentialing concepts (W3C VCs, decentralized identity, cryptographic verification, revocation registries, schemas) were unfamiliar to issuer institutions. Without training, institutions could not independently operate the platform.

**Solution deployed:** Structured capacity-building sessions covering VC fundamentals, issuance workflows, verification processes, revocation mechanisms, security, and governance.

**Design constraint for this plan:** The Training Hub (Screen 76) must be more than a document repository. It must include interactive onboarding flows — guided walkthroughs for first-time issuer admins, sandbox exercises, and concept explainers embedded in context (e.g., a "What is a schema?" tooltip in the schema builder, not just a separate training page).

### 5.7 Security Must Be Proactive, Not Post-Deployment

**Problem:** Security assessments during the project lifecycle identified vulnerabilities that required remediation. Relying on periodic post-deployment reviews was insufficient.

**Design constraint for this plan:** The Platform Health Dashboard (Screen 77) must include security posture indicators — dependency vulnerability status, security header compliance, last penetration test date, and remediation tracking. This is observability, not implementation — the actual scanning and hardening are backend operations.

### 5.8 Verification Must Support Multiple Modalities From Day One

**Confirmed from VerifyTT:** The verification portal supported three modalities out of the box — QR code upload, QR code scan (camera), and Verifiable Presentation (VP) request via generated QR. The VP flow involved selecting credential type, generating a request QR, and having the holder scan it from their wallet.

**Design constraint for this plan:** The Verification Portal (Screen 45) must support all three modalities natively. The QR/Deep Link Generator (Screen 44) must support VP request generation with credential type selection. These are not optional or Phase 2 features — they must be present at launch.

### 5.9 Wallet UX Patterns Validated

**Confirmed from VerifyTT mobile app (Inji-based):** The holder experience followed this pattern: onboarding walkthrough → wallet home → add credential (select issuer from list → authenticate with issuer-specific credentials → receive credential) → view stored credentials → share via QR/link/selective disclosure → backup and restore via cloud account.

**Design constraint for this plan:** The Holder workspace (Screens 33–42) must support this validated flow. Key UX requirements: issuer discovery list (not just a URL), issuer-specific authentication (StudentID + Graduation Date in the VerifyTT case — each issuer defines its own auth), credential card view with detail expansion, sharing via multiple channels, and wallet backup/restore. The wallet-initiated flow (Screens 22–26) extends this pattern by adding a request-and-wait step before credential receipt.

---

## 6. Architecture Overview

### 6.1 Role-Based Workspaces

The application is organized into six workspaces, each accessible based on RBAC assignment:

| Workspace | Primary Users | Purpose |
|---|---|---|
| **Issuer** | Credential Officers, Issuer Admins | Schema design, credential issuance, status management |
| **Holder / Credential Store** | Citizens, Guardians | Receive, store, manage, present credentials |
| **Verifier / Accepting Entity** | Banks, Employers, Government agencies | Request and validate credential presentations |
| **Trust Infrastructure** | Platform Admins, DPI Owners | Schema registries, issuer directories, governance config |
| **Interoperability & Adaptors** | Platform Admins, Integration Engineers | Cross-DPG bridges, protocol translation, last-mile channels |
| **Platform Admin** | Super Admins, DPI Owners | RBAC, deployment config, reporting, data portability |

### 6.2 Backend Adapter Interface

The UI calls abstract methods — `issueCredential()`, `verifyPresentation()`, `resolveSchema()`, etc. Each deployment wires in its own provider. Supported stacks include but are not limited to:

- Inji (Certify, Verify, Web, Mobile)
- Credebl
- Walt.id
- Quark.id
- Custom ISV offerings
- Any stack compliant with W3C VC 2.0 and the DaaS functional requirements

The adapter interface is defined as a typed contract. Swapping backends requires zero UI code changes.

### 6.3 White-Label Configuration

All branding is driven by a single configuration object:

```
{
  "brand": {
    "name": "string",
    "logo": { "light": "url", "dark": "url" },
    "colors": {
      "primary": "hex",
      "secondary": "hex",
      "accent": "hex",
      "background": "hex",
      "surface": "hex",
      "text": "hex",
      "textSecondary": "hex",
      "success": "hex",
      "warning": "hex",
      "error": "hex"
    },
    "typography": {
      "displayFont": "Josefin Sans",
      "bodyFont": "Geist",
      "monoFont": "Geist Mono"
    },
    "locale": {
      "default": "string",
      "supported": ["string"],
      "rtl": "boolean"
    }
  }
}
```

---

## 7. Shared Platform Shell

These components are shared across all workspaces.

### 7.1 Authentication & SSO Adapter

**Req refs:** CS-02, Section 7 Component 4

The auth layer adapts to whichever SSO provider is deployed. The UI renders login flows; it does not implement auth logic.

| Feature | Details |
|---|---|
| **Pluggable SSO** | eSignet, WSO2 Identity Server, Keycloak, custom country SSO |
| **Auth modalities** | Knowledge-based, OTP, biometric, face auth |
| **Session management** | Role-aware session with workspace routing |
| **Delegated auth** | Guardian login on behalf of minor (CI-16, CS-04) |

**Screens:**
1. **Login** — SSO-provider-adaptive login form; modality selector
2. **Guardian Access** — identity verification + minor linkage validation against authoritative registry

### 7.2 Role-Based Access Control

**Req ref:** CI-03

Four minimum roles, extensible per deployment:

| Role | Permissions |
|---|---|
| **Super Admin** | Platform-wide management, RBAC config, deployment settings |
| **Issuer Admin** | Schema/template/user management within issuer org |
| **Credential Officer** | Credential issuance only |
| **Auditor** | Read-only access to logs and reports |

**Screens:**
3. **RBAC Manager** — create, edit, assign roles; permission matrix view
4. **User Directory** — search, filter, manage users across orgs

### 7.3 Notification Hub

**Req ref:** CV-08

Multi-channel notification engine for issuance, storage, and verification events.

| Channel | Use |
|---|---|
| **In-app** | Real-time toast + notification center |
| **Email** | Credential ready, verification request, status changes |
| **SMS** | OTP delivery, credential availability (last-mile) |
| **Push** | Mobile wallet events |

**Screens:**
5. **Notification Center** — filterable feed of all events; mark read; link to relevant screen
6. **Notification Config** — per-module admin: which events trigger which channels

### 7.4 Audit & Activity Log

**Req ref:** CI-03 (Auditor role), Section 9.2

Immutable event stream across all modules. Privacy-preserving — minimal holder PII in logs.

**Screens:**
7. **Audit Log** — timestamped events; filterable by module, actor, action, outcome; exportable CSV/JSON
8. **Activity Dashboard** — visual summary: events/day, errors, latency

---

## 8. Issuer Workspace

### 8.1 Issuer Onboarding

**Req refs:** CI-01, CI-02, CI-04

Self-service portal for new issuers to join the ecosystem.

| Feature | Details |
|---|---|
| **Registration form** | Legal entity name, registration number, authorized signatories, technical contacts, DID/signing key config, geographic jurisdiction |
| **Approval workflows** | Configurable: manual approval, automated verification, hybrid |
| **DID provisioning** | Auto-generate or upload DID document (did:web, did:key), X.509 certificates, public keys |
| **Trust registry registration** | Automatic registration of issuer identity in trust registry on approval |

**Screens:**
9. **Issuer Registration Form** — multi-step guided form with validation
10. **Onboarding Queue** — admin view of pending/approved/rejected applications
11. **DID & Key Manager** — generate, upload, rotate keys; view trust registry status

### 8.2 Schema & Template Design

**Req refs:** CI-06, CI-15

| Feature | Details |
|---|---|
| **Schema Designer** | Visual builder for credential schemas; JSON/XML/JSON-LD; align with schema.org; version control; multi-credential-type support |
| **Template Designer** | Visual editor for human-readable output (PDF/SVG): logo, color themes, layout, language; QR code content config |
| **Preview** | Live preview of rendered credential in all output formats |

**Screens:**
12. **Schema List** — browse, search, version schemas
13. **Schema Builder** — drag-and-drop claim fields; type constraints; required/optional; import/export JSON-LD or JSON Schema
14. **Template List** — browse credential display templates
15. **Template Editor** — WYSIWYG for PDF/SVG layout; logo upload; color theming; language variants
16. **Credential Preview** — render sample credential in all formats (wallet card, PDF, QR, JSON)

### 8.3 Issuance Flows

There are two fundamentally different initiation patterns. The UI must serve both independently.

#### 8.3.1 Issuer-Initiated Flow

The issuer decides when to issue. The holder is a recipient, not a requester.

**Req refs:** CI-08, CI-09, CI-10, CI-11, CI-12, CI-19

| Step | Screen | What happens |
|---|---|---|
| 1 | **Credential Campaign Builder** | Define a batch or event-triggered issuance: select schema, define population (CSV/Excel upload, registry query, API trigger), set delivery channels |
| 2 | **Data Attestation & Review** | Upload/connect attested data; preview generated credentials before signing; validation (schema compliance, duplicate detection, data completeness) |
| 3 | **Signing & Dispatch** | Bulk sign → dispatch to holder's chosen store (eLocker, wallet push, download portal, print queue); delivery status tracking per credential |
| 4 | **Holder Receive & Accept** | Holder gets notification → reviews offered credential (full claim inspection) → accepts into store or rejects with reason |
| 5 | **Delivery Confirmation Dashboard** | Issuer sees: issued / delivered / accepted / rejected / pending — per credential and aggregate |

Performance target: minimum 10,000 credentials/hour with <2 second latency per credential for on-demand issuance (CI-19).

**Screens:**
17. **Campaign Builder** — schema selector, population definition, channel config
18. **Batch Upload** — CSV/Excel drag-and-drop; column mapping; validation report; error correction
19. **Issuance Review** — preview sample credentials from batch; approve/reject before signing
20. **Dispatch Monitor** — real-time issuance progress; per-credential delivery status
21. **Single Issuance Form** — on-demand: select schema, enter/lookup holder, populate claims, review, issue

#### 8.3.2 Wallet-Initiated (Holder-Requested) Flow

The holder decides they need a credential and requests it. The issuer responds.

| Step | Screen | What happens |
|---|---|---|
| 1 | **Credential Catalog** | Holder browses available credential types across issuers; filter by sector, issuer, jurisdiction; shows required claims, auth requirements, estimated processing time |
| 2 | **Request Builder** | Holder selects credential type → authenticates identity per issuer requirements → submits request with any required supporting evidence |
| 3 | **Issuer Review Queue** | Issuer receives request → automated or manual validation against authoritative registry → approve / reject / request additional info |
| 4 | **Async Status Tracker** | Holder sees: submitted → under review → approved → credential ready → collected. Supports long-running processes with notification hooks |
| 5 | **Credential Delivery** | On approval, credential pushed to holder's preferred store; holder reviews and accepts |

**Screens:**
22. **Credential Catalog** — public-facing directory of available credential types
23. **Credential Request Form** — holder authentication + evidence submission
24. **Request Queue (Issuer side)** — incoming requests; bulk approve/reject; request additional info
25. **Request Status Tracker (Holder side)** — timeline view of request lifecycle
26. **Request Analytics (Issuer side)** — volume, approval rates, processing time, bottlenecks

### 8.4 Status Management

**Req refs:** CI-13, CI-14

| Feature | Details |
|---|---|
| **Individual actions** | Revoke, suspend, reinstate any issued credential |
| **Batch actions** | Bulk status changes via selection or filter |
| **Workflow-based** | Optional approval workflow for revocation/suspension |
| **Status list visualization** | Bitstring status list view; StatusList2021, CRL, OCSP support |

**Screens:**
27. **Issued Credentials Dashboard** — searchable list; status indicators; bulk actions
28. **Credential Detail (Issuer view)** — full metadata, status history, revocation controls
29. **Revocation Workflow** — approval chain for sensitive status changes

### 8.5 Delegated Access

**Req ref:** CI-16

Parent or legal guardian management of minor's credentials. Linkage validated against authoritative registry data.

**Screen:**
30. **Delegated Access Config** — define guardian-minor relationships; validation rules; audit trail

### 8.6 Localization

**Req ref:** CI-06a, CV-03, NFR-07

Per-issuer language configuration. Full i18n including RTL support. Low-literacy considerations per NFR-11.

**Screen:**
31. **Language & Locale Settings** — per-issuer language selection; string override editor

### 8.7 Integration Dashboard

**Req ref:** CI-20

Status view of all connected issuer systems; health checks; go-live readiness.

**Screen:**
32. **Issuer Integration Status** — connected systems, API health, last sync, go-live checklist

---

## 9. Holder / Credential Store Workspace

### 9.1 Multi-Modal Credential Store

**Req refs:** CS-01, CS-02, CS-03

The holder controls where credentials live. The UI presents a unified view regardless of storage location.

| Storage Option | Details |
|---|---|
| **Local device** | Personal computer or mobile device storage |
| **Cloud store** | Cloud-hosted personal storage |
| **eLocker** | Managed cloud locker service |
| **Mobile wallet** | Native wallet app (Inji Mobile or compliant alternative) |
| **Physical print** | PDF with signed QR for paper-based use |

**Screens:**
33. **Credential Wallet** — card-based view of all held credentials; search, filter by issuer/type/expiry/status; group by storage location
34. **Credential Detail** — full claim set, metadata, issuer info, issuance/expiry dates, status, cryptographic proof summary, storage location
35. **Credential Retrieval** — import credentials via auth modalities (knowledge-based, OTP, biometric, face); download from portal; in-person collection flow

### 9.2 Delegated Storage

**Req ref:** CS-04

Guardian view and presentation of minor's credentials. Validated linkage to child via authoritative registry.

**Screen:**
36. **Dependents View** — guardian sees linked minors' credentials; present on their behalf

### 9.3 Sharing & Presentation

| Feature | Details |
|---|---|
| **Selective disclosure** | Where format supports it (BBS+, SD-JWT): toggle individual claims on/off |
| **Multi-credential presentation** | Combine credentials into a Verifiable Presentation |
| **Sharing channels** | Deep link, Bluetooth, NFC, file upload, QR display, print PDF |
| **Preview** | See exactly what the verifier will receive before sending |

**Screens:**
37. **Presentation Request Inbox** — incoming verification requests; shows which claims are requested vs. what credential contains
38. **Selective Disclosure Composer** — per-claim toggle; ZKP indicator where supported
39. **Presentation Builder** — combine multiple credentials; preview verifier's view; sign and send
40. **Share Screen** — channel selector (QR, deep link, Bluetooth, file, print); generate and transmit

### 9.4 Lifecycle & Export

**Screens:**
41. **Credential Timeline** — per-credential visual: issued → active → suspended → revoked → expired; event history with timestamps
42. **Multi-Format Export** — download as signed PDF with QR, JSON, XML; generate printable format

---

## 10. Verifier / Accepting Entity Workspace

### 10.1 Verification Channels

**Req ref:** CV-01

| Channel | Details |
|---|---|
| **SDK / Libraries** | Embeddable verification components for third-party apps (mandatory) |
| **APIs** | RESTful verification endpoints |
| **Web portal** | Standalone verification web app |
| **Mobile app** | Verification via camera (QR scan) or file upload |

### 10.2 Verification Request Builder

**Req refs:** CV-04, CV-05, CV-06, CV-07

| Feature | Details |
|---|---|
| **Request definition** | Specify credential types, required claims, constraints (issuer trust list, expiry, status) |
| **QR / deep link generation** | Render request as scannable QR or shareable link |
| **In-person mode** | QR scan, NFC tap, Bluetooth proximity |
| **Remote mode** | Deep link, file upload, online portal submission |
| **Self-service** | Holder-driven presentation to a verifier endpoint |
| **Assisted mode** | Agent operates on behalf of holder with consent capture |
| **ZKP acceptance** | When credential supports ZKP, verify attributes without seeing underlying data |

**Screens:**
43. **Verification Request Builder** — select credential types, claims, constraints; generate presentation request
44. **QR / Deep Link Generator** — render request as QR code or copyable link; configurable expiry
45. **Verification Portal** — accepting entity's branded page where holders submit credentials

### 10.3 Proof Validation

**Req ref:** CV-02

Every verification produces a structured result covering six checks:

| Check | What it validates |
|---|---|
| **Cryptographic signature** | Issuer's public key from trust registry |
| **Proof chain** | JSON-LD proof chains for W3C VCs |
| **Certificate path** | X.509 certificate path validation |
| **Revocation status** | CRL / OCSP / StatusList2021 |
| **Expiry** | Credential expiry timestamps |
| **Schema compliance** | Credential structure matches expected schema |

**Screens:**
46. **Verification Dashboard** — incoming presentations; per-presentation pass/fail summary
47. **Proof Detail** — six-check breakdown with clear pass/fail per check; raw proof data expandable
48. **Verification History** — audit log of all verification events; exportable

### 10.4 Online / Offline Verification

**Req ref:** CV-03

| Mode | Details |
|---|---|
| **Online** | Real-time verification against live trust registry and status lists |
| **Offline** | Cached trust data (issuer keys, revocation lists, schemas); staleness indicator; sync when connectivity restored |

**Screen:**
49. **Offline Config** — select which trust data to cache; set sync intervals; view staleness

### 10.5 Verifier Onboarding & Integration

**Req ref:** CV-07, PR-908

**Screens:**
50. **SDK Integration Guide** — interactive documentation; code samples; test endpoints
51. **Verifier Integration Dashboard** — SDK embed status, API health, onboarded accepting entities

---

## 11. Trust Infrastructure Workspace

**Req refs:** CI-04, PR-905, Appendix 1 (Supporting Trust Infrastructure)

### 11.1 Schema Registry

| Feature | Details |
|---|---|
| **Publish** | Issuers publish credential schemas to well-known URLs |
| **Discover** | Anyone can browse available schemas |
| **Version** | Schema versioning with backward compatibility tracking |
| **Formats** | JSON Schema, JSON-LD contexts, XML Schema |

**Screens:**
52. **Schema Registry Browser** — search, filter, view published schemas; version history
53. **Schema Publisher** — publish new schema; set visibility; configure well-known URL

### 11.2 Issuer Directory / Trust Registry

| Feature | Details |
|---|---|
| **Register** | Issuer DIDs and verification keys |
| **Browse** | Verifiers discover trusted issuers |
| **Governance** | Controls for who can publish; approval workflows |

**Screens:**
54. **Issuer Directory** — searchable registry of trusted issuers; DID, keys, jurisdiction, credential types
55. **Trust Registry Admin** — approve/reject issuer registrations; manage governance rules

### 11.3 Verifier Registry

**Screen:**
56. **Verifier Directory** — onboarded accepting entities; accepted credential types; verification policies

### 11.4 Governance Configuration

Governance at community, sector, regional, and national levels for inter- and intra-operable networks.

**Screen:**
57. **Governance Framework Editor** — define trust rules, acceptance policies, cross-jurisdiction agreements

---

## 12. Interoperability & Adaptor Layer

### 12.1 Inter-DPG / Inter-Service Adaptor Manager

The system must bridge across DPG stacks (Inji, Credebl, Walt.id, Quark.id, custom) and national service registries without requiring holders or verifiers to know which stack issued a credential.

| Feature | Details |
|---|---|
| **Adaptor registry** | Catalog of all connected bridges; protocol, status, throughput, error rate |
| **Configuration** | Per-adaptor: endpoint URLs, auth, protocol version (OID4VCI, OID4VP, DIDComm v2, CHAPI), data mapping rules, retry/fallback |
| **Protocol translation** | Live monitoring of cross-system credential exchanges; format translation (W3C-VC ↔ mDL, JSON-LD ↔ SD-JWT); failure flagging |
| **Trust bridging** | When ecosystems have different trust registries: define trust mappings (unilateral, bilateral, multilateral) |
| **Schema harmonization** | Field-level mappings across differing schemas (e.g., "full_name" ↔ "given_name" + "family_name"); validation for lossy transformations |

**Screens:**
58. **Adaptor Registry** — all connected adaptors; status, last sync, throughput, errors
59. **Adaptor Configuration** — per-adaptor settings: endpoints, auth, protocol, data mapping, fallback
60. **Protocol Translation Monitor** — live cross-system exchange view; translation events; failure alerts
61. **Trust Bridge Config** — cross-ecosystem trust mappings; issuer recognition rules
62. **Schema Harmonization Tool** — field-level mapping editor; validation rules; lossy transformation warnings

### 12.2 Last-Mile Connectivity Adaptors

Addresses the reality that many holders and verifiers operate in low-bandwidth, intermittent-connectivity, or non-smartphone environments.

| Feature | Details |
|---|---|
| **Channel adaptors** | SMS-based credential delivery, USSD flows, IVR (voice) verification, Bluetooth/NFC device-to-device, print-and-scan (QR on paper) |
| **Offline sync** | Configure cached trust data for offline verification; sync intervals; staleness monitoring |
| **Agent-assisted mode** | Kiosk/field-agent workflows; consent capture; agent identity verification; audit trail |
| **Connectivity health** | Per-channel availability, latency, failure rates; auto-fallback rules |
| **Multi-modal rendering** | Same credential rendered across: full wallet, simplified SMS, printed PDF, audio readout (IVR) |

**Screens:**
63. **Channel Adaptor Config** — enable/disable last-mile channels; per-channel settings
64. **Offline Sync Manager** — select trust data for caching; sync intervals; staleness dashboard
65. **Agent-Assisted Mode Config** — field agent workflows; consent capture templates; agent RBAC
66. **Connectivity Health Dashboard** — per-channel metrics; degradation alerts; fallback rule editor
67. **Multi-Modal Rendering Preview** — preview credential rendering across all configured channels

---

## 13. Self-Service Onboarding Toolkit

**Req ref:** Programmatic Objective 3 (DaaS Scope v2.0)

Enables the DPI owner to independently onboard new issuers and verifiers after the initial rollout.

| Feature | Details |
|---|---|
| **Issuer intake** | Guided registration for new issuers |
| **Schema builder** | Simplified schema creation for non-technical issuers |
| **Preview generator** | Instant PDF/QR preview from schema + sample data |
| **Sandbox** | Test issuance/verification without production impact |
| **Review & approval** | DPI owner reviews and approves onboarding |

**Screens:**
68. **Issuer Intake Form** — simplified guided registration
69. **Guided Schema Builder** — non-technical schema creation wizard
70. **Credential Preview Generator** — instant preview from schema + sample data
71. **Sandbox Environment** — isolated test environment; sample issuance/verification
72. **Onboarding Review & Approval** — DPI owner approval queue for new issuers/verifiers

---

## 14. Platform Admin Workspace

### 14.1 Deployment & Infrastructure

**Req refs:** NFR-04, NFR-12

| Feature | Details |
|---|---|
| **Deployment config** | Cloud provider settings; data sovereignty controls; environment management |
| **IaC awareness** | UI reflects Terraform-deployed infrastructure; environment health |
| **Cloud-agnostic** | AWS, GCP, Azure, OCI, OpenStack, on-premises |

**Screen:**
73. **Deployment Configuration** — environment settings; cloud provider; data residency; IaC status

### 14.2 Data Portability

**Req ref:** Section 9.2 item 9

Export all data in standardized formats (JSON-LD, CSV) with migration scripts. Import from other instances. Zero data loss validation.

**Screen:**
74. **Data Portability Console** — export/import wizards; format selection; migration validation; zero-loss verification

### 14.3 Reporting

**Req ref:** Section 10 (Project Governance)

Configurable dashboards for different stakeholder tiers.

| Tier | Report Type |
|---|---|
| **Funders / Convenors** | High-level summary: credentials issued, population reached, system uptime |
| **DPI Owner** | Operational: adoption rates, issuer/verifier activity, support tickets |
| **DPG Owner** | Granular: software quality, deployment health, code contribution metrics |

**Screen:**
75. **Reporting Dashboard** — tier-selectable views; configurable widgets; export PDF/CSV

### 14.4 Training & Documentation Hub

**Req refs:** NFR-07, NFR-08

Host SOPs, documentation, training content in local languages.

**Screen:**
76. **Training Hub** — searchable documentation; SOPs; training materials; language selector

### 14.5 Platform Metrics

**Screen:**
77. **Platform Health Dashboard** — system uptime (target 99.9% per NFR-06), API latency, error rates, active users, storage utilization

---

## 15. Screen Inventory Summary

| Workspace | Screens | IDs |
|---|---|---|
| **Landing Page & Exploration** | 10 | 78–87 |
| Shared Platform Shell | 8 | 1–8 |
| Issuer — Onboarding | 3 | 9–11 |
| Issuer — Schema & Templates | 5 | 12–16 |
| Issuer — Issuer-Initiated Flow | 5 | 17–21 |
| Issuer — Wallet-Initiated Flow | 5 | 22–26 |
| Issuer — Status Management | 3 | 27–29 |
| Issuer — Delegated, Locale, Integration | 3 | 30–32 |
| Holder / Credential Store | 10 | 33–42 |
| Verifier / Accepting Entity | 9 | 43–51 |
| Trust Infrastructure | 6 | 52–57 |
| Interoperability — Inter-DPG | 5 | 58–62 |
| Interoperability — Last-Mile | 5 | 63–67 |
| Self-Service Toolkit | 5 | 68–72 |
| Platform Admin | 5 | 73–77 |
| **Total** | **87** | |

---

## 16. Requirements Traceability Matrix

Every functional requirement from the DaaS Scope v2.0 mapped to specific screens.

### 16.1 Credential Issuance (CI)

| Req ID | Requirement Summary | Screen(s) |
|---|---|---|
| CI-01 | Self-service issuer onboarding with configurable approval workflows | 9, 10 |
| CI-02 | Issuer onboarding collects legal entity details, DID/key config | 9 |
| CI-03 | RBAC: Super Admin, Issuer Admin, Credential Officer, Auditor | 3, 4 |
| CI-04 | DID auto-generation/upload, X.509, trust registry registration | 11 |
| CI-05 | Issuance offered as SDK, SaaS, or Hosted | Adapter layer (non-UI) |
| CI-06 | Language, credential templates, QR content, schema formats, multi-type | 13, 14, 15, 16, 31 |
| CI-07 | Signing key generation and secure storage; verification key availability | 11 |
| CI-08 | Issuance in signed QR, signed PDF, machine-readable JSON/XML | 17, 19, 21 |
| CI-09 | Batch and on-demand issuance via APIs | 17, 18, 21 |
| CI-10 | CSV/Excel file upload for batch credential generation | 18 |
| CI-11 | Print format with signed QR | 16, 40, 42 |
| CI-12 | Issuance to user-controlled store (eLocker or wallet) | 17, 20 |
| CI-13 | Credential revocation | 27, 28, 29 |
| CI-14 | Workflow-based credential management for issuance/revocation | 29 |
| CI-15 | UI/UX customization: templates, logo, language, colors | 14, 15, 31 |
| CI-16 | Delegated access for guardians of minors | 2, 30, 36 |
| CI-17 | L1/L2/L3 support (operational, not UI) | 76 (training materials) |
| CI-18 | L3 support in local language for 12 months | 76 |
| CI-19 | 10,000 credentials/hour, <2s latency | 20 (monitoring) |
| CI-20 | Go-live integration with all issuers | 32 |

### 16.2 Credential Verification (CV)

| Req ID | Requirement Summary | Screen(s) |
|---|---|---|
| CV-01 | Verification via SDK, API, web portal, mobile app | 43, 45, 50, 51 |
| CV-02 | Six-point validation (signature, proof chain, cert path, revocation, expiry, schema) | 46, 47 |
| CV-03 | Online/offline; bilingual support | 49, 31 |
| CV-04 | In-person and remote acceptance | 43, 44, 45 |
| CV-05 | Self-service and assisted use cases | 43, 65 |
| CV-06 | ZKP-based selective disclosure acceptance | 38, 47 |
| CV-07 | SDK integration in accepting entity apps | 50, 51 |
| CV-08 | Multi-channel notifications | 5, 6 |
| CV-09 | L1/L2/L3 support in local language | 76 |
| CV-10 | L3 support for 12 months post-rollout | 76 |

### 16.3 Credential Store (CS)

| Req ID | Requirement Summary | Screen(s) |
|---|---|---|
| CS-01 | Multi-modal store: local, cloud, eLocker, wallet | 33 |
| CS-02 | Import via multiple auth modalities | 35 |
| CS-03 | Hold multiple credential types across issuers and standards | 33, 34 |
| CS-04 | Delegated storage for minors | 36 |

### 16.4 Programmatic Requirements (PR)

| Req ID | Requirement Summary | Screen(s) |
|---|---|---|
| PR-101 | Meet rollout objectives | 75 (reporting) |
| PR-201 | Go live on two use cases | 32, 75 |
| PR-202 | Onboard additional use case + identify 3 issuer institutions | 68, 72 |
| PR-203 | Outreach strategy playbook | 76 (documentation) |
| PR-204 | Whitepaper publication | Out of UI scope |
| PR-301 | Technical deliverables | All screens |
| PR-401 | Delivery schedule | 75 (reporting) |
| PR-905 | Schema and trust registry hosting at well-known URLs | 52, 53, 54, 55 |
| PR-906 | Policy enablement | 57 |
| PR-907 | Issuer provides attested data | 18 |
| PR-908 | Accepting entity SDK integration support | 50, 51 |

### 16.5 Non-Functional Requirements (NFR)

| Req ID | Requirement Summary | Screen(s) / Notes |
|---|---|---|
| NFR-01 | Integration, failover testing | QA process (not UI) |
| NFR-02 | Data integrity and credential accuracy | 19, 47 (validation views) |
| NFR-03 | Security best practices | Platform-wide |
| NFR-04 | 12-month cloud hosting and support | 73 |
| NFR-05 | Availability and security monitoring | 77 |
| NFR-06 | 99.9% uptime SLA | 77 |
| NFR-07 | Training materials in local language | 76 |
| NFR-08 | Implementation documentation | 76 |
| NFR-09 | Source code and documentation handover | 74 |
| NFR-10 | Vulnerability assessment, pen test reports | 77 |
| NFR-11 | Iterative UX testing with diverse user groups | Design process (not a screen) |
| NFR-12 | Infrastructure as Code, cloud-agnostic | 73 |

---

## 17. Issuance Flow Topology — Detailed Comparison

| Dimension | Issuer-Initiated | Wallet-Initiated |
|---|---|---|
| **Trigger** | Issuer decides to issue | Holder requests a credential |
| **Data source** | Issuer uploads attested data (CSV, registry, API) | Holder authenticates + submits evidence |
| **Timing** | Immediate or batch | Async — may take days/weeks |
| **Holder action** | Receive → review → accept/reject | Request → wait → receive → accept |
| **Issuer action** | Configure → upload → sign → dispatch | Receive request → validate → approve → issue |
| **Use cases** | Mass issuance (national ID rollout, graduation certificates) | On-demand (replacement credential, new enrollment) |
| **Convergence** | Both flows converge at the holder accept/reject screen (Screen 34) and the credential store (Screen 33) |

The UI must make it clear to the holder which flow originated the credential — pushed vs. requested — for transparency and consent.

---

## 18. Interoperability Architecture — Detailed

### 18.1 Supported Protocols

| Protocol | Role | UI Touchpoint |
|---|---|---|
| **OID4VCI** | Credential issuance | Issuer adaptor config (Screen 59) |
| **OID4VP** | Credential presentation/verification | Verifier adaptor config (Screen 59) |
| **DIDComm v2** | Secure messaging between agents | Adaptor config (Screen 59) |
| **CHAPI** | Credential handler API (browser-based) | Adaptor config (Screen 59) |

### 18.2 Supported Proof Formats

All five credential standards are supported from day one.

| Format | Standard | Selective Disclosure | ZKP | UI Behavior |
|---|---|---|---|---|
| **Data Integrity (JSON-LD + EdDSA/ECDSA)** | W3C-VCDM 2.0 | No (full credential only) | No | Disclosure composer hidden |
| **Data Integrity (JSON-LD + BBS+)** | W3C-VCDM 2.0 | Yes (claim-level) | Yes | Disclosure composer + ZKP indicator shown |
| **SD-JWT** | SD-JWT | Yes (claim-level) | No | Disclosure composer shown |
| **JOSE/COSE (JWT)** | W3C-VCDM 2.0 / 1.1 | No | No | Disclosure composer hidden |
| **mDL (CBOR/COSE)** | ISO 18013-5 | Yes (namespace-level) | No | Namespace-level disclosure selector |
| **AnonCreds** | AnonCreds | Yes (attribute-level) | Yes | Full disclosure composer + ZKP indicator |
| **JSON-LD Proof (legacy)** | W3C-VCDM 1.1 | No | No | Full credential only; legacy proof display |

The UI adapts dynamically: when the credential's proof format supports selective disclosure, the disclosure composer (Screen 38) is enabled. When it doesn't, the holder can only share the full credential.

### 18.3 Supported Schema Formats

| Format | UI Touchpoint |
|---|---|
| **JSON Schema** | Schema builder (Screen 13), registry (Screen 52) |
| **JSON-LD Contexts** | Schema builder (Screen 13), registry (Screen 52) |
| **XML Schema** | Schema builder (Screen 13), import/export |

### 18.4 Supported Status Methods

| Method | UI Touchpoint |
|---|---|
| **Bitstring Status List (W3C)** | Status management (Screen 27), verification (Screen 47) |
| **StatusList2021** | Verification check (Screen 47) |
| **CRL / OCSP** | Verification check (Screen 47) |
| **Token Status List** | Verification check (Screen 47) |

---

## 19. Delivery Format Matrix

Every credential must be renderable in multiple formats for inclusive access.

| Format | Channel | Use Case | Screens |
|---|---|---|---|
| **Signed PDF with QR** | Download, email, print | Paper-based verification, low-tech environments | 16, 40, 42 |
| **Signed QR (SVG)** | Print, display on screen | Quick in-person verification | 16, 40, 44 |
| **Machine-readable JSON** | API, deep link, file transfer | System-to-system verification, auto-form-fill | 42 |
| **Machine-readable XML** | API, file transfer | Legacy system integration | 42 |
| **Wallet card** | Mobile wallet app | Daily use, tap/scan verification | 33, 34 |
| **SMS text** | SMS delivery | Last-mile, no smartphone | 63, 67 |
| **Audio readout** | IVR (voice call) | Accessibility, no literacy required | 63, 67 |

---

## 20. Component-to-DPG Stack Mapping

The UI is agnostic; these are reference mappings showing which DPG/ISV products can serve each component.

| Component | Inji | Credebl | Walt.id | Quark.id | Custom ISV |
|---|---|---|---|---|---|
| Credential Issuance | Inji Certify | ✓ | ✓ | ✓ | Via adapter |
| Credential Verification | Inji Verify | ✓ | ✓ | ✓ | Via adapter |
| Credential Store (Web) | Inji Web | ✓ | ✓ | ✓ | Via adapter |
| Credential Store (Mobile) | Inji Mobile | ✓ | ✓ | ✓ | Via adapter |
| SSO | eSignet | — | — | — | WSO2, Keycloak, Custom |

---

## 21. Deployment Topology

The UI must function identically across all deployment models:

| Model | Description | UI Impact |
|---|---|---|
| **SaaS** | ISV-hosted on hyperscaler | Standard web deployment |
| **Hosted** | Customer-controlled cloud (AWS, GCP, Azure, OCI) | Same UI, different endpoints |
| **On-premises** | Government data center | Same UI, local network |
| **Hybrid** | Split between cloud and on-prem | UI connects to whichever endpoints are configured |

All deployments use Infrastructure as Code (Terraform or equivalent) to remain cloud-agnostic (NFR-12). The UI's deployment configuration screen (Screen 73) reflects but does not manage IaC — it shows environment health and allows endpoint configuration.

---

## 22. Accessibility & Inclusivity Requirements

**Req ref:** NFR-11

| Requirement | Implementation |
|---|---|
| **Low-literacy users** | Icon-heavy navigation; visual credential cards; minimal text-only screens; audio guidance option |
| **RTL languages** | Full RTL layout support in white-label config |
| **Rural / low-bandwidth** | Progressive loading; offline-capable verification; minimal asset sizes |
| **Disability** | WCAG 2.1 AA compliance minimum; screen reader compatible; keyboard navigable; high contrast mode |
| **Diverse user groups** | Iterative UX testing with low-literacy, rural, guardian, and elderly populations throughout lifecycle |
| **Agent-assisted** | Dedicated agent mode for field workers operating on behalf of holders |

---

## 23. Security Considerations

| Concern | Approach |
|---|---|
| **Auth** | All sensitive operations require active session; re-auth for signing/revocation |
| **Data minimization** | Audit logs store minimal holder PII; ZKP supported where available |
| **Key management** | UI surfaces key lifecycle (generation, rotation, revocation); actual crypto is backend |
| **Transport** | TLS everywhere; no plaintext credential transmission |
| **Consent** | Explicit holder consent before any credential sharing; consent audit trail |
| **Agent-assisted** | Agent identity verified; all agent actions logged; holder consent captured separately |

---

## 24. Phased Build Strategy

### Phase 0 — Landing Page & Exploration Shell (Weeks 1–4)

This phase ships first because it's the top of the funnel — it acquires users, teaches the domain, and generates demand for the production infrastructure. It also forces early resolution of the sandbox backend, which de-risks all subsequent phases.

- Landing page with audience selector (Screen 78)
- VC explainer journey — progressive disclosure screens (Screen 79)
- Interactive mockup launcher with sandbox backend (Screen 80)
- Standards & protocol explorer overlays (Screen 81)
- Requirements matrix mapper overlay (Screen 82)
- Tiered package mapper (Screen 83)
- Chatbot interface — exploration and decision support modes (Screen 86)
- Agent output viewer for generated documents (Screen 87)
- White-label config engine + theme system (Josefin Sans / Geist, light-first)
- Sandbox backend deployment (same infrastructure as production, synthetic data)

### Phase 1 — Foundation & Portal Shell (Weeks 5–8)

- Auth shell with SSO adapter
- RBAC implementation (four roles)
- Role selector and production portal entry (Screen 84)
- Navigation shell with workspace routing
- Notification hub (Screens 5–6)
- Audit log and activity dashboard (Screens 7–8)
- Auditor dashboard (Screen 85)

### Phase 2 — Issuer Core (Weeks 9–12)

- Issuer onboarding with automated provisioning pipeline (Screens 9–11)
- Schema designer + template editor (Screens 12–16)
- Issuer-initiated issuance flow (Screens 17–21)
- Status management (Screens 27–29)

### Phase 3 — Holder Core (Weeks 13–16)

- Credential wallet + detail views (Screens 33–35)
- Credential retrieval + multi-modal store
- Sharing & presentation builder (Screens 37–42)
- Wallet-initiated request flow (Screens 22–26)

### Phase 4 — Verifier Core (Weeks 17–20)

- Verification request builder (Screens 43–45)
- Proof validation dashboard (Screens 46–48)
- Online/offline verification (Screen 49)
- SDK integration guide (Screens 50–51)

### Phase 5 — Trust & Interop (Weeks 21–24)

- Trust infrastructure (Screens 52–57)
- Inter-DPG adaptor manager (Screens 58–62)
- Last-mile connectivity (Screens 63–67)

### Phase 6 — Platform, Self-Service & Agent Integration (Weeks 25–28)

- Self-service onboarding toolkit (Screens 68–72)
- Platform admin (Screens 73–77)
- Chatbot agent routing to Operations & Technical agents (n8n/OpenFn pipelines)
- Document generation pipeline (PPTX, DOCX, PDF export from agent outputs)
- Notification hub finalization
- End-to-end integration testing
- Exploration-to-production transition flow testing

---

## 25. Scope Boundaries

### In Scope

- Landing page and exploration layer — VC explainer, interactive mockups, standards explorer, requirements/tier mapping
- AI chatbot with guided exploration, decision support, and agent routing
- Two document-generation agents (Operations & Program Management, Technical Architect) integrated via n8n/OpenFn
- Sandbox environment sharing production codebase with synthetic data
- Exploration-to-production continuity (same UI, different backend)
- Auditor portal with cross-workspace read-only access
- Every UI surface a human touches during issuance, holding, verification, and platform management
- Both issuer-initiated and wallet-initiated credential flows
- Cross-DPG adaptor management and monitoring
- Last-mile channel configuration (SMS, USSD, IVR, print, agent-assisted)
- Full white-label theming
- Responsive web (mobile-first, desktop-capable)
- Offline-capable verification UI
- Multi-language, RTL, low-literacy support
- Backend adapter interface contracts (typed, documented)
- All 87 screens defined in this document

### Out of Scope

- Cryptographic operations (signing, proof generation, DID resolution) — backend
- Native mobile wallet app shell — web-first; native wrapping is a separate effort
- SMS/USSD/IVR gateway operations — the UI configures, does not implement transport
- NFC firmware or Bluetooth stack — platform-level concerns
- Physical kiosk hardware
- Cloud infrastructure provisioning — Terraform scripts are backend
- Governance policy authoring — institutional/legal process; UI enforces configured policies
- DaaS commercial model / pricing engine
- Actual SSO provider implementation
- Outreach campaign design and execution (led by DPI owner)
- Whitepaper authoring (PR-204)

---

## 26. Resolved Design Decisions

The following questions were raised during planning and have been resolved:

### 26.1 Credential Standard Priority

**Decision:** W3C-VCDM 2.0 is the primary standard. Support for mDL (ISO 18013-5), SD-JWT, W3C-VCDM 1.1, and AnonCreds must be present from day one.

**UI impact:** The schema designer (Screen 13), template editor (Screen 15), and all issuance/verification flows must include a standard selector. Proof format determines which UI features are available (e.g., AnonCreds enables ZKP-based selective disclosure; SD-JWT enables claim-level disclosure; VCDM 1.1 credentials render with legacy proof formatting). The verification dashboard (Screen 47) must handle all five proof validation paths.

| Standard | Selective Disclosure | ZKP | UI Behavior |
|---|---|---|---|
| W3C-VCDM 2.0 (Data Integrity) | Via BBS+ only | Via BBS+ | Disclosure composer conditional on cryptosuite |
| W3C-VCDM 1.1 | No | No | Full credential only; legacy proof display |
| SD-JWT | Yes (claim-level) | No | Disclosure composer shown |
| mDL (ISO 18013-5) | Yes (namespace-level) | No | Namespace-level disclosure selector |
| AnonCreds | Yes (attribute-level) | Yes | Full disclosure composer + ZKP indicator |

### 26.2 Wallet-Initiated Flow — DPG Readiness

**Decision:** Walt.id is one among several DPGs that support holder-requested issuance. The deployment must remain ready for implementation of any compliant DPG.

**UI impact:** The credential catalog (Screen 22) and request builder (Screen 23) must be DPG-agnostic. The catalog is populated dynamically via the backend adapter — it does not assume any specific DPG's API shape. When a new DPG is connected, its available credential types appear in the catalog without UI code changes. The adaptor configuration (Screen 59) is the only place where DPG-specific settings are managed.

### 26.3 Last-Mile Channels

**Decision:** All last-mile channels are confirmed for the first deployment: SMS, USSD, IVR (voice), Bluetooth/NFC, print-and-scan (QR on paper), agent-assisted.

**UI impact:** All channel adaptor screens (Screens 63–67) are in scope for Phase 5. No channels are deferred. The connectivity health dashboard (Screen 66) must monitor all channels from launch. The multi-modal rendering preview (Screen 67) must support all six renderings.

### 26.4 Sandbox Isolation

**Decision:** The sandbox (Screen 71) is a separate environment by default. A logical partition within the production instance is available as a configurable option if the jurisdiction requires it (e.g., for data residency or infrastructure constraints).

**UI impact:** The sandbox environment selector must offer two modes: "Isolated" (separate infrastructure, separate data) and "Partitioned" (shared infrastructure, logically separated data). The default is "Isolated." Switching to "Partitioned" requires Super Admin approval and displays a clear warning about shared infrastructure implications. Sandbox credentials must be visually distinct (watermarked, labeled "TEST") in all rendering formats to prevent confusion with production credentials.

### 26.5 Agent-Assisted Consent

**Decision:** Legal frameworks for agent-mediated credential presentation vary by jurisdiction. The UX must support multiple consent capture methods by default, selectable per deployment.

**UI impact:** The agent-assisted mode config (Screen 65) must offer a consent method selector with at minimum the following options:

| Consent Method | Description | When Used |
|---|---|---|
| **Digital signature** | Holder signs consent on agent's device (touchscreen) | Holder is present and literate |
| **OTP confirmation** | Holder receives OTP on their phone, provides to agent | Holder has phone, may not be present at device |
| **Biometric capture** | Fingerprint or face capture as consent proof | Jurisdictions requiring biometric consent |
| **Verbal consent (recorded)** | Audio recording of holder's verbal consent | Low-literacy, no device access |
| **Witness attestation** | Third-party witness signs consent alongside agent | Jurisdictions requiring witness for legal validity |
| **Paper consent (scanned)** | Physical signature on paper form, scanned and attached | Fallback for no-tech environments |

Each deployment configures which methods are available and which are required. The audit trail (Screen 7) records the consent method used for every agent-assisted transaction. Consent records are immutable and exportable for legal review.

---

*End of document. Nothing gets built until this plan is approved.*
