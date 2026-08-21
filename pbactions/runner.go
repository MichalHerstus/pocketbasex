package pbactions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dop251/goja"
	"github.com/pocketbase/pocketbase/core"
)

// timeLimit bounds a single action execution (Goja is single-threaded, so a
// long-running script blocks; Interrupt() is called after this expires).
var timeLimit = 10 * time.Second

// ErrTimeout is returned when an action exceeds timeLimit.
var ErrTimeout = errors.New("action timed out (10s limit)")

// Runner executes actions in a fresh Goja VM per Run call. It carries the
// PocketBase app handle and the authenticated caller so every builtin enforces
// the same collection rules as the regular API.
type Runner struct {
	App        core.App
	AuthRecord *core.Record

	// Affected counts every write operation performed by an action.
	Affected int
}

// NewRunner creates a Runner bound to a caller (AuthRecord may be nil for an
// anonymous caller; rule checks handle nil).
func NewRunner(app core.App, authRecord *core.Record) *Runner {
	return &Runner{App: app, AuthRecord: authRecord}
}

func (r *Runner) isSuper() bool {
	return r.AuthRecord != nil && r.AuthRecord.Collection().Name == "_superusers"
}

// Run executes an action script with the given selected record ids. If
// recordIDs is empty, an empty record/records array is injected. It returns
// the accumulated result (output + affected count), or an error when a fatal
// setup problem occurs (script errors are folded into ok=false result).
func (r *Runner) Run(ctx context.Context, action *ActionDef, recordIDs []string) (ActionResult, error) {
	vm := goja.New()
	vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))

	res := ActionResult{OK: true}

	records := r.loadRecords(action.Collection, recordIDs)

	// register builtins
	if err := r.registerBuiltins(vm, records, &res); err != nil {
		return res, err
	}

	// inject current record(s)
	vm.Set("records", records)
	if len(records) > 0 {
		vm.Set("record", records[0])
	} else {
		vm.Set("record", goja.Null())
	}

	// timeout: interrupt the VM after timeLimit unless it finishes first
	interrupted := false
	timer := time.AfterFunc(timeLimit, func() {
		interrupted = true
		vm.Interrupt(ErrTimeout)
	})
	defer timer.Stop()

	program, err := goja.Compile("action.js", action.Script, false)
	if err != nil {
		return res, fmt.Errorf("script compile error: %w", err)
	}

	_, callErr := vm.RunProgram(program)
	if callErr != nil {
		if interrupted {
			return res, ErrTimeout
		}
		// any generic (goja) error -> wrap
		if gojaErr, ok := callErr.(*goja.Exception); ok {
			res.OK = false
			res.Error = gojaErr.String()
			return res, nil
		}
		return res, callErr
	}

	res.Affected = r.Affected
	return res, nil
}

// loadRecords fetches the selected records (as public-export maps), enforced
// through the collection's view rule.
func (r *Runner) loadRecords(collName string, recordIDs []string) []map[string]any {
	records := make([]map[string]any, 0, len(recordIDs))
	for _, id := range recordIDs {
		rec, err := r.App.FindRecordById(collName, id)
		if err != nil {
			continue
		}
		if !r.canAccessRecord(rec) {
			continue
		}
		records = append(records, rec.PublicExport())
	}
	return records
}
