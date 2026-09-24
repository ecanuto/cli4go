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
// The interface itself is unexported-method-only, mirroring the
// Option.flag()/Option.long() pattern already used elsewhere in this
// package: a concrete type keeps its fields public (Name, Help, ...) for
// struct-literal construction, and satisfies Argument through small
// unexported accessor methods with different names, so field and method
// never collide.
type Argument interface {
	item
}

// argumentValidate is the base behavior behind every Argument
// implementation's validate() method (the closest thing Go has to a
// parent-class call: a concrete type's validate() calls it first, then
// adds any rule of its own): every Argument needs a display name, which
// shows in the usage line and the "Arguments:" help section. It never
// calls a.validate().
func argumentValidate(a Argument) error {
	if a.name() == "" {
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
}

func (a *StringArgument) name() string       { return a.Name }
func (a *StringArgument) help() string       { return a.Help }
func (a *StringArgument) env() string        { return a.Env }
func (a *StringArgument) validate() error    { return argumentValidate(a) }
func (a *StringArgument) set(s string) error { return setDest(a.Dest, s, parseString) }
func (a *StringArgument) setDefault()        { setDestDefault(a.Dest, a.Default) }

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

func (a *IntArgument) name() string       { return a.Name }
func (a *IntArgument) help() string       { return a.Help }
func (a *IntArgument) env() string        { return a.Env }
func (a *IntArgument) validate() error    { return argumentValidate(a) }
func (a *IntArgument) set(s string) error { return setDest(a.Dest, s, parseInt) }
func (a *IntArgument) setDefault()        { setDestDefault(a.Dest, a.Default) }
