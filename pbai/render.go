package pbai

// Server-side rendering for AI agent chat responses. A ChatResult is turned
// into a single sanitized HTML fragment that the chat page injects into the
// conversation bubble (instead of the old client-side mini-markdown/table
// pipeline). Response kinds:
//
//   - no records       → markdown only (goldmark + bluemonday)
//   - one record       → markdown lead-in (if any) + read-only detail card
//   - many records     → markdown lead-in (if any) + tabular-style table
//   - pending action   → markdown summary (confirm modal is unchanged)
//
// All text is escaped with html/template auto-escaping (tables/cards) or
// sanitized with bluemonday (markdown), so model output can never inject
// markup into the page.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	xhtml "golang.org/x/net/html"
	goldmark "github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// --- markdown ---

var mdRenderer = goldmark.New(goldmark.WithExtensions(extension.GFM))

// mdPolicy sanitizes goldmark output. UGCPolicy keeps tables/lists/links;
// AllowStandardURLs keeps http/https/mailto hrefs usable; images are stripped
// so a prompt-injected record value can never make the browser fetch an
// external resource (tracking beacon) on render.
func mdPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowStandardURLs()
	p.RequireNoFollowOnLinks(true)
	p.RequireNoReferrerOnLinks(true)
	return p
}

var mdSanitizer = mdPolicy()

// mdHTML renders markdown to a sanitized HTML fragment.
func mdHTML(src string) template.HTML {
	var b bytes.Buffer
	if err := mdRenderer.Convert([]byte(src), &b); err != nil {
		return template.HTML(html.EscapeString(src))
	}
	return template.HTML(stripImages(mdSanitizer.Sanitize(b.String())))
}

// stripImages drops <img> elements from sanitized HTML using the html tokenizer.
func stripImages(s string) string {
	if !strings.Contains(s, "<img") {
		return s
	}
	doc, err := xhtml.Parse(strings.NewReader(s))
	if err != nil {
		return s
	}
	var b strings.Builder
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == "img" {
			return
		}
		if n.Type == xhtml.TextNode {
			b.WriteString(n.Data)
			return
		}
		if n.Type == xhtml.ElementNode {
			b.WriteString("<")
			b.WriteString(n.Data)
			for _, a := range n.Attr {
				b.WriteString(" ")
				b.WriteString(a.Key)
				b.WriteString(`="`)
				b.WriteString(html.EscapeString(a.Val))
				b.WriteString(`"`)
			}
			b.WriteString(">")
			if strings.EqualFold(n.Data, "br") {
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == xhtml.ElementNode {
			b.WriteString("</")
			b.WriteString(n.Data)
			b.WriteString(">")
		}
	}
	walk(doc)
	return b.String()
}

// --- templates ---

type tableRow struct{ Cells []string }

type tableData struct {
	Columns []string
	Rows    []tableRow
}

var tableTpl = template.Must(template.New("table").Parse(
	`<div class="ai-tbl"><table class="ai-table"><thead><tr>{{range .Columns}}<th>{{.}}</th>{{end}}</tr></thead><tbody>{{range .Rows}}<tr>{{range .Cells}}<td>{{.}}</td>{{end}}</tr>{{end}}</tbody></table></div>`,
))

type detailField struct {
	Label string
	Value string
}

type detailData struct {
	Title  string
	Fields []detailField
}

var detailTpl = template.Must(template.New("detail").Parse(
	`<div class="ai-detail">{{if .Title}}<div class="ai-detail-title">{{.Title}}</div>{{end}}<dl>{{range .Fields}}<dt>{{.Label}}</dt><dd>{{.Value}}</dd>{{end}}</dl></div>`,
))

type renderData struct {
	Lead template.HTML // sanitized markdown HTML (the explanation above cards/tables)
	Body template.HTML // sanitized table/detail HTML
}

var resultTpl = template.Must(template.New("result").Parse(
	`{{if .Lead}}<div class="ai-md">{{.Lead}}</div>{{end}}{{.Body}}`,
))

// --- value formatting ---

// formatCell renders a record value for a table cell or detail card. The
// result is later auto-escaped by html/template.
func formatCell(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case time.Time:
		return t.String()
	default:
		if b, err := json.Marshal(t); err == nil {
			return string(b)
		}
		return fmt.Sprint(t)
	}
}

// --- tables & detail cards ---

// viewConfig renders nothing directly; it returns the display title and a
// field→label map for the collection a record belongs to, derived from the
// _views configs (pageTitle + _form formLabels/labels).
func viewConfig(app core.App, collName string) (title string, labels map[string]string, ok bool) {
	if app == nil || collName == "" {
		return "", nil, false
	}
	recs, err := app.FindRecordsByFilter("_views", "_collName = {:c}", "", 1, 0, dbx.Params{"c": collName})
	if err != nil || len(recs) == 0 {
		return "", nil, false
	}
	rec := recs[0]
	labels = map[string]string{}

	var tab struct {
		PageTitle string `json:"pageTitle"`
	}
	if j := rec.GetString("_tabulator"); j != "" {
		_ = json.Unmarshal([]byte(j), &tab)
	}
	title = tab.PageTitle

	var form struct {
		FormLabels string            `json:"formLabels"`
		Labels     map[string]string `json:"labels"`
	}
	if j := rec.GetString("_form"); j != "" {
		_ = json.Unmarshal([]byte(j), &form)
	}
	for _, pair := range strings.Split(form.FormLabels, ",") {
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) == 2 && kv[0] != "" {
			labels[kv[0]] = kv[1]
		}
	}
	for k, v := range form.Labels {
		labels[k] = v
	}
	return title, labels, len(labels) > 0 || title != ""
}

// displayColumns derives the column order for a records dataset: id first,
// system collection fields dropped, remaining fields sorted by name (the
// same rule the client-side addRecordsTable used).
func displayColumns(records []map[string]any) []string {
	var cols []string
	seen := map[string]bool{}
	for _, rec := range records {
		for k := range rec {
			if seen[k] {
				continue
			}
			switch k {
			case "collectionId", "collectionName", "id":
				// handled below
			}
			seen[k] = true
			if k != "collectionId" && k != "collectionName" {
				cols = append(cols, k)
			}
		}
	}
	sort.Slice(cols, func(i, j int) bool {
		if cols[i] == "id" {
			return true
		}
		if cols[j] == "id" {
			return false
		}
		return cols[i] < cols[j]
	})
	return cols
}

func renderTable(records []map[string]any) string {
	cols := displayColumns(records)
	if len(cols) == 0 {
		return ""
	}
	data := tableData{Columns: cols}
	for _, rec := range records {
		row := tableRow{}
		for _, c := range cols {
			row.Cells = append(row.Cells, formatCell(rec[c]))
		}
		data.Rows = append(data.Rows, row)
	}
	var buf bytes.Buffer
	if err := tableTpl.Execute(&buf, data); err != nil {
		return ""
	}
	return buf.String()
}

func renderDetail(rec map[string]any, title string, labels map[string]string) string {
	data := detailData{Title: title}
	sys := []string{"id", "created", "updated"}
	for _, c := range sys {
		if _, ok := rec[c]; ok {
			label := map[string]string{"id": "ID", "created": "Created", "updated": "Updated"}[c]
			data.Fields = append(data.Fields, detailField{Label: label, Value: formatCell(rec[c])})
		}
	}
	for _, c := range displayColumns([]map[string]any{rec}) {
		if c == "id" {
			continue
		}
		label := labels[c]
		if label == "" {
			label = c
		}
		data.Fields = append(data.Fields, detailField{Label: label, Value: formatCell(rec[c])})
	}
	if len(data.Fields) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := detailTpl.Execute(&buf, data); err != nil {
		return ""
	}
	return buf.String()
}

// --- composition ---

// RenderResult turns a ChatResult into the HTML fragment shown in the chat.
func RenderResult(app core.App, res *ChatResult) string {
	lead := strings.TrimSpace(res.FinalText)
	data := renderData{}
	if lead != "" {
		data.Lead = mdHTML(lead)
	}

	switch {
	case res.PendingAction != nil:
		// summary only; the confirm modal handles the actual approve flow
	case len(res.Records) == 1:
		coll := ""
		if c, ok := res.Records[0]["collectionName"].(string); ok {
			coll = c
		}
		title, labels, ok := viewConfig(app, coll)
		if !ok {
			title = coll
		}
		data.Body = template.HTML(renderDetail(res.Records[0], title, labels))
	case len(res.Records) > 1:
		data.Body = template.HTML(renderTable(res.Records))
	}

	var buf bytes.Buffer
	if err := resultTpl.Execute(&buf, data); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}