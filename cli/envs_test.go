package cli

import (
	"testing"
)

// --- StringEnvVar ------------------------------------------------------------

// TestStringEnvVar_Accessors covers the accessors of StringEnvVar: its
// Name doubles as the environment variable it reads (env()).
func TestStringEnvVar_Accessors(t *testing.T) {
	e := &StringEnvVar{Name: "APP_DEBUG", Help: "Enable debug logging"}
	if got := e.name(); got != "APP_DEBUG" {
		t.Errorf("name() = %q, want %q", got, "APP_DEBUG")
	}
	if got := e.help(); got != "Enable debug logging" {
		t.Errorf("help() = %q, want %q", got, "Enable debug logging")
	}
	if got := e.env(); got != "APP_DEBUG" {
		t.Errorf("env() = %q, want %q", got, "APP_DEBUG")
	}
}

func TestStringEnvVar_Set(t *testing.T) {
	t.Run("assigns to Dest", func(t *testing.T) {
		var s string
		e := &StringEnvVar{Name: "APP_X", Dest: &s}
		if err := e.set("hello"); err != nil {
			t.Fatalf("set() = %v, want nil", err)
		}
		if s != "hello" {
			t.Errorf("Dest = %q, want %q", s, "hello")
		}
	})

	t.Run("nil Dest is a no-op", func(t *testing.T) {
		e := &StringEnvVar{Name: "APP_X"}
		if err := e.set("hello"); err != nil {
			t.Errorf("set() = %v, want nil", err)
		}
	})
}

func TestStringEnvVar_SetDefault(t *testing.T) {
	t.Run("assigns Default to Dest", func(t *testing.T) {
		s := "old"
		e := &StringEnvVar{Name: "APP_X", Dest: &s, Default: "new"}
		e.setDefault()
		if s != "new" {
			t.Errorf("Dest = %q, want %q", s, "new")
		}
	})

	t.Run("the zero value applies too", func(t *testing.T) {
		s := "old"
		e := &StringEnvVar{Name: "APP_X", Dest: &s}
		e.setDefault()
		if s != "" {
			t.Errorf("Dest = %q, want the zero value", s)
		}
	})

	t.Run("nil Dest is a no-op", func(t *testing.T) {
		e := &StringEnvVar{Name: "APP_X", Default: "new"}
		e.setDefault()
	})
}

// --- Validate ----------------------------------------------------------------

// TestEnvVar_Validate covers the per-item rule of every EnvVar type.
func TestEnvVar_Validate(t *testing.T) {
	tests := []struct {
		name    string
		env     EnvVar
		wantErr bool
	}{
		{name: "StringEnvVar valid", env: &StringEnvVar{Name: "APP_X"}},
		{name: "StringEnvVar empty Name", env: &StringEnvVar{}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.env.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateEnvVars exercises validateEnvVars directly. The names
// rule is cross-family (Arguments' Env, Options' Env, and EnvVars
// together), so each case takes a full Command, whose EnvVars and
// items() are what Validate hands to the function.
func TestValidateEnvVars(t *testing.T) {
	tests := []struct {
		name    string
		cmd     Command
		wantErr bool
	}{
		{
			name: "no env vars anywhere",
			cmd: Command{
				Arguments: []Argument{&StringArgument{Name: "qux"}},
				Options:   []Option{&StringOption{Flag: "-f"}},
			},
		},
		{
			name: "valid, one env var per source",
			cmd: Command{
				Arguments: []Argument{&StringArgument{Name: "qux", Env: "APP_QUX"}},
				Options:   []Option{&StringOption{Flag: "-b", Env: "APP_BAR"}},
				EnvVars:   []EnvVar{&StringEnvVar{Name: "APP_DEBUG"}},
			},
		},
		{
			name:    "empty EnvVar.Name",
			cmd:     Command{EnvVars: []EnvVar{&StringEnvVar{Name: ""}}},
			wantErr: true,
		},
		{
			name: "duplicate name, both from EnvVars",
			cmd: Command{
				EnvVars: []EnvVar{&StringEnvVar{Name: "APP_DEBUG"}, &StringEnvVar{Name: "APP_DEBUG"}},
			},
			wantErr: true,
		},
		{
			name: "duplicate name, both from Arguments",
			cmd: Command{
				Arguments: []Argument{
					&StringArgument{Name: "qux", Env: "APP_QUX"},
					&StringArgument{Name: "quux", Env: "APP_QUX"},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate name, Argument.Env vs Option.Env",
			cmd: Command{
				Arguments: []Argument{&StringArgument{Name: "qux", Env: "APP_QUX"}},
				Options:   []Option{&StringOption{Flag: "-b", Env: "APP_QUX"}},
			},
			wantErr: true,
		},
		{
			name: "duplicate name, Argument.Env vs EnvVars",
			cmd: Command{
				Arguments: []Argument{&StringArgument{Name: "qux", Env: "APP_QUX"}},
				EnvVars:   []EnvVar{&StringEnvVar{Name: "APP_QUX"}},
			},
			wantErr: true,
		},
		{
			name: "duplicate name, Option.Env vs EnvVars",
			cmd: Command{
				Options: []Option{&StringOption{Flag: "-b", Env: "APP_BAR"}},
				EnvVars: []EnvVar{&StringEnvVar{Name: "APP_BAR"}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEnvVars(tt.cmd.EnvVars, tt.cmd.items())
			if (err != nil) != tt.wantErr {
				t.Errorf("validateEnvVars() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
