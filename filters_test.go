package main

import (
	"strings"
	"testing"

	"pbx/views"
)

func TestValidateFilterDef(t *testing.T) {
	cases := []struct {
		name     string
		def      views.FilterDef
		wantErr  bool
		errPiece string
	}{
		{
			name:    "valid single",
			def:     views.FilterDef{Name: "Pricy", Conditions: []views.FilterCondition{{Field: "price", Op: ">", Value: "1000"}}},
			wantErr: false,
		},
		{
			name:    "valid with params and chains",
			def:     views.FilterDef{Name: "Range", Conditions: []views.FilterCondition{{Field: "price", Op: ">", Value: "?"}, {Field: "price", Op: "<", Value: "?"}}, Chains: []string{"and"}},
			wantErr: false,
		},
		{
			name:     "missing name",
			def:      views.FilterDef{Conditions: []views.FilterCondition{{Field: "price", Op: ">"}}},
			wantErr:  true,
			errPiece: "name",
		},
		{
			name:     "no conditions",
			def:      views.FilterDef{Name: "empty"},
			wantErr:  true,
			errPiece: "least one",
		},
		{
			name:     "missing field",
			def:      views.FilterDef{Name: "x", Conditions: []views.FilterCondition{{Op: ">", Value: "1"}}},
			wantErr:  true,
			errPiece: "field",
		},
		{
			name:     "missing op",
			def:      views.FilterDef{Name: "x", Conditions: []views.FilterCondition{{Field: "price", Value: "1"}}},
			wantErr:  true,
			errPiece: "operator",
		},
		{
			name:     "connector mismatch",
			def:      views.FilterDef{Name: "x", Conditions: []views.FilterCondition{{Field: "a", Op: "="}, {Field: "b", Op: "="}}, Chains: []string{"and", "or"}},
			wantErr:  true,
			errPiece: "connectors",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFilterDef(tc.def)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errPiece != "" && !strings.Contains(err.Error(), tc.errPiece) {
					t.Fatalf("error %q does not mention %q", err.Error(), tc.errPiece)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}