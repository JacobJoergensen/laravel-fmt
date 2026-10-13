package formatter

import (
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestAttrQuote(t *testing.T) {
	cases := []struct {
		name       string
		quoteStyle string
		src        byte
		value      string
		want       byte
	}{
		{"auto canonicalizes to double", config.QuoteStyleAuto, '\'', "x", '"'},
		{"double forces double", config.QuoteStyleDouble, '\'', "x", '"'},
		{"single forces single", config.QuoteStyleSingle, '"', "x", '\''},
		{"already target left unchanged", config.QuoteStyleDouble, '"', "x", '"'},
		{"target inside value keeps source", config.QuoteStyleDouble, '\'', `say "hi"`, '\''},
		{"single target inside value keeps source", config.QuoteStyleSingle, '"', "it's", '"'},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.QuoteStyle = tc.quoteStyle

			if got := AttrQuote(tc.src, []byte(tc.value), cfg); got != tc.want {
				t.Fatalf("AttrQuote(%q, %q) = %q, want %q", tc.src, tc.value, got, tc.want)
			}
		})
	}
}

func TestUnquotedAttrDelimiter(t *testing.T) {
	cases := []struct {
		name       string
		quoteStyle string
		value      string
		want       byte
	}{
		{"plain value takes the configured quote", config.QuoteStyleAuto, "x", '"'},
		{"single style is honored", config.QuoteStyleSingle, "x", '\''},
		{"value holding the configured quote takes the other", config.QuoteStyleDouble, `a"b`, '\''},
		{"value holding both has no delimiter", config.QuoteStyleDouble, `a"b'c`, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.QuoteStyle = tc.quoteStyle

			if got := UnquotedAttrDelimiter([]byte(tc.value), cfg); got != tc.want {
				t.Fatalf("UnquotedAttrDelimiter(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestWriteAttrValue(t *testing.T) {
	cases := []struct {
		name  string
		src   byte
		value string
		want  string
	}{
		{"quoted value takes the configured quote", '\'', "x", `"x"`},
		{"quoted value holding the target keeps its quote", '\'', `say "hi"`, `'say "hi"'`},
		{"unquoted value gains the configured quote", 0, "x", `"x"`},
		{"unquoted value holding the target goes out bare", 0, `a"b`, `a"b`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			w := NewWriter(cfg)

			WriteAttrValue(w, tc.src, []byte(tc.value), cfg)

			if got := string(w.Bytes()); got != tc.want {
				t.Fatalf("WriteAttrValue(%q, %q) wrote %s, want %s", tc.src, tc.value, got, tc.want)
			}
		})
	}
}
