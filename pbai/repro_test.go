package pbai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"pbx/views"
)

// TestSingleRecordRenderIsEmitted is a regression test for the "detail view"
// chat flow with a single-match record: the terminal response must carry a
// non-empty server-rendered fragment (a detail card) AND the records, so the
// chat page renders them instead of silently showing nothing.
func TestSingleRecordRenderIsEmitted(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	coll := core.NewBaseCollection("produkty")
	for _, f := range []struct {
		name string
		typ  string
	}{{"productCode", core.FieldTypeText}, {"prodShortName", core.FieldTypeText}, {"prodPrice", core.FieldTypeNumber}} {
		cf := core.Fields[f.typ]()
		cf.SetName(f.name)
		coll.Fields.Add(cf)
	}
	if err := app.Save(coll); err != nil {
		t.Fatal(err)
	}
	for _, rec := range []struct {
		code, name string
		price      float64
	}{
		{"p0001", "Widget", 9.99},
		{"p0002", "Gadget", 19.99},
	} {
		r := core.NewRecord(coll)
		r.Set("productCode", rec.code)
		r.Set("prodShortName", rec.name)
		r.Set("prodPrice", rec.price)
		if err := app.Save(r); err != nil {
			t.Fatal(err)
		}
	}

	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{"choices": []map[string]any{{"delta": map[string]any{
			"role": "assistant",
			"tool_calls": []map[string]any{{"id": "call_1", "type": "function", "function": map[string]any{
				"name":      "query_records",
				"arguments": `{"collection":"produkty","filter":"productCode = 'p0001'"}`,
			}}},
		}, "finish_reason": "tool_calls"}}}

		b, _ := json.Marshal(resp)
		fmt.Fprintf(w, "data: %s\n\n", b)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	su, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, "test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	agent := NewAgent(app, &core.RequestInfo{Context: "ai", Auth: su}, views.AgentConfig{Provider: "mock", BaseURL: srv.URL, APIKey: "k", Model: "m"})

	res, err := agent.Run(context.Background(), []ChatMessage{{Role: "user", Content: "show record detail view for productcode=p0001 from collection Produkty"}}, nil)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(res.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(res.Records))
	}
	if res.Records[0]["productCode"] != "p0001" {
		t.Errorf("wrong record matched: %v", res.Records[0])
	}
	if res.Render == "" {
		t.Fatal("Render must be non-empty for a single-record answer")
	}
	if !strings.Contains(res.Render, "ai-detail") {
		t.Errorf("single-record render should be a detail card, got:\n%s", res.Render)
	}
}

// TestRecordsWithoutRenderFallback documents the client contract: when a
// response carries records, the render fragment must be non-empty; if a
// downstream caller ever returns records with an empty render the UI falls
// back to its own table renderer for them (never silently drops the answer).
func TestRecordsAlwaysHaveRender(t *testing.T) {
	for _, records := range [][]map[string]any{
		{{"id": "1", "collectionName": "x", "a": "1"}},
		{{"id": "1", "collectionName": "x", "a": "1"}, {"id": "2", "collectionName": "x", "a": "2"}},
	} {
		res := &ChatResult{Records: records}
		if out := RenderResult(nil, res); out == "" {
			t.Errorf("records present but render empty: %v", records)
		}
	}
}