package main

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func newSeedsTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	return app
}

func fieldNames(coll *core.Collection) map[string]string {
	out := map[string]string{}
	for _, f := range coll.Fields {
		out[f.GetName()] = f.Type()
	}
	for _, name := range []string{"collectionId", "collectionName"} {
		delete(out, name)
	}
	return out
}

func TestEnsureDefaultCollectionsCreatesSchemas(t *testing.T) {
	app := newSeedsTestApp(t)

	if err := ensureDefaultCollections(app); err != nil {
		t.Fatalf("ensureDefaultCollections: %v", err)
	}

	want := map[string]map[string]string{
		"_app": {
			"group":           core.FieldTypeText,
			"group_label":     core.FieldTypeText,
			"group_icon":      core.FieldTypeFile,
			"collection":      core.FieldTypeText,
			"configName":      core.FieldTypeText,
			"collectionLabel": core.FieldTypeText,
		},
		"_views": {
			"_name":      core.FieldTypeText,
			"_collName":  core.FieldTypeText,
			"_tabulator": core.FieldTypeJSON,
			"_form":      core.FieldTypeJSON,
			"_mssql":     core.FieldTypeJSON,
		},
		"_actions": {
			"_name":        core.FieldTypeText,
			"_description": core.FieldTypeText,
			"_script":      core.FieldTypeEditor,
			"_collection":  core.FieldTypeText,
			"_onList":      core.FieldTypeBool,
			"_onForm":      core.FieldTypeBool,
			"_public":      core.FieldTypeBool,
		},
		"_filters": {
			"_name":   core.FieldTypeText,
			"_coll":   core.FieldTypeText,
			"_config": core.FieldTypeText,
			"_user":   core.FieldTypeText,
			"_def":    core.FieldTypeJSON,
			"created": core.FieldTypeAutodate,
			"updated": core.FieldTypeAutodate,
		},
		"_conversations": {
			"_title":    core.FieldTypeText,
			"_messages": core.FieldTypeJSON,
			"_user":     core.FieldTypeText,
			"_model":    core.FieldTypeText,
			"created":   core.FieldTypeAutodate,
			"updated":   core.FieldTypeAutodate,
		},
		"_agent": {
			"_name":   core.FieldTypeText,
			"_config": core.FieldTypeJSON,
		},
		"example": {
			"name":        core.FieldTypeText,
			"address":     core.FieldTypeText,
			"age":         core.FieldTypeNumber,
			"is_customer": core.FieldTypeBool,
		},
	}

	for collName, wantFields := range want {
		coll, err := app.FindCachedCollectionByNameOrId(collName)
		if err != nil {
			t.Fatalf("missing collection %q: %v", collName, err)
		}
		got := fieldNames(coll)
		for name, typ := range wantFields {
			if got[name] != typ {
				t.Errorf("collection %q: field %q = %q, want %q", collName, name, got[name], typ)
			}
		}
	}
}

func TestEnsureDefaultCollectionsSeedsSampleData(t *testing.T) {
	app := newSeedsTestApp(t)

	if err := ensureDefaultCollections(app); err != nil {
		t.Fatalf("ensureDefaultCollections: %v", err)
	}

	if total, _ := app.CountRecords("example"); total != 3 {
		t.Fatalf("expected 3 seeded example records, got %d", total)
	}

	viewRec, err := app.FindFirstRecordByFilter("_views", "_name = {:n}", map[string]any{"n": "example"})
	if err != nil {
		t.Fatalf("sample _views record missing: %v", err)
	}
	if viewRec.GetString("_collName") != "example" {
		t.Errorf("sample _views record targets %q, want example", viewRec.GetString("_collName"))
	}
	if viewRec.GetString("_tabulator") == "" {
		t.Errorf("sample _views record has no _tabulator config")
	}
	if viewRec.GetString("_form") == "" {
		t.Errorf("sample _views record has no _form config")
	}

	appRec, err := app.FindFirstRecordByFilter("_app", "collection = {:c}", map[string]any{"c": "example"})
	if err != nil {
		t.Fatalf("sample _app dashboard entry missing: %v", err)
	}
	if appRec.GetString("configName") != "example" {
		t.Errorf("sample _app entry configName = %q, want example", appRec.GetString("configName"))
	}

	if total, _ := app.CountRecords("_agent"); total != 1 {
		t.Errorf("expected 1 _agent record, got %d", total)
	}

	actions, _ := app.FindRecordsByFilter("_actions", "_collection = {:c}", "", 0, 0, nil, map[string]any{"c": "example"})
	if len(actions) != 1 {
		t.Errorf("expected 1 sample action for example, got %d", len(actions))
	}
}

func TestEnsureDefaultCollectionsIdempotent(t *testing.T) {
	app := newSeedsTestApp(t)

	if err := ensureDefaultCollections(app); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := ensureDefaultCollections(app); err != nil {
		t.Fatalf("second run: %v", err)
	}

	if total, _ := app.CountRecords("example"); total != 3 {
		t.Errorf("second run duplicated example records: got %d", total)
	}
	if total, _ := app.CountRecords("_agent"); total != 1 {
		t.Errorf("second run duplicated _agent records: got %d", total)
	}
	if total, _ := app.CountRecords("_views"); total != 1 {
		t.Errorf("second run duplicated _views records: got %d", total)
	}
	for _, collName := range []string{"_app", "_views", "_actions", "_filters", "_conversations", "_agent", "example"} {
		if _, err := app.FindCollectionByNameOrId(collName); err != nil {
			t.Errorf("collection %q lost after second run: %v", collName, err)
		}
	}
}