package cli

import (
	"errors"
	"fmt"
	"strings"
)

// --- Option ------------------------------------------------------------------

// Option describes a single flag (e.g. "-f, --foo"). Unlike Argument, the
// order of Options in Command.Options has no effect on parsing: flags can
// appear in any order on the command line.
//
// Option is implemented by the generic OptionOf, through the aliases
// StringOption, IntOption, and BoolOption, each instantiation owning its
// Dest and Default as its own Go type; see Argument's doc comment. The
// interface's accessors are exported (GetFlag, GetLong, ...) so a value
// held only as Option (e.g. from Command.Options) can still be read
// without a type assertion to its concrete struct; each concrete type
// also keeps its fields public for struct-literal construction. The Get prefix is what keeps field
// and method from colliding -- a plain Flag() would clash with the Flag
// field.
type Option interface {
	// GetName returns the identifier of this Option: its Name when set,
	// else its long form, else its flag form, without dashes.
	GetName() string

	// GetFlag returns the short form with exactly one leading dash (e.g.
	// "-f"), or "" if the Option has none.
	GetFlag() string

	// GetLong returns the long form with exactly two leading dashes (e.g.
	// "--foo"), or "" if the Option has none.
	GetLong() string

	// GetHelp returns the description shown in the help output.
	GetHelp() string

	// GetEnv returns the name of the environment variable this Option
	// falls back to when the command line does not set it, or "" if none.
	GetEnv() string

	// GetDefault returns Default as a plain value (a string, an int, ...),
	// which is the zero value of its type when none was declared.
	GetDefault() any

	// GetAction returns the hook that runs when this Option is seen on
	// the command line, or nil if none (see Action).
	GetAction() Action

	// GetRequired reports whether Parse fails with a usage error when
	// neither the command line nor GetEnv's variable sets this Option.
	GetRequired() bool

	// Set parses s into Dest, the token Parse (or an Env lookup) just
	// resolved for this Option. It returns whatever conversion error the
	// concrete type produces, and does nothing -- returning nil -- when
	// Dest is unset.
	Set(s string) error

	// SetDefault assigns Default to Dest directly, when Dest is set -- no
	// string round trip, and it can never fail, since Default and Dest are
	// the same Go type. The zero value applies too, like any other Default.
	SetDefault()

	// Validate reports a structural problem with this Option on its own,
	// independent of any other item -- a missing Flag/Long, for instance.
	// Cross-item rules (unique names, forms, and environment variable
	// names) stay in Command.Validate.
	Validate() error
}

// normalizeDashes returns s with exactly dashes leading dashes, whatever
// it had before: the rule behind both a Flag ("f", "-f", "--f" -> "-f")
// and a Long ("bar", "-bar", "---bar" -> "--bar"). A single dash on a
// Long is a likely typo, so it is normalized rather than rejected. An
// empty s stays empty.
func normalizeDashes(s string, dashes int) string {
	if s == "" {
		return s
	}
	return strings.Repeat("-", dashes) + strings.TrimLeft(s, "-")
}

// optionName returns the identifier of an Option: name when set,
// otherwise long without its "--", otherwise flag without its "-". flag
// and long are expected normalized (see normalizeDashes); Validate guarantees
// one of them is set, so a validated Option's name is never empty.
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

// --- OptionOf ----------------------------------------------------------------

// OptionOf describes a single flag whose value is a T, read by the
// Parser P. Use it through the aliases for the common types,
// StringOption, IntOption, and BoolOption; see ArgumentOf for adding one
// of your own.
type OptionOf[T any, P Parser[T]] struct {
	// Name is an optional identifier, decoupled from Flag/Long. It is
	// the Option's GetName(); when unset, that is Long, else Flag, without
	// its dashes. Validate requires it to be unique across every
	// Argument, Option, and EnvVar.
	Name string

	// Flag is the short form, e.g. "f" or "-f" (the dash is added when
	// omitted). Optional.
	//
	// Claiming both forms of a built-in (-h and --help, or -v and
	// --version), on one Option or several, suppresses that built-in
	// from the help and from Parse's dispatch; claiming only one form is
	// a conflict, rejected by Validate. The exception is -v alone, which
	// makes the version built-in use -V. See options' doc comment.
	Flag string

	// Long is the long form, e.g. "bar" or "--bar" (the "--" is added
	// when omitted). Optional, though at least one of Flag/Long must be
	// set. See Flag for how claiming a built-in's forms affects it; note
	// that "--version" without "-v" is a conflict, not a fallback.
	Long string

	// Help explains what the option does. Shown in the generated
	// "Options:" help section.
	Help string

	// Dest binds the parsed value directly to a variable, e.g.
	// &cfg.Bar. Optional. May be combined with Action -- see the Action
	// doc comment for the exact resolution order.
	Dest *T

	// Default is assigned to Dest when neither the command line nor Env
	// sets it. Its zero value applies just like any other value -- there
	// is no separate "unset" state.
	Default T

	// Env, if set, is the environment variable consulted when this
	// option is not provided on the command line.
	Env string

	// Action, if set, runs when this Option's flag is seen on the
	// command line -- see the Action doc comment for its exact contract
	// and how it combines with Dest.
	Action Action

	// Required makes Parse fail with a usage error (ExitCode() == 2)
	// when this option is resolved from neither the command line nor
	// Env. Not meant to be combined with a non-zero Default; not
	// validated, since Default would just be dead code, not a bug.
	Required bool
}

func (o *OptionOf[T, P]) GetName() string    { return optionName(o.Name, o.GetFlag(), o.GetLong()) }
func (o *OptionOf[T, P]) GetFlag() string    { return normalizeDashes(o.Flag, 1) }
func (o *OptionOf[T, P]) GetLong() string    { return normalizeDashes(o.Long, 2) }
func (o *OptionOf[T, P]) GetHelp() string    { return o.Help }
func (o *OptionOf[T, P]) GetEnv() string     { return o.Env }
func (o *OptionOf[T, P]) GetDefault() any    { return o.Default }
func (o *OptionOf[T, P]) GetAction() Action  { return o.Action }
func (o *OptionOf[T, P]) GetRequired() bool  { return o.Required }
func (o *OptionOf[T, P]) Set(s string) error { return setDest(o.Dest, s, parseWith[T, P]) }
func (o *OptionOf[T, P]) SetDefault()        { setDestDefault(o.Dest, o.Default) }

// Validate checks the normalized flag and long forms.
func (o *OptionOf[T, P]) Validate() error {
	flag, long := o.GetFlag(), o.GetLong()

	// An Option needs at least one of Flag/Long to be reachable from
	// the command line at all.
	if flag == "" && long == "" {
		return errors.New("cli: Option must set Flag and/or Long")
	}

	// The short form's fixed length ("-x") is what lets help output use
	// a constant-width Flag column instead of computing one per Command.
	if flag != "" && len(flag) != 2 {
		return fmt.Errorf("cli: Option.Flag %q must be a dash followed by exactly one character", flag)
	}
	return nil
}

// StringOption is a flag whose value is the token as typed, with no
// conversion.
type StringOption = OptionOf[string, stringParser]

// IntOption is a flag whose value must parse as a Go int
// (strconv.Atoi), e.g. "-n 7".
type IntOption = OptionOf[int, intParser]

// BoolOption is a boolean flag -- one whose mere presence on the command
// line sets it, never consuming a following token (see the Action doc
// comment). The type itself is what says "this flag is boolean".
type BoolOption = OptionOf[bool, boolParser]

// --- Action ------------------------------------------------------------------

// Action is a hook that runs when its Option's flag is seen on the
// command line. Parse calls it as opt.GetAction()(c, opt, value): c is
// the Command being parsed (so an Action can call c.PrintHelp,
// c.PrintVersion, or c.PrintErrorf, as Parse itself does), opt is the
// matched Option (assert it to its concrete type, e.g.
// opt.(*StringOption).Flag, to read its declared fields), and value is
// the string Parse hands to the Option's Set method: the token consumed
// from the command line, or "true" for a *BoolOption, which never
// consumes one. Any other Option, including an Action-only one with no
// Dest, consumes a value by default -- it is the type, not the Dest,
// that opts out.
//
// An Option may set Dest, Action, both, or neither. When both are set,
// Dest resolves first; Action runs only if that succeeded, and sees
// the same value Dest was just given.
//
// An Action's return value tells Parse whether to keep going:
//
//	nil              -- parsing continues, as for an ordinary Option.
//	a non-nil *Error -- parsing stops; Parse returns it unchanged.
//
// This is the contract PrintErrorf/ErrHelp/ErrVersion already follow:
// an Action reports a usage error with `return c.PrintErrorf(...)`, and
// may return ErrHelp, ErrVersion, or any other *Error. It is how the
// built-in -h/--help and -v/--version get their behavior (see options):
// each is a *BoolOption with a throwaway Dest and an Action that prints
// and returns ErrHelp/ErrVersion. A user-declared Option can do the
// same, with no special status for the built-ins.
type Action func(c *Command, opt Option, value string) *Error
