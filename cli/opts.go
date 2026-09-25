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
// Option is implemented by a family of typed structs -- StringOption,
// IntOption, and BoolOption -- each owning its Dest and Default as its
// own Go type; see Argument's doc comment. The interface itself is
// unexported-method-only: a concrete type keeps its fields public for
// struct-literal construction and satisfies Option through small
// unexported accessor methods, so field and method never collide.
type Option interface {
	item
	flag() string
	long() string
	action() Action
	required() bool
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
// and long are expected normalized (see normalizeDashes); optionValidate
// guarantees one of them is set, so a validated Option's name is never
// empty.
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

// optionValidate is the base behavior behind every Option
// implementation's validate() method (see argumentValidate): it checks
// the normalized flag() and long() forms, and never calls o.validate().
func optionValidate(o Option) error {
	flag, long := o.flag(), o.long()

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

// --- StringOption ------------------------------------------------------------

// StringOption describes a single string-typed flag -- the Option
// implementation for the common case of a flag whose value is just the
// token as typed, with no conversion.
type StringOption struct {
	// Name is an optional identifier, decoupled from Flag/Long. It is
	// the Option's name(); when unset, that is Long, else Flag, without
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

	// Dest binds the parsed value directly to a string variable, e.g.
	// &cfg.Bar. Optional. May be combined with Action -- see the Action
	// doc comment for the exact resolution order.
	Dest *string

	// Default is assigned to Dest when neither the command line nor Env
	// sets it. Its zero value ("") applies just like any other value --
	// there is no separate "unset" state.
	Default string

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

func (o *StringOption) name() string       { return optionName(o.Name, o.flag(), o.long()) }
func (o *StringOption) flag() string       { return normalizeDashes(o.Flag, 1) }
func (o *StringOption) long() string       { return normalizeDashes(o.Long, 2) }
func (o *StringOption) help() string       { return o.Help }
func (o *StringOption) env() string        { return o.Env }
func (o *StringOption) action() Action     { return o.Action }
func (o *StringOption) required() bool     { return o.Required }
func (o *StringOption) validate() error    { return optionValidate(o) }
func (o *StringOption) set(s string) error { return setDest(o.Dest, s, parseString) }
func (o *StringOption) setDefault()        { setDestDefault(o.Dest, o.Default) }

// --- IntOption ---------------------------------------------------------------

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

	// Required -- see StringOption.Required.
	Required bool
}

func (o *IntOption) name() string       { return optionName(o.Name, o.flag(), o.long()) }
func (o *IntOption) flag() string       { return normalizeDashes(o.Flag, 1) }
func (o *IntOption) long() string       { return normalizeDashes(o.Long, 2) }
func (o *IntOption) help() string       { return o.Help }
func (o *IntOption) env() string        { return o.Env }
func (o *IntOption) action() Action     { return o.Action }
func (o *IntOption) required() bool     { return o.Required }
func (o *IntOption) validate() error    { return optionValidate(o) }
func (o *IntOption) set(s string) error { return setDest(o.Dest, s, parseInt) }
func (o *IntOption) setDefault()        { setDestDefault(o.Dest, o.Default) }

// --- BoolOption --------------------------------------------------------------

// BoolOption describes a single boolean flag -- one whose mere presence
// on the command line sets it, never consuming a following token (see
// the Action doc comment). It carries the same fields StringOption does;
// the type itself is what says "this flag is boolean".
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

	// Required -- see StringOption.Required.
	Required bool
}

func (o *BoolOption) name() string       { return optionName(o.Name, o.flag(), o.long()) }
func (o *BoolOption) flag() string       { return normalizeDashes(o.Flag, 1) }
func (o *BoolOption) long() string       { return normalizeDashes(o.Long, 2) }
func (o *BoolOption) help() string       { return o.Help }
func (o *BoolOption) env() string        { return o.Env }
func (o *BoolOption) action() Action     { return o.Action }
func (o *BoolOption) required() bool     { return o.Required }
func (o *BoolOption) validate() error    { return optionValidate(o) }
func (o *BoolOption) set(s string) error { return setDest(o.Dest, s, parseBool) }
func (o *BoolOption) setDefault()        { setDestDefault(o.Dest, o.Default) }

// --- Action ------------------------------------------------------------------

// Action is a hook that runs when its Option's flag is seen on the
// command line. Parse calls it as opt.action()(c, opt, value): c is
// the Command being parsed (so an Action can call c.PrintHelp,
// c.PrintVersion, or c.PrintErrorf, as Parse itself does), opt is the
// matched Option (assert it to its concrete type, e.g.
// opt.(*StringOption).Flag, to read its declared fields), and value is
// the string Parse hands to the Option's set method: the token consumed
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
