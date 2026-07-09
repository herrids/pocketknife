package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultAnthropicModel is used when the host process does not name a model.
const DefaultAnthropicModel = "claude-opus-4-8"

// anthropicMaxTokens caps one response. Prompt functions are single-shot
// text transforms, so this is generous without risking HTTP timeouts on a
// non-streaming request.
const anthropicMaxTokens = 16000

// anthropicCaller is the Caller backed by the Anthropic Messages API. Like
// httpCaller, the key is unexported with no JSON tag, no String method and no
// accessor: nothing in this package ever hands it back out. The caller is
// deliberately hand-rolled net/http rather than an SDK so the entire
// token-custody surface stays auditable in this one small file.
type anthropicCaller struct {
	baseURL string // unexported seam so same-package tests can use httptest
	apiKey  string
	model   string
	client  *http.Client
}

// NewAnthropicCaller builds a Caller that sends a single-turn user message to
// the Anthropic Messages API and returns the response text. apiKey is
// typically sourced from the host process's own environment (configuration a
// function never sees) — never from a manifest or function input. An empty
// model selects DefaultAnthropicModel.
func NewAnthropicCaller(apiKey, model string) Caller {
	if model == "" {
		model = DefaultAnthropicModel
	}
	return &anthropicCaller{
		baseURL: "https://api.anthropic.com",
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Thinking  anthropicThinking  `json:"thinking"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicThinking struct {
	Type string `json:"type"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *anthropicCaller) Call(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(anthropicRequest{
		Model:     c.model,
		MaxTokens: anthropicMaxTokens,
		Thinking:  anthropicThinking{Type: "adaptive"},
		Messages:  []anthropicMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", fmt.Errorf("broker: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("broker: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("broker: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("broker: read response: %w", err)
	}

	var out anthropicResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("broker: provider returned status %d", resp.StatusCode)
		}
		return "", fmt.Errorf("broker: decode response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// The provider's error message is safe to relay; it never contains
		// the key. The status alone is kept as the primary signal.
		if out.Error != nil {
			return "", fmt.Errorf("broker: provider returned status %d: %s", resp.StatusCode, out.Error.Message)
		}
		return "", fmt.Errorf("broker: provider returned status %d", resp.StatusCode)
	}

	if out.StopReason == "refusal" {
		return "", fmt.Errorf("broker: provider declined the request")
	}

	var parts []string
	for _, block := range out.Content {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("broker: provider returned no text content")
	}
	return strings.Join(parts, "\n"), nil
}
