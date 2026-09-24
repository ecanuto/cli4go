package cli

import (
	"fmt"
	"strconv"
)

// parseString, parseInt and parseBool are the string -> Go value
// conversions behind the typed Argument/Option/EnvVar family's set
// methods (see setDest), so the error messages live in one place.
func parseString(s string) (string, error) { return s, nil }

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

// setDest converts s with parse and stores the result in dest, doing
// nothing -- and returning nil -- when dest is unset. It is the base
// behavior behind every typed item's set method: only parse differs
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
// behavior behind every typed item's setDefault method: def and dest
// are the same Go type, so it can never fail.
func setDestDefault[T any](dest *T, def T) {
	if dest != nil {
		*dest = def
	}
}
