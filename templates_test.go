package main

import (
	"io"
	"testing"

	"pbx/views"
)

func TestTemplatesParse(t *testing.T) {
	if templates == nil {
		t.Fatal("templates not initialized")
	}
	for _, name := range []string{"login.html", "app.html", "pbxsetup.html", "setup-record.html", "tabulator.html", "form.html", "pbxconfig.html", "config.html", "import-wizard.html", "agent.html"} {
		if templates.Lookup(name) == nil {
			t.Errorf("template %q missing", name)
		}
	}
}

// TestTemplatesRenderLocalized renders every user-facing template with a
// sample Czech locale to catch execution-time errors (e.g. a missing .Lang or
// a nil config pointer referenced by the template). Templates are already
// parsed in init(), so this exercises the execute path with realistic data.
func TestTemplatesRenderLocalized(t *testing.T) {
	lang := func() views.LangData { return views.LangData{Lang: "cs"} }
	run := func(name string, data any) {
		t.Helper()
		if err := templates.ExecuteTemplate(io.Discard, name, data); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}

	run("login.html", map[string]string{"Theme": "light", "Lang": "cs", "Error": ""})
	run("app.html", views.AppPageData{LangData: lang(), Theme: "light", BasePath: "", Name: "x"})
	run("tabulator.html", &views.TabulatorPageData{
		LangData:         lang(),
		Theme:            "light",
		CollectionName:   "produkty",
		TotalRecords:     2,
		Config:           views.TabulatorConfig{PageTitle: "Produkty", SearchBox: true, Pagination: true},
		FieldsJSON:       "[]",
		FieldTypesJSON:   "[]",
		HeadersJSON:      "[]",
		RecordsJSON:      "[]",
		FieldOptionsJSON: "{}",
	})
	// tabulator without MSSQL config must render (no nil-pointer on .Mssql)
	run("tabulator.html", &views.TabulatorPageData{
		LangData:         lang(),
		Theme:            "light",
		CollectionName:   "bez_mssql",
		Config:           views.TabulatorConfig{},
		FieldsJSON:       "[]",
		FieldTypesJSON:   "[]",
		HeadersJSON:      "[]",
		RecordsJSON:      "[]",
		FieldOptionsJSON: "{}",
	})
	run("form.html", views.FormPageData{
		LangData: lang(), Theme: "light", ConfigName: "c", CollectionName: "produkty",
		Title: "Produkty", ViewOnly: true,
	})
	run("pbxsetup.html", views.PbxSetupPageData{LangData: lang(), Theme: "light", Agent: views.AgentConfig{}})
	run("pbxconfig.html", views.PbxConfigPageData{LangData: lang(), Theme: "light"})
	run("config.html", views.ConfigEditorPageData{LangData: lang(), Theme: "light", CollName: "produkty"})
	run("import-wizard.html", views.ImportWizardPageData{LangData: lang(), Theme: "light", Source: "excel", Step: 1})
	run("agent.html", views.AgentPageData{LangData: lang(), Theme: "light", Config: views.AgentConfig{}})
	run("setup-record.html", views.SetupRecordPageData{LangData: lang(), Theme: "light", CollName: "_views", IsNew: true})
}
