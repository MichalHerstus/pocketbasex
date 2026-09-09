package pbai

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// newSuggestionsTestApp seeds one user collection "widgets" (3 records) plus
// an internal "_log" collection (must be excluded). A user with a listRule
// sees only the rule-listed widgets; the superuser sees all.
func newSuggestionsTestApp(t *testing.T) (*tests.TestApp, *core.RequestInfo) {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}

	w := core.NewBaseCollection("widgets")
	wf := core.Fields[core.FieldTypeText]()
	wf.SetName("label")
	w.Fields.Add(wf)
	if err := app.Save(w); err != nil {
		app.Cleanup()
		t.Fatal(err)
	}
	for _, lbl := range []string{"aa", "bb", "cc", "dd", "ee", "ff", "gg", "hh", "ii", "jj"} {
		r := core.NewRecord(w)
		r.Set("label", lbl)
		if err := app.Save(r); err != nil {
			app.Cleanup()
			t.Fatal(err)
		}
	}

	// internal collection must be skipped by SuggestCollections
	internal := core.NewBaseCollection("_log")
	if err := app.Save(internal); err != nil {
		app.Cleanup()
		t.Fatal(err)
	}

	// view collection must be skipped too
	v := core.NewViewCollection("widgets_view")
	v.ViewQuery = "select id from widgets"
	if err := app.Save(v); err != nil {
		app.Cleanup()
		t.Fatal(err)
	}

	su, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, "test@example.com")
	if err != nil {
		app.Cleanup()
		t.Fatal(err)
	}
	info := &core.RequestInfo{Context: "ai", Auth: su}
	return app, info
}

func TestSuggestCollectionsSuperuserTopFive(t *testing.T) {
	app, info := newSuggestionsTestApp(t)
	defer app.Cleanup()

	sug, err := SuggestCollections(app, info)
	if err != nil {
		t.Fatal(err)
	}
	if len(sug) > 5 {
		t.Fatalf("expected at most 5 suggestions, got %d: %+v", len(sug), sug)
	}
	// widgets has 3 records; it should be present with the right count
	found := false
	for _, s := range sug {
		if s.Name == "widgets" {
			found = true
			if s.Count != 10 {
				t.Fatalf("widgets count should be 10, got %d", s.Count)
			}
		}
	}
	if !found {
		t.Fatalf("widgets missing from suggestions: %+v", sug)
	}
}

func TestSuggestCollectionsRegularUserListRule(t *testing.T) {
	app, _ := newSuggestionsTestApp(t)
	defer app.Cleanup()

	usersColl, err := app.FindCachedCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	u := core.NewRecord(usersColl)
	u.Set("email", "bob@example.com")
	u.Set("password", "password123")
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}

	// rule limits widgets to those the user owns; bob owns only "aa"
	w, err := app.FindCollectionByNameOrId("widgets")
	if err != nil {
		t.Fatal(err)
	}
	rule := "label = 'aa'"
	w.ListRule = &rule
	if err := app.Save(w); err != nil {
		t.Fatal(err)
	}

	info := &core.RequestInfo{Context: "ai", Auth: u}
	sug, err := SuggestCollections(app, info)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range sug {
		if s.Name == "widgets" {
			found = true
			if s.Count != 1 {
				t.Fatalf("expected listRule-filtered count 1 for bob, got %d", s.Count)
			}
		}
	}
	if !found {
		t.Fatalf("widgets missing from bob's suggestions: %+v", sug)
	}
}

func TestSuggestCollectionsExcludesInternalAndViews(t *testing.T) {
	app, info := newSuggestionsTestApp(t)
	defer app.Cleanup()

	sug, err := SuggestCollections(app, info)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sug {
		if strings.HasPrefix(s.Name, "_") || strings.HasSuffix(s.Name, "_view") {
			t.Fatalf("internal/view collection leaked into suggestions: %+v", s)
		}
	}
}