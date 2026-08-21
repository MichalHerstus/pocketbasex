package pbactions

import (
	"encoding/json"
	"fmt"

	"github.com/dop251/goja"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/search"
	"github.com/pocketbase/pocketbase/tools/security"
)

// canAccessRecord reports whether the caller may read the given record under
// the collection's viewRule (superusers always pass, nil denies others).
func (r *Runner) canAccessRecord(rec *core.Record) bool {
	coll := rec.Collection()
	if r.isSuper() {
		return true
	}
	if coll.ViewRule == nil {
		return false
	}
	info := r.requestInfo()
	ok, err := r.App.CanAccessRecord(rec, info, coll.ViewRule)
	return err == nil && ok
}

// canList reports whether the caller may list the collection (listRule != nil
// or superuser).
func (r *Runner) canList(coll *core.Collection) bool {
	return r.isSuper() || coll.ListRule != nil
}

func (r *Runner) requestInfo() *core.RequestInfo {
	info := &core.RequestInfo{Context: "actions"}
	if r.AuthRecord != nil {
		info.Auth = r.AuthRecord
	}
	return info
}

// checkCreateRule enforces the collection createRule for a non-superuser.
// Mirrors pbai/main.checkCreateRule (kept decoupled).
func (r *Runner) checkCreateRule(coll *core.Collection, data map[string]any) error {
	if r.isSuper() {
		return nil
	}
	if coll.CreateRule == nil {
		return fmt.Errorf("only superusers can create records in %q", coll.Name)
	}
	rule := *coll.CreateRule
	if rule == "" {
		return nil
	}
	rec := core.NewRecord(coll)
	for k, v := range data {
		rec.Set(k, v)
	}
	if rec.Id == "" {
		rec.Id = "__pb_create__" + security.PseudorandomString(6)
	}
	rec.SetVerified(false)
	ok, err := r.App.CanAccessRecord(rec, r.requestInfo(), &rule)
	if err != nil {
		return fmt.Errorf("failed to evaluate create rule: %w", err)
	}
	if !ok {
		return fmt.Errorf("the create rule for %q forbids this record", coll.Name)
	}
	return nil
}

// registerBuiltins installs the action builtin functions onto the VM.
func (r *Runner) registerBuiltins(vm *goja.Runtime, records []map[string]any, res *ActionResult) error {
	vm.Set("log", func(call goja.FunctionCall) goja.Value {
		parts := make([]string, 0, len(call.Arguments))
		for _, a := range call.Arguments {
			parts = append(parts, a.String())
		}
		line := ""
		for i, p := range parts {
			if i > 0 {
				line += " "
			}
			line += p
		}
		res.Output = append(res.Output, line)
		return goja.Undefined()
	})

	vm.Set("currentUser", func(call goja.FunctionCall) goja.Value {
		if r.AuthRecord == nil {
			return goja.Null()
		}
		name := r.AuthRecord.GetString("name")
		email := r.AuthRecord.GetString("email")
		m := map[string]any{
			"id":    r.AuthRecord.Id,
			"name":  name,
			"email": email,
		}
		return toValue(vm, m)
	})

	vm.Set("currentRecord", func(call goja.FunctionCall) goja.Value {
		if len(records) == 0 {
			return goja.Null()
		}
		return toValue(vm, records[0])
	})
	vm.Set("selectedRecords", func(call goja.FunctionCall) goja.Value {
		return toValue(vm, records)
	})

	// select(coll, filter, sort?, limit?) -> []map
	vm.Set("select", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panicVM(vm, "select: collection name required")
		}
		collName := call.Argument(0).String()
		filter := ""
		if len(call.Arguments) >= 2 {
			filter = call.Argument(1).String()
		}
		sort := ""
		if len(call.Arguments) >= 3 {
			sort = call.Argument(2).String()
		}
		limit := 20
		if len(call.Arguments) >= 4 {
			if n := toInt(call.Argument(3)); n > 0 {
				limit = n
			}
		}
		coll, err := r.App.FindCachedCollectionByNameOrId(collName)
		if err != nil {
			panicVM(vm, "select: collection %q not found", collName)
		}
		if !r.canList(coll) {
			panicVM(vm, "select: no permission to list %q", coll.Name)
		}
		var recs []*core.Record
		if filter == "" {
			recs, err = r.App.FindRecordsByFilter(coll, "", sort, limit, 0)
		} else {
			recs, err = r.App.FindRecordsByFilter(coll, filter, sort, limit, 0)
		}
		if err != nil {
			panicVM(vm, "select: %v", err)
		}
		out := make([]map[string]any, 0, len(recs))
		for _, rec := range recs {
			if r.canAccessRecord(rec) {
				out = append(out, rec.PublicExport())
			}
		}
		return toValue(vm, out)
	})

	// get(coll, id) -> map
	vm.Set("get", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panicVM(vm, "get: collection name and id required")
		}
		collName := call.Argument(0).String()
		id := call.Argument(1).String()
		coll, err := r.App.FindCachedCollectionByNameOrId(collName)
		if err != nil {
			panicVM(vm, "get: collection %q not found", collName)
		}
		rec, err := r.App.FindRecordById(coll, id)
		if err != nil {
			return goja.Null()
		}
		if !r.canAccessRecord(rec) {
			return goja.Null()
		}
		return toValue(vm, rec.PublicExport())
	})

	// count(coll, filter?) -> number
	vm.Set("count", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panicVM(vm, "count: collection name required")
		}
		collName := call.Argument(0).String()
		filter := ""
		if len(call.Arguments) >= 2 {
			filter = call.Argument(1).String()
		}
		coll, err := r.App.FindCachedCollectionByNameOrId(collName)
		if err != nil {
			panicVM(vm, "count: collection %q not found", collName)
		}
		if !r.canList(coll) {
			panicVM(vm, "count: no permission to list %q", coll.Name)
		}
		total, err := r.countRecords(coll, filter)
		if err != nil {
			panicVM(vm, "count: %v", err)
		}
		return vm.ToValue(total)
	})

	// insert(coll, data) -> string (new record id)
	vm.Set("insert", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panicVM(vm, "insert: collection name and data required")
		}
		collName := call.Argument(0).String()
		data := toMap(vm, call.Argument(1))
		coll, err := r.App.FindCachedCollectionByNameOrId(collName)
		if err != nil {
			panicVM(vm, "insert: collection %q not found", collName)
		}
		if err := r.checkCreateRule(coll, data); err != nil {
			panicVM(vm, "insert: %v", err)
		}
		rec := core.NewRecord(coll)
		for k, v := range data {
			rec.Set(k, v)
		}
		if err := r.App.Save(rec); err != nil {
			panicVM(vm, "insert: %v", err)
		}
		r.Affected++
		return vm.ToValue(rec.Id)
	})

	// update(coll, id, data) -> bool
	vm.Set("update", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			panicVM(vm, "update: collection name, id and data required")
		}
		collName := call.Argument(0).String()
		id := call.Argument(1).String()
		data := toMap(vm, call.Argument(2))
		coll, err := r.App.FindCachedCollectionByNameOrId(collName)
		if err != nil {
			panicVM(vm, "update: collection %q not found", collName)
		}
		rec, err := r.App.FindRecordById(coll, id)
		if err != nil {
			return vm.ToValue(false)
		}
		if !r.canUpdateRecord(rec) {
			panicVM(vm, "update: no permission to update %q", collName)
		}
		for k, v := range data {
			rec.Set(k, v)
		}
		if err := r.App.Save(rec); err != nil {
			panicVM(vm, "update: %v", err)
		}
		r.Affected++
		return vm.ToValue(true)
	})

	// delete(coll, id) -> bool
	vm.Set("delete", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panicVM(vm, "delete: collection name and id required")
		}
		collName := call.Argument(0).String()
		id := call.Argument(1).String()
		coll, err := r.App.FindCachedCollectionByNameOrId(collName)
		if err != nil {
			panicVM(vm, "delete: collection %q not found", collName)
		}
		rec, err := r.App.FindRecordById(coll, id)
		if err != nil {
			return vm.ToValue(false)
		}
		if !r.canDeleteRecord(rec) {
			panicVM(vm, "delete: no permission to delete %q", collName)
		}
		if err := r.App.Delete(rec); err != nil {
			panicVM(vm, "delete: %v", err)
		}
		r.Affected++
		return vm.ToValue(true)
	})

	// currentUser already set above; record list captured in closures
	return nil
}

// canUpdateRecord / canDeleteRecord enforce updateRule/deleteRule per record.
func (r *Runner) canUpdateRecord(rec *core.Record) bool {
	if r.isSuper() {
		return true
	}
	if rec.Collection().UpdateRule == nil {
		return false
	}
	ok, err := r.App.CanAccessRecord(rec, r.requestInfo(), rec.Collection().UpdateRule)
	return err == nil && ok
}

func (r *Runner) canDeleteRecord(rec *core.Record) bool {
	if r.isSuper() {
		return true
	}
	if rec.Collection().DeleteRule == nil {
		return false
	}
	ok, err := r.App.CanAccessRecord(rec, r.requestInfo(), rec.Collection().DeleteRule)
	return err == nil && ok
}

// countRecords returns the number of records in a collection matching a PB
// filter. It builds a COUNT query using the same filter resolver PB uses.
func (r *Runner) countRecords(coll *core.Collection, filter string) (int64, error) {
	query := r.App.ConcurrentDB().Select("COUNT(*)").From(coll.Name)
	if filter != "" {
		resolver := core.NewRecordFieldResolver(r.App, coll, r.requestInfo(), true)
		expr, err := search.FilterData(filter).BuildExpr(resolver)
		if err != nil {
			return 0, err
		}
		query.AndWhere(expr)
		if err := resolver.UpdateQuery(query); err != nil {
			return 0, err
		}
	}
	var total int64
	if err := query.Row(&total); err != nil {
		return 0, err
	}
	return total, nil
}

// --- helpers ---

func toValue(vm *goja.Runtime, v any) goja.Value {
	b, err := json.Marshal(v)
	if err != nil {
		return goja.Undefined()
	}
	var out any
	_ = json.Unmarshal(b, &out)
	return vm.ToValue(out)
}

func toMap(vm *goja.Runtime, v goja.Value) map[string]any {
	obj := v.ToObject(vm)
	m := make(map[string]any)
	for _, k := range obj.Keys() {
		m[k] = obj.Get(k).Export()
	}
	return m
}

func toInt(v goja.Value) int {
	switch t := v.Export().(type) {
	case float64:
		return int(t)
	case int64:
		return int(t)
	case int:
		return t
	}
	return 0
}

func panicVM(vm *goja.Runtime, format string, args ...any) {
	panic(vm.ToValue(fmt.Sprintf(format, args...)))
}
