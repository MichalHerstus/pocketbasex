package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestTabularProduktyRealConfig(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	// produkty base collection mirroring the real schema
	coll := core.NewBaseCollection("produkty")
	for _, f := range []struct {
		name string
		typ  string
	}{
		{"productCode", core.FieldTypeText},
		{"prodShortName", core.FieldTypeText},
		{"prodLogName", core.FieldTypeText},
		{"prodDescription", core.FieldTypeEditor},
		{"prodPrice", core.FieldTypeNumber},
		{"active", core.FieldTypeBool},
		{"kategorie", core.FieldTypeText},
	} {
		cf := core.Fields[f.typ]()
		cf.SetName(f.name)
		coll.Fields.Add(cf)
	}
	if err := app.Save(coll); err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(coll)
	r.Set("productCode", "P1")
	r.Set("prodPrice", 10.5)
	r.Set("active", true)
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		r := core.NewRecord(coll)
		r.Set("productCode", "P"+string(rune('A'+i)))
		r.Set("prodPrice", float64(i))
		if err := app.Save(r); err != nil {
			t.Fatal(err)
		}
	}

	// _views collection + config record with the EXACT real _tabulator incl. the
	// client-side "?" filter that must NOT be pushed into the DB query
	vc := core.NewBaseCollection("_views")
	for _, f := range []struct {
		name string
		typ  string
	}{
		{"_name", core.FieldTypeText},
		{"_collName", core.FieldTypeText},
		{"_tabulator", core.FieldTypeJSON},
		{"_form", core.FieldTypeJSON},
	} {
		cf := core.Fields[f.typ]()
		cf.SetName(f.name)
		vc.Fields.Add(cf)
	}
	if err := app.Save(vc); err != nil {
		t.Fatal(err)
	}
	vrec := core.NewRecord(vc)
	vrec.Set("_name", "produkty")
	vrec.Set("_collName", "produkty")
	vrec.Set("_tabulator", `{"pageTitle":"Seznam produktů","collectionDescr":"databáze produktů naší firmy","columnTitles":"Kód, Název, Cena, Aktivní","columnOrder":"2,3,6,7,8,9","columnSorting":true,"searchBox":true,"pagination":true,"displaySystemCol":false,"filter":"(prodPrice > ?) && (prodShortName ~ ?)"}`)
	if err := app.Save(vrec); err != nil {
		t.Fatal(err)
	}

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	se := &core.ServeEvent{Router: router, Server: &http.Server{Addr: "127.0.0.1:0"}}
	registerDataRoutes(se)
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/tabular/produkty", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		body, _ := io.ReadAll(rec.Body)
		t.Fatalf("status=%d body=%s", rec.Code, string(body))
	}

	// page 2 must also work
	req2 := httptest.NewRequest(http.MethodGet, "/tabular/produkty?page=2", nil)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		body, _ := io.ReadAll(rec2.Body)
		t.Fatalf("page2 status=%d body=%s", rec2.Code, string(body))
	}
}