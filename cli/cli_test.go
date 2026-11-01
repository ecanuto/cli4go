package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
)

// TestSet covers every typed Argument/Option's set: it converts the
// string into Dest's Go type, returns the conversion error unchanged
// for a bad one (leaving Dest as it was), and keeps the value in the
// item itself (GetValue) whether or not Dest is set.
func TestSet(t *testing.T) {
	t.Run("stores into Dest", func(t *testing.T) {
		var s1, s2 string
		var i1, i2 int
		var b bool
		tests := []struct {
			name string
			set  func(string) error
			in   string
			ok   func() bool
		}{
			{"StringArgument", (&StringArgument{Dest: &s1}).Set, "x", func() bool { return s1 == "x" }},
			{"StringOption", (&StringOption{Flag: "-s", Dest: &s2}).Set, "y", func() bool { return s2 == "y" }},
			{"IntArgument", (&IntArgument{Dest: &i1}).Set, "42", func() bool { return i1 == 42 }},
			{"IntOption", (&IntOption{Flag: "-i", Dest: &i2}).Set, "-7", func() bool { return i2 == -7 }},
			{"BoolOption", (&BoolOption{Flag: "-b", Dest: &b}).Set, "true", func() bool { return b }},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if err := tt.set(tt.in); err != nil {
					t.Fatalf("set(%q) = %v, want nil", tt.in, err)
				}
				if !tt.ok() {
					t.Errorf("Dest not updated for input %q", tt.in)
				}
			})
		}
	})

	t.Run("a conversion error is returned and Dest is left alone", func(t *testing.T) {
		i, b := 5, true
		tests := []struct {
			name string
			set  func(string) error
			want string
		}{
			{"IntArgument", (&IntArgument{Dest: &i}).Set, "invalid integer value"},
			{"IntOption", (&IntOption{Flag: "-i", Dest: &i}).Set, "invalid integer value"},
			{"BoolOption", (&BoolOption{Flag: "-b", Dest: &b}).Set, "invalid boolean value"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := tt.set("nope")
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Errorf("set(%q) = %v, want an error containing %q", "nope", err, tt.want)
				}
				if i != 5 || !b {
					t.Errorf("Dest changed on error: i=%d, b=%v", i, b)
				}
			})
		}
	})

	t.Run("a nil Dest still stores the value in the item", func(t *testing.T) {
		tests := []struct {
			name string
			it   item
			in   string
			want any
		}{
			{"StringArgument", &StringArgument{}, "x", "x"},
			{"StringOption", &StringOption{Flag: "-s"}, "y", "y"},
			{"IntArgument", &IntArgument{}, "42", 42},
			{"IntOption", &IntOption{Flag: "-i"}, "-7", -7},
			{"BoolOption", &BoolOption{Flag: "-b"}, "true", true},
			{"StringEnvVar", &StringEnvVar{Name: "APP_X"}, "z", "z"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if err := tt.it.Set(tt.in); err != nil {
					t.Fatalf("Set(%q) = %v, want nil", tt.in, err)
				}
				if got := tt.it.GetValue(); got != tt.want {
					t.Errorf("GetValue() = %#v, want %#v", got, tt.want)
				}
			})
		}
	})

	t.Run("a nil Dest still reports a conversion error", func(t *testing.T) {
		for name, it := range map[string]item{
			"IntArgument": &IntArgument{},
			"IntOption":   &IntOption{Flag: "-i"},
			"BoolOption":  &BoolOption{Flag: "-b"},
		} {
			t.Run(name, func(t *testing.T) {
				if err := it.Set("nope"); err == nil {
					t.Error("Set(\"nope\") = nil, want a conversion error")
				}
			})
		}
	})
}

func TestStdout(t *testing.T) {
	tests := []struct {
		input io.Writer
		want  io.Writer
	}{
		{input: nil, want: os.Stdout},
		{input: os.Stdout, want: os.Stdout},
		{input: os.Stderr, want: os.Stderr},
	}

	for _, tt := range tests {
		c := &Command{Stdout: tt.input}
		if got, want := c.stdout(), tt.want; got != want {
			t.Errorf("c.stdout() = %v, want %v", got, want)
		}
	}
}

func TestStderr(t *testing.T) {
	tests := []struct {
		input io.Writer
		want  io.Writer
	}{
		{input: nil, want: os.Stderr},
		{input: os.Stderr, want: os.Stderr},
		{input: os.Stdout, want: os.Stdout},
	}

	for _, tt := range tests {
		c := &Command{Stderr: tt.input}
		if got, want := c.stderr(), tt.want; got != want {
			t.Errorf("c.stderr() = %v, want %v", got, want)
		}
	}
}

func optionSlicesEqual(a, b []Option) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].GetName() != b[i].GetName() ||
			a[i].GetFlag() != b[i].GetFlag() ||
			a[i].GetLong() != b[i].GetLong() ||
			a[i].GetHelp() != b[i].GetHelp() ||
			a[i].GetEnv() != b[i].GetEnv() ||
			(a[i].GetAction() == nil) != (b[i].GetAction() == nil) {
			return false
		}
	}
	return true
}

func TestOptions(t *testing.T) {
	// A placeholder Action, used only to mark a "want" Option as
	// carrying one -- optionSlicesEqual compares Action by nil-ness,
	// not identity, so any non-nil func here works.
	anyAction := func(*Command, Option, string) *Error { return nil }
	var discard bool
	anyDest := &discard

	tests := []struct {
		name string
		cmd  Command
		want []Option
	}{
		{
			name: "no user options, no version",
			cmd:  Command{Name: "myprogram"},
			want: []Option{
				&BoolOption{Flag: "-h", Long: "--help", Help: "Print help", Dest: anyDest, Action: anyAction},
			},
		},
		{
			name: "no user options, with version",
			cmd:  Command{Name: "myprogram", Version: "0.1.0"},
			want: []Option{
				&BoolOption{Flag: "-h", Long: "--help", Help: "Print help", Dest: anyDest, Action: anyAction},
				&BoolOption{Flag: "-v", Long: "--version", Help: "Print version", Dest: anyDest, Action: anyAction},
			},
		},
		{
			name: "user options first, built-ins last",
			cmd: Command{
				Name:    "myprogram",
				Version: "0.1.0",
				Options: []Option{
					&StringOption{Flag: "-f", Help: "Enable foo mode"},
					&StringOption{Long: "--bar", Help: "Enable bar mode"},
				},
			},
			want: []Option{
				&StringOption{Flag: "-f", Help: "Enable foo mode"},
				&StringOption{Long: "--bar", Help: "Enable bar mode"},
				&BoolOption{Flag: "-h", Long: "--help", Help: "Print help", Dest: anyDest, Action: anyAction},
				&BoolOption{Flag: "-v", Long: "--version", Help: "Print version", Dest: anyDest, Action: anyAction},
			},
		},
		{
			name: "user claims both help forms: help built-in suppressed",
			cmd: Command{
				Name: "myprogram",
				Options: []Option{
					&StringOption{Flag: "-h", Long: "--help", Help: "Custom help"},
				},
			},
			want: []Option{
				&StringOption{Flag: "-h", Long: "--help", Help: "Custom help"},
			},
		},
		{
			name: "user claims both version forms: version built-in suppressed",
			cmd: Command{
				Name:    "myprogram",
				Version: "0.1.0",
				Options: []Option{
					&StringOption{Flag: "-v", Long: "--version", Help: "Custom version"},
				},
			},
			want: []Option{
				&StringOption{Flag: "-v", Long: "--version", Help: "Custom version"},
				&BoolOption{Flag: "-h", Long: "--help", Help: "Print help", Dest: anyDest, Action: anyAction},
			},
		},
		{
			name: "user claims both built-ins: only user Options yielded",
			cmd: Command{
				Name:    "myprogram",
				Version: "0.1.0",
				Options: []Option{
					&StringOption{Flag: "-h", Long: "--help", Help: "Custom help"},
					&StringOption{Flag: "-v", Long: "--version", Help: "Custom version"},
				},
			},
			want: []Option{
				&StringOption{Flag: "-h", Long: "--help", Help: "Custom help"},
				&StringOption{Flag: "-v", Long: "--version", Help: "Custom version"},
			},
		},
		{
			name: "user claims both version forms but Version is empty: nothing suppressed",
			cmd: Command{
				Name: "myprogram",
				Options: []Option{
					&StringOption{Flag: "-v", Long: "--version", Help: "Custom version"},
				},
			},
			want: []Option{
				&StringOption{Flag: "-v", Long: "--version", Help: "Custom version"},
				&BoolOption{Flag: "-h", Long: "--help", Help: "Print help", Dest: anyDest, Action: anyAction},
			},
		},
		{
			name: "user claims -v only: version built-in falls back to -V",
			cmd: Command{
				Name:    "myprogram",
				Version: "0.1.0",
				Options: []Option{
					&StringOption{Flag: "-v", Long: "--verbose", Help: "Verbose"},
				},
			},
			want: []Option{
				&StringOption{Flag: "-v", Long: "--verbose", Help: "Verbose"},
				&BoolOption{Flag: "-h", Long: "--help", Help: "Print help", Dest: anyDest, Action: anyAction},
				&BoolOption{Flag: "-V", Long: "--version", Help: "Print version", Dest: anyDest, Action: anyAction},
			},
		},
		{
			name: "user claims -v and -V: version built-in keeps only --version",
			cmd: Command{
				Name:    "myprogram",
				Version: "0.1.0",
				Options: []Option{
					&StringOption{Flag: "-v", Long: "--verbose", Help: "Verbose"},
					&StringOption{Flag: "-V", Long: "--vv", Help: "Very verbose"},
				},
			},
			want: []Option{
				&StringOption{Flag: "-v", Long: "--verbose", Help: "Verbose"},
				&StringOption{Flag: "-V", Long: "--vv", Help: "Very verbose"},
				&BoolOption{Flag: "-h", Long: "--help", Help: "Print help", Dest: anyDest, Action: anyAction},
				&BoolOption{Long: "--version", Help: "Print version", Dest: anyDest, Action: anyAction},
			},
		},
		{
			name: "user claims -V only: version built-in keeps -v",
			cmd: Command{
				Name:    "myprogram",
				Version: "0.1.0",
				Options: []Option{
					&StringOption{Flag: "-V", Long: "--vv", Help: "Very verbose"},
				},
			},
			want: []Option{
				&StringOption{Flag: "-V", Long: "--vv", Help: "Very verbose"},
				&BoolOption{Flag: "-h", Long: "--help", Help: "Print help", Dest: anyDest, Action: anyAction},
				&BoolOption{Flag: "-v", Long: "--version", Help: "Print version", Dest: anyDest, Action: anyAction},
			},
		},
		{
			name: "user claims -h only: help built-in NOT suppressed (collision)",
			cmd: Command{
				Name:    "myprogram",
				Options: []Option{&StringOption{Flag: "-h"}},
			},
			want: []Option{
				&StringOption{Flag: "-h"},
				&BoolOption{Flag: "-h", Long: "--help", Help: "Print help", Dest: anyDest, Action: anyAction},
			},
		},
		{
			name: "user claims --help only: help built-in NOT suppressed (collision)",
			cmd: Command{
				Name:    "myprogram",
				Options: []Option{&StringOption{Long: "--help"}},
			},
			want: []Option{
				&StringOption{Long: "--help"},
				&BoolOption{Flag: "-h", Long: "--help", Help: "Print help", Dest: anyDest, Action: anyAction},
			},
		},
		{
			// Suppression is per-token, not per-Option: two Options that
			// between them claim both forms leave nothing for the
			// built-in to collide with, so it is suppressed.
			name: "both help forms claimed across two Options: help built-in suppressed",
			cmd: Command{
				Name:    "myprogram",
				Version: "0.1.0",
				Options: []Option{
					&StringOption{Flag: "-h"},
					&StringOption{Long: "--help"},
				},
			},
			want: []Option{
				&StringOption{Flag: "-h"},
				&StringOption{Long: "--help"},
				&BoolOption{Flag: "-v", Long: "--version", Help: "Print version", Dest: anyDest, Action: anyAction},
			},
		},
		{
			name: "user claims -h only: no -H fallback for help",
			cmd: Command{
				Name:    "myprogram",
				Options: []Option{&StringOption{Flag: "-h", Long: "--host", Help: "Host"}},
			},
			want: []Option{
				&StringOption{Flag: "-h", Long: "--host", Help: "Host"},
				&BoolOption{Flag: "-h", Long: "--help", Help: "Print help", Dest: anyDest, Action: anyAction},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slices.Collect(tt.cmd.options())
			if !optionSlicesEqual(got, tt.want) {
				t.Errorf("options() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestOptions_StopsEarly(t *testing.T) {
	anyAction := func(*Command, Option, string) *Error { return nil }
	var discard bool
	anyDest := &discard

	cmd := Command{
		Name:    "myprogram",
		Version: "0.1.0",
		Options: []Option{
			&StringOption{Flag: "-f", Help: "Enable foo mode"},
		},
	}

	tests := []struct {
		name      string
		stopAfter int // yield returns false once len(got) reaches this count
		want      []Option
	}{
		{
			name:      "stops inside the user-declared Options",
			stopAfter: 1,
			want: []Option{
				&StringOption{Flag: "-f", Help: "Enable foo mode"},
			},
		},
		{
			name:      "stops right after the synthesized -h/--help",
			stopAfter: 2,
			want: []Option{
				&StringOption{Flag: "-f", Help: "Enable foo mode"},
				&BoolOption{Flag: "-h", Long: "--help", Help: "Print help", Dest: anyDest, Action: anyAction},
			},
		},
		{
			name:      "stops right after the synthesized -v/--version",
			stopAfter: 3,
			want: []Option{
				&StringOption{Flag: "-f", Help: "Enable foo mode"},
				&BoolOption{Flag: "-h", Long: "--help", Help: "Print help", Dest: anyDest, Action: anyAction},
				&BoolOption{Flag: "-v", Long: "--version", Help: "Print version", Dest: anyDest, Action: anyAction},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []Option
			cmd.options()(func(opt Option) bool {
				got = append(got, opt)
				return len(got) < tt.stopAfter
			})

			if !optionSlicesEqual(got, tt.want) {
				t.Errorf("options() stopped after %d = %+v, want %+v", tt.stopAfter, got, tt.want)
			}
		})
	}

	t.Run("suppression shortens the sequence", func(t *testing.T) {
		cmd := Command{
			Name:    "myprogram",
			Version: "0.1.0",
			Options: []Option{
				&StringOption{Flag: "-h", Long: "--help", Help: "Custom help"},
			},
		}

		var got []Option
		cmd.options()(func(opt Option) bool {
			got = append(got, opt)
			return true
		})

		if len(got) != 2 {
			t.Fatalf("options() = %d options, want 2 (user -h/--help + version built-in)", len(got))
		}
		if got[1].GetFlag() != "-v" || got[1].GetLong() != "--version" {
			t.Errorf("second yielded Option = %+v, want the version built-in", got[1])
		}
	})
}

// TestItems covers items' order: every Argument, then every Option,
// then every EnvVar -- and that the built-in options are yielded, after
// the user-declared ones, only with withBuiltins.
func TestItems(t *testing.T) {
	tests := []struct {
		name  string
		cmd   Command
		scope int
		want  []string
	}{
		{
			name:  "nothing declared, declaredOnly",
			cmd:   Command{Name: "myprogram", Version: "1.0.0"},
			scope: declaredOnly,
			want:  nil,
		},
		{
			name: "Arguments, Options, and EnvVars, in that order",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux"}, &IntArgument{Name: "count"}},
				Options:   []Option{&StringOption{Flag: "-b", Long: "--bar"}},
				EnvVars:   []EnvVar{&StringEnvVar{Name: "APP_DEBUG"}},
			},
			scope: declaredOnly,
			want:  []string{"qux", "count", "bar", "APP_DEBUG"},
		},
		{
			name:  "nothing declared, withBuiltins",
			cmd:   Command{Name: "myprogram", Version: "1.0.0"},
			scope: withBuiltins,
			want:  []string{"help", "version"},
		},
		{
			name: "withBuiltins yields the built-ins after the user-declared Options",
			cmd: Command{
				Name:      "myprogram",
				Version:   "1.0.0",
				Arguments: []Argument{&StringArgument{Name: "qux"}},
				Options:   []Option{&StringOption{Flag: "-b", Long: "--bar"}},
				EnvVars:   []EnvVar{&StringEnvVar{Name: "APP_DEBUG"}},
			},
			scope: withBuiltins,
			want:  []string{"qux", "bar", "help", "version", "APP_DEBUG"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for it := range tt.cmd.items(tt.scope) {
				got = append(got, it.GetName())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("items() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestItems_StopsEarly covers the early exit of each of items' three
// loops, for the same reason as TestOptions_StopsEarly: one stopping
// point per loop.
func TestItems_StopsEarly(t *testing.T) {
	cmd := Command{
		Name:      "myprogram",
		Arguments: []Argument{&StringArgument{Name: "qux"}},
		Options:   []Option{&StringOption{Flag: "-b", Long: "--bar"}},
		EnvVars:   []EnvVar{&StringEnvVar{Name: "APP_DEBUG"}},
	}

	tests := []struct {
		name      string
		stopAfter int
		want      []string
	}{
		{name: "stops inside the Arguments loop", stopAfter: 1, want: []string{"qux"}},
		{name: "stops inside the Options loop", stopAfter: 2, want: []string{"qux", "bar"}},
		{name: "stops inside the EnvVars loop", stopAfter: 3, want: []string{"qux", "bar", "APP_DEBUG"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			cmd.items(declaredOnly)(func(it item) bool {
				got = append(got, it.GetName())
				return len(got) < tt.stopAfter
			})
			if !slices.Equal(got, tt.want) {
				t.Errorf("items() stopped after %d = %v, want %v", tt.stopAfter, got, tt.want)
			}
		})
	}
}

// --- Validate ----------------------------------------------------------------

// TestValidate covers Validate end to end: the Command.Name check and
// the pass over every item -- its own structure, then the uniqueness
// rules for environment variable names, Flag/Long forms, and names. The
// per-item rules themselves are also covered next to each type, in
// TestArgument_Validate, TestOption_Validate, and TestEnvVar_Validate.
func TestValidate(t *testing.T) {
	var s string

	tests := []struct {
		name    string
		cmd     Command
		wantErr bool
	}{
		{
			name: "valid, empty Command",
			cmd:  Command{Name: "myprogram"},
		},
		{
			name:    "empty Command.Name",
			cmd:     Command{},
			wantErr: true,
		},

		// Per-item structure, reached through the item pass.
		{
			name: "valid, one item per family",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux", Dest: &s}},
				Options:   []Option{&StringOption{Flag: "-f", Long: "--foo"}},
				EnvVars:   []EnvVar{&StringEnvVar{Name: "APP_DEBUG"}},
			},
		},
		{
			name: "valid, Option with Action instead of Dest",
			cmd: Command{
				Name:    "myprogram",
				Options: []Option{&StringOption{Flag: "-f", Action: func(*Command, Option, string) *Error { return nil }}},
			},
		},
		{
			name: "valid, Option with both Dest and Action",
			cmd: Command{
				Name:    "myprogram",
				Options: []Option{&StringOption{Flag: "-f", Dest: &s, Action: func(*Command, Option, string) *Error { return nil }}},
			},
		},
		{
			name: "empty Argument.Name",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: ""}},
			},
			wantErr: true,
		},
		{
			name: "Option missing both Flag and Long",
			cmd: Command{
				Name:    "myprogram",
				Options: []Option{&StringOption{}},
			},
			wantErr: true,
		},
		{
			name: "Option Flag longer than one character",
			cmd: Command{
				Name:    "myprogram",
				Options: []Option{&StringOption{Flag: "-ab"}},
			},
			wantErr: true,
		},
		{
			name: "Option Flag with only dashes",
			cmd: Command{
				Name:    "myprogram",
				Options: []Option{&StringOption{Flag: "--"}},
			},
			wantErr: true,
		},
		{
			name: "empty EnvVar.Name",
			cmd: Command{
				Name:    "myprogram",
				EnvVars: []EnvVar{&StringEnvVar{Name: ""}},
			},
			wantErr: true,
		},

		// Environment variable names are unique across all three families.
		{
			name: "no Env declared anywhere",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux"}},
				Options:   []Option{&StringOption{Flag: "-f"}},
			},
		},
		{
			name: "valid, one Env per source",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux", Env: "APP_QUX"}},
				Options:   []Option{&StringOption{Flag: "-b", Env: "APP_BAR"}},
				EnvVars:   []EnvVar{&StringEnvVar{Name: "APP_DEBUG"}},
			},
		},
		{
			name: "duplicate environment variable, both from EnvVars",
			cmd: Command{
				Name:    "myprogram",
				EnvVars: []EnvVar{&StringEnvVar{Name: "APP_DEBUG"}, &StringEnvVar{Name: "APP_DEBUG"}},
			},
			wantErr: true,
		},
		{
			name: "duplicate environment variable, both from Arguments",
			cmd: Command{
				Name: "myprogram",
				Arguments: []Argument{
					&StringArgument{Name: "qux", Env: "APP_QUX"},
					&StringArgument{Name: "baz", Env: "APP_QUX"},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate environment variable, Argument.Env vs Option.Env",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux", Env: "APP_QUX"}},
				Options:   []Option{&StringOption{Flag: "-b", Env: "APP_QUX"}},
			},
			wantErr: true,
		},
		{
			name: "duplicate environment variable, Argument.Env vs EnvVars",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux", Env: "APP_QUX"}},
				EnvVars:   []EnvVar{&StringEnvVar{Name: "APP_QUX"}},
			},
			wantErr: true,
		},
		{
			name: "duplicate environment variable, Option.Env vs EnvVars",
			cmd: Command{
				Name:    "myprogram",
				Options: []Option{&StringOption{Flag: "-b", Env: "APP_BAR"}},
				EnvVars: []EnvVar{&StringEnvVar{Name: "APP_BAR"}},
			},
			wantErr: true,
		},

		// Item names are unique across all three families.
		{
			name: "duplicate Argument.Name",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux"}, &StringArgument{Name: "qux"}},
			},
			wantErr: true,
		},
		{
			name: "duplicate explicit Option.Name",
			cmd: Command{
				Name: "myprogram",
				Options: []Option{
					&StringOption{Name: "x", Flag: "-a"},
					&StringOption{Name: "x", Flag: "-b"},
				},
			},
			wantErr: true,
		},
		{
			name: "Option names derived from Long/Flag collide: Flag -f and Long --f",
			cmd: Command{
				Name: "myprogram",
				Options: []Option{
					&StringOption{Flag: "-f"},
					&StringOption{Long: "--f"},
				},
			},
			wantErr: true,
		},
		{
			name: "explicit Option.Name collides with another Option's derived name",
			cmd: Command{
				Name: "myprogram",
				Options: []Option{
					&StringOption{Name: "bar", Flag: "-a"},
					&StringOption{Long: "--bar"},
				},
			},
			wantErr: true,
		},
		{
			name: "Argument.Name collides with an Option's derived name",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "file"}},
				Options:   []Option{&StringOption{Long: "--file"}},
			},
			wantErr: true,
		},
		{
			name: "explicit Option.Name settles a collision with an Argument.Name",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "file"}},
				Options:   []Option{&StringOption{Name: "input", Long: "--file"}},
			},
		},
		{
			name: "Argument.Name collides with the version built-in's name",
			cmd: Command{
				Name:      "myprogram",
				Version:   "0.1.0",
				Arguments: []Argument{&StringArgument{Name: "version"}},
			},
			wantErr: true,
		},
		{
			name: "Argument.Name matches the version built-in's name, but the built-in is absent without Version",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "version"}},
			},
		},
		{
			name: "Argument.Name collides with an EnvVar.Name",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "APP_DEBUG"}},
				EnvVars:   []EnvVar{&StringEnvVar{Name: "APP_DEBUG"}},
			},
			wantErr: true,
		},

		// Flag/Long uniqueness, and the built-ins' suppression rules.
		{
			name: "valid, distinct Flags and Longs",
			cmd: Command{
				Name: "myprogram",
				Options: []Option{
					&StringOption{Flag: "-f", Long: "--foo"},
					&StringOption{Flag: "-b"},
				},
			},
		},
		{
			name: "duplicate Flag is reported before the name it also derives",
			cmd: Command{
				Name: "myprogram",
				Options: []Option{
					&StringOption{Flag: "-f"},
					&StringOption{Flag: "-f"},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate Flag, different spelling normalizes the same",
			cmd: Command{
				Name: "myprogram",
				Options: []Option{
					&StringOption{Name: "a", Flag: "f"},
					&StringOption{Name: "b", Flag: "-f"},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate Long, different spelling normalizes the same",
			cmd: Command{
				Name: "myprogram",
				Options: []Option{
					&StringOption{Name: "a", Long: "bar"},
					&StringOption{Name: "b", Long: "--bar"},
				},
			},
			wantErr: true,
		},
		{
			name: "partial claim on the help built-in's forms: collision rejected",
			cmd: Command{
				Name:    "myprogram",
				Options: []Option{&StringOption{Flag: "-h", Help: "Custom help"}},
			},
			wantErr: true,
		},
		{
			name: "partial claim on the version built-in's forms: collision rejected",
			cmd: Command{
				Name:    "myprogram",
				Version: "0.1.0",
				Options: []Option{&StringOption{Long: "--version", Help: "Custom version"}},
			},
			wantErr: true,
		},
		{
			name: "-v claimed without --version: version built-in falls back to -V, no conflict",
			cmd: Command{
				Name:    "myprogram",
				Version: "0.1.0",
				Options: []Option{&StringOption{Flag: "-v", Long: "--verbose", Help: "Verbose"}},
			},
		},
		{
			name: "-v and -V claimed without --version: version built-in keeps --version only, no conflict",
			cmd: Command{
				Name:    "myprogram",
				Version: "0.1.0",
				Options: []Option{
					&StringOption{Flag: "-v", Long: "--verbose", Help: "Verbose"},
					&StringOption{Flag: "-V", Long: "--vv", Help: "Very verbose"},
				},
			},
		},
		{
			name: "full claim on the help built-in's forms: built-in suppressed, no conflict",
			cmd: Command{
				Name:    "myprogram",
				Options: []Option{&StringOption{Flag: "-h", Long: "--help", Help: "Custom help"}},
			},
		},
		{
			name: "full claim on the version built-in's forms: built-in suppressed, no conflict",
			cmd: Command{
				Name:    "myprogram",
				Version: "0.1.0",
				Options: []Option{&StringOption{Flag: "-v", Long: "--version", Help: "Custom version"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			// Every Validate failure reports ExitCode() == 1 -- see
			// Validate's doc comment.
			if err != nil && err.ExitCode() != 1 {
				t.Errorf("Validate().ExitCode() = %d, want 1", err.ExitCode())
			}
		})
	}
}

// --- Parse -------------------------------------------------------------------

// TestParse covers Parse's Default -> Env resolution across Arguments,
// Options, and standalone EnvVars, with an empty args slice. The
// Validate wiring is exercised end to end via DefaultParse in
// TestDefaultParse.
func TestParse(t *testing.T) {
	t.Run("no default, no env: Dest keeps its zero value", func(t *testing.T) {
		var s string
		c := &Command{
			Name:    "myprogram",
			Options: []Option{&StringOption{Flag: "-b", Dest: &s}},
		}

		if err := c.Parse(nil); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if s != "" {
			t.Errorf("Dest = %q, want zero value", s)
		}
	})

	t.Run("default applied when the env var is not set", func(t *testing.T) {
		var s string
		c := &Command{
			Name:    "myprogram",
			Options: []Option{&StringOption{Flag: "-b", Env: "APP_BAR", Dest: &s, Default: "fallback"}},
		}

		if err := c.Parse(nil); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if s != "fallback" {
			t.Errorf("Dest = %q, want %q", s, "fallback")
		}
	})

	t.Run("env overrides default, for an Argument", func(t *testing.T) {
		var s string
		t.Setenv("APP_QUX", "from-env")
		c := &Command{
			Name:      "myprogram",
			Arguments: []Argument{&StringArgument{Name: "qux", Env: "APP_QUX", Dest: &s, Default: "fallback"}},
		}

		if err := c.Parse(nil); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if s != "from-env" {
			t.Errorf("Dest = %q, want %q", s, "from-env")
		}
	})

	t.Run("env overrides default, for an Option", func(t *testing.T) {
		var i int
		t.Setenv("APP_NUM", "42")
		c := &Command{
			Name:    "myprogram",
			Options: []Option{&IntOption{Flag: "-n", Env: "APP_NUM", Dest: &i, Default: 7}},
		}

		if err := c.Parse(nil); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if i != 42 {
			t.Errorf("Dest = %d, want %d", i, 42)
		}
	})

	t.Run("a standalone EnvVar's Dest is resolved too", func(t *testing.T) {
		var s string
		t.Setenv("APP_DEBUG", "verbose")
		c := &Command{
			Name:    "myprogram",
			EnvVars: []EnvVar{&StringEnvVar{Name: "APP_DEBUG", Dest: &s}},
		}

		if err := c.Parse(nil); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if s != "verbose" {
			t.Errorf("Dest = %q, want %q", s, "verbose")
		}
	})

	t.Run("invalid env value: ExitCode 2 and a clap-style block on Stderr", func(t *testing.T) {
		var i int
		var buf bytes.Buffer
		t.Setenv("APP_NUM", "not-a-number")
		c := &Command{
			Name:    "myprogram",
			Options: []Option{&IntOption{Flag: "-n", Env: "APP_NUM", Dest: &i}},
			Stderr:  &buf,
		}

		err := c.Parse(nil)
		if err == nil {
			t.Fatal("Parse() = nil, want error")
		}
		if got, want := err.ExitCode(), 2; got != want {
			t.Errorf("ExitCode() = %d, want %d", got, want)
		}
		if got, want := buf.String(), c.ErrorString(err); got != want {
			t.Errorf("Parse() wrote %q to Stderr, want %q", got, want)
		}
	})

	t.Run("a Validate failure short-circuits before any Env resolution", func(t *testing.T) {
		c := &Command{} // an empty Name makes Validate fail
		err := c.Parse(nil)
		if err == nil {
			t.Fatal("Parse() = nil, want error")
		}
		if got, want := err.ExitCode(), 1; got != want {
			t.Errorf("ExitCode() = %d, want %d", got, want)
		}
	})
}

// TestParse_TokenLoop covers Parse's token loop: resolving Options
// (built-in and user-declared alike, through the same lookup) and
// positional Arguments from a real args slice.
func TestParse_TokenLoop(t *testing.T) {
	t.Run("-h prints help and returns ErrHelp", func(t *testing.T) {
		var buf bytes.Buffer
		c := &Command{Name: "myprogram", Stdout: &buf}

		err := c.Parse([]string{"-h"})
		if !errors.Is(err, ErrHelp) {
			t.Fatalf("Parse() = %v, want ErrHelp", err)
		}
		if got, want := buf.String(), c.HelpString(); got != want {
			t.Errorf("Parse() wrote %q to Stdout, want %q", got, want)
		}
	})

	t.Run("--help, long form, behaves the same as -h", func(t *testing.T) {
		c := &Command{Name: "myprogram", Stdout: &bytes.Buffer{}}

		if err := c.Parse([]string{"--help"}); !errors.Is(err, ErrHelp) {
			t.Errorf("Parse() = %v, want ErrHelp", err)
		}
	})

	t.Run("user claims both help forms: -h dispatches to the user's Option, not the built-in", func(t *testing.T) {
		var ran, custom bool
		var buf bytes.Buffer
		c := &Command{
			Name: "myprogram",
			Options: []Option{&BoolOption{
				Flag: "-h", Long: "--help", Help: "Custom help",
				// A BoolOption never consumes the next token as its value.
				Dest: &custom,
				Action: func(c *Command, opt Option, value string) *Error {
					ran = true
					return nil
				},
			}},
			Stdout: &buf,
		}

		if err := c.Parse([]string{"-h"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if !ran {
			t.Error("user-declared -h Action did not run; the built-in help fired instead")
		}
		if buf.Len() != 0 {
			t.Errorf("Parse() wrote %q to Stdout, want nothing (no built-in help)", buf.String())
		}
	})

	t.Run("-v prints version and returns ErrVersion, when Version is set", func(t *testing.T) {
		var buf bytes.Buffer
		c := &Command{Name: "myprogram", Version: "0.1.0", Stdout: &buf}

		err := c.Parse([]string{"-v"})
		if !errors.Is(err, ErrVersion) {
			t.Fatalf("Parse() = %v, want ErrVersion", err)
		}
		if got, want := buf.String(), c.VersionString()+"\n"; got != want {
			t.Errorf("Parse() wrote %q to Stdout, want %q", got, want)
		}
	})

	t.Run("-v claimed by the user: -v runs the user's Option, -V prints version", func(t *testing.T) {
		var verbose bool
		var buf bytes.Buffer
		c := &Command{
			Name:    "myprogram",
			Version: "0.1.0",
			Stdout:  &buf,
			Options: []Option{&BoolOption{Flag: "-v", Long: "--verbose", Help: "Verbose", Dest: &verbose}},
		}

		if err := c.Parse([]string{"-v"}); err != nil {
			t.Fatalf("Parse(-v) = %v, want nil", err)
		}
		if !verbose {
			t.Error("Parse(-v) did not set the user's Option")
		}
		if buf.Len() != 0 {
			t.Errorf("Parse(-v) wrote %q to Stdout, want nothing", buf.String())
		}

		err := c.Parse([]string{"-V"})
		if !errors.Is(err, ErrVersion) {
			t.Fatalf("Parse(-V) = %v, want ErrVersion", err)
		}
		if got, want := buf.String(), c.VersionString()+"\n"; got != want {
			t.Errorf("Parse(-V) wrote %q to Stdout, want %q", got, want)
		}
	})

	t.Run("-v and -V both claimed: --version still prints version", func(t *testing.T) {
		var v1, v2 bool
		var buf bytes.Buffer
		c := &Command{
			Name:    "myprogram",
			Version: "0.1.0",
			Stdout:  &buf,
			Options: []Option{
				&BoolOption{Flag: "-v", Long: "--verbose", Dest: &v1},
				&BoolOption{Flag: "-V", Long: "--vv", Dest: &v2},
			},
		}

		if err := c.Parse([]string{"--version"}); !errors.Is(err, ErrVersion) {
			t.Fatalf("Parse(--version) = %v, want ErrVersion", err)
		}
		if got, want := buf.String(), c.VersionString()+"\n"; got != want {
			t.Errorf("Parse(--version) wrote %q to Stdout, want %q", got, want)
		}
	})

	t.Run("-v is an unknown option when Version is unset", func(t *testing.T) {
		var buf bytes.Buffer
		c := &Command{Name: "myprogram", Stderr: &buf}

		err := c.Parse([]string{"-v"})
		if err == nil || err.ExitCode() != 2 {
			t.Fatalf("Parse() = %v, want a code-2 error", err)
		}
		if want := `unknown option "-v"`; err.Error() != want {
			t.Errorf("Parse().Error() = %q, want %q", err.Error(), want)
		}
	})

	t.Run("unknown option", func(t *testing.T) {
		var buf bytes.Buffer
		c := &Command{Name: "myprogram", Stderr: &buf}

		err := c.Parse([]string{"--nope"})
		if err == nil || err.ExitCode() != 2 {
			t.Fatalf("Parse() = %v, want a code-2 error", err)
		}
		if want := `unknown option "--nope"`; err.Error() != want {
			t.Errorf("Parse().Error() = %q, want %q", err.Error(), want)
		}
		if got, want := buf.String(), c.ErrorString(errors.New(`unknown option "--nope"`)); got != want {
			t.Errorf("Parse() wrote %q to Stderr, want %q", got, want)
		}
	})

	t.Run("an Option repeated in either form is a usage error", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want string
		}{
			{"same token twice", []string{"-b", "x", "-b", "y"}, `option "-b" cannot be used multiple times`},
			{"Flag then Long", []string{"-b", "x", "--bar", "y"}, `option "--bar" cannot be used multiple times`},
			{"Long then Long with =", []string{"--bar", "x", "--bar=y"}, `option "--bar=y" cannot be used multiple times`},
			{"boolean Option", []string{"-f", "-f"}, `option "-f" cannot be used multiple times`},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var bar string
				var foo bool
				var buf bytes.Buffer
				c := &Command{
					Name: "myprogram",
					Options: []Option{
						&StringOption{Flag: "-b", Long: "--bar", Dest: &bar},
						&BoolOption{Flag: "-f", Dest: &foo},
					},
					Stderr: &buf,
				}

				err := c.Parse(tt.args)
				if err == nil || err.ExitCode() != 2 {
					t.Fatalf("Parse() = %v, want a code-2 error", err)
				}
				if err.Error() != tt.want {
					t.Errorf("Parse().Error() = %q, want %q", err.Error(), tt.want)
				}
				if got, want := buf.String(), c.ErrorString(errors.New(tt.want)); got != want {
					t.Errorf("Parse() wrote %q to Stderr, want %q", got, want)
				}
			})
		}
	})

	t.Run("bool flag: presence alone sets it, without consuming the next token", func(t *testing.T) {
		var foo bool
		var qux string
		c := &Command{
			Name:      "myprogram",
			Options:   []Option{&BoolOption{Flag: "-f", Dest: &foo}},
			Arguments: []Argument{&StringArgument{Name: "qux", Dest: &qux}},
		}

		if err := c.Parse([]string{"-f", "hello"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if !foo {
			t.Error("foo = false, want true")
		}
		if qux != "hello" {
			t.Errorf("qux = %q, want %q -- bool flag must not consume the next token", qux, "hello")
		}
	})

	t.Run("value-taking option consumes the next token", func(t *testing.T) {
		var bar string
		c := &Command{Name: "myprogram", Options: []Option{&StringOption{Flag: "-b", Long: "--bar", Dest: &bar}}}

		if err := c.Parse([]string{"--bar", "value"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if bar != "value" {
			t.Errorf("bar = %q, want %q", bar, "value")
		}
	})

	t.Run("value-taking option missing its value", func(t *testing.T) {
		var bar string
		var buf bytes.Buffer
		c := &Command{Name: "myprogram", Options: []Option{&StringOption{Flag: "-b", Dest: &bar}}, Stderr: &buf}

		err := c.Parse([]string{"-b"})
		if err == nil || err.ExitCode() != 2 {
			t.Fatalf("Parse() = %v, want a code-2 error", err)
		}
		if want := `option "-b" requires a value`; err.Error() != want {
			t.Errorf("Parse().Error() = %q, want %q", err.Error(), want)
		}
	})

	t.Run("value-taking option, invalid value", func(t *testing.T) {
		var n int
		c := &Command{Name: "myprogram", Options: []Option{&IntOption{Flag: "-n", Dest: &n}}, Stderr: &bytes.Buffer{}}

		err := c.Parse([]string{"-n", "not-a-number"})
		if err == nil || err.ExitCode() != 2 {
			t.Fatalf("Parse() = %v, want a code-2 error", err)
		}
		if want := `option "-n": invalid integer value "not-a-number": strconv.Atoi: parsing "not-a-number": invalid syntax`; err.Error() != want {
			t.Errorf("Parse().Error() = %q, want %q", err.Error(), want)
		}
	})

	t.Run("--bar=value attaches the value without consuming the next token", func(t *testing.T) {
		var bar, qux string
		c := &Command{
			Name:      "myprogram",
			Options:   []Option{&StringOption{Flag: "-b", Long: "--bar", Dest: &bar}},
			Arguments: []Argument{&StringArgument{Name: "qux", Dest: &qux}},
		}

		if err := c.Parse([]string{"--bar=value", "hello"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if bar != "value" {
			t.Errorf("bar = %q, want %q", bar, "value")
		}
		if qux != "hello" {
			t.Errorf(`qux = %q, want %q -- "--bar=value" must not consume the next token`, qux, "hello")
		}
	})

	t.Run("--bar= attaches an empty value, still without consuming the next token", func(t *testing.T) {
		var bar, qux string
		c := &Command{
			Name:      "myprogram",
			Options:   []Option{&StringOption{Long: "--bar", Dest: &bar}},
			Arguments: []Argument{&StringArgument{Name: "qux", Dest: &qux}},
		}

		if err := c.Parse([]string{"--bar=", "hello"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if bar != "" {
			t.Errorf("bar = %q, want empty string", bar)
		}
		if qux != "hello" {
			t.Errorf("qux = %q, want %q", qux, "hello")
		}
	})

	t.Run("--bar=a=b splits only on the first =", func(t *testing.T) {
		var bar string
		c := &Command{Name: "myprogram", Options: []Option{&StringOption{Long: "--bar", Dest: &bar}}}

		if err := c.Parse([]string{"--bar=a=b"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if bar != "a=b" {
			t.Errorf("bar = %q, want %q", bar, "a=b")
		}
	})

	t.Run("--flag=false overrides a boolean Dest's bare-presence true", func(t *testing.T) {
		var verbose bool
		c := &Command{Name: "myprogram", Options: []Option{&BoolOption{Long: "--verbose", Dest: &verbose}}}

		if err := c.Parse([]string{"--verbose=false"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if verbose {
			t.Error(`verbose = true, want false -- "--verbose=false" must override the bare-presence default`)
		}
	})

	t.Run(`-b=value is not split -- "=" merging is long-form only`, func(t *testing.T) {
		var b string
		var buf bytes.Buffer
		c := &Command{Name: "myprogram", Options: []Option{&StringOption{Flag: "-b", Dest: &b}}, Stderr: &buf}

		err := c.Parse([]string{"-b=value"})
		if err == nil || err.ExitCode() != 2 {
			t.Fatalf("Parse() = %v, want a code-2 error", err)
		}
		if want := `unknown option "-b=value"`; err.Error() != want {
			t.Errorf("Parse().Error() = %q, want %q", err.Error(), want)
		}
	})

	t.Run("unknown long option with an attached value: the full token is echoed", func(t *testing.T) {
		var buf bytes.Buffer
		c := &Command{Name: "myprogram", Stderr: &buf}

		err := c.Parse([]string{"--nope=value"})
		if err == nil || err.ExitCode() != 2 {
			t.Fatalf("Parse() = %v, want a code-2 error", err)
		}
		if want := `unknown option "--nope=value"`; err.Error() != want {
			t.Errorf("Parse().Error() = %q, want %q", err.Error(), want)
		}
	})

	t.Run("positional argument fills the next declared Argument, in order", func(t *testing.T) {
		var a, b string
		c := &Command{
			Name: "myprogram",
			Arguments: []Argument{
				&StringArgument{Name: "first", Dest: &a},
				&StringArgument{Name: "second", Dest: &b},
			},
		}

		if err := c.Parse([]string{"one", "two"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if a != "one" || b != "two" {
			t.Errorf("a, b = %q, %q, want %q, %q", a, b, "one", "two")
		}
	})

	t.Run("more positional tokens than declared Arguments", func(t *testing.T) {
		c := &Command{Name: "myprogram", Stderr: &bytes.Buffer{}}

		err := c.Parse([]string{"extra"})
		if err == nil || err.ExitCode() != 2 {
			t.Fatalf("Parse() = %v, want a code-2 error", err)
		}
		if want := `unexpected argument "extra"`; err.Error() != want {
			t.Errorf("Parse().Error() = %q, want %q", err.Error(), want)
		}
	})

	t.Run("positional argument, invalid value", func(t *testing.T) {
		var n int
		c := &Command{Name: "myprogram", Arguments: []Argument{&IntArgument{Name: "num", Dest: &n}}, Stderr: &bytes.Buffer{}}

		err := c.Parse([]string{"not-a-number"})
		if err == nil || err.ExitCode() != 2 {
			t.Fatalf("Parse() = %v, want a code-2 error", err)
		}
		if want := `argument "not-a-number": invalid integer value "not-a-number": strconv.Atoi: parsing "not-a-number": invalid syntax`; err.Error() != want {
			t.Errorf("Parse().Error() = %q, want %q", err.Error(), want)
		}
	})

	t.Run("Argument with no Dest is still consumed positionally, just not bound", func(t *testing.T) {
		c := &Command{Name: "myprogram", Arguments: []Argument{&StringArgument{Name: "qux"}}}

		if err := c.Parse([]string{"anything"}); err != nil {
			t.Errorf("Parse() = %v, want nil", err)
		}
	})

	t.Run("options and arguments mixed together", func(t *testing.T) {
		var foo bool
		var qux string
		c := &Command{
			Name:      "myprogram",
			Options:   []Option{&BoolOption{Flag: "-f", Dest: &foo}},
			Arguments: []Argument{&StringArgument{Name: "qux", Dest: &qux}},
		}

		if err := c.Parse([]string{"-f", "hello"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if !foo || qux != "hello" {
			t.Errorf("foo, qux = %v, %q, want %v, %q", foo, qux, true, "hello")
		}
	})

	t.Run("a caller-declared Option's Action fires, same as a built-in", func(t *testing.T) {
		var ran bool
		var seen Option
		var seenValue string
		c := &Command{
			Name: "myprogram",
			Options: []Option{
				&StringOption{
					Flag: "-x", Long: "--extra",
					Action: func(c *Command, opt Option, value string) *Error {
						ran = true
						seen = opt
						seenValue = value
						return nil
					},
				},
			},
		}

		if err := c.Parse([]string{"-x", "something"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if !ran {
			t.Fatal("Action did not run")
		}
		if seen.GetFlag() != "-x" || seen.GetLong() != "--extra" {
			t.Errorf("Action received Option %+v, want Flag=-x Long=--extra", seen)
		}
		if seenValue != "something" {
			t.Errorf("Action received value = %q, want %q -- an Option with Action but no Dest consumes a value by default", seenValue, "something")
		}
	})

	t.Run("Action returning nil lets parsing continue past its own consumed value", func(t *testing.T) {
		var ran bool
		var seenValue string
		var qux string
		c := &Command{
			Name: "myprogram",
			Options: []Option{
				&StringOption{Flag: "-x", Action: func(c *Command, opt Option, value string) *Error { ran = true; seenValue = value; return nil }},
			},
			Arguments: []Argument{&StringArgument{Name: "qux", Dest: &qux}},
		}

		if err := c.Parse([]string{"-x", "consumed", "hello"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if !ran {
			t.Fatal("Action did not run")
		}
		if seenValue != "consumed" {
			t.Errorf("Action received value = %q, want %q", seenValue, "consumed")
		}
		if qux != "hello" {
			t.Errorf("qux = %q, want %q -- parsing should have continued past -x's own value", qux, "hello")
		}
	})

	t.Run("Action returning a non-nil *Error stops parsing immediately", func(t *testing.T) {
		var later bool
		sentinel := errors.New("boom")
		c := &Command{
			Name: "myprogram",
			Options: []Option{
				&StringOption{Flag: "-x", Action: func(c *Command, opt Option, value string) *Error { return Errorf(2, "%s", sentinel) }},
				&BoolOption{Flag: "-y", Dest: &later},
			},
		}

		err := c.Parse([]string{"-x", "ignored", "-y"})
		if err == nil {
			t.Fatal("Parse() = nil, want error")
		}
		if err.ExitCode() != 2 || err.Error() != sentinel.Error() {
			t.Errorf("Parse() = %v (code %d), want %q (code 2)", err, err.ExitCode(), sentinel.Error())
		}
		if later {
			t.Error("-y was processed after Action stopped parsing, want it skipped")
		}
	})

	t.Run("Dest and Action combined: Dest fires first, Action receives the same value", func(t *testing.T) {
		var bar string
		var seenViaAction string
		c := &Command{
			Name: "myprogram",
			Options: []Option{&StringOption{
				Flag: "-b", Dest: &bar,
				Action: func(c *Command, opt Option, value string) *Error {
					seenViaAction = value
					return nil
				},
			}},
		}

		if err := c.Parse([]string{"-b", "hello"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if bar != "hello" {
			t.Errorf("Dest = %q, want %q", bar, "hello")
		}
		if seenViaAction != "hello" {
			t.Errorf("Action received value = %q, want %q", seenViaAction, "hello")
		}
	})

	t.Run("Dest and Action combined, boolean flag: Action receives \"true\"", func(t *testing.T) {
		var foo bool
		var seenViaAction string
		c := &Command{
			Name: "myprogram",
			Options: []Option{&BoolOption{
				Flag: "-f", Dest: &foo,
				Action: func(c *Command, opt Option, value string) *Error {
					seenViaAction = value
					return nil
				},
			}},
		}

		if err := c.Parse([]string{"-f"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if !foo {
			t.Error("Dest = false, want true")
		}
		if seenViaAction != "true" {
			t.Errorf("Action received value = %q, want %q", seenViaAction, "true")
		}
	})

	t.Run("Action-only Option (no Dest) consumes the next token by default", func(t *testing.T) {
		var seenValue string
		c := &Command{
			Name: "myprogram",
			Options: []Option{&StringOption{
				Flag: "-x",
				Action: func(c *Command, opt Option, value string) *Error {
					seenValue = value
					return nil
				},
			}},
		}

		if err := c.Parse([]string{"-x", "hello"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if seenValue != "hello" {
			t.Errorf("Action received value = %q, want %q", seenValue, "hello")
		}
	})

	t.Run("a boolean Dest opts an Action-only Option out of the default consuming behavior", func(t *testing.T) {
		var seenValue string
		var qux string
		var triggered bool
		c := &Command{
			Name: "myprogram",
			Options: []Option{&BoolOption{
				Flag: "-x",
				Dest: &triggered,
				Action: func(c *Command, opt Option, value string) *Error {
					seenValue = value
					return nil
				},
			}},
			Arguments: []Argument{&StringArgument{Name: "qux", Dest: &qux}},
		}

		if err := c.Parse([]string{"-x", "hello"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if !triggered {
			t.Error("Dest = false, want true")
		}
		if seenValue != "true" {
			t.Errorf("Action received value = %q, want %q", seenValue, "true")
		}
		if qux != "hello" {
			t.Errorf("qux = %q, want %q -- -x with a boolean Dest must not consume a token", qux, "hello")
		}
	})

	t.Run("Dest and Action combined: a Dest.Set failure prevents Action from running", func(t *testing.T) {
		var n int
		var ran bool
		c := &Command{
			Name: "myprogram",
			Options: []Option{&IntOption{
				Flag: "-n", Dest: &n,
				Action: func(c *Command, opt Option, value string) *Error { ran = true; return nil },
			}},
			Stderr: &bytes.Buffer{},
		}

		err := c.Parse([]string{"-n", "not-a-number"})
		if err == nil || err.ExitCode() != 2 {
			t.Fatalf("Parse() = %v, want a code-2 error", err)
		}
		if ran {
			t.Error("Action ran after Dest.Set failed, want it skipped")
		}
	})
}

// TestDefaultParse only exercises the DefaultParse -> Parse -> Validate
// wiring (one success, one Validate failure) using os.Args directly.
// Everything else Parse does -- consuming flags, positional Arguments,
// Env values -- is Parse's own behavior, not something specific to
// DefaultParse, so those cases belong in TestParse, not here.
func TestDefaultParse(t *testing.T) {
	// os.Args is global process state; save and restore it so this test
	// doesn't leak into other tests or a real `go test` invocation's own
	// arguments.
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })

	os.Args = []string{"myprogram"}
	c := &Command{Name: "myprogram"}
	if err := c.DefaultParse(); err != nil {
		t.Errorf("DefaultParse() = %v, want nil", err)
	}

	os.Args = []string{"myprogram"}
	c = &Command{} // empty Name -> Validate fails
	err := c.DefaultParse()
	if err == nil {
		t.Fatal("DefaultParse() = nil, want error")
	}
	if got, want := err.ExitCode(), 1; got != want {
		t.Errorf("DefaultParse() ExitCode() = %d, want %d", got, want)
	}
}

// --- Error output, Usage, Help, Version --------------------------------------

func TestErrorString(t *testing.T) {
	tests := []struct {
		name string
		cmd  Command
		err  error
		want []string
	}{
		{
			name: "simple message, no arguments",
			cmd:  Command{Name: "myprogram"},
			err:  errors.New("something went wrong"),
			want: []string{
				"error: something went wrong",
				"Usage: myprogram [options]",
				"",
				"For more information, try '--help'.",
			},
		},
		{
			// The "error: " prefix is display-only: added here, never part
			// of the message the underlying error itself carries.
			name: "message with a quoted token",
			cmd:  Command{Name: "myprogram"},
			err:  errors.New(`unknown option "--nope"`),
			want: []string{
				`error: unknown option "--nope"`,
				"Usage: myprogram [options]",
				"",
				"For more information, try '--help'.",
			},
		},
		{
			// The usage line embedded in the error block must reflect
			// the Command's own Arguments, exactly like UsageString
			// does on its own.
			name: "usage line reflects declared Arguments",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux"}},
			},
			err: errors.New(`unexpected argument "extra"`),
			want: []string{
				`error: unexpected argument "extra"`,
				"Usage: myprogram [options] [qux]",
				"",
				"For more information, try '--help'.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := strings.Join(tt.want, "\n") + "\n"

			if got := tt.cmd.ErrorString(tt.err); got != want {
				t.Errorf("ErrorString() =\n%q\nwant\n%q", got, want)
			}
		})
	}
}

// TestPrintErrorf checks the Stderr routing -- PrintErrorf must write
// there even though every other Print* method (PrintUsage, PrintHelp,
// PrintVersion) writes to Stdout instead -- the %w wrapping through
// its internal fmt.Errorf, and the returned *Error: always
// ExitCode() == 2, with a message equal to the formatted error's,
// unprefixed (the "error: " prefix is display-only, added by
// ErrorString).
func TestPrintErrorf(t *testing.T) {
	var buf bytes.Buffer
	c := &Command{Name: "myprogram", Stderr: &buf}
	cause := errors.New("bad value")

	got := c.PrintErrorf("invalid value %q: %w", "nope", cause)

	wantMsg := `invalid value "nope": bad value`
	if want := c.ErrorString(errors.New(wantMsg)); buf.String() != want {
		t.Errorf("PrintErrorf() wrote %q, want %q", buf.String(), want)
	}
	if got.ExitCode() != 2 {
		t.Errorf("PrintErrorf().ExitCode() = %d, want 2", got.ExitCode())
	}
	if got.Error() != wantMsg {
		t.Errorf("PrintErrorf().Error() = %q, want %q", got.Error(), wantMsg)
	}
}

func TestUsageString(t *testing.T) {
	tests := []struct {
		name string
		cmd  Command
		want string
	}{
		{
			name: "no arguments",
			cmd:  Command{Name: "myprogram"},
			want: "Usage: myprogram [options]\n",
		},
		{
			name: "one argument",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux"}},
			},
			want: "Usage: myprogram [options] [qux]\n",
		},
		{
			name: "multiple arguments, declaration order",
			cmd: Command{
				Name: "myprogram",
				Arguments: []Argument{
					&StringArgument{Name: "qux"},
					&StringArgument{Name: "baz"},
				},
			},
			want: "Usage: myprogram [options] [qux] [baz]\n",
		},
		{
			// Brackets mean optional, so a Required Argument is bare.
			name: "required argument is not bracketed",
			cmd: Command{
				Name: "myprogram",
				Arguments: []Argument{
					&StringArgument{Name: "qux", Required: true},
					&StringArgument{Name: "baz"},
				},
			},
			want: "Usage: myprogram [options] qux [baz]\n",
		},
		{
			// Options never affects the usage line beyond the constant
			// "[options]" token -- individual flags are only spelled
			// out in HelpString's "Options:" section.
			name: "options declared, still just \"[options]\"",
			cmd: Command{
				Name:    "myprogram",
				Options: []Option{&StringOption{Flag: "f"}, &StringOption{Long: "bar"}},
			},
			want: "Usage: myprogram [options]\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cmd.UsageString(); got != tt.want {
				t.Errorf("UsageString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrintUsage(t *testing.T) {
	var buf bytes.Buffer
	c := &Command{
		Name:      "myprogram",
		Arguments: []Argument{&StringArgument{Name: "qux"}},
		Stdout:    &buf,
	}

	c.PrintUsage()

	if got, want := buf.String(), "Usage: myprogram [options] [qux]\n"; got != want {
		t.Errorf("PrintUsage() wrote %q, want %q", got, want)
	}
}

// TestOptionLabel covers the value placeholder the help shows after an
// Option's long form: the upper-cased GetName() in angle brackets, except
// for a BoolOption, which takes no value.
func TestOptionLabel(t *testing.T) {
	tests := []struct {
		name string
		opt  Option
		want string
	}{
		{name: "long form gets a placeholder", opt: &StringOption{Long: "--bar"}, want: "--bar <BAR>"},
		{name: "explicit Name drives the placeholder", opt: &IntOption{Long: "--count", Name: "n"}, want: "--count <N>"},
		{name: "flag-only shows the placeholder alone", opt: &StringOption{Flag: "-s"}, want: "<S>"},
		{name: "a BoolOption has no placeholder", opt: &BoolOption{Long: "--foo"}, want: "--foo"},
		{name: "a flag-only BoolOption is empty", opt: &BoolOption{Flag: "-f"}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := optionLabel(tt.opt); got != tt.want {
				t.Errorf("optionLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

// nilDefault is an item whose GetDefault returns nil, as a hand-written
// type with no default might.
type nilDefault struct{ item }

func (nilDefault) GetDefault() any { return nil }

// TestDescribe covers the description the help shows for an item: its
// Help, followed by the environment variable it falls back to, if any.
func TestDescribe(t *testing.T) {
	tests := []struct {
		name string
		it   item
		want string
	}{
		{name: "help only", it: &StringArgument{Name: "qux", Help: "The qux"}, want: "The qux"},
		{name: "help and env", it: &StringArgument{Name: "qux", Help: "The qux", Env: "APP_QUX"}, want: "The qux [env: APP_QUX]"},
		{name: "env without help", it: &StringArgument{Name: "qux", Env: "APP_QUX"}, want: "[env: APP_QUX]"},
		{name: "neither", it: &StringArgument{Name: "qux"}, want: ""},
		{name: "non-zero default", it: &StringOption{Long: "--bar", Help: "Bar", Default: "x"}, want: `Bar [default: "x"]`},
		{name: "env and default", it: &IntOption{Long: "--n", Help: "N", Env: "APP_N", Default: 3}, want: "N [env: APP_N] [default: 3]"},
		{name: "default without help", it: &IntArgument{Name: "n", Default: 3}, want: "[default: 3]"},
		{name: "true default", it: &BoolOption{Long: "--on", Help: "On", Default: true}, want: "On [default: true]"},
		{name: "zero defaults are not shown", it: &BoolOption{Long: "--on", Help: "On"}, want: "On"},
		{name: "a Required item shows no default", it: &StringArgument{Name: "qux", Help: "Qux", Default: "x", Required: true}, want: "Qux"},
		{name: "an EnvVar shows its default", it: &StringEnvVar{Name: "APP_X", Help: "X", Default: "y"}, want: `X [env: APP_X] [default: "y"]`},
		{name: "a nil GetDefault shows none", it: nilDefault{&StringArgument{Name: "qux", Help: "Qux"}}, want: "Qux"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describe(tt.it); got != tt.want {
				t.Errorf("describe() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestGetDefault covers GetDefault() across the generic types.
func TestGetDefault(t *testing.T) {
	tests := []struct {
		name string
		it   item
		want any
	}{
		{name: "StringArgument", it: &StringArgument{Name: "a", Default: "x"}, want: "x"},
		{name: "IntOption", it: &IntOption{Long: "--n", Default: 7}, want: 7},
		{name: "BoolOption", it: &BoolOption{Long: "--on", Default: true}, want: true},
		{name: "StringEnvVar", it: &StringEnvVar{Name: "APP_X", Default: "y"}, want: "y"},
		{name: "unset is the zero value", it: &IntArgument{Name: "n"}, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.it.GetDefault(); got != tt.want {
				t.Errorf("GetDefault() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestHelpString(t *testing.T) {
	tests := []struct {
		name string
		cmd  Command
		want []string
	}{
		{
			name: "minimal usage",
			cmd:  Command{Name: "myprogram"},
			want: []string{
				"Usage: myprogram [options]",
				"",
				"Options:",
				"  -h, --help   Print help",
			},
		},
		{
			name: "version, no options",
			cmd: Command{
				Name:    "myprogram",
				Version: "0.1.0",
			},
			want: []string{
				"Usage: myprogram [options]",
				"",
				"Options:",
				"  -h, --help      Print help",
				"  -v, --version   Print version",
			},
		},
		{
			name: "description, no options",
			cmd: Command{
				Name:        "myprogram",
				Description: "My foo bar program",
			},
			want: []string{
				"Usage: myprogram [options]",
				"",
				"My foo bar program",
				"",
				"Options:",
				"  -h, --help   Print help",
			},
		},
		{
			name: "single option with both Flag and Long",
			cmd: Command{
				Name:    "myprogram",
				Version: "0.1.0",
				Options: []Option{
					&StringOption{Flag: "-f", Long: "--file", Help: "Specify file name"},
				},
			},
			want: []string{
				"Usage: myprogram [options]",
				"",
				"Options:",
				"  -f, --file <FILE>   Specify file name",
				"  -h, --help          Print help",
				"  -v, --version       Print version",
			},
		},
		{
			name: "flag-only and long-only options mixed with a full one",
			cmd: Command{
				Name:    "myprogram",
				Version: "0.1.0",
				Options: []Option{
					&StringOption{Flag: "-f", Help: "Enable foo mode"},
					&StringOption{Flag: "-b", Long: "--bar", Help: "Enable bar mode"},
					&StringOption{Long: "--grault", Help: "Enable grault mode"},
				},
			},
			want: []string{
				"Usage: myprogram [options]",
				"",
				"Options:",
				"  -f  <F>                 Enable foo mode",
				"  -b, --bar <BAR>         Enable bar mode",
				"      --grault <GRAULT>   Enable grault mode",
				"  -h, --help              Print help",
				"  -v, --version           Print version",
			},
		},
		{
			name: "single argument, no options",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux", Help: "The qux to operate on"}},
			},
			want: []string{
				"Usage: myprogram [options] [qux]",
				"",
				"Arguments:",
				"  qux          The qux to operate on",
				"",
				"Options:",
				"  -h, --help   Print help",
			},
		},
		{
			// An Argument row has no "-x, " flag column, so its help text
			// must still land in the same output column as an Option row's.
			name: "argument and option aligned to one shared column",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux", Help: "The qux to operate on"}},
				Options:   []Option{&StringOption{Flag: "-b", Long: "--bar", Help: "Set the bar value"}},
			},
			want: []string{
				"Usage: myprogram [options] [qux]",
				"",
				"Arguments:",
				"  qux               The qux to operate on",
				"",
				"Options:",
				"  -b, --bar <BAR>   Set the bar value",
				"  -h, --help        Print help",
			},
		},
		{
			name: "multiple arguments, declaration order, uneven widths",
			cmd: Command{
				Name: "myprogram",
				Arguments: []Argument{
					&StringArgument{Name: "qux", Help: "The qux to operate on"},
					&StringArgument{Name: "bazlong", Help: "The bazlong to operate on"},
				},
			},
			want: []string{
				"Usage: myprogram [options] [qux] [bazlong]",
				"",
				"Arguments:",
				"  qux          The qux to operate on",
				"  bazlong      The bazlong to operate on",
				"",
				"Options:",
				"  -h, --help   Print help",
			},
		},
		{
			name: "a long argument name widens the shared column past every option",
			cmd: Command{
				Name:      "myprogram",
				Version:   "0.1.0",
				Arguments: []Argument{&StringArgument{Name: "argumentwithalongname", Help: "Wide one"}},
				Options:   []Option{&StringOption{Flag: "-f", Help: "Enable foo mode"}},
			},
			want: []string{
				"Usage: myprogram [options] [argumentwithalongname]",
				"",
				"Arguments:",
				"  argumentwithalongname   Wide one",
				"",
				"Options:",
				"  -f  <F>                 Enable foo mode",
				"  -h, --help              Print help",
				"  -v, --version           Print version",
			},
		},
		{
			name: "env var from an Option",
			cmd: Command{
				Name:    "myprogram",
				Options: []Option{&StringOption{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "PROGRAM_BAR"}},
			},
			want: []string{
				"Usage: myprogram [options]",
				"",
				"Options:",
				"  -b, --bar <BAR>   Set the bar value [env: PROGRAM_BAR]",
				"  -h, --help        Print help",
				"",
				"Environment Variables:",
				"  PROGRAM_BAR       Set the bar value",
			},
		},
		{
			name: "env var from an Argument",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux", Help: "The qux to operate on", Env: "PROGRAM_QUX"}},
			},
			want: []string{
				"Usage: myprogram [options] [qux]",
				"",
				"Arguments:",
				"  qux           The qux to operate on [env: PROGRAM_QUX]",
				"",
				"Options:",
				"  -h, --help    Print help",
				"",
				"Environment Variables:",
				"  PROGRAM_QUX   The qux to operate on",
			},
		},
		{
			// The section lists Arguments before Options.
			name: "env vars from both an Argument and an Option, Arguments first",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux", Help: "The qux to operate on", Env: "APP_QUX"}},
				Options:   []Option{&StringOption{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "APP_BAR"}},
			},
			want: []string{
				"Usage: myprogram [options] [qux]",
				"",
				"Arguments:",
				"  qux               The qux to operate on [env: APP_QUX]",
				"",
				"Options:",
				"  -b, --bar <BAR>   Set the bar value [env: APP_BAR]",
				"  -h, --help        Print help",
				"",
				"Environment Variables:",
				"  APP_QUX           The qux to operate on",
				"  APP_BAR           Set the bar value",
			},
		},
		{
			name: "a long env var name widens the shared column past everything else",
			cmd: Command{
				Name: "myprogram",
				Options: []Option{
					&StringOption{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "PROGRAM_REALLY_LONG_BAR_NAME"},
				},
			},
			want: []string{
				"Usage: myprogram [options]",
				"",
				"Options:",
				"  -b, --bar <BAR>                Set the bar value [env: PROGRAM_REALLY_LONG_BAR_NAME]",
				"  -h, --help                     Print help",
				"",
				"Environment Variables:",
				"  PROGRAM_REALLY_LONG_BAR_NAME   Set the bar value",
			},
		},
		{
			name: "non-zero defaults are shown after the description and the env note",
			cmd: Command{
				Name: "myprogram",
				Options: []Option{
					&StringOption{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "PROGRAM_BAR", Default: "default-bar"},
					&IntOption{Long: "--count", Help: "How many", Default: 0},
				},
			},
			want: []string{
				"Usage: myprogram [options]",
				"",
				"Options:",
				"  -b, --bar <BAR>       Set the bar value [env: PROGRAM_BAR] [default: \"default-bar\"]",
				"      --count <COUNT>   How many",
				"  -h, --help            Print help",
				"",
				"Environment Variables:",
				"  PROGRAM_BAR           Set the bar value",
			},
		},
		{
			name: "only the Arguments/Options with Env show up in Environment Variables",
			cmd: Command{
				Name: "myprogram",
				Arguments: []Argument{
					&StringArgument{Name: "qux", Help: "The qux to operate on", Env: "APP_QUX"},
					&StringArgument{Name: "baz", Help: "No env for this one"},
				},
				Options: []Option{
					&StringOption{Flag: "-f", Help: "No env for this one either"},
					&StringOption{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "APP_BAR"},
				},
			},
			want: []string{
				"Usage: myprogram [options] [qux] [baz]",
				"",
				"Arguments:",
				"  qux               The qux to operate on [env: APP_QUX]",
				"  baz               No env for this one",
				"",
				"Options:",
				"  -f  <F>           No env for this one either",
				"  -b, --bar <BAR>   Set the bar value [env: APP_BAR]",
				"  -h, --help        Print help",
				"",
				"Environment Variables:",
				"  APP_QUX           The qux to operate on",
				"  APP_BAR           Set the bar value",
			},
		},
		{
			// The section's order end to end: Arguments, then Options,
			// then standalone EnvVars last.
			name: "standalone EnvVars listed after Argument/Option env vars",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux", Help: "The qux to operate on", Env: "APP_QUX"}},
				Options:   []Option{&StringOption{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "APP_BAR"}},
				EnvVars:   []EnvVar{&StringEnvVar{Name: "APP_DEBUG", Help: "Enable debug logging"}},
			},
			want: []string{
				"Usage: myprogram [options] [qux]",
				"",
				"Arguments:",
				"  qux               The qux to operate on [env: APP_QUX]",
				"",
				"Options:",
				"  -b, --bar <BAR>   Set the bar value [env: APP_BAR]",
				"  -h, --help        Print help",
				"",
				"Environment Variables:",
				"  APP_QUX           The qux to operate on",
				"  APP_BAR           Set the bar value",
				"  APP_DEBUG         Enable debug logging",
			},
		},
		{
			// -v is taken, so the version built-in is listed as -V.
			name: "user claims -v: version built-in listed as -V",
			cmd: Command{
				Name:    "myprogram",
				Version: "0.1.0",
				Options: []Option{
					&StringOption{Flag: "-v", Long: "--verbose", Help: "Verbose output"},
				},
			},
			want: []string{
				"Usage: myprogram [options]",
				"",
				"Options:",
				"  -v, --verbose <VERBOSE>   Verbose output",
				"  -h, --help                Print help",
				"  -V, --version             Print version",
			},
		},
		{
			// Suppression case: the user claims both of the help
			// built-in's forms, so the built-in is not yielded and
			// there is no duplicate -h row. The Options column width
			// is still driven by "--help" (the user's own Long), so
			// the alignment is unchanged from the usual three-column
			// case.
			name: "user claims both help forms: no built-in help row, no duplicate",
			cmd: Command{
				Name: "myprogram",
				Options: []Option{
					&StringOption{Flag: "-h", Long: "--help", Help: "Show this message"},
				},
			},
			want: []string{
				"Usage: myprogram [options]",
				"",
				"Options:",
				"  -h, --help <HELP>   Show this message",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := strings.Join(tt.want, "\n") + "\n"

			if got := tt.cmd.HelpString(); got != want {
				t.Errorf("HelpString() =\n%q\nwant\n%q", got, want)
			}
		})
	}
}

func TestPrintHelp(t *testing.T) {
	var buf bytes.Buffer
	c := &Command{Name: "myprogram", Version: "0.1.0", Stdout: &buf}

	c.PrintHelp()

	if got, want := buf.String(), c.HelpString(); got != want {
		t.Errorf("PrintHelp() wrote %q, want %q", got, want)
	}
}

func TestVersionString(t *testing.T) {
	c := Command{Name: "myprogram", Version: "0.1.0"}
	if got, want := c.VersionString(), "myprogram 0.1.0"; got != want {
		t.Errorf("VersionString() = %q, want %q", got, want)
	}

	c = Command{Name: "myprogram"}
	if got, want := c.VersionString(), "myprogram "; got != want {
		t.Errorf("VersionString() = %q, want %q", got, want)
	}
}

func TestPrintVersion(t *testing.T) {
	var buf bytes.Buffer
	c := &Command{Name: "myprogram", Version: "0.1.0", Stdout: &buf}

	c.PrintVersion()

	if got, want := buf.String(), "myprogram 0.1.0\n"; got != want {
		t.Errorf("PrintVersion() wrote %q, want %q", got, want)
	}
}

// TestParse_Resolution covers Env/Default resolution, which runs after
// the token loop: only items the command line did not set
// are resolved, and a failed parse leaves every Dest untouched.
func TestParse_Resolution(t *testing.T) {
	t.Run("command line wins and its item's bad env value is never read", func(t *testing.T) {
		var i int
		t.Setenv("APP_NUM", "not-a-number")
		c := &Command{
			Name:    "myprogram",
			Options: []Option{&IntOption{Flag: "-n", Env: "APP_NUM", Dest: &i, Default: 7}},
		}
		if err := c.Parse([]string{"-n", "5"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if i != 5 {
			t.Errorf("Dest = %d, want 5", i)
		}
	})

	t.Run("a positional Argument set on the command line beats its Env", func(t *testing.T) {
		var s string
		t.Setenv("APP_QUX", "from-env")
		c := &Command{
			Name:      "myprogram",
			Arguments: []Argument{&StringArgument{Name: "qux", Env: "APP_QUX", Dest: &s, Default: "d"}},
		}
		if err := c.Parse([]string{"typed"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if s != "typed" {
			t.Errorf("Dest = %q, want %q", s, "typed")
		}
	})

	t.Run("an unset Argument still gets its bad Env reported", func(t *testing.T) {
		var i int
		var buf bytes.Buffer
		t.Setenv("APP_N", "nope")
		c := &Command{
			Name:      "myprogram",
			Arguments: []Argument{&IntArgument{Name: "n", Env: "APP_N", Dest: &i}},
			Stderr:    &buf,
		}
		err := c.Parse(nil)
		if err == nil {
			t.Fatal("Parse() = nil, want error")
		}
		if err.ExitCode() != 2 {
			t.Errorf("ExitCode() = %d, want 2", err.ExitCode())
		}
	})

	t.Run("a token error leaves every Dest untouched", func(t *testing.T) {
		s, n := "keep", 99
		var buf bytes.Buffer
		t.Setenv("APP_STR", "from-env")
		c := &Command{
			Name: "myprogram",
			Options: []Option{
				&StringOption{Flag: "-s", Env: "APP_STR", Dest: &s, Default: "d"},
				&IntOption{Flag: "-n", Dest: &n, Default: 7},
			},
			Stderr: &buf,
		}
		if err := c.Parse([]string{"--nope"}); err == nil {
			t.Fatal("Parse() = nil, want error")
		}
		if s != "keep" || n != 99 {
			t.Errorf("Dests = %q, %d; want them untouched (%q, %d)", s, n, "keep", 99)
		}
	})

	t.Run("--help wins over a bad Env value", func(t *testing.T) {
		var i int
		var out bytes.Buffer
		t.Setenv("APP_NUM", "nope")
		c := &Command{
			Name:    "myprogram",
			Options: []Option{&IntOption{Flag: "-n", Env: "APP_NUM", Dest: &i}},
			Stdout:  &out,
		}
		if err := c.Parse([]string{"--help"}); !errors.Is(err, ErrHelp) {
			t.Errorf("Parse() = %v, want ErrHelp", err)
		}
	})

	t.Run("standalone EnvVar: env, else default", func(t *testing.T) {
		var a, b string
		t.Setenv("APP_A", "from-env")
		c := &Command{
			Name: "myprogram",
			EnvVars: []EnvVar{
				&StringEnvVar{Name: "APP_A", Dest: &a, Default: "da"},
				&StringEnvVar{Name: "APP_SURELY_UNSET_B", Dest: &b, Default: "db"},
			},
		}
		if err := c.Parse(nil); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if a != "from-env" || b != "db" {
			t.Errorf("Dests = %q, %q; want %q, %q", a, b, "from-env", "db")
		}
	})
}

// TestParse_Required covers Required Arguments, Options, and EnvVars:
// Parse fails with a code-2 usage error when one is resolved from
// neither the command line nor Env (for an EnvVar, from the environment
// alone), and succeeds -- exactly like a non-Required item -- when
// either one supplies it.
func TestParse_Required(t *testing.T) {
	t.Run("a missing Required Option is a usage error", func(t *testing.T) {
		var s string
		var buf bytes.Buffer
		c := &Command{
			Name:    "myprogram",
			Options: []Option{&StringOption{Flag: "-s", Dest: &s, Required: true}},
			Stderr:  &buf,
		}
		err := c.Parse(nil)
		if err == nil || err.ExitCode() != 2 {
			t.Fatalf("Parse() = %v, want a code-2 error", err)
		}
	})

	t.Run("a missing Required Argument is a usage error", func(t *testing.T) {
		var s string
		var buf bytes.Buffer
		c := &Command{
			Name:      "myprogram",
			Arguments: []Argument{&StringArgument{Name: "qux", Dest: &s, Required: true}},
			Stderr:    &buf,
		}
		err := c.Parse(nil)
		if err == nil || err.ExitCode() != 2 {
			t.Fatalf("Parse() = %v, want a code-2 error", err)
		}
	})

	t.Run("a Required Option set on the command line is not an error", func(t *testing.T) {
		var s string
		c := &Command{
			Name:    "myprogram",
			Options: []Option{&StringOption{Flag: "-s", Dest: &s, Required: true}},
		}
		if err := c.Parse([]string{"-s", "hi"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if s != "hi" {
			t.Errorf("Dest = %q, want %q", s, "hi")
		}
	})

	t.Run("a Required Argument set on the command line is not an error", func(t *testing.T) {
		var s string
		c := &Command{
			Name:      "myprogram",
			Arguments: []Argument{&StringArgument{Name: "qux", Dest: &s, Required: true}},
		}
		if err := c.Parse([]string{"hi"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if s != "hi" {
			t.Errorf("Dest = %q, want %q", s, "hi")
		}
	})

	t.Run("a Required Option resolved via Env is not an error", func(t *testing.T) {
		var s string
		t.Setenv("APP_S", "from-env")
		c := &Command{
			Name:    "myprogram",
			Options: []Option{&StringOption{Flag: "-s", Env: "APP_S", Dest: &s, Required: true}},
		}
		if err := c.Parse(nil); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if s != "from-env" {
			t.Errorf("Dest = %q, want %q", s, "from-env")
		}
	})

	t.Run("a missing Required EnvVar is a usage error", func(t *testing.T) {
		var s string
		var buf bytes.Buffer
		c := &Command{
			Name:    "myprogram",
			EnvVars: []EnvVar{&StringEnvVar{Name: "APP_SURELY_UNSET_REQ", Dest: &s, Required: true}},
			Stderr:  &buf,
		}
		err := c.Parse(nil)
		if err == nil || err.ExitCode() != 2 {
			t.Fatalf("Parse() = %v, want a code-2 error", err)
		}
	})

	t.Run("a Required EnvVar set in the environment is not an error", func(t *testing.T) {
		t.Setenv("APP_REQ", "from-env")
		var s string
		c := &Command{
			Name:    "myprogram",
			EnvVars: []EnvVar{&StringEnvVar{Name: "APP_REQ", Dest: &s, Required: true}},
		}
		if err := c.Parse(nil); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if s != "from-env" {
			t.Errorf("Dest = %q, want %q", s, "from-env")
		}
	})
}

// TestSetDefault covers every typed Argument/Option's setDefault: it
// assigns Default straight to Dest -- the zero value included -- and
// does nothing when Dest is unset.
func TestSetDefault(t *testing.T) {
	t.Run("assigns Default to Dest", func(t *testing.T) {
		s1, s2, i1, i2, b := "old", "old", 1, 1, true
		items := []interface{ SetDefault() error }{
			&StringArgument{Dest: &s1, Default: "new"},
			&StringOption{Flag: "-s", Dest: &s2, Default: "new"},
			&IntArgument{Dest: &i1, Default: 7},
			&IntOption{Flag: "-i", Dest: &i2, Default: 7},
			&BoolOption{Flag: "-b", Dest: &b, Default: false},
		}
		for _, it := range items {
			it.SetDefault()
		}
		if s1 != "new" || s2 != "new" || i1 != 7 || i2 != 7 || b {
			t.Errorf("Dests = %q, %q, %d, %d, %v; want new, new, 7, 7, false", s1, s2, i1, i2, b)
		}
	})

	t.Run("a nil Dest is a no-op", func(t *testing.T) {
		items := []interface{ SetDefault() error }{
			&StringArgument{Default: "x"},
			&StringOption{Flag: "-s", Default: "x"},
			&IntArgument{Default: 1},
			&IntOption{Flag: "-i", Default: 1},
			&BoolOption{Flag: "-b", Default: true},
		}
		for _, it := range items {
			it.SetDefault() // a nil Dest must not panic
		}
	})
}

func TestParse_ArgumentDefault(t *testing.T) {
	var s string
	var n int
	c := &Command{
		Name: "myprogram",
		Arguments: []Argument{
			&StringArgument{Name: "qux", Dest: &s, Default: "d"},
			&IntArgument{Name: "n", Dest: &n, Default: 7},
		},
	}
	if err := c.Parse(nil); err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}
	if s != "d" || n != 7 {
		t.Errorf("Dests = %q, %d; want %q, %d", s, n, "d", 7)
	}
}

func TestLookup(t *testing.T) {
	newCmd := func() *Command {
		return &Command{
			Name:      "app",
			Arguments: []Argument{&StringArgument{Name: "qux"}},
			Options: []Option{
				&StringOption{Long: "--bar", Default: "def"},
				&IntOption{Flag: "-n", Name: "count", Env: "APP_COUNT"},
				&BoolOption{Flag: "-f", Long: "--foo"},
			},
			EnvVars: []EnvVar{&StringEnvVar{Name: "APP_TOKEN"}},
			Stdout:  io.Discard,
			Stderr:  io.Discard,
		}
	}

	t.Run("values resolved by Parse, with no Dest", func(t *testing.T) {
		t.Setenv("APP_COUNT", "3")
		t.Setenv("APP_TOKEN", "secret")
		c := newCmd()
		if err := c.Parse([]string{"-f", "thing"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		want := map[string]any{
			"qux":       "thing",
			"bar":       "def",
			"count":     3,
			"foo":       true,
			"APP_TOKEN": "secret",
		}
		for name, w := range want {
			got, ok := c.Lookup(name)
			if !ok || got != w {
				t.Errorf("Lookup(%q) = %#v, %v; want %#v, true", name, got, ok, w)
			}
		}
	})

	t.Run("the command line wins and Dest gets the same value", func(t *testing.T) {
		var bar string
		c := newCmd()
		c.Options[0].(*StringOption).Dest = &bar
		if err := c.Parse([]string{"--bar=cli", "thing"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		got, ok := c.Lookup("bar")
		if !ok || got != "cli" || bar != "cli" {
			t.Errorf("Lookup(\"bar\") = %#v, %v, Dest = %q; want \"cli\", true, \"cli\"", got, ok, bar)
		}
	})

	t.Run("before Parse it is the zero value", func(t *testing.T) {
		got, ok := newCmd().Lookup("count")
		if !ok || got != 0 {
			t.Errorf("Lookup(\"count\") = %#v, %v; want 0, true", got, ok)
		}
	})

	t.Run("an unknown name", func(t *testing.T) {
		got, ok := newCmd().Lookup("nope")
		if ok || got != nil {
			t.Errorf("Lookup(\"nope\") = %#v, %v; want nil, false", got, ok)
		}
	})

	t.Run("the built-ins are not declared", func(t *testing.T) {
		if _, ok := newCmd().Lookup("help"); ok {
			t.Error("Lookup(\"help\") found the built-in -h/--help")
		}
	})
}

// --- Commands ----------------------------------------------------------------

func TestValidate_Commands(t *testing.T) {
	tests := []struct {
		name string
		cmd  *Command
		want string // substring of the error; "" means valid
	}{
		{"valid", &Command{Name: "app", Commands: []*Command{{Name: "serve"}, {Name: "build"}}}, ""},
		{"Arguments next to Commands", &Command{
			Name:      "app",
			Arguments: []Argument{&StringArgument{Name: "x"}},
			Commands:  []*Command{{Name: "serve"}},
		}, "both Arguments and Commands"},
		{"invalid subcommand", &Command{Name: "app", Commands: []*Command{{}}}, "Name must not be empty"},
		{"duplicate name", &Command{Name: "app", Commands: []*Command{{Name: "serve"}, {Name: "serve"}}}, `duplicate subcommand "serve"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cmd.Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Validate() = %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

func TestParse_Commands(t *testing.T) {
	newApp := func(verbose *bool, port *int, out, errOut io.Writer) *Command {
		return &Command{
			Name:    "app",
			Options: []Option{&BoolOption{Flag: "-V", Long: "--verbose", Dest: verbose}},
			Commands: []*Command{
				{
					Name:        "serve",
					Description: "Run the server",
					Options:     []Option{&IntOption{Long: "--port", Default: 80, Dest: port}},
				},
				{Name: "build"},
			},
			Stdout: out,
			Stderr: errOut,
		}
	}

	t.Run("the subcommand parses the rest", func(t *testing.T) {
		var verbose bool
		var port int
		app := newApp(&verbose, &port, io.Discard, io.Discard)
		if err := app.Parse([]string{"-V", "serve", "--port", "8080"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if !verbose || port != 8080 {
			t.Errorf("verbose = %v, port = %d; want true, 8080", verbose, port)
		}
	})

	t.Run("the parent resolves its own items first", func(t *testing.T) {
		var errOut bytes.Buffer
		app := newApp(new(bool), new(int), io.Discard, &errOut)
		app.EnvVars = []EnvVar{&StringEnvVar{Name: "APP_TOKEN_UNSET", Required: true}}
		os.Unsetenv("APP_TOKEN_UNSET")
		if err := app.Parse([]string{"build"}); err == nil || err.ExitCode() != 2 {
			t.Fatalf("Parse() = %v, want exit 2", err)
		}
		if !strings.Contains(errOut.String(), `"APP_TOKEN_UNSET" is required`) {
			t.Errorf("stderr = %q", errOut.String())
		}
	})

	errTests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown command", []string{"nope"}, `error: unknown command "nope"`},
		{"missing command", []string{"-V"}, "error: a command is required"},
		{"error inside a subcommand", []string{"serve", "--bogus"}, "Usage: app serve [options]"},
	}
	for _, tt := range errTests {
		t.Run(tt.name, func(t *testing.T) {
			var errOut bytes.Buffer
			app := newApp(new(bool), new(int), io.Discard, &errOut)
			app.Commands[0].Stderr = &errOut // a subcommand has its own streams
			if err := app.Parse(tt.args); err == nil || err.ExitCode() != 2 {
				t.Fatalf("Parse() = %v, want exit 2", err)
			}
			if !strings.Contains(errOut.String(), tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", errOut.String(), tt.want)
			}
		})
	}
}

func TestCommand_Action(t *testing.T) {
	var ran []string
	record := func(name string) CommandAction {
		return func(c *Command) *Error {
			ran = append(ran, name)
			return nil
		}
	}
	newApp := func() *Command {
		ran = nil
		return &Command{
			Name:   "app",
			Action: record("app"),
			Commands: []*Command{
				{Name: "serve", Action: record("serve")},
				{Name: "build", Action: record("build")},
				{Name: "noop"},
			},
		}
	}

	t.Run("only the selected subcommand runs", func(t *testing.T) {
		if err := newApp().Parse([]string{"build"}); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if !slices.Equal(ran, []string{"build"}) {
			t.Errorf("ran = %v, want [build]", ran)
		}
	})

	t.Run("a subcommand without Action is fine", func(t *testing.T) {
		if err := newApp().Parse([]string{"noop"}); err != nil || len(ran) != 0 {
			t.Errorf("Parse() = %v, ran = %v; want nil, none", err, ran)
		}
	})

	t.Run("a Command without Commands runs its own", func(t *testing.T) {
		app := &Command{Name: "app", Action: record("app")}
		ran = nil
		if err := app.Parse(nil); err != nil || !slices.Equal(ran, []string{"app"}) {
			t.Errorf("Parse() = %v, ran = %v; want nil, [app]", err, ran)
		}
	})

	t.Run("it does not run when parsing fails", func(t *testing.T) {
		app := newApp()
		app.Stderr = io.Discard
		app.Commands[0].Stderr = io.Discard
		if err := app.Parse([]string{"serve", "--bogus"}); err == nil || len(ran) != 0 {
			t.Errorf("Parse() = %v, ran = %v; want an error, none", err, ran)
		}
	})

	t.Run("its error is what Parse returns", func(t *testing.T) {
		want := Errorf(3, "boom")
		app := newApp()
		app.Commands[0].Action = func(c *Command) *Error { return want }
		if err := app.Parse([]string{"serve"}); err != want {
			t.Errorf("Parse() = %v, want %v", err, want)
		}
	})
}

func TestHelpString_Commands(t *testing.T) {
	app := &Command{
		Name:        "app",
		Description: "My app",
		Commands:    []*Command{{Name: "serve", Description: "Run the server"}, {Name: "build", Description: "Build it"}},
	}
	want := strings.Join([]string{
		"Usage: app [options] <command> [args]",
		"",
		"My app",
		"",
		"Options:",
		"  -h, --help   Print help",
		"",
		"Commands:",
		"  serve        Run the server",
		"  build        Build it",
		"",
	}, "\n")
	if got := app.HelpString(); got != want {
		t.Errorf("HelpString() =\n%s\nwant\n%s", got, want)
	}
}

// failingDefaultOption is an Option implemented outside OptionOf whose
// default can fail, the case SetDefault's error exists for.
type failingDefaultOption struct {
	*StringOption
	err error
}

func (o *failingDefaultOption) SetDefault() error {
	return o.err
}

func TestParse_OptionSetDefaultError(t *testing.T) {
	newCmd := func(err error) *Command {
		return &Command{
			Name:   "myprogram",
			Stderr: io.Discard,
			Options: []Option{
				&failingDefaultOption{StringOption: &StringOption{Long: "--site"}, err: err},
			},
		}
	}

	t.Run("a failing default is a usage error naming the option", func(t *testing.T) {
		err := newCmd(errors.New("no site")).Parse(nil)
		if err == nil || err.ExitCode() != 2 {
			t.Fatalf("Parse() = %v, want a usage error", err)
		}
		if !strings.Contains(err.Error(), "site") || !strings.Contains(err.Error(), "no site") {
			t.Errorf("Parse().Error() = %q, want it to name the option and the cause", err.Error())
		}
	})

	t.Run("not consulted when the flag is set", func(t *testing.T) {
		if err := newCmd(errors.New("no site")).Parse([]string{"--site", "a"}); err != nil {
			t.Errorf("Parse() = %v, want nil", err)
		}
	})
}
