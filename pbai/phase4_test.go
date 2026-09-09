package pbai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"pbx/views"
)

// TestGetStatsCount runs a superuser-only count over a small collection and
// verifies the tool returns an aggregate line rather than row data.
func TestGetStatsCount(t *testing.T) {
	app, a, _ := newPhase4TestApp(t)
	defer app.Cleanup()

	out, err := getStatsTool().exec(a, json.RawMessage(`{"collection":"products","ops":["count"]}`))
	if err != nil {
		t.Fatalf("get_stats: %v", err)
	}
	if !strings.Contains(out, "count: 3") {
		t.Fatalf("expected count 3, got: %q", out)
	}
}

// TestGetStatsSum verifies aggregates over a numeric field.
func TestGetStatsSum(t *testing.T) {
	app, a, _ := newPhase4TestApp(t)
	defer app.Cleanup()

	out, err := getStatsTool().exec(a, json.RawMessage(`{"collection":"products","field":"price","ops":["sum","avg","min","max","distinct"]}`))
	if err != nil {
		t.Fatalf("get_stats: %v", err)
	}
	for _, want := range []string{"sum: 29.97", "avg: 9.99", "min: 4.99", "max: 14.99", "distinct: 3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output %q", want, out)
		}
	}
}

// TestGetStatsRejectsNonNumericAggregate ensures numeric ops reject a text field.
func TestGetStatsRejectsNonNumericAggregate(t *testing.T) {
	app, a, _ := newPhase4TestApp(t)
	defer app.Cleanup()

	if _, err := getStatsTool().exec(a, json.RawMessage(`{"collection":"products","field":"name","ops":["sum"]}`)); err == nil {
		t.Fatal("expected error for non-numeric sum")
	}
}

// TestQueryRelatedExpands verifies cross-collection expansion respects the
// related collection's view rule: a hidden record is not returned.
func TestQueryRelatedExpands(t *testing.T) {
	app, a, prodID := newPhase4TestApp(t)
	defer app.Cleanup()

	out, err := queryRelatedTool().exec(a, json.RawMessage(`{"collection":"products","ids":["`+prodID+`"],"expand":["category"]}`))
	if err != nil {
		t.Fatalf("query_related: %v", err)
	}
	if !strings.Contains(out, `"cat-a"`) {
		t.Fatalf("expected expanded category cat-a in output %q", out)
	}
	if strings.Contains(out, "secreta") {
		t.Fatalf("should not leak rule-hidden related record: %q", out)
	}
}

// TestCreateRecordsBatch inserts multiple records and reports their ids.
func TestCreateRecordsBatch(t *testing.T) {
	app, a, _ := newPhase4TestApp(t)
	defer app.Cleanup()

	out, err := createRecordsBatchTool().exec(a, json.RawMessage(`{"collection":"products","records":[{"name":"x1","price":1},{"name":"x2","price":2}]}`))
	if err != nil {
		t.Fatalf("create_records_batch: %v", err)
	}
	if !strings.Contains(out, "2") {
		t.Fatalf("expected engaged count in output %q", out)
	}
	total, err := app.CountRecords("products", nil)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Fatalf("expected 5 records after batch insert, got %d", total)
	}
}

// TestCreateRecordsBatchRejectsOverLimit ensures the 200-record ceiling.
func TestCreateRecordsBatchRejectsOverLimit(t *testing.T) {
	app, a, _ := newPhase4TestApp(t)
	defer app.Cleanup()

	var big []map[string]any
	for i := 0; i < 201; i++ {
		big = append(big, map[string]any{"name": "x", "price": 1})
	}
	b, _ := json.Marshal(map[string]any{"collection": "products", "records": big})
	if _, err := createRecordsBatchTool().exec(a, b); err == nil {
		t.Fatal("expected error for >200 batch")
	}
}

// TestRunActionExecutesPublicAction confirms a public action runs for a
// regular user and reports its output/affected count.
func TestRunActionExecutesPublicAction(t *testing.T) {
	app, a, _ := newPhase4TestApp(t)
	defer app.Cleanup()

	actColl, err := app.FindCollectionByNameOrId("_actions")
	if err != nil {
		t.Fatal(err)
	}
	act := core.NewRecord(actColl)
	act.Set("_name", "bump")
	act.Set("_script", `var n=0; for(var i=0;i<selectedRecords().length;i++){var r=selectedRecords()[i]; update("products",r.id,{price:r.price+1}); n++;} log("bumped "+n);`)
	act.Set("_collection", "products")
	act.Set("_public", true)
	act.Set("_onList", true)
	if err := app.Save(act); err != nil {
		t.Fatal(err)
	}

	out, err := runActionTool().exec(a, json.RawMessage(`{"actionId":"`+act.Id+`"}`))
	if err != nil {
		t.Fatalf("run_action: %v", err)
	}
	if !strings.Contains(out, "bumped 3") {
		t.Fatalf("expected bumped 3 in output %q", out)
	}
}

// TestExportDataJSON produces a JSON export with a download id.
func TestExportDataJSON(t *testing.T) {
	app, a, _ := newPhase4TestApp(t)
	defer app.Cleanup()

	out, err := exportDataTool().exec(a, json.RawMessage(`{"collection":"products","format":"json"}`))
	if err != nil {
		t.Fatalf("export_data: %v", err)
	}
	var res struct {
		Export *ExportSuggestion `json:"export"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil || res.Export == nil {
		t.Fatalf("expected export suggestion in %q (err=%v)", out, err)
	}
	if res.Export.ID == "" || res.Export.Count != 3 {
		t.Fatalf("unexpected export: %+v", res.Export)
	}
	ent := LoadExport(res.Export.ID)
	if ent == nil {
		t.Fatalf("stored export not found for id %q", res.Export.ID)
	}
	if !strings.Contains(string(ent.Data), "Widget") {
		t.Fatalf("export data should contain a product: %q", string(ent.Data))
	}
}

// TestExportDataCSV produces CSV with headers and rows.
func TestExportDataCSV(t *testing.T) {
	app, a, _ := newPhase4TestApp(t)
	defer app.Cleanup()

	out, err := exportDataTool().exec(a, json.RawMessage(`{"collection":"products","format":"csv","fields":["name","price"]}`))
	if err != nil {
		t.Fatalf("export_data csv: %v", err)
	}
	var res struct {
		Export *ExportSuggestion `json:"export"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	ent := LoadExport(res.Export.ID)
	if !strings.HasPrefix(string(ent.Data), "name,price\n") {
		t.Fatalf("unexpected csv head: %q", string(ent.Data))
	}
}

// newPhase4TestApp builds a two-collection test app: products (with a public
// relation category) and categories (one hidden by viewRule). Returns the app
// and a superuser Agent bound to it, plus the first product id.
func newPhase4TestApp(t *testing.T) (*tests.TestApp, *Agent, string) {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}

	cat := core.NewBaseCollection("categories")
	cn := core.Fields[core.FieldTypeText]()
	cn.SetName("name")
	cat.Fields.Add(cn)
	if err := app.Save(cat); err != nil {
		app.Cleanup()
		t.Fatal(err)
	}
	open := core.NewRecord(cat)
	open.Set("name", "cat-a")
	if err := app.Save(open); err != nil {
		app.Cleanup()
		t.Fatal(err)
	}
	secret := core.NewRecord(cat)
	secret.Set("name", "secreta")
	if err := app.Save(secret); err != nil {
		app.Cleanup()
		t.Fatal(err)
	}
	r := `id = '` + open.Id + `'`
	cat.ViewRule = &r
	if err := app.Save(cat); err != nil {
		app.Cleanup()
		t.Fatal(err)
	}

	prod := core.NewBaseCollection("products")
	pn := core.Fields[core.FieldTypeText]()
	pn.SetName("name")
	prod.Fields.Add(pn)
	num := core.Fields[core.FieldTypeNumber]()
	num.SetName("price")
	prod.Fields.Add(num)
	rel := core.Fields[core.FieldTypeRelation]().(*core.RelationField)
	rel.SetName("category")
	rel.CollectionId = cat.Id
	prod.Fields.Add(rel)
	if err := app.Save(prod); err != nil {
		app.Cleanup()
		t.Fatal(err)
	}
	var prodID string
	for _, rec := range []struct {
		name  string
		price float64
		cat   string
	}{
		{"Widget", 9.99, open.Id},
		{"Gadget", 14.99, open.Id},
		{"Thing", 4.99, ""},
	} {
		r := core.NewRecord(prod)
		r.Set("name", rec.name)
		r.Set("price", rec.price)
		if rec.cat != "" {
			r.Set("category", rec.cat)
		}
		if err := app.Save(r); err != nil {
			app.Cleanup()
			t.Fatal(err)
		}
		if r.Id != "" && prodID == "" {
			prodID = r.Id
		}
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
		app.Cleanup()
		t.Fatal(err)
	}

	su, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, "test@example.com")
	if err != nil {
		app.Cleanup()
		t.Fatal(err)
	}
	agent := NewAgent(app, &core.RequestInfo{Context: "ai", Auth: su}, views.AgentConfig{Provider: "mock", APIKey: "k", Model: "m"})
	agent.allowedTools = nil
	return app, agent, prodID
}
