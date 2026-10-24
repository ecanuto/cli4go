package cli

import (
	"testing"
)

// --- StringEnvVar ------------------------------------------------------------

// TestStringEnvVar_Accessors covers the accessors of StringEnvVar: its
// Name doubles as the environment variable it reads (GetEnv()).
func TestStringEnvVar_Accessors(t *testing.T) {
	e := &StringEnvVar{Name: "APP_DEBUG", Help: "Enable debug logging"}
	if got := e.GetRequired(); got != false {
		t.Errorf("GetRequired() = %v, want false, unset", got)
	}
	if got := (&StringEnvVar{Name: "APP_X", Required: true}).GetRequired(); got != true {
		t.Errorf("GetRequired() = %v, want true", got)
	}
	if got := e.GetName(); got != "APP_DEBUG" {
		t.Errorf("GetName() = %q, want %q", got, "APP_DEBUG")
	}
	if got := e.GetHelp(); got != "Enable debug logging" {
		t.Errorf("GetHelp() = %q, want %q", got, "Enable debug logging")
	}
	if got := e.GetEnv(); got != "APP_DEBUG" {
		t.Errorf("GetEnv() = %q, want %q", got, "APP_DEBUG")
	}
}

func TestStringEnvVar_Set(t *testing.T) {
	t.Run("assigns to Dest", func(t *testing.T) {
		var s string
		e := &StringEnvVar{Name: "APP_X", Dest: &s}
		if err := e.Set("hello"); err != nil {
			t.Fatalf("Set() = %v, want nil", err)
		}
		if s != "hello" {
			t.Errorf("Dest = %q, want %q", s, "hello")
		}
	})

	t.Run("nil Dest is a no-op", func(t *testing.T) {
		e := &StringEnvVar{Name: "APP_X"}
		if err := e.Set("hello"); err != nil {
			t.Errorf("Set() = %v, want nil", err)
		}
	})
}

func TestStringEnvVar_SetDefault(t *testing.T) {
	t.Run("assigns Default to Dest", func(t *testing.T) {
		s := "old"
		e := &StringEnvVar{Name: "APP_X", Dest: &s, Default: "new"}
		e.SetDefault()
		if s != "new" {
			t.Errorf("Dest = %q, want %q", s, "new")
		}
	})

	t.Run("the zero value applies too", func(t *testing.T) {
		s := "old"
		e := &StringEnvVar{Name: "APP_X", Dest: &s}
		e.SetDefault()
		if s != "" {
			t.Errorf("Dest = %q, want the zero value", s)
		}
	})

	t.Run("nil Dest is a no-op", func(t *testing.T) {
		e := &StringEnvVar{Name: "APP_X", Default: "new"}
		e.SetDefault()
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
			err := tt.env.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
