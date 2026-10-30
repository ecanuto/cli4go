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
// EnvVar is implemented by the generic EnvVarOf (through the alias
// StringEnvVar) with the same shape as Argument and Option, and for the
// same reason: each instantiation owns its Dest and Default as its own
// Go type, so there is nothing to type-check at runtime. See Argument's
// doc comment.
type EnvVar interface {
	// GetName returns the name of the environment variable itself.
	GetName() string

	// GetHelp returns the description shown in the help output.
	GetHelp() string

	// GetEnv returns the name of the environment variable, which is the
	// same as GetName.
	GetEnv() string

	// GetDefault returns Default as a plain value (a string, an int, ...),
	// which is the zero value of its type when none was declared.
	GetDefault() any

	// GetRequired reports whether Parse fails with a usage error when the
	// environment variable is not set.
	GetRequired() bool

	// GetValue returns the value of the environment variable, as a plain
	// value (a string, an int, ...): the environment's, else Default.
	// Before Parse it is the zero value of its type. It does not depend on
	// Dest, which receives the same value.
	GetValue() any

	// Set parses s and stores the result as this EnvVar's value, and in
	// Dest too when it is set. s is the value of the environment
	// variable. It returns whatever conversion error the concrete type
	// produces (none for a string).
	Set(s string) error

	// SetDefault stores Default as this EnvVar's value, and in Dest too
	// when it is set, for when the environment variable is not set. It can
	// never fail, since Default and Dest are the same Go type. The zero
	// value applies too, like any other Default.
	SetDefault()

	// Validate reports a structural problem with this EnvVar on its own,
	// independent of any other item -- an empty Name, for instance.
	// Cross-item rules (unique names and environment variable names) stay
	// in Command.Validate.
	Validate() error
}

// --- EnvVarOf ----------------------------------------------------------------

// EnvVarOf describes a standalone environment variable whose value is a
// T, read by the Parser P. Use it through the alias for the common type,
// StringEnvVar; see ArgumentOf for adding one of your own.
type EnvVarOf[T any, P Parser[T]] struct {
	// Name is the environment variable name.
	Name string

	// Help explains what the environment variable is for. Shown in the
	// generated "Environment Variables:" help section.
	Help string

	// Dest binds the resolved value directly to a variable, e.g.
	// &cfg.Debug. Optional -- without it the value is still available
	// through GetValue and Command.Lookup.
	Dest *T

	// Default is assigned to Dest when the environment variable is not
	// set. Its zero value applies just like any other value -- see
	// ArgumentOf.Default.
	Default T

	// Required makes Parse fail with a usage error (ExitCode() == 2)
	// when the environment variable is not set. Not meant to be combined
	// with a non-zero Default; not validated, since Default would just be
	// dead code, not a bug.
	Required bool

	// value is the item's own copy of the resolved value, which is what
	// GetValue returns. Dest, when set, receives the same value.
	value T
}

func (e *EnvVarOf[T, P]) GetName() string    { return e.Name }
func (e *EnvVarOf[T, P]) GetHelp() string    { return e.Help }
func (e *EnvVarOf[T, P]) GetEnv() string     { return e.Name }
func (e *EnvVarOf[T, P]) GetDefault() any    { return e.Default }
func (e *EnvVarOf[T, P]) GetRequired() bool  { return e.Required }
func (e *EnvVarOf[T, P]) GetValue() any      { return e.value }
func (e *EnvVarOf[T, P]) Set(s string) error { return setValue(&e.value, e.Dest, s, parseWith[T, P]) }
func (e *EnvVarOf[T, P]) SetDefault()        { setValueDefault(&e.value, e.Dest, e.Default) }

// Validate requires a name, since the name is the environment variable
// itself.
func (e *EnvVarOf[T, P]) Validate() error {
	if e.Name == "" {
		return errors.New("cli: EnvVar.Name must not be empty")
	}
	return nil
}

// StringEnvVar is a standalone environment variable whose value is used
// as-is, with no conversion.
type StringEnvVar = EnvVarOf[string, stringParser]
