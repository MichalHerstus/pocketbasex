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
	"io"
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
// base URL defaults to the OpenAI endpoint. For LM Studio a missing /v1
// suffix is appended automatically (a bare host URL would hit paths that
// answer HTTP 200 with an error body and no choices).
func NewClient(cfg views.AgentConfig) *Client {
	cc := openai.DefaultConfig(cfg.APIKey)
	cc.BaseURL = resolveBaseURL(cfg)

	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 90
	}
	cc.HTTPClient = &http.Client{Timeout: time.Duration(timeout) * time.Second}

	return &Client{client: openai.NewClientWithConfig(cc), cfg: cfg}
}

// Model returns the configured model identifier.
func (c *Client) Model() string { return c.cfg.Model }

// resolveBaseURL normalizes the configured base URL: trailing slashes are
// trimmed, a missing base URL defaults to the OpenRouter endpoint, and for
// LM Studio a missing /v1 suffix is appended automatically (a bare host URL
// would hit paths that answer HTTP 200 with an error body and no choices).
func resolveBaseURL(cfg views.AgentConfig) string {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	if cfg.Provider == "lmstudio" {
		baseURL = ensureV1(baseURL)
	}
	return baseURL
}

// ensureV1 appends the /v1 API prefix when the base URL does not already end
// in a version segment (e.g. /v1, /v2).
func ensureV1(baseURL string) string {
	if i := strings.LastIndex(baseURL, "/"); i >= 0 {
		seg := baseURL[i+1:]
		if len(seg) > 1 && seg[0] == 'v' && allDigits(seg[1:]) {
			return baseURL
		}
	}
	return baseURL + "/v1"
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

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
		return resp, fmt.Errorf("empty response from model %q (verify that the base URL points at the OpenAI-compatible /v1 endpoint)", c.cfg.Model)
	}
	return resp, nil
}

// StreamResult carries the fully assembled assistant message produced from
// a streamed chat completion plus the finish reason of the last chunk.
type StreamResult struct {
	Msg          openai.ChatCompletionMessage
	FinishReason string
}

// maxEmptyStreamRetries bounds how many times an empty streamed completion
// is re-requested before giving up. LM Studio (and some other providers)
// intermittently terminate a stream immediately with an empty delta and
// finish_reason "stop"; a retry usually generates the intended reply.
const maxEmptyStreamRetries = 2

// Stream sends a streaming chat completion request and forwards content
// deltas to onText as they arrive (nil onText is allowed). Tool-call deltas
// are assembled by index into a single message that is returned once the
// stream ends. An empty stream is retried a couple of times before the same
// diagnostic error as Complete is returned.
func (c *Client) Stream(ctx context.Context, messages []openai.ChatCompletionMessage, tools []openai.Tool, onText func(string)) (*StreamResult, error) {
	for attempt := 0; ; attempt++ {
		res, empty, err := c.streamOnce(ctx, messages, tools, onText)
		if err != nil {
			return nil, err
		}
		if !empty {
			return res, nil
		}
		if attempt >= maxEmptyStreamRetries {
			return nil, fmt.Errorf("empty response from model %q after %d attempts (verify that the base URL points at the OpenAI-compatible /v1 endpoint)", c.cfg.Model, attempt+1)
		}
	}
}

// streamOnce performs a single streaming chat completion request. The second
// return value reports whether the completion came back empty (no content,
// no tool calls) - the caller decides whether to retry.
func (c *Client) streamOnce(ctx context.Context, messages []openai.ChatCompletionMessage, tools []openai.Tool, onText func(string)) (*StreamResult, bool, error) {
	req := openai.ChatCompletionRequest{
		Model:    c.cfg.Model,
		Messages: messages,
		Stream:   true,
	}
	if len(tools) > 0 {
		req.Tools = tools
	}
	stream, err := c.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, false, err
	}
	defer stream.Close()

	res := &StreamResult{Msg: openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant}}
	chunks := 0
	for {
		resp, rerr := stream.Recv()
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, false, rerr
		}
		chunks++
		if len(resp.Choices) == 0 {
			continue
		}
		choice := resp.Choices[0]
		if choice.Delta.Content != "" {
			res.Msg.Content += choice.Delta.Content
			if onText != nil {
				onText(choice.Delta.Content)
			}
		}
		for _, tc := range choice.Delta.ToolCalls {
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}
			for idx >= len(res.Msg.ToolCalls) {
				res.Msg.ToolCalls = append(res.Msg.ToolCalls, openai.ToolCall{})
			}
			dst := &res.Msg.ToolCalls[idx]
			if tc.ID != "" {
				dst.ID = tc.ID
			}
			if tc.Type != "" {
				dst.Type = tc.Type
			}
			if tc.Function.Name != "" && dst.Function.Name == "" {
				dst.Function.Name = tc.Function.Name
			}
			dst.Function.Arguments += tc.Function.Arguments
		}
		if choice.FinishReason != "" {
			res.FinishReason = string(choice.FinishReason)
		}
	}
	empty := chunks == 0 || (res.Msg.Content == "" && len(res.Msg.ToolCalls) == 0)
	return res, empty, nil
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