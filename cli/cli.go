// Package cli provides a declarative toolkit for building command-line
// interfaces in Go, combining an argparse-like API with clap-style
// formatted help output.
package cli

import (
	"fmt"
	"io"
	"iter"
	"os"
	"reflect"

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
//
// item is not part of the public API: Argument, Option, and EnvVar each
// declare these methods and document their contracts, and each satisfies
// item structurally, so the compiler keeps the declarations in sync.
type item interface {
	GetName() string
	GetHelp() string
	GetEnv() string
	GetDefault() any
	GetRequired() bool
	GetValue() any
	Set(s string) error
	SetDefault() error
	Validate() error
}

// Command describes a complete command-line interface: its identity
// (Name/Version/Description), its positional Arguments, its Options
// (flags), and its standalone EnvVars. A Command is plain data — build
// it as a struct literal, then call Parse to read os.Args into the
// bound variables.
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

	// Commands are the program's subcommands. The first non-dash token
	// selects one by Name and the tokens after it are parsed by that
	// Command alone, so each declares its own items. Options before it
	// belong to this Command. Not allowed together with Arguments.
	Commands []*Command

	// Action, if set, runs at the end of a successful Parse when this
	// Command is the one selected: a subcommand, or a Command without
	// Commands. A non-nil *Error it returns is what Parse returns.
	Action CommandAction

	// Stdout is where PrintUsage, PrintHelp, PrintVersion, and standalone
	// help/version output write to. Defaults to os.Stdout when nil.
	Stdout io.Writer

	// Stderr is where Parse's clap-style error block (a real parsing
	// error) writes to, always, independent of Stdout. Defaults to
	// os.Stderr when nil.
	Stderr io.Writer

	parent *Command
}

// Program is an alias for Command.
type Program = Command

// CommandAction runs when its Command is selected and parsed: c is that
// Command, with every item resolved.
type CommandAction func(c *Command) *Error

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

// options yields every Option of the Command: the user-declared ones
// first, then the built-in -h/--help and, when Version is set,
// -v/--version.
//
// A built-in is suppressed -- not yielded at all -- when the
// user-declared Options claim both of its normalized forms, on one
// Option or on two. A partial claim (only -h, or only --help) does not
// suppress it: it collides, and Validate rejects the duplicate.
//
// The version built-in has one extra rule: when -v is claimed but not
// --version, it is yielded with -V as its short form instead (-v is
// commonly "verbose", and -v/-V is an established convention); if -V is
// claimed too, only --version is yielded. Help has no such fallback: -H
// conventionally means "host", so the help built-in is either -h/--help
// or suppressed.
func (c *Command) options() iter.Seq[Option] {
	return func(yield func(Option) bool) {
		for _, opt := range c.Options {
			if !yield(opt) {
				return
			}
		}

		// Each built-in is a *BoolOption, which is what keeps it from
		// consuming a following token (see Parse's value
		// determination), with a throwaway Dest so set has somewhere to
		// write "true" before its Action runs. discard is never read.
		var discard bool

		// The normalized Flag/Long forms the user-declared Options
		// claim, whichever Option claims them and whatever else it
		// declares.
		takenFlags := make(map[string]bool, len(c.Options))
		takenLongs := make(map[string]bool, len(c.Options))
		for _, opt := range c.Options {
			if f := opt.GetFlag(); f != "" {
				takenFlags[f] = true
			}
			if l := opt.GetLong(); l != "" {
				takenLongs[l] = true
			}
		}

		// Suppress the help built-in only when both of its forms are
		// claimed; a partial claim leaves it intact, and Validate
		// reports the duplicate.
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
			// -v is often taken by "verbose", so a claimed -v falls back
			// to -V; if -V is taken too the short form is dropped, and if
			// --version is claimed as well the built-in is suppressed, as
			// for help.
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
//  2. For every item -- Arguments, the effective Options (options():
//     the user-declared ones plus the built-ins that survive
//     suppression), then EnvVars -- its own structure: Argument.Name
//     and EnvVar.Name non-empty, an Option with a Flag and/or Long, a
//     Flag exactly 2 characters.
//  3. Uniqueness across all of those items: environment variable names
//     (Arguments' Env, Options' Env, and EnvVars); normalized Flag and
//     Long forms, so a partial claim on a built-in's forms is a real
//     conflict (see options' doc comment); and every item's GetName() (an
//     Option's defaults to Long/Flag without its dashes), since a name
//     identifies one item.
//
// Required and a non-zero Default are not checked against each other --
// see Required's doc comment.
//
// Dest and Action are not checked against each other -- an Option may
// set neither, either, or both (see Option.Action's doc comment for
// what setting both means for Parse's dispatch order).
//
// A Validate failure never prints anything (it is a bug in the caller's
// Go code, not something an end user mistyped) and always reports
// ExitCode() == 1, so a caller using Validate on its own gets the same
// os.Exit(err.ExitCode()) idiom as Parse/DefaultParse.
func (c *Command) Validate() *Error {
	// Command.Name is required: usage, help, and version output use it.
	if c.Name == "" {
		return Errorf(1, "cli: Command.Name must not be empty")
	}

	if len(c.Commands) > 0 && len(c.Arguments) > 0 {
		return Errorf(1, "cli: Command %q declares both Arguments and Commands", c.Name)
	}
	cmds := make(map[string]bool, len(c.Commands))
	for _, sub := range c.Commands {
		if err := sub.Validate(); err != nil {
			return err
		}
		if cmds[sub.Name] {
			return Errorf(1, "cli: duplicate subcommand %q", sub.Name)
		}
		cmds[sub.Name] = true
	}

	// One pass over every item, built-ins included: its own structure,
	// then the uniqueness rules, from the most to the least specific
	// claim. That order is what reports two EnvVars sharing a name (also
	// their variable) as a duplicate environment variable, and two
	// Options sharing only a Flag as a duplicate Flag rather than as the
	// duplicate name they also derive.
	envs := make(map[string]bool)
	forms := make(map[string]bool)
	names := make(map[string]bool)
	for it := range c.items(withBuiltins) {
		if err := it.Validate(); err != nil {
			return Errorf(1, "%s", err)
		}

		if env := it.GetEnv(); env != "" {
			if envs[env] {
				return Errorf(1, "cli: duplicate environment variable %q", env)
			}
			envs[env] = true
		}

		// Only an Option has Flag/Long forms. A user-declared Option
		// claiming just one of a built-in's forms collides with it here
		// like any other duplicate. A normalized Flag and Long never
		// collide with each other (a Flag is 2 characters, a Long starts
		// with "--"), so one map holds both.
		if opt, ok := it.(Option); ok {
			if flag := opt.GetFlag(); flag != "" {
				if forms[flag] {
					return Errorf(1, "cli: duplicate option flag form %q", flag)
				}
				forms[flag] = true
			}

			if long := opt.GetLong(); long != "" {
				if forms[long] {
					return Errorf(1, "cli: duplicate option long form %q", long)
				}
				forms[long] = true
			}
		}

		// A name identifies exactly one item, so it can be looked up
		// unambiguously. An Option's derived name can collide with an
		// Argument's; an explicit Name settles it.
		name := it.GetName()
		if names[name] {
			return Errorf(1, "cli: duplicate name %q; set an explicit Name on one of them", name)
		}
		names[name] = true
	}

	return nil
}

// --- Parse -------------------------------------------------------------------

// Parse resolves every Argument, Option, and EnvVar from args (typically
// os.Args[1:]) and the environment, applying the Default -> Env ->
// command-line-flag precedence. It also handles the built-in -h/--help flag (always
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
// An Option can be given only once on the command line, through either
// its Flag or its Long form: a repeat is a usage error, and the first
// occurrence's Dest and Action have already run by then.
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
//   - a real parsing/usage error (ExitCode() == 2): an unknown, invalid,
//     repeated, or missing-but-Required option/argument, including a
//     bad Env value, or whatever *Error a caller-declared Option's
//     Action chose to return. Parse prints a clap-style block to
//     c.Stderr before returning (see below) for everything except an
//     Action's own *Error, which Parse returns exactly as given -- an
//     Action that wants that same clap-style block builds it itself via
//     c.UsageErrorf.
//
// For a real parsing/usage error Parse builds itself, Parse prints (via
// UsageErrorf -- see its doc comment):
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
		if flag := opt.GetFlag(); flag != "" {
			opts[flag] = opt
		}
		if long := opt.GetLong(); long != "" {
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
				return c.UsageErrorf("unknown option %q", tok)
			}

			// An Option may appear at most once, whichever of its
			// forms (Flag or Long) each occurrence uses; the error
			// echoes the token that repeated it.
			if seen[opt] {
				return c.UsageErrorf("option %q cannot be used multiple times", tok)
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
						return c.UsageErrorf("option %q requires a value", tok)
					}
					value = args[i]
				}
			}

			// Dest resolves first, if set: Action (below) receives the
			// exact same value, and only runs at all if set succeeded.
			if setErr := opt.Set(value); setErr != nil {
				return c.UsageErrorf("option %q: %w", tok, setErr)
			}

			// See OptionAction for its nil/non-nil contract.
			if action := opt.GetAction(); action != nil {
				if err := action(c, opt, value); err != nil {
					return err
				}
			}

		default:
			// With Commands, this token names the subcommand: resolve this
			// Command's items, then hand over the rest.
			if len(c.Commands) > 0 {
				idx := slices.IndexFunc(c.Commands, func(s *Command) bool { return s.Name == tok })
				if idx < 0 {
					return c.UsageErrorf("unknown command %q", tok)
				}
				if err := c.resolve(seen); err != nil {
					return err
				}
				c.Commands[idx].parent = c
				return c.Commands[idx].Parse(args[i+1:])
			}

			// A non-dash token fills the next declared Argument, in
			// declaration order.
			if argIndex >= len(c.Arguments) {
				return c.UsageErrorf("unexpected argument %q", tok)
			}
			arg := c.Arguments[argIndex]
			argIndex++
			seen[arg] = true

			if setErr := arg.Set(tok); setErr != nil {
				return c.UsageErrorf("argument %q: %w", tok, setErr)
			}
		}
	}

	if err := c.resolve(seen); err != nil {
		return err
	}

	if len(c.Commands) > 0 {
		return c.UsageErrorf("a command is required")
	}

	if c.Action != nil {
		return c.Action(c)
	}
	return nil
}

// resolve gives everything the command line did not set (not in seen)
// its Env variable, else its Default -- Arguments, then Options, then
// standalone EnvVars, each in declaration order. Nothing before it wrote
// a Default or Env value, so a failure leaves every untouched Dest
// exactly as the caller had it.
func (c *Command) resolve(seen map[item]bool) *Error {
	for it := range c.items(declaredOnly) {
		if seen[it] {
			continue
		}

		// Env wins over Default, and exactly one of the two is applied,
		// so each Dest is written at most once. A bad Env value is a
		// real parsing/usage error (ExitCode() == 2), printed by
		// UsageErrorf before it is returned.
		if name := it.GetEnv(); name != "" {
			if val, ok := os.LookupEnv(name); ok {
				if err := it.Set(val); err != nil {
					return c.UsageErrorf("invalid value %q for environment variable %q: %w", val, name, err)
				}
				continue
			}
		}

		// Neither the command line nor the environment supplied a value.
		if it.GetRequired() {
			return c.UsageErrorf("%q is required", it.GetName())
		}
		if err := it.SetDefault(); err != nil {
			return c.UsageErrorf("default value for %q: %w", it.GetName(), err)
		}
	}
	return nil
}

// Lookup returns the value of the Argument, Option, or EnvVar whose
// GetName is name, as a plain value (a string, an int, ...), and whether
// such an item is declared. The value is the one Parse resolved: from the
// command line, else from the environment, else Default; before Parse it
// is the zero value of the item's type. It works with or without Dest.
// The built-in -h/--help and -v/--version Options are not declared by the
// caller and are not found.
func (c *Command) Lookup(name string) (any, bool) {
	for it := range c.items(declaredOnly) {
		if it.GetName() == name {
			return it.GetValue(), true
		}
	}
	return nil, false
}

// DefaultParse is a convenience wrapper around Parse that reads
// os.Args[1:] -- the common case for a program's main(). Use Parse
// directly (with an explicit slice) when testing, or when args come
// from somewhere other than the running process's own command line.
func (c *Command) DefaultParse() *Error {
	return c.Parse(os.Args[1:])
}

// --- Error output, Usage, Help, Version --------------------------------------

// usageErrorString returns the clap-style error block for a real
// parsing/usage error: the "error: <message>" line, the usage line (see
// UsageString), and a pointer to '--help'. Parse uses it for every real
// parsing/usage error (ExitCode() == 2); see the Parse doc comment for
// the exact layout.
//
// err is the bare underlying error (e.g. fmt.Errorf("unknown option
// %q", tok)). The "error: " prefix is display-only: the message stored
// in the *Error Parse returns is never prefixed with it, so
// errors.Is/errors.As and any caller-side wrapping keep working against
// the bare message.
func (c *Command) usageErrorString(err error) string {
	var b strings.Builder
	fmt.Fprintf(&b, "error: %v\n", err)
	b.WriteString(c.UsageString())
	b.WriteString("\nFor more information, try '--help'.\n")
	return b.String()
}

// UsageErrorf formats err per format/args -- exactly like fmt.Errorf,
// including %w to wrap an underlying error -- writes usageErrorString of the
// result to c.Stderr (os.Stderr by default), and returns the *Error the
// caller should return in turn. Unlike PrintUsage/PrintHelp/
// PrintVersion, which write to Stdout and return nothing, it writes to
// Stderr and returns a value: formatting the error, printing the block,
// and building the *Error are the same step at every call site, and
// every block this package prints is a real parsing/usage error, so the
// exit code is always 2 (see Error.ExitCode's doc comment).
//
// The "f" suffix is what lets `go vet`'s printf checker recognize
// format/args here and catch a mismatched verb.
//
// A caller holding a plain error can pass it through with "%w", err, which
// keeps it reachable through errors.Is and errors.As on the returned Error. A
// user-declared Option's Action is such a caller; see OptionAction.
func (c *Command) UsageErrorf(format string, args ...any) *Error {
	err := fmt.Errorf(format, args...)
	fmt.Fprint(c.stderr(), c.usageErrorString(err))
	return Errorf(2, "%w", err)
}

// UsageString returns the one-line usage summary, e.g.
// "Usage: myprogram [options] [qux]\n" -- the same line HelpString shows
// first, and the line Parse prints as part of its error block for a real
// parsing error (see the Parse doc comment). For the full help text --
// with the Arguments / Options / Environment Variables sections -- see
// HelpString.
func (c *Command) UsageString() string {
	// Build the "arg1 [arg2] ..." part of the usage line from the
	// Command's Arguments: an optional Argument is bracketed, a Required
	// one is not. Options are always represented by the "[options]"
	// placeholder, so they don't need to be listed here.
	var args strings.Builder
	for _, arg := range c.Arguments {
		if arg.GetRequired() {
			fmt.Fprintf(&args, " %s", arg.GetName())
		} else {
			fmt.Fprintf(&args, " [%s]", arg.GetName())
		}
	}

	// The usage line is always "Usage: <program name> [options] arg1 [arg2] ..."
	if len(c.Commands) > 0 {
		args.WriteString(" <command> [args]")
	}

	// Full name from the root down, e.g. "couch setup env".
	path := c.Name
	for p := c.parent; p != nil; p = p.parent {
		path = p.Name + " " + path
	}
	return fmt.Sprintf("Usage: %s [options]%s\n", path, args.String())
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
// Arguments is omitted entirely when there are no Arguments, and
// Environment Variables when nothing declares an environment variable
// (no Argument or Option Env, and no EnvVars). Options is never omitted:
// it always contains at least the synthesized -h/--help entry (unless
// the user-declared Options claim both of its forms -- see options' doc
// comment), plus -v/--version when Version is set, both listed last.
func (c *Command) HelpString() string {
	var b strings.Builder

	// Compute the shared column width from the longest Argument.Name, the
	// longest Option label (its long form plus its value placeholder, e.g.
	// "--bar <BAR>") plus 4 characters for the "-x, " short flag column an
	// Option row has but an Argument row doesn't, and the longest
	// environment variable name. Each is computed on its own and combined
	// below.
	var widthArgs, widthOpts, widthEnvs int
	for _, arg := range c.Arguments {
		if w := len(arg.GetName()); w > widthArgs {
			widthArgs = w
		}
	}
	for _, sub := range c.Commands {
		widthArgs = max(widthArgs, len(sub.Name))
	}
	for opt := range c.options() {
		if w := len(optionLabel(opt)) + 4; w > widthOpts { // 4 = len("-x, ")
			widthOpts = w
		}
	}
	for it := range c.items(declaredOnly) {
		if w := len(it.GetEnv()); w > widthEnvs {
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
			fmt.Fprintf(&b, "  %-*s   %s\n", width, arg.GetName(), describe(arg))
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
		flag := opt.GetFlag()
		if flag != "" && opt.GetLong() != "" {
			flag += ","
		}
		fmt.Fprintf(&b,
			"  %-3s %-*s   %s\n",
			flag, width-4, optionLabel(opt), describe(opt), // 4 = len("-x, ")
		)
	}

	if len(c.Commands) > 0 {
		fmt.Fprintf(&b, "\nCommands:\n")
		for _, sub := range c.Commands {
			fmt.Fprintf(&b, "  %-*s   %s\n", width, sub.Name, sub.Description)
		}
	}

	// Environment Variables section, only render it if we actually found at least
	// one environment variable
	if widthEnvs > 0 {
		fmt.Fprintf(&b, "\nEnvironment Variables:\n")
		for it := range c.items(declaredOnly) {
			if it.GetEnv() == "" {
				continue
			}
			fmt.Fprintf(&b, "  %-*s   %s\n", width, it.GetEnv(), it.GetHelp())
		}
	}

	return b.String()
}

// optionLabel returns what the help shows for an Option after its short
// form: the long form followed by a value placeholder -- the upper-cased
// GetName() in angle brackets, e.g. "--bar <BAR>" -- unless the Option is a
// *BoolOption, which takes no value. An Option with no long form shows the
// placeholder alone.
func optionLabel(opt Option) string {
	label := opt.GetLong()
	if _, isBool := opt.(*BoolOption); isBool {
		return label
	}
	if label != "" {
		label += " "
	}
	return label + "<" + strings.ToUpper(opt.GetName()) + ">"
}

// describe returns the description the help shows for an item: its Help,
// then "[env: NAME]" when it falls back to an environment variable, then
// "[default: VALUE]" when it has a non-zero Default. A Required item shows
// no default, since it would never apply.
func describe(it item) string {
	var parts []string
	if help := it.GetHelp(); help != "" {
		parts = append(parts, help)
	}
	if env := it.GetEnv(); env != "" {
		parts = append(parts, "[env: "+env+"]")
	}
	if def := it.GetDefault(); !it.GetRequired() && def != nil && !reflect.ValueOf(def).IsZero() {
		parts = append(parts, fmt.Sprintf("[default: %#v]", def))
	}
	return strings.Join(parts, " ")
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
