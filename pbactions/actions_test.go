package pbactions

import (
	"context"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func setupTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	coll := core.NewBaseCollection("products")
	f1 := core.Fields[core.FieldTypeText]()
	f1.SetName("name")
	f2 := core.Fields[core.FieldTypeNumber]()
	f2.SetName("price")
	coll.Fields.Add(f1, f2)
	// allow any caller (anonymous) for the builtin rule checks
	pub := ""
	coll.ListRule = &pub
	coll.ViewRule = &pub
	coll.CreateRule = &pub
	coll.UpdateRule = &pub
	coll.DeleteRule = &pub
	if err := app.Save(coll); err != nil {
		t.Fatal(err)
	}
	return app
}

func seedRecord(t *testing.T, app *tests.TestApp, name string, price float64) *core.Record {
	t.Helper()
	coll, err := app.FindCollectionByNameOrId("products")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(coll)
	rec.Set("name", name)
	rec.Set("price", price)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestRunLogOutput(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	seedRecord(t, app, "Widget", 50)
	seedRecord(t, app, "Gadget", 120)

	runner := NewRunner(app, nil)
	res, err := runner.Run(context.Background(), &ActionDef{
		Collection: "products",
		Script:     `var rows = select("products", "", "", 0); log("count=" + rows.length);`,
	}, nil)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK, got error: %s", res.Error)
	}
	if len(res.Output) != 1 || res.Output[0] != "count=2" {
		t.Errorf("unexpected output: %#v", res.Output)
	}
}

func TestRunInsertAffected(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	runner := NewRunner(app, nil)
	res, err := runner.Run(context.Background(), &ActionDef{
		Collection: "products",
		Script:     `var id = insert("products", {name: "New", price: 9}); log("id=" + id);`,
	}, nil)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK, got error: %s", res.Error)
	}
	if res.Affected != 1 {
		t.Errorf("expected 1 affected, got %d", res.Affected)
	}
	if total, _ := app.CountRecords("products"); total != 1 {
		t.Errorf("expected 1 product in DB, got %d", total)
	}
}

func TestRunScriptError(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	seedRecord(t, app, "Widget", 50)

	runner := NewRunner(app, nil)
	res, err := runner.Run(context.Background(), &ActionDef{
		Collection: "products",
		Script:     `var x = undefinedValue + 1;`,
	}, nil)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.OK {
		t.Fatalf("expected script error to be reported")
	}
	if res.Error == "" {
		t.Errorf("expected an error message")
	}
}

func TestRunTimeout(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	original := timeLimit
	timeLimit = 100 * time.Millisecond
	defer func() { timeLimit = original }()

	runner := NewRunner(app, nil)
	_, err := runner.Run(context.Background(), &ActionDef{
		Collection: "products",
		Script:     `while (true) {}`,
	}, nil)
	if err != ErrTimeout {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
}
