// Package pbai implements the built-in AI agent: an LLM-backed chat that can
// inspect PocketBase collections and (with explicit user confirmation) insert
// records, create collections and manage view configurations.
//
// The agent talks to any OpenAI-compatible chat completions endpoint
// (OpenRouter or a local LM Studio server).
package pbai

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"pbx/views"
)

// Client wraps an OpenAI-compatible chat completions client plus the agent
// configuration that was used to build it.
type Client struct {
	client *openai.Client
	cfg    views.AgentConfig
}

// NewClient builds a chat completions client from the given agent config.
// The configured base URL may be trimmed of trailing slashes; a missing
// base URL defaults to the OpenAI endpoint.
func NewClient(cfg views.AgentConfig) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}

	cc := openai.DefaultConfig(cfg.APIKey)
	cc.BaseURL = baseURL

	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 90
	}
	cc.HTTPClient = &http.Client{Timeout: time.Duration(timeout) * time.Second}

	return &Client{client: openai.NewClientWithConfig(cc), cfg: cfg}
}

// Model returns the configured model identifier.
func (c *Client) Model() string { return c.cfg.Model }

// Complete sends a chat completion request with the given messages and
// optional tools and returns the first non-empty response choice.
func (c *Client) Complete(ctx context.Context, messages []openai.ChatCompletionMessage, tools []openai.Tool) (openai.ChatCompletionResponse, error) {
	req := openai.ChatCompletionRequest{
		Model:    c.cfg.Model,
		Messages: messages,
	}
	if len(tools) > 0 {
		req.Tools = tools
	}
	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return resp, err
	}
	if len(resp.Choices) == 0 {
		return resp, fmt.Errorf("empty response from model %q", c.cfg.Model)
	}
	return resp, nil
}

// ImagePart builds a multimodal content part from a base64-encoded image.
func ImagePart(mime, b64 string) openai.ChatMessagePart {
	return openai.ChatMessagePart{
		Type: openai.ChatMessagePartTypeImageURL,
		ImageURL: &openai.ChatMessageImageURL{
			URL:    "data:" + mime + ";base64," + b64,
			Detail: openai.ImageURLDetailAuto,
		},
	}
}

// TextPart builds a plain text content part.
func TextPart(text string) openai.ChatMessagePart {
	return openai.ChatMessagePart{
		Type: openai.ChatMessagePartTypeText,
		Text: text,
	}
}