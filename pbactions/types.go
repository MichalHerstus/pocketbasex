// Package pbactions implements the custom-action execution engine for PBX.
//
// Actions are scripts (Goja / JavaScript) stored in the _actions collection.
// Each action targets a collection and can run from the tabular (list) or form
// view. Scripts execute as the current user with PocketBase collection rules
// enforced at the Go level (list/view/create/update/delete), synchronously, with
// a 10 second timeout and bounded loop iterations.
package pbactions

// ActionDef is a single action loaded from the _actions collection.
type ActionDef struct {
	ID          string
	Name        string
	Description string
	Script      string
	Collection  string
	OnList      bool
	OnForm      bool
	Public      bool
}

// ActionResult is returned by Run and surfaced to the UI (result modal).
type ActionResult struct {
	OK       bool     `json:"ok"`
	Output   []string `json:"output"`            // log() calls
	Affected int      `json:"affected"`          // total inserts+updates+deletes
	Error    string   `json:"error,omitempty"`
}
