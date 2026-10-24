package cli

import (
	"errors"
)

// --- Argument ----------------------------------------------------------------

// Argument describes a single positional argument. Its position in
// Command.Arguments determines the order in which it is expected on the
// command line.
//
// Argument is implemented by a family of typed structs -- StringArgument
// and IntArgument -- rather than by one struct carrying a Dest/Default
// pair whose concrete type has to be inferred at runtime. Each concrete
// type owns its Dest and Default as its own Go type (e.g. *string/string,
// *int/int), so there is nothing to type-switch on, and the compiler
// keeps Default in sync with Dest for free.
//
// The interface's accessors are exported (GetName, GetHelp, ...) so a
// value held only as Argument (e.g. from Command.Arguments) can still be
// read without a type assertion to its concrete struct; each concrete
// type also keeps its fields public (Name, Help, ...) for struct-literal
// construction. The Get prefix, mirrored by Option.GetFlag/Option.GetLong
// elsewhere in this package, is what keeps field and method from
// colliding -- a plain Name() would clash with the Name field.
type Argument interface {
	// GetName returns the display name shown in the usage line and in the
	// "Arguments:" help section.
	GetName() string

	// GetHelp returns the description shown in the help output.
	GetHelp() string

	// GetEnv returns the name of the environment variable this Argument
	// falls back to when the command line does not set it, or "" if none.
	GetEnv() string

	// GetRequired reports whether Parse fails with a usage error when
	// neither the command line nor GetEnv's variable sets this Argument.
	GetRequired() bool

	// Validate reports a structural problem with this Argument on its own,
	// independent of any other item -- an empty Name, for instance.
	// Cross-item rules (unique names and environment variable names) stay
	// in Command.Validate.
	Validate() error

	// Set parses s into Dest, the token Parse (or an Env lookup) just
	// resolved for this Argument. It returns whatever conversion error the
	// concrete type produces (none for a string), and does nothing --
	// returning nil -- when Dest is unset.
	Set(s string) error

	// SetDefault assigns Default to Dest directly, when Dest is set -- no
	// string round trip, and it can never fail, since Default and Dest are
	// the same Go type. The zero value applies too, like any other Default
	// (see StringArgument's Default doc comment).
	SetDefault()
}

// argumentValidate is the base behavior behind every Argument
// implementation's Validate() method (the closest thing Go has to a
// parent-class call: a concrete type's Validate() calls it first, then
// adds any rule of its own): every Argument needs a display name, which
// shows in the usage line and the "Arguments:" help section. It never
// calls a.Validate().
func argumentValidate(a Argument) error {
	if a.GetName() == "" {
		return errors.New("cli: Argument.Name must not be empty")
	}
	return nil
}

// --- StringArgument ----------------------------------------------------------

// StringArgument describes a single string-typed positional argument --
// the Argument implementation for the common case of an argument whose
// value is just the token as typed, with no conversion.
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

	// Default is assigned to Dest when neither the command line nor Env
	// sets it. Its zero value ("") applies just like any other value --
	// there is no separate "unset" state.
	Default string

	// Env, if set, is the environment variable consulted when this
	// argument is not provided on the command line.
	Env string

	// Required makes Parse fail with a usage error (ExitCode() == 2)
	// when this argument is resolved from neither the command line nor
	// Env. Not meant to be combined with a non-zero Default; not
	// validated, since Default would just be dead code, not a bug.
	Required bool
}

func (a *StringArgument) GetName() string    { return a.Name }
func (a *StringArgument) GetHelp() string    { return a.Help }
func (a *StringArgument) GetEnv() string     { return a.Env }
func (a *StringArgument) GetRequired() bool  { return a.Required }
func (a *StringArgument) Validate() error    { return argumentValidate(a) }
func (a *StringArgument) Set(s string) error { return setDest(a.Dest, s, parseString) }
func (a *StringArgument) SetDefault()        { setDestDefault(a.Dest, a.Default) }

// --- IntArgument -------------------------------------------------------------

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

	// Required -- see StringArgument.Required.
	Required bool
}

func (a *IntArgument) GetName() string    { return a.Name }
func (a *IntArgument) GetHelp() string    { return a.Help }
func (a *IntArgument) GetEnv() string     { return a.Env }
func (a *IntArgument) GetRequired() bool  { return a.Required }
func (a *IntArgument) Validate() error    { return argumentValidate(a) }
func (a *IntArgument) Set(s string) error { return setDest(a.Dest, s, parseInt) }
func (a *IntArgument) SetDefault()        { setDestDefault(a.Dest, a.Default) }
