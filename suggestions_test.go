package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func newSuggestionsApp(t *testing.T) (*tests.TestApp, http.Handler, *core.Record) {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	router.GET("/api/ai/suggestions", func(e *core.RequestEvent) error { return handleAISuggestions(e) })
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	usersColl, err := app.FindCachedCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	u := core.NewRecord(usersColl)
	u.Set("email", "alice@example.com")
	u.Set("password", "password123")
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	return app, mux, u
}

func TestAISuggestionsRequiresAuth(t *testing.T) {
	_, mux, _ := newSuggestionsApp(t)

	req := httptest.NewRequest(http.MethodGet, "/api/ai/suggestions", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without auth, got %d", rec.Code)
	}
}

func TestAISuggestionsReturnsCollections(t *testing.T) {
	_, mux, u := newSuggestionsApp(t)
	token, err := u.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/ai/suggestions", nil)
	req.AddCookie(&http.Cookie{Name: "pb_auth", Value: token})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body struct {
		Collections []struct {
			Name  string `json:"name"`
			Count int64  `json:"count"`
		} `json:"collections"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(body.Collections) == 0 {
		t.Fatal("expected at least one suggested collection")
	}
	if len(body.Collections) > 5 {
		t.Fatalf("expected max 5 suggestions, got %d", len(body.Collections))
	}
	for _, c := range body.Collections {
		if c.Count < 1 {
			t.Fatalf("suggestion %q should have a positive count, got %d", c.Name, c.Count)
		}
	}
}