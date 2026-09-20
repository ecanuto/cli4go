# cli4go

A declarative, struct-based CLI toolkit for Go.

`cli4go` lets you describe a command-line program as plain data — a struct
literal — and get an `argparse`-style parsing experience with `clap`-style
formatted help output. No fluent builder chains, no required constructor:
just declare your `Program`, bind your config fields, and call `Parse()`.

```go
package main

import (
	"errors"
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
			{
				Name: "qux", Help: "The qux to operate on",
				Dest: cli.StringVar(&cfg.Qux),
			},
		},
		Options: []cli.Option{
			{
				Flag: "-f", Long: "--foo", Help: "Enable foo mode",
				Dest: cli.BoolVar(&cfg.Foo),
			},
			{
				Flag: "-b", Long: "--bar", Help: "Set the bar value",
				Dest: cli.StringVar(&cfg.Bar), Default: "default-bar", Env: "PROGRAM_BAR",
			},
		},
	}

	err := prg.DefaultParse()
	if err := prg.DefaultParse(); err != nil {
		os.Exit(err.ExitCode())
	}

	fmt.Printf("%s: foo=%v bar=%q qux=%q\n", prg.Name, cfg.Foo, cfg.Bar, cfg.Qux)
}
```

## Installation

```sh
go get github.com/ecanuto/cli4go/cli
```

## Features

- **Declarative by design.** A `Program` is a plain struct — arguments and
  options are data, not method calls. Build it inline, load it from a
  config, or generate it programmatically.
- **`clap`-style help output.** Usage, description, arguments, options, and
  environment variables are rendered in clearly separated, aligned sections.
- **Environment variable support.** Any option can declare an `Env` name;
  `cli4go` resolves it automatically as part of the value's precedence
  chain.
- **Typed variable binding.** `StringVar`, `IntVar`, `BoolVar` (and more to
  come) bind directly into your own config struct fields — no reflection
  gymnastics required on your end.

## Design notes

### Config as data

Because a `Program` (and its `Arguments` and `Options`) are plain struct
literals, `gofmt` aligns every field automatically. Declaring a CLI in
`cli4go` reads like a table, not a sequence of builder calls:

```go
Options: []cli.Option{
	{Flag: "-f", Long: "--foo", Help: "...", Dest: cli.BoolVar(&cfg.Foo)},
	{Flag: "-b", Long: "--bar", Help: "...", Dest: cli.StringVar(&cfg.Bar)},
},
```

This also means a `Program` is trivially inspectable and serializable —
laying the groundwork for generating one from a YAML/JSON config, or
generating Go code from a `Program` definition, in a future release.

### Value precedence

For any bound `Dest`, values are resolved in this order, from lowest to
highest priority:

```
Default → Environment variable (Env) → Command-line flag
```

A flag passed on the command line always wins; if it's absent, `cli4go`
falls back to the environment variable (if `Env` is set and present); if
that's also absent, it falls back to `Default`.

## Status

`cli4go` is under active early development. The API is not stable yet and
may change without notice until a `v1.0.0` is tagged.

## Roadmap

- [ ] Core parser (arguments, options, help/version generation)
- [ ] Subcommands
- [ ] Generate a `Program` from a YAML/JSON definition

## License

MIT — see [LICENSE](LICENSE).
