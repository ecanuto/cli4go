package cli

import (
	"errors"
)

// --- Argument ----------------------------------------------------------------

// Argument describes a single positional argument. Its position in
// Command.Arguments determines the order in which it is expected on the
// command line.
//
// Argument is implemented by the generic ArgumentOf, through the aliases
// StringArgument and IntArgument, rather than by one struct carrying a
// Dest/Default pair whose concrete type has to be inferred at runtime.
// Each instantiation owns its Dest and Default as its own Go type (e.g.
// *string/string, *int/int), so there is nothing to type-switch on, and
// the compiler keeps Default in sync with Dest for free.
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

// --- ArgumentOf --------------------------------------------------------------

// ArgumentOf describes a single positional argument whose value is a T,
// read from the command line by the Parser P. Use it through the aliases
// for the common types, StringArgument and IntArgument; an external
// package gets the same shape for its own type by declaring a Parser and
// an alias of its own.
type ArgumentOf[T any, P Parser[T]] struct {
	// Name is the argument's display name, used in generated usage text
	// (e.g. "qux" in "Usage: myprogram qux").
	Name string

	// Help explains what the argument is for. Shown in the generated
	// "Arguments:" help section.
	Help string

	// Dest binds the parsed value directly to a variable, e.g.
	// &cfg.Qux. Optional.
	Dest *T

	// Default is assigned to Dest when neither the command line nor Env
	// sets it. Its zero value applies just like any other value -- there
	// is no separate "unset" state.
	Default T

	// Env, if set, is the environment variable consulted when this
	// argument is not provided on the command line.
	Env string

	// Required makes Parse fail with a usage error (ExitCode() == 2)
	// when this argument is resolved from neither the command line nor
	// Env. Not meant to be combined with a non-zero Default; not
	// validated, since Default would just be dead code, not a bug.
	Required bool
}

func (a *ArgumentOf[T, P]) GetName() string   { return a.Name }
func (a *ArgumentOf[T, P]) GetHelp() string   { return a.Help }
func (a *ArgumentOf[T, P]) GetEnv() string    { return a.Env }
func (a *ArgumentOf[T, P]) GetRequired() bool { return a.Required }
func (a *ArgumentOf[T, P]) Set(s string) error {
	var p P
	return setDest(a.Dest, s, p.Parse)
}

func (a *ArgumentOf[T, P]) SetDefault() { setDestDefault(a.Dest, a.Default) }

// Validate requires a display name, which shows in the usage line and
// the "Arguments:" help section.
func (a *ArgumentOf[T, P]) Validate() error {
	if a.Name == "" {
		return errors.New("cli: Argument.Name must not be empty")
	}
	return nil
}

// StringArgument is a positional argument whose value is the token as
// typed, with no conversion.
type StringArgument = ArgumentOf[string, stringParser]

// IntArgument is a positional argument whose value must parse as a Go
// int (strconv.Atoi).
type IntArgument = ArgumentOf[int, intParser]
