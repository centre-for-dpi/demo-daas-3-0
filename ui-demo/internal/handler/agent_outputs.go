package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// AgentArtifact is a saved chatbot-generated document (pitch deck, advisory note,
// technical scope, etc.) that the user can revisit on the Outputs page.
type AgentArtifact struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Type      string    `json:"type"`    // "pitch_deck", "advisory_note", "technical_scope", "country_proposal", "other"
	Persona   string    `json:"persona"` // "architect", "programs", "guide"
	Content   string    `json:"content"` // markdown
	Source    string    `json:"source"`  // the user message that triggered the generation
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// agentArtifactStore is an in-memory map of artifacts mirrored to a JSON file
// on disk so artifacts survive server restarts. The persistence file path is
// configurable via AGENT_OUTPUTS_FILE; default is ./agent-outputs.json relative
// to the server's working directory.
type agentArtifactStore struct {
	mu       sync.RWMutex
	items    map[string]*AgentArtifact
	filePath string
}

func newAgentArtifactStore() *agentArtifactStore {
	path := os.Getenv("AGENT_OUTPUTS_FILE")
	if path == "" {
		path = "./agent-outputs.json"
	}
	s := &agentArtifactStore{
		items:    make(map[string]*AgentArtifact),
		filePath: path,
	}
	s.loadFromDisk()
	return s
}

// loadFromDisk reads the artifact JSON file at startup. Missing file is fine.
func (s *agentArtifactStore) loadFromDisk() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("agentArtifactStore: failed to read %s: %v", s.filePath, err)
		}
		return
	}
	var loaded map[string]*AgentArtifact
	if err := json.Unmarshal(data, &loaded); err != nil {
		log.Printf("agentArtifactStore: failed to parse %s: %v", s.filePath, err)
		return
	}
	s.items = loaded
	log.Printf("agentArtifactStore: loaded %d artifacts from %s", len(loaded), s.filePath)
}

// persistLocked writes the entire map to disk atomically. Caller holds s.mu.
func (s *agentArtifactStore) persistLocked() {
	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		log.Printf("agentArtifactStore: marshal failed: %v", err)
		return
	}
	tmp := s.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Printf("agentArtifactStore: write %s failed: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, s.filePath); err != nil {
		log.Printf("agentArtifactStore: rename %s -> %s failed: %v", tmp, s.filePath, err)
	}
}

func (s *agentArtifactStore) create(a *AgentArtifact) *AgentArtifact {
	s.mu.Lock()
	defer s.mu.Unlock()
	a.ID = randomID()
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now
	s.items[a.ID] = a
	s.persistLocked()
	return a
}

func (s *agentArtifactStore) list() []*AgentArtifact {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*AgentArtifact, 0, len(s.items))
	for _, a := range s.items {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *agentArtifactStore) get(id string) (*AgentArtifact, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.items[id]
	return a, ok
}

func (s *agentArtifactStore) update(id, content, title string) (*AgentArtifact, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.items[id]
	if !ok {
		return nil, false
	}
	if content != "" {
		a.Content = content
	}
	if title != "" {
		a.Title = title
	}
	a.UpdatedAt = time.Now().UTC()
	s.persistLocked()
	return a, true
}

func (s *agentArtifactStore) delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return false
	}
	delete(s.items, id)
	s.persistLocked()
	return true
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// detectType picks an artifact type from the markdown content, with a
// persona-based fallback. Heuristic but predictable.
func detectArtifactType(persona, content string) string {
	trimmed := strings.TrimSpace(content)
	lc := strings.ToLower(trimmed)
	// Blog post: explicit "# Blog:" prefix on the first heading line.
	if strings.HasPrefix(trimmed, "# Blog:") || strings.HasPrefix(lc, "# blog:") {
		return "blog_post"
	}
	// Marp pitch deck: explicit frontmatter at the very top.
	if strings.HasPrefix(trimmed, "---") && strings.Contains(strings.SplitN(trimmed, "\n---", 2)[0], "marp: true") {
		return "pitch_deck"
	}
	switch {
	case strings.Contains(lc, "slide 1") || strings.Contains(lc, "pitch deck"):
		return "pitch_deck"
	case strings.Contains(lc, "country adoption proposal") || strings.Contains(lc, "adoption proposal"):
		return "country_proposal"
	case strings.Contains(lc, "## ask") && strings.Contains(lc, "## why") && strings.Contains(lc, "## scope"):
		return "advisory_note"
	case strings.Contains(lc, "advisory note"):
		return "advisory_note"
	case strings.Contains(lc, "technical scope") || strings.Contains(lc, "scope document"):
		return "technical_scope"
	}
	switch persona {
	case "architect":
		return "advisory_note"
	case "programs":
		return "pitch_deck"
	}
	return "other"
}

// detectArtifactTitle pulls the first markdown H1 or H2, or falls back to a
// truncated source message.
func detectArtifactTitle(content, source string) string {
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "#"))
		}
	}
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "## ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "##"))
		}
	}
	src := strings.TrimSpace(source)
	if len(src) > 80 {
		return src[:80] + "…"
	}
	if src == "" {
		return "Untitled output"
	}
	return src
}

// ---- HTTP handlers ----

// AgentOutputsCreate — POST /api/agent/outputs
// Body: { persona, content, source, title? (optional override), type? (optional override) }
func (h *Handler) AgentOutputsCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Persona string `json:"persona"`
		Content string `json:"content"`
		Source  string `json:"source"`
		Title   string `json:"title"`
		Type    string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(in.Content) == "" {
		writeJSONError(w, http.StatusBadRequest, "content is required")
		return
	}
	a := &AgentArtifact{
		Persona: in.Persona,
		Content: in.Content,
		Source:  in.Source,
		Title:   in.Title,
		Type:    in.Type,
	}
	if a.Title == "" {
		a.Title = detectArtifactTitle(in.Content, in.Source)
	}
	if a.Type == "" {
		a.Type = detectArtifactType(in.Persona, in.Content)
	}
	saved := h.agentArtifacts.create(a)
	writeJSON(w, http.StatusCreated, saved)
}

// AgentOutputsList — GET /api/agent/outputs
func (h *Handler) AgentOutputsList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	items := h.agentArtifacts.list()
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "count": len(items)})
}

// AgentOutputsGet — GET /api/agent/outputs/{id}
func (h *Handler) AgentOutputsGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id := r.PathValue("id")
	a, ok := h.agentArtifacts.get(id)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "artifact not found")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// AgentOutputsUpdate — PUT /api/agent/outputs/{id}
// Body: { content?, title? }
func (h *Handler) AgentOutputsUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		Content string `json:"content"`
		Title   string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	a, ok := h.agentArtifacts.update(id, in.Content, in.Title)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "artifact not found")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// AgentOutputsDelete — DELETE /api/agent/outputs/{id}
func (h *Handler) AgentOutputsDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !h.agentArtifacts.delete(id) {
		writeJSONError(w, http.StatusNotFound, "artifact not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AgentOutputsExport — GET /api/agent/outputs/{id}/export?format=md|pptx
// md: raw markdown download.
// pptx: pipe markdown through Marp with the CDPI theme to produce a CDPI-branded PowerPoint.
func (h *Handler) AgentOutputsExport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, ok := h.agentArtifacts.get(id)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "artifact not found")
		return
	}
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "" {
		format = "md"
	}
	switch format {
	case "md", "markdown":
		filename := safeFilename(a.Title) + ".md"
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		_, _ = w.Write([]byte(a.Content))
	case "pptx":
		exportPPTX(w, a)
	default:
		writeJSONError(w, http.StatusNotImplemented, "format "+format+" is not supported yet (md, pptx)")
	}
}

// exportPPTX runs marp on the artifact's markdown content with the CDPI theme
// and streams the resulting .pptx back to the client.
func exportPPTX(w http.ResponseWriter, a *AgentArtifact) {
	marpBin := os.Getenv("MARP_BIN")
	if marpBin == "" {
		if p, err := exec.LookPath("marp"); err == nil {
			marpBin = p
		} else {
			marpBin = "/home/adam/.nvm/versions/node/v24.11.1/bin/marp"
		}
	}
	themePath := os.Getenv("MARP_THEME_PATH")
	if themePath == "" {
		themePath = "/home/adam/cdpi/n8n-demo/agent-service/marp/cdpi.css"
	}

	if _, err := os.Stat(marpBin); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "marp binary not found at "+marpBin)
		return
	}
	if _, err := os.Stat(themePath); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "marp theme not found at "+themePath)
		return
	}

	tmpDir, err := os.MkdirTemp("", "marp-export-")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create temp dir: "+err.Error())
		return
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input.md")
	outputPath := filepath.Join(tmpDir, "output.pptx")

	// Strip a wrapping triple-backtick code fence if the LLM produced one.
	// LLMs sometimes wrap their entire markdown output in ```...``` for
	// "presentation" — that breaks marp's frontmatter parser.
	content := strings.TrimSpace(a.Content)
	content = stripWrappingCodeFence(content)

	// Marp expects YAML frontmatter at the top of the file. If the artifact
	// doesn't start with one, prepend a minimal frontmatter so marp doesn't error.
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		content = "---\nmarp: true\ntheme: cdpi\npaginate: true\n---\n\n" + content
	}
	if err := os.WriteFile(inputPath, []byte(content), 0o644); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to write temp markdown: "+err.Error())
		return
	}

	cmd := exec.Command(marpBin, inputPath, "--pptx", "--theme-set", themePath, "--output", outputPath, "--allow-local-files")
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("marp failed for artifact %s: %v\n%s", a.ID, err, out)
		writeJSONError(w, http.StatusInternalServerError, "PPTX generation failed: "+err.Error())
		return
	}

	pptx, err := os.ReadFile(outputPath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read generated PPTX: "+err.Error())
		return
	}

	filename := safeFilename(a.Title) + ".pptx"
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.presentationml.presentation")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = w.Write(pptx)
}

// stripWrappingCodeFence removes a leading ```... and trailing ``` if the
// entire content is wrapped in a single code block. Idempotent; leaves inline
// code fences inside the document untouched.
func stripWrappingCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Remove the opening fence (which may have a language tag like ```markdown).
	nl := strings.Index(s, "\n")
	if nl == -1 {
		return s
	}
	body := s[nl+1:]
	// Trim a trailing closing fence.
	body = strings.TrimRight(body, " \t\n")
	if strings.HasSuffix(body, "```") {
		body = strings.TrimSuffix(body, "```")
	}
	return strings.TrimSpace(body)
}

func safeFilename(title string) string {
	if title == "" {
		return "agent-output"
	}
	out := strings.Builder{}
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			out.WriteRune('-')
		}
	}
	s := strings.Trim(out.String(), "-")
	if s == "" {
		return "agent-output"
	}
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

// writeJSONError is a tiny helper for the agent outputs handlers.
// (Not named writeAgentError because that name is taken by agent.go.)
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
