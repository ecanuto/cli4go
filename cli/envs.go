package cli

import (
	"errors"
)

// --- EnvVar ------------------------------------------------------------------

// EnvVar describes a standalone environment variable: one not bound to
// any Argument or Option's Env field. Unlike Argument/Option, it has no
// Flag/Long -- it is reachable only via the environment, never via a
// command-line token -- so its value precedence is just Env, then
// Default, with no flag position to override it.
//
// EnvVar is implemented by typed structs (StringEnvVar) with the same
// shape as Argument and Option, and for the same reason: each concrete
// type owns its Dest and Default as its own Go type, so there is
// nothing to type-check at runtime. See Argument's doc comment.
type EnvVar interface {
	item
}

// envVarValidate is the base behavior behind every EnvVar
// implementation's validate() method (see argumentValidate): every
// EnvVar needs a name, since the name is the environment variable
// itself. It never calls e.validate().
func envVarValidate(e EnvVar) error {
	if e.name() == "" {
		return errors.New("cli: EnvVar.Name must not be empty")
	}
	return nil
}

// --- StringEnvVar ------------------------------------------------------------

// StringEnvVar describes a standalone string-typed environment
// variable -- the EnvVar implementation for the common case of a
// variable whose value is used as-is, with no conversion.
type StringEnvVar struct {
	// Name is the environment variable name.
	Name string

	// Help explains what the environment variable is for. Shown in the
	// generated "Environment Variables:" help section.
	Help string

	// Dest binds the resolved value directly to a string variable, e.g.
	// &cfg.Debug. Optional -- a StringEnvVar with no Dest is purely
	// informational, shown in help but not resolved into anything (the
	// consuming application reads it itself, e.g. via os.Getenv).
	Dest *string

	// Default is assigned to Dest when the environment variable is not
	// set. Its zero value ("") applies just like any other value -- see
	// StringArgument.Default.
	Default string
}

func (e *StringEnvVar) name() string       { return e.Name }
func (e *StringEnvVar) help() string       { return e.Help }
func (e *StringEnvVar) env() string        { return e.Name }
func (e *StringEnvVar) validate() error    { return envVarValidate(e) }
func (e *StringEnvVar) set(s string) error { return setDest(e.Dest, s, parseString) }
func (e *StringEnvVar) setDefault()        { setDestDefault(e.Dest, e.Default) }
