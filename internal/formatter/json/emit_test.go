package json

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
)

func TestFormatMatchesGolden(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".json", false) {
		t.Run(name, func(t *testing.T) {
			input := formattertest.ReadFixture(t, name+".json")
			want := formattertest.ReadFixture(t, name+".formatted.json")

			got := mustFormat(t, name+".json", input, config.Default())

			if !bytes.Equal(got, want) {
				t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
			}
		})
	}
}

func TestFormatIsIdempotent(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".json", false) {
		t.Run(name, func(t *testing.T) {
			input := formattertest.ReadFixture(t, name+".json")

			once := mustFormat(t, name+".json", input, config.Default())
			twice := mustFormat(t, name+".json", once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("Format is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
			}
		})
	}
}

func TestFormatJSONCMatchesGolden(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".jsonc", false) {
		t.Run(name, func(t *testing.T) {
			input := formattertest.ReadFixture(t, name+".jsonc")
			want := formattertest.ReadFixture(t, name+".formatted.jsonc")

			got := mustFormat(t, name+".jsonc", input, config.Default())

			if !bytes.Equal(got, want) {
				t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
			}
		})
	}
}

func TestFormatJSONCIsIdempotent(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".jsonc", false) {
		t.Run(name, func(t *testing.T) {
			input := formattertest.ReadFixture(t, name+".jsonc")

			once := mustFormat(t, name+".jsonc", input, config.Default())
			twice := mustFormat(t, name+".jsonc", once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("Format is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
			}
		})
	}
}

// 116 characters plus `[""]` is exactly the print width of 120. The é rows
// check that width is counted per character, not per byte.
func TestFormatKeepsContainerFlatUpToPrintWidth(t *testing.T) {
	cfg := config.Default()

	tests := []struct {
		name  string
		rune  string
		count int
		flat  bool
	}{
		{"ascii at print width", "a", 116, true},
		{"ascii one column over", "a", 117, false},
		{"multi-byte at print width", "é", 116, true},
		{"multi-byte one column over", "é", 117, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			content := strings.Repeat(tc.rune, tc.count)
			src := []byte(`["` + content + `"]`)

			want := "[\"" + content + "\"]\n"
			if !tc.flat {
				want = "[\n  \"" + content + "\"\n]\n"
			}

			got := mustFormat(t, "test.json", src, cfg)

			if string(got) != want {
				t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
			}
		})
	}
}

func TestFormatJSONCKeepsEveryComment(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			"between key and value",
			"{\n  \"a\": /* x */ 1,\n  \"b\" /* y */: 2\n}\n",
			"{\n  \"a\": /* x */ 1,\n  \"b\": /* y */ 2\n}\n",
		},
		{
			"line comment before a value",
			"{\n  \"c\": // why\n    3\n}\n",
			"{\n  \"c\": // why\n  3\n}\n",
		},
		{
			"comments only",
			"// only a comment\n",
			"// only a comment\n",
		},
		{
			"same-line comment after the root",
			"{\"a\": 1} // why\n// own\n",
			"{ \"a\": 1 } // why\n// own\n",
		},
		{
			"multi-line block comment follows its indent",
			"{\n    /* a\n       b */\n    \"x\": 1\n}\n",
			"{\n  /* a\n     b */\n  \"x\": 1\n}\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, "tsconfig.json", []byte(tc.src), config.Default())
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			twice := mustFormat(t, "tsconfig.json", got, config.Default())
			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent: %q then %q", got, twice)
			}
		})
	}
}

func TestFormatRejectsInvalidSource(t *testing.T) {
	out, err := Format("test.json", []byte(`{"a": 1,}`), config.Default())

	if err == nil {
		t.Fatal("Format accepted invalid source; want a parse error")
	}

	if out != nil {
		t.Fatalf("Format returned non-nil bytes on parse error: %q", out)
	}
}

func TestFormatEmptyInput(t *testing.T) {
	out, err := Format("test.json", []byte("   \n\t  "), config.Default())
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}

	if len(out) != 0 {
		t.Fatalf("Format returned %q, want empty", out)
	}
}

func mustFormat(t *testing.T, filename string, src []byte, cfg *config.File) []byte {
	t.Helper()

	out, err := Format(filename, src, cfg)
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}

	return out
}

func TestFormatKeepsAnAuthoredBlankLineBetweenItems(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		src      string
		want     string
	}{
		{"between members", "package.json", "{\n\"a\": 1,\n\n\"b\": 2\n}", "{\n  \"a\": 1,\n\n  \"b\": 2\n}"},
		{"between elements", "data.json", "[\n1,\n\n2\n]", "[\n  1,\n\n  2\n]"},
		{"around a comment", "tsconfig.json", "{\n\"a\": 1,\n\n// note\n\n\"b\": 2\n}", "{\n  \"a\": 1,\n\n  // note\n\n  \"b\": 2\n}"},
		{"a run collapses to one", "data.json", "{\n\"a\": 1,\n\n\n\n\"b\": 2\n}", "{\n  \"a\": 1,\n\n  \"b\": 2\n}"},
		{"edges drop", "data.json", "{\n\n\"a\": 1\n\n}", "{\n  \"a\": 1\n}"},
	}

	cfg := config.Default()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, tc.filename, []byte(tc.src+"\n"), cfg)
			if want := tc.want + "\n"; string(got) != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := mustFormat(t, tc.filename, got, cfg); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent:\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}
