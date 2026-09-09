package pbai

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	search "github.com/pocketbase/pocketbase/tools/search"
)

// Suggestion is a collection the signed-in user may browse together with the
// number of records they can list. Returned by the /api/ai/suggestions
// endpoint to seed the chat welcome screen.
type Suggestion struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// SuggestCollections returns the top 5 listable user collections by record
// count (listRule-aware for non-superusers). Internal/view collections are
// skipped and empty collections are omitted.
func SuggestCollections(app core.App, info *core.RequestInfo) ([]Suggestion, error) {
	colls, err := app.FindAllCollections()
	if err != nil {
		return nil, err
	}
	var out []Suggestion
	for _, c := range colls {
		if c.IsView() || strings.HasPrefix(c.Name, "_") {
			continue
		}
		n, cerr := countListableRecords(app, c, info)
		if cerr != nil || n == 0 {
			continue
		}
		out = append(out, Suggestion{Name: c.Name, Count: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	if len(out) > 5 {
		out = out[:5]
	}
	return out, nil
}

// countListableRecords counts the records of coll reachable by the caller:
// all records for superusers, all records for public collections (empty rule)
// and a listRule-filtered count otherwise. It returns 0 when the caller has
// no list permission (nil rule) or the rule excludes everything.
func countListableRecords(app core.App, coll *core.Collection, info *core.RequestInfo) (int64, error) {
	if info.HasSuperuserAuth() {
		return app.CountRecords(coll, nil)
	}
	if coll.ListRule == nil {
		return 0, nil
	}
	if strings.TrimSpace(*coll.ListRule) == "" {
		return app.CountRecords(coll, nil)
	}
	q := app.RecordQuery(coll)
	resolver := core.NewRecordFieldResolver(app, coll, info, true)
	expr, err := search.FilterData(*coll.ListRule).BuildExpr(resolver, dbx.Params{})
	if err != nil {
		return 0, fmt.Errorf("invalid list rule for %q: %w", coll.Name, err)
	}
	q.AndWhere(expr)
	if err := resolver.UpdateQuery(q); err != nil {
		return 0, err
	}
	q.Select("count(*)")
	var n int64
	if err := q.Row(&n); err != nil {
		return 0, err
	}
	return n, nil
}