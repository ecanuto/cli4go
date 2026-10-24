package cli

import (
	"fmt"
	"strconv"
)

// Parser converts a string into a T. It is the type parameter that tells
// ArgumentOf, OptionOf, and EnvVarOf how to read their value, and what
// binds each alias to its Go type (StringOption is OptionOf[string,
// stringParser]). A Parser is a zero-size type with no state: the
// conversion depends on T alone, and per-declaration configuration
// belongs in a type of its own.
type Parser[T any] interface {
	Parse(s string) (T, error)
}

// stringParser, intParser and boolParser are the Parsers behind the
// built-in aliases, so the error messages live in one place.
type stringParser struct{}

func (stringParser) Parse(s string) (string, error) {
	return s, nil
}

type intParser struct{}

func (intParser) Parse(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid integer value %q: %w", s, err)
	}
	return n, nil
}

type boolParser struct{}

func (boolParser) Parse(s string) (bool, error) {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return false, fmt.Errorf("invalid boolean value %q: %w", s, err)
	}
	return b, nil
}

// setDest converts s with parse and stores the result in dest, doing
// nothing -- and returning nil -- when dest is unset. It is the base
// behavior behind every item's Set method: only parse differs
// from one type to the next. On a conversion error dest is left alone.
func setDest[T any](dest *T, s string, parse func(string) (T, error)) error {
	if dest == nil {
		return nil
	}
	v, err := parse(s)
	if err != nil {
		return err
	}
	*dest = v
	return nil
}

// setDestDefault stores def in dest, when dest is set. It is the base
// behavior behind every item's SetDefault method: def and dest
// are the same Go type, so it can never fail.
func setDestDefault[T any](dest *T, def T) {
	if dest != nil {
		*dest = def
	}
}
