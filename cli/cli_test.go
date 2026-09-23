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

// --- Argument, Option, EnvVar, Command ---------------------------------------

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

// TestSet covers every typed Argument/Option's set: it converts the
// string into Dest's Go type, returns the conversion error unchanged
// for a bad one (leaving Dest as it was), and does nothing at all --
// returning nil, even for input that wouldn't parse -- when Dest is
// unset.
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
			{"StringArgument", (&StringArgument{Dest: &s1}).set, "x", func() bool { return s1 == "x" }},
			{"StringOption", (&StringOption{Flag: "-s", Dest: &s2}).set, "y", func() bool { return s2 == "y" }},
			{"IntArgument", (&IntArgument{Dest: &i1}).set, "42", func() bool { return i1 == 42 }},
			{"IntOption", (&IntOption{Flag: "-i", Dest: &i2}).set, "-7", func() bool { return i2 == -7 }},
			{"BoolOption", (&BoolOption{Flag: "-b", Dest: &b}).set, "true", func() bool { return b }},
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
			{"IntArgument", (&IntArgument{Dest: &i}).set, "invalid integer value"},
			{"IntOption", (&IntOption{Flag: "-i", Dest: &i}).set, "invalid integer value"},
			{"BoolOption", (&BoolOption{Flag: "-b", Dest: &b}).set, "invalid boolean value"},
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

	t.Run("a nil Dest is a no-op", func(t *testing.T) {
		tests := []struct {
			name string
			set  func(string) error
		}{
			{"StringArgument", (&StringArgument{}).set},
			{"StringOption", (&StringOption{Flag: "-s"}).set},
			{"IntArgument", (&IntArgument{}).set},
			{"IntOption", (&IntOption{Flag: "-i"}).set},
			{"BoolOption", (&BoolOption{Flag: "-b"}).set},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if err := tt.set("nope"); err != nil {
					t.Errorf("set() = %v, want nil", err)
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
		if a[i].name() != b[i].name() ||
			a[i].flag() != b[i].flag() ||
			a[i].long() != b[i].long() ||
			a[i].help() != b[i].help() ||
			a[i].env() != b[i].env() ||
			(a[i].action() == nil) != (b[i].action() == nil) {
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
		if got[1].flag() != "-v" || got[1].long() != "--version" {
			t.Errorf("second yielded Option = %+v, want the version built-in", got[1])
		}
	})
}

func TestAllEnvVars(t *testing.T) {
	tests := []struct {
		name string
		cmd  Command
		want []EnvVar
	}{
		{
			name: "no Env declared anywhere",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux"}},
				Options:   []Option{&StringOption{Flag: "-f"}},
			},
			want: nil,
		},
		{
			name: "only an Argument declares Env",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux", Help: "The qux to operate on", Env: "PROGRAM_QUX"}},
			},
			want: []EnvVar{
				{Name: "PROGRAM_QUX", Help: "The qux to operate on"},
			},
		},
		{
			name: "only an Option declares Env",
			cmd: Command{
				Name:    "myprogram",
				Options: []Option{&StringOption{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "PROGRAM_BAR"}},
			},
			want: []EnvVar{
				{Name: "PROGRAM_BAR", Help: "Set the bar value"},
			},
		},
		{
			// Documented order in allEnvVars: Arguments before Options.
			name: "env vars from both, Arguments first",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux", Help: "The qux to operate on", Env: "APP_QUX"}},
				Options:   []Option{&StringOption{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "APP_BAR"}},
			},
			want: []EnvVar{
				{Name: "APP_QUX", Help: "The qux to operate on"},
				{Name: "APP_BAR", Help: "Set the bar value"},
			},
		},
		{
			name: "only the Arguments/Options that declare Env are yielded",
			cmd: Command{
				Name: "myprogram",
				Arguments: []Argument{
					&StringArgument{Name: "qux", Help: "The qux to operate on", Env: "APP_QUX"},
					&StringArgument{Name: "quux", Help: "No env for this one"},
				},
				Options: []Option{
					&StringOption{Flag: "-f", Help: "No env for this one either"},
					&StringOption{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "APP_BAR"},
				},
			},
			want: []EnvVar{
				{Name: "APP_QUX", Help: "The qux to operate on"},
				{Name: "APP_BAR", Help: "Set the bar value"},
			},
		},
		{
			name: "only a standalone EnvVar",
			cmd: Command{
				Name:    "myprogram",
				EnvVars: []EnvVar{{Name: "APP_DEBUG", Help: "Enable debug logging"}},
			},
			want: []EnvVar{
				{Name: "APP_DEBUG", Help: "Enable debug logging"},
			},
		},
		{
			// Documented order in allEnvVars: Arguments, then Options,
			// then standalone EnvVars last.
			name: "Arguments, Options, and standalone EnvVars all present, in that order",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux", Help: "The qux to operate on", Env: "APP_QUX"}},
				Options:   []Option{&StringOption{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "APP_BAR"}},
				EnvVars:   []EnvVar{{Name: "APP_DEBUG", Help: "Enable debug logging"}},
			},
			want: []EnvVar{
				{Name: "APP_QUX", Help: "The qux to operate on"},
				{Name: "APP_BAR", Help: "Set the bar value"},
				{Name: "APP_DEBUG", Help: "Enable debug logging"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slices.Collect(tt.cmd.allEnvVars())
			if !slices.Equal(got, tt.want) {
				t.Errorf("allEnvVars() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestAllEnvVars_StopsEarly covers all three early-exit points in
// allEnvVars -- the "if !yield(...) { return }" inside the Arguments
// loop, the one inside the Options loop, and the one inside the
// standalone EnvVars loop -- the same reason TestOptions_StopsEarly
// needed more than a single stopping point.
func TestAllEnvVars_StopsEarly(t *testing.T) {
	cmd := Command{
		Name:      "myprogram",
		Arguments: []Argument{&StringArgument{Name: "qux", Help: "The qux to operate on", Env: "APP_QUX"}},
		Options:   []Option{&StringOption{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "APP_BAR"}},
		EnvVars:   []EnvVar{{Name: "APP_DEBUG", Help: "Enable debug logging"}},
	}

	tests := []struct {
		name      string
		stopAfter int
		wantNames []string
	}{
		{
			name:      "stops inside the Arguments loop",
			stopAfter: 1,
			wantNames: []string{"APP_QUX"},
		},
		{
			name:      "stops inside the Options loop",
			stopAfter: 2,
			wantNames: []string{"APP_QUX", "APP_BAR"},
		},
		{
			name:      "stops inside the EnvVars loop",
			stopAfter: 3,
			wantNames: []string{"APP_QUX", "APP_BAR", "APP_DEBUG"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			cmd.allEnvVars()(func(env EnvVar) bool {
				got = append(got, env.Name)
				return len(got) < tt.stopAfter
			})

			if !slices.Equal(got, tt.wantNames) {
				t.Errorf("allEnvVars() stopped after %d = %+v, want %+v", tt.stopAfter, got, tt.wantNames)
			}
		})
	}
}

// --- Validate ----------------------------------------------------------------

// TestValidate covers what belongs to Validate itself: the Command.Name
// check (not part of either helper) and that a failure from either
// helper propagates as Validate's own error. Detailed Argument/Option
// edge cases live in TestValidateArguments and TestValidateOptions,
// next to the functions they exercise.
func TestValidate(t *testing.T) {
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
		{
			name: "propagates a validateArguments failure",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: ""}},
			},
			wantErr: true,
		},
		{
			name: "propagates a validateOptions failure",
			cmd: Command{
				Name:    "myprogram",
				Options: []Option{&StringOption{}},
			},
			wantErr: true,
		},
		{
			name: "propagates a validateEnvVars failure",
			cmd: Command{
				Name:    "myprogram",
				EnvVars: []EnvVar{{Name: ""}},
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
			// Every Validate failure reports ExitCode() == 1 -- see its
			// doc comment.
			if err != nil && err.ExitCode() != 1 {
				t.Errorf("Validate().ExitCode() = %d, want 1", err.ExitCode())
			}
		})
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

// TestValidateEnvVars exercises (*Command).validateEnvVars directly.
// Unlike validateArguments/validateOptions, this check is inherently
// cross-slice (Arguments' Env, Options' Env, and EnvVars together), so
// it takes a full Command rather than a bare slice.
func TestValidateEnvVars(t *testing.T) {
	var s string

	tests := []struct {
		name    string
		cmd     Command
		wantErr bool
	}{
		{
			name: "no env vars anywhere",
			cmd: Command{
				Arguments: []Argument{&StringArgument{Name: "qux"}},
				Options:   []Option{&StringOption{Flag: "-f"}},
			},
		},
		{
			name: "valid, one env var per source",
			cmd: Command{
				Arguments: []Argument{&StringArgument{Name: "qux", Env: "APP_QUX"}},
				Options:   []Option{&StringOption{Flag: "-b", Env: "APP_BAR"}},
				EnvVars:   []EnvVar{{Name: "APP_DEBUG"}},
			},
		},
		{
			name:    "empty EnvVar.Name",
			cmd:     Command{EnvVars: []EnvVar{{Name: ""}}},
			wantErr: true,
		},
		{
			name: "duplicate name, both from EnvVars",
			cmd: Command{
				EnvVars: []EnvVar{{Name: "APP_DEBUG"}, {Name: "APP_DEBUG"}},
			},
			wantErr: true,
		},
		{
			// The only scenario that hits the Arguments loop's OWN
			// duplicate-return line (as opposed to the Options or
			// EnvVars loop catching a collision seen earlier).
			name: "duplicate name, both from Arguments",
			cmd: Command{
				Arguments: []Argument{
					&StringArgument{Name: "qux", Env: "APP_QUX"},
					&StringArgument{Name: "quux", Env: "APP_QUX"},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate name, Argument.Env vs Option.Env",
			cmd: Command{
				Arguments: []Argument{&StringArgument{Name: "qux", Env: "APP_QUX"}},
				Options:   []Option{&StringOption{Flag: "-b", Env: "APP_QUX"}},
			},
			wantErr: true,
		},
		{
			name: "duplicate name, Argument.Env vs EnvVars",
			cmd: Command{
				Arguments: []Argument{&StringArgument{Name: "qux", Env: "APP_QUX"}},
				EnvVars:   []EnvVar{{Name: "APP_QUX"}},
			},
			wantErr: true,
		},
		{
			name: "duplicate name, Option.Env vs EnvVars",
			cmd: Command{
				Options: []Option{&StringOption{Flag: "-b", Env: "APP_BAR"}},
				EnvVars: []EnvVar{{Name: "APP_BAR"}},
			},
			wantErr: true,
		},
		{
			name: "Default matches Dest type",
			cmd: Command{
				EnvVars: []EnvVar{{Name: "APP_DEBUG", Dest: StringVar(&s), Default: "info"}},
			},
		},
		{
			name: "Default mismatches Dest type",
			cmd: Command{
				EnvVars: []EnvVar{{Name: "APP_DEBUG", Dest: StringVar(&s), Default: 42}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cmd.validateEnvVars()
			if (err != nil) != tt.wantErr {
				t.Errorf("validateEnvVars() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// customValue is a minimal Value implementation used to verify that
// validateDefaultType skips anything other than the three built-in
// concrete types (stringValue/boolValue/intValue) it knows how to check.
type customValue struct{}

func (customValue) String() string   { return "" }
func (customValue) Set(string) error { return nil }

// failingValue is a Value whose Set always fails. Used to exercise the
// resolveEnvVar's Default Set-failure path (TestResolveEnvVar) -- validateDefaultType
// can't catch a bad Default for a custom Value ahead of time (see its
// doc comment), so Validate() lets it through and resolveEnvVar is the only
// place left to catch it.
type failingValue struct{}

func (failingValue) String() string   { return "" }
func (failingValue) Set(string) error { return errors.New("boom") }

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

// --- Parse -------------------------------------------------------------------

// TestParse covers what Parse itself does today: the Validate wiring
// (already exercised end-to-end via DefaultParse in TestDefaultParse,
// not repeated here) and step 2 of the algorithm, Default -> Env
// resolution, across Arguments, Options, and standalone EnvVars. The
// token loop (steps 3-5) doesn't exist yet, so every case here passes
// an empty args slice.
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
			EnvVars: []EnvVar{{Name: "APP_DEBUG", Dest: StringVar(&s)}},
		}

		if err := c.Parse(nil); err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}
		if s != "verbose" {
			t.Errorf("Dest = %q, want %q", s, "verbose")
		}
	})

	t.Run("an unassignable Default propagates: a code-1 *Error, nothing printed", func(t *testing.T) {
		// validateDefaultType can't check a custom Value's Default (see
		// its doc comment), so Validate() lets this through and
		// resolveEnvVar is the one that has to reject it. A custom
		// Value can only be plugged in via a standalone EnvVar now --
		// a typed Option's Dest is a native pointer, not a Value.
		var buf bytes.Buffer
		c := &Command{
			Name:    "myprogram",
			EnvVars: []EnvVar{{Name: "APP_X", Dest: failingValue{}, Default: "anything"}},
			Stderr:  &buf,
		}

		err := c.Parse(nil)
		if err == nil {
			t.Fatal("Parse() = nil, want error")
		}
		if err.ExitCode() != 1 {
			t.Errorf("ExitCode() = %d, want 1", err.ExitCode())
		}
		if buf.Len() != 0 {
			t.Errorf("Parse() wrote %q to Stderr, want nothing", buf.String())
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
		c := &Command{} // empty Name -> Validate fails
		err := c.Parse(nil)
		if err == nil {
			t.Fatal("Parse() = nil, want error")
		}
		if got, want := err.ExitCode(), 1; got != want {
			t.Errorf("ExitCode() = %d, want %d", got, want)
		}
	})
}

// TestParse_TokenLoop covers step 3-5 of the Parse algorithm: the
// actual token loop, resolving Options (built-in and user-declared
// alike, through the same flagOpts/longOpts lookup) and positional
// Arguments from a real args slice.
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
				// A boolean Dest keeps the Option from consuming a value.
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
		if seen.flag() != "-x" || seen.long() != "--extra" {
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
				&StringOption{Flag: "-x", Action: func(c *Command, opt Option, value string) *Error { return newError(2, "%s", sentinel) }},
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
// Everything else Parse will eventually do -- consuming flags,
// positional Arguments, Env values -- is Parse's own behavior, not
// something specific to DefaultParse, so those cases belong in
// TestParse, not here.
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
			// Regression guard for the "error: " prefix: it must be
			// display-only, added here, and never baked into the
			// message the underlying error itself carries.
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
					&StringArgument{Name: "quux"},
				},
			},
			want: "Usage: myprogram [options] [qux] [quux]\n",
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
				"  -f, --file      Specify file name",
				"  -h, --help      Print help",
				"  -v, --version   Print version",
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
				"  -f              Enable foo mode",
				"  -b, --bar       Enable bar mode",
				"      --grault    Enable grault mode",
				"  -h, --help      Print help",
				"  -v, --version   Print version",
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
			// Regression guard for the width+4/-4 offset: an Argument row has
			// no "-x, " flag column, so its help text must still land in the
			// same output column as an Option row's.
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
				"  qux          The qux to operate on",
				"",
				"Options:",
				"  -b, --bar    Set the bar value",
				"  -h, --help   Print help",
			},
		},
		{
			name: "multiple arguments, declaration order, uneven widths",
			cmd: Command{
				Name: "myprogram",
				Arguments: []Argument{
					&StringArgument{Name: "qux", Help: "The qux to operate on"},
					&StringArgument{Name: "quuxlong", Help: "The quuxlong to operate on"},
				},
			},
			want: []string{
				"Usage: myprogram [options] [qux] [quuxlong]",
				"",
				"Arguments:",
				"  qux          The qux to operate on",
				"  quuxlong     The quuxlong to operate on",
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
				"  -f                      Enable foo mode",
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
				"  -b, --bar     Set the bar value",
				"  -h, --help    Print help",
				"",
				"Environment Variables:",
				"  PROGRAM_BAR   Set the bar value",
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
				"  qux           The qux to operate on",
				"",
				"Options:",
				"  -h, --help    Print help",
				"",
				"Environment Variables:",
				"  PROGRAM_QUX   The qux to operate on",
			},
		},
		{
			// allEnvVars documents Arguments before Options; this is the
			// regression guard for that order.
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
				"  qux          The qux to operate on",
				"",
				"Options:",
				"  -b, --bar    Set the bar value",
				"  -h, --help   Print help",
				"",
				"Environment Variables:",
				"  APP_QUX      The qux to operate on",
				"  APP_BAR      Set the bar value",
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
				"  -b, --bar                      Set the bar value",
				"  -h, --help                     Print help",
				"",
				"Environment Variables:",
				"  PROGRAM_REALLY_LONG_BAR_NAME   Set the bar value",
			},
		},
		{
			name: "only the Arguments/Options with Env show up in Environment Variables",
			cmd: Command{
				Name: "myprogram",
				Arguments: []Argument{
					&StringArgument{Name: "qux", Help: "The qux to operate on", Env: "APP_QUX"},
					&StringArgument{Name: "quux", Help: "No env for this one"},
				},
				Options: []Option{
					&StringOption{Flag: "-f", Help: "No env for this one either"},
					&StringOption{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "APP_BAR"},
				},
			},
			want: []string{
				"Usage: myprogram [options] [qux] [quux]",
				"",
				"Arguments:",
				"  qux          The qux to operate on",
				"  quux         No env for this one",
				"",
				"Options:",
				"  -f           No env for this one either",
				"  -b, --bar    Set the bar value",
				"  -h, --help   Print help",
				"",
				"Environment Variables:",
				"  APP_QUX      The qux to operate on",
				"  APP_BAR      Set the bar value",
			},
		},
		{
			// End-to-end regression guard for allEnvVars' documented
			// order: Arguments, then Options, then standalone EnvVars
			// last.
			name: "standalone EnvVars listed after Argument/Option env vars",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{&StringArgument{Name: "qux", Help: "The qux to operate on", Env: "APP_QUX"}},
				Options:   []Option{&StringOption{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "APP_BAR"}},
				EnvVars:   []EnvVar{{Name: "APP_DEBUG", Help: "Enable debug logging"}},
			},
			want: []string{
				"Usage: myprogram [options] [qux]",
				"",
				"Arguments:",
				"  qux          The qux to operate on",
				"",
				"Options:",
				"  -b, --bar    Set the bar value",
				"  -h, --help   Print help",
				"",
				"Environment Variables:",
				"  APP_QUX      The qux to operate on",
				"  APP_BAR      Set the bar value",
				"  APP_DEBUG    Enable debug logging",
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
				"  -v, --verbose   Verbose output",
				"  -h, --help      Print help",
				"  -V, --version   Print version",
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
				"  -h, --help   Show this message",
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

// TestResolveEnvVar exercises (*Command).resolveEnvVar directly: the Env -> Default
// fallback for one item the command line did not set.
func TestResolveEnvVar(t *testing.T) {
	t.Run("nil Dest does nothing", func(t *testing.T) {
		t.Setenv("APP_X", "boom")
		c := &Command{Name: "myprogram"}
		if err := c.resolveEnvVar(nil, "APP_X", "d"); err != nil {
			t.Errorf("resolveEnvVar() = %v, want nil", err)
		}
	})

	t.Run("env wins over default", func(t *testing.T) {
		var s string
		t.Setenv("APP_X", "from-env")
		c := &Command{Name: "myprogram"}
		if err := c.resolveEnvVar(StringVar(&s), "APP_X", "d"); err != nil {
			t.Fatalf("resolveEnvVar() = %v, want nil", err)
		}
		if s != "from-env" {
			t.Errorf("Dest = %q, want %q", s, "from-env")
		}
	})

	t.Run("default when env is absent", func(t *testing.T) {
		var s string
		c := &Command{Name: "myprogram"}
		if err := c.resolveEnvVar(StringVar(&s), "APP_SURELY_UNSET_X", "d"); err != nil {
			t.Fatalf("resolveEnvVar() = %v, want nil", err)
		}
		if s != "d" {
			t.Errorf("Dest = %q, want %q", s, "d")
		}
	})

	t.Run("no env name: default applies", func(t *testing.T) {
		var s string
		c := &Command{Name: "myprogram"}
		if err := c.resolveEnvVar(StringVar(&s), "", "d"); err != nil {
			t.Fatalf("resolveEnvVar() = %v, want nil", err)
		}
		if s != "d" {
			t.Errorf("Dest = %q, want %q", s, "d")
		}
	})

	t.Run("nil default leaves Dest alone", func(t *testing.T) {
		s := "keep"
		c := &Command{Name: "myprogram"}
		if err := c.resolveEnvVar(StringVar(&s), "APP_SURELY_UNSET_X", nil); err != nil {
			t.Fatalf("resolveEnvVar() = %v, want nil", err)
		}
		if s != "keep" {
			t.Errorf("Dest = %q, want %q", s, "keep")
		}
	})

	t.Run("bad env value: ExitCode 2, printed", func(t *testing.T) {
		var i int
		var buf bytes.Buffer
		t.Setenv("APP_X", "nope")
		c := &Command{Name: "myprogram", Stderr: &buf}
		err := c.resolveEnvVar(IntVar(&i), "APP_X", nil)
		if err == nil {
			t.Fatal("resolveEnvVar() = nil, want error")
		}
		if err.ExitCode() != 2 {
			t.Errorf("ExitCode() = %d, want 2", err.ExitCode())
		}
		if buf.Len() == 0 {
			t.Error("resolveEnvVar() wrote nothing to Stderr, want the clap-style block")
		}
	})

	t.Run("unassignable default: ExitCode 1, nothing printed", func(t *testing.T) {
		var buf bytes.Buffer
		c := &Command{Name: "myprogram", Stderr: &buf}
		err := c.resolveEnvVar(failingValue{}, "", "anything")
		if err == nil {
			t.Fatal("resolveEnvVar() = nil, want error")
		}
		if err.ExitCode() != 1 {
			t.Errorf("ExitCode() = %d, want 1", err.ExitCode())
		}
		if buf.Len() != 0 {
			t.Errorf("resolveEnvVar() wrote %q to Stderr, want nothing", buf.String())
		}
	})
}

// TestParse_Resolution covers what moving Env/Default resolution after
// the token loop guarantees: only items the command line did not set
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
				{Name: "APP_A", Dest: StringVar(&a), Default: "da"},
				{Name: "APP_SURELY_UNSET_B", Dest: StringVar(&b), Default: "db"},
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

// TestSetDefault covers every typed Argument/Option's setDefault: it
// assigns Default straight to Dest -- the zero value included -- and
// does nothing when Dest is unset.
func TestSetDefault(t *testing.T) {
	t.Run("assigns Default to Dest", func(t *testing.T) {
		s1, s2, i1, i2, b := "old", "old", 1, 1, true
		items := []interface{ setDefault() }{
			&StringArgument{Dest: &s1, Default: "new"},
			&StringOption{Flag: "-s", Dest: &s2, Default: "new"},
			&IntArgument{Dest: &i1, Default: 7},
			&IntOption{Flag: "-i", Dest: &i2, Default: 7},
			&BoolOption{Flag: "-b", Dest: &b, Default: false},
		}
		for _, it := range items {
			it.setDefault()
		}
		if s1 != "new" || s2 != "new" || i1 != 7 || i2 != 7 || b {
			t.Errorf("Dests = %q, %q, %d, %d, %v; want new, new, 7, 7, false", s1, s2, i1, i2, b)
		}
	})

	t.Run("a nil Dest is a no-op", func(t *testing.T) {
		items := []interface{ setDefault() }{
			&StringArgument{Default: "x"},
			&StringOption{Flag: "-s", Default: "x"},
			&IntArgument{Default: 1},
			&IntOption{Flag: "-i", Default: 1},
			&BoolOption{Flag: "-b", Default: true},
		}
		for _, it := range items {
			it.setDefault() // must not panic
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
