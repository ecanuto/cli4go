// Command quickstart is a minimal, runnable version of the example from the
// README: it wires up a boolean flag, a string option with a default and an
// environment variable fallback, and a single positional argument.
package main

import (
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
				Dest: cli.StringVar(&cfg.Qux),
			},
		},
		Options: []cli.Option{
			&cli.BoolOption{
				// A boolean flag: presence sets it true, absence leaves the
				// zero value (false).
				Flag: "-f", Long: "--foo", Help: "Enable foo mode",
				Dest: cli.BoolVar(&cfg.Foo),
			},
			&cli.StringOption{
				// Precedence chain for a value that can come from several
				// places: an explicit -b/--bar flag wins over the
				// PROGRAM_BAR environment variable, which wins over Default.
				Flag: "-b", Long: "--bar", Help: "Set the bar value",
				Dest: cli.StringVar(&cfg.Bar), Default: "default-bar", Env: "PROGRAM_BAR",
			},
		},
	}

	// DefaultParse parses os.Args, handles -h/--help and -v/--version, and
	// prints any error itself; a non-nil return only means "stop, exit with
	// this code".
	if err := prg.DefaultParse(); err != nil {
		os.Exit(err.ExitCode())
	}

	// Print the parsed values.
	fmt.Printf("%s: foo=%v, bar=%q, qux=%q\n", prg.Name, cfg.Foo, cfg.Bar, cfg.Qux)
}
