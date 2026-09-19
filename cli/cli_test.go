package cli

import (
	"errors"
	"io"
	"os"
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

// --- Argument, Option, Command -----------------------------------------------

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

// --- Validate ----------------------------------------------------------------

// TODO ...

// --- Parse -------------------------------------------------------------------

// TODO ...

// --- Usage, Help, Version output ---------------------------------------------

// TODO ...
