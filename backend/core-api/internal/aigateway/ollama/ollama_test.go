package ollama_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/aigateway"
	"github.com/example/core-platform/backend/core-api/internal/aigateway/ollama"
)

// These tests run a real net/http/httptest.Server speaking the exact
// wire shape a real Ollama (or any OpenAI-compatible) server returns -
// a genuine HTTP round trip through Provider.Complete's real request
// marshaling and response parsing, not a mocked interface. The actual
// live server is exercised separately in this phase's live validation
// (see VALIDATION.md), which this test can't depend on.
func TestCompleteParsesARealResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req["model"] != "qwen2.5:0.5b" {
			t.Fatalf("unexpected model in request: %v", req["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"role": "assistant", "content": "Hello there!"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 12, "completion_tokens": 3}
		}`))
	}))
	defer srv.Close()

	p := ollama.New(ollama.Config{BaseURL: srv.URL})
	result, err := p.Complete(context.Background(), "qwen2.5:0.5b", []aigateway.Message{{Role: "user", Content: "hi"}}, 50, 0.7)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if result.Text != "Hello there!" || result.PromptTokens != 12 || result.CompletionTokens != 3 || result.FinishReason != "stop" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestCompleteReturnsErrorOnNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": {"message": "model not found"}}`))
	}))
	defer srv.Close()

	p := ollama.New(ollama.Config{BaseURL: srv.URL})
	_, err := p.Complete(context.Background(), "nonexistent", []aigateway.Message{{Role: "user", Content: "hi"}}, 50, 0.7)
	if err == nil {
		t.Fatal("expected an error for a non-OK response")
	}
}

func TestCompleteRespectsContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // never respond - simulates a hung provider
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled

	p := ollama.New(ollama.Config{BaseURL: srv.URL})
	_, err := p.Complete(ctx, "qwen2.5:0.5b", []aigateway.Message{{Role: "user", Content: "hi"}}, 50, 0.7)
	if err == nil {
		t.Fatal("expected an error for an already-canceled context")
	}
}

func TestNameIsOllama(t *testing.T) {
	p := ollama.New(ollama.Config{BaseURL: "http://unused"})
	if p.Name() != "ollama" {
		t.Fatalf("expected Name() to be \"ollama\", got %q", p.Name())
	}
}
