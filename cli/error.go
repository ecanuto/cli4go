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
}

// newError builds a dynamic-message Error for a given exit code (1 for a
// Validate failure, 2 for a real parsing/usage error).
func newError(code int, format string, args ...any) *Error {
	return &Error{msg: fmt.Sprintf(format, args...), code: code}
}

func (e *Error) Error() string { return e.msg }

// ExitCode reports the process exit code appropriate for this failure:
//
//	0 -- a built-in action fired (-h/--help or -v/--version); the text
//	     has already been printed to Stdout.
//	1 -- the Command itself was declared incorrectly (duplicate flag,
//	     empty Name, a Default that doesn't match its Dest's type or
//	     can't actually be assigned to it, ...) -- a bug in the
//	     caller's Go code, not something the end user mistyped on the
//	     command line. Most often from Validate(), but also from
//	     Parse's Default resolution (see resolveEnvVar's doc comment).
//	2 -- a real parsing/usage error (unknown option, missing value,
//	     invalid value, unexpected positional argument, a bad Env
//	     value) -- matches the getopt/argparse convention of using 2
//	     specifically for command-line usage errors.
func (e *Error) ExitCode() int { return e.code }

// Is lets errors.Is(err, ErrExit) keep working: an Error classifies as
// ErrExit only when its ExitCode is 0 (i.e. only ErrHelp/ErrVersion).
// The e.code == 0 guard matters -- without it, a real parsing/usage
// error or a Validate failure would also incorrectly satisfy
// errors.Is(err, ErrExit), since Is would then only be checking which
// error was asked for, not which instance it's being called on.
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
