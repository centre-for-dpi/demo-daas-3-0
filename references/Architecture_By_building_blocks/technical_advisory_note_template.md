# Technical Advisory Note — Prompt Template

You are writing a technical advisory note for digital public infrastructure. Follow this structure exactly.

## Format Rules
- The document is one page A4 maximum.
- The document is self-contained. No reference docs, no external links.
- If the content won't fit in one page, the ask is too broad.
- Tables are encouraged wherever they make the document shorter.
- Every line in Ask, Why, Scope, and Context must be 9–13 words. No exceptions.

## Structure

### 1. Ask
- Exactly one line, 9–13 words.
- States what needs to be resolved.
- Must be framed around interoperability and implementation of open standards.
- Do not prescribe a specific technology in the ask.

### 2. Why
- Exactly one line, 9–13 words.
- States what the reader gains by reading this document.
- Must be a concrete, tangible benefit (e.g., cost savings, time savings, reduced risk).
- Do not make it abstract or procedural (e.g., avoid "architecture must be selected before X").

### 3. Scope
- Exactly one line, 9–13 words.
- States what the document covers in plain, approachable language.
- Do not use jargon like "in-scope" or "out-of-scope."
- Anything not described in this line is out of scope.

### 4. Context
- One to three lines, each 9–13 words.
- Three lines is the maximum, not the target. Use fewer if fewer are needed.
- Each line states a fact the reader needs in order to evaluate the options.
- Facts should be grounded in interoperability, open standards, speed, and security.
- Do not repeat information that appears in the options table.

### 5. Options
- A comparison table mapping approaches against open standards and DPI principles.
- Column headers for each option must include the specific version evaluated (e.g., "CREDEBL SDK v1.0.0", not just "CREDEBL SDK"). This makes the doc auditable and updatable when capabilities change between versions.
- Include only features that differentiate the options.
- Do not include features where all options are identical unless that parity is itself noteworthy (e.g., both lacking offline support).

### 6. Advice
- A table with four columns: Requirement Category | Ubiquitous Standard | Guidance | Considerations.
- Maximum five rows.
- The advice is never "pick X over Y."
- The matrix maps requirements to ubiquitous standards and implementation guidance. The reader uses it to identify which standards apply to their context and what trade-offs to weigh.
- The ubiquitous standard column indicates which specifications constrain each decision.
- The considerations column flags trade-offs between standards or approaches. Leave blank if no conflict exists.
- Use a short closing paragraph only if the table alone cannot capture the advice.

## Tone
- Write for implementers and decision-makers alike.
- Be direct and concise. No filler, no preamble.
- Do not fabricate facts. Use only verified, accurate information.
