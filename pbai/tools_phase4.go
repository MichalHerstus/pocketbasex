package pbai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	search "github.com/pocketbase/pocketbase/tools/search"

	"pbx/pbactions"
)

// --- query_related: cross-collection relation queries ---

func queryRelatedTool() tool {
	return tool{
		name: "query_related",
		description: "Fetches specific records of a collection and expands their relation fields (records in OTHER collections they point to). Args: {\"collection\": \"name\", \"ids\": [\"id1\",...], \"expand\": [\"relField\", ...]}. Expand names are relation fields of the base collection (a single field or dot-paths like \"category.minor\"). Nested related records are returned inline under the \"expand\" key. Returns up to 50 records as JSON.",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"collection": map[string]any{"type": "string"},
				"ids":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"expand":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"collection", "ids", "expand"},
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			var in struct {
				Collection string   `json:"collection"`
				IDs        []string `json:"ids"`
				Expand     []string `json:"expand"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			if in.Collection == "" {
				return "", fmt.Errorf("collection name is required")
			}
			if len(in.IDs) == 0 {
				return "", fmt.Errorf("ids are required")
			}
			if len(in.IDs) > 50 {
				return "", fmt.Errorf("maximum 50 record ids per query_related call")
			}
			if len(in.Expand) == 0 {
				return "", fmt.Errorf("expand relation fields are required")
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return "", err
			}
			if !a.canView(coll) {
				return "", fmt.Errorf("you do not have permission to view records in %q", coll.Name)
			}
			// validate expand paths: the top-level segment must be a relation field
			for _, exp := range in.Expand {
				top := strings.SplitN(exp, ".", 2)[0]
				f := coll.Fields.GetByName(top)
				if f == nil || f.Type() != "relation" {
					return "", fmt.Errorf("expand field %q is not a relation field of %q", top, coll.Name)
				}
			}

			// fetch + expand; related records are filtered through the related
			// collection's view rule so the tool never leaks a record the caller
			// could not read via the regular API.
			fetchFunc := func(relColl *core.Collection, relIds []string) ([]*core.Record, error) {
				rels, ferr := a.App.FindRecordsByIds(relColl.Id, relIds)
				if ferr != nil {
					return nil, ferr
				}
				var out []*core.Record
				for _, rr := range rels {
					ok, actx := a.App.CanAccessRecord(rr, a.Info, relColl.ViewRule)
					if actx == nil && ok {
						out = append(out, rr)
					}
				}
				return out, nil
			}

			var visible []map[string]any
			for _, id := range in.IDs {
				if id == "" {
					continue
				}
				rec, err := a.App.FindRecordById(coll, id)
				if err != nil || !a.canAccessRecord(rec) {
					continue
				}
				expFailures := a.App.ExpandRecord(rec, in.Expand, fetchFunc)
				for path, eerr := range expFailures {
					return "", fmt.Errorf("failed to expand %q: %w", path, eerr)
				}
				visible = append(visible, rec.PublicExport())
			}
if len(visible) == 0 {
			return a.tr("ai.noRecords"), nil
		}
			data, err := json.Marshal(visible)
			if err != nil {
				return "", err
			}
			return string(data), nil
		},
	}
}

// --- get_stats: SQL-level aggregates with listRule enforcement ---

func getStatsTool() tool {
	return tool{
		name: "get_stats",
		description: "Aggregate statistics over the records of a collection (respects the same list rule as the regular API). Args: {\"collection\": \"name\", \"filter\": \"optional PB filter expression\", \"field\": \"optional field name\", \"ops\": [\"count\",\"distinct\",\"min\",\"max\",\"avg\",\"sum\"]}. count needs no field; distinct counts unique values of a field; min/max/avg/sum need a numeric field. Returns one line per requested op.",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"collection": map[string]any{"type": "string"},
				"filter":     map[string]any{"type": "string"},
				"field":      map[string]any{"type": "string"},
				"ops": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string"},
				},
			},
			"required": []string{"collection"},
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			var in struct {
				Collection string   `json:"collection"`
				Filter     string   `json:"filter"`
				Field      string   `json:"field"`
				Ops        []string `json:"ops"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			if in.Collection == "" {
				return "", fmt.Errorf("collection name is required")
			}
			if len(in.Ops) == 0 {
				in.Ops = []string{"count"}
			}
			// forbid trailing semicolon / SQL-injection patterns in op names
			allowed := map[string]bool{"count": true, "distinct": true, "min": true, "max": true, "avg": true, "sum": true}
			for _, op := range in.Ops {
				if !allowed[op] {
					return "", fmt.Errorf("unknown op %q (allowed: count, distinct, min, max, avg, sum)", op)
				}
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return "", err
			}
			if !a.canList(coll) {
				return "", fmt.Errorf("you do not have permission to list %q", coll.Name)
			}
			if !isNumericFieldName(in.Field) {
				return "", fmt.Errorf("field name %q is invalid", in.Field)
			}

			var col string
			if in.Field != "" {
				f := coll.Fields.GetByName(in.Field)
				if f == nil {
					return "", fmt.Errorf("field %q not found in %q", in.Field, coll.Name)
				}
				col = f.GetName()
			}

			type result struct {
				op string
				v  string
			}
			var out []result
			needsField := map[string]bool{"distinct": true, "min": true, "max": true, "avg": true, "sum": true}
			for _, op := range in.Ops {
				if needsField[op] {
					if col == "" {
						return "", fmt.Errorf("op %q requires a field name", op)
					}
					f := coll.Fields.GetByName(col)
					if op != "distinct" && f.Type() != "number" {
						return "", fmt.Errorf("op %q requires a numeric field", op)
					}
				}

				q, err := a.statsQuery(coll, in.Filter)
				if err != nil {
					return "", err
				}
				switch op {
				case "count":
					q = q.Select("count(*)")
					var n int64
					if err := q.Row(&n); err != nil {
						return "", fmt.Errorf("count failed: %w", err)
					}
					out = append(out, result{op: "count", v: fmt.Sprintf("%d", n)})
				case "distinct":
					q = q.Select("count(distinct " + col + ")")
					var n int64
					if err := q.Row(&n); err != nil {
						return "", fmt.Errorf("distinct failed: %w", err)
					}
					out = append(out, result{op: "distinct", v: fmt.Sprintf("%d", n)})
				default:
					q = q.Select(op + "(" + col + ")")
					var v float64
					if err := q.Row(&v); err != nil {
						return "", fmt.Errorf("%s failed: %w", op, err)
					}
					out = append(out, result{op: op, v: formatNumber(v)})
				}
			}

			var b strings.Builder
			fmt.Fprintf(&b, "stats for %q", coll.Name)
			if in.Filter != "" {
				fmt.Fprintf(&b, " (filter: %s)", in.Filter)
			}
			b.WriteString(":\n")
			for _, r := range out {
				fmt.Fprintf(&b, "  %s: %s\n", r.op, r.v)
			}
			return strings.TrimRight(b.String(), "\n"), nil
		},
	}
}

// statsQuery builds the record query for get_stats, applying the caller's
// filter and (for non-superusers) the collection listRule as SQL conditions.
// The query is fresh per call so each op can replace the SELECT clause.
func (a *Agent) statsQuery(coll *core.Collection, filter string) (*dbx.SelectQuery, error) {
	q := a.App.RecordQuery(coll)
	resolver := core.NewRecordFieldResolver(a.App, coll, a.Info, true)
	if filter != "" {
		expr, err := search.FilterData(filter).BuildExpr(resolver, dbx.Params{})
		if err != nil {
			return nil, fmt.Errorf("invalid filter expression: %w", err)
		}
		q.AndWhere(expr)
	}
	if !a.isSuper() && coll.ListRule != nil && strings.TrimSpace(*coll.ListRule) != "" {
		expr, err := search.FilterData(*coll.ListRule).BuildExpr(resolver, dbx.Params{})
		if err != nil {
			return nil, fmt.Errorf("invalid list rule: %w", err)
		}
		q.AndWhere(expr)
	}
	if err := resolver.UpdateQuery(q); err != nil {
		return nil, err
	}
	return q, nil
}

// isNumericFieldName ensures a field name contains only [a-z0-9_] characters so
// it can be safely interpolated into an aggregate SQL expression.
func isNumericFieldName(name string) bool {
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return true
}

// formatNumber renders a float64 without unnecessary trailing zeros.
func formatNumber(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", v), "0"), ".")
}

// --- create_records_batch: bulk insert (confirmed, max 200) ---

func createRecordsBatchTool() tool {
	return tool{
		name: "create_records_batch",
		description: "Bulk-inserts many new records into a collection at once. Requires explicit user confirmation. Args: {\"collection\": \"name\", \"records\": [{\"field\": \"value\"}, ...]}. Up to 200 records per call. Only include fields that exist in the collection schema. Prefer this over repeated insert_records calls for large imports.",
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
			if len(in.Records) > 200 {
				return nil, fmt.Errorf("maximum 200 records per batch")
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return nil, err
			}
			if !a.canCreate(coll) {
				return nil, fmt.Errorf("you do not have permission to create records in %q", coll.Name)
			}
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
				Type:       "create_records_batch",
				Summary:    a.tr("ai.confirmInsert", len(preview), a.recordNoun(len(preview)), coll.Name),
				Detail:     string(detail),
				Collection: coll.Name,
				toolName:   "create_records_batch",
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
			if len(in.Records) > 200 {
				return "", fmt.Errorf("maximum 200 records per batch")
			}
			coll, err := a.findCollection(in.Collection)
			if err != nil {
				return "", err
			}
			created, err := execCreateRecords(a, coll, in.Records)
			if err != nil {
				return "", err
			}
			return a.tr("ai.insertedRecords", len(created), a.recordNoun(len(created)), coll.Name, strings.Join(created, ", ")), nil
		},
	}
}

// execCreateRecords is the shared per-record insert loop used by insert_records
// and create_records_batch.
func execCreateRecords(a *Agent, coll *core.Collection, records []map[string]any) ([]string, error) {
	var created []string
	for _, raw := range records {
		data, err := coerceRecordValues(coll, raw)
		if err != nil {
			return nil, err
		}
		if err := a.checkCreateRule(coll, data); err != nil {
			return nil, err
		}
		rec := core.NewRecord(coll)
		for k, v := range data {
			rec.Set(k, v)
		}
		if err := a.App.Save(rec); err != nil {
			return nil, fmt.Errorf("failed to save record in %q: %w", coll.Name, err)
		}
		created = append(created, rec.Id)
	}
	return created, nil
}

// --- run_action: execute a custom action (confirmed) ---

func runActionTool() tool {
	return tool{
		name: "run_action",
		description: "Executes a custom action (a saved JavaScript action targeting a collection). Requires explicit user confirmation. Args: {\"actionId\": \"action record id\", \"recordIds\": [\"optional record ids selected for the action\"]}. Public actions can be run by any signed-in user; non-public actions are superuser-only. When recordIds is omitted, the action runs on up to 200 readable records of the collection (respecting the list rule).",
		params: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"actionId":  map[string]any{"type": "string"},
				"recordIds": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"actionId"},
		},
		write: true,
		pending: func(a *Agent, args json.RawMessage) (*PendingAction, error) {
			in, def, err := parseRunAction(a, args)
			if err != nil {
				return nil, err
			}
			detail, _ := json.MarshalIndent(in, "", "  ")
			return &PendingAction{
				Type:     "run_action",
				Summary:  fmt.Sprintf("Run action %q", def.Name),
				Detail:   string(detail),
				toolName: "run_action",
				params:   mustMarshal(in),
			}, nil
		},
		exec: func(a *Agent, args json.RawMessage) (string, error) {
			in, def, err := parseRunAction(a, args)
			if err != nil {
				return "", err
			}
			recordIDs := in.RecordIDs
			if len(recordIDs) == 0 && def.Collection != "" {
				// auto-select up to 200 readable records when the LLM omits
				// recordIds — matches the common "run on all selected" UX.
				recs, selErr := a.App.FindRecordsByFilter(def.Collection, "", "", 200, 0)
				if selErr != nil {
					return "", fmt.Errorf("failed to select records: %w", selErr)
				}
				for _, r := range recs {
					if a.canAccessRecord(r) {
						recordIDs = append(recordIDs, r.Id)
					}
				}
			}
			result, runErr := pbactions.NewRunner(a.App, a.Info.Auth).Run(context.Background(), &def, recordIDs)
			if runErr != nil {
				return "", fmt.Errorf("action failed: %w", runErr)
			}
			if !result.OK {
				return "", fmt.Errorf("action failed: %s", result.Error)
			}
			var b strings.Builder
			fmt.Fprintf(&b, "Action %q completed.", def.Name)
			if result.Affected > 0 {
				fmt.Fprintf(&b, " Affected %d record(s).", result.Affected)
			}
			if len(result.Output) > 0 {
				fmt.Fprintf(&b, " Output:\n%s", strings.Join(result.Output, "\n"))
			}
			return b.String(), nil
		},
	}
}

// parseRunAction validates the run_action args, resolves the _actions record
// and enforces the public/superuser access rule. It returns the parsed args
// and the resolved ActionDef, mirroring main.go's actionDefToPbx mapping.
func parseRunAction(a *Agent, args json.RawMessage) (struct {
	ActionID  string   `json:"actionId"`
	RecordIDs []string `json:"recordIds"`
}, pbactions.ActionDef, error) {
	var in struct {
		ActionID  string   `json:"actionId"`
		RecordIDs []string `json:"recordIds"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return in, pbactions.ActionDef{}, err
	}
	if in.ActionID == "" {
		return in, pbactions.ActionDef{}, fmt.Errorf("actionId is required")
	}
	rec, err := a.App.FindRecordById("_actions", in.ActionID)
	if err != nil {
		return in, pbactions.ActionDef{}, fmt.Errorf("action not found: %w", err)
	}
	def := pbactions.ActionDef{
		ID:          rec.Id,
		Name:        rec.GetString("_name"),
		Description: rec.GetString("_description"),
		Script:      rec.GetString("_script"),
		Collection:  rec.GetString("_collection"),
		OnList:      rec.GetBool("_onList"),
		OnForm:      rec.GetBool("_onForm"),
		Public:      rec.GetBool("_public"),
	}
	if !def.Public && !a.isSuper() {
		return in, def, fmt.Errorf("you are not allowed to run this action (not public and you are not a superuser)")
	}
	return in, def, nil
}
