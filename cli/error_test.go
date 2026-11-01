package cli

import (
	"errors"
	"io"
	"testing"
)

func TestErrorf(t *testing.T) {
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
			err := Errorf(tt.code, tt.format, tt.args...)
			if got := err.Error(); got != tt.wantMsg {
				t.Errorf("Error() = %q, want %q", got, tt.wantMsg)
			}
			if got := err.ExitCode(); got != tt.code {
				t.Errorf("ExitCode() = %d, want %d", got, tt.code)
			}
		})
	}
}

func TestErrorf_Wrap(t *testing.T) {
	sentinel := errors.New("sentinel")
	err := Errorf(3, "step failed: %w", sentinel)

	if !errors.Is(err, sentinel) {
		t.Error("errors.Is(err, sentinel) = false, want true")
	}
	if got, want := err.Error(), "step failed: sentinel"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if errors.Is(err, ErrExit) {
		t.Error("errors.Is(err, ErrExit) = true for code 3, want false")
	}
}

func TestErrorf_CodeZero(t *testing.T) {
	err := Errorf(0, "done")

	if !errors.Is(err, ErrExit) {
		t.Error("errors.Is(err, ErrExit) = false for code 0, want true")
	}
	if got := err.ExitCode(); got != 0 {
		t.Errorf("ExitCode() = %d, want 0", got)
	}
}

func TestPrintErrorf_Wrap(t *testing.T) {
	sentinel := errors.New("sentinel")
	cmd := &Command{Name: "prog", Stderr: io.Discard}

	err := cmd.PrintErrorf("bad thing: %w", sentinel)
	if !errors.Is(err, sentinel) {
		t.Error("errors.Is(err, sentinel) = false, want true")
	}
	if got := err.ExitCode(); got != 2 {
		t.Errorf("ExitCode() = %d, want 2", got)
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
			err:        Errorf(1, "cli: Command.Name must not be empty"),
			wantCode:   1,
			wantIsExit: false,
		},
		{
			name:       "usage error",
			err:        Errorf(2, "unknown option %q", "--nope"),
			wantCode:   2,
			wantIsExit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.ExitCode(); got != tt.wantCode {
				t.Errorf("ExitCode() = %d, want %d", got, tt.wantCode)
			}
			// Only a code-0 Error (ErrHelp/ErrVersion) also classifies as
			// ErrExit. A Validate failure or a usage error must not, or a
			// caller checking errors.Is(err, ErrExit) would mistake a real
			// failure for an already-printed clean exit.
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
