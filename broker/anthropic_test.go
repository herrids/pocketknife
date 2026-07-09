package broker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestAnthropicCaller points an anthropicCaller at a local test server via
// the unexported baseURL seam.
func newTestAnthropicCaller(srvURL, apiKey, model string) *anthropicCaller {
	c := NewAnthropicCaller(apiKey, model).(*anthropicCaller)
	c.baseURL = srvURL
	return c
}

func TestAnthropicCallerSendsMessagesRequest(t *testing.T) {
	const secret = "sk-ant-test-key"
	var gotPath, gotKey, gotVersion, gotContentType string
	var gotBody anthropicRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		gotContentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"content": [
				{"type": "thinking", "thinking": ""},
				{"type": "text", "text": "first"},
				{"type": "text", "text": "second"}
			],
			"stop_reason": "end_turn"
		}`))
	}))
	defer srv.Close()

	caller := newTestAnthropicCaller(srv.URL, secret, "")
	got, err := caller.Call(context.Background(), "summarize this")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/v1/messages" {
		t.Errorf("path = %q, want /v1/messages", gotPath)
	}
	if gotKey != secret {
		t.Errorf("x-api-key = %q, want the configured key", gotKey)
	}
	if gotVersion != "2023-06-01" {
		t.Errorf("anthropic-version = %q", gotVersion)
	}
	if gotContentType != "application/json" {
		t.Errorf("content-type = %q", gotContentType)
	}
	if gotBody.Model != DefaultAnthropicModel {
		t.Errorf("model = %q, want default %q", gotBody.Model, DefaultAnthropicModel)
	}
	if gotBody.MaxTokens <= 0 {
		t.Errorf("max_tokens = %d, want positive", gotBody.MaxTokens)
	}
	if gotBody.Thinking.Type != "adaptive" {
		t.Errorf("thinking.type = %q, want adaptive", gotBody.Thinking.Type)
	}
	if len(gotBody.Messages) != 1 || gotBody.Messages[0].Role != "user" || gotBody.Messages[0].Content != "summarize this" {
		t.Errorf("messages = %+v", gotBody.Messages)
	}

	// Text blocks are joined; the thinking block is ignored.
	if got != "first\nsecond" {
		t.Errorf("response = %q, want joined text blocks", got)
	}
}

func TestAnthropicCallerModelOverride(t *testing.T) {
	var gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body anthropicRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotModel = body.Model
		_, _ = w.Write([]byte(`{"content": [{"type": "text", "text": "ok"}], "stop_reason": "end_turn"}`))
	}))
	defer srv.Close()

	caller := newTestAnthropicCaller(srv.URL, "k", "claude-haiku-4-5")
	if _, err := caller.Call(context.Background(), "hi"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotModel != "claude-haiku-4-5" {
		t.Errorf("model = %q, want the override", gotModel)
	}
}

func TestAnthropicCallerErrorsNeverEchoTheKey(t *testing.T) {
	const secret = "sk-ant-very-secret"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type": "error", "error": {"type": "authentication_error", "message": "invalid x-api-key"}}`))
	}))
	defer srv.Close()

	caller := newTestAnthropicCaller(srv.URL, secret, "")
	_, err := caller.Call(context.Background(), "hi")
	if err == nil {
		t.Fatal("expected an error for a 401")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error message leaked the key: %v", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should carry the status: %v", err)
	}
}

func TestAnthropicCallerRefusalIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"content": [], "stop_reason": "refusal"}`))
	}))
	defer srv.Close()

	caller := newTestAnthropicCaller(srv.URL, "k", "")
	if _, err := caller.Call(context.Background(), "hi"); err == nil {
		t.Fatal("expected an error for a refusal stop reason")
	}
}

// TestAnthropicCallerNeverExposesKey mirrors TestBrokerNeverExposesToken for
// the Anthropic caller: the only serialization path in this codebase is
// encoding/json, and it must never surface the key.
func TestAnthropicCallerNeverExposesKey(t *testing.T) {
	const secret = "sk-ant-super-secret-do-not-leak"
	caller := NewAnthropicCaller(secret, "")
	b := New(caller)

	if j, err := json.Marshal(caller); err == nil && strings.Contains(string(j), secret) {
		t.Fatalf("json.Marshal(caller) leaked the key: %s", j)
	}
	if j, err := json.Marshal(b); err == nil && strings.Contains(string(j), secret) {
		t.Fatalf("json.Marshal(broker) leaked the key: %s", j)
	}
}
