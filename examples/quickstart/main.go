// Command quickstart is a minimal, runnable version of the example from the
// README: it wires up a boolean flag, a string option with a default and an
// environment variable fallback, and a single positional argument.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/ecanuto/cli4go/cli"
)

// config holds the parsed values; each field is wired below via Dest,
// not by name or struct tag.
type config struct {
	Foo bool
	Bar string
	Qux string
}

func main() {
	var cfg config

	prg := &cli.Program{
		Name:        "myprogram",
		Version:     "0.1.0",
		Description: "My foo bar program",
		Arguments: []cli.Argument{
			&cli.StringArgument{
				// A positional argument: no Flag/Long, just a Name and a Dest.
				Name: "qux", Help: "The qux to operate on",
				Dest: &cfg.Qux,
			},
		},
		Options: []cli.Option{
			&cli.BoolOption{
				// A boolean flag: presence sets it true, absence leaves the
				// zero value (false).
				Flag: "-f", Long: "--foo",
				Help: "Enable foo mode", Dest: &cfg.Foo,
			},
			&cli.StringOption{
				// Precedence chain for a value that can come from several
				// places: an explicit -b/--bar flag wins over the
				// PROGRAM_BAR environment variable, which wins over Default.
				Flag: "-b", Long: "--bar", Default: "default-bar", Env: "PROGRAM_BAR",
				Help: "Set the bar value", Dest: &cfg.Bar,
			},
		},
	}

	// DefaultParse parses os.Args and handles -h/--help and -v/--version.
	// A non-nil return means "stop, exit with this code". ErrExit marks the
	// errors Parse already printed (help, version, usage errors); any other
	// error is still ours to print.
	if err := prg.DefaultParse(); err != nil {
		if !errors.Is(err, cli.ErrExit) {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(err.ExitCode())
	}

	// Print the parsed values.
	fmt.Printf("%s: foo=%v, bar=%q, qux=%q\n", prg.Name, cfg.Foo, cfg.Bar, cfg.Qux)
}
