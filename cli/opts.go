package cli

import (
	"errors"
	"fmt"
	"strings"
)

// --- Option ------------------------------------------------------------------

// Option describes a single flag (e.g. "-f, --foo"). Unlike Argument, the
// order of Options in Command.Options has no effect on parsing: flags can
// appear in any order on the command line.
//
// Option is implemented by a family of typed structs -- StringOption,
// IntOption, and BoolOption today -- instead of one struct carrying a
// Dest/Default pair whose concrete type has to be inferred at runtime.
// See StringOption, IntOption, and BoolOption, and Argument's doc
// comment above for the same reasoning applied there.
//
// The interface itself is unexported-method-only: a concrete type keeps
// its fields public (Name, Flag, Long, ...) for struct-literal
// construction, and satisfies Option through small unexported accessor
// methods with different names from the fields they read, so field and
// method never collide -- the same pattern the old Option.flag()/
// Option.long() already used before Option became an interface.
type Option interface {
	name() string
	help() string
	env() string
	flag() string
	long() string
	action() Action

	// validate reports a structural problem with this Option on its
	// own, independent of any other Option -- a missing Flag/Long or a
	// malformed Flag. Cross-item rules (uniqueness of Flag, Long and
	// name()) stay in validateOptions.
	validate() error

	set(s string) error // see Argument.set
	setDefault()        // see Argument.setDefault
}

// normalizeDashes returns s with exactly dashes leading dashes, first
// stripping whatever dashes s already has -- the shared rule behind
// both a Flag ("f", "-f", "--f" -> "-f" with dashes=1) and a Long
// ("bar", "-bar", "--bar", "---bar" -> "--bar" with dashes=2). A single
// leading dash on what's meant to be a Long is treated the same as no
// dash at all, so "-bar" (a likely typo) is normalized rather than
// rejected. Idempotent on an already-normalized or empty s. Shared by
// every Option implementation's flag()/long() methods, so the
// normalization rule only lives in one place.
func normalizeDashes(s string, dashes int) string {
	if s == "" {
		return s
	}
	return strings.Repeat("-", dashes) + strings.TrimLeft(s, "-")
}

// optionName returns the identifier of an Option: name when set,
// otherwise long without its leading "--", otherwise flag without its
// leading "-". flag and long are expected in normalized form (see
// normalizeDashes). optionValidate guarantees at least one of the two
// is set, so the result is never empty for a validated Option. Shared
// by every Option implementation's name() method.
func optionName(name, flag, long string) string {
	switch {
	case name != "":
		return name
	case long != "":
		return strings.TrimPrefix(long, "--")
	default:
		return strings.TrimPrefix(flag, "-")
	}
}

// optionValidate is the base behavior behind every Option
// implementation's validate() method (the closest thing Go has to a
// parent-class call: a concrete type's validate() calls it first, then
// adds any rule of its own). It checks the normalized flag() and
// long() forms, and never calls o.validate() itself.
func optionValidate(o Option) error {
	flag, long := o.flag(), o.long()

	// An Option needs at least one of Flag/Long to be reachable from
	// the command line at all.
	if flag == "" && long == "" {
		return errors.New("cli: Option must set Flag and/or Long")
	}

	// The short form's fixed length ("-x") is what lets help output use
	// a constant-width Flag column instead of computing one per Command.
	if flag != "" && len(flag) != 2 {
		return fmt.Errorf("cli: Option.Flag %q must be a dash followed by exactly one character", flag)
	}
	return nil
}

// --- StringOption ------------------------------------------------------------

// StringOption describes a single string-typed flag -- the Option
// implementation for the common case of a flag whose value is just the
// token as typed, with no conversion. It carries the exact same fields
// the old, unified Option struct did; only the name and its place in
// the type system have changed.
type StringOption struct {
	// Name is an optional, stable identifier for this Option, decoupled
	// from Flag/Long. It exists for two purposes: as the preferred
	// source for the generated help placeholder (e.g. "<BAR>", falling
	// back to Long then Flag when Name is unset), and, in a future
	// bind/AutoBind feature, as the key used to match this Option
	// against a config struct field -- something Flag/Long ("-b",
	// "--bar") are not well suited for, since they're user-facing
	// display text rather than a stable identity.
	Name string

	// Flag is the short flag form, e.g. "f" or "-f" -- the leading dash
	// is optional and added automatically if omitted. Optional field.
	//
	// Declaring both this and the matching Long form of a built-in
	// (-h and --help, or -v and --version), whether on one Option or
	// split across several, suppresses that built-in entry from the
	// generated help and from Parse's dispatch; declaring only one of
	// the two forms is a conflict, rejected by Validate. The one
	// exception is -v: declaring it without --version makes the
	// version built-in use -V instead. See options' doc comment.
	Flag string

	// Long is the long flag form, e.g. "bar" or "--bar" -- the leading "--"
	// is optional and added automatically if omitted. Optional, though at
	// least one of Flag/Long should be set.
	//
	// See Flag's doc comment for how declaring a built-in's forms
	// affects that built-in. In particular, declaring "--version"
	// while leaving "-v" free is a conflict, not a fallback: only a
	// claimed "-v" makes the version built-in switch to "-V".
	Long string

	// Help explains what the option does. Shown in the generated
	// "Options:" help section.
	Help string

	// Dest binds the parsed value directly to a string variable, e.g.
	// &cfg.Bar. Optional. May be combined with Action -- see the Action
	// doc comment for the exact resolution order.
	Dest *string

	// Default is shown in the help text as "(default: ...)" and is
	// assigned to Dest before Env and the command line are considered.
	// Its zero value ("") applies just like any other value -- there
	// is no separate "unset" state, the same way a stdlib flag always
	// has a default, even an empty one.
	Default string

	// Env, if set, is the environment variable consulted when this
	// option is not provided on the command line.
	Env string

	// Action, if set, runs when this Option's flag is seen on the
	// command line -- see the Action doc comment for its exact contract
	// and how it combines with Dest.
	Action Action
}

func (o *StringOption) name() string    { return optionName(o.Name, o.flag(), o.long()) }
func (o *StringOption) help() string    { return o.Help }
func (o *StringOption) flag() string    { return normalizeDashes(o.Flag, 1) }
func (o *StringOption) long() string    { return normalizeDashes(o.Long, 2) }
func (o *StringOption) env() string     { return o.Env }
func (o *StringOption) action() Action  { return o.Action }
func (o *StringOption) validate() error { return optionValidate(o) }

func (o *StringOption) set(s string) error {
	if o.Dest != nil {
		*o.Dest = s
	}
	return nil
}

func (o *StringOption) setDefault() {
	if o.Dest != nil {
		*o.Dest = o.Default
	}
}

// --- IntOption ---------------------------------------------------------------

// IntOption describes a single flag whose value must parse as a Go int
// (strconv.Atoi) -- the Option implementation for the common case of an
// integer-valued flag, e.g. "-n 7". Same fields and shape as
// StringOption; see its doc comment for Dest/Default's ergonomics and
// semantics, and Flag/Long's for how a built-in's forms are affected.
type IntOption struct {
	// Name -- see StringOption.Name.
	Name string

	// Flag -- see StringOption.Flag.
	Flag string

	// Long -- see StringOption.Long.
	Long string

	// Help -- see StringOption.Help.
	Help string

	// Dest binds the parsed value directly to an int variable, e.g.
	// &cfg.Count. Optional. May be combined with Action -- see the
	// Action doc comment for the exact resolution order.
	Dest *int

	// Default -- see StringOption.Default.
	Default int

	// Env -- see StringOption.Env.
	Env string

	// Action -- see StringOption.Action.
	Action Action
}

func (o *IntOption) name() string    { return optionName(o.Name, o.flag(), o.long()) }
func (o *IntOption) help() string    { return o.Help }
func (o *IntOption) flag() string    { return normalizeDashes(o.Flag, 1) }
func (o *IntOption) long() string    { return normalizeDashes(o.Long, 2) }
func (o *IntOption) env() string     { return o.Env }
func (o *IntOption) action() Action  { return o.Action }
func (o *IntOption) validate() error { return optionValidate(o) }

func (o *IntOption) set(s string) error {
	if o.Dest == nil {
		return nil
	}
	n, err := parseInt(s)
	if err != nil {
		return err
	}
	*o.Dest = n
	return nil
}

func (o *IntOption) setDefault() {
	if o.Dest != nil {
		*o.Dest = o.Default
	}
}

// --- BoolOption --------------------------------------------------------------

// BoolOption describes a single boolean flag -- one whose mere presence
// on the command line sets it, never consuming a following token (see
// the Action doc comment). It carries the same fields StringOption does;
// the type itself is what says "this flag is boolean", replacing the
// old design's isBoolFlag(Dest) runtime check on a BoolVar-bound Dest.
type BoolOption struct {
	// Name -- see StringOption.Name.
	Name string

	// Flag -- see StringOption.Flag.
	Flag string

	// Long -- see StringOption.Long.
	Long string

	// Help -- see StringOption.Help.
	Help string

	// Dest binds the parsed value directly to a bool variable, e.g.
	// &cfg.Foo. Optional.
	Dest *bool

	// Default -- see StringOption.Default.
	Default bool

	// Env -- see StringOption.Env.
	Env string

	// Action -- see StringOption.Action.
	Action Action
}

func (o *BoolOption) name() string    { return optionName(o.Name, o.flag(), o.long()) }
func (o *BoolOption) help() string    { return o.Help }
func (o *BoolOption) flag() string    { return normalizeDashes(o.Flag, 1) }
func (o *BoolOption) long() string    { return normalizeDashes(o.Long, 2) }
func (o *BoolOption) env() string     { return o.Env }
func (o *BoolOption) action() Action  { return o.Action }
func (o *BoolOption) validate() error { return optionValidate(o) }

func (o *BoolOption) set(s string) error {
	if o.Dest == nil {
		return nil
	}
	b, err := parseBool(s)
	if err != nil {
		return err
	}
	*o.Dest = b
	return nil
}

func (o *BoolOption) setDefault() {
	if o.Dest != nil {
		*o.Dest = o.Default
	}
}

// --- Action ------------------------------------------------------------------

// Action is a hook that runs when its Option's flag is seen on the
// command line. Parse calls it as opt.action()(c, opt, value): c is
// the Command being parsed (so an Action can call
// c.PrintHelp/c.PrintVersion/c.PrintErrorf, exactly like Parse itself
// does), opt is the matched Option itself (so an Action can refer back
// to its own declared fields by asserting it to its concrete type,
// e.g. opt.(*StringOption).Flag, or by calling the small set of
// unexported accessor methods available to code inside this package),
// and value is the same string Parse would hand (or just handed) to
// the Option's set method: the token consumed from the command line by default, or
// "true" for a flag that opts out of consuming one -- see Dest's doc
// comment and BoolOption's; an Action-only Option (no Dest at all)
// still consumes a value by default, exactly like any other flag,
// unless it is declared as a *BoolOption -- the type itself is what
// opts it out, not anything about its Dest (a BoolOption can still
// leave Dest nil and rely on Action alone).
//
// An Option may set Dest, Action, both, or neither. When both are set,
// Dest resolves first -- Action only runs if set succeeded, and
// sees the exact same value Dest was just given, so it never needs to
// re-derive or re-parse anything Dest already handled.
//
// An Action's return value tells Parse whether to keep going:
//
//	nil              -- parsing continues, exactly as if this were an
//	                    ordinary Option with no Action.
//	a non-nil *Error -- parsing stops immediately; Parse returns this
//	                    *Error as-is, unchanged.
//
// This is the same contract PrintErrorf/ErrHelp/ErrVersion already
// follow, so an Action reports a real parsing/usage error the same way
// the rest of Parse does -- `return c.PrintErrorf("...", ...)` prints
// the clap-style block and returns the *Error to return -- and is free
// to return ErrHelp, ErrVersion, or any other *Error (e.g. built with
// newError) if that's the exit behavior it wants. This is how the
// built-in -h/--help and -v/--version flags get their behavior (see
// options): each is a BoolOption with Dest bound to a throwaway bool
// (never consuming a token, value always "true") and Action set to a
// closure that prints and returns ErrHelp/ErrVersion. A caller-declared
// Option can set its own Action exactly the same way, with no special
// status over the built-in ones -- optionally alongside a Dest of its
// own, and opting out of consuming a value the exact same way the
// built-ins do.
type Action func(c *Command, opt Option, value string) *Error

// --- Validate ----------------------------------------------------------------

// validateOptions checks opts for structural problems: an invalid Option
// (see Option.validate: a missing Flag/Long or a malformed Flag), a
// duplicate Flag/Long, or a duplicate name()
// (Option.Name, or the name derived from Long/Flag when Name is unset
// -- Parse keys its "set on the command line" bookkeeping by it, so it
// has to be unique). Same free-function
// shape as validateArguments, for the same reason -- and the same note
// applies: no Default/Dest type check here, since a typed Option's
// Default is already the same Go type as its Dest.
//
// Validate calls this with the effective Option set
// (slices.Collect(c.options())), not c.Options, so a user-declared
// Option colliding with a synthesized built-in is caught here exactly
// like two user-declared Options colliding. See options' doc
// comment for the suppression rule that decides which built-ins are
// even present in that effective set.
func validateOptions(opts []Option) error {
	seen := make(map[string]bool, len(opts)*2)
	seenNames := make(map[string]bool, len(opts))
	for _, opt := range opts {
		// Compare Flag/Long in their normalized form, so "-f"/"f" or
		// "--bar"/"bar" on two different Options are correctly treated
		// as the same form instead of slipping past the uniqueness
		// checks below as if they were different.
		flag := opt.flag()
		long := opt.long()

		// Per-item rules live on the Option itself.
		if err := opt.validate(); err != nil {
			return err
		}

		if flag != "" {
			if seen[flag] {
				return fmt.Errorf("cli: duplicate option flag form %q", flag)
			}
			seen[flag] = true
		}

		if long != "" {
			// Parse looks up an incoming "--xxx" token by its normalized
			// form, so Long must be unique across Options too (and Flag
			// above, for the same reason).
			if seen[long] {
				return fmt.Errorf("cli: duplicate option long form %q", long)
			}
			seen[long] = true
		}

		// Checked last so a duplicate Flag/Long is still reported as
		// such -- e.g. two Options sharing only a Flag would otherwise
		// also derive the same name and hit this error first.
		name := opt.name()
		if seenNames[name] {
			return fmt.Errorf("cli: duplicate option name %q", name)
		}
		seenNames[name] = true
	}
	return nil
}
