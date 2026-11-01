package cli

import (
	"errors"
	"fmt"
)

// Error is returned by Parse and DefaultParse. It behaves like a normal
// error (Error() string), but always also reports an ExitCode, so a
// caller's main() can be a single line:
//
//	if err := prg.DefaultParse(); err != nil {
//	    os.Exit(err.ExitCode())
//	}
//
// Error is a concrete type, not an interface: this package only ever
// builds one kind of failure value, so there is no abstraction to gain
// from a separate interface, and returning *Error directly (instead of
// the built-in error) means a nil check is a plain pointer comparison,
// with no typed-nil-in-interface pitfall to worry about.
type Error struct {
	msg  string
	code int
	err  error
}

// Errorf builds an Error with the given exit code and a message
// formatted like fmt.Errorf. It prints nothing: it is for a caller (for
// example an Action) that wants to return its own failure, with its own
// exit code, and decide for itself how to report it. To print the
// clap-style block as well, use Command.UsageErrorf.
//
// A %w verb in format wraps an underlying error, which errors.Is and
// errors.As then find through the returned Error. The code is not
// validated: 0 makes errors.Is(err, ErrExit) true, which asks the caller
// to exit successfully without any further output.
func Errorf(code int, format string, args ...any) *Error {
	err := fmt.Errorf(format, args...)
	return &Error{msg: err.Error(), code: code, err: err}
}

func (e *Error) Error() string { return e.msg }

// Unwrap returns the error built from Errorf's format, so errors.Is and
// errors.As reach anything wrapped with %w. It is nil for ErrHelp and
// ErrVersion.
func (e *Error) Unwrap() error { return e.err }

// ExitCode reports the process exit code appropriate for this failure:
//
//	0 -- a built-in action fired (-h/--help or -v/--version); the text
//	     has already been printed to Stdout.
//	1 -- the Command itself was declared incorrectly (duplicate flag,
//	     empty Name, ...) -- a bug in the caller's Go code, not
//	     something the end user mistyped on the command line. Reported
//	     by Validate().
//	2 -- a real parsing/usage error (unknown option, missing value,
//	     invalid value, unexpected positional argument, a bad Env
//	     value) -- matches the getopt/argparse convention of using 2
//	     specifically for command-line usage errors.
func (e *Error) ExitCode() int { return e.code }

// Is makes errors.Is(err, ErrExit) true for an Error whose ExitCode is
// 0 (only ErrHelp and ErrVersion). The e.code == 0 guard keeps a real
// parsing/usage error or a Validate failure from satisfying it too.
func (e *Error) Is(target error) bool { return e.code == 0 && target == ErrExit }

var (
	// ErrExit is the shared classification for the built-in actions Parse
	// handles on its own (help/version already printed). Callers that
	// don't care which one fired can check errors.Is(err, ErrExit) instead
	// of comparing against ErrHelp/ErrVersion individually.
	ErrExit = errors.New("cli: exit requested")

	// ErrHelp is returned by Parse when -h/--help was requested. The
	// full help text has already been printed via PrintHelp;
	// errors.Is(err, ErrExit) is also true, and ErrHelp.ExitCode() is 0.
	ErrHelp = &Error{msg: "cli: help requested", code: 0}

	// ErrVersion is returned by Parse when -v/--version was requested.
	// The version string has already been printed; errors.Is(err,
	// ErrExit) is also true, and ErrVersion.ExitCode() is 0.
	ErrVersion = &Error{msg: "cli: version requested", code: 0}
)
