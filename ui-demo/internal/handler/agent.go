package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

var agentWebhookURL = func() string {
	if u := os.Getenv("AGENT_WEBHOOK_URL"); u != "" {
		return u
	}
	return "http://localhost:5678/webhook/agent-chat"
}()

var agentHTTPClient = &http.Client{Timeout: 120 * time.Second}

// AgentChat proxies a chat message from the browser to the n8n agent webhook.
//
// Request body:
//
//	{ "message": "...", "conversationId": "...", "context": { "currentPath": "/tiers", "currentTitle": "..." } }
//
// Response body (from n8n Respond to Webhook):
//
//	{ "persona": "guide", "answer": "...", "conversationId": "..." }
func (h *Handler) AgentChat(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeAgentError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if msg, _ := payload["message"].(string); msg == "" {
		writeAgentError(w, http.StatusBadRequest, "message is required")
		return
	}

	body, err := json.Marshal(payload)
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "failed to encode upstream request")
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, agentWebhookURL, bytes.NewReader(body))
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "failed to build upstream request")
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := agentHTTPClient.Do(req)
	if err != nil {
		log.Printf("agent chat upstream error: %v", err)
		writeAgentError(w, http.StatusBadGateway, "agent backend unavailable")
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "failed to read upstream response")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

func writeAgentError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":   msg,
		"persona": "",
		"answer":  "",
	})
}
