package xml

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
)

func TestFormatMatchesGolden(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".xml", false) {
		t.Run(name, func(t *testing.T) {
			input := formattertest.ReadFixture(t, name+".xml")
			want := formattertest.ReadFixture(t, name+".formatted.xml")

			got := mustFormat(t, input, config.Default())

			if !bytes.Equal(got, want) {
				t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
			}
		})
	}
}

func TestFormatIsIdempotent(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".xml", false) {
		t.Run(name, func(t *testing.T) {
			input := formattertest.ReadFixture(t, name+".xml")

			once := mustFormat(t, input, config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("Format is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
			}
		})
	}
}

func TestFormatOnFormattedIsStable(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".xml", false) {
		t.Run(name, func(t *testing.T) {
			want := formattertest.ReadFixture(t, name+".formatted.xml")

			got := mustFormat(t, want, config.Default())

			if !bytes.Equal(got, want) {
				t.Fatalf("formatted input was not stable.\n=== got ===\n%s\n=== want ===\n%s", got, want)
			}
		})
	}
}

func TestFormatEmptyInput(t *testing.T) {
	out, err := Format([]byte("   \n\t  "), config.Default())
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}

	if len(out) != 0 {
		t.Fatalf("Format returned %q, want empty", out)
	}
}

func TestFormatRejectsInvalidSource(t *testing.T) {
	out, err := Format([]byte("<a><b></a>"), config.Default())

	if err == nil {
		t.Fatal("Format accepted invalid source; want a parse error")
	}

	if out != nil {
		t.Fatalf("Format returned non-nil bytes on parse error: %q", out)
	}
}

func TestAttrQuoteStyle(t *testing.T) {
	cases := []struct {
		name         string
		quoteStyle   string
		input        string
		wantContains string
	}{
		{"auto canonicalizes to double", config.QuoteStyleAuto, `<tag a='x'/>`, `a="x"`},
		{"single forces single", config.QuoteStyleSingle, `<tag a="x"/>`, `a='x'`},
		{"conflicting inner quote left untouched", config.QuoteStyleDouble, `<tag a='say "hi"'/>`, `a='say "hi"'`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.QuoteStyle = tc.quoteStyle

			got := mustFormat(t, []byte(tc.input), cfg)

			if !bytes.Contains(got, []byte(tc.wantContains)) {
				t.Fatalf("output missing %q:\n%s", tc.wantContains, got)
			}

			if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
				t.Fatalf("attribute quoting is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func mustFormat(t *testing.T, src []byte, cfg *config.File) []byte {
	t.Helper()

	out, err := Format(src, cfg)
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}

	return out
}

func TestFormatKeepsTrailingSpaceInText(t *testing.T) {
	src := []byte("<root>\n    <t>line one   \nline two</t>\n</root>\n")

	cfg := config.Default()
	got := mustFormat(t, src, cfg)

	if !bytes.Contains(got, []byte("line one   \n")) {
		t.Fatalf("trailing spaces deleted from the text node:\n%q", got)
	}

	if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
	}
}

func TestCharacterDataIsByteExact(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"text content", "<project>\n    <sql>SELECT *\nFROM users;</sql>\n</project>\n"},
		{"cdata", "<root>\n    <d><![CDATA[line1\nline2]]></d>\n</root>\n"},
		{"nested three deep", "<root>\n    <a>\n        <b>\n            <desc>x\ny</desc>\n        </b>\n    </a>\n</root>\n"},
		{"xml:space with text", "<root>\n    <pre xml:space=\"preserve\">a\nb</pre>\n</root>\n"},
		{"xml:space with elements", "<root xml:space=\"preserve\">\n  <a>x</a>\n     <b>y</b>\n</root>\n"},
		{"trailing spaces in content", "<root>\n    <t>one   \ntwo</t>\n</root>\n"},
		{"no-break space between elements", "<root>\n    <a>x</a>\u00a0<b>y</b>\n</root>\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.src {
				t.Fatalf("character data was re-indented.\n=== in ===\n%q\n=== out ===\n%q", tc.src, got)
			}
		})
	}
}

func TestElementOnlyContentStillIndents(t *testing.T) {
	src := "<root>\n<a>x</a>\n<b><c>y</c></b>\n</root>\n"
	want := "<root>\n    <a>x</a>\n    <b>\n        <c>y</c>\n    </b>\n</root>\n"

	got := mustFormat(t, []byte(src), config.Default())

	if string(got) != want {
		t.Fatalf("element-only content stopped indenting.\n=== got ===\n%q\n=== want ===\n%q", got, want)
	}
}
