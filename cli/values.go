package cli

import (
	"fmt"
	"strconv"
)

// Value is implemented by anything that can be set from a single
// command-line token. cli4go ships implementations for common types
// (see StringVar, BoolVar, IntVar). The typed Argument/Option family
// (StringArgument, StringOption, BoolOption, IntOption, ...) does not
// use Value at all: it binds Dest directly to a native pointer (e.g.
// *string) and converts strings through its own set method, sharing
// only the parseInt/parseBool helpers below with these implementations.
// Value remains the way to bind a standalone EnvVar's Dest, which has
// no typed family of its own, or to bind a custom implementation
// there.
type Value interface {
	// String returns the current value, formatted for display (used in
	// generated help text, e.g. when rendering defaults).
	String() string

	// Set parses s and stores the result. It returns an error if s
	// cannot be parsed into the underlying type.
	Set(s string) error
}

// --- Value implementations ---------------------------------------------------

type stringValue struct{ v *string }

// StringVar binds a string flag or argument to v.
func StringVar(v *string) Value { return &stringValue{v: v} }

func (s *stringValue) String() string {
	if s.v == nil {
		return ""
	}
	return *s.v
}

func (s *stringValue) Set(val string) error {
	*s.v = val
	return nil
}

type intValue struct{ v *int }

// IntVar binds an integer flag or argument to v.
func IntVar(v *int) Value { return &intValue{v: v} }

func (i *intValue) String() string {
	if i.v == nil {
		return "0"
	}
	return strconv.Itoa(*i.v)
}

func (i *intValue) Set(val string) error {
	parsed, err := parseInt(val)
	if err != nil {
		return err
	}
	*i.v = parsed
	return nil
}

type boolValue struct{ v *bool }

// BoolVar binds a boolean flag to v. A boolean option never consumes a
// following token: its mere presence on the command line sets v to true.
func BoolVar(v *bool) Value { return &boolValue{v: v} }

func (b *boolValue) String() string {
	if b.v == nil {
		return "false"
	}
	return strconv.FormatBool(*b.v)
}

func (b *boolValue) Set(val string) error {
	parsed, err := parseBool(val)
	if err != nil {
		return err
	}
	*b.v = parsed
	return nil
}

// parseInt and parseBool are the string -> Go value conversions shared
// by the Value implementations above and the typed Argument/Option
// family's set methods, so the error messages live in one place.
func parseInt(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid integer value %q: %w", s, err)
	}
	return n, nil
}

func parseBool(s string) (bool, error) {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return false, fmt.Errorf("invalid boolean value %q: %w", s, err)
	}
	return b, nil
}
