package cli

import (
	"fmt"
)

// EnvVar describes a standalone environment variable: one not bound to
// any Argument or Option's Env field. Unlike Argument/Option, it has no
// Flag/Long -- it is reachable only via the environment, never via a
// command-line token -- so its value precedence is just Default -> Env,
// with no flag position to override it.
type EnvVar struct {
	// Name is the environment variable name.
	Name string

	// Help explains what the environment variable is for. Shown in the
	// generated "Environment Variables:" help section.
	Help string

	// Dest binds the resolved value to a variable. See StringVar,
	// BoolVar, IntVar. Optional -- an EnvVar with no Dest is purely
	// informational, shown in help but not resolved into anything (the
	// consuming application reads it itself, e.g. via os.Getenv).
	Dest Value

	// Default is shown in the help text as "(default: ...)" and is
	// assigned to Dest before Env is considered.
	Default any
}

// validateDefaultType checks that def, when set, matches the Go type
// expected by dest's constructor (StringVar -> string, BoolVar -> bool,
// IntVar -> int). Only a standalone EnvVar still needs this check --
// its Dest/Default pair is untyped (Value/any), unlike Argument/
// Option's typed Dest/Default fields (e.g. StringOption's *string/
// string), which the compiler already keeps in sync. Custom Value
// implementations -- ones not created by cli4go's own constructors --
// are not checked here, since cli4go has no way to know what type they
// expect.
func validateDefaultType(dest Value, def any) error {
	if dest == nil || def == nil {
		return nil
	}

	switch dest.(type) {
	case *stringValue:
		if _, ok := def.(string); !ok {
			return fmt.Errorf("cli: Default %v (%T) is not a string, but Dest is bound via StringVar", def, def)
		}
	case *boolValue:
		if _, ok := def.(bool); !ok {
			return fmt.Errorf("cli: Default %v (%T) is not a bool, but Dest is bound via BoolVar", def, def)
		}
	case *intValue:
		if _, ok := def.(int); !ok {
			return fmt.Errorf("cli: Default %v (%T) is not an int, but Dest is bound via IntVar", def, def)
		}
	}

	return nil
}
