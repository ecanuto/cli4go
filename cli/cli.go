// Package cli provides a declarative toolkit for building command-line
// interfaces in Go, combining an argparse-like API with clap-style
// formatted help output.
package cli

import (
	"fmt"
	"io"
	"iter"
	"os"

	"slices"
	"strings"
)

// item is what Argument, Option, and EnvVar have in common: something
// declared on a Command that can receive a value -- from the command
// line, from an environment variable, or from its Default -- and be
// checked for structural problems. Parse and Validate treat every item
// alike through this interface (see items); what sets each family apart
// (positional order, Flag/Long, Action) lives on Argument and Option
// themselves.
type item interface {
	name() string
	help() string

	// env returns the name of the environment variable this item falls
	// back to when the command line does not set it, or "" if none. An
	// EnvVar's env is its own name.
	env() string

	// validate reports a structural problem with this item on its own,
	// independent of any other item -- an empty Name, a missing
	// Flag/Long, ... Cross-item rules (unique names, flags, and
	// environment variable names) stay in the Command-level checks.
	validate() error

	// set parses s into Dest, the token Parse (or an Env lookup) just
	// resolved for this item. It returns whatever conversion error the
	// concrete type produces (none for a string), and does nothing --
	// returning nil -- when Dest is unset.
	set(s string) error

	// setDefault assigns Default to Dest directly, when Dest is set --
	// no string round trip, and it can never fail, since Default and
	// Dest are the same Go type. The zero value applies too, like any
	// other Default (see StringArgument's Default doc comment).
	setDefault()
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
	// Variables:" help section, which lists Arguments' Env entries
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

// Values for items' scope parameter.
const (
	declaredOnly = iota // Arguments, Options, and EnvVars as declared
	withBuiltins        // ... plus the built-in Options options() synthesizes
)

// items yields every Argument, then every Option, then every EnvVar,
// each in declaration order -- the three families as one sequence of
// item. With withBuiltins the Options are the effective set (options()),
// the built-ins last; with declaredOnly, just the user-declared ones.
func (c *Command) items(scope int) iter.Seq[item] {
	return func(yield func(item) bool) {
		for _, arg := range c.Arguments {
			if !yield(arg) {
				return
			}
		}

		var opts iter.Seq[Option]
		if scope == withBuiltins {
			opts = c.options()
		} else {
			opts = slices.Values(c.Options)
		}

		for opt := range opts {
			if !yield(opt) {
				return
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
//  2. Normalized Flag and Long unique across the effective Option set
//     (options(): the user-declared Options plus the built-ins that
//     survive suppression), so a partial claim on a built-in's forms is
//     a conflict; see options' doc comment.
//  3. For every item -- Arguments, effective Options, then EnvVars --
//     its own structure: Argument.Name and EnvVar.Name non-empty, an
//     Option with a Flag and/or Long, a Flag exactly 2 characters.
//  4. Environment variable names (Arguments' Env, Options' Env, and
//     EnvVars) unique across all three families, and every item's
//     name() (an Option's defaults to Long/Flag without its dashes)
//     unique across all of them, since a name identifies one item.
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
	// Command.Name is required: usage, help, and version output use it.
	if c.Name == "" {
		return newError(1, "cli: Command.Name must not be empty")
	}

	// Flag/Long uniqueness runs over the effective Option set, so a
	// user-declared Option colliding with a built-in is caught like any
	// other duplicate. It runs first so a shared Flag is reported as
	// such, not as the duplicate name the two Options also derive.
	if err := validateOptions(slices.Collect(c.options())); err != nil {
		return newError(1, "%s", err)
	}

	// One pass over every item, built-ins included: its own structure,
	// then the uniqueness rules that cross families. The environment
	// variable is checked first so two EnvVars sharing a name (also
	// their variable) report as a duplicate environment variable.
	names := make(map[string]bool)
	envs := make(map[string]bool)
	for it := range c.items(withBuiltins) {
		if err := it.validate(); err != nil {
			return newError(1, "%s", err)
		}

		if env := it.env(); env != "" {
			if envs[env] {
				return newError(1, "cli: duplicate environment variable %q", env)
			}
			envs[env] = true
		}

		// A name identifies exactly one item, so it can be looked up
		// unambiguously. An Option's derived name can collide with an
		// Argument's; an explicit Name settles it.
		name := it.name()
		if names[name] {
			return newError(1, "cli: duplicate name %q; set an explicit Name on one of them", name)
		}
		names[name] = true
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
//     declared incorrectly, as reported by Validate(). Nothing is
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

	// Which Arguments/Options the command line itself set, by identity:
	// the resolution phase after the loop only touches the rest.
	seen := make(map[item]bool, len(c.Arguments)+len(c.Options))

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
			seen[opt] = true

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
			seen[arg] = true

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
	for it := range c.items(declaredOnly) {
		if seen[it] {
			continue
		}

		// Env wins over Default, and exactly one of the two is applied,
		// so each Dest is written at most once. A bad Env value is a
		// real parsing/usage error (ExitCode() == 2), printed by
		// PrintErrorf before it is returned.
		if name := it.env(); name != "" {
			if val, ok := os.LookupEnv(name); ok {
				if err := it.set(val); err != nil {
					return c.PrintErrorf("invalid value %q for environment variable %q: %w", val, name, err)
				}
				continue
			}
		}
		it.setDefault()
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
	for it := range c.items(declaredOnly) {
		w := len(it.env())
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
			flag, width-4, long, opt.help(), // 4 = len("-x, ")
		)
	}

	// Environment Variables section, only render it if we actually found at least
	// one environment variable
	if widthEnvs > 0 {
		fmt.Fprintf(&b, "\nEnvironment Variables:\n")
		for it := range c.items(declaredOnly) {
			if it.env() == "" {
				continue
			}
			fmt.Fprintf(&b, "  %-*s   %s\n", width, it.env(), it.help())
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
