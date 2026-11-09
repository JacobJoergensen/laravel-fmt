package xml

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestIgnoredElementKeepsItsCharacterData(t *testing.T) {
	src := "<root>\n        <!-- laravel-fmt-ignore -->\n        <pre xml:space=\"preserve\">a\n        b</pre>\n</root>\n"
	want := "    <pre xml:space=\"preserve\">a\n        b</pre>\n"

	cfg := config.Default()
	got := mustFormat(t, []byte(src), cfg)

	if !bytes.Contains(got, []byte(want)) {
		t.Fatalf("ignored element changed.\n=== got ===\n%q\n=== want it to hold ===\n%q", got, want)
	}

	if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
	}
}

func TestIgnoredElementWithoutCharacterDataMovesAsOne(t *testing.T) {
	src := "<root>\n\t<!-- laravel-fmt-ignore -->\n\t<keep>\n\t\t<a   b='1'/>\n\t</keep>\n</root>\n"
	want := "    <keep>\n    \t<a   b='1'/>\n    </keep>\n"

	cfg := config.Default()
	got := mustFormat(t, []byte(src), cfg)

	if !bytes.Contains(got, []byte(want)) {
		t.Fatalf("ignored element came apart.\n=== got ===\n%q\n=== want it to hold ===\n%q", got, want)
	}

	if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
	}
}
