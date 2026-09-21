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

// --- Value implementations ---------------------------------------------------

func TestStringVar(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "hello", want: "hello"},
		{input: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var s string
			v := StringVar(&s)

			err := v.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Set(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if s != tt.want {
				t.Errorf("bound variable = %q, want %q", s, tt.want)
			}
			if got := v.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStringVar_String(t *testing.T) {
	s := "hello"
	if got := StringVar(&s).String(); got != "hello" {
		t.Errorf("String() = %q, want %q", got, "hello")
	}
	if got := StringVar(nil).String(); got != "" {
		t.Errorf("String() with nil pointer = %q, want empty", got)
	}
}

func TestIntVar(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{input: "42", want: 42},
		{input: "-7", want: -7},
		{input: "not-a-number", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var i int
			v := IntVar(&i)

			err := v.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Set(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if i != tt.want {
				t.Errorf("bound variable = %d, want %d", i, tt.want)
			}
		})
	}
}

func TestIntVar_String(t *testing.T) {
	i := 42
	if got := IntVar(&i).String(); got != "42" {
		t.Errorf("String() = %q, want %q", got, "42")
	}
	if got := IntVar(nil).String(); got != "0" {
		t.Errorf("String() with nil pointer = %q, want %q", got, "0")
	}
}

func TestBoolVar(t *testing.T) {
	tests := []struct {
		input   string
		want    bool
		wantErr bool
	}{
		{input: "true", want: true},
		{input: "false", want: false},
		{input: "1", want: true},
		{input: "not-a-bool", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var b bool
			v := BoolVar(&b)

			err := v.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Set(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if b != tt.want {
				t.Errorf("bound variable = %v, want %v", b, tt.want)
			}
		})
	}
}

func TestBoolVar_String(t *testing.T) {
	b := true
	if got := BoolVar(&b).String(); got != "true" {
		t.Errorf("String() = %q, want %q", got, "true")
	}
	if got := BoolVar(nil).String(); got != "false" {
		t.Errorf("String() with nil pointer = %q, want %q", got, "false")
	}
}

func TestIsBoolFlag(t *testing.T) {
	var b bool
	if !isBoolFlag(BoolVar(&b)) {
		t.Error("isBoolFlag(BoolVar(...)) = false, want true")
	}

	var s string
	if isBoolFlag(StringVar(&s)) {
		t.Error("isBoolFlag(StringVar(...)) = true, want false")
	}

	var i int
	if isBoolFlag(IntVar(&i)) {
		t.Error("isBoolFlag(IntVar(...)) = true, want false")
	}
}

// --- Errors ------------------------------------------------------------------

func TestNewError(t *testing.T) {
	tests := []struct {
		name    string
		code    int
		args    []any
		format  string
		wantMsg string
	}{
		{
			name:    "no args",
			code:    1,
			format:  "cli: Command.Name must not be empty",
			wantMsg: "cli: Command.Name must not be empty",
		},
		{
			name:    "with args",
			code:    2,
			args:    []any{"--nope"},
			format:  "unknown option %q",
			wantMsg: `unknown option "--nope"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := newError(tt.code, tt.format, tt.args...)
			if got := err.Error(); got != tt.wantMsg {
				t.Errorf("Error() = %q, want %q", got, tt.wantMsg)
			}
			if got := err.ExitCode(); got != tt.code {
				t.Errorf("ExitCode() = %d, want %d", got, tt.code)
			}
		})
	}
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		name       string
		err        *Error
		wantCode   int
		wantIsExit bool
	}{
		{
			name:       "help",
			err:        ErrHelp,
			wantCode:   0,
			wantIsExit: true,
		},
		{
			name:       "version",
			err:        ErrVersion,
			wantCode:   0,
			wantIsExit: true,
		},
		{
			name:       "validate failure",
			err:        newError(1, "cli: Command.Name must not be empty"),
			wantCode:   1,
			wantIsExit: false,
		},
		{
			name:       "usage error",
			err:        newError(2, "unknown option %q", "--nope"),
			wantCode:   2,
			wantIsExit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.ExitCode(); got != tt.wantCode {
				t.Errorf("ExitCode() = %d, want %d", got, tt.wantCode)
			}
			// Regression guard: only a code-0 Error (ErrHelp/ErrVersion) may
			// also classify as ErrExit. A Validate failure or a real
			// parsing/usage error must NOT satisfy errors.Is(err, ErrExit),
			// or a caller checking that would wrongly treat a real failure
			// as an already-printed clean exit.
			if got := errors.Is(tt.err, ErrExit); got != tt.wantIsExit {
				t.Errorf("errors.Is(err, ErrExit) = %v, want %v", got, tt.wantIsExit)
			}
		})
	}
}

func TestErrHelp_Is(t *testing.T) {
	if !errors.Is(ErrHelp, ErrExit) {
		t.Error("errors.Is(ErrHelp, ErrExit) = false, want true")
	}
	if errors.Is(ErrHelp, ErrVersion) {
		t.Error("errors.Is(ErrHelp, ErrVersion) = true, want false")
	}
	if errors.Unwrap(ErrHelp) != nil {
		t.Errorf("errors.Unwrap(ErrHelp) = %v, want nil", errors.Unwrap(ErrHelp))
	}
	if got, want := ErrHelp.Error(), "cli: help requested"; got != want {
		t.Errorf("ErrHelp.Error() = %q, want %q", got, want)
	}
}

func TestErrVersion_Is(t *testing.T) {
	if !errors.Is(ErrVersion, ErrExit) {
		t.Error("errors.Is(ErrVersion, ErrExit) = false, want true")
	}
	if errors.Is(ErrVersion, ErrHelp) {
		t.Error("errors.Is(ErrVersion, ErrHelp) = true, want false")
	}
	if errors.Unwrap(ErrVersion) != nil {
		t.Errorf("errors.Unwrap(ErrVersion) = %v, want nil", errors.Unwrap(ErrVersion))
	}
	if got, want := ErrVersion.Error(), "cli: version requested"; got != want {
		t.Errorf("ErrVersion.Error() = %q, want %q", got, want)
	}
}

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
			if got := (Option{Flag: tt.input}).flag(); got != tt.want {
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
			if got := (Option{Long: tt.input}).long(); got != tt.want {
				t.Errorf("Option{Long: %q}.long() = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
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
		if a[i].Name != b[i].Name ||
			a[i].Flag != b[i].Flag ||
			a[i].Long != b[i].Long ||
			a[i].Help != b[i].Help ||
			(a[i].Dest == nil) != (b[i].Dest == nil) ||
			a[i].Default != b[i].Default ||
			a[i].Env != b[i].Env ||
			(a[i].Action == nil) != (b[i].Action == nil) {
			return false
		}
	}
	return true
}

func TestAllOptions(t *testing.T) {
	// A placeholder Action, used only to mark a "want" Option as
	// carrying one -- optionSlicesEqual compares Action by nil-ness,
	// not identity, so any non-nil func here works.
	anyAction := func(*Command, Option, string) *Error { return nil }
	var discard bool
	anyDest := BoolVar(&discard)

	tests := []struct {
		name string
		cmd  Command
		want []Option
	}{
		{
			name: "no user options, no version",
			cmd:  Command{Name: "myprogram"},
			want: []Option{
				{Flag: "-h", Long: "--help", Help: "Print help", Dest: anyDest, Action: anyAction},
			},
		},
		{
			name: "no user options, with version",
			cmd:  Command{Name: "myprogram", Version: "0.1.0"},
			want: []Option{
				{Flag: "-h", Long: "--help", Help: "Print help", Dest: anyDest, Action: anyAction},
				{Flag: "-v", Long: "--version", Help: "Print version", Dest: anyDest, Action: anyAction},
			},
		},
		{
			name: "user options first, built-ins last",
			cmd: Command{
				Name:    "myprogram",
				Version: "0.1.0",
				Options: []Option{
					{Flag: "-f", Help: "Enable foo mode"},
					{Long: "--bar", Help: "Enable bar mode"},
				},
			},
			want: []Option{
				{Flag: "-f", Help: "Enable foo mode"},
				{Long: "--bar", Help: "Enable bar mode"},
				{Flag: "-h", Long: "--help", Help: "Print help", Dest: anyDest, Action: anyAction},
				{Flag: "-v", Long: "--version", Help: "Print version", Dest: anyDest, Action: anyAction},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slices.Collect(tt.cmd.allOptions())
			if !optionSlicesEqual(got, tt.want) {
				t.Errorf("allOptions() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestAllOptions_StopsEarly(t *testing.T) {
	anyAction := func(*Command, Option, string) *Error { return nil }
	var discard bool
	anyDest := BoolVar(&discard)

	cmd := Command{
		Name:    "myprogram",
		Version: "0.1.0",
		Options: []Option{
			{Flag: "-f", Help: "Enable foo mode"},
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
				{Flag: "-f", Help: "Enable foo mode"},
			},
		},
		{
			name:      "stops right after the synthesized -h/--help",
			stopAfter: 2,
			want: []Option{
				{Flag: "-f", Help: "Enable foo mode"},
				{Flag: "-h", Long: "--help", Help: "Print help", Dest: anyDest, Action: anyAction},
			},
		},
		{
			name:      "stops right after the synthesized -v/--version",
			stopAfter: 3,
			want: []Option{
				{Flag: "-f", Help: "Enable foo mode"},
				{Flag: "-h", Long: "--help", Help: "Print help", Dest: anyDest, Action: anyAction},
				{Flag: "-v", Long: "--version", Help: "Print version", Dest: anyDest, Action: anyAction},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []Option
			cmd.allOptions()(func(opt Option) bool {
				got = append(got, opt)
				return len(got) < tt.stopAfter
			})

			if !optionSlicesEqual(got, tt.want) {
				t.Errorf("allOptions() stopped after %d = %+v, want %+v", tt.stopAfter, got, tt.want)
			}
		})
	}
}

func TestAllEnvVars(t *testing.T) {
	// Shared Value instances for the Dest-forwarding case below: the
	// same *stringValue pointer must appear on both the Argument/Option
	// side and the "want" side for slices.Equal's interface comparison
	// to hold.
	var qux, bar string
	destQux, destBar := StringVar(&qux), StringVar(&bar)

	tests := []struct {
		name string
		cmd  Command
		want []EnvVar
	}{
		{
			name: "no Env declared anywhere",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{{Name: "qux"}},
				Options:   []Option{{Flag: "-f"}},
			},
			want: nil,
		},
		{
			name: "only an Argument declares Env",
			cmd: Command{
				Name:      "myprogram",
				Arguments: []Argument{{Name: "qux", Help: "The qux to operate on", Env: "PROGRAM_QUX"}},
			},
			want: []EnvVar{
				{Name: "PROGRAM_QUX", Help: "The qux to operate on"},
			},
		},
		{
			name: "only an Option declares Env",
			cmd: Command{
				Name:    "myprogram",
				Options: []Option{{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "PROGRAM_BAR"}},
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
				Arguments: []Argument{{Name: "qux", Help: "The qux to operate on", Env: "APP_QUX"}},
				Options:   []Option{{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "APP_BAR"}},
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
					{Name: "qux", Help: "The qux to operate on", Env: "APP_QUX"},
					{Name: "quux", Help: "No env for this one"},
				},
				Options: []Option{
					{Flag: "-f", Help: "No env for this one either"},
					{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "APP_BAR"},
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
				Arguments: []Argument{{Name: "qux", Help: "The qux to operate on", Env: "APP_QUX"}},
				Options:   []Option{{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "APP_BAR"}},
				EnvVars:   []EnvVar{{Name: "APP_DEBUG", Help: "Enable debug logging"}},
			},
			want: []EnvVar{
				{Name: "APP_QUX", Help: "The qux to operate on"},
				{Name: "APP_BAR", Help: "Set the bar value"},
				{Name: "APP_DEBUG", Help: "Enable debug logging"},
			},
		},
		{
			// Regression guard: allEnvVars must forward Dest and
			// Default too, not just Name/Help -- Parse's Default ->
			// Env resolution phase walks this exact same sequence and
			// needs both to actually apply values.
			name: "Dest and Default are forwarded from both Argument and Option",
			cmd: Command{
				Name: "myprogram",
				Arguments: []Argument{
					{Name: "qux", Help: "The qux to operate on", Env: "APP_QUX", Dest: destQux, Default: "fallback-qux"},
				},
				Options: []Option{
					{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "APP_BAR", Dest: destBar, Default: "fallback-bar"},
				},
			},
			want: []EnvVar{
				{Name: "APP_QUX", Help: "The qux to operate on", Dest: destQux, Default: "fallback-qux"},
				{Name: "APP_BAR", Help: "Set the bar value", Dest: destBar, Default: "fallback-bar"},
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
// standalone EnvVars loop -- the same reason TestAllOptions_StopsEarly
// needed more than a single stopping point.
func TestAllEnvVars_StopsEarly(t *testing.T) {
	cmd := Command{
		Name:      "myprogram",
		Arguments: []Argument{{Name: "qux", Help: "The qux to operate on", Env: "APP_QUX"}},
		Options:   []Option{{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "APP_BAR"}},
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

// TestAllDefaults exercises allDefaults directly. Unlike
// allOptions/allEnvVars, it yields unconditionally -- every
// Argument/Option/EnvVar produces a (Dest, Default) pair even when
// both are nil -- so the cases here cover that alongside the
// Arguments-then-Options-then-EnvVars order.
func TestAllDefaults(t *testing.T) {
	type pair struct {
		Dest    Value
		Default any
	}
	collect := func(c Command) []pair {
		var got []pair
		c.allDefaults()(func(dest Value, def any) bool {
			got = append(got, pair{dest, def})
			return true
		})
		return got
	}

	t.Run("no Arguments/Options/EnvVars: yields nothing", func(t *testing.T) {
		if got := collect(Command{Name: "myprogram"}); got != nil {
			t.Errorf("allDefaults() = %+v, want nil", got)
		}
	})

	t.Run("items with no Default are still yielded, as a nil pair", func(t *testing.T) {
		c := Command{
			Name:      "myprogram",
			Arguments: []Argument{{Name: "qux"}},
			Options:   []Option{{Flag: "-f"}},
		}
		want := []pair{{nil, nil}, {nil, nil}}
		if got := collect(c); !slices.Equal(got, want) {
			t.Errorf("allDefaults() = %+v, want %+v", got, want)
		}
	})

	t.Run("Default set on an Option with no Env at all -- regression guard", func(t *testing.T) {
		// A Default applies regardless of Env; unlike allEnvVars(),
		// allDefaults() must not filter on it.
		var i int
		dest := IntVar(&i)
		c := Command{Name: "myprogram", Options: []Option{{Flag: "-n", Dest: dest, Default: 7}}}
		want := []pair{{dest, 7}}
		if got := collect(c); !slices.Equal(got, want) {
			t.Errorf("allDefaults() = %+v, want %+v", got, want)
		}
	})

	t.Run("Arguments, then Options, then EnvVars, in that order", func(t *testing.T) {
		var qux, bar, debug string
		destQux, destBar, destDebug := StringVar(&qux), StringVar(&bar), StringVar(&debug)
		c := Command{
			Name:      "myprogram",
			Arguments: []Argument{{Name: "qux", Dest: destQux, Default: "fallback-qux"}},
			Options:   []Option{{Flag: "-b", Dest: destBar, Default: "fallback-bar"}},
			EnvVars:   []EnvVar{{Name: "APP_DEBUG", Dest: destDebug, Default: "fallback-debug"}},
		}
		want := []pair{
			{destQux, "fallback-qux"},
			{destBar, "fallback-bar"},
			{destDebug, "fallback-debug"},
		}
		if got := collect(c); !slices.Equal(got, want) {
			t.Errorf("allDefaults() = %+v, want %+v", got, want)
		}
	})
}

// TestAllDefaults_StopsEarly covers all three early-exit points in
// allDefaults, the same reason TestAllEnvVars_StopsEarly needed more
// than a single stopping point.
func TestAllDefaults_StopsEarly(t *testing.T) {
	var a, b, e string
	cmd := Command{
		Name:      "myprogram",
		Arguments: []Argument{{Name: "qux", Dest: StringVar(&a), Default: "a"}},
		Options:   []Option{{Flag: "-b", Dest: StringVar(&b), Default: "b"}},
		EnvVars:   []EnvVar{{Name: "APP_DEBUG", Dest: StringVar(&e), Default: "e"}},
	}

	tests := []struct {
		name      string
		stopAfter int
		wantDefs  []any
	}{
		{name: "stops inside the Arguments loop", stopAfter: 1, wantDefs: []any{"a"}},
		{name: "stops inside the Options loop", stopAfter: 2, wantDefs: []any{"a", "b"}},
		{name: "stops inside the EnvVars loop", stopAfter: 3, wantDefs: []any{"a", "b", "e"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []any
			cmd.allDefaults()(func(_ Value, def any) bool {
				got = append(got, def)
				return len(got) < tt.stopAfter
			})

			if !slices.Equal(got, tt.wantDefs) {
				t.Errorf("allDefaults() stopped after %d = %+v, want %+v", tt.stopAfter, got, tt.wantDefs)
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
				Arguments: []Argument{{Name: ""}},
			},
			wantErr: true,
		},
		{
			name: "propagates a validateOptions failure",
			cmd: Command{
				Name:    "myprogram",
				Options: []Option{{}},
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
			args: []Argument{{Name: "qux", Dest: StringVar(&s)}},
		},
		{
			name:    "empty Argument.Name",
			args:    []Argument{{Name: ""}},
			wantErr: true,
		},
		{
			name: "duplicate Argument.Name",
			args: []Argument{
				{Name: "qux"},
				{Name: "qux"},
			},
			wantErr: true,
		},
		{
			name: "Default matches Dest type",
			args: []Argument{
				{Name: "qux", Dest: StringVar(&s), Default: "bar"},
			},
		},
		{
			name: "Default mismatches Dest type",
			args: []Argument{
				{Name: "qux", Dest: StringVar(&s), Default: 42},
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
			opts: []Option{{Flag: "f"}},
		},
		{
			name: "valid, option with Long only",
			opts: []Option{{Long: "foo"}},
		},
		{
			name: "valid, option with Flag and Long",
			opts: []Option{{Flag: "f", Long: "foo"}},
		},
		{
			name: "valid, option with Action instead of Dest",
			opts: []Option{{Flag: "f", Action: func(*Command, Option, string) *Error { return nil }}},
		},
		{
			name: "valid, option with both Dest and Action",
			opts: []Option{{Flag: "f", Dest: StringVar(&s), Action: func(*Command, Option, string) *Error { return nil }}},
		},
		{
			name:    "option missing both Flag and Long",
			opts:    []Option{{}},
			wantErr: true,
		},
		{
			name:    "option Flag longer than one character",
			opts:    []Option{{Flag: "-ab"}},
			wantErr: true,
		},
		{
			name:    "option Flag with only dashes",
			opts:    []Option{{Flag: "--"}},
			wantErr: true,
		},
		{
			name: "duplicate option flag, same spelling",
			opts: []Option{
				{Flag: "f"},
				{Flag: "f"},
			},
			wantErr: true,
		},
		{
			name: "duplicate option flag, different spelling normalizes the same",
			opts: []Option{
				{Flag: "f"},
				{Flag: "-f"},
			},
			wantErr: true,
		},
		{
			name: "duplicate option long, different spelling normalizes the same",
			opts: []Option{
				{Long: "bar"},
				{Long: "--bar"},
			},
			wantErr: true,
		},
		{
			name: "Default matches Dest type",
			opts: []Option{
				{Flag: "f", Dest: StringVar(&s), Default: "bar"},
			},
		},
		{
			name: "Default mismatches Dest type",
			opts: []Option{
				{Flag: "f", Dest: StringVar(&s), Default: 42},
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
				Arguments: []Argument{{Name: "qux"}},
				Options:   []Option{{Flag: "-f"}},
			},
		},
		{
			name: "valid, one env var per source",
			cmd: Command{
				Arguments: []Argument{{Name: "qux", Env: "APP_QUX"}},
				Options:   []Option{{Flag: "-b", Env: "APP_BAR"}},
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
					{Name: "qux", Env: "APP_QUX"},
					{Name: "quux", Env: "APP_QUX"},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate name, Argument.Env vs Option.Env",
			cmd: Command{
				Arguments: []Argument{{Name: "qux", Env: "APP_QUX"}},
				Options:   []Option{{Flag: "-b", Env: "APP_QUX"}},
			},
			wantErr: true,
		},
		{
			name: "duplicate name, Argument.Env vs EnvVars",
			cmd: Command{
				Arguments: []Argument{{Name: "qux", Env: "APP_QUX"}},
				EnvVars:   []EnvVar{{Name: "APP_QUX"}},
			},
			wantErr: true,
		},
		{
			name: "duplicate name, Option.Env vs EnvVars",
			cmd: Command{
				Options: []Option{{Flag: "-b", Env: "APP_BAR"}},
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
// applyDefaults Set-failure path (TestParse) -- validateDefaultType
// can't catch a bad Default for a custom Value ahead of time (see its
// doc comment), so Validate() lets it through and applyDefaults is the
// only place left to catch it.
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
			Options: []Option{{Flag: "-b", Dest: StringVar(&s)}},
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
			Options: []Option{{Flag: "-b", Env: "APP_BAR", Dest: StringVar(&s), Default: "fallback"}},
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
			Arguments: []Argument{{Name: "qux", Env: "APP_QUX", Dest: StringVar(&s), Default: "fallback"}},
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
			Options: []Option{{Flag: "-n", Env: "APP_NUM", Dest: IntVar(&i), Default: 7}},
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

	t.Run("applyDefaults failure propagates: a code-1 *Error, nothing printed", func(t *testing.T) {
		// validateDefaultType can't check a custom Value's Default (see
		// its doc comment), so Validate() lets this through and
		// applyDefaults is the one that has to reject it.
		var buf bytes.Buffer
		c := &Command{
			Name:    "myprogram",
			Options: []Option{{Flag: "-x", Dest: failingValue{}, Default: "anything"}},
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
			Options: []Option{{Flag: "-n", Env: "APP_NUM", Dest: IntVar(&i)}},
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
			Options:   []Option{{Flag: "-f", Dest: BoolVar(&foo)}},
			Arguments: []Argument{{Name: "qux", Dest: StringVar(&qux)}},
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
		c := &Command{Name: "myprogram", Options: []Option{{Flag: "-b", Long: "--bar", Dest: StringVar(&bar)}}}

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
		c := &Command{Name: "myprogram", Options: []Option{{Flag: "-b", Dest: StringVar(&bar)}}, Stderr: &buf}

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
		c := &Command{Name: "myprogram", Options: []Option{{Flag: "-n", Dest: IntVar(&n)}}, Stderr: &bytes.Buffer{}}

		err := c.Parse([]string{"-n", "not-a-number"})
		if err == nil || err.ExitCode() != 2 {
			t.Fatalf("Parse() = %v, want a code-2 error", err)
		}
		if want := `option "-n": invalid integer value "not-a-number": strconv.Atoi: parsing "not-a-number": invalid syntax`; err.Error() != want {
			t.Errorf("Parse().Error() = %q, want %q", err.Error(), want)
		}
	})

	t.Run("positional argument fills the next declared Argument, in order", func(t *testing.T) {
		var a, b string
		c := &Command{
			Name: "myprogram",
			Arguments: []Argument{
				{Name: "first", Dest: StringVar(&a)},
				{Name: "second", Dest: StringVar(&b)},
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
		c := &Command{Name: "myprogram", Arguments: []Argument{{Name: "num", Dest: IntVar(&n)}}, Stderr: &bytes.Buffer{}}

		err := c.Parse([]string{"not-a-number"})
		if err == nil || err.ExitCode() != 2 {
			t.Fatalf("Parse() = %v, want a code-2 error", err)
		}
		if want := `argument "not-a-number": invalid integer value "not-a-number": strconv.Atoi: parsing "not-a-number": invalid syntax`; err.Error() != want {
			t.Errorf("Parse().Error() = %q, want %q", err.Error(), want)
		}
	})

	t.Run("Argument with no Dest is still consumed positionally, just not bound", func(t *testing.T) {
		c := &Command{Name: "myprogram", Arguments: []Argument{{Name: "qux"}}}

		if err := c.Parse([]string{"anything"}); err != nil {
			t.Errorf("Parse() = %v, want nil", err)
		}
	})

	t.Run("options and arguments mixed together", func(t *testing.T) {
		var foo bool
		var qux string
		c := &Command{
			Name:      "myprogram",
			Options:   []Option{{Flag: "-f", Dest: BoolVar(&foo)}},
			Arguments: []Argument{{Name: "qux", Dest: StringVar(&qux)}},
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
				{
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
		if seen.Flag != "-x" || seen.Long != "--extra" {
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
				{Flag: "-x", Action: func(c *Command, opt Option, value string) *Error { ran = true; seenValue = value; return nil }},
			},
			Arguments: []Argument{{Name: "qux", Dest: StringVar(&qux)}},
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
				{Flag: "-x", Action: func(c *Command, opt Option, value string) *Error { return newError(2, "%s", sentinel) }},
				{Flag: "-y", Dest: BoolVar(&later)},
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
			Options: []Option{{
				Flag: "-b", Dest: StringVar(&bar),
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
			Options: []Option{{
				Flag: "-f", Dest: BoolVar(&foo),
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
			Options: []Option{{
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
			Options: []Option{{
				Flag: "-x",
				Dest: BoolVar(&triggered),
				Action: func(c *Command, opt Option, value string) *Error {
					seenValue = value
					return nil
				},
			}},
			Arguments: []Argument{{Name: "qux", Dest: StringVar(&qux)}},
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
			Options: []Option{{
				Flag: "-n", Dest: IntVar(&n),
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

// TestApplyDefaults exercises (*Command).applyDefaults directly.
// Walking Arguments/Options/EnvVars is allDefaults' own concern (see
// TestAllDefaults); this covers the wiring, the nil-pair skip, and the
// Set-failure path.
func TestApplyDefaults(t *testing.T) {
	t.Run("wiring: Default assigned to Dest, for an Argument and an Option", func(t *testing.T) {
		var s string
		var i int
		c := &Command{
			Name:      "myprogram",
			Arguments: []Argument{{Name: "qux", Dest: StringVar(&s), Default: "fallback"}},
			Options:   []Option{{Flag: "-n", Dest: IntVar(&i), Default: 7}},
		}

		if err := c.applyDefaults(); err != nil {
			t.Fatalf("applyDefaults() = %v, want nil", err)
		}
		if s != "fallback" {
			t.Errorf("Dest = %q, want %q", s, "fallback")
		}
		if i != 7 {
			t.Errorf("Dest = %d, want %d", i, 7)
		}
	})

	t.Run("nil Dest/Default pairs are skipped", func(t *testing.T) {
		c := &Command{Name: "myprogram", Options: []Option{{Flag: "-b"}}}

		if err := c.applyDefaults(); err != nil {
			t.Errorf("applyDefaults() = %v, want nil", err)
		}
	})

	t.Run("Set failure: a code-1 *Error, nothing printed", func(t *testing.T) {
		var buf bytes.Buffer
		var i int
		c := &Command{
			Name:    "myprogram",
			Options: []Option{{Flag: "-n", Dest: IntVar(&i), Default: "not-a-number"}},
			Stderr:  &buf,
		}

		err := c.applyDefaults()
		if err == nil {
			t.Fatal("applyDefaults() = nil, want error")
		}
		if err.ExitCode() != 1 {
			t.Errorf("ExitCode() = %d, want 1", err.ExitCode())
		}
		if buf.Len() != 0 {
			t.Errorf("applyDefaults() wrote %q to Stderr, want nothing", buf.String())
		}
	})
}

// TestApplyEnvVars exercises (*Command).applyEnvVars directly --
// Arguments, Options, and standalone EnvVars each get their own case,
// without going through Validate/Parse.
func TestApplyEnvVars(t *testing.T) {
	t.Run("no Env declared: Dest untouched", func(t *testing.T) {
		var s string
		c := &Command{Options: []Option{{Flag: "-b", Dest: StringVar(&s)}}}

		if err := c.applyEnvVars(); err != nil {
			t.Errorf("applyEnvVars() = %v, want nil", err)
		}
		if s != "" {
			t.Errorf("Dest = %q, want zero value", s)
		}
	})

	t.Run("Env declared but no Dest bound: informational only, no panic even if present", func(t *testing.T) {
		t.Setenv("APP_BAR", "some-value")
		c := &Command{Options: []Option{{Flag: "-b", Env: "APP_BAR"}}}

		if err := c.applyEnvVars(); err != nil {
			t.Errorf("applyEnvVars() = %v, want nil", err)
		}
	})

	t.Run("standalone EnvVar with empty Name: skipped defensively, no panic", func(t *testing.T) {
		// Validate() would normally reject this before Parse ever gets
		// here; applyEnvVars' own guard is exercised by calling it
		// directly, bypassing Validate.
		var s string
		c := &Command{EnvVars: []EnvVar{{Name: "", Dest: StringVar(&s)}}}

		if err := c.applyEnvVars(); err != nil {
			t.Errorf("applyEnvVars() = %v, want nil", err)
		}
		if s != "" {
			t.Errorf("Dest = %q, want zero value", s)
		}
	})

	t.Run("Env declared but absent from the environment: Dest untouched", func(t *testing.T) {
		var s string
		c := &Command{Options: []Option{{Flag: "-b", Env: "APP_DOES_NOT_EXIST", Dest: StringVar(&s)}}}

		if err := c.applyEnvVars(); err != nil {
			t.Errorf("applyEnvVars() = %v, want nil", err)
		}
		if s != "" {
			t.Errorf("Dest = %q, want zero value", s)
		}
	})

	t.Run("env present overrides Dest, for an Argument", func(t *testing.T) {
		var s string
		t.Setenv("APP_QUX", "from-env")
		c := &Command{Arguments: []Argument{{Name: "qux", Env: "APP_QUX", Dest: StringVar(&s)}}}

		if err := c.applyEnvVars(); err != nil {
			t.Errorf("applyEnvVars() = %v, want nil", err)
		}
		if s != "from-env" {
			t.Errorf("Dest = %q, want %q", s, "from-env")
		}
	})

	t.Run("env present overrides Dest, for an Option", func(t *testing.T) {
		var i int
		t.Setenv("APP_NUM", "42")
		c := &Command{Options: []Option{{Flag: "-n", Env: "APP_NUM", Dest: IntVar(&i)}}}

		if err := c.applyEnvVars(); err != nil {
			t.Errorf("applyEnvVars() = %v, want nil", err)
		}
		if i != 42 {
			t.Errorf("Dest = %d, want %d", i, 42)
		}
	})

	t.Run("env present overrides Dest, for a standalone EnvVar", func(t *testing.T) {
		var s string
		t.Setenv("APP_DEBUG", "verbose")
		c := &Command{EnvVars: []EnvVar{{Name: "APP_DEBUG", Dest: StringVar(&s)}}}

		if err := c.applyEnvVars(); err != nil {
			t.Errorf("applyEnvVars() = %v, want nil", err)
		}
		if s != "verbose" {
			t.Errorf("Dest = %q, want %q", s, "verbose")
		}
	})

	t.Run("invalid env value: a code-2 *Error, already printed to Stderr", func(t *testing.T) {
		var i int
		var buf bytes.Buffer
		t.Setenv("APP_NUM", "not-a-number")
		c := &Command{
			Name:    "myprogram",
			Options: []Option{{Flag: "-n", Env: "APP_NUM", Dest: IntVar(&i)}},
			Stderr:  &buf,
		}

		err := c.applyEnvVars()
		if err == nil {
			t.Fatal("applyEnvVars() = nil, want error")
		}
		if err.ExitCode() != 2 {
			t.Errorf("ExitCode() = %d, want 2", err.ExitCode())
		}
		if got, want := buf.String(), c.ErrorString(err); got != want {
			t.Errorf("applyEnvVars() wrote %q to Stderr, want %q", got, want)
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
				Arguments: []Argument{{Name: "qux"}},
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
				Arguments: []Argument{{Name: "qux"}},
			},
			want: "Usage: myprogram [options] [qux]\n",
		},
		{
			name: "multiple arguments, declaration order",
			cmd: Command{
				Name: "myprogram",
				Arguments: []Argument{
					{Name: "qux"},
					{Name: "quux"},
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
				Options: []Option{{Flag: "f"}, {Long: "bar"}},
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
		Arguments: []Argument{{Name: "qux"}},
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
					{Flag: "-f", Long: "--file", Help: "Specify file name"},
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
					{Flag: "-f", Help: "Enable foo mode"},
					{Flag: "-b", Long: "--bar", Help: "Enable bar mode"},
					{Long: "--grault", Help: "Enable grault mode"},
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
				Arguments: []Argument{{Name: "qux", Help: "The qux to operate on"}},
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
				Arguments: []Argument{{Name: "qux", Help: "The qux to operate on"}},
				Options:   []Option{{Flag: "-b", Long: "--bar", Help: "Set the bar value"}},
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
					{Name: "qux", Help: "The qux to operate on"},
					{Name: "quuxlong", Help: "The quuxlong to operate on"},
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
				Arguments: []Argument{{Name: "argumentwithalongname", Help: "Wide one"}},
				Options:   []Option{{Flag: "-f", Help: "Enable foo mode"}},
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
				Options: []Option{{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "PROGRAM_BAR"}},
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
				Arguments: []Argument{{Name: "qux", Help: "The qux to operate on", Env: "PROGRAM_QUX"}},
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
				Arguments: []Argument{{Name: "qux", Help: "The qux to operate on", Env: "APP_QUX"}},
				Options:   []Option{{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "APP_BAR"}},
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
					{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "PROGRAM_REALLY_LONG_BAR_NAME"},
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
					{Name: "qux", Help: "The qux to operate on", Env: "APP_QUX"},
					{Name: "quux", Help: "No env for this one"},
				},
				Options: []Option{
					{Flag: "-f", Help: "No env for this one either"},
					{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "APP_BAR"},
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
				Arguments: []Argument{{Name: "qux", Help: "The qux to operate on", Env: "APP_QUX"}},
				Options:   []Option{{Flag: "-b", Long: "--bar", Help: "Set the bar value", Env: "APP_BAR"}},
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
