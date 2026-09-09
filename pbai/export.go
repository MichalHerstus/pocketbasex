package pbai

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
)

// ExportSuggestion is surfaced to the client so the chat UI can render a
// download chip pointing at /api/ai/exports/{id}.
type ExportSuggestion struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Format   string `json:"format"` // "csv" | "json"
	Count    int    `json:"count"`
}

type exportEntry struct {
	ownerID     string
	filename    string
	contentType string
	data        []byte
	expiresAt   time.Time
}

var (
	exportStoreMu sync.RWMutex
	exportStore   = map[string]*exportEntry{}
)

const exportTTL = 10 * time.Minute

// storeExport keeps a serialized export accessible for exportTTL.
func storeExport(ownerID string, filename string, contentType string, data []byte) *ExportSuggestion {
	id := security.RandomString(32)
	exportStoreMu.Lock()
	exportStore[id] = &exportEntry{
		ownerID:     ownerID,
		filename:    filename,
		contentType: contentType,
		data:        data,
		expiresAt:   time.Now().Add(exportTTL),
	}
	exportStoreMu.Unlock()
	return &ExportSuggestion{ID: id, Filename: filename, Format: map[string]string{"text/csv": "csv", "application/json": "json"}[contentType], Count: 0}
}

// StoreExportForTest seeds the download store directly. It exists so tests in
// the main package can exercise the /api/ai/exports handler without going
// through an LLM round-trip.
func StoreExportForTest(ownerID string, filename string, contentType string, data []byte) *ExportSuggestion {
	return storeExport(ownerID, filename, contentType, data)
}

// loadExport returns the export entry if present and not expired.
func loadExport(id string) *exportEntry {
	exportStoreMu.Lock()
	defer exportStoreMu.Unlock()
	ent, ok := exportStore[id]
	if !ok {
		return nil
	}
	if time.Now().After(ent.expiresAt) {
		delete(exportStore, id)
		return nil
	}
	return ent
}

// ExportEntry mirrors a stored export for consumers outside the pbai package
// (e.g. the HTTP download handler in main.go).
type ExportEntry struct {
	OwnerID     string
	Filename    string
	ContentType string
	Data        []byte
}

// LoadExport returns a copy of the export with the given id if it exists and
// is not expired. It is exported so the main package can serve downloads.
func LoadExport(id string) *ExportEntry {
	ent := loadExport(id)
	if ent == nil {
		return nil
	}
	return &ExportEntry{
		OwnerID:     ent.ownerID,
		Filename:    ent.filename,
		ContentType: ent.contentType,
		Data:        ent.data,
	}
}

// exportDataTool produces a CSV or JSON export of the records the caller can
// read and hands back a short-lived download link.
func exportDataTool() tool {
	return tool{
		name: "export_data",
		description: "Exports records from a collection as CSV or JSON and returns a download link valid for 10 minutes. Args: {\"collection\": \"name\", \"format\": \"csv\" | \"json\" (default json), \"filter\": \"optional PB filter expression\", \"fields\": [\"optional field names to include\"], \"limit\": \"records to export (default 100, max 1000)\"}. Respects the collection list rule. Use when the user wants a file download.",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"collection": map[string]any{"type": "string"},
				"format":     map[string]any{"type": "string"},
				"filter":     map[string]any{"type": "string"},
				"fields":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"limit":      map[string]any{"type": "integer"},
			},
			"required": []string{"collection"},
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			var in struct {
				Collection string   `json:"collection"`
				Format     string   `json:"format"`
				Filter     string   `json:"filter"`
				Fields     []string `json:"fields"`
				Limit      int      `json:"limit"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			if in.Collection == "" {
				return "", fmt.Errorf("collection name is required")
			}
			switch in.Format {
			case "":
				in.Format = "json"
			case "csv", "json":
			default:
				return "", fmt.Errorf("format must be \"csv\" or \"json\"")
			}
			if in.Limit < 0 || in.Limit > 1000 {
				return "", fmt.Errorf("limit must be between 1 and 1000")
			}
			if in.Limit == 0 {
				in.Limit = 100
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return "", err
			}
			if !a.canList(coll) {
				return "", fmt.Errorf("you do not have permission to list %q", coll.Name)
			}
			records, err := a.App.FindRecordsByFilter(coll, in.Filter, "", in.Limit, 0)
			if err != nil {
				return "", fmt.Errorf("query failed: %w", err)
			}
			visible := make([]*core.Record, 0, len(records))
			for _, r := range records {
				if a.canAccessRecord(r) {
					visible = append(visible, r)
				}
			}
			if len(visible) == 0 {
				return "No accessible records found.", nil
			}

			var data []byte
			var contentType string
			if in.Format == "csv" {
				data, err = marshalCSVExport(visible, in.Fields)
				contentType = "text/csv"
			} else {
				data, err = marshalJSONExport(visible, in.Fields)
				contentType = "application/json"
			}
			if err != nil {
				return "", err
			}

			ownerID := ""
			if a.Info.Auth != nil {
				ownerID = a.Info.Auth.Id
			}
			filename := fmt.Sprintf("%s_%s.%s", coll.Name, time.Now().Format("20060102-150405"), in.Format)
			exp := storeExport(ownerID, filename, contentType, data)
			exp.Count = len(visible)
			expOut, _ := json.Marshal(map[string]any{"export": exp})
			return string(expOut), nil
		},
	}
}

// marshalJSONExport serializes the given records as a JSON array. The system
// fields (id, created, updated, collection*) are always included unless an
// explicit field list limits the columns.
func marshalJSONExport(records []*core.Record, fields []string) ([]byte, error) {
	rows := make([]map[string]any, 0, len(records))
	for _, r := range records {
		rows = append(rows, exportRow(r, fields))
	}
	return json.MarshalIndent(rows, "", "  ")
}

// marshalCSVExport serializes the given records as CSV with a header row.
func marshalCSVExport(records []*core.Record, fields []string) ([]byte, error) {
	var first *core.Record
	if len(records) > 0 {
		first = records[0]
	}
	cols := exportColumns(first, fields)

	var b strings.Builder
	w := csv.NewWriter(&b)
	if err := w.Write(cols); err != nil {
		return nil, err
	}
	for _, r := range records {
		row := make([]string, 0, len(cols))
		for _, c := range cols {
			row = append(row, csvCell(r.Get(c)))
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

// exportColumns derives the column list: explicit fields when provided,
// otherwise every schema field plus the record id.
func exportColumns(r *core.Record, fields []string) []string {
	if len(fields) > 0 {
		return fields
	}
	if r == nil {
		return []string{"id"}
	}
	names := r.Collection().Fields.FieldNames()
	if !containsString(names, "id") {
		names = append([]string{"id"}, names...)
	}
	return names
}

// exportRow maps one record to a row honoring the optional field list.
func exportRow(r *core.Record, fields []string) map[string]any {
	cols := exportColumns(r, fields)
	out := make(map[string]any, len(cols))
	for _, c := range cols {
		out[c] = r.Get(c)
	}
	return out
}

func csvCell(v any) string {
	switch t := v.(type) {
	case []byte:
		return string(t)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

func containsString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
