package html

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestFormatMovesOnlyMultilineValuesWhoseWhitespaceDoesNotRender(t *testing.T) {
	src := "<div>\n<a title=\"one\ntwo\" class=\"a\nb\">k</a>\n</div>\n"
	want := "<div>\n    <a\n        title=\"one\ntwo\"\n        class=\"a\n        b\"\n    >k</a>\n</div>\n"

	got := mustFormat(t, []byte(src), config.Default())

	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
	}
}

func TestFormatKeepsTheLinesOfATemplateLiteralInAJSValue(t *testing.T) {
	src := "<div>\n<a x-data=\"{ msg: `one\n  two` }\" class=\"a\">k</a>\n</div>\n"
	want := "<div>\n    <a\n        x-data=\"{ msg: `one\n  two` }\"\n        class=\"a\"\n    >k</a>\n</div>\n"

	got := mustFormat(t, []byte(src), config.Default())

	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
	}
}
