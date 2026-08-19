package pbai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"pbx/views"
)

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
					"message": map[string]any{
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
			json.NewEncoder(w).Encode(resp)
			return
		}
		// second call: respond with a normal text answer
		resp := map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "Products collection exists."},
				"finish_reason": "stop",
			}},
		}
		json.NewEncoder(w).Encode(resp)
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