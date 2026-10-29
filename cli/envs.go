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

	// Set parses s into Dest, the value of the environment variable. It
	// returns whatever conversion error the concrete type produces (none
	// for a string), and does nothing -- returning nil -- when Dest is
	// unset.
	Set(s string) error

	// SetDefault assigns Default to Dest directly, when Dest is set, for
	// when the environment variable is not set. It can never fail, since
	// Default and Dest are the same Go type. The zero value applies too,
	// like any other Default.
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
	// &cfg.Debug. Optional -- an EnvVarOf with no Dest is purely
	// informational, shown in help but not resolved into anything (the
	// consuming application reads it itself, e.g. via os.Getenv).
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
}

func (e *EnvVarOf[T, P]) GetName() string    { return e.Name }
func (e *EnvVarOf[T, P]) GetHelp() string    { return e.Help }
func (e *EnvVarOf[T, P]) GetEnv() string     { return e.Name }
func (e *EnvVarOf[T, P]) GetDefault() any    { return e.Default }
func (e *EnvVarOf[T, P]) GetRequired() bool  { return e.Required }
func (e *EnvVarOf[T, P]) Set(s string) error { return setDest(e.Dest, s, parseWith[T, P]) }
func (e *EnvVarOf[T, P]) SetDefault()        { setDestDefault(e.Dest, e.Default) }

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
