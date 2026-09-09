package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// newConversationsTestApp builds a TestApp with the _conversations collection
// (mirroring the migration) and a registered conversations router, plus a ready
// user auth cookie for the given user record.
func newConversationsTestApp(t *testing.T) (*tests.TestApp, http.Handler, *core.Record) {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	coll := core.NewBaseCollection("_conversations")
	for _, f := range []struct {
		name string
		typ  string
	}{
		{"_title", core.FieldTypeText},
		{"_messages", core.FieldTypeJSON},
		{"_user", core.FieldTypeText},
		{"_model", core.FieldTypeText},
	} {
		cf := core.Fields[f.typ]()
		cf.SetName(f.name)
		coll.Fields.Add(cf)
	}
	// autodate fields matching the migration (required by -updated sort)
	for _, name := range []string{"created", "updated"} {
		af, ok := core.Fields[core.FieldTypeAutodate]().(*core.AutodateField)
		if !ok {
			t.Fatal("autodate field type mismatch")
		}
		af.SetName(name)
		af.OnCreate = name == "created" || name == "updated"
		af.OnUpdate = name == "updated"
		coll.Fields.Add(af)
	}
	if err := app.Save(coll); err != nil {
		t.Fatal(err)
	}

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	router.GET("/ai/conversations", func(e *core.RequestEvent) error { return handleAgentConversationsList(e) })
	router.POST("/ai/conversations/save", func(e *core.RequestEvent) error { return handleAgentConversationSave(e) })
	router.GET("/ai/conversations/{id}", func(e *core.RequestEvent) error { return handleAgentConversationGet(e) })
	router.POST("/ai/conversations/delete", func(e *core.RequestEvent) error { return handleAgentConversationDelete(e) })
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	usersColl, err := app.FindCachedCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	userRec := core.NewRecord(usersColl)
	userRec.Set("email", "alice@example.com")
	userRec.Set("password", "password123")
	if err := app.Save(userRec); err != nil {
		t.Fatal(err)
	}

	return app, mux, userRec
}

func cookieReq(t *testing.T, token string, method, path string, body any) *http.Request {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: "pb_auth", Value: token})
	}
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestConversationPersistenceFlow(t *testing.T) {
	app, mux, userRec := newConversationsTestApp(t)
	token, err := userRec.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	// 1) list is empty
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, cookieReq(t, token, http.MethodGet, "/ai/conversations", nil))
	if rec.Code != 200 {
		t.Fatalf("list: status %d body=%s", rec.Code, rec.Body.String())
	}
	var list struct {
		Conversations []agentConvSummaries `json:"conversations"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Conversations) != 0 {
		t.Fatalf("expected empty list, got %d", len(list.Conversations))
	}

	// 2) save a new conversation (fire-and-forget upsert)
	saveBody := map[string]any{
		"messages": []agentConvMessage{
			{Role: "user", Content: "how many products are there in the whole dataset? ask the table view"},
			{Role: "assistant", Content: "12"},
		},
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, cookieReq(t, token, http.MethodPost, "/ai/conversations/save", saveBody))
	if rec.Code != 200 {
		t.Fatalf("save: status %d body=%s", rec.Code, rec.Body.String())
	}
	var saved struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil || saved.ID == "" {
		t.Fatalf("save: no id returned: %v body=%s", err, rec.Body.String())
	}

	// 3) list now has one conversation with a derived title
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, cookieReq(t, token, http.MethodGet, "/ai/conversations", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Conversations) != 1 {
		t.Fatalf("expected 1 conversation after save, got %d", len(list.Conversations))
	}
	gotTitle := list.Conversations[0].Title
	wantTitle := restring("how many products are there in the whole dataset? ask the table view", 48)
	if gotTitle != wantTitle {
		t.Errorf("title mismatch: got %q want %q", gotTitle, wantTitle)
	}

	// 4) get it back and inspect the messages
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, cookieReq(t, token, http.MethodGet, "/ai/conversations/"+saved.ID, nil))
	if rec.Code != 200 {
		t.Fatalf("get: status %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		ID       string             `json:"id"`
		Title    string             `json:"title"`
		Messages []agentConvMessage `json:"messages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 2 || got.Messages[0].Role != "user" || got.Messages[1].Content != "12" {
		t.Errorf("unexpected messages: %#v", got.Messages)
	}

	// 5) updating with the same conversationId keeps ONE record (upsert)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, cookieReq(t, token, http.MethodPost, "/ai/conversations/save", map[string]any{
		"conversationId": saved.ID,
		"messages": []agentConvMessage{
			{Role: "user", Content: "how many products are there in the whole dataset? ask the table view"},
			{Role: "assistant", Content: "12"},
			{Role: "user", Content: "and in the small view?"},
			{Role: "assistant", Content: "3"},
		},
	}))
	if rec.Code != 200 {
		t.Fatalf("upsert: status %d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, cookieReq(t, token, http.MethodGet, "/ai/conversations", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Conversations) != 1 {
		t.Fatalf("upsert should keep 1 conversation, got %d", len(list.Conversations))
	}

	// 6) another user cannot read it
	usersColl, err := app.FindCachedCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user2 := core.NewRecord(usersColl)
	user2.Set("email", "bob@example.com")
	user2.Set("password", "password123")
	if err := app.Save(user2); err != nil {
		t.Fatal(err)
	}
	tok2, err := user2.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, cookieReq(t, tok2, http.MethodGet, "/ai/conversations/"+saved.ID, nil))
	if rec.Code != 404 {
		t.Fatalf("other user should get 404, got %d", rec.Code)
	}

	// 7) owner deletes it
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, cookieReq(t, token, http.MethodPost, "/ai/conversations/delete", map[string]any{"id": saved.ID}))
	if rec.Code != 200 {
		t.Fatalf("delete: status %d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, cookieReq(t, token, http.MethodGet, "/ai/conversations", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Conversations) != 0 {
		t.Fatalf("after delete expected 0 conversations, got %d", len(list.Conversations))
	}
}

// TestConversationGhostOfTheCurrentUser ensures a request without a pb_auth
// cookie cannot list/save (unauthenticated clients get an empty list / a 403).
func TestConversationUnauthenticated(t *testing.T) {
	_, mux, _ := newConversationsTestApp(t)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, cookieReq(t, "", http.MethodGet, "/ai/conversations", nil))
	if rec.Code != 200 {
		t.Fatalf("list should be 200 with empty list, got %d", rec.Code)
	}
	var list struct {
		Conversations []agentConvSummaries `json:"conversations"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Conversations) != 0 {
		t.Fatalf("unauthenticated list should be empty, got %d", len(list.Conversations))
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, cookieReq(t, "", http.MethodPost, "/ai/conversations/save", map[string]any{
		"messages": []agentConvMessage{{Role: "user", Content: "hi"}},
	}))
	if rec.Code != 403 {
		t.Fatalf("unauthenticated save should be 403, got %d", rec.Code)
	}
}

// TestConversationSuperuserSeesAll ensures superusers list every owner's
// conversations, not just their own.
func TestConversationSuperuserSeesAll(t *testing.T) {
	app, mux, _ := newConversationsTestApp(t)

	usersColl, err := app.FindCachedCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	u1 := core.NewRecord(usersColl)
	u1.Set("email", "carol@example.com")
	u1.Set("password", "password123")
	if err := app.Save(u1); err != nil {
		t.Fatal(err)
	}
	tok1, _ := u1.NewAuthToken()

	// carol saves one
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, cookieReq(t, tok1, http.MethodPost, "/ai/conversations/save", map[string]any{
		"messages": []agentConvMessage{{Role: "user", Content: "carol's chat"}},
	}))
	if rec.Code != 200 {
		t.Fatalf("save: %d", rec.Code)
	}

	// find a superuser
	su, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, "test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	suTok, err := su.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, cookieReq(t, suTok, http.MethodGet, "/ai/conversations", nil))
	if rec.Code != 200 {
		t.Fatalf("superuser list: %d", rec.Code)
	}
	var list struct {
		Conversations []agentConvSummaries `json:"conversations"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Conversations) != 1 {
		t.Fatalf("superuser should see carol's conversation, got %d: %s", len(list.Conversations), rec.Body.String())
	}
	if list.Conversations[0].User == "" {
		t.Errorf("superuser list should include the owner id")
	}
}