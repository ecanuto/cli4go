package cli

import (
	"errors"
	"fmt"
)

// Error is returned by Parse and DefaultParse. It behaves like a normal
// error (Error() string), but always also reports an ExitCode, so a
// caller's main() can exit with the right status:
//
//	if err := prg.DefaultParse(); err != nil {
//	    if !errors.Is(err, cli.ErrExit) {
//	        fmt.Fprintln(os.Stderr, "error:", err)
//	    }
//	    os.Exit(err.ExitCode())
//	}
//
// errors.Is(err, cli.ErrExit) tells main that the error needs nothing but
// the exit: help, version, or a usage error Parse already printed. Any
// other Error has to be printed by main before it exits.
//
// Error is a concrete type, not an interface: this package only ever
// builds one kind of failure value, so there is no abstraction to gain
// from a separate interface, and returning *Error directly (instead of
// the built-in error) means a nil check is a plain pointer comparison,
// with no typed-nil-in-interface pitfall to worry about.
type Error struct {
	err    error
	code   int
	silent bool // true: main prints nothing, only exits; false: print, then exit
}

// newError is the one place an Error is built, so every Error is born with
// its instruction (silent) and is never changed afterwards.
func newError(code int, silent bool, err error) *Error {
	return &Error{
		err:    err,
		code:   code,
		silent: silent,
	}
}

// Errorf builds an Error with the given exit code and a message
// formatted like fmt.Errorf. It prints nothing, and the returned Error
// does not satisfy errors.Is(err, ErrExit): main has to print it. It is
// for a caller (for example an Action) that wants to return its own
// failure, with its own exit code. To print the clap-style block right
// away, use Command.UsageErrorf.
//
// A %w verb in format wraps an underlying error, which errors.Is and
// errors.As then find through the returned Error. The code is not
// validated.
func Errorf(code int, format string, args ...any) *Error {
	return newError(code, false, fmt.Errorf(format, args...))
}

func (e *Error) Error() string { return e.err.Error() }

// Unwrap returns the error built from Errorf's format, so errors.Is and
// errors.As reach anything wrapped with %w.
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

// Is makes errors.Is(err, ErrExit) true for an Error that needs nothing
// more than an exit: ErrHelp, ErrVersion, or a usage error that
// Command.UsageErrorf already printed. A Validate failure or an Error
// built with Errorf is not, so main still has to print it.
func (e *Error) Is(target error) bool { return target == ErrExit && e.silent }

var (
	// ErrExit classifies an Error that needs nothing but the exit: the
	// text (help, version, or a usage error block) has already been
	// printed. It is true for ErrHelp, ErrVersion and every usage error
	// Parse prints, and false for a Validate failure and for an Error
	// built with Errorf, which main has to print itself. It does not mean
	// success: use ExitCode() == 0 for that.
	ErrExit = errors.New("cli: exit requested")

	// ErrHelp is returned by Parse when -h/--help was requested. The
	// full help text has already been printed via PrintHelp;
	// errors.Is(err, ErrExit) is also true, and ErrHelp.ExitCode() is 0.
	ErrHelp = newError(0, true, errors.New("cli: help requested"))

	// ErrVersion is returned by Parse when -v/--version was requested.
	// The version string has already been printed; errors.Is(err,
	// ErrExit) is also true, and ErrVersion.ExitCode() is 0.
	ErrVersion = newError(0, true, errors.New("cli: version requested"))
)
