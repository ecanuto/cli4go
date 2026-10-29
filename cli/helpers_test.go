package cli

import (
	"testing"
)

func TestStringParser(t *testing.T) {
	for _, in := range []string{"", "hello", "--not-a-flag"} {
		got, err := stringParser{}.Parse(in)
		if err != nil || got != in {
			t.Errorf("stringParser.Parse(%q) = %q, %v; want %q, nil", in, got, err, in)
		}
	}
}

func TestIntParser(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{input: "42", want: 42},
		{input: "-7", want: -7},
		{input: "abc", wantErr: true},
		{input: "", wantErr: true},
		{input: "3.5", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := intParser{}.Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("intParser.Parse(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("intParser.Parse(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestBoolParser(t *testing.T) {
	tests := []struct {
		input   string
		want    bool
		wantErr bool
	}{
		{input: "true", want: true},
		{input: "1", want: true},
		{input: "false", want: false},
		{input: "0", want: false},
		{input: "maybe", wantErr: true},
		{input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := boolParser{}.Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("boolParser.Parse(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("boolParser.Parse(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSetDest(t *testing.T) {
	parse := func(s string) (int, error) { return intParser{}.Parse(s) }

	tests := []struct {
		name    string
		dest    *int
		input   string
		want    int
		wantErr bool
	}{
		{name: "stores the converted value", dest: new(int), input: "42", want: 42},
		{name: "nil dest is a no-op", dest: nil, input: "42"},
		{name: "nil dest skips conversion errors", dest: nil, input: "abc"},
		{name: "conversion error leaves dest alone", dest: ptr(7), input: "abc", want: 7, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetDest(tt.dest, tt.input, parse)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SetDest() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.dest != nil && *tt.dest != tt.want {
				t.Errorf("*dest = %d, want %d", *tt.dest, tt.want)
			}
		})
	}
}

func TestSetDestDefault(t *testing.T) {
	tests := []struct {
		name string
		dest *string
		def  string
		want string
	}{
		{name: "stores the default", dest: ptr("old"), def: "new", want: "new"},
		{name: "stores the zero value", dest: ptr("old"), def: "", want: ""},
		{name: "nil dest is a no-op", dest: nil, def: "new"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetDestDefault(tt.dest, tt.def)
			if tt.dest != nil && *tt.dest != tt.want {
				t.Errorf("*dest = %q, want %q", *tt.dest, tt.want)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
