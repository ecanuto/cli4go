package cli

import (
	"testing"
)

// customValue is a minimal Value implementation used to verify that
// validateDefaultType skips anything other than the three built-in
// concrete types (stringValue/boolValue/intValue) it knows how to check.
type customValue struct{}

func (customValue) String() string   { return "" }
func (customValue) Set(string) error { return nil }

func TestValidateDefaultType(t *testing.T) {
	var s string
	var b bool
	var i int

	tests := []struct {
		name    string
		dest    Value
		def     any
		wantErr bool
	}{
		{name: "nil dest, nil default", dest: nil, def: nil},
		{name: "nil dest, non-nil default", dest: nil, def: "foo"},
		{name: "StringVar, nil default", dest: StringVar(&s), def: nil},
		{name: "StringVar, matching default", dest: StringVar(&s), def: "foo"},
		{name: "StringVar, mismatched default", dest: StringVar(&s), def: 42, wantErr: true},
		{name: "BoolVar, matching default", dest: BoolVar(&b), def: true},
		{name: "BoolVar, mismatched default", dest: BoolVar(&b), def: "true", wantErr: true},
		{name: "IntVar, matching default", dest: IntVar(&i), def: 7},
		{name: "IntVar, mismatched default", dest: IntVar(&i), def: "7", wantErr: true},
		{name: "custom Value implementation, any default", dest: customValue{}, def: 123},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDefaultType(tt.dest, tt.def)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDefaultType(%v, %v) error = %v, wantErr %v", tt.dest, tt.def, err, tt.wantErr)
			}
		})
	}
}
