// Package cli provides a declarative toolkit for building command-line
// interfaces in Go, combining an argparse-like API with clap-style
// formatted help output.
package cli

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"slices"
	"strconv"
	"strings"
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

// IsBoolFlag reports that boolValue never consumes a following token --
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
//	     Parse's own applyDefaults step (see its doc comment).
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

// --- Argument, Option, EnvVar, Command, --------------------------------------

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

// Action is a hook that runs when its Option's flag is seen on the
// command line. Parse calls it as opt.Action(c, opt, value): c is the
// Command being parsed (so an Action can call
// c.PrintHelp/c.PrintVersion/c.PrintErrorf, exactly like Parse itself
// does), opt is the matched Option itself (so an Action can refer back
// to its own Flag/Long/Name/Help, e.g. when building a message through
// c.PrintErrorf), and value is the same string Parse would hand (or
// just handed) to Dest.Set: the token consumed from the command line
// by default, or "true" for a flag that opts out of consuming one --
// see Dest's doc comment and isBoolFlag; an Action-only Option (no
// Dest at all) still consumes a value by default, exactly like any
// other flag, unless its Dest is explicitly set to something
// boolean-like (a plain BoolVar, even one nothing else ever reads).
//
// An Option may set Dest, Action, both, or neither. When both are set,
// Dest resolves first -- Action only runs if Dest.Set succeeded, and
// sees the exact same value Dest was just given, so it never needs to
// re-derive or re-parse anything Dest already handled.
//
// An Action's return value tells Parse whether to keep going:
//
//	nil              -- parsing continues, exactly as if this were an
//	                    ordinary Option with no Action.
//	a non-nil *Error -- parsing stops immediately; Parse returns this
//	                    *Error as-is, unchanged.
//
// This is the same contract PrintErrorf/ErrHelp/ErrVersion already
// follow, so an Action reports a real parsing/usage error the same way
// the rest of Parse does -- `return c.PrintErrorf("...", ...)` prints
// the clap-style block and returns the *Error to return -- and is free
// to return ErrHelp, ErrVersion, or any other *Error (e.g. built with
// newError) if that's the exit behavior it wants. This is how the
// built-in -h/--help and -v/--version flags get their behavior (see
// allOptions): each binds Dest to a BoolVar over a throwaway bool
// (never consuming a token, value always "true") and sets Action to a
// closure that prints and returns ErrHelp/ErrVersion. A caller-declared
// Option can set its own Action exactly the same way, with no special
// status over the built-in ones -- optionally alongside a Dest of its
// own, and opting out of consuming a value the exact same way the
// built-ins do.
type Action func(c *Command, opt Option, value string) *Error

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
	//
	// Declaring both this and the matching Long form of a built-in
	// (-h and --help, or -v and --version), whether on one Option or
	// split across several, suppresses that built-in entry from the
	// generated help and from Parse's dispatch; declaring only one of
	// the two forms is a conflict, rejected by Validate. The one
	// exception is -v: declaring it without --version makes the
	// version built-in use -V instead. See allOptions' doc comment.
	Flag string

	// Long is the long flag form, e.g. "bar" or "--bar" -- the leading "--"
	// is optional and added automatically if omitted. Optional, though at
	// least one of Flag/Long should be set.
	//
	// See Flag's doc comment for how declaring a built-in's forms
	// affects that built-in. In particular, declaring "--version"
	// while leaving "-v" free is a conflict, not a fallback: only a
	// claimed "-v" makes the version built-in switch to "-V".
	Long string

	// Help explains what the option does. Shown in the generated
	// "Options:" help section.
	Help string

	// Dest binds the parsed value to a variable. See StringVar, BoolVar,
	// IntVar. A BoolVar binding makes this a boolean flag that never
	// consumes a following token; leaving Dest nil does NOT do the
	// same when Action is set -- see the Action doc comment. May be
	// combined with Action -- see the Action doc comment for the exact
	// resolution order.
	Dest Value

	// Default is shown in the help text as "(default: ...)" and is
	// assigned to Dest before Env and the command line are considered.
	Default any

	// Env, if set, is the environment variable consulted when this
	// option is not provided on the command line.
	Env string

	// Action, if set, runs when this Option's flag is seen on the
	// command line -- see the Action doc comment for its exact contract
	// and how it combines with Dest. Used by allOptions to give its
	// synthesized -h/--help and -v/--version entries real behavior; a
	// caller-declared Option can set this too, optionally alongside
	// Dest, with no special status over the built-in ones.
	Action Action
}

// flag returns o.Flag in its normalized short-flag form: a single
// leading dash followed by the rest of the string with any existing
// leading dashes stripped. "f", "-f", and "--f" all normalize to "-f".
// Idempotent on an already-normalized or empty o.Flag.
func (o Option) flag() string {
	if o.Flag == "" {
		return o.Flag
	}
	return "-" + strings.TrimLeft(o.Flag, "-")
}

// long returns o.Long in its normalized long-flag form: two leading
// dashes followed by the rest of the string with any existing leading
// dashes stripped. "bar", "-bar", "--bar", and "---bar" all normalize to
// "--bar" -- a single leading dash is treated the same as no dash at
// all, so "-bar" (a likely typo) is normalized rather than rejected.
// Idempotent on an already-normalized or empty o.Long.
func (o Option) long() string {
	if o.Long == "" {
		return o.Long
	}
	return "--" + strings.TrimLeft(o.Long, "-")
}

// EnvVar describes a standalone environment variable: one not bound to
// any Argument or Option's Env field. Unlike Argument/Option, it has no
// Flag/Long -- it is reachable only via the environment, never via a
// command-line token -- so its value precedence is just Default -> Env,
// with no flag position to override it.
type EnvVar struct {
	// Name is the environment variable name.
	Name string

	// Help explains what the environment variable is for. Shown in the
	// generated "Environment Variables:" help section.
	Help string

	// Dest binds the resolved value to a variable. See StringVar,
	// BoolVar, IntVar. Optional -- an EnvVar with no Dest is purely
	// informational, shown in help but not resolved into anything (the
	// consuming application reads it itself, e.g. via os.Getenv).
	Dest Value

	// Default is shown in the help text as "(default: ...)" and is
	// assigned to Dest before Env is considered.
	Default any
}

// Command describes a complete command-line interface: its identity
// (Name/Version/Description), its positional Arguments, and its Options
// (flags). A Command is plain data — build it as a struct literal, then
// call Parse to read os.Args into the bound variables.
//
// Program is an alias for Command, kept as the conventional name for the
// common single-command case (a CLI with no subcommands). The two are
// identical to the compiler. There is currently no mechanism for
// declaring subcommands; a future Commands field and its routing design
// are still being worked out.
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

	// EnvVars are standalone environment variables -- not bound to any
	// Argument or Option -- shown in the generated "Environment
	// Variables:" help section. allEnvVars yields Arguments' Env entries
	// first, then Options', then EnvVars, in that order.
	EnvVars []EnvVar

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

// allOptions returns a generator function that yields every Option in
// the Command, including the built-in -h/--help and -v/--version entries.
// The generator yields user-declared Options first, then the built-in
// entries last. The generator stops yielding when yield returns false.
//
// A built-in entry is suppressed entirely -- not yielded at all --
// when the user-declared Options already claim both of its normalized
// forms. The two forms may be claimed by a single Option or by two
// different ones; only the set of claimed forms matters. Suppression
// is per built-in, not per form: claiming only
// -h (and not --help) does NOT suppress the help built-in, it
// collides with it, and validateOptions rejects that collision as a
// duplicate flag. Claiming both -h and --help, on the other hand,
// suppresses the help built-in cleanly -- the user's declaration
// wins, and there is no duplicate to reject. The same rule applies
// to -v/--version, and only when Version is set.
//
// The version built-in has one extra rule: when the user-declared Options
// claim -v but not --version, the built-in is not a collision. It is
// yielded with -V as its short form instead, since -v is commonly taken
// by "verbose" and -v/-V is an established convention. If -V is claimed
// too, the short form is dropped and only --version is yielded. Help has
// no such fallback: -H conventionally means "host", so the help built-in
// is either -h/--help or suppressed.
func (c *Command) allOptions() iter.Seq[Option] {
	return func(yield func(Option) bool) {
		for _, opt := range c.Options {
			if !yield(opt) {
				return
			}
		}

		// -h/--help and -v/--version must never consume a following
		// token, so each binds Dest to a plain BoolVar -- an Option
		// with an Action but no Dest otherwise defaults to consuming a
		// value (see the value-determination comment in Parse), and a
		// boolean Dest is how any Option opts back out of that, built-in
		// or not. discard is never read; it exists purely so BoolVar has
		// somewhere to write "true" -- neither built-in has any value of
		// its own to store.
		var discard bool

		// Which normalized Flag/Long forms the user-declared Options
		// claim, so the suppression/fallback rules below can be
		// decided per built-in. "Taken" is per-token: a form counts
		// whether or not the Option claiming it also declares the
		// other form, and whether or not both of a built-in's forms
		// come from the same Option -- see the doc comment above.
		takenFlags := make(map[string]bool, len(c.Options))
		takenLongs := make(map[string]bool, len(c.Options))
		for _, opt := range c.Options {
			if f := opt.flag(); f != "" {
				takenFlags[f] = true
			}
			if l := opt.long(); l != "" {
				takenLongs[l] = true
			}
		}

		// Suppress the help built-in only when the user has claimed
		// BOTH of its forms. A partial claim (only -h, or only
		// --help) leaves the built-in intact, and validateOptions
		// then reports the resulting duplicate.
		if !(takenFlags["-h"] && takenLongs["--help"]) {
			if !yield(Option{
				Flag: "-h", Long: "--help", Help: "Print help",
				Dest: BoolVar(&discard),
				Action: func(c *Command, opt Option, value string) *Error {
					c.PrintHelp()
					return ErrHelp
				},
			}) {
				return
			}
		}

		if c.Version != "" {
			// -v is overloaded in the wild (verbose vs. version), so when
			// a user-declared Option already claims it, fall back to -V,
			// the conventional capital-letter alternative. If -V is taken
			// too, the short form is dropped and only --version remains.
			// When --version is claimed as well, both forms are taken and
			// the built-in is suppressed, exactly as for help.
			short := "-v"
			if takenFlags["-v"] {
				switch {
				case takenLongs["--version"]:
					return
				case takenFlags["-V"]:
					short = ""
				default:
					short = "-V"
				}
			}

			yield(Option{
				Flag: short, Long: "--version", Help: "Print version",
				Dest: BoolVar(&discard),
				Action: func(c *Command, opt Option, value string) *Error {
					c.PrintVersion()
					return ErrVersion
				},
			})
		}
	}
}

// allEnvVars returns a generator function that yields an EnvVar for
// every Argument and Option in the Command that defines an Env field,
// followed by every entry in EnvVars itself. The generator yields
// environment variables from Arguments first, then Options, then
// EnvVars, in that order. The generator stops yielding when yield
// returns false.
//
// Each yielded EnvVar carries its originating Argument/Option's Dest
// and Default alongside Name/Help -- not just what HelpString needs to
// render the "Environment Variables:" section, but everything Parse's
// Default -> Env resolution phase needs too, so Parse can walk this
// exact same sequence instead of re-deriving it over Arguments/Options
// on its own.
func (c *Command) allEnvVars() iter.Seq[EnvVar] {
	return func(yield func(EnvVar) bool) {
		for _, arg := range c.Arguments {
			if arg.Env != "" {
				if !yield(EnvVar{Name: arg.Env, Help: arg.Help, Dest: arg.Dest, Default: arg.Default}) {
					return
				}
			}
		}

		for opt := range c.allOptions() {
			if opt.Env != "" {
				if !yield(EnvVar{Name: opt.Env, Help: opt.Help, Dest: opt.Dest, Default: opt.Default}) {
					return
				}
			}
		}

		for _, env := range c.EnvVars {
			if !yield(env) {
				return
			}
		}
	}
}

// allDefaults returns a generator function that yields the (Dest,
// Default) pair for every Argument, Option, and standalone EnvVar, in
// that order -- unconditionally, unlike allEnvVars() (which only
// yields entries that declare an Env name): a Default applies whether
// or not the item also has an Env, so this doesn't filter by Env, or
// even by whether Dest/Default themselves are set -- applyDefaults'
// own nil checks decide which pairs actually do anything. The
// generator stops yielding when yield returns false.
func (c *Command) allDefaults() iter.Seq2[Value, any] {
	return func(yield func(Value, any) bool) {
		for _, arg := range c.Arguments {
			if !yield(arg.Dest, arg.Default) {
				return
			}
		}

		for _, opt := range c.Options {
			if !yield(opt.Dest, opt.Default) {
				return
			}
		}

		for _, env := range c.EnvVars {
			if !yield(env.Dest, env.Default) {
				return
			}
		}
	}
}

// --- Validate ----------------------------------------------------------------

// Validate checks the Command definition for structural problems (e.g.
// duplicate flag names) that would make parsing ambiguous or generated
// help misleading. Parse calls Validate internally, so most callers won't
// need to call it directly; it is exported mainly to allow validating a
// Command early (e.g. in a test) without running Parse.
//
// Checks, in order:
//  1. Command.Name non-empty.
//  2. Every Argument.Name non-empty and unique.
//  3. Every Argument's Default (if set) type-matches its Dest's concrete
//     type.
//  4. Every Option has Flag and/or Long set.
//  5. Normalized Flag is exactly 2 characters if set.
//  6. A built-in (-h/--help, -v/--version) whose both normalized forms
//     are claimed by the user-declared Options is suppressed, not
//     yielded by allOptions(). The version built-in also falls back to
//     -V (or to --version alone) when -v is claimed without
//     --version; see allOptions' doc comment.
//  7. Normalized Flag and Long are each unique across the effective
//     Option set (allOptions() -- user-declared Options plus whichever
//     built-ins survived suppression), so a partial claim on a
//     built-in's forms is rejected as a real conflict.
//  8. Same Default/Dest type check as Arguments, for Options.
//  9. Every standalone EnvVar.Name is non-empty.
//  10. Every environment variable name -- from Arguments' Env, Options'
//     Env, and EnvVars -- is unique across all three.
//  11. Same Default/Dest type check as Arguments/Options, for EnvVars.
//
// Dest and Action are not checked against each other -- an Option may
// set neither, either, or both (see Option.Action's doc comment for
// what setting both means for Parse's dispatch order).
//
// A Validate failure never prints anything -- see the package-level
// design notes for why (a Validate failure is a bug in the caller's Go
// code, not something an end user mistyped). It always reports
// ExitCode() == 1 -- see Error.ExitCode's doc comment -- so a caller
// using Validate on its own (outside Parse) gets the same
// os.Exit(err.ExitCode()) idiom Parse/DefaultParse already document.
func (c *Command) Validate() *Error {
	// Command.Name is required -- it's used in usage/help/version output.
	// This check has nowhere else to live: it isn't part of either loop
	// below, so it stays here rather than in a helper.
	if c.Name == "" {
		return newError(1, "cli: Command.Name must not be empty")
	}

	// Arguments validation
	if err := validateArguments(c.Arguments); err != nil {
		return newError(1, "%s", err)
	}

	// Options validation. Runs against the effective Option set --
	// c.allOptions(), not c.Options -- so a user-declared Option that
	// collides with a synthesized built-in (a partial claim on -h or
	// --help, say) is caught here exactly like any other duplicate
	// flag. See allOptions' doc comment for the suppression rule.
	if err := validateOptions(slices.Collect(c.allOptions())); err != nil {
		return newError(1, "%s", err)
	}

	// Environment variable validation -- spans Arguments, Options, and
	// EnvVars together, so it can't live in either helper above.
	if err := c.validateEnvVars(); err != nil {
		return newError(1, "%s", err)
	}

	return nil
}

// validateArguments checks args for structural problems: an empty or
// duplicate Name, or a Default whose type doesn't match Dest. It is a
// free function (not a *Command method) so it stays decoupled from
// Command and testable on its own, mirroring validateDefaultType below.
func validateArguments(args []Argument) error {
	seenNames := make(map[string]bool, len(args))
	for _, arg := range args {
		// Every Argument needs a display name -- it's what shows up in
		// the usage line (e.g. "[qux]") and the Arguments: help section.
		if arg.Name == "" {
			return errors.New("cli: Argument.Name must not be empty")
		}

		// Argument names must be unique -- Parse fills positional
		// Arguments in declaration order, so a duplicate name would be
		// ambiguous in help output even though parsing itself doesn't
		// use Name as a lookup key.
		if seenNames[arg.Name] {
			return fmt.Errorf("cli: duplicate argument name %q", arg.Name)
		}
		seenNames[arg.Name] = true

		if err := validateDefaultType(arg.Dest, arg.Default); err != nil {
			return err
		}
	}
	return nil
}

// validateOptions checks opts for structural problems: a missing
// Flag/Long, a malformed Flag, a duplicate Flag/Long, or a Default
// whose type doesn't match Dest. Same free-function shape as
// validateArguments, for the same reason.
//
// Validate calls this with the effective Option set
// (slices.Collect(c.allOptions())), not c.Options, so a user-declared
// Option colliding with a synthesized built-in is caught here exactly
// like two user-declared Options colliding. See allOptions' doc
// comment for the suppression rule that decides which built-ins are
// even present in that effective set.
func validateOptions(opts []Option) error {
	seenFlag := make(map[string]bool, len(opts))
	seenLong := make(map[string]bool, len(opts))
	for _, opt := range opts {
		// Compare Flag/Long in their normalized form, so "-f"/"f" or
		// "--bar"/"bar" on two different Options are correctly treated
		// as the same form instead of slipping past the uniqueness
		// checks below as if they were different.
		flag := opt.flag()
		long := opt.long()

		// An Option needs at least one of Flag/Long to be reachable from
		// the command line at all.
		if flag == "" && long == "" {
			return errors.New("cli: Option must set Flag and/or Long")
		}

		if flag != "" {
			// The short form's fixed length ("-x") is what lets help
			// output use a constant-width Flag column instead of
			// computing one per Command.
			if len(flag) != 2 {
				return fmt.Errorf("cli: Option.Flag %q must be a dash followed by exactly one character", opt.Flag)
			}
			if seenFlag[flag] {
				return fmt.Errorf("cli: duplicate option flag form %q", flag)
			}
			seenFlag[flag] = true
		}

		if long != "" {
			// Same reasoning as Flag above: Parse looks up an incoming
			// "--xxx" token by its normalized form, so Long must be
			// unique across Options too.
			if seenLong[long] {
				return fmt.Errorf("cli: duplicate option long form %q", long)
			}
			seenLong[long] = true
		}

		if err := validateDefaultType(opt.Dest, opt.Default); err != nil {
			return err
		}
	}
	return nil
}

// validateEnvVars checks that every standalone EnvVar.Name is non-empty
// (mirroring the Argument.Name check in validateArguments), that no
// environment variable name is declared more than once across
// Arguments' Env, Options' Env, and EnvVars, and that each EnvVar's
// Default (if set) type-matches its Dest -- the same check
// validateArguments/validateOptions run for Argument/Option. A
// duplicate name would be genuinely ambiguous -- two entries claiming
// the same environment variable -- and would render as a literal
// duplicate row in the generated "Environment Variables:" help section.
//
// Unlike validateArguments/validateOptions, this is a *Command method
// rather than a free function: the name-uniqueness check is inherently
// cross-slice (it has to see Arguments, Options, and EnvVars together),
// so there's no single []T it could take instead.
func (c *Command) validateEnvVars() error {
	seen := make(map[string]bool)

	for _, arg := range c.Arguments {
		if arg.Env == "" {
			continue
		}
		if seen[arg.Env] {
			return fmt.Errorf("cli: duplicate environment variable %q", arg.Env)
		}
		seen[arg.Env] = true
	}

	for _, opt := range c.Options {
		if opt.Env == "" {
			continue
		}
		if seen[opt.Env] {
			return fmt.Errorf("cli: duplicate environment variable %q", opt.Env)
		}
		seen[opt.Env] = true
	}

	for _, env := range c.EnvVars {
		if env.Name == "" {
			return errors.New("cli: EnvVar.Name must not be empty")
		}
		if seen[env.Name] {
			return fmt.Errorf("cli: duplicate environment variable %q", env.Name)
		}
		seen[env.Name] = true

		if err := validateDefaultType(env.Dest, env.Default); err != nil {
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
//   - a code-1 failure (ExitCode() == 1): the Command itself was
//     declared incorrectly -- from Validate() or from Parse's own
//     applyDefaults step -- see their doc comments. Nothing is
//     printed.
//   - a real parsing/usage error (ExitCode() == 2): an unknown or
//     invalid option/argument, including a bad Env value, or whatever
//     *Error a caller-declared Option's Action chose to return. Parse
//     prints a clap-style block to c.Stderr before returning (see
//     below) for everything except an Action's own *Error, which
//     Parse returns exactly as given -- an Action that wants that
//     same clap-style block builds it itself via c.PrintErrorf.
//
// For a real parsing/usage error Parse builds itself, Parse prints (via
// PrintErrorf -- see its doc comment):
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
	// Reject a Command that was declared incorrectly.
	if err := c.Validate(); err != nil {
		return err
	}

	// Default -> Env resolution, the Default half.
	if err := c.applyDefaults(); err != nil {
		return err
	}

	// Default -> Env resolution, the Env half.
	if err := c.applyEnvVars(); err != nil {
		return err
	}

	// Normalized lookup maps built once, ahead of the token loop, so
	// each dash-prefixed token below resolves to its Option in O(1)
	// instead of scanning c.Options per token. Built from allOptions(),
	// not c.Options directly, so the synthesized -h/--help and
	// -v/--version entries share this exact same lookup and dispatch
	// path -- Parse never special-cases their tokens or their
	// behavior; it just resolves Dest/Action the same way it would for
	// any other Option.
	flagOpts := make(map[string]Option, len(c.Options)+2)
	longOpts := make(map[string]Option, len(c.Options)+2)
	for opt := range c.allOptions() {
		if flag := opt.flag(); flag != "" {
			flagOpts[flag] = opt
		}
		if long := opt.long(); long != "" {
			longOpts[long] = opt
		}
	}

	// Classic index loop, not range: consuming a value-taking Option's
	// value requires advancing i an extra position mid-iteration.
	argIndex := 0
	for i := 0; i < len(args); i++ {
		tok := args[i]

		switch {
		case strings.HasPrefix(tok, "-"):
			opt, ok := flagOpts[tok]
			if !ok {
				opt, ok = longOpts[tok]
			}
			if !ok {
				return c.PrintErrorf("unknown option %q", tok)
			}

			// value is what Dest.Set (if Dest is set) and Action (if
			// Action is set) both receive. The default is to consume
			// the next token, same as any ordinary value-taking flag
			// -- including an Action-only Option with no Dest at all,
			// so an Action always gets a real value unless something
			// says otherwise. The one thing that says otherwise is
			// isBoolFlag(opt.Dest): a boolean Dest (BoolVar) makes
			// this Option boolean-like, so value is just "true" and no
			// token is consumed. isBoolFlag(nil) is false, so a nil
			// Dest does NOT opt out on its own -- an Action-only
			// Option that must never consume a token needs an explicit
			// BoolVar Dest to say so, the way the built-in -h/--help
			// and -v/--version do (see allOptions).
			value := "true"
			if !isBoolFlag(opt.Dest) {
				i++
				if i >= len(args) {
					return c.PrintErrorf("option %q requires a value", tok)
				}
				value = args[i]
			}

			// Dest resolves first, if set: Action (below) receives the
			// exact same value, and only runs at all if Dest.Set
			// succeeded.
			if opt.Dest != nil {
				if setErr := opt.Dest.Set(value); setErr != nil {
					return c.PrintErrorf("option %q: %w", tok, setErr)
				}
			}

			// See the Action doc comment for its nil/non-nil contract.
			if opt.Action != nil {
				if err := opt.Action(c, opt, value); err != nil {
					return err
				}
			}

		default:
			// A non-dash token fills the next declared Argument, in
			// declaration order.
			if argIndex >= len(c.Arguments) {
				return c.PrintErrorf("unexpected argument %q", tok)
			}
			arg := c.Arguments[argIndex]
			argIndex++

			if arg.Dest == nil {
				continue
			}
			if setErr := arg.Dest.Set(tok); setErr != nil {
				return c.PrintErrorf("argument %q: %w", tok, setErr)
			}
		}
	}

	return nil
}

// applyDefaults assigns each Argument, Option, and standalone EnvVar's
// Default (if set) to its Dest, via allDefaults() -- the Default half
// of Default -> Env resolution. Parse always runs applyEnvVars right
// after (the Env half), so a present environment variable still
// overrides whatever Default set here.
//
// def's type has already been checked against dest's concrete type by
// validateDefaultType/Validate for the three built-in Value kinds
// (StringVar/BoolVar/IntVar), so a Set failure here can only happen
// for a custom Value implementation with an incompatible Default --
// which validateDefaultType deliberately can't check (see its doc
// comment). That makes it the same kind of problem Validate() itself
// reports: the Command was declared incorrectly, not something the
// end user typed -- so, like Validate(), it reports ExitCode() == 1
// and prints nothing, unlike a bad command-line or Env value (which
// goes through PrintErrorf instead).
func (c *Command) applyDefaults() *Error {
	for dest, def := range c.allDefaults() {
		if dest == nil || def == nil {
			continue
		}
		if setErr := dest.Set(fmt.Sprint(def)); setErr != nil {
			return newError(1, "%s", fmt.Errorf("cli: invalid default value %v: %w", def, setErr))
		}
	}

	return nil
}

// applyEnvVars resolves the Env half of Default -> Env for every
// Argument, Option, and standalone EnvVar -- in the order allEnvVars()
// yields them (Arguments, then Options, then EnvVars): when Dest is
// bound and the named variable is present in the environment, its
// value overrides whatever Dest already holds (applyDefaults having
// already run first -- see Parse).
//
// A bad value is a real parsing/usage error (ExitCode() == 2),
// already printed via PrintErrorf by the time this returns it, so
// Parse can return it as-is.
func (c *Command) applyEnvVars() *Error {
	for env := range c.allEnvVars() {
		if env.Name == "" || env.Dest == nil {
			continue
		}

		val, ok := os.LookupEnv(env.Name)
		if !ok {
			continue
		}

		if setErr := env.Dest.Set(val); setErr != nil {
			return c.PrintErrorf("invalid value %q for environment variable %q: %w", val, env.Name, setErr)
		}
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

// --- Error output, Usage, Help, Version --------------------------------------

// ErrorString returns the clap-style error block for a real
// parsing/usage error: the "error: <message>" line, the usage line (see
// UsageString), and a pointer to '--help'. Parse uses this internally
// for every real parsing/usage error (ExitCode() == 2) before returning
// -- see the Parse doc comment for the exact block layout.
//
// err is the bare underlying error (e.g. fmt.Errorf("unknown option
// %q", tok)) -- the "error: " prefix added here is purely for display,
// mirroring the same rule already documented on Error.Error(): the
// message stored in the *Error Parse eventually returns is never
// prefixed with it, so errors.Is/errors.As and any caller-side wrapping
// keep working against the bare message.
//
// Exported for the same reason PrintUsage is: it remains available as a
// small standalone convenience for a caller's own post-Parse validation
// (e.g. a future cross-field check) that wants the same clap-style
// formatting for its own errors.
func (c *Command) ErrorString(err error) string {
	var b strings.Builder
	fmt.Fprintf(&b, "error: %v\n", err)
	b.WriteString(c.UsageString())
	b.WriteString("\nFor more information, try '--help'.\n")
	return b.String()
}

// PrintErrorf formats err per format/args -- exactly like fmt.Errorf,
// including %w to wrap an underlying error -- writes ErrorString of
// the result to c.Stderr (os.Stderr by default), and returns the
// *Error the caller should return in turn. Unlike
// PrintUsage/PrintHelp/PrintVersion -- which always write to Stdout
// and return nothing -- PrintErrorf writes to Stderr and returns a
// value, because formatting the error, printing the block, and
// building the *Error to return are the same step at every call site:
// every clap-style error block this package ever prints represents a
// real parsing/usage error, so the exit code is always 2 (see
// Error.ExitCode's doc comment) -- there is no code-1 or code-0 case
// that goes through PrintErrorf.
//
// The "f" suffix matters beyond naming convention: it is what lets
// `go vet`'s printf checker recognize format/args here and catch a
// mismatched verb -- e.g. %w only wraps inside fmt.Errorf/PrintErrorf;
// used with a plain fmt.Sprintf-based helper (no "f" suffix) it just
// prints as an unrecognized verb instead of wrapping the error.
//
// A caller that already holds a plain error rather than building one
// here can still use PrintErrorf -- pass it through with "%s", err,
// the same way Validate's own failure is wrapped into a *Error via
// newError(1, "%s", err). A caller-declared Option's Action is a
// caller too, in exactly this sense -- see the Action doc comment.
func (c *Command) PrintErrorf(format string, args ...any) *Error {
	err := fmt.Errorf(format, args...)
	fmt.Fprint(c.stderr(), c.ErrorString(err))
	return newError(2, "%s", err)
}

// UsageString returns the one-line usage summary, e.g.
// "Usage: myprogram [options] [qux]\n" -- the same line HelpString shows
// first, and the line Parse prints as part of its error block for a real
// parsing error (see the Parse doc comment). For the full help text --
// with the Arguments / Options / Environment Variables sections -- see
// HelpString.
func (c *Command) UsageString() string {
	// Build the "[arg1] [arg2] ..." part of the usage line from the
	// Command's Arguments. Options are always represented by the
	// "[options]" placeholder, so they don't need to be listed here.
	var args strings.Builder
	for _, arg := range c.Arguments {
		fmt.Fprintf(&args, " [%s]", arg.Name)
	}

	// The usage line is always "Usage: <program name> [options] [arg1] [arg2] ..."
	return fmt.Sprintf("Usage: %s [options]%s\n", c.Name, args.String())
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
// the synthesized -h/--help entry (unless the user-declared Options
// claim both of its forms -- see allOptions' doc comment), plus
// -v/--version when Version is set, both always listed last.
func (c *Command) HelpString() string {
	var b strings.Builder

	// Compute the shared column width: the longest Argument.Name, and the
	// longest normalized long flag (e.g. "--bar") plus 4 characters for the
	// "-x, " short flag column an Option row has but an Argument row doesn't.
	// The two are computed separately (rather than folded into one loop) so a
	// future per-section alignment mode only needs to change how they're
	// combined below, not how each is computed.
	widthArgs := 0
	for _, arg := range c.Arguments {
		w := len(arg.Name)
		if w > widthArgs {
			widthArgs = w
		}
	}
	widthOpts := 0
	for opt := range c.allOptions() {
		w := len(opt.long()) + 4 // 4 = len("-x, ")
		if w > widthOpts {
			widthOpts = w
		}
	}
	widthEnvs := 0
	for env := range c.allEnvVars() {
		w := len(env.Name)
		if w > widthEnvs {
			widthEnvs = w
		}
	}
	width := max(widthArgs, widthOpts, widthEnvs)

	// Usage line is always first, so the user sees it immediately.
	b.WriteString(c.UsageString())

	// Description is optional, so only show it when set.
	if c.Description != "" {
		fmt.Fprintf(&b, "\n%s\n", c.Description)
	}

	// Arguments section (Symmetrically aligned with Options)
	if len(c.Arguments) > 0 {
		fmt.Fprintf(&b, "\nArguments:\n")
		for _, arg := range c.Arguments {
			fmt.Fprintf(&b, "  %-*s   %s\n", width, arg.Name, arg.Help)
		}
	}

	// allOptions() yields the user-declared Options, then the built-in -h/--help
	// and -v/--version entries (if Version is set). The latter two are always
	// listed last, so they don't get lost in the middle of a long list of user
	// flags. A built-in is omitted only when the user claims both of its
	// forms (see allOptions' doc comment), in which case a user-declared
	// Option takes its place, so the Options section is never empty.
	fmt.Fprintf(&b, "\nOptions:\n")
	for opt := range c.allOptions() {
		flag := opt.flag()
		long := opt.long()
		if flag != "" && long != "" {
			flag += ","
		}
		fmt.Fprintf(&b,
			"  %-3s %-*s   %s\n",
			flag, width-4, opt.long(), opt.Help, // 4 = len("-x, ")
		)
	}

	// Environment Variables section, only render it if we actually found at least
	// one environment variable
	if widthEnvs > 0 {
		fmt.Fprintf(&b, "\nEnvironment Variables:\n")
		for env := range c.allEnvVars() {
			fmt.Fprintf(&b, "  %-*s   %s\n", width, env.Name, env.Help)
		}
	}

	return b.String()
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
