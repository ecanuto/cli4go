package cli

import (
	"testing"
)

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
		{input: "---bar", want: "--bar"}, // documented quirk: kept from the old test
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

// TestValidateOptions exercises validateOptions directly, against a bare
// []Option, without needing a full Command wrapper.
func TestValidateOptions(t *testing.T) {
	var s string

	tests := []struct {
		name    string
		opts    []Option
		wantErr bool
	}{
		{
			name: "no options",
			opts: nil,
		},
		{
			name: "valid, option with Flag only",
			opts: []Option{&StringOption{Flag: "f"}},
		},
		{
			name: "valid, option with Long only",
			opts: []Option{&StringOption{Long: "foo"}},
		},
		{
			name: "valid, option with Flag and Long",
			opts: []Option{&StringOption{Flag: "f", Long: "foo"}},
		},
		{
			name: "valid, option with Action instead of Dest",
			opts: []Option{&StringOption{Flag: "f", Action: func(*Command, Option, string) *Error { return nil }}},
		},
		{
			name: "valid, option with both Dest and Action",
			opts: []Option{&StringOption{Flag: "f", Dest: &s, Action: func(*Command, Option, string) *Error { return nil }}},
		},
		{
			name:    "option missing both Flag and Long",
			opts:    []Option{&StringOption{}},
			wantErr: true,
		},
		{
			name:    "option Flag longer than one character",
			opts:    []Option{&StringOption{Flag: "-ab"}},
			wantErr: true,
		},
		{
			name:    "option Flag with only dashes",
			opts:    []Option{&StringOption{Flag: "--"}},
			wantErr: true,
		},
		{
			name: "duplicate option flag, same spelling",
			opts: []Option{
				&StringOption{Flag: "f"},
				&StringOption{Flag: "f"},
			},
			wantErr: true,
		},
		{
			name: "duplicate option flag, different spelling normalizes the same",
			opts: []Option{
				&StringOption{Flag: "f"},
				&StringOption{Flag: "-f"},
			},
			wantErr: true,
		},
		{
			name: "duplicate option long, different spelling normalizes the same",
			opts: []Option{
				&StringOption{Long: "bar"},
				&StringOption{Long: "--bar"},
			},
			wantErr: true,
		},
		{
			name: "distinct derived names are fine",
			opts: []Option{
				&StringOption{Flag: "-f", Long: "--foo"},
				&StringOption{Flag: "-b"},
			},
		},
		{
			name: "duplicate explicit Name",
			opts: []Option{
				&StringOption{Name: "x", Flag: "-a"},
				&StringOption{Name: "x", Flag: "-b"},
			},
			wantErr: true,
		},
		{
			name: "derived name collides: Flag -f and Long --f",
			opts: []Option{
				&StringOption{Flag: "-f"},
				&StringOption{Long: "--f"},
			},
			wantErr: true,
		},
		{
			name: "explicit Name collides with another Option's derived name",
			opts: []Option{
				&StringOption{Name: "bar", Flag: "-a"},
				&StringOption{Long: "--bar"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOptions(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
