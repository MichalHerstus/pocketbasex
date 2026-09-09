package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"pbx/pbai"
)

// newExportsTestApp wires the AI export download route onto a fresh test app
// and creates two distinct users. Returns alice and bob records.
func newExportsTestApp(t *testing.T) (*tests.TestApp, http.Handler, *core.Record, *core.Record) {
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
	router.GET("/api/ai/exports/{id}", func(e *core.RequestEvent) error { return handleAIExport(e) })
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	usersColl, err := app.FindCachedCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	makeUser := func(email string) *core.Record {
		ur := core.NewRecord(usersColl)
		ur.Set("email", email)
		ur.Set("password", "password123")
		if err := app.Save(ur); err != nil {
			t.Fatal(err)
		}
		return ur
	}
	bob := makeUser("bob@example.com")
	alice := makeUser("alice@example.com")
	return app, mux, alice, bob
}

func TestAIExportRequiresAuth(t *testing.T) {
	_, mux, _, _ := newExportsTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/api/ai/exports/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for unauthenticated export download, got %d", rec.Code)
	}
}

func TestAIExportUnknownIdNotFound(t *testing.T) {
	_, mux, alice, _ := newExportsTestApp(t)
	token, err := alice.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/ai/exports/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	req.AddCookie(&http.Cookie{Name: "pb_auth", Value: token})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown export id, got %d", rec.Code)
	}
}

func TestAIExportForeignUserDenied(t *testing.T) {
	_, mux, alice, bob := newExportsTestApp(t)

	exp := pbai.StoreExportForTest(alice.Id, "data.csv", "text/csv", []byte("a,b\n1,2\n"))
	if exp == nil {
		t.Fatal("StoreExportForTest failed")
	}

	bobToken, err := bob.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/ai/exports/"+exp.ID, nil)
	req.AddCookie(&http.Cookie{Name: "pb_auth", Value: bobToken})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for another user's export, got %d", rec.Code)
	}
}

func TestAIExportOwnsAndDownloads(t *testing.T) {
	_, mux, alice, _ := newExportsTestApp(t)
	token, err := alice.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	exp := pbai.StoreExportForTest(alice.Id, "plik.csv", "text/csv", []byte("a,b\n1,2\n"))
	if exp == nil {
		t.Fatal("StoreExportForTest failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/ai/exports/"+exp.ID, nil)
	req.AddCookie(&http.Cookie{Name: "pb_auth", Value: token})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for own export, got %d", rec.Code)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "plik.csv") {
		t.Fatalf("missing Content-Disposition filename: %q", cd)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/csv" {
		t.Fatalf("unexpected content type: %q", ct)
	}
	if body := rec.Body.String(); body != "a,b\n1,2\n" {
		t.Fatalf("unexpected body: %q", body)
	}
}