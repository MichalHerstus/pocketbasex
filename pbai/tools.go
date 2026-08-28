package pbai

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	openai "github.com/sashabaranov/go-openai"

	"pbx/pbrules"
)

// tool is a single agent tool definition.
type tool struct {
	name        string
	description string
	params      any // JSON Schema object
	write       bool
	exec        func(a *Agent, args json.RawMessage) (string, error)
	pending     func(a *Agent, args json.RawMessage) (*PendingAction, error)
}

// toolDefs returns the list of tools exposed to the LLM.
func toolDefs() []openai.Tool {
	defs := allTools()
	out := make([]openai.Tool, 0, len(defs))
	for _, d := range defs {
		out = append(out, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        d.name,
				Description: d.description,
				Parameters:  d.params,
			},
		})
	}
	return out
}

// filteredToolDefs returns only the tools whose names appear in the allow list.
func filteredToolDefs(allow []string) []openai.Tool {
	allowSet := make(map[string]bool, len(allow))
	for _, name := range allow {
		allowSet[name] = true
	}
	defs := allTools()
	out := make([]openai.Tool, 0, len(allow))
	for _, d := range defs {
		if allowSet[d.name] {
			out = append(out, openai.Tool{
				Type: openai.ToolTypeFunction,
				Function: &openai.FunctionDefinition{
					Name:        d.name,
					Description: d.description,
					Parameters:  d.params,
				},
			})
		}
	}
	return out
}

// allTools returns the tool definitions in registry order.
func allTools() []tool {
	return []tool{
		listCollectionsTool(),
		getSchemaTool(),
		queryRecordsTool(),
		insertRecordsTool(),
		createCollectionTool(),
		setViewConfigTool(),
		createActionTool(),
		listActionsTool(),
		updateRecordsTool(),
		deleteRecordsTool(),
		deleteSelectedRecordsTool(),
		updateCollectionTool(),
		deleteCollectionTool(),
		setCollectionRulesTool(),
		updateViewConfigTool(),
		deleteViewConfigTool(),
	}
}

var toolRegistry = map[string]tool{}

func register(t tool) {
	toolRegistry[t.name] = t
}

func findTool(name string) *tool {
	t, ok := toolRegistry[name]
	if !ok {
		return nil
	}
	return &t
}

// --- access helpers (mirror PB API rule semantics) ---

func (a *Agent) canList(coll *core.Collection) bool {
	return a.isSuper() || coll.ListRule != nil
}

func (a *Agent) canView(coll *core.Collection) bool {
	return a.isSuper() || coll.ViewRule != nil
}

func (a *Agent) canCreate(coll *core.Collection) bool {
	return a.isSuper() || coll.CreateRule != nil
}

func (a *Agent) canUpdate(coll *core.Collection) bool {
	return a.isSuper() || coll.UpdateRule != nil
}

func (a *Agent) canDelete(coll *core.Collection) bool {
	return a.isSuper() || coll.DeleteRule != nil
}

// canAccessRecord reports whether the caller may read the given record
// (enforces the collection view rule per record).
func (a *Agent) canAccessRecord(rec *core.Record) bool {
	ok, err := a.App.CanAccessRecord(rec, a.Info, rec.Collection().ViewRule)
	return err == nil && ok
}

// findCollection resolves a collection by name or id. On failure it returns
// an error suggesting the closest matching accessible collection names -
// LLMs frequently mistype collection names in tool arguments, and a
// self-correcting error lets them retry with the right one.
func (a *Agent) findCollection(name string) (*core.Collection, error) {
	coll, err := a.App.FindCachedCollectionByNameOrId(name)
	if err == nil {
		return coll, nil
	}

	var available, suggestions []string
	colls, lerr := a.App.FindAllCollections()
	if lerr == nil {
		threshold := len(name) / 3
		if threshold < 1 {
			threshold = 1
		}
		if threshold > 2 {
			threshold = 2
		}
		for _, c := range colls {
			if strings.HasPrefix(c.Name, "_") || c.IsView() || !a.canList(c) {
				continue
			}
			available = append(available, c.Name)
			if levenshtein(strings.ToLower(name), strings.ToLower(c.Name)) <= threshold {
				suggestions = append(suggestions, c.Name)
			}
		}
		sort.Strings(available)
	}

	msg := fmt.Sprintf("collection %q not found", name)
	switch len(suggestions) {
	case 0:
	case 1:
		msg += fmt.Sprintf(" (did you mean %q?)", suggestions[0])
	default:
		msg += fmt.Sprintf(" (did you mean one of %q?)", suggestions)
	}
	if len(available) > 0 {
		msg += ". Available collections: " + strings.Join(available, ", ")
	}
	return nil, errors.New(msg)
}

// levenshtein computes the edit distance between two strings.
func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
}

// checkCreateRule enforces the collection createRule for a non-superuser.
// Uses the shared pbrules package.
func (a *Agent) checkCreateRule(coll *core.Collection, data map[string]any) error {
	return pbrules.CheckCreateRule(pbrules.CheckCreateRuleContext{
		App:         a.App,
		RequestInfo: a.Info,
		IsSuperuser: a.isSuper(),
	}, coll, data)
}

// --- collection listing ---

func listCollectionsTool() tool {
	return tool{
		name:        "list_collections",
		description: "Lists the collections the current user is allowed to list. Returns collection names with their field count. Use this first to discover what data is available.",
		params: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			colls, err := a.App.FindAllCollections()
			if err != nil {
				return "", err
			}
			var names []string
			for _, c := range colls {
				if strings.HasPrefix(c.Name, "_") || c.IsView() {
					continue
				}
				if !a.canList(c) {
					continue
				}
				names = append(names, fmt.Sprintf("%s (%d fields)", c.Name, len(c.Fields)))
			}
			sort.Strings(names)
			if len(names) == 0 {
				return "No listable collections.", nil
			}
			return strings.Join(names, "\n"), nil
		},
	}
}

func getSchemaTool() tool {
	return tool{
		name:        "get_collection_schema",
		description: "Returns the field names and types of a collection. Args: {\"collection\": \"name\"}.",
		params: map[string]any{
			"type":       "object",
			"properties": map[string]any{"collection": map[string]any{"type": "string"}},
			"required":   []string{"collection"},
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			var in struct {
				Collection string `json:"collection"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			if in.Collection == "" {
				return "", fmt.Errorf("collection name is required")
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return "", err
			}
			if !a.canView(coll) {
				return "", fmt.Errorf("you do not have permission to view %q", coll.Name)
			}
			var out []string
			for _, f := range coll.Fields {
				out = append(out, fmt.Sprintf("%s (%s)%s", f.GetName(), f.Type(), requiredMark(f)))
			}
			if len(out) == 0 {
				return "Collection has no fields.", nil
			}
			return strings.Join(out, "\n"), nil
		},
	}
}

func requiredMark(f core.Field) string {
	if j, err := json.Marshal(f); err == nil {
		var m map[string]any
		if json.Unmarshal(j, &m) == nil {
			if r, ok := m["required"].(bool); ok && r {
				return " required"
			}
		}
	}
	return ""
}

func queryRecordsTool() tool {
	return tool{
		name:        "query_records",
		description: "Queries records from a collection the user is allowed to read. Args: {\"collection\": \"name\", \"filter\": \"optional PB filter expression\", \"limit\": 20, \"offset\": 0, \"sort\": \"field1,-field2\", \"fields\": \"col1,col2\"}. Returns up to 20 records as JSON. Filter syntax examples: \"title ~ 'foo'\", \"price > 100 && visible = true\". Sort accepts field names or column labels from view config (prefix with - for descending). Fields limits returned columns.",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"collection": map[string]any{"type": "string"},
				"filter":     map[string]any{"type": "string"},
				"limit":      map[string]any{"type": "integer"},
				"offset":     map[string]any{"type": "integer"},
				"sort":       map[string]any{"type": "string"},
				"fields":     map[string]any{"type": "string"},
			},
			"required": []string{"collection"},
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			var in struct {
				Collection string `json:"collection"`
				Filter     string `json:"filter"`
				Limit      int    `json:"limit"`
				Offset     int    `json:"offset"`
				Sort       string `json:"sort"`
				Fields     string `json:"fields"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			if in.Collection == "" {
				return "", fmt.Errorf("collection name is required")
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return "", err
			}
			if !a.canList(coll) {
				return "", fmt.Errorf("you do not have permission to list %q", coll.Name)
			}
			limit := in.Limit
			if limit <= 0 {
				limit = 10
			}
			if limit > 20 {
				limit = 20
			}
			offset := in.Offset
			if offset < 0 {
				offset = 0
			}
			sortStr := translateSort(a, coll.Name, in.Sort)
			recs, err := a.App.FindRecordsByFilter(coll, in.Filter, sortStr, limit, offset)
			if err != nil {
				return "", fmt.Errorf("query failed: %w", err)
			}
			var visible []map[string]any
			for _, r := range recs {
				if !a.canAccessRecord(r) {
					continue
				}
				visible = append(visible, r.PublicExport())
			}
			if len(visible) == 0 {
				return "No accessible records found.", nil
			}
			if in.Fields != "" {
				fieldSet := map[string]bool{}
				for _, f := range strings.Split(in.Fields, ",") {
					f = strings.TrimSpace(f)
					if f != "" {
						fieldSet[f] = true
					}
				}
				fieldSet["id"] = true
				var filtered []map[string]any
				for _, rec := range visible {
					row := map[string]any{}
					for k, v := range rec {
						if fieldSet[k] {
							row[k] = v
						}
					}
					filtered = append(filtered, row)
				}
				visible = filtered
			}
			data, err := json.Marshal(visible)
			if err != nil {
				return "", err
			}
			return string(data), nil
		},
	}
}

// --- write tools (pending action) ---

func insertRecordsTool() tool {
	return tool{
		name: "insert_records",
		description: "Inserts one or more new records into a collection. Requires explicit user confirmation. Args: {\"collection\": \"name\", \"records\": [{\"field\": \"value\"}]}. Only include fields that exist in the collection schema.",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"collection": map[string]any{"type": "string"},
				"records": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "object"},
				},
			},
			"required": []string{"collection", "records"},
		},
		write: true,
		pending: func(a *Agent, args json.RawMessage) (*PendingAction, error) {
			var in struct {
				Collection string           `json:"collection"`
				Records    []map[string]any `json:"records"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Collection == "" {
				return nil, fmt.Errorf("collection name is required")
			}
			if len(in.Records) == 0 {
				return nil, fmt.Errorf("no records provided")
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return nil, err
			}
			if !a.canCreate(coll) {
				return nil, fmt.Errorf("you do not have permission to create records in %q", coll.Name)
			}

			// preview: coerce values and show the first record
			preview := make([]map[string]any, 0, len(in.Records))
			for _, r := range in.Records {
				coerced, err := coerceRecordValues(coll, r)
				if err != nil {
					return nil, err
				}
				preview = append(preview, coerced)
			}

			detail, err := json.MarshalIndent(preview, "", "  ")
			if err != nil {
				return nil, err
			}

			return &PendingAction{
				Type:       "insert_records",
				Summary:    a.tr("ai.confirmInsert", len(preview), a.recordNoun(len(preview)), coll.Name),
				Detail:     string(detail),
				Collection: coll.Name,
				toolName:   "insert_records",
				params:     mustMarshal(in),
			}, nil
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			var in struct {
				Collection string           `json:"collection"`
				Records    []map[string]any `json:"records"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return "", err
			}
			var created []string
			for _, raw := range in.Records {
				data, err := coerceRecordValues(coll, raw)
				if err != nil {
					return "", err
				}
				if err := a.checkCreateRule(coll, data); err != nil {
					return "", err
				}
				rec := core.NewRecord(coll)
				for k, v := range data {
					rec.Set(k, v)
				}
				if err := a.App.Save(rec); err != nil {
					return "", fmt.Errorf("failed to save record in %q: %w", coll.Name, err)
				}
				created = append(created, rec.Id)
			}
			return a.tr("ai.insertedRecords", len(created), a.recordNoun(len(created)), coll.Name, strings.Join(created, ", ")), nil
		},
	}
}

// coerceRecordValues normalizes incoming JSON values to PB-friendly types.
func coerceRecordValues(coll *core.Collection, data map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(data))
	for k, v := range data {
		if strings.HasPrefix(k, "_") || k == "id" || k == "created" || k == "updated" {
			continue
		}
		f := coll.Fields.GetByName(k)
		if f == nil {
			continue // skip unknown fields
		}
		switch f.Type() {
		case "bool":
			out[k] = coerceBool(v)
		case "number":
			val, err := coerceNumber(v)
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", k, err)
			}
			out[k] = val
		case "date":
			out[k] = coerceDate(v)
		default:
			out[k] = v
		}
	}
	return out, nil
}

func coerceBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		b, _ := strconv.ParseBool(t)
		return b
	case float64:
		return t != 0
	case int:
		return t != 0
	}
	return false
}

func coerceNumber(v any) (any, error) {
	switch t := v.(type) {
	case float64:
		return t, nil
	case int:
		return float64(t), nil
	case json.Number:
		return t.Float64()
	case string:
		if strings.TrimSpace(t) == "" {
			return nil, nil
		}
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q", t)
		}
		return f, nil
	}
	return nil, fmt.Errorf("unsupported number value %v", v)
}

func coerceDate(v any) string {
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return ""
		}
		if tm, err := time.Parse("2006-01-02", s); err == nil {
			return tm.Format("2006-01-02 15:04:05.000Z")
		}
		if tm, err := time.Parse(time.RFC3339, s); err == nil {
			return tm.Format("2006-01-02 15:04:05.000Z")
		}
		return s
	case time.Time:
		return t.Format("2006-01-02 15:04:05.000Z")
	}
	return fmt.Sprint(v)
}

// createCollectionTool creates a new base collection with the given fields.
func createCollectionTool() tool {
	return tool{
		name: "create_collection",
		description: "Creates a new base collection. Superuser only. Args: {\"name\": \"collection_name\", \"fields\": [{\"name\": \"field_name\", \"type\": \"text|number|bool|date|json\"}]}.",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string"},
				"fields": map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			},
			"required": []string{"name", "fields"},
		},
		write: true,
		pending: func(a *Agent, args json.RawMessage) (*PendingAction, error) {
			var in struct {
				Name   string `json:"name"`
				Fields []struct {
					Name string `json:"name"`
					Type string `json:"type"`
				} `json:"fields"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if !a.isSuper() {
				return nil, fmt.Errorf("only superusers can create collections")
			}
			if in.Name == "" {
				return nil, fmt.Errorf("collection name is required")
			}
			if len(in.Fields) == 0 {
				return nil, fmt.Errorf("at least one field is required")
			}
			detail, _ := json.MarshalIndent(in, "", "  ")
			return &PendingAction{
				Type:     "create_collection",
				Summary:  fmt.Sprintf("Create collection %q with %d field(s)", in.Name, len(in.Fields)),
				Detail:   string(detail),
				toolName: "create_collection",
				params:   mustMarshal(in),
			}, nil
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			var in struct {
				Name   string `json:"name"`
				Fields []struct {
					Name string `json:"name"`
					Type string `json:"type"`
				} `json:"fields"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			if !a.isSuper() {
				return "", fmt.Errorf("only superusers can create collections")
			}
			name := sanitizeName(in.Name)
			if existing, _ := a.App.FindCachedCollectionByNameOrId(name); existing != nil {
				return "", fmt.Errorf("collection %q already exists", name)
			}

			used := map[string]bool{}
			for _, reserved := range []string{"id", "created", "updated", "collectionid", "collectionname", "expand"} {
				used[reserved] = true
			}
			var newFields []core.Field
			for _, f := range in.Fields {
				fn := sanitizeName(f.Name)
				if fn == "" || used[fn] {
					continue
				}
				used[fn] = true
				typ := core.FieldTypeText
				switch strings.ToLower(f.Type) {
				case "number":
					typ = core.FieldTypeNumber
				case "bool":
					typ = core.FieldTypeBool
				case "date":
					typ = core.FieldTypeDate
				case "json":
					typ = core.FieldTypeJSON
				}
				cf := core.Fields[typ]()
				cf.SetName(fn)
				newFields = append(newFields, cf)
			}
			if len(newFields) == 0 {
				return "", fmt.Errorf("no usable fields provided")
			}

			coll := core.NewBaseCollection(name)
			coll.Fields.Add(newFields...)
			if err := a.App.Save(coll); err != nil {
				return "", fmt.Errorf("failed to create collection: %w", err)
			}
			return fmt.Sprintf("Collection %q created with %d field(s).", name, len(newFields)), nil
		},
	}
}

// setViewConfigTool upserts a _views config for a collection.
func setViewConfigTool() tool {
	return tool{
		name: "set_view_config",
		description: "Creates or updates the view configuration (list page + form) for a collection. Superuser only. Args: {\"collection\": \"name\", \"configName\": \"optional name (defaults to collection name)\", \"pageTitle\": \"optional list heading\", \"columnTitles\": \"optional comma-separated column titles\"}.",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"collection":    map[string]any{"type": "string"},
				"configName":    map[string]any{"type": "string"},
				"pageTitle":     map[string]any{"type": "string"},
				"columnTitles":  map[string]any{"type": "string"},
				"columnSorting": map[string]any{"type": "boolean"},
				"searchBox":     map[string]any{"type": "boolean"},
				"pagination":    map[string]any{"type": "boolean"},
			},
			"required": []string{"collection"},
		},
		write: true,
		pending: func(a *Agent, args json.RawMessage) (*PendingAction, error) {
			var in struct {
				Collection    string `json:"collection"`
				ConfigName    string `json:"configName"`
				PageTitle     string `json:"pageTitle"`
				ColumnTitles  string `json:"columnTitles"`
				ColumnSorting *bool  `json:"columnSorting"`
				SearchBox     *bool  `json:"searchBox"`
				Pagination    *bool  `json:"pagination"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if !a.isSuper() {
				return nil, fmt.Errorf("only superusers can update view configurations")
			}
			if in.Collection == "" {
				return nil, fmt.Errorf("collection name is required")
			}
			if _, err := a.findCollection(in.Collection); err != nil {
			return nil, err
			}
			if in.ConfigName == "" {
				in.ConfigName = sanitizeName(in.Collection)
			}
			detail, _ := json.MarshalIndent(in, "", "  ")
			return &PendingAction{
				Type:       "set_view_config",
				Summary:    fmt.Sprintf("Set view configuration %q for collection %q", in.ConfigName, in.Collection),
				Detail:     string(detail),
				Collection: in.Collection,
				toolName:   "set_view_config",
				params:     mustMarshal(in),
			}, nil
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			var in struct {
				Collection    string `json:"collection"`
				ConfigName    string `json:"configName"`
				PageTitle     string `json:"pageTitle"`
				ColumnTitles  string `json:"columnTitles"`
				ColumnSorting *bool  `json:"columnSorting"`
				SearchBox     *bool  `json:"searchBox"`
				Pagination    *bool  `json:"pagination"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			if !a.isSuper() {
				return "", fmt.Errorf("only superusers can update view configurations")
			}
			if _, err := a.findCollection(in.Collection); err != nil {
			return "", err
			}
			if in.ConfigName == "" {
				in.ConfigName = sanitizeName(in.Collection)
			}

			coll, err := a.App.FindCachedCollectionByNameOrId("_views")
			if err != nil {
				return "", fmt.Errorf("views collection not found")
			}
			recs, err := a.App.FindRecordsByFilter("_views", "_name = {:name}", "", 1, 0, dbx.Params{"name": in.ConfigName})
			if err != nil {
				return "", err
			}
			var rec *core.Record
			if len(recs) > 0 {
				rec = recs[0]
			} else {
				rec = core.NewRecord(coll)
				rec.Set("_name", in.ConfigName)
			}
			rec.Set("_collName", in.Collection)

			tab := map[string]any{
				"pageTitle": in.PageTitle,
			}
			if in.ColumnTitles != "" {
				tab["columnTitles"] = in.ColumnTitles
			}
			if in.ColumnSorting != nil {
				tab["columnSorting"] = *in.ColumnSorting
			}
			if in.SearchBox != nil {
				tab["searchBox"] = *in.SearchBox
			}
			if in.Pagination != nil {
				tab["pagination"] = *in.Pagination
			}
			tabJSON, _ := json.Marshal(tab)
			rec.Set("_tabulator", string(tabJSON))

			if err := a.App.Save(rec); err != nil {
				return "", fmt.Errorf("failed to save view config: %w", err)
			}
			return fmt.Sprintf("View configuration %q saved for collection %q.", in.ConfigName, in.Collection), nil
		},
	}
}

// actionArgs holds the parsed create_action arguments (shared by pending/exec).
type actionArgs struct {
	Name        string `json:"name"`
	Collection  string `json:"collection"`
	Script      string `json:"script"`
	Description string `json:"description"`
	OnList      *bool  `json:"onList"`
	OnForm      *bool  `json:"onForm"`
	Public      *bool  `json:"public"`
}

// resolveDefaults applies the documented defaults for unset flags.
func (in *actionArgs) resolveDefaults() {
	t, f := true, false
	if in.OnList == nil {
		in.OnList = &t
	}
	if in.OnForm == nil {
		in.OnForm = &f
	}
	if in.Public == nil {
		in.Public = &f
	}
}

// createActionTool upserts an _actions record (custom Goja action script).
func createActionTool() tool {
	return tool{
		name: "create_action",
		description: "Creates or updates a custom action: a JavaScript script attached to a collection that the user can run from its tabular or form view. Superuser only. Upserts by name - if an action with the same name exists it is updated. Args: {\"name\": \"display name\", \"collection\": \"target collection\", \"script\": \"JavaScript source\", \"description\": \"optional help text\", \"onList\": true, \"onForm\": false, \"public\": false}.",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":        map[string]any{"type": "string"},
				"collection":  map[string]any{"type": "string"},
				"script":      map[string]any{"type": "string"},
				"description": map[string]any{"type": "string"},
				"onList":      map[string]any{"type": "boolean", "description": "show in tabular view dropdown (default true)"},
				"onForm":      map[string]any{"type": "boolean", "description": "show in form view dropdown (default false)"},
				"public":      map[string]any{"type": "boolean", "description": "visible to non-superusers (default false)"},
			},
			"required": []string{"name", "collection", "script"},
		},
		write: true,
		pending: func(a *Agent, args json.RawMessage) (*PendingAction, error) {
			var in actionArgs
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if !a.isSuper() {
				return nil, fmt.Errorf("only superusers can create or update actions")
			}
			if in.Name == "" {
				return nil, fmt.Errorf("action name is required")
			}
			if in.Script == "" {
				return nil, fmt.Errorf("script is required")
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return nil, err
			}
			if _, err := goja.Compile("action.js", in.Script, false); err != nil {
				return nil, fmt.Errorf("script compile error: %w", err)
			}
			detail, _ := json.MarshalIndent(in, "", "  ")
			return &PendingAction{
				Type:       "create_action",
				Summary:    fmt.Sprintf("Create/update action %q on collection %q", in.Name, coll.Name),
				Detail:     string(detail),
				Collection: coll.Name,
				toolName:   "create_action",
				params:     mustMarshal(in),
			}, nil
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			var in actionArgs
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			if !a.isSuper() {
				return "", fmt.Errorf("only superusers can create or update actions")
			}
			if _, err := a.findCollection(in.Collection); err != nil {
			return "", err
			}
			in.resolveDefaults()

			actionsColl, err := a.App.FindCachedCollectionByNameOrId("_actions")
			if err != nil {
				return "", fmt.Errorf("actions collection not found")
			}
			recs, err := a.App.FindRecordsByFilter("_actions", "_name = {:name}", "", 1, 0, dbx.Params{"name": in.Name})
			if err != nil {
				return "", err
			}
			var rec *core.Record
			created := false
			if len(recs) > 0 {
				rec = recs[0]
			} else {
				rec = core.NewRecord(actionsColl)
				created = true
			}
			rec.Set("_name", in.Name)
			rec.Set("_collection", in.Collection)
			rec.Set("_script", in.Script)
			rec.Set("_description", in.Description)
			rec.Set("_onList", *in.OnList)
			rec.Set("_onForm", *in.OnForm)
			rec.Set("_public", *in.Public)
			if err := a.App.Save(rec); err != nil {
				return "", fmt.Errorf("failed to save action: %w", err)
			}
			verb := "updated"
			if created {
				verb = "created"
			}
			return fmt.Sprintf("Action %q %s for collection %q.", in.Name, verb, in.Collection), nil
		},
	}
}

// listActionsTool lists the custom actions defined for a collection.
func listActionsTool() tool {
	return tool{
		name:        "list_actions",
		description: "Lists the custom actions defined for a collection with their flags and descriptions. Superuser only. Args: {\"collection\": \"name\"}.",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"collection": map[string]any{"type": "string"},
			},
			"required": []string{"collection"},
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			var in struct {
				Collection string `json:"collection"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			if !a.isSuper() {
				return "", fmt.Errorf("only superusers can list actions")
			}
			if in.Collection == "" {
				return "", fmt.Errorf("collection name is required")
			}
			recs, err := a.App.FindRecordsByFilter("_actions", "_collection = {:coll}", "_name", 100, 0, dbx.Params{"coll": in.Collection})
			if err != nil {
				return "", err
			}
			if len(recs) == 0 {
				return fmt.Sprintf("No custom actions defined for collection %q.", in.Collection), nil
			}
			var out []string
			for _, r := range recs {
				var flags []string
				if r.GetBool("_onList") {
					flags = append(flags, "list")
				}
				if r.GetBool("_onForm") {
					flags = append(flags, "form")
				}
				if r.GetBool("_public") {
					flags = append(flags, "public")
				}
				out = append(out, fmt.Sprintf("- %s [%s]: %s", r.GetString("_name"), strings.Join(flags, ","), r.GetString("_description")))
			}
			return strings.Join(out, "\n"), nil
		},
	}
}

// --- record update/delete tools ---

func updateRecordsTool() tool {
	return tool{
		name: "update_records",
		description: "Updates one or more existing records in a collection. Requires explicit user confirmation. Args: {\"collection\": \"name\", \"records\": [{\"id\": \"record_id\", \"field\": \"value\", ...}]}. Max 50 records. Only include fields that exist in the collection schema.",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"collection": map[string]any{"type": "string"},
				"records": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "object"},
				},
			},
			"required": []string{"collection", "records"},
		},
		write: true,
		pending: func(a *Agent, args json.RawMessage) (*PendingAction, error) {
			var in struct {
				Collection string           `json:"collection"`
				Records    []map[string]any `json:"records"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Collection == "" {
				return nil, fmt.Errorf("collection name is required")
			}
			if len(in.Records) == 0 {
				return nil, fmt.Errorf("no records provided")
			}
			if len(in.Records) > 50 {
				return nil, fmt.Errorf("maximum 50 records per update")
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return nil, err
			}
			if !a.canUpdate(coll) {
				return nil, fmt.Errorf("you do not have permission to update records in %q", coll.Name)
			}
			for _, r := range in.Records {
				if _, ok := r["id"]; !ok {
					return nil, fmt.Errorf("each record must have an 'id' field")
				}
			}
			preview := make([]map[string]any, 0, len(in.Records))
			for _, r := range in.Records {
				coerced, err := coerceRecordValues(coll, r)
				if err != nil {
					return nil, err
				}
				coerced["id"] = r["id"]
				preview = append(preview, coerced)
			}
			detail, err := json.MarshalIndent(preview, "", "  ")
			if err != nil {
				return nil, err
			}
			return &PendingAction{
				Type:       "update_records",
				Summary:    a.tr("ai.confirmUpdate", len(preview), a.recordNoun(len(preview)), coll.Name),
				Detail:     string(detail),
				Collection: coll.Name,
				toolName:   "update_records",
				params:     mustMarshal(in),
			}, nil
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			var in struct {
				Collection string           `json:"collection"`
				Records    []map[string]any `json:"records"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return "", err
			}
			var updated []string
			for _, raw := range in.Records {
				id, _ := raw["id"].(string)
				if id == "" {
					continue
				}
				rec, err := a.App.FindRecordById(coll, id)
				if err != nil {
					return "", fmt.Errorf("record %q not found in %q: %w", id, coll.Name, err)
				}
				ok, err := a.App.CanAccessRecord(rec, a.Info, coll.UpdateRule)
				if err != nil || !ok {
					return "", fmt.Errorf("you do not have permission to update record %q in %q", id, coll.Name)
				}
				data, err := coerceRecordValues(coll, raw)
				if err != nil {
					return "", err
				}
				for k, v := range data {
					rec.Set(k, v)
				}
				if err := a.App.Save(rec); err != nil {
					return "", fmt.Errorf("failed to save record %q in %q: %w", id, coll.Name, err)
				}
				updated = append(updated, id)
			}
			return a.tr("ai.updatedRecords", len(updated), a.recordNoun(len(updated)), coll.Name, strings.Join(updated, ", ")), nil
		},
	}
}

func deleteRecordsTool() tool {
	return tool{
		name: "delete_records",
		description: "Deletes one or more records from a collection. Requires explicit user confirmation. Args: {\"collection\": \"name\", \"ids\": [\"id1\", \"id2\", ...]}. Max 50 records.",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"collection": map[string]any{"type": "string"},
				"ids":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"collection", "ids"},
		},
		write: true,
		pending: func(a *Agent, args json.RawMessage) (*PendingAction, error) {
			var in struct {
				Collection string   `json:"collection"`
				IDs        []string `json:"ids"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Collection == "" {
				return nil, fmt.Errorf("collection name is required")
			}
			if len(in.IDs) == 0 {
				return nil, fmt.Errorf("no record IDs provided")
			}
			if len(in.IDs) > 50 {
				return nil, fmt.Errorf("maximum 50 records per delete")
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return nil, err
			}
			if !a.canDelete(coll) {
				return nil, fmt.Errorf("you do not have permission to delete records in %q", coll.Name)
			}
			detail, _ := json.MarshalIndent(in.IDs, "", "  ")
			return &PendingAction{
				Type:       "delete_records",
				Summary:    a.tr("ai.confirmDelete", len(in.IDs), a.recordNoun(len(in.IDs)), coll.Name),
				Detail:     string(detail),
				Collection: coll.Name,
				toolName:   "delete_records",
				params:     mustMarshal(in),
			}, nil
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			var in struct {
				Collection string   `json:"collection"`
				IDs        []string `json:"ids"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			return execDeleteRecords(a, in.Collection, in.IDs)
		},
	}
}

// deleteSelectedRecordsTool deletes the records the user has currently selected
// (checked) in the tabular view. The IDs come from the agent's SelectedIDs
// (captured from the client request), never from the LLM, so the model cannot
// target records the user did not explicitly select. Requires user confirmation.
func deleteSelectedRecordsTool() tool {
	return tool{
		name:        "delete_selected_records",
		description: "Deletes the records the user has currently selected (checked) in the table. Requires explicit user confirmation. Takes no arguments; the affected records are the user's current row selection.",
		params:      map[string]any{"type": "object", "properties": map[string]any{}},
		write:       true,
		pending: func(a *Agent, args json.RawMessage) (*PendingAction, error) {
			if len(a.SelectedIDs) == 0 {
				return nil, fmt.Errorf("no records selected: ask the user to select rows with the checkboxes first")
			}
			if len(a.SelectedIDs) > 50 {
				return nil, fmt.Errorf("maximum 50 records per delete")
			}
			if a.viewCollection == "" {
				return nil, fmt.Errorf("collection name is required")
			}
			coll, err := a.findCollection(a.viewCollection)
			if err != nil {
				return nil, err
			}
			if !a.canDelete(coll) {
				return nil, fmt.Errorf("you do not have permission to delete records in %q", coll.Name)
			}
			params := struct {
				Collection string   `json:"collection"`
				IDs        []string `json:"ids"`
			}{Collection: coll.Name, IDs: a.SelectedIDs}
			detail, _ := json.MarshalIndent(a.SelectedIDs, "", "  ")
			return &PendingAction{
				Type:       "delete_selected_records",
				Summary:    a.tr("ai.confirmDeleteSelected", len(a.SelectedIDs), a.selectedNoun(len(a.SelectedIDs)), coll.Name),
				Detail:     string(detail),
				Collection: coll.Name,
				toolName:   "delete_selected_records",
				params:     mustMarshal(params),
			}, nil
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			var in struct {
				Collection string   `json:"collection"`
				IDs        []string `json:"ids"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			return execDeleteRecords(a, in.Collection, in.IDs)
		},
	}
}

// execDeleteRecords performs the per-record delete loop shared by delete_records
// and delete_selected_records. Every record is re-checked against the collection
// delete rule with the caller's request info before deletion.
func execDeleteRecords(a *Agent, collName string, ids []string) (string, error) {
	coll, err := a.findCollection(collName)
	if err != nil {
		return "", err
	}
	var deleted []string
	for _, id := range ids {
		if id == "" {
			continue
		}
		rec, err := a.App.FindRecordById(coll, id)
		if err != nil {
			return "", fmt.Errorf("record %q not found in %q: %w", id, coll.Name, err)
		}
		ok, err := a.App.CanAccessRecord(rec, a.Info, coll.DeleteRule)
		if err != nil || !ok {
			return "", fmt.Errorf("you do not have permission to delete record %q in %q", id, coll.Name)
		}
		if err := a.App.Delete(rec); err != nil {
			return "", fmt.Errorf("failed to delete record %q in %q: %w", id, coll.Name, err)
		}
		deleted = append(deleted, id)
	}
	return a.tr("ai.deletedRecords", len(deleted), a.recordNoun(len(deleted)), coll.Name, strings.Join(deleted, ", ")), nil
}

// --- collection management tools ---

func updateCollectionTool() tool {
	return tool{
		name: "update_collection",
		description: "Adds or removes fields from a collection. Superuser only. Args: {\"collection\": \"name\", \"addFields\": [{\"name\": \"field_name\", \"type\": \"text|number|bool|date|json\"}], \"removeFields\": [\"field_name\", ...]}. Cannot change field types - only add or remove.",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"collection":    map[string]any{"type": "string"},
				"addFields":     map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
				"removeFields":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"collection"},
		},
		write: true,
		pending: func(a *Agent, args json.RawMessage) (*PendingAction, error) {
			var in struct {
				Collection   string `json:"collection"`
				AddFields    []struct {
					Name string `json:"name"`
					Type string `json:"type"`
				} `json:"addFields"`
				RemoveFields []string `json:"removeFields"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if !a.isSuper() {
				return nil, fmt.Errorf("only superusers can update collections")
			}
			if in.Collection == "" {
				return nil, fmt.Errorf("collection name is required")
			}
			if len(in.AddFields) == 0 && len(in.RemoveFields) == 0 {
				return nil, fmt.Errorf("no changes specified")
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return nil, err
			}
			summary := fmt.Sprintf("Update collection %q: ", coll.Name)
			if len(in.AddFields) > 0 {
				summary += fmt.Sprintf("add %d field(s)", len(in.AddFields))
			}
			if len(in.RemoveFields) > 0 {
				if len(in.AddFields) > 0 {
					summary += ", "
				}
				summary += fmt.Sprintf("remove %d field(s)", len(in.RemoveFields))
			}
			detail, _ := json.MarshalIndent(in, "", "  ")
			return &PendingAction{
				Type:     "update_collection",
				Summary:  summary,
				Detail:   string(detail),
				toolName: "update_collection",
				params:   mustMarshal(in),
			}, nil
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			var in struct {
				Collection   string `json:"collection"`
				AddFields    []struct {
					Name string `json:"name"`
					Type string `json:"type"`
				} `json:"addFields"`
				RemoveFields []string `json:"removeFields"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			if !a.isSuper() {
				return "", fmt.Errorf("only superusers can update collections")
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return "", err
			}
			existingNames := map[string]bool{}
			for _, f := range coll.Fields {
				existingNames[f.GetName()] = true
			}
			for _, name := range in.RemoveFields {
				if existingNames[name] {
					coll.Fields.RemoveByName(name)
					delete(existingNames, name)
				}
			}
			var added []string
			for _, f := range in.AddFields {
				fn := sanitizeName(f.Name)
				if fn == "" || existingNames[fn] {
					continue
				}
				typ := core.FieldTypeText
				switch strings.ToLower(f.Type) {
				case "number":
					typ = core.FieldTypeNumber
				case "bool":
					typ = core.FieldTypeBool
				case "date":
					typ = core.FieldTypeDate
				case "json":
					typ = core.FieldTypeJSON
				}
				cf := core.Fields[typ]()
				cf.SetName(fn)
				coll.Fields.Add(cf)
				existingNames[fn] = true
				added = append(added, fn)
			}
			if err := a.App.Save(coll); err != nil {
				return "", fmt.Errorf("failed to update collection: %w", err)
			}
			msg := fmt.Sprintf("Collection %q updated.", coll.Name)
			if len(added) > 0 {
				msg += fmt.Sprintf(" Added: %s.", strings.Join(added, ", "))
			}
			if len(in.RemoveFields) > 0 {
				msg += fmt.Sprintf(" Removed: %s.", strings.Join(in.RemoveFields, ", "))
			}
			return msg, nil
		},
	}
}

func deleteCollectionTool() tool {
	return tool{
		name: "delete_collection",
		description: "Deletes a base collection. Superuser only. Warns if view configurations reference the collection. Args: {\"collection\": \"name\", \"force\": false}.",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"collection": map[string]any{"type": "string"},
				"force":      map[string]any{"type": "boolean"},
			},
			"required": []string{"collection"},
		},
		write: true,
		pending: func(a *Agent, args json.RawMessage) (*PendingAction, error) {
			var in struct {
				Collection string `json:"collection"`
				Force      *bool `json:"force"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if !a.isSuper() {
				return nil, fmt.Errorf("only superusers can delete collections")
			}
			if in.Collection == "" {
				return nil, fmt.Errorf("collection name is required")
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return nil, err
			}
			force := false
			if in.Force != nil {
				force = *in.Force
			}
			if !force {
				recs, _ := a.App.FindRecordsByFilter("_views", "_collName = {:c}", "", 100, 0, dbx.Params{"c": coll.Name})
				if len(recs) > 0 {
					return nil, fmt.Errorf("collection %q has %d view configuration(s); use force=true to delete and remove all view configs", coll.Name, len(recs))
				}
			}
			detail, _ := json.MarshalIndent(in, "", "  ")
			return &PendingAction{
				Type:     "delete_collection",
				Summary:  fmt.Sprintf("Delete collection %q", coll.Name),
				Detail:   string(detail),
				toolName: "delete_collection",
				params:   mustMarshal(in),
			}, nil
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			var in struct {
				Collection string `json:"collection"`
				Force      *bool `json:"force"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			if !a.isSuper() {
				return "", fmt.Errorf("only superusers can delete collections")
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return "", err
			}
			force := false
			if in.Force != nil {
				force = *in.Force
			}
			if force {
				recs, _ := a.App.FindRecordsByFilter("_views", "_collName = {:c}", "", 100, 0, dbx.Params{"c": coll.Name})
				for _, r := range recs {
					_ = a.App.Delete(r)
				}
			}
			if err := a.App.Delete(coll); err != nil {
				return "", fmt.Errorf("failed to delete collection: %w", err)
			}
			return fmt.Sprintf("Collection %q deleted.", coll.Name), nil
		},
	}
}

func setCollectionRulesTool() tool {
	return tool{
		name: "set_collection_rules",
		description: "Sets the API rules for a collection. Superuser only. Args: {\"collection\": \"name\", \"listRule\": \"filter or empty\", \"viewRule\": \"filter or empty\", \"createRule\": \"filter or empty\", \"updateRule\": \"filter or empty\", \"deleteRule\": \"filter or empty\"}. Use empty string for public, null for superuser-only, or a PB filter expression.",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"collection": map[string]any{"type": "string"},
				"listRule":   map[string]any{"type": "string"},
				"viewRule":   map[string]any{"type": "string"},
				"createRule": map[string]any{"type": "string"},
				"updateRule": map[string]any{"type": "string"},
				"deleteRule": map[string]any{"type": "string"},
			},
			"required": []string{"collection"},
		},
		write: true,
		pending: func(a *Agent, args json.RawMessage) (*PendingAction, error) {
			var in struct {
				Collection string  `json:"collection"`
				ListRule   *string `json:"listRule"`
				ViewRule   *string `json:"viewRule"`
				CreateRule *string `json:"createRule"`
				UpdateRule *string `json:"updateRule"`
				DeleteRule *string `json:"deleteRule"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if !a.isSuper() {
				return nil, fmt.Errorf("only superusers can set collection rules")
			}
			if in.Collection == "" {
				return nil, fmt.Errorf("collection name is required")
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return nil, err
			}
			detail, _ := json.MarshalIndent(in, "", "  ")
			return &PendingAction{
				Type:       "set_collection_rules",
				Summary:    fmt.Sprintf("Update rules for collection %q", coll.Name),
				Detail:     string(detail),
				Collection: coll.Name,
				toolName:   "set_collection_rules",
				params:     mustMarshal(in),
			}, nil
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			var in struct {
				Collection string  `json:"collection"`
				ListRule   *string `json:"listRule"`
				ViewRule   *string `json:"viewRule"`
				CreateRule *string `json:"createRule"`
				UpdateRule *string `json:"updateRule"`
				DeleteRule *string `json:"deleteRule"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			if !a.isSuper() {
				return "", fmt.Errorf("only superusers can set collection rules")
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return "", err
			}
			if in.ListRule != nil {
				coll.ListRule = in.ListRule
			}
			if in.ViewRule != nil {
				coll.ViewRule = in.ViewRule
			}
			if in.CreateRule != nil {
				coll.CreateRule = in.CreateRule
			}
			if in.UpdateRule != nil {
				coll.UpdateRule = in.UpdateRule
			}
			if in.DeleteRule != nil {
				coll.DeleteRule = in.DeleteRule
			}
			if err := a.App.Save(coll); err != nil {
				return "", fmt.Errorf("failed to save rules: %w", err)
			}
			return fmt.Sprintf("Rules updated for collection %q.", coll.Name), nil
		},
	}
}

// --- view config management tools ---

func updateViewConfigTool() tool {
	return tool{
		name: "update_view_config",
		description: "Updates an existing view configuration. Superuser only. Args: {\"configName\": \"name\", \"pageTitle\": \"optional\", \"columnTitles\": \"optional\", \"columnSorting\": bool, \"searchBox\": bool, \"pagination\": bool, \"displaySystemCol\": bool, \"filter\": \"optional\", \"formTitle\": \"optional\", \"formDescr\": \"optional\", \"formLabels\": \"optional\", \"formLayout\": \"optional\", \"columnOrder\": \"optional\"}. Full replace of tabulator/form JSON.",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"configName":     map[string]any{"type": "string"},
				"pageTitle":      map[string]any{"type": "string"},
				"columnTitles":   map[string]any{"type": "string"},
				"columnSorting":  map[string]any{"type": "boolean"},
				"searchBox":      map[string]any{"type": "boolean"},
				"pagination":     map[string]any{"type": "boolean"},
				"displaySystemCol": map[string]any{"type": "boolean"},
				"filter":         map[string]any{"type": "string"},
				"formTitle":      map[string]any{"type": "string"},
				"formDescr":      map[string]any{"type": "string"},
				"formLabels":     map[string]any{"type": "string"},
				"formLayout":     map[string]any{"type": "string"},
				"columnOrder":    map[string]any{"type": "string"},
			},
			"required": []string{"configName"},
		},
		write: true,
		pending: func(a *Agent, args json.RawMessage) (*PendingAction, error) {
			var in struct {
				ConfigName      string `json:"configName"`
				PageTitle       string `json:"pageTitle"`
				ColumnTitles    string `json:"columnTitles"`
				ColumnSorting   *bool  `json:"columnSorting"`
				SearchBox       *bool  `json:"searchBox"`
				Pagination      *bool  `json:"pagination"`
				DisplaySystemCol *bool `json:"displaySystemCol"`
				Filter          string `json:"filter"`
				FormTitle       string `json:"formTitle"`
				FormDescr       string `json:"formDescr"`
				FormLabels      string `json:"formLabels"`
				FormLayout      string `json:"formLayout"`
				ColumnOrder     string `json:"columnOrder"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if !a.isSuper() {
				return nil, fmt.Errorf("only superusers can update view configurations")
			}
			if in.ConfigName == "" {
				return nil, fmt.Errorf("configName is required")
			}
			detail, _ := json.MarshalIndent(in, "", "  ")
			return &PendingAction{
				Type:       "update_view_config",
				Summary:    fmt.Sprintf("Update view configuration %q", in.ConfigName),
				Detail:     string(detail),
				toolName:   "update_view_config",
				params:     mustMarshal(in),
			}, nil
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			var in struct {
				ConfigName      string `json:"configName"`
				PageTitle       string `json:"pageTitle"`
				ColumnTitles    string `json:"columnTitles"`
				ColumnSorting   *bool  `json:"columnSorting"`
				SearchBox       *bool  `json:"searchBox"`
				Pagination      *bool  `json:"pagination"`
				DisplaySystemCol *bool `json:"displaySystemCol"`
				Filter          string `json:"filter"`
				FormTitle       string `json:"formTitle"`
				FormDescr       string `json:"formDescr"`
				FormLabels      string `json:"formLabels"`
				FormLayout      string `json:"formLayout"`
				ColumnOrder     string `json:"columnOrder"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			if !a.isSuper() {
				return "", fmt.Errorf("only superusers can update view configurations")
			}
			if in.ConfigName == "" {
				return "", fmt.Errorf("configName is required")
			}
			recs, err := a.App.FindRecordsByFilter("_views", "_name = {:name}", "", 1, 0, dbx.Params{"name": in.ConfigName})
			if err != nil || len(recs) == 0 {
				return "", fmt.Errorf("view configuration %q not found", in.ConfigName)
			}
			rec := recs[0]
			collName := rec.GetString("_collName")
			if collName == "" {
				return "", fmt.Errorf("view configuration %q has no associated collection", in.ConfigName)
			}
			if _, err := a.findCollection(collName); err != nil {
				return "", err
			}
			tab := map[string]any{}
			if in.PageTitle != "" {
				tab["pageTitle"] = in.PageTitle
			}
			if in.ColumnTitles != "" {
				tab["columnTitles"] = in.ColumnTitles
			}
			if in.ColumnSorting != nil {
				tab["columnSorting"] = *in.ColumnSorting
			}
			if in.SearchBox != nil {
				tab["searchBox"] = *in.SearchBox
			}
			if in.Pagination != nil {
				tab["pagination"] = *in.Pagination
			}
			if in.DisplaySystemCol != nil {
				tab["displaySystemCol"] = *in.DisplaySystemCol
			}
			if in.Filter != "" {
				tab["filter"] = in.Filter
			}
			if in.ColumnOrder != "" {
				tab["columnOrder"] = in.ColumnOrder
			}
			tabJSON, _ := json.Marshal(tab)
			rec.Set("_tabulator", string(tabJSON))
			form := map[string]any{}
			if in.FormTitle != "" {
				form["formTitle"] = in.FormTitle
			}
			if in.FormDescr != "" {
				form["formDescr"] = in.FormDescr
			}
			if in.FormLabels != "" {
				form["formLabels"] = in.FormLabels
			}
			if in.FormLayout != "" {
				form["formLayout"] = in.FormLayout
			}
			formJSON, _ := json.Marshal(form)
			rec.Set("_form", string(formJSON))
			if err := a.App.Save(rec); err != nil {
				return "", fmt.Errorf("failed to save view config: %w", err)
			}
			return fmt.Sprintf("View configuration %q updated for collection %q.", in.ConfigName, collName), nil
		},
	}
}

func deleteViewConfigTool() tool {
	return tool{
		name: "delete_view_config",
		description: "Deletes a view configuration. Superuser only. Args: {\"configName\": \"name\"}.",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"configName": map[string]any{"type": "string"},
			},
			"required": []string{"configName"},
		},
		write: true,
		pending: func(a *Agent, args json.RawMessage) (*PendingAction, error) {
			var in struct {
				ConfigName string `json:"configName"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if !a.isSuper() {
				return nil, fmt.Errorf("only superusers can delete view configurations")
			}
			if in.ConfigName == "" {
				return nil, fmt.Errorf("configName is required")
			}
			recs, err := a.App.FindRecordsByFilter("_views", "_name = {:name}", "", 1, 0, dbx.Params{"name": in.ConfigName})
			if err != nil || len(recs) == 0 {
				return nil, fmt.Errorf("view configuration %q not found", in.ConfigName)
			}
			detail, _ := json.MarshalIndent(in, "", "  ")
			return &PendingAction{
				Type:     "delete_view_config",
				Summary:  fmt.Sprintf("Delete view configuration %q", in.ConfigName),
				Detail:   string(detail),
				toolName: "delete_view_config",
				params:   mustMarshal(in),
			}, nil
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			var in struct {
				ConfigName string `json:"configName"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			if !a.isSuper() {
				return "", fmt.Errorf("only superusers can delete view configurations")
			}
			if in.ConfigName == "" {
				return "", fmt.Errorf("configName is required")
			}
			recs, err := a.App.FindRecordsByFilter("_views", "_name = {:name}", "", 1, 0, dbx.Params{"name": in.ConfigName})
			if err != nil || len(recs) == 0 {
				return "", fmt.Errorf("view configuration %q not found", in.ConfigName)
			}
			if err := a.App.Delete(recs[0]); err != nil {
				return "", fmt.Errorf("failed to delete view configuration: %w", err)
			}
			return fmt.Sprintf("View configuration %q deleted.", in.ConfigName), nil
		},
	}
}

// --- collection listing with labels for sort translation ---

// viewLabels returns a field→label map for the collection from its _views config.
func viewLabels(a *Agent, collName string) map[string]string {
	labels := map[string]string{}
	recs, err := a.App.FindRecordsByFilter("_views", "_collName = {:c}", "", 1, 0, dbx.Params{"c": collName})
	if err != nil || len(recs) == 0 {
		return labels
	}
	var form struct {
		FormLabels string            `json:"formLabels"`
		Labels     map[string]string `json:"labels"`
	}
	if j := recs[0].GetString("_form"); j != "" {
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
	return labels
}

// translateSort converts a label-based sort string to a field-based sort string.
func translateSort(a *Agent, collName, sort string) string {
	if sort == "" {
		return ""
	}
	labels := viewLabels(a, collName)
	if len(labels) == 0 {
		return sort
	}
	reverse := map[string]string{}
	for field, label := range labels {
		reverse[strings.ToLower(label)] = field
	}
	var parts []string
	for _, part := range strings.Split(sort, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		desc := false
		if strings.HasPrefix(part, "-") {
			desc = true
			part = part[1:]
		}
		field, ok := reverse[strings.ToLower(part)]
		if !ok {
			field = sanitizeName(part)
		}
		if desc {
			field = "-" + field
		}
		parts = append(parts, field)
	}
	return strings.Join(parts, ",")
}

// --- sanitizeName normalizes a collection/field name to lowercase [a-z0-9_].
func sanitizeName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	s := b.String()
	if s == "" || (s[0] >= '0' && s[0] <= '9') {
		s = "f_" + s
	}
	return s
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// register the tools in the registry
func init() {
	for _, d := range allTools() {
		register(d)
	}
}