package cli

import (
	"testing"
)

// --- Option ------------------------------------------------------------------

func TestOptionFlag(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "", want: ""},
		{input: "f", want: "-f"},
		{input: "-f", want: "-f"},
		{input: "--f", want: "-f"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := (&StringOption{Flag: tt.input}).flag(); got != tt.want {
				t.Errorf("Option{Flag: %q}.flag() = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestOptionLong(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "", want: ""},
		{input: "bar", want: "--bar"},
		{input: "-bar", want: "--bar"},
		{input: "--bar", want: "--bar"},
		{input: "---bar", want: "--bar"}, // extra leading dashes are all stripped
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := (&StringOption{Long: tt.input}).long(); got != tt.want {
				t.Errorf("Option{Long: %q}.long() = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestOptionName covers the name() fallback shared by every Option
// type: Name, else Long without "--", else Flag without "-".
func TestOptionName(t *testing.T) {
	tests := []struct {
		name string
		opt  Option
		want string
	}{
		{name: "Name wins", opt: &StringOption{Name: "cfg", Flag: "-c", Long: "--config"}, want: "cfg"},
		{name: "Long when no Name", opt: &StringOption{Flag: "-c", Long: "--config"}, want: "config"},
		{name: "Long spelled without dashes", opt: &IntOption{Long: "config"}, want: "config"},
		{name: "Flag when only Flag", opt: &BoolOption{Flag: "-v"}, want: "v"},
		{name: "Flag spelled without dash", opt: &BoolOption{Flag: "v"}, want: "v"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opt.name(); got != tt.want {
				t.Errorf("name() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- IntOption ---------------------------------------------------------------

// TestIntOption_Accessors covers the IntOption accessors the rest of the
// suite happens not to reach: Parse/Validate/HelpString already exercise
// name()/flag()/long() (via the various "-n" test cases), but none of
// them touch help() or action() when unset.
func TestIntOption_Accessors(t *testing.T) {
	o := &IntOption{Name: "count", Flag: "-n", Help: "How many"}
	if got := o.name(); got != "count" {
		t.Errorf("name() = %q, want %q", got, "count")
	}
	if got := o.help(); got != "How many" {
		t.Errorf("help() = %q, want %q", got, "How many")
	}
	if got := o.action(); got != nil {
		t.Errorf("action() = %v, want nil, unset", got)
	}
}

// --- Validate ----------------------------------------------------------------

// TestOption_Validate covers the per-item rules of every Option type.
func TestOption_Validate(t *testing.T) {
	tests := []struct {
		name    string
		opt     Option
		wantErr bool
	}{
		{name: "StringOption Flag only", opt: &StringOption{Flag: "f"}},
		{name: "StringOption Long only", opt: &StringOption{Long: "foo"}},
		{name: "StringOption missing Flag and Long", opt: &StringOption{}, wantErr: true},
		{name: "StringOption Flag too long", opt: &StringOption{Flag: "-ab"}, wantErr: true},
		{name: "IntOption Flag and Long", opt: &IntOption{Flag: "n", Long: "num"}},
		{name: "IntOption missing Flag and Long", opt: &IntOption{}, wantErr: true},
		{name: "IntOption Flag with only dashes", opt: &IntOption{Flag: "--"}, wantErr: true},
		{name: "BoolOption Long only", opt: &BoolOption{Long: "verbose"}},
		{name: "BoolOption missing Flag and Long", opt: &BoolOption{}, wantErr: true},
		{name: "BoolOption Flag too long", opt: &BoolOption{Flag: "ab"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opt.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
