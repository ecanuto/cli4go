# cli4go

[![CI](https://github.com/ecanuto/cli4go/actions/workflows/ci.yml/badge.svg)](https://github.com/ecanuto/cli4go/actions/workflows/ci.yml)

A declarative, struct-based CLI toolkit for Go.

`cli4go` lets you describe a command-line program as plain data -- a
struct literal -- and get an `argparse`-style parsing experience with
`clap`-style formatted help output. No fluent builder chains, no
required constructor: just declare your `Program`, bind your config
fields, and call `Parse()`.

```go
package main

import (
	"fmt"
	"os"

	"github.com/ecanuto/cli4go/cli"
)

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
				Name: "qux",
				Help: "The qux to operate on", Dest: &cfg.Qux,
			},
		},
		Options: []cli.Option{
			&cli.BoolOption{
				Flag: "-f", Long: "--foo",
				Help: "Enable foo mode", Dest: &cfg.Foo,
			},
			&cli.StringOption{
				Flag: "-b", Long: "--bar", Default: "default-bar", Env: "PROGRAM_BAR",
				Help: "Set the bar value", Dest: &cfg.Bar,
			},
		},
	}

	if err := prg.DefaultParse(); err != nil {
		os.Exit(err.ExitCode())
	}

	fmt.Printf("%s: foo=%v, bar=%q, qux=%q\n", prg.Name, cfg.Foo, cfg.Bar, cfg.Qux)
}
```

```console
$ go build -o myprogram .
$ ./myprogram --help
Usage: myprogram [options] [qux]

My foo bar program

Arguments:
  qux               The qux to operate on

Options:
  -f, --foo         Enable foo mode
  -b, --bar <BAR>   Set the bar value [env: PROGRAM_BAR] [default: default-bar]
  -h, --help        Print help
  -v, --version     Print version

Environment Variables:
  PROGRAM_BAR       Set the bar value

$ ./myprogram -f -b hello widget-42
myprogram: foo=true, bar="hello", qux="widget-42"
```

## Installation

```sh
go get github.com/ecanuto/cli4go/cli
```

## Features

- **Declarative by design.** A `Program` is a plain struct -- arguments
  and options are data, not method calls. Build it inline, load it from
  a config, or generate it programmatically.
- **`clap`-style help output.** Usage, description, arguments, options,
  and environment variables are rendered in clearly separated, aligned
  sections.
- **Environment variable support.** Any argument or option can declare
  an `Env` name, and a standalone `EnvVars` list covers variables that
  have no flag; `cli4go` resolves them automatically as part of the
  value's precedence chain.
- **Typed fields.** `StringArgument`, `IntArgument`, `StringOption`,
  `IntOption`, `BoolOption` and `StringEnvVar` each own a typed `Dest`
  and `Default`, so the compiler keeps them in sync -- no reflection and
  no runtime type checks on your end.

## Design notes

### Config as data

Because a `Program` (and its `Arguments`, `Options` and `EnvVars`) are
plain struct literals, `gofmt` aligns every field automatically.
Declaring a CLI in `cli4go` reads like a table, not a sequence of
builder calls:

```go
Options: []cli.Option{
	&cli.BoolOption{Flag: "-f", Long: "--foo", Help: "...", Dest: &cfg.Foo},
	&cli.StringOption{Flag: "-b", Long: "--bar", Help: "...", Dest: &cfg.Bar},
},
```

This also means a `Program` is trivially inspectable and serializable
-- laying the groundwork for generating one from a YAML/JSON config, or
generating Go code from a `Program` definition, in a future release.

### Typed fields

Every concrete type carries its own `Dest *T` and `Default T`, for
example `StringOption{Dest *string; Default string}` or
`IntOption{Dest *int; Default int}`. A mismatch between the two is a
compile error rather than a runtime one.

### Value precedence

For any bound `Dest`, values are resolved in this order, from lowest to
highest priority:

```
Default -> Environment variable (Env) -> Command-line flag
```

A flag passed on the command line always wins; if it's absent, `cli4go`
falls back to the environment variable (if `Env` is set and present);
if that's also absent, it falls back to `Default`.

Every item that is not given on the command line is written exactly
once, so a `Default` left at its zero value is applied like any other:
it overwrites whatever `Dest` held before `Parse()`. Set the initial
value through `Default`, not by pre-filling the destination.

An option can be given only once, through either its short or long
form; repeating it is a usage error too (exit code 2).

Invalid values (for example a non-numeric `IntOption` or a bad
environment variable) are reported as usage errors with exit code 2,
while declaration mistakes (duplicate flags, empty names, duplicate
environment variables) are reported by `Validate()` with exit code 1.

### Validating your declaration

A `Program` is plain data, so a mistake in it -- an empty `Name`, two
options claiming the same flag, two items sharing a name or an
environment variable -- is a bug in your Go code, not something an end
user typed. `Validate()` checks the whole declaration without parsing
anything and returns an `*Error` with exit code 1. `Parse()` runs it
first too, but a test catches the mistake before you ship, without
running your `main()`.

We recommend at least one test per program that calls it:

```go
func TestProgram(t *testing.T) {
	if err := newProgram().Validate(); err != nil {
		t.Fatal(err)
	}
}
```

Here `newProgram()` is whatever function builds your `Program` literal;
keeping it out of `main()` is what makes it testable.

## Status

`cli4go` is under active early development. The API is not stable yet
and may change without notice until a `v1.0.0` is tagged.

## License

MIT -- see [LICENSE](LICENSE).
