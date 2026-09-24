package cli

import (
	"testing"
)

// --- IntArgument -------------------------------------------------------------

// TestIntArgument_Accessors covers IntArgument.help(), an accessor the
// rest of the suite happens not to reach.
func TestIntArgument_Accessors(t *testing.T) {
	a := &IntArgument{Name: "count", Help: "How many"}
	if got := a.help(); got != "How many" {
		t.Errorf("help() = %q, want %q", got, "How many")
	}
}

// --- Validate ----------------------------------------------------------------

func TestArgument_Validate(t *testing.T) {
	tests := []struct {
		name    string
		arg     Argument
		wantErr bool
	}{
		{name: "StringArgument valid", arg: &StringArgument{Name: "qux"}},
		{name: "StringArgument empty Name", arg: &StringArgument{}, wantErr: true},
		{name: "IntArgument valid", arg: &IntArgument{Name: "count"}},
		{name: "IntArgument empty Name", arg: &IntArgument{}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.arg.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
