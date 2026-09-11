package main

import (
	"fmt"
	"log"

	"github.com/pocketbase/pocketbase/core"
)

// seedField describes a single field definition for ensureDefaultCollections.
type seedField struct {
	name string
	typ  string
}

// seedCollection describes a collection plus its fields (in creation order).
type seedCollection struct {
	name        string
	fields      []seedField
	addAutodate bool // append system created/updated autodate fields
}

// defaultCollections lists the PBX system collections plus the sample "example"
// collection. They are created on first run so every PBX feature works before
// any manual schema setup.
var defaultCollections = []seedCollection{
	{
		name: "_app",
		fields: []seedField{
			{"group", core.FieldTypeText},
			{"group_label", core.FieldTypeText},
			{"group_icon", core.FieldTypeFile},
			{"collection", core.FieldTypeText},
			{"configName", core.FieldTypeText},
			{"collectionLabel", core.FieldTypeText},
		},
	},
	{
		name: "_views",
		fields: []seedField{
			{"_name", core.FieldTypeText},
			{"_collName", core.FieldTypeText},
			{"_tabulator", core.FieldTypeJSON},
			{"_form", core.FieldTypeJSON},
			{"_mssql", core.FieldTypeJSON},
		},
	},
	{
		name: "_actions",
		fields: []seedField{
			{"_name", core.FieldTypeText},
			{"_description", core.FieldTypeText},
			{"_script", core.FieldTypeEditor},
			{"_collection", core.FieldTypeText},
			{"_onList", core.FieldTypeBool},
			{"_onForm", core.FieldTypeBool},
			{"_public", core.FieldTypeBool},
		},
	},
	{
		name: "_filters",
		fields: []seedField{
			{"_name", core.FieldTypeText},
			{"_coll", core.FieldTypeText},
			{"_config", core.FieldTypeText},
			{"_user", core.FieldTypeText},
			{"_def", core.FieldTypeJSON},
		},
		addAutodate: true,
	},
	{
		name: "_conversations",
		fields: []seedField{
			{"_title", core.FieldTypeText},
			{"_messages", core.FieldTypeJSON},
			{"_user", core.FieldTypeText},
			{"_model", core.FieldTypeText},
		},
		addAutodate: true,
	},
	{
		name: "_agent",
		fields: []seedField{
			{"_name", core.FieldTypeText},
			{"_config", core.FieldTypeJSON},
		},
	},
	{
		name: "example",
		fields: []seedField{
			{"name", core.FieldTypeText},
			{"address", core.FieldTypeText},
			{"age", core.FieldTypeNumber},
			{"is_customer", core.FieldTypeBool},
		},
	},
}

// ensureDefaultCollections creates all default collections (and the sample data)
// on first run. It is idempotent: existing collections are left untouched, and
// the sample configuration/data records are only inserted when the "example"
// collection is newly created.
func ensureDefaultCollections(app core.App) error {
	firstRun := false

	for _, def := range defaultCollections {
		if existing, _ := app.FindCachedCollectionByNameOrId(def.name); existing != nil {
			continue
		}

		coll := core.NewBaseCollection(def.name)
		for _, f := range def.fields {
			cf := core.Fields[f.typ]()
			cf.SetName(f.name)
			coll.Fields.Add(cf)
		}
		if def.addAutodate {
			if err := addAutodateFields(coll); err != nil {
				return fmt.Errorf("seed %q autodate fields: %w", def.name, err)
			}
		}

		if err := app.Save(coll); err != nil {
			return fmt.Errorf("create collection %q: %w", def.name, err)
		}

		log.Printf("pbx: created collection %q", def.name)
		if def.name == "example" {
			firstRun = true
		}
	}

	if firstRun {
		if err := seedSampleData(app); err != nil {
			return err
		}
	}

	return nil
}

// addAutodateFields appends the "created" (set on create) and "updated" (set on
// create and update) autodate fields. They are regular record fields required by
// the -created / -updated sorts used in list queries.
func addAutodateFields(coll *core.Collection) error {
	for _, name := range []string{"created", "updated"} {
		af, ok := core.Fields[core.FieldTypeAutodate]().(*core.AutodateField)
		if !ok {
			return fmt.Errorf("autodate field type mismatch")
		}
		af.SetName(name)
		af.OnCreate = true
		af.OnUpdate = name == "updated"
		coll.Fields.Add(af)
	}
	return nil
}

// seedSampleData inserts the sample configuration records (view config,
// dashboard entry, agent config, sample action, saved filter) and a few rows in
// the sample "example" collection. Runs only on a truly empty first-run DB, so
// the inserts cannot collide with user-created records.
func seedSampleData(app core.App) error {
	configRecords := []struct {
		collection string
		values     map[string]any
	}{
		{
			"_views",
			map[string]any{
				"_name":     "example",
				"_collName": "example",
				"_tabulator": `{"pageTitle":"Example","collectionDescr":"Sample data collection created on first run","columnOrder":"1,2,3,4","columnSorting":true,"searchBox":true,"pagination":true,"displaySystemCol":false,"filter":"","columns":[{"field":"name","title":"Name","sortable":true,"searchable":true},{"field":"address","title":"Address","sortable":false,"searchable":true},{"field":"age","title":"Age","sortable":true,"searchable":false},{"field":"is_customer","title":"Is Customer","sortable":true,"searchable":true}]}`,
"_form": `{"formTitle":"Example","formDescr":"Sample data form created on first run","formLabels":"name=Name,address=Address,age=Age,is_customer=Is Customer","formLayout":"row:(1,2) (3,4)","columnOrder":"1,2,3,4","displaySystemCol":false}`,
			},
		},
		{
			"_app",
			map[string]any{
				"group":           "Data",
				"group_label":     "Sample Data",
				"group_icon":      "",
				"collection":      "example",
				"configName":      "example",
				"collectionLabel": "Example",
			},
		},
		{
			"_agent",
			map[string]any{
				"_name":   agentConfigName,
				"_config": "{}",
			},
		},
		{
			"_actions",
			map[string]any{
				"_name":        "Example count",
				"_description": "Counts the records in the example collection",
				"_script":      `var rows = select("example", "", "", 0); log("Selected " + rows.length + " example record(s)");`,
				"_collection":  "example",
				"_onList":      true,
				"_onForm":      false,
				"_public":      false,
			},
		},
		{
			"_filters",
			map[string]any{
				"_name":   "Customers",
				"_coll":   "example",
				"_config": "example",
				"_user":   "",
				"_def":    `{"name":"Customers","conditions":[{"field":"is_customer","op":"=","value":"true"}],"chains":[]}`,
			},
		},
	}
	for _, c := range configRecords {
		if err := insertCollectionRecord(app, c.collection, c.values); err != nil {
			return err
		}
	}

	exampleRows := []map[string]any{
		{"name": "John Doe", "address": "123 Main St", "age": 30, "is_customer": true},
		{"name": "Jane Smith", "address": "456 Oak Ave", "age": 25, "is_customer": false},
		{"name": "Bob Johnson", "address": "789 Pine Rd", "age": 45, "is_customer": true},
	}
	for _, row := range exampleRows {
		if err := insertCollectionRecord(app, "example", row); err != nil {
			return err
		}
	}

	return nil
}

// insertCollectionRecord builds and saves a new record in the given collection.
func insertCollectionRecord(app core.App, collection string, values map[string]any) error {
	coll, err := app.FindCachedCollectionByNameOrId(collection)
	if err != nil {
		return fmt.Errorf("seed %q: %w", collection, err)
	}
	rec := core.NewRecord(coll)
	for k, v := range values {
		rec.Set(k, v)
	}
	if err := app.Save(rec); err != nil {
		return fmt.Errorf("seed record in %q: %w", collection, err)
	}
	return nil
}