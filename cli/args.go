package cli

import (
	"errors"
	"fmt"
)

// --- Argument ----------------------------------------------------------------

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

	// validate reports a structural problem with this Argument on its
	// own, independent of any other Argument -- today, an empty Name.
	// Cross-item rules (unique names) stay in validateArguments.
	validate() error
}

// --- StringArgument ----------------------------------------------------------

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

func (a *StringArgument) name() string    { return a.Name }
func (a *StringArgument) help() string    { return a.Help }
func (a *StringArgument) env() string     { return a.Env }
func (a *StringArgument) validate() error { return validateArgumentName(a.Name) }

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
}

func (a *IntArgument) name() string    { return a.Name }
func (a *IntArgument) help() string    { return a.Help }
func (a *IntArgument) env() string     { return a.Env }
func (a *IntArgument) validate() error { return validateArgumentName(a.Name) }

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

// --- Validate ----------------------------------------------------------------

// validateArgumentName is the per-item rule shared by every Argument
// implementation's validate() method: every Argument needs a display
// name -- it's what shows up in the usage line (e.g. "[qux]") and the
// Arguments: help section.
func validateArgumentName(name string) error {
	if name == "" {
		return errors.New("cli: Argument.Name must not be empty")
	}
	return nil
}

// validateArguments checks args for structural problems: an invalid
// Argument (see Argument.validate) or a duplicate Name. It is a free function (not a *Command method) so it
// stays decoupled from Command and testable on its own. Unlike the old
// design, there's no Default/Dest type check to run here -- a typed
// Argument's Default is already the same Go type as its Dest, enforced
// by the compiler (see validateDefaultType's doc comment for the one
// place that check still happens at runtime).
func validateArguments(args []Argument) error {
	seenNames := make(map[string]bool, len(args))
	for _, arg := range args {
		// Per-item rules live on the Argument itself.
		if err := arg.validate(); err != nil {
			return err
		}
		name := arg.name()

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
