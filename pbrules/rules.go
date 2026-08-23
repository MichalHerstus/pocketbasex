package pbrules

import (
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/inflector"
	"github.com/pocketbase/pocketbase/tools/search"
	"github.com/pocketbase/pocketbase/tools/security"
)

// CheckCreateRuleContext holds the context needed for create rule evaluation
type CheckCreateRuleContext struct {
	App             core.App
	RequestInfo     *core.RequestInfo
	IsSuperuser     bool
}

// CheckCreateRule enforces the collection createRule for a non-superuser.
// It uses a dummy-record evaluation mirroring the PB create API.
func CheckCreateRule(ctx CheckCreateRuleContext, coll *core.Collection, data map[string]any) error {
	if ctx.IsSuperuser {
		return nil
	}
	if coll.CreateRule == nil {
		return fmt.Errorf("only superusers can create records in %q", coll.Name)
	}
	rule := *coll.CreateRule
	if rule == "" {
		return nil
	}

	// Validate collection name to prevent SQL injection (defense in depth)
	if !isValidCollectionName(coll.Name) {
		return fmt.Errorf("invalid collection name")
	}

	record := core.NewRecord(coll)
	for k, v := range data {
		record.Set(k, v)
	}
	if record.Id == "" {
		record.Id = "__pb_create__" + security.PseudorandomString(6)
	}
	record.SetVerified(false)

	dummyExport, err := record.DBExport(ctx.App)
	if err != nil {
		return fmt.Errorf("failed to evaluate create rule: %w", err)
	}
	dummyParams := make(dbx.Params, len(dummyExport))
	selects := make([]string, 0, len(dummyExport))
	for k, v := range dummyExport {
		k = inflector.Columnify(k)
		param := "__pb_create__" + k
		dummyParams[param] = v
		selects = append(selects, "{:"+param+"} AS [["+k+"]]")
	}

	dummyCollection := *coll
	dummyCollection.Id += "__pb_create__" + security.PseudorandomString(6)
	// Use a sanitized name for the dummy collection to prevent SQL injection
	dummyCollection.Name += inflector.Columnify("__pb_create__" + security.PseudorandomString(6))

	withFrom := fmt.Sprintf("WITH {{%s}} as (SELECT %s)", dummyCollection.Name, strings.Join(selects, ","))

	ruleQuery := ctx.App.ConcurrentDB().Select("(1)").PreFragment(withFrom).From(dummyCollection.Name).AndBind(dummyParams)
	resolver := core.NewRecordFieldResolver(ctx.App, &dummyCollection, ctx.RequestInfo, true)
	expr, err := search.FilterData(rule).BuildExpr(resolver)
	if err != nil {
		return fmt.Errorf("invalid create rule: %w", err)
	}
	ruleQuery.AndWhere(expr)
	if err := resolver.UpdateQuery(ruleQuery); err != nil {
		return fmt.Errorf("failed to evaluate create rule: %w", err)
	}

	var exists int
	if err := ruleQuery.Limit(1).Row(&exists); err != nil || exists == 0 {
		return fmt.Errorf("the create rule for %q forbids this record", coll.Name)
	}

	return nil
}

// isValidCollectionName validates that a collection name contains only safe characters
func isValidCollectionName(name string) bool {
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return len(name) > 0 && len(name) <= 64
}