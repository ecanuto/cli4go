package cli

import (
	"testing"
)

func TestStringVar(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "hello", want: "hello"},
		{input: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var s string
			v := StringVar(&s)

			err := v.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Set(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if s != tt.want {
				t.Errorf("bound variable = %q, want %q", s, tt.want)
			}
			if got := v.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStringVar_String(t *testing.T) {
	s := "hello"
	if got := StringVar(&s).String(); got != "hello" {
		t.Errorf("String() = %q, want %q", got, "hello")
	}
	if got := StringVar(nil).String(); got != "" {
		t.Errorf("String() with nil pointer = %q, want empty", got)
	}
}

func TestIntVar(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{input: "42", want: 42},
		{input: "-7", want: -7},
		{input: "not-a-number", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var i int
			v := IntVar(&i)

			err := v.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Set(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if i != tt.want {
				t.Errorf("bound variable = %d, want %d", i, tt.want)
			}
		})
	}
}

func TestIntVar_String(t *testing.T) {
	i := 42
	if got := IntVar(&i).String(); got != "42" {
		t.Errorf("String() = %q, want %q", got, "42")
	}
	if got := IntVar(nil).String(); got != "0" {
		t.Errorf("String() with nil pointer = %q, want %q", got, "0")
	}
}

func TestBoolVar(t *testing.T) {
	tests := []struct {
		input   string
		want    bool
		wantErr bool
	}{
		{input: "true", want: true},
		{input: "false", want: false},
		{input: "1", want: true},
		{input: "not-a-bool", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var b bool
			v := BoolVar(&b)

			err := v.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Set(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if b != tt.want {
				t.Errorf("bound variable = %v, want %v", b, tt.want)
			}
		})
	}
}

func TestBoolVar_String(t *testing.T) {
	b := true
	if got := BoolVar(&b).String(); got != "true" {
		t.Errorf("String() = %q, want %q", got, "true")
	}
	if got := BoolVar(nil).String(); got != "false" {
		t.Errorf("String() with nil pointer = %q, want %q", got, "false")
	}
}
