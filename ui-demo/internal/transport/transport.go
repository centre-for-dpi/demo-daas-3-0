package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client abstracts HTTP communication. Implementations can call APIs directly
// or route through intermediaries (n8n webhooks, OpenFn).
type Client interface {
	Do(ctx context.Context, method, path string, body any) ([]byte, int, error)
}

// HTTPClient calls REST APIs directly using net/http.
type HTTPClient struct {
	BaseURL    string
	AuthToken  string
	HTTPClient *http.Client
}

func NewHTTPClient(baseURL, authToken string) *HTTPClient {
	return &HTTPClient{
		BaseURL:   baseURL,
		AuthToken: authToken,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *HTTPClient) Do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	url := c.BaseURL + path

	var bodyReader io.Reader
	if body != nil {
		switch v := body.(type) {
		case string:
			bodyReader = bytes.NewBufferString(v)
		case []byte:
			bodyReader = bytes.NewBuffer(v)
		default:
			data, err := json.Marshal(body)
			if err != nil {
				return nil, 0, fmt.Errorf("marshal body: %w", err)
			}
			bodyReader = bytes.NewBuffer(data)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	if body != nil {
		switch body.(type) {
		case string:
			req.Header.Set("Content-Type", "text/plain")
		default:
			req.Header.Set("Content-Type", "application/json")
		}
	}

	if c.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AuthToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}

	return respBody, resp.StatusCode, nil
}

// WebhookClient routes API calls through a webhook (n8n, OpenFn).
// The webhook receives the method/path/body and returns the DPG response.
type WebhookClient struct {
	WebhookURL string
	Secret     string
	HTTPClient *http.Client
}

func NewWebhookClient(webhookURL, secret string) *WebhookClient {
	return &WebhookClient{
		WebhookURL: webhookURL,
		Secret:     secret,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

type webhookPayload struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   any    `json:"body,omitempty"`
}

func (c *WebhookClient) Do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	payload := webhookPayload{Method: method, Path: path, Body: body}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.WebhookURL, bytes.NewBuffer(data))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Secret != "" {
		req.Header.Set("X-Webhook-Secret", c.Secret)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, nil
}
