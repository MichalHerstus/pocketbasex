package main

import "testing"

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
