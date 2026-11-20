package formatter

import (
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestCSSQuoteTarget(t *testing.T) {
	cases := []struct {
		quoteStyle string
		want       byte
	}{
		{config.QuoteStyleAuto, '"'},
		{config.QuoteStyleSingle, '\''},
	}

	for _, tc := range cases {
		t.Run(tc.quoteStyle, func(t *testing.T) {
			cfg := config.Default()
			cfg.QuoteStyle = tc.quoteStyle

			if got := CSSQuoteTarget(cfg); got != tc.want {
				t.Fatalf("CSSQuoteTarget = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAttrQuote(t *testing.T) {
	cases := []struct {
		name  string
		src   byte
		value string
		want  byte
	}{
		{"single quote canonicalizes to double", '\'', "x", '"'},
		{"double is left unchanged", '"', "x", '"'},
		{"double quote inside value keeps single", '\'', `say "hi"`, '\''},
		{"single quote inside value still takes double", '\'', "it's", '"'},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AttrQuote(tc.src, []byte(tc.value)); got != tc.want {
				t.Fatalf("AttrQuote(%q, %q) = %q, want %q", tc.src, tc.value, got, tc.want)
			}
		})
	}
}

func TestUnquotedAttrDelimiter(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  byte
	}{
		{"plain value takes double", "x", '"'},
		{"value holding a double quote takes single", `a"b`, '\''},
		{"value holding both has no delimiter", `a"b'c`, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := UnquotedAttrDelimiter([]byte(tc.value)); got != tc.want {
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
		{"unquoted value gains double", 0, "x", `"x"`},
		{"unquoted value holding a double quote goes out bare", 0, `a"b`, `a"b`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := NewWriter(config.Default())

			WriteAttrValue(w, tc.src, []byte(tc.value))

			if got := string(w.Bytes()); got != tc.want {
				t.Fatalf("WriteAttrValue(%q, %q) wrote %s, want %s", tc.src, tc.value, got, tc.want)
			}
		})
	}
}
