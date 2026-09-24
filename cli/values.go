package cli

import (
	"fmt"
	"strconv"
)

// parseInt and parseBool are the string -> Go value conversions shared
// by the typed Argument/Option family's set methods, so the error
// messages live in one place.
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
