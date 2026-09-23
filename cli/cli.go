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
	"strings"
)

// --- Argument, Option, EnvVar, Command, --------------------------------------

// Argument describes a single positional argument. Its position in
// Command.Arguments determines the order in which it is expected on the
// command line.
//
// Argument is implemented by a family of typed structs -- StringArgument
// and IntArgument today, with more (BoolArgument, ...) planned -- instead
// of by one struct carrying a Dest/Default pair whose concrete type has
// to be inferred at runtime (the old design, still visible as comments
// in cli.go on the exp/typed-fields branch). Each concrete type owns its
// Dest and Default as its own Go type (e.g. *string/string, *int/int),
// so there is nothing to type-switch on, and the compiler keeps Default
// in sync with Dest for free.
//
// The interface itself is unexported-method-only, mirroring the
// Option.flag()/Option.long() pattern already used elsewhere in this
// package: a concrete type keeps its fields public (Name, Help, ...) for
// struct-literal construction, and satisfies Argument through small
// unexported accessor methods with different names, so field and method
// never collide.
type Argument interface {
	name() string
	help() string
	env() string

	// set parses s into Dest, the token Parse (or an Env lookup) just
	// resolved for this Argument. It returns whatever conversion error
	// the concrete type produces (none for a string), and does nothing
	// -- returning nil -- when Dest is unset.
	set(s string) error

	// setDefault assigns Default to Dest directly, when Dest is set --
	// no string round trip, and it can never fail, since Default and
	// Dest are the same Go type. The zero value applies too, like any
	// other Default (see StringArgument's Default doc comment).
	setDefault()
}

// StringArgument describes a single string-typed positional argument --
// the Argument implementation for the common case of an argument whose
// value is just the token as typed, with no conversion. It carries the
// exact same fields the old, unified Argument struct did; only the name
// and its place in the type system have changed.
type StringArgument struct {
	// Name is the argument's display name, used in generated usage text
	// (e.g. "qux" in "Usage: myprogram [qux]").
	Name string

	// Help explains what the argument is for. Shown in the generated
	// "Arguments:" help section.
	Help string

	// Dest binds the parsed value directly to a string variable, e.g.
	// &cfg.Qux. Optional.
	Dest *string

	// Default is shown in the help text as "(default: ...)" and is
	// assigned to Dest before Env and the command line are considered.
	// Its zero value ("") applies just like any other value -- there
	// is no separate "unset" state, the same way a stdlib flag always
	// has a default, even an empty one.
	Default string

	// Env, if set, is the environment variable consulted when this
	// argument is not provided on the command line.
	Env string
}

func (a *StringArgument) name() string { return a.Name }
func (a *StringArgument) help() string { return a.Help }
func (a *StringArgument) env() string  { return a.Env }

func (a *StringArgument) set(s string) error {
	if a.Dest != nil {
		*a.Dest = s
	}
	return nil
}

func (a *StringArgument) setDefault() {
	if a.Dest != nil {
		*a.Dest = a.Default
	}
}

// IntArgument describes a single int-typed positional argument -- the
// Argument implementation for a value that must parse as a Go int
// (strconv.Atoi). Same fields and shape as StringArgument; see its doc
// comment for Dest/Default's ergonomics and semantics.
type IntArgument struct {
	// Name -- see StringArgument.Name.
	Name string

	// Help -- see StringArgument.Help.
	Help string

	// Dest binds the parsed value directly to an int variable, e.g.
	// &cfg.Count. Optional.
	Dest *int

	// Default -- see StringArgument.Default.
	Default int

	// Env -- see StringArgument.Env.
	Env string
}

func (a *IntArgument) name() string { return a.Name }
func (a *IntArgument) help() string { return a.Help }
func (a *IntArgument) env() string  { return a.Env }

func (a *IntArgument) set(s string) error {
	if a.Dest == nil {
		return nil
	}
	n, err := parseInt(s)
	if err != nil {
		return err
	}
	*a.Dest = n
	return nil
}

func (a *IntArgument) setDefault() {
	if a.Dest != nil {
		*a.Dest = a.Default
	}
}

// Option describes a single flag (e.g. "-f, --foo"). Unlike Argument, the
// order of Options in Command.Options has no effect on parsing: flags can
// appear in any order on the command line.
//
// Option is implemented by a family of typed structs -- StringOption,
// IntOption, and BoolOption today -- instead of one struct carrying a
// Dest/Default pair whose concrete type has to be inferred at runtime.
// See StringOption, IntOption, and BoolOption, and Argument's doc
// comment above for the same reasoning applied there.
//
// The interface itself is unexported-method-only: a concrete type keeps
// its fields public (Name, Flag, Long, ...) for struct-literal
// construction, and satisfies Option through small unexported accessor
// methods with different names from the fields they read, so field and
// method never collide -- the same pattern the old Option.flag()/
// Option.long() already used before Option became an interface.
type Option interface {
	name() string
	help() string
	env() string
	set(s string) error // see Argument.set
	setDefault()        // see Argument.setDefault
	flag() string
	long() string
	action() Action
}

// normalizeDashes returns s with exactly dashes leading dashes, first
// stripping whatever dashes s already has -- the shared rule behind
// both a Flag ("f", "-f", "--f" -> "-f" with dashes=1) and a Long
// ("bar", "-bar", "--bar", "---bar" -> "--bar" with dashes=2). A single
// leading dash on what's meant to be a Long is treated the same as no
// dash at all, so "-bar" (a likely typo) is normalized rather than
// rejected. Idempotent on an already-normalized or empty s. Shared by
// every Option implementation's flag()/long() methods, so the
// normalization rule only lives in one place.
func normalizeDashes(s string, dashes int) string {
	if s == "" {
		return s
	}
	return strings.Repeat("-", dashes) + strings.TrimLeft(s, "-")
}

// optionName returns the identifier of an Option: name when set,
// otherwise long without its leading "--", otherwise flag without its
// leading "-". flag and long are expected in normalized form (see
// normalizeDashes). validateOptions guarantees at least one of the two
// is set, so the result is never empty for a validated Option. Shared
// by every Option implementation's name() method.
func optionName(name, flag, long string) string {
	switch {
	case name != "":
		return name
	case long != "":
		return strings.TrimPrefix(long, "--")
	default:
		return strings.TrimPrefix(flag, "-")
	}
}

// StringOption describes a single string-typed flag -- the Option
// implementation for the common case of a flag whose value is just the
// token as typed, with no conversion. It carries the exact same fields
// the old, unified Option struct did; only the name and its place in
// the type system have changed.
type StringOption struct {
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
	// version built-in use -V instead. See options' doc comment.
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

	// Dest binds the parsed value directly to a string variable, e.g.
	// &cfg.Bar. Optional. May be combined with Action -- see the Action
	// doc comment for the exact resolution order.
	Dest *string

	// Default is shown in the help text as "(default: ...)" and is
	// assigned to Dest before Env and the command line are considered.
	// Its zero value ("") applies just like any other value -- there
	// is no separate "unset" state, the same way a stdlib flag always
	// has a default, even an empty one.
	Default string

	// Env, if set, is the environment variable consulted when this
	// option is not provided on the command line.
	Env string

	// Action, if set, runs when this Option's flag is seen on the
	// command line -- see the Action doc comment for its exact contract
	// and how it combines with Dest.
	Action Action
}

func (o *StringOption) name() string { return optionName(o.Name, o.flag(), o.long()) }
func (o *StringOption) help() string { return o.Help }
func (o *StringOption) env() string  { return o.Env }

func (o *StringOption) set(s string) error {
	if o.Dest != nil {
		*o.Dest = s
	}
	return nil
}

func (o *StringOption) setDefault() {
	if o.Dest != nil {
		*o.Dest = o.Default
	}
}

func (o *StringOption) flag() string   { return normalizeDashes(o.Flag, 1) }
func (o *StringOption) long() string   { return normalizeDashes(o.Long, 2) }
func (o *StringOption) action() Action { return o.Action }

// IntOption describes a single flag whose value must parse as a Go int
// (strconv.Atoi) -- the Option implementation for the common case of an
// integer-valued flag, e.g. "-n 7". Same fields and shape as
// StringOption; see its doc comment for Dest/Default's ergonomics and
// semantics, and Flag/Long's for how a built-in's forms are affected.
type IntOption struct {
	// Name -- see StringOption.Name.
	Name string

	// Flag -- see StringOption.Flag.
	Flag string

	// Long -- see StringOption.Long.
	Long string

	// Help -- see StringOption.Help.
	Help string

	// Dest binds the parsed value directly to an int variable, e.g.
	// &cfg.Count. Optional. May be combined with Action -- see the
	// Action doc comment for the exact resolution order.
	Dest *int

	// Default -- see StringOption.Default.
	Default int

	// Env -- see StringOption.Env.
	Env string

	// Action -- see StringOption.Action.
	Action Action
}

func (o *IntOption) name() string { return optionName(o.Name, o.flag(), o.long()) }
func (o *IntOption) help() string { return o.Help }
func (o *IntOption) env() string  { return o.Env }

func (o *IntOption) set(s string) error {
	if o.Dest == nil {
		return nil
	}
	n, err := parseInt(s)
	if err != nil {
		return err
	}
	*o.Dest = n
	return nil
}

func (o *IntOption) setDefault() {
	if o.Dest != nil {
		*o.Dest = o.Default
	}
}

func (o *IntOption) flag() string   { return normalizeDashes(o.Flag, 1) }
func (o *IntOption) long() string   { return normalizeDashes(o.Long, 2) }
func (o *IntOption) action() Action { return o.Action }

// BoolOption describes a single boolean flag -- one whose mere presence
// on the command line sets it, never consuming a following token (see
// the Action doc comment). It carries the same fields StringOption does;
// the type itself is what says "this flag is boolean", replacing the
// old design's isBoolFlag(Dest) runtime check on a BoolVar-bound Dest.
type BoolOption struct {
	// Name -- see StringOption.Name.
	Name string

	// Flag -- see StringOption.Flag.
	Flag string

	// Long -- see StringOption.Long.
	Long string

	// Help -- see StringOption.Help.
	Help string

	// Dest binds the parsed value directly to a bool variable, e.g.
	// &cfg.Foo. Optional.
	Dest *bool

	// Default -- see StringOption.Default.
	Default bool

	// Env -- see StringOption.Env.
	Env string

	// Action -- see StringOption.Action.
	Action Action
}

func (o *BoolOption) name() string { return optionName(o.Name, o.flag(), o.long()) }
func (o *BoolOption) help() string { return o.Help }
func (o *BoolOption) env() string  { return o.Env }

func (o *BoolOption) set(s string) error {
	if o.Dest == nil {
		return nil
	}
	b, err := parseBool(s)
	if err != nil {
		return err
	}
	*o.Dest = b
	return nil
}

func (o *BoolOption) setDefault() {
	if o.Dest != nil {
		*o.Dest = o.Default
	}
}

func (o *BoolOption) flag() string   { return normalizeDashes(o.Flag, 1) }
func (o *BoolOption) long() string   { return normalizeDashes(o.Long, 2) }
func (o *BoolOption) action() Action { return o.Action }

// Action is a hook that runs when its Option's flag is seen on the
// command line. Parse calls it as opt.action()(c, opt, value): c is
// the Command being parsed (so an Action can call
// c.PrintHelp/c.PrintVersion/c.PrintErrorf, exactly like Parse itself
// does), opt is the matched Option itself (so an Action can refer back
// to its own declared fields by asserting it to its concrete type,
// e.g. opt.(*StringOption).Flag, or by calling the small set of
// unexported accessor methods available to code inside this package),
// and value is the same string Parse would hand (or just handed) to
// the Option's set method: the token consumed from the command line by default, or
// "true" for a flag that opts out of consuming one -- see Dest's doc
// comment and BoolOption's; an Action-only Option (no Dest at all)
// still consumes a value by default, exactly like any other flag,
// unless it is declared as a *BoolOption -- the type itself is what
// opts it out, not anything about its Dest (a BoolOption can still
// leave Dest nil and rely on Action alone).
//
// An Option may set Dest, Action, both, or neither. When both are set,
// Dest resolves first -- Action only runs if set succeeded, and
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
// options): each is a BoolOption with Dest bound to a throwaway bool
// (never consuming a token, value always "true") and Action set to a
// closure that prints and returns ErrHelp/ErrVersion. A caller-declared
// Option can set its own Action exactly the same way, with no special
// status over the built-in ones -- optionally alongside a Dest of its
// own, and opting out of consuming a value the exact same way the
// built-ins do.
type Action func(c *Command, opt Option, value string) *Error

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

// options returns a generator function that yields every Option in
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
func (c *Command) options() iter.Seq[Option] {
	return func(yield func(Option) bool) {
		for _, opt := range c.Options {
			if !yield(opt) {
				return
			}
		}

		// -h/--help and -v/--version must never consume a following
		// token -- each is declared as a *BoolOption below, which is
		// what opts it out (see the value-determination comment in
		// Parse): an Option with an Action but no Dest otherwise
		// defaults to consuming a value, and being a *BoolOption is
		// what says otherwise, built-in or not, independent of Dest.
		// Each built-in still binds Dest to a throwaway bool anyway, so
		// set has somewhere to write "true" (Dest resolves before
		// Action -- see the Action doc comment); discard itself is
		// never read, since neither built-in has any value of its own
		// to store.
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
			if !yield(&BoolOption{
				Flag: "-h", Long: "--help", Help: "Print help",
				Dest: &discard,
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

			yield(&BoolOption{
				Flag: short, Long: "--version", Help: "Print version",
				Dest: &discard,
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
// An entry derived from an Argument or Option carries only Name and
// Help -- all HelpString, its only caller, reads. Its Dest and Default
// stay on the Argument/Option itself: Parse resolves Env/Default per
// item (see fromEnv/setDefault) and does not walk this sequence.
func (c *Command) allEnvVars() iter.Seq[EnvVar] {
	return func(yield func(EnvVar) bool) {
		for _, arg := range c.Arguments {
			if arg.env() != "" {
				if !yield(EnvVar{Name: arg.env(), Help: arg.help()}) {
					return
				}
			}
		}

		for opt := range c.options() {
			if opt.env() != "" {
				if !yield(EnvVar{Name: opt.env(), Help: opt.help()}) {
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
//  3. Every Option has Flag and/or Long set.
//  4. Normalized Flag is exactly 2 characters if set.
//  5. A built-in (-h/--help, -v/--version) whose both normalized forms
//     are claimed by the user-declared Options is suppressed, not
//     yielded by options(). The version built-in also falls back to
//     -V (or to --version alone) when -v is claimed without
//     --version; see options' doc comment.
//  6. Normalized Flag and Long are each unique across the effective
//     Option set (options() -- user-declared Options plus whichever
//     built-ins survived suppression), so a partial claim on a
//     built-in's forms is rejected as a real conflict.
//  7. Every Option's name() -- Name, else Long/Flag without its
//     dashes -- is unique across that same effective set.
//  8. Every standalone EnvVar.Name is non-empty.
//  9. Every environment variable name -- from Arguments' Env, Options'
//     Env, and EnvVars -- is unique across all three.
//  10. Every standalone EnvVar's Default (if set) type-matches its
//     Dest's concrete type -- the one Default/Dest check Validate still
//     performs at runtime, now that Argument/Option's typed Dest/
//     Default fields (e.g. StringOption's *string/string pair) are
//     already kept in sync by the compiler.
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
	// c.options(), not c.Options -- so a user-declared Option that
	// collides with a synthesized built-in (a partial claim on -h or
	// --help, say) is caught here exactly like any other duplicate
	// flag. See options' doc comment for the suppression rule.
	if err := validateOptions(slices.Collect(c.options())); err != nil {
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
// duplicate Name. It is a free function (not a *Command method) so it
// stays decoupled from Command and testable on its own. Unlike the old
// design, there's no Default/Dest type check to run here -- a typed
// Argument's Default is already the same Go type as its Dest, enforced
// by the compiler (see validateDefaultType's doc comment for the one
// place that check still happens at runtime).
func validateArguments(args []Argument) error {
	seenNames := make(map[string]bool, len(args))
	for _, arg := range args {
		// Every Argument needs a display name -- it's what shows up in
		// the usage line (e.g. "[qux]") and the Arguments: help section.
		name := arg.name()
		if name == "" {
			return errors.New("cli: Argument.Name must not be empty")
		}

		// Argument names must be unique -- Parse fills positional
		// Arguments in declaration order, so a duplicate name would be
		// ambiguous in help output even though parsing itself doesn't
		// use Name as a lookup key.
		if seenNames[name] {
			return fmt.Errorf("cli: duplicate argument name %q", name)
		}
		seenNames[name] = true
	}
	return nil
}

// validateOptions checks opts for structural problems: a missing
// Flag/Long, a malformed or duplicate Flag/Long, or a duplicate name()
// (Option.Name, or the name derived from Long/Flag when Name is unset
// -- Parse keys its "set on the command line" bookkeeping by it, so it
// has to be unique). Same free-function
// shape as validateArguments, for the same reason -- and the same note
// applies: no Default/Dest type check here, since a typed Option's
// Default is already the same Go type as its Dest.
//
// Validate calls this with the effective Option set
// (slices.Collect(c.options())), not c.Options, so a user-declared
// Option colliding with a synthesized built-in is caught here exactly
// like two user-declared Options colliding. See options' doc
// comment for the suppression rule that decides which built-ins are
// even present in that effective set.
func validateOptions(opts []Option) error {
	seen := make(map[string]bool, len(opts)*2)
	seenNames := make(map[string]bool, len(opts))
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
				return fmt.Errorf("cli: Option.Flag %q must be a dash followed by exactly one character", flag)
			}
			if seen[flag] {
				return fmt.Errorf("cli: duplicate option flag form %q", flag)
			}
			seen[flag] = true
		}

		if long != "" {
			// Same reasoning as Flag above: Parse looks up an incoming
			// "--xxx" token by its normalized form, so Long must be
			// unique across Options too.
			if seen[long] {
				return fmt.Errorf("cli: duplicate option long form %q", long)
			}
			seen[long] = true
		}

		// Checked last so a duplicate Flag/Long is still reported as
		// such -- e.g. two Options sharing only a Flag would otherwise
		// also derive the same name and hit this error first.
		name := opt.name()
		if seenNames[name] {
			return fmt.Errorf("cli: duplicate option name %q", name)
		}
		seenNames[name] = true
	}
	return nil
}

// validateEnvVars checks that every standalone EnvVar.Name is non-empty
// (mirroring the Argument.Name check in validateArguments), that no
// environment variable name is declared more than once across
// Arguments' Env, Options' Env, and EnvVars, and that each standalone
// EnvVar's Default (if set) type-matches its Dest -- unlike Argument/
// Option, EnvVar's Dest/Default pair is still untyped (Value/any), so
// this is the one place that check still runs at runtime; see
// validateDefaultType's doc comment. A duplicate name would be
// genuinely ambiguous -- two entries claiming the same environment
// variable -- and would render as a literal duplicate row in the
// generated "Environment Variables:" help section.
//
// Unlike validateArguments/validateOptions, this is a *Command method
// rather than a free function: the name-uniqueness check is inherently
// cross-slice (it has to see Arguments, Options, and EnvVars together),
// so there's no single []T it could take instead.
func (c *Command) validateEnvVars() error {
	seen := make(map[string]bool)

	for _, arg := range c.Arguments {
		env := arg.env()
		if env == "" {
			continue
		}
		if seen[env] {
			return fmt.Errorf("cli: duplicate environment variable %q", env)
		}
		seen[env] = true
	}

	for _, opt := range c.Options {
		env := opt.env()
		if env == "" {
			continue
		}
		if seen[env] {
			return fmt.Errorf("cli: duplicate environment variable %q", env)
		}
		seen[env] = true
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
// IntVar -> int). Only a standalone EnvVar still needs this check --
// its Dest/Default pair is untyped (Value/any), unlike Argument/
// Option's typed Dest/Default fields (e.g. StringOption's *string/
// string), which the compiler already keeps in sync. Custom Value
// implementations -- ones not created by cli4go's own constructors --
// are not checked here, since cli4go has no way to know what type they
// expect.
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
// A long-form Option's value can be attached directly to its own
// token -- "--bar=value" is equivalent to "--bar" "value" -- for both
// a value-taking Option and a boolean one (where it overrides the
// bare-presence "true", e.g. "--verbose=false"). This form is
// long-only: a short-form token like "-b=value" is not split and is
// looked up as-is, so it fails as an unknown option unless something
// is literally flagged "-b=value".
//
// Parse returns:
//   - nil: parsing succeeded.
//   - ErrHelp / ErrVersion (ExitCode() == 0): a built-in action fired;
//     PrintHelp/PrintVersion already wrote the full text to c.Stdout.
//     errors.Is(err, ErrExit) is also true for both; use errors.Is(err,
//     ErrHelp) / errors.Is(err, ErrVersion) to tell which one fired, if
//     that matters.
//   - a code-1 failure (ExitCode() == 1): the Command itself was
//     declared incorrectly -- from Validate() or from a Default that
//     can't be assigned (see resolveEnvVar's doc comment). Nothing is
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

	// Lookup map built once so each token below resolves to its Option
	// in O(1). Built from options(), not c.Options, so the built-in
	// -h/--help and -v/--version go through the same lookup and
	// dispatch as any other Option. A normalized Flag and Long never
	// collide (Flag is always 2 chars, Long always starts with "--"),
	// so one map safely holds both -- an Option with both set just
	// gets two keys pointing at it.
	opts := make(map[string]Option, (len(c.Options)+2)*2)
	for opt := range c.options() {
		if flag := opt.flag(); flag != "" {
			opts[flag] = opt
		}
		if long := opt.long(); long != "" {
			opts[long] = opt
		}
	}

	// Which Arguments/Options the command line itself set, by name():
	// the resolution phase after the loop only touches the rest.
	seenArgs := make(map[string]bool, len(c.Arguments))
	seenOpts := make(map[string]bool, len(c.Options))

	// Classic index loop, not range: consuming a value-taking Option's
	// value requires advancing i an extra position mid-iteration.
	argIndex := 0
	for i := 0; i < len(args); i++ {
		tok := args[i]

		switch {
		case strings.HasPrefix(tok, "-"):
			// "--bar=value" splits off its value before the lookup below;
			// only long ("--") tokens split this way. Cut on the first
			// "=" only, so a value containing "=" keeps the rest intact.
			key := tok
			var value string
			hasValue := false
			if strings.HasPrefix(tok, "--") {
				if k, v, ok := strings.Cut(tok, "="); ok {
					key, value, hasValue = k, v, true
				}
			}

			opt, ok := opts[key]
			if !ok {
				return c.PrintErrorf("unknown option %q", tok)
			}
			seenOpts[opt.name()] = true

			// value is what set and Action both receive. hasValue
			// (an attached "=value") wins over the *BoolOption check
			// below, so "--flag=false" can override a boolean Dest
			// explicitly. Otherwise the next token is consumed as the
			// value, unless opt is a *BoolOption -- its type alone marks
			// it as taking no token (see BoolOption's doc comment),
			// giving value "true" instead.
			if !hasValue {
				value = "true"
				if _, ok := opt.(*BoolOption); !ok {
					i++
					if i >= len(args) {
						return c.PrintErrorf("option %q requires a value", tok)
					}
					value = args[i]
				}
			}

			// Dest resolves first, if set: Action (below) receives the
			// exact same value, and only runs at all if set succeeded.
			if setErr := opt.set(value); setErr != nil {
				return c.PrintErrorf("option %q: %w", tok, setErr)
			}

			// See the Action doc comment for its nil/non-nil contract.
			if action := opt.action(); action != nil {
				if err := action(c, opt, value); err != nil {
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
			seenArgs[arg.name()] = true

			if setErr := arg.set(tok); setErr != nil {
				return c.PrintErrorf("argument %q: %w", tok, setErr)
			}
		}
	}

	// Everything the command line did not set falls back to its Env
	// variable, else its Default -- Arguments, then Options, then
	// standalone EnvVars, each in declaration order. Nothing above wrote
	// a Default or Env value, so a failure in the loop leaves every
	// untouched Dest exactly as the caller had it.
	for _, arg := range c.Arguments {
		if !seenArgs[arg.name()] {
			found, err := c.fromEnv(arg.env(), arg.set)
			if err != nil {
				return err
			}
			if !found {
				arg.setDefault()
			}
		}
	}
	for _, opt := range c.Options {
		if !seenOpts[opt.name()] {
			found, err := c.fromEnv(opt.env(), opt.set)
			if err != nil {
				return err
			}
			if !found {
				opt.setDefault()
			}
		}
	}
	for _, env := range c.EnvVars {
		if err := c.resolveEnvVar(env.Dest, env.Name, env.Default); err != nil {
			return err
		}
	}

	return nil
}

// fromEnv hands the value of the environment variable name to set, when
// name is non-empty and the variable is present, reporting whether it
// was present. An empty or absent name just reports false.
//
// A bad Env value is a real parsing/usage error (ExitCode() == 2),
// already printed via PrintErrorf by the time this returns it, so
// Parse can return it as-is.
func (c *Command) fromEnv(name string, set func(string) error) (bool, *Error) {
	if name == "" {
		return false, nil
	}

	val, ok := os.LookupEnv(name)
	if !ok {
		return false, nil
	}

	if err := set(val); err != nil {
		return false, c.PrintErrorf("invalid value %q for environment variable %q: %w", val, name, err)
	}
	return true, nil
}

// resolveEnvVar assigns a standalone EnvVar's Dest its fallback value:
// the environment variable envName when it is present (see fromEnv),
// otherwise def. Exactly one of the two is applied, so dest is written
// at most once. Unlike an Argument/Option -- whose typed Default is
// assigned directly by setDefault -- an EnvVar's Default is still an
// untyped any, so it goes through dest.Set(fmt.Sprint(def)).
//
// A Default that can't be assigned is a different kind of problem from
// a bad Env value: it can only come from a standalone EnvVar with a
// mismatched Dest/Default -- caught first by validateDefaultType/
// Validate -- or a custom Value implementation Validate deliberately
// can't check (see validateDefaultType's doc comment). That is the same
// kind of problem Validate() itself reports -- the Command was declared
// incorrectly, not something the end user typed -- so, like Validate(),
// it reports ExitCode() == 1 and prints nothing.
func (c *Command) resolveEnvVar(dest Value, envName string, def any) *Error {
	if dest == nil {
		return nil
	}

	if found, err := c.fromEnv(envName, dest.Set); err != nil || found {
		return err
	}

	if def == nil {
		return nil
	}
	if err := dest.Set(fmt.Sprint(def)); err != nil {
		return newError(1, "%s", fmt.Errorf("cli: invalid default value %v: %w", def, err))
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
		fmt.Fprintf(&args, " [%s]", arg.name())
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
// claim both of its forms -- see options' doc comment), plus
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
		w := len(arg.name())
		if w > widthArgs {
			widthArgs = w
		}
	}
	widthOpts := 0
	for opt := range c.options() {
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
			fmt.Fprintf(&b, "  %-*s   %s\n", width, arg.name(), arg.help())
		}
	}

	// options() yields the user-declared Options, then the built-in -h/--help
	// and -v/--version entries (if Version is set). The latter two are always
	// listed last, so they don't get lost in the middle of a long list of user
	// flags. A built-in is omitted only when the user claims both of its
	// forms (see options' doc comment), in which case a user-declared
	// Option takes its place, so the Options section is never empty.
	fmt.Fprintf(&b, "\nOptions:\n")
	for opt := range c.options() {
		flag := opt.flag()
		long := opt.long()
		if flag != "" && long != "" {
			flag += ","
		}
		fmt.Fprintf(&b,
			"  %-3s %-*s   %s\n",
			flag, width-4, opt.long(), opt.help(), // 4 = len("-x, ")
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
