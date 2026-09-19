// Package cli provides a declarative toolkit for building command-line
// interfaces in Go, combining an argparse-like API with clap-style
// formatted help output.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
)

// Value is implemented by anything that can be set from a single
// command-line token. cli4go ships implementations for common types (see
// StringVar, BoolVar, IntVar); custom types can implement Value themselves
// to be used as an Option or Argument binding.
type Value interface {
	// String returns the current value, formatted for display (used in
	// generated help text, e.g. when rendering defaults).
	String() string

	// Set parses s and stores the result. It returns an error if s
	// cannot be parsed into the underlying type.
	Set(s string) error
}

// --- Value implementations ---------------------------------------------------

type stringValue struct{ v *string }

// StringVar binds a string flag or argument to v.
func StringVar(v *string) Value { return &stringValue{v: v} }

func (s *stringValue) String() string {
	if s.v == nil {
		return ""
	}
	return *s.v
}

func (s *stringValue) Set(val string) error {
	*s.v = val
	return nil
}

type intValue struct{ v *int }

// IntVar binds an integer flag or argument to v.
func IntVar(v *int) Value { return &intValue{v: v} }

func (i *intValue) String() string {
	if i.v == nil {
		return "0"
	}
	return strconv.Itoa(*i.v)
}

func (i *intValue) Set(val string) error {
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return fmt.Errorf("invalid integer value %q: %w", val, err)
	}
	*i.v = parsed
	return nil
}

type boolValue struct{ v *bool }

// BoolVar binds a boolean flag to v. A boolean option never consumes a
// following token: its mere presence on the command line sets v to true.
func BoolVar(v *bool) Value { return &boolValue{v: v} }

func (b *boolValue) String() string {
	if b.v == nil {
		return "false"
	}
	return strconv.FormatBool(*b.v)
}

func (b *boolValue) Set(val string) error {
	parsed, err := strconv.ParseBool(val)
	if err != nil {
		return fmt.Errorf("invalid boolean value %q: %w", val, err)
	}
	*b.v = parsed
	return nil
}

// IsBoolFlag reports that boolValue never consumes a following token —
// its mere presence on the command line is enough to set it to true.
// This satisfies the unexported interface Parse checks for via
// isBoolFlag, mirroring the standard library flag package's own
// internal convention.
func (b *boolValue) IsBoolFlag() bool { return true }

// isBoolFlag reports whether v should be treated as a boolean flag: one
// that never consumes a following token. This mirrors the internal
// boolFlag interface used by the standard library's flag package.
func isBoolFlag(v Value) bool {
	bf, ok := v.(interface{ IsBoolFlag() bool })
	return ok && bf.IsBoolFlag()
}

// --- Errors ------------------------------------------------------------------

// Error is returned by Parse and DefaultParse. It behaves like a normal
// error (Error() string), but always also reports an ExitCode, so a
// caller's main() can be a single line:
//
//	if err := prg.DefaultParse(); err != nil {
//	    os.Exit(err.ExitCode())
//	}
//
// Error is a concrete type, not an interface: Parse only ever builds one
// kind of failure value, so there is no abstraction to gain from a
// separate interface, and returning *Error directly (instead of the
// built-in error) means a nil check is a plain pointer comparison, with
// no typed-nil-in-interface pitfall to worry about.
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
//	1 -- Command.Validate() failed: the Command itself was declared
//	     incorrectly (duplicate flag, empty Name, a Default that
//	     doesn't match its Dest's type, ...) -- a bug in the caller's
//	     Go code, not something the end user mistyped on the command
//	     line.
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

// --- Argument, Option, Command -----------------------------------------------

// Argument describes a single positional argument. Its position in
// Command.Arguments determines the order in which it is expected on the
// command line.
type Argument struct {
	// Name is the argument's display name, used in generated usage text
	// (e.g. "qux" in "Usage: myprogram [qux]").
	Name string

	// Help explains what the argument is for. Shown in the generated
	// "Arguments:" help section.
	Help string

	// Dest binds the parsed value to a variable. See StringVar, BoolVar,
	// IntVar.
	Dest Value

	// Default is shown in the help text as "(default: ...)" and is
	// assigned to Dest before Env and the command line are considered.
	Default any

	// Env, if set, is the environment variable consulted when this
	// argument is not provided on the command line.
	Env string
}

// Option describes a single flag (e.g. "-f, --foo"). Unlike Argument, the
// order of Options in Command.Options has no effect on parsing: flags can
// appear in any order on the command line.
type Option struct {
	// Name is an optional, stable identifier for this Option, decoupled
	// from Flag/Long. It exists for two purposes: as the preferred
	// source for the generated help placeholder (e.g. "<BAR>", falling
	// back to Long then Flag when Name is unset), and, in a future
	// bind/AutoBind feature, as the key used to match this Option
	// against a config struct field -- something Flag/Long ("-b",
	// "--bar") are not well suited for, since they're user-facing
	// display text rather than a stable identity.
	Name string

	// Flag is the short flag form, e.g. "f" or "-f" -- the leading dash
	// is optional and added automatically if omitted. Optional field.
	Flag string

	// Long is the long flag form, e.g. "bar" or "--bar" -- the leading "--"
	// is optional and added automatically if omitted. Optional, though at
	// least one of Flag/Long should be set.
	Long string

	// Help explains what the option does. Shown in the generated
	// "Options:" help section.
	Help string

	// Dest binds the parsed value to a variable. See StringVar, BoolVar,
	// IntVar. A BoolVar binding makes this a boolean flag that never
	// consumes a following token.
	Dest Value

	// Default is shown in the help text as "(default: ...)" and is
	// assigned to Dest before Env and the command line are considered.
	Default any

	// Env, if set, is the environment variable consulted when this
	// option is not provided on the command line.
	Env string
}

// Command describes a complete command-line interface: its identity
// (Name/Version/Description), its positional Arguments, and its Options
// (flags). A Command is plain data — build it as a struct literal, then
// call Parse to read os.Args into the bound variables.
//
// Program is an alias for Command, kept as the conventional name for the
// common single-command case (a CLI with no subcommands). The two are
// identical to the compiler; there is currently no separate mechanism
// for declaring subcommands (see project-context.md for the planned
// Commands field and routing design).
type Command struct {
	// Name is the program's name, shown in usage and version output.
	Name string

	// Version is the program's version string, shown by PrintVersion /
	// VersionString and by the built-in -v/--version flag.
	Version string

	// Description is a short summary shown below the usage line in the
	// generated help text.
	Description string

	// Arguments are the program's positional arguments, in the order
	// they are expected on the command line.
	Arguments []Argument

	// Options are the program's flags. Order has no effect on parsing.
	Options []Option

	// Stdout is where PrintUsage, PrintHelp, PrintVersion, and standalone
	// help/version output write to. Defaults to os.Stdout when nil.
	Stdout io.Writer

	// Stderr is where Parse's clap-style error block (a real parsing
	// error) writes to, always, independent of Stdout. Defaults to
	// os.Stderr when nil.
	Stderr io.Writer
}

// Program is an alias for Command -- see the Command doc comment.
type Program = Command

// stdout returns the Command's standard output stream, defaulting to
// os.Stdout when Stdout is nil. Used by PrintUsage, PrintHelp, and
// PrintVersion -- help/version output always goes here.
func (c *Command) stdout() io.Writer {
	if c.Stdout != nil {
		return c.Stdout
	}
	return os.Stdout
}

// stderr returns the Command's error output stream, defaulting to
// os.Stderr when Stderr is nil. Used by Parse for a real parsing
// error's clap-style block -- always here, independent of stdout().
func (c *Command) stderr() io.Writer {
	if c.Stderr != nil {
		return c.Stderr
	}
	return os.Stderr
}

// --- Validate ----------------------------------------------------------------

// Validate checks the Command definition for structural problems (e.g.
// duplicate flag names) that would make parsing ambiguous or generated
// help misleading. Parse calls Validate internally, so most callers won't
// need to call it directly; it is exported mainly to allow validating a
// Command early (e.g. in a test) without running Parse.
//
// A Validate failure never prints anything -- not even the short usage
// line. It means the Command itself was declared incorrectly (a
// duplicate flag, an empty Name, a Default that doesn't match its
// Dest's type, ...) -- a bug in the caller's Go code, not something an
// end user mistyped on the command line -- so there's no sensible "try
// --help" advice to attach to it, unlike a real parsing error (see the
// Parse doc comment). Parse wraps a Validate failure into an *Error with
// ExitCode() == 1.
func (c *Command) Validate() error {
	// Command.Name is required -- it's used in usage/help/version output.
	if c.Name == "" {
		return errors.New("cli: Command.Name must not be empty")
	}

	seenArgNames := make(map[string]bool, len(c.Arguments))
	for _, arg := range c.Arguments {
		// Every Argument needs a display name -- it's what shows up in
		// the usage line (e.g. "[qux]") and the Arguments: help section.
		if arg.Name == "" {
			return errors.New("cli: Argument.Name must not be empty")
		}

		// Argument names must be unique -- Parse fills positional
		// Arguments in declaration order, so a duplicate name would be
		// ambiguous in help output even though parsing itself doesn't
		// use Name as a lookup key.
		if seenArgNames[arg.Name] {
			return fmt.Errorf("cli: duplicate argument name %q", arg.Name)
		}
		seenArgNames[arg.Name] = true

		// If both Dest and Default are set, Default's concrete type must
		// match what Dest's constructor (StringVar/BoolVar/IntVar)
		// expects -- otherwise it would only fail at Parse time, or
		// silently misbehave for custom Value implementations.
		if err := validateDefaultType(arg.Dest, arg.Default); err != nil {
			return err
		}
	}

	return nil
}

// validateDefaultType checks that def, when set, matches the Go type
// expected by dest's constructor (StringVar -> string, BoolVar -> bool,
// IntVar -> int). Custom Value implementations -- ones not created by
// cli4go's own constructors -- are not checked here, since cli4go has
// no way to know what type they expect.
func validateDefaultType(dest Value, def any) error {
	if dest == nil || def == nil {
		return nil
	}

	switch dest.(type) {
	case *stringValue:
		if _, ok := def.(string); !ok {
			return fmt.Errorf("cli: Default %v (%T) is not a string, but Dest is bound via StringVar", def, def)
		}
	case *boolValue:
		if _, ok := def.(bool); !ok {
			return fmt.Errorf("cli: Default %v (%T) is not a bool, but Dest is bound via BoolVar", def, def)
		}
	case *intValue:
		if _, ok := def.(int); !ok {
			return fmt.Errorf("cli: Default %v (%T) is not an int, but Dest is bound via IntVar", def, def)
		}
	}

	return nil
}

// --- Parse -------------------------------------------------------------------

// Parse resolves every Option and Argument from args (typically
// os.Args[1:]), applying the Default -> Env -> command-line-flag
// precedence. It also handles the built-in -h/--help flag (always
// available) and the built-in -v/--version flag (available only when
// Command.Version is set) -- neither is declared in Command.Options by
// the developer.
//
// Parse returns:
//   - nil: parsing succeeded.
//   - ErrHelp / ErrVersion (ExitCode() == 0): a built-in action fired;
//     PrintHelp/PrintVersion already wrote the full text to c.Stdout.
//     errors.Is(err, ErrExit) is also true for both; use errors.Is(err,
//     ErrHelp) / errors.Is(err, ErrVersion) to tell which one fired, if
//     that matters.
//   - a Validate failure (ExitCode() == 1): the Command itself was
//     declared incorrectly -- see the Validate doc comment. Nothing is
//     printed.
//   - a real parsing/usage error (ExitCode() == 2): an unknown or
//     invalid option/argument, including a bad Env value. Parse prints
//     a clap-style block to c.Stderr before returning (see below).
//
// For a real parsing/usage error, Parse prints:
//
//	error: <message>
//	Usage: myprogram [options] [qux]
//
//	For more information, try '--help'.
//
// The "error: " prefix is display-only -- the message returned by
// Error() is never prefixed with it, so errors.Is/errors.As and any
// caller-side wrapping keep working against the bare message (e.g.
// `unknown option "--nope"`).
func (c *Command) Parse(args []string) *Error {
	if err := c.Validate(); err != nil {
		return newError(1, "%s", err)
	}

	return nil
}

// DefaultParse is a convenience wrapper around Parse that reads
// os.Args[1:] -- the common case for a program's main(). Use Parse
// directly (with an explicit slice) when testing, or when args come
// from somewhere other than the running process's own command line.
func (c *Command) DefaultParse() *Error {
	return c.Parse(os.Args[1:])
}

// --- Usage, Help, Version output ---------------------------------------------

// UsageString returns the one-line usage summary, e.g.
// "Usage: myprogram [options] [qux]\n" -- the same line HelpString shows
// first, and the line Parse prints as part of its error block for a real
// parsing error (see the Parse doc comment). For the full help text --
// with the Arguments / Options / Environment Variables sections -- see
// HelpString.
func (c *Command) UsageString() string {
	return ""
}

// PrintUsage writes UsageString (the one-line usage summary) to c.Stdout
// (os.Stdout by default).
//
// Parse does not call PrintUsage for its own -h/--help handling (it
// calls PrintHelp) or for a real parsing error (it assembles its own
// clap-style block internally -- see the Parse doc comment). PrintUsage
// remains here as a small standalone convenience, e.g. for a caller's
// own post-Parse validation that wants to show just the short line.
func (c *Command) PrintUsage() {
	fmt.Fprint(c.stdout(), c.UsageString())
}

// HelpString returns the full formatted help text: the usage line (see
// UsageString), the description (if set), and the Arguments / Options /
// Environment Variables sections.
//
// Arguments is omitted entirely when there are no Arguments.
// Environment Variables is omitted when no Option or Argument declares
// an Env name. Options is never omitted -- it always contains at least
// the synthesized -h/--help entry, plus -v/--version when Version is
// set, both always listed last.
func (c *Command) HelpString() string {
	return ""
}

// PrintHelp writes HelpString (the full help text) to c.Stdout (os.Stdout
// by default). Parse calls this for an explicit -h/--help request.
func (c *Command) PrintHelp() {
	fmt.Fprint(c.stdout(), c.HelpString())
}

// VersionString returns the formatted "name version" string shown by
// PrintVersion and by the built-in -v/--version flag.
func (c *Command) VersionString() string {
	return fmt.Sprintf("%s %s", c.Name, c.Version)
}

// PrintVersion writes VersionString to c.Stdout (os.Stdout by default).
func (c *Command) PrintVersion() {
	fmt.Fprintln(c.stdout(), c.VersionString())
}
