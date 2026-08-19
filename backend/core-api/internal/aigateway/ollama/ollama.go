// Package ollama implements aigateway.Provider against a real, local
// Ollama server - the one provider this phase ships as genuinely
// live-testable without any vendor API key, matching the roadmap's own
// "local models, Ollama" option. It speaks Ollama's OpenAI-compatible
// /v1/chat/completions endpoint deliberately, not Ollama's own native
// /api/generate shape: the request/response bodies below are
// structurally the same ones a real OpenAI provider adapter would use,
// so adding one later is a base-URL-and-auth-header change, not a
// second incompatible client implementation.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/aigateway"
)

type Config struct {
	BaseURL string
	// Timeout is a backstop on the underlying HTTP client - the real
	// per-call deadline is the context Service.attempt already applies
	// (this phase's "timeouts" capability); this just guards against a
	// client that somehow ignores ctx.
	Timeout time.Duration
}

type Provider struct {
	baseURL string
	client  *http.Client
}

func New(cfg Config) *Provider {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Provider{baseURL: cfg.BaseURL, client: &http.Client{Timeout: timeout}}
}

func (p *Provider) Name() string { return "ollama" }

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (p *Provider) Complete(ctx context.Context, model string, messages []aigateway.Message, maxTokens int, temperature float64) (aigateway.ProviderResult, error) {
	chatMessages := make([]chatMessage, 0, len(messages))
	for _, m := range messages {
		chatMessages = append(chatMessages, chatMessage{Role: m.Role, Content: m.Content})
	}
	reqBody, err := json.Marshal(chatRequest{Model: model, Messages: chatMessages, MaxTokens: maxTokens, Temperature: temperature})
	if err != nil {
		return aigateway.ProviderResult{}, fmt.Errorf("marshal ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return aigateway.ProviderResult{}, fmt.Errorf("build ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return aigateway.ProviderResult{}, fmt.Errorf("call ollama: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return aigateway.ProviderResult{}, fmt.Errorf("read ollama response: %w", err)
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return aigateway.ProviderResult{}, fmt.Errorf("parse ollama response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if parsed.Error != nil {
			return aigateway.ProviderResult{}, fmt.Errorf("ollama error (%d): %s", resp.StatusCode, parsed.Error.Message)
		}
		return aigateway.ProviderResult{}, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}
	if len(parsed.Choices) == 0 {
		return aigateway.ProviderResult{}, fmt.Errorf("ollama returned no choices")
	}

	return aigateway.ProviderResult{
		Text:             parsed.Choices[0].Message.Content,
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
		FinishReason:     parsed.Choices[0].FinishReason,
	}, nil
}
