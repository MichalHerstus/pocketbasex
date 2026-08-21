package pbai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	openai "github.com/sashabaranov/go-openai"

	"pbx/views"
)

func TestNewClientBaseURL(t *testing.T) {
	cases := []struct {
		name string
		cfg  views.AgentConfig
		want string
	}{
		{"lmstudio bare host", views.AgentConfig{Provider: "lmstudio", BaseURL: "http://127.0.0.1:1234"}, "http://127.0.0.1:1234/v1"},
		{"lmstudio trailing slash", views.AgentConfig{Provider: "lmstudio", BaseURL: "http://127.0.0.1:1234/"}, "http://127.0.0.1:1234/v1"},
		{"lmstudio already v1", views.AgentConfig{Provider: "lmstudio", BaseURL: "http://127.0.0.1:1234/v1"}, "http://127.0.0.1:1234/v1"},
		{"lmstudio other version kept", views.AgentConfig{Provider: "lmstudio", BaseURL: "http://host/api/v2"}, "http://host/api/v2"},
		{"openrouter untouched", views.AgentConfig{Provider: "openrouter", BaseURL: "http://127.0.0.1:1234"}, "http://127.0.0.1:1234"},
		{"empty defaults to openrouter", views.AgentConfig{Provider: "lmstudio"}, "https://openrouter.ai/api/v1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveBaseURL(tc.cfg)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// sseChunk writes one mock chat-completion response as a single SSE stream
// chunk (the pbai client now consumes the streaming API exclusively).
func sseChunk(w http.ResponseWriter, resp map[string]any) {
	w.Header().Set("Content-Type", "text/event-stream")
	b, _ := json.Marshal(resp)
	fmt.Fprintf(w, "data: %s\n\n", b)
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func setupTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	// create a test collection with a couple of fields
	coll := core.NewBaseCollection("products")
	f1 := core.Fields[core.FieldTypeText]()
	f1.SetName("title")
	f2 := core.Fields[core.FieldTypeNumber]()
	f2.SetName("price")
	coll.Fields.Add(f1, f2)
	if err := app.Save(coll); err != nil {
		t.Fatal(err)
	}

	// mirror the _actions collection from pb_migrations (TestApp does not run them)
	ac := core.NewBaseCollection("_actions")
	an := core.Fields[core.FieldTypeText]()
	an.SetName("_name")
	ad := core.Fields[core.FieldTypeText]()
	ad.SetName("_description")
	as := core.Fields[core.FieldTypeEditor]()
	as.SetName("_script")
	acoll := core.Fields[core.FieldTypeText]()
	acoll.SetName("_collection")
	ol := core.Fields[core.FieldTypeBool]()
	ol.SetName("_onList")
	of := core.Fields[core.FieldTypeBool]()
	of.SetName("_onForm")
	pub := core.Fields[core.FieldTypeBool]()
	pub.SetName("_public")
	ac.Fields.Add(an, ad, as, acoll, ol, of, pub)
	if err := app.Save(ac); err != nil {
		t.Fatal(err)
	}
	return app
}

// mockLLM returns an httptest server simulating an OpenAI-compatible endpoint.
// It responds with a list_collections tool call first, then a final text.
func mockLLM(t *testing.T) *httptest.Server {
	t.Helper()
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("bad request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			resp := map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"id": "call_1",
							"type": "function",
							"function": map[string]any{"name": "list_collections", "arguments": "{}"},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}
			sseChunk(w, resp)
			return
		}
		// second call: respond with a normal text answer
		resp := map[string]any{
			"choices": []map[string]any{{
				"delta":       map[string]any{"role": "assistant", "content": "Products collection exists."},
				"finish_reason": "stop",
			}},
		}
		sseChunk(w, resp)
	}))
	return srv
}

func TestAgentLoop(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	srv := mockLLM(t)
	defer srv.Close()

	cfg := views.AgentConfig{
		Provider: "mock",
		BaseURL:  srv.URL,
		APIKey:   "test-key",
		Model:    "mock-model",
	}

	// superuser request context
	su, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, "test@example.com")
	if err != nil {
		t.Fatalf("no superuser: %v", err)
	}
	info := &core.RequestInfo{Context: "ai", Auth: su}

	agent := NewAgent(app, info, cfg)
	res, err := agent.Run(context.Background(), []ChatMessage{{Role: "user", Content: "What collections are there?"}}, nil)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.PendingAction != nil {
		t.Fatalf("unexpected pending action: %+v", res.PendingAction)
	}
	if len(res.Transcript) == 0 {
		t.Fatalf("empty transcript: %+v", res)
	}
	last := res.Transcript[len(res.Transcript)-1]
	if last.Content != "Products collection exists." {
		t.Errorf("unexpected final answer: %q", last.Content)
	}
}

// TestToolCallHistoryShape is a regression test: the second LLM request must
// carry the assistant message with tool_calls immediately followed by the
// matching tool result (OpenAI protocol requirement). Without it, models
// re-issue the same tool call until the iteration cap.
func TestToolCallHistoryShape(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("bad request: %v", err)
		}
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		if len(bodies) == 1 {
			resp := map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"id":   "call_42",
							"type": "function",
							"function": map[string]any{
								"name":      "get_collection_schema",
								"arguments": `{"collection":"products"}`,
							},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}
			sseChunk(w, resp)
			return
		}
		resp := map[string]any{
			"choices": []map[string]any{{
				"delta":       map[string]any{"role": "assistant", "content": "done"},
				"finish_reason": "stop",
			}},
		}
		sseChunk(w, resp)
	}))
	defer srv.Close()

	su, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, "test@example.com")
	if err != nil {
		t.Fatalf("no superuser: %v", err)
	}
	cfg := views.AgentConfig{Provider: "mock", BaseURL: srv.URL, APIKey: "k", Model: "m"}
	agent := NewAgent(app, &core.RequestInfo{Context: "ai", Auth: su}, cfg)

	res, err := agent.Run(context.Background(), []ChatMessage{{Role: "user", Content: "fields of products?"}}, nil)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.FinalText != "done" {
		t.Fatalf("unexpected final text %q", res.FinalText)
	}

	if len(bodies) < 2 {
		t.Fatalf("expected 2 LLM calls, got %d", len(bodies))
	}
	msgs, ok := bodies[1]["messages"].([]any)
	if !ok || len(msgs) < 4 {
		t.Fatalf("second request should carry system,user,assistant,tool; got %d messages", len(msgs))
	}
	sawAssistantToolCalls := false
	for i, m := range msgs {
		mm, ok := m.(map[string]any)
		if !ok || mm["role"] != "assistant" {
			continue
		}
		tcs, ok := mm["tool_calls"].([]any)
		if !ok || len(tcs) == 0 {
			continue
		}
		sawAssistantToolCalls = true
		tc, _ := tcs[0].(map[string]any)
		id, _ := tc["id"].(string)
		if i+1 >= len(msgs) {
			t.Fatal("assistant tool_calls not followed by a tool result")
		}
		next, _ := msgs[i+1].(map[string]any)
		if next["role"] != "tool" {
			t.Fatalf("message after assistant tool_calls has role %v, want tool", next["role"])
		}
		if next["tool_call_id"] != id {
			t.Fatalf("tool_call_id mismatch: got %v, want %v", next["tool_call_id"], id)
		}
	}
	if !sawAssistantToolCalls {
		t.Fatal("second request history lacks an assistant message with tool_calls")
	}
}

// TestRunCapturesQueryRecords verifies that Run attaches the last
// query_records dataset to ChatResult.Records for UI table rendering.
func TestRunCapturesQueryRecords(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	coll, err := app.FindCachedCollectionByNameOrId("products")
	if err != nil {
		t.Fatalf("no products collection: %v", err)
	}
	rec := core.NewRecord(coll)
	rec.Set("title", "Widget")
	rec.Set("price", 99.5)
	if err := app.Save(rec); err != nil {
		t.Fatalf("seed save error: %v", err)
	}

	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			resp := map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"id":   "call_7",
							"type": "function",
							"function": map[string]any{
								"name":      "query_records",
								"arguments": `{"collection":"products","filter":"price > 50"}`,
							},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}
			sseChunk(w, resp)
			return
		}
		resp := map[string]any{
			"choices": []map[string]any{{
				"delta":       map[string]any{"role": "assistant", "content": "Here is what I found."},
				"finish_reason": "stop",
			}},
		}
		sseChunk(w, resp)
	}))
	defer srv.Close()

	su, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, "test@example.com")
	if err != nil {
		t.Fatalf("no superuser: %v", err)
	}
	cfg := views.AgentConfig{Provider: "mock", BaseURL: srv.URL, APIKey: "k", Model: "m"}
	agent := NewAgent(app, &core.RequestInfo{Context: "ai", Auth: su}, cfg)

	res, err := agent.Run(context.Background(), []ChatMessage{{Role: "user", Content: "products over 50?"}}, nil)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	// tabular fast path: a lone successful query_records must return after
	// exactly one LLM call - no follow-up round-trip for the final answer.
	if call != 1 {
		t.Fatalf("expected 1 LLM call (fast path), got %d", call)
	}
	if res.FinalText != "" {
		t.Fatalf("expected empty final text on fast path, got %q", res.FinalText)
	}
	if len(res.Records) != 1 {
		t.Fatalf("expected 1 captured record, got %d", len(res.Records))
	}
	if res.Records[0]["title"] != "Widget" {
		t.Errorf("wrong record title: %v", res.Records[0]["title"])
	}
}

// TestQueryRecordsFastPathNotForMultiStep verifies the fast-path guardrail:
// when query_records is not the first executed tool call of the turn
// (here: get_collection_schema runs first), the loop keeps its final LLM pass.
func TestQueryRecordsFastPathNotForMultiStep(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	coll, err := app.FindCachedCollectionByNameOrId("products")
	if err != nil {
		t.Fatalf("no products collection: %v", err)
	}
	rec := core.NewRecord(coll)
	rec.Set("title", "Widget")
	rec.Set("price", 99.5)
	if err := app.Save(rec); err != nil {
		t.Fatalf("seed save error: %v", err)
	}

	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		w.Header().Set("Content-Type", "application/json")
		var resp map[string]any
		switch call {
		case 1:
			resp = map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"id":   "call_1",
							"type": "function",
							"function": map[string]any{
								"name":      "get_collection_schema",
								"arguments": `{"collection":"products"}`,
							},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}
		case 2:
			resp = map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"id":   "call_2",
							"type": "function",
							"function": map[string]any{
								"name":      "query_records",
								"arguments": `{"collection":"products"}`,
							},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}
		default:
			resp = map[string]any{
				"choices": []map[string]any{{
					"delta":       map[string]any{"role": "assistant", "content": "Found 1 product."},
					"finish_reason": "stop",
				}},
			}
		}
		sseChunk(w, resp)
	}))
	defer srv.Close()

	su, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, "test@example.com")
	if err != nil {
		t.Fatalf("no superuser: %v", err)
	}
	cfg := views.AgentConfig{Provider: "mock", BaseURL: srv.URL, APIKey: "k", Model: "m"}
	agent := NewAgent(app, &core.RequestInfo{Context: "ai", Auth: su}, cfg)

	res, err := agent.Run(context.Background(), []ChatMessage{{Role: "user", Content: "products over 50?"}}, nil)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if call != 3 {
		t.Fatalf("expected 3 LLM calls in multi-step flow, got %d", call)
	}
	if res.FinalText != "Found 1 product." {
		t.Fatalf("unexpected final text %q", res.FinalText)
	}
	if len(res.Records) != 1 {
		t.Fatalf("expected 1 captured record, got %d", len(res.Records))
	}
}

// TestRunSendsHistoryToLLM verifies that prior conversation turns are
// passed through to the LLM ahead of the new user message.
func TestRunSendsHistoryToLLM(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	var gotRoles []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, m := range req.Messages {
			gotRoles = append(gotRoles, m.Role)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"role":"assistant","content":"Will do."},"finish_reason":null}]}`)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	su, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, "test@example.com")
	if err != nil {
		t.Fatalf("no superuser: %v", err)
	}
	cfg := views.AgentConfig{Provider: "mock", BaseURL: srv.URL, APIKey: "k", Model: "m"}
	agent := NewAgent(app, &core.RequestInfo{Context: "ai", Auth: su}, cfg)

	history := []ChatMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
	res, err := agent.Run(context.Background(), append(history, ChatMessage{Role: "user", Content: "add it"}), nil)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.FinalText != "Will do." {
		t.Fatalf("unexpected final text %q", res.FinalText)
	}
	want := []string{"system", "user", "assistant", "user"}
	if strings.Join(gotRoles, ",") != strings.Join(want, ",") {
		t.Fatalf("message roles = %v, want %v", gotRoles, want)
	}
}

// TestStreamAssemblyMultiChunk verifies that streamed tool-call argument
// fragments are assembled into a complete call that the loop can execute.
func TestStreamAssemblyMultiChunk(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	coll, err := app.FindCachedCollectionByNameOrId("products")
	if err != nil {
		t.Fatalf("no products collection: %v", err)
	}
	rec := core.NewRecord(coll)
	rec.Set("title", "Widget")
	rec.Set("price", 99.5)
	if err := app.Save(rec); err != nil {
		t.Fatalf("seed save error: %v", err)
	}

	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_9","type":"function","function":{"name":"query_records","arguments":"{\"collection\":\"prod"}}]}}]}`+"\n\n")
		fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ucts\"}"}}]}}]}`+"\n\n")
		fmt.Fprint(w, "data: "+`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	su, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, "test@example.com")
	if err != nil {
		t.Fatalf("no superuser: %v", err)
	}
	cfg := views.AgentConfig{Provider: "mock", BaseURL: srv.URL, APIKey: "k", Model: "m"}
	agent := NewAgent(app, &core.RequestInfo{Context: "ai", Auth: su}, cfg)

	res, err := agent.Run(context.Background(), []ChatMessage{{Role: "user", Content: "list products"}}, nil)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if call != 1 {
		t.Fatalf("expected 1 LLM call (assembled call fast-pathed), got %d", call)
	}
	if len(res.Records) != 1 || res.Records[0]["title"] != "Widget" {
		t.Fatalf("expected the seeded record, got %v", res.Records)
	}
}

// TestStreamRetriesEmptyCompletion simulates the LM Studio flake where a
// stream terminates immediately with an empty delta and finish_reason
// "stop"; Stream must retry and succeed on a later attempt.
func TestStreamRetriesEmptyCompletion(t *testing.T) {
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		w.Header().Set("Content-Type", "text/event-stream")
		if call == 1 {
			// empty completion: one chunk, no content, immediate stop
			fmt.Fprint(w, `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`+"\n\n")
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		fmt.Fprintf(w, `data: %s`+"\n\n", `{"choices":[{"delta":{"role":"assistant","content":"Collections available: a, b"}}]}`)
		fmt.Fprint(w, `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	cfg := views.AgentConfig{Provider: "mock", BaseURL: srv.URL, Model: "m"}
	app := setupTestApp(t)
	defer app.Cleanup()
	agent := NewAgent(app, nil, cfg)

	res, err := agent.Run(context.Background(), []ChatMessage{{Role: "user", Content: "list collections"}}, nil)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if call != 2 {
		t.Fatalf("expected 2 LLM calls (1 empty + 1 retry), got %d", call)
	}
	if res.FinalText != "Collections available: a, b" {
		t.Fatalf("unexpected final text %q", res.FinalText)
	}
}

// TestStreamEmptyExhaustsRetries verifies that after all retries the
// diagnostic error - not a crash - is surfaced.
func TestStreamEmptyExhaustsRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	cfg := views.AgentConfig{Provider: "mock", BaseURL: srv.URL, Model: "m"}
	client := NewClient(cfg)
	_, err := client.Stream(context.Background(), []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "hi"},
	}, nil, nil)
	if err == nil {
		t.Fatal("expected error after exhausted retries")
	}
	if !strings.Contains(err.Error(), "empty response from model") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestFindCollectionSuggestsTypo verifies that a misspelled collection name
// produces a self-correcting error with a did-you-mean suggestion.
func TestFindCollectionSuggestsTypo(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	su, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, "test@example.com")
	if err != nil {
		t.Fatalf("no superuser: %v", err)
	}
	agent := NewAgent(app, &core.RequestInfo{Context: "ai", Auth: su}, views.AgentConfig{})

	coll, err := agent.findCollection("products")
	if err != nil || coll == nil {
		t.Fatalf("exact match failed: %v", err)
	}

	_, err = agent.findCollection("produkti")
	if err == nil {
		t.Fatal("expected error for misspelled collection")
	}
	if !strings.Contains(err.Error(), `"products"`) {
		t.Errorf("error lacks suggestion: %v", err)
	}
	if !strings.Contains(err.Error(), "Available collections:") {
		t.Errorf("error lacks available list: %v", err)
	}

	// the query_records tool surfaces the same self-correcting error
	out, err := findTool("query_records").exec(agent, json.RawMessage(`{"collection":"produkti"}`))
	if err == nil {
		t.Fatalf("expected tool error, got output %q", out)
	}
	if !strings.Contains(err.Error(), `"products"`) {
		t.Errorf("tool error lacks suggestion: %v", err)
	}
}

func TestInsertRecordsPendingAndConfirm(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	srv := mockLLM(t)
	defer srv.Close()

	cfg := views.AgentConfig{
		Provider: "mock",
		BaseURL:  srv.URL,
		APIKey:   "test-key",
		Model:    "mock-model",
	}

	su, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, "test@example.com")
	if err != nil {
		t.Fatalf("no superuser: %v", err)
	}
	info := &core.RequestInfo{Context: "ai", Auth: su}

	agent := NewAgent(app, info, cfg)

	// simulate the pending action creation via the tool directly
	args := json.RawMessage(`{"collection":"products","records":[{"title":"Widget","price":9.99}]}`)
	tool := findTool("insert_records")
	if tool == nil {
		t.Fatal("insert_records tool not found")
	}
	p, err := tool.pending(agent, args)
	if err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if p.Type != "insert_records" {
		t.Errorf("wrong type: %s", p.Type)
	}

	stored := storePending(p)
	res, err := agent.Confirm(context.Background(), stored.ID, true)
	if err != nil {
		t.Fatalf("confirm error: %v", err)
	}
	if !res.OK {
		t.Fatalf("confirm not ok: %s", res.Message)
	}

	// verify the record was created
	recs, err := app.FindRecordsByFilter("products", "", "", 10, 0)
	if err != nil || len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d err=%v", len(recs), err)
	}
	if recs[0].GetString("title") != "Widget" {
		t.Errorf("wrong title: %q", recs[0].GetString("title"))
	}
}

func TestInsertDeniedForNonSuperuserWhenCreateRuleNil(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	// the products collection has createRule=nil (default) → only superusers
	user, err := app.FindAuthRecordByEmail("users", "test@example.com")
	if err != nil {
		t.Fatalf("no user: %v", err)
	}
	info := &core.RequestInfo{Context: "ai", Auth: user}
	cfg := views.AgentConfig{Provider: "mock", BaseURL: "http://127.0.0.1:1", Model: "m"}

	agent := NewAgent(app, info, cfg)
	tool := findTool("insert_records")
	args := json.RawMessage(`{"collection":"products","records":[{"title":"x"}]}`)
	if _, err := tool.pending(agent, args); err == nil {
		t.Fatal("expected permission error in pending, got nil")
	}
}

func TestCreateActionPendingAndConfirm(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	su, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, "test@example.com")
	if err != nil {
		t.Fatalf("no superuser: %v", err)
	}
	info := &core.RequestInfo{Context: "ai", Auth: su}
	cfg := views.AgentConfig{Provider: "mock", BaseURL: "http://127.0.0.1:1", Model: "m"}
	agent := NewAgent(app, info, cfg)

	tool := findTool("create_action")
	if tool == nil {
		t.Fatal("create_action tool not found")
	}
	if !tool.write {
		t.Fatal("create_action must be a write tool")
	}

	args := json.RawMessage(`{"name":"Count products","collection":"products","script":"log(count('products'))"}`)
	p, err := tool.pending(agent, args)
	if err != nil {
		t.Fatalf("pending error: %v", err)
	}
	if p.Type != "create_action" {
		t.Errorf("wrong type: %s", p.Type)
	}

	stored := storePending(p)
	res, err := agent.Confirm(context.Background(), stored.ID, true)
	if err != nil {
		t.Fatalf("confirm error: %v", err)
	}
	if !res.OK {
		t.Fatalf("confirm not ok: %s", res.Message)
	}

	recs, err := app.FindRecordsByFilter("_actions", "_name = {:name}", "", 1, 0, dbx.Params{"name": "Count products"})
	if err != nil || len(recs) != 1 {
		t.Fatalf("expected 1 action record, got %d err=%v", len(recs), err)
	}
	rec := recs[0]
	if rec.GetString("_collection") != "products" {
		t.Errorf("wrong _collection: %q", rec.GetString("_collection"))
	}
	// defaults: onList=true, onForm=false, public=false
	if !rec.GetBool("_onList") || rec.GetBool("_onForm") || rec.GetBool("_public") {
		t.Errorf("wrong flag defaults: onList=%v onForm=%v public=%v", rec.GetBool("_onList"), rec.GetBool("_onForm"), rec.GetBool("_public"))
	}

	// upsert by name: same name updates the existing record
	args2 := json.RawMessage(`{"name":"Count products","collection":"products","script":"log(1)","description":"updated","onForm":true,"public":true}`)
	p2, err := tool.pending(agent, args2)
	if err != nil {
		t.Fatalf("pending error (update): %v", err)
	}
	stored2 := storePending(p2)
	res2, err := agent.Confirm(context.Background(), stored2.ID, true)
	if err != nil || !res2.OK {
		t.Fatalf("confirm error: %v / %v", err, res2)
	}
	recs2, err := app.FindRecordsByFilter("_actions", "_name = {:name}", "", 10, 0, dbx.Params{"name": "Count products"})
	if err != nil || len(recs2) != 1 {
		t.Fatalf("expected still 1 action record after upsert, got %d err=%v", len(recs2), err)
	}
	if recs2[0].GetString("_description") != "updated" || !recs2[0].GetBool("_onForm") || !recs2[0].GetBool("_public") {
		t.Errorf("upsert did not apply changes: %+v", recs2[0].PublicExport())
	}
}

func TestCreateActionRejectsBadScript(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	su, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, "test@example.com")
	if err != nil {
		t.Fatalf("no superuser: %v", err)
	}
	info := &core.RequestInfo{Context: "ai", Auth: su}
	agent := NewAgent(app, info, views.AgentConfig{})

	tool := findTool("create_action")
	args := json.RawMessage(`{"name":"Broken","collection":"products","script":"function( {"}`)
	if _, err := tool.pending(agent, args); err == nil {
		t.Fatal("expected compile error for invalid script, got nil")
	}
}

func TestActionToolsDeniedForNonSuperuser(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	user, err := app.FindAuthRecordByEmail("users", "test@example.com")
	if err != nil {
		t.Fatalf("no user: %v", err)
	}
	info := &core.RequestInfo{Context: "ai", Auth: user}
	agent := NewAgent(app, info, views.AgentConfig{})

	createArgs := json.RawMessage(`{"name":"x","collection":"products","script":"log(1)"}`)
	if _, err := findTool("create_action").pending(agent, createArgs); err == nil {
		t.Error("expected permission error for create_action pending")
	}
	if _, err := findTool("list_actions").exec(agent, json.RawMessage(`{"collection":"products"}`)); err == nil {
		t.Error("expected permission error for list_actions exec")
	}
}

func TestListActions(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	su, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, "test@example.com")
	if err != nil {
		t.Fatalf("no superuser: %v", err)
	}
	info := &core.RequestInfo{Context: "ai", Auth: su}
	agent := NewAgent(app, info, views.AgentConfig{})

	// empty case
	out, err := findTool("list_actions").exec(agent, json.RawMessage(`{"collection":"products"}`))
	if err != nil {
		t.Fatalf("list_actions error: %v", err)
	}
	if out != `No custom actions defined for collection "products".` {
		t.Errorf("unexpected empty output: %q", out)
	}

	// seed an action record
	actionsColl, err := app.FindCachedCollectionByNameOrId("_actions")
	if err != nil {
		t.Fatalf("no _actions collection: %v", err)
	}
	rec := core.NewRecord(actionsColl)
	rec.Set("_name", "Tag all")
	rec.Set("_description", "Tags selected records")
	rec.Set("_script", "log(1)")
	rec.Set("_collection", "products")
	rec.Set("_onList", true)
	rec.Set("_onForm", false)
	rec.Set("_public", true)
	if err := app.Save(rec); err != nil {
		t.Fatalf("seed save error: %v", err)
	}

	out, err = findTool("list_actions").exec(agent, json.RawMessage(`{"collection":"products"}`))
	if err != nil {
		t.Fatalf("list_actions error: %v", err)
	}
	want := "- Tag all [list,public]: Tags selected records"
	if out != want {
		t.Errorf("unexpected output:\n%q\nwant:\n%q", out, want)
	}
}