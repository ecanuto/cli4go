package cli

import (
	"testing"
)

// TestIntArgument_Accessors covers IntArgument.help(), an accessor the
// rest of the suite happens not to reach.
func TestIntArgument_Accessors(t *testing.T) {
	a := &IntArgument{Name: "count", Help: "How many"}
	if got := a.help(); got != "How many" {
		t.Errorf("help() = %q, want %q", got, "How many")
	}
}

// TestValidateArguments exercises validateArguments directly, against a
// bare []Argument, without needing a full Command wrapper.
func TestValidateArguments(t *testing.T) {
	var s string

	tests := []struct {
		name    string
		args    []Argument
		wantErr bool
	}{
		{
			name: "no arguments",
			args: nil,
		},
		{
			name: "valid, one argument",
			args: []Argument{&StringArgument{Name: "qux", Dest: &s}},
		},
		{
			name:    "empty Argument.Name",
			args:    []Argument{&StringArgument{Name: ""}},
			wantErr: true,
		},
		{
			name: "duplicate Argument.Name",
			args: []Argument{
				&StringArgument{Name: "qux"},
				&StringArgument{Name: "qux"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateArguments(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateArguments() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
