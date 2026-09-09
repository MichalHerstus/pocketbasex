package pbai

import (
	"strings"
	"testing"
)

func TestMdHTMLBasic(t *testing.T) {
	out := string(mdHTML("**bold** and `code` and a bullet:\n\n- one\n- two"))
	for _, want := range []string{"<strong>bold</strong>", "<code>code</code>", "<li>one</li>", "<li>two</li>"} {
		if !strings.Contains(out, want) {
			t.Errorf("md output missing %q:\n%s", want, out)
		}
	}
}

func TestMdHTMLStripsScript(t *testing.T) {
	out := string(mdHTML("**safe**\n\n<script>alert(1)</script>"))
	if strings.Contains(out, "<script") || strings.Contains(out, "alert(1)") {
		t.Errorf("script survived sanitization: %s", out)
	}
	if !strings.Contains(out, "<strong>safe</strong>") {
		t.Errorf("safe markdown lost after sanitization: %s", out)
	}
}

func TestMdHTMLStripsImages(t *testing.T) {
	// an image would fire an external request when rendered; it must be dropped
	out := string(mdHTML("![x](http://evil.test/beacon.png) fine"))
	if strings.Contains(out, "<img") || strings.Contains(out, "http://evil.test") {
		t.Errorf("image/URL survived sanitization: %s", out)
	}
	if !strings.Contains(out, "fine") {
		t.Errorf("text lost after image strip: %s", out)
	}
}

func TestMdHTMLUnsafeLinkAttribute(t *testing.T) {
	out := string(mdHTML(`[x](javascript:alert(1))`))
	if strings.Contains(out, "javascript:") {
		t.Errorf("javascript scheme survived: %s", out)
	}
}

func TestRenderTableColumnsAndEscaping(t *testing.T) {
	records := []map[string]any{
		{
			"collectionId":   "cid",
			"collectionName": "products",
			"id":             "r1",
			"title":          "A <b>& B",
			"qty":            3,
			"tags":           []any{"a", "b"},
		},
	}
	html := renderTable(records, "", "")
	// system collection fields are never rendered as columns
	if strings.Contains(html, "collectionName") || strings.Contains(html, "collectionId") {
		t.Errorf("system columns leaked: %s", html)
	}
	// id column first, then sorted
	iID := strings.Index(html, ">id</th>")
	iTitle := strings.Index(html, ">title</th>")
	if iID < 0 || iTitle < 0 || iID > iTitle {
		t.Errorf("column order wrong (id=%d title=%d): %s", iID, iTitle, html)
	}
	// HTML is escaped, not raw
	if !strings.Contains(html, "&lt;b&gt;&amp; B") {
		t.Errorf("cell value not escaped: %s", html)
	}
	if !strings.Contains(html, `&#34;a&#34;`) {
		t.Errorf("object cell not JSON-encoded: %s", html)
	}
}

func TestRenderDetailLabelsAndTitle(t *testing.T) {
	labels := map[string]string{"title": "Název", "qty": "Počet"}
	rec := map[string]any{
		"id":             "r1",
		"created":        "2026-01-01 10:00:00.000Z",
		"updated":        "2026-01-01 10:00:00.000Z",
		"collectionName": "products",
		"title":          "Widget",
		"qty":            3,
	}
	html := renderDetail(rec, "Produkty", labels, "", "produkty", false)
	if !strings.Contains(html, "Produkty") {
		t.Errorf("title missing: %s", html)
	}
	for _, want := range []string{"Název", "Počet", "Widget", "3", "ID", "Created", "Updated"} {
		if !strings.Contains(html, want) {
			t.Errorf("detail missing %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "collectionName") {
		t.Errorf("collectionName leaked into detail: %s", html)
	}
}

func TestRenderDetailEscapes(t *testing.T) {
	rec := map[string]any{"id": "r1", "collectionName": "x", "note": "<script>x</script>"}
	html := renderDetail(rec, "", nil, "", "", false)
	if strings.Contains(html, "<script>") {
		t.Errorf("detail value not escaped: %s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Errorf("escaped form missing: %s", html)
	}
}

func TestRenderResultTableComposition(t *testing.T) {
	res := &ChatResult{
		FinalText: "Here:",
		Records: []map[string]any{
			{"collectionName": "p", "id": "1", "title": "A"},
			{"collectionName": "p", "id": "2", "title": "B"},
		},
	}
	html := RenderResult(nil, res, "")
	if !strings.Contains(html, "ai-md") || !strings.Contains(html, "Here") {
		t.Errorf("lead-in missing: %s", html)
	}
	if !strings.Contains(html, "ai-table") {
		t.Errorf("table missing: %s", html)
	}
}

func TestRenderResultDetailComposition(t *testing.T) {
	res := &ChatResult{
		FinalText: "Found:",
		Records: []map[string]any{
			{"collectionName": "p", "id": "1", "title": "A"},
		},
	}
	html := RenderResult(nil, res, "")
	if !strings.Contains(html, "ai-md") || !strings.Contains(html, "Found") {
		t.Errorf("lead-in missing: %s", html)
	}
	if !strings.Contains(html, "ai-detail") {
		t.Errorf("detail card missing: %s", html)
	}
}

func TestRenderResultMarkdownOnly(t *testing.T) {
	res := &ChatResult{FinalText: "Just text with **bold**."}
	html := RenderResult(nil, res, "")
	if strings.Contains(html, "ai-table") || strings.Contains(html, "ai-detail") {
		t.Errorf("unexpected record cards: %s", html)
	}
	if !strings.Contains(html, "<strong>bold</strong>") {
		t.Errorf("markdown not rendered: %s", html)
	}
}

func TestRenderResultPending(t *testing.T) {
	res := &ChatResult{
		PendingAction: &PendingAction{Type: "insert_records", Summary: "Insert 2 records"},
		FinalText:     "Awaiting confirmation: Insert 2 records",
	}
	html := RenderResult(nil, res, "")
	if !strings.Contains(html, "Awaiting confirmation") {
		t.Errorf("pending summary missing: %s", html)
	}
	if strings.Contains(html, "ai-table") || strings.Contains(html, "ai-detail") {
		t.Errorf("unexpected cards in pending render: %s", html)
	}
}

func TestRenderTableRecordLinks(t *testing.T) {
	records := []map[string]any{
		{"collectionName": "products", "id": "abc123", "title": "Widget"},
		{"collectionName": "products", "id": "def456", "title": "Gadget"},
	}
	html := renderTable(records, "", "produkty")
	if !strings.Contains(html, `href="/form/produkty/abc123"`) {
		t.Errorf("record link missing for id column: %s", html)
	}
	if !strings.Contains(html, `href="/form/produkty/def456"`) {
		t.Errorf("record link missing for second id: %s", html)
	}
	if strings.Contains(html, `href="/form/produkty/title"`) {
		t.Errorf("non-id column got a link: %s", html)
	}
}

func TestRenderTableBasePathPrefix(t *testing.T) {
	records := []map[string]any{
		{"collectionName": "products", "id": "abc123", "title": "Widget"},
	}
	html := renderTable(records, "/mobile", "produkty")
	if !strings.Contains(html, `href="/mobile/form/produkty/abc123"`) {
		t.Errorf("basePath prefix missing: %s", html)
	}
}

func TestRenderDetailEditAndDelete(t *testing.T) {
	labels := map[string]string{"title": "Název"}
	rec := map[string]any{
		"collectionName": "products",
		"id":             "abc123",
		"title":          "Widget",
	}
	html := renderDetail(rec, "Produkty", labels, "", "produkty", true)
	if !strings.Contains(html, `href="/form/produkty/abc123"`) {
		t.Errorf("edit link missing: %s", html)
	}
	if !strings.Contains(html, `data-coll="products"`) || !strings.Contains(html, `data-id="abc123"`) {
		t.Errorf("delete button data missing: %s", html)
	}
	// non-superuser render omits the delete button
	noDel := renderDetail(rec, "Produkty", labels, "", "produkty", false)
	if strings.Contains(noDel, "del-rec-btn") {
		t.Errorf("delete button should be hidden for non-superusers: %s", noDel)
	}
}
func TestRenderResultExportChip(t *testing.T) {
	res := &ChatResult{
		FinalText: "done",
		Export: &ExportSuggestion{
			ID:       "abc123XYZ",
			Filename: `export <b>&".csv`,
			Format:   "csv",
			Count:    12,
		},
	}
	out := RenderResult(nil, res, "")
	for _, want := range []string{"/api/ai/exports/abc123XYZ", `Download export`, "&lt;b&gt;&amp;", "12 rows"} {
		if !strings.Contains(out, want) {
			t.Errorf("export chip missing %q in output:\n%s", want, out)
		}
	}
	if strings.Contains(out, `<b>&"`) {
		t.Errorf("raw filename leaked into output:\n%s", out)
	}
}
