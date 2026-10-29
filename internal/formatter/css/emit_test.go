package css

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/css"
)

func TestFormatMatchesGolden(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".css", true) {
		t.Run(name, func(t *testing.T) {
			input := formattertest.ReadFixture(t, name+".css")
			want := formattertest.ReadFixture(t, name+".formatted.css")

			got := mustFormat(t, input, config.Default())

			if !bytes.Equal(got, want) {
				t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
			}
		})
	}
}

func TestFormatIsIdempotent(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".css", true) {
		t.Run(name, func(t *testing.T) {
			input := formattertest.ReadFixture(t, name+".css")

			once := mustFormat(t, input, config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("Format is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
			}
		})
	}
}

func TestEmitStyleBodyRejectsForeignAtRule(t *testing.T) {
	cfg := config.Default()

	if EmitStyleBody(formatter.NewWriter(cfg), []byte("@if ($x) .a { color: red; }"), cfg) {
		t.Fatal("EmitStyleBody accepted a Blade @if directive as CSS")
	}

	w := formatter.NewWriter(cfg)
	if !EmitStyleBody(w, []byte("@media (min-width: 1px) { .a { color: red; } }\n@page { margin: 0; }\n"), cfg) {
		t.Fatal("EmitStyleBody declined pure CSS at-rules")
	}

	out := w.Bytes()
	if !bytes.Contains(out, []byte("@media")) || !bytes.Contains(out, []byte("@page")) {
		t.Fatalf("CSS at-rules lost:\n%s", out)
	}
}

func TestFormatTailwindEnabled(t *testing.T) {
	input := formattertest.ReadFixture(t, "tailwind_apply_input.css")
	want := formattertest.ReadFixture(t, "tailwind_apply.formatted.css")

	cfg := config.Default()
	cfg.Tailwind = true

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("Format is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatRejectsInvalidSource(t *testing.T) {
	out, err := Format([]byte(".x { color: red"), config.Default())

	if err == nil {
		t.Fatal("Format accepted invalid source; want a parse error")
	}

	if out != nil {
		t.Fatalf("Format returned non-nil bytes on parse error: %q", out)
	}
}

func TestSelectorSpacing(t *testing.T) {
	cases := []struct {
		name         string
		input        string
		wantContains string
	}{
		{"child combinator spaced", "a>b{color:red}", "a > b"},
		{"adjacent sibling spaced", "a+b{color:red}", "a + b"},
		{"general sibling spaced", "a~b{color:red}", "a ~ b"},
		{"descendant collapses to one space", "a    b{color:red}", "a b"},
		{"attribute string spaces preserved", `[title="a  b"]{color:red}`, `[title="a  b"]`},
		{"nth-child plus untouched", "a:nth-child(2n+1){color:red}", "a:nth-child(2n+1)"},
		{"form feed is whitespace", "a\f>\fb{color:red}", "a > b"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.input), config.Default())

			if !bytes.Contains(got, []byte(tc.wantContains)) {
				t.Fatalf("output missing %q:\n%s", tc.wantContains, got)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("selector spacing is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestValueSpacing(t *testing.T) {
	cases := []struct {
		name         string
		input        string
		wantContains string
		wantAbsent   string
	}{
		{"function arg commas spaced", "a{color:rgba(0,0,0,.5)}", "rgba(0, 0, 0, .5)", ""},
		{"calc division spaced", "a{width:calc(100%/3)}", "calc(100% / 3)", ""},
		{"calc multiplication spaced", "a{width:calc(1px*2)}", "calc(1px * 2)", ""},
		{"form feed is whitespace", "a{width:calc(1px\f*\f2)}", "calc(1px * 2)", ""},
		{"calc minus left as written", "a{width:calc(100% - 10px)}", "calc(100% - 10px)", ""},
		{"font shorthand slash preserved", "a{font:16px/1.5 sans-serif}", "16px/1.5", "16px / 1.5"},
		{"aspect-ratio slash preserved", "a{aspect-ratio:16/9}", "aspect-ratio: 16/9", "16 / 9"},
		{"url slash preserved", "a{background:url(a/b.png)}", "url(a/b.png)", "a / b"},
		{"custom property identifier preserved", "a{color:var(--accent-color)}", "var(--accent-color)", ""},
		{"calc comment kept whole", "a{width:calc(100% - /* gutter */ 2rem)}", "calc(100% - /* gutter */ 2rem)", "/ * gutter"},
		{"clamp comment kept whole", "a{font-size:clamp(1rem, /* fluid */ 2vw, 3rem)}", "clamp(1rem, /* fluid */ 2vw, 3rem)", "* /"},
		{"unspaced comment in math kept whole", "a{width:max(/*x*/0)}", "max(/*x*/0)", "/ *"},
		{"quote inside math comment kept whole", `a{width:max(/*"*/0)}`, `max(/*"*/0)`, "/ *"},
		{"comment between math operands", "a{width:calc(/*a*/100%/*b*/ - 1px)}", "calc(/*a*/100%/*b*/ - 1px)", "* /"},
		{
			"data uri comma not spaced",
			"a{background:url(data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=) no-repeat}",
			"url(data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=)",
			"base64, ",
		},
		{"path comma not spaced", "a{background:url(/img/a,b.png)}", "url(/img/a,b.png)", "a, b"},
		{"query comma not spaced", "a{src:url(font.woff2?a=1,b=2)}", "url(font.woff2?a=1,b=2)", "1, b"},
		{"quoted data uri untouched", `a{background:url("data:image/svg+xml;base64,PHN2Zz4=")}`, `url("data:image/svg+xml;base64,PHN2Zz4=")`, "base64, "},
		{"quoted url keeps arg spacing elsewhere", "a{background:rgba(0,0,0,.5)}", "rgba(0, 0, 0, .5)", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.input), config.Default())

			if !bytes.Contains(got, []byte(tc.wantContains)) {
				t.Fatalf("output missing %q:\n%s", tc.wantContains, got)
			}

			if tc.wantAbsent != "" && bytes.Contains(got, []byte(tc.wantAbsent)) {
				t.Fatalf("output unexpectedly contains %q:\n%s", tc.wantAbsent, got)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("value spacing is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestAtRulePreludeSpacing(t *testing.T) {
	cases := []struct {
		name         string
		input        string
		wantContains string
		wantAbsent   string
	}{
		{"media feature colon spaced", "@media(min-width:100px){a{x:1}}", "@media (min-width: 100px)", ""},
		{"supports feature colon spaced", "@supports(display:grid){a{x:1}}", "@supports (display: grid)", ""},
		{"media and-condition spaced", "@media screen and (min-width:100px){a{x:1}}", "screen and (min-width: 100px)", ""},
		{"page pseudo not spaced", "@page :first{margin:0}", "@page :first", "@page : first"},
		{"selector function pseudo preserved", "@supports selector(a:hover){a{x:1}}", "selector(a:hover)", "selector(a: hover)"},
		{"layer list comma spaced", "@layer a,b;", "@layer a, b", ""},
		{"form feed is whitespace", "@media screen\fand\f(min-width:100px){a{x:1}}", "screen and (min-width: 100px)", ""},
		{"import url comma not spaced", "@import url(a,b.css);", "url(a,b.css)", "a, b"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.input), config.Default())

			if !bytes.Contains(got, []byte(tc.wantContains)) {
				t.Fatalf("output missing %q:\n%s", tc.wantContains, got)
			}

			if tc.wantAbsent != "" && bytes.Contains(got, []byte(tc.wantAbsent)) {
				t.Fatalf("output unexpectedly contains %q:\n%s", tc.wantAbsent, got)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("prelude spacing is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestQuoteStyleNormalization(t *testing.T) {
	cases := []struct {
		name         string
		quoteStyle   string
		input        string
		wantContains string
		wantAbsent   string
	}{
		{"auto canonicalizes to double", config.QuoteStyleAuto, "a { content: 'x'; }", `content: "x"`, `'x'`},
		{"explicit single forces single", config.QuoteStyleSingle, `a { content: "x"; }`, `content: 'x'`, `"x"`},
		{"url normalized", config.QuoteStyleAuto, "a { background: url('a.png'); }", `url("a.png")`, ""},
		{"conflicting inner quote left untouched", config.QuoteStyleDouble, `a { content: 'say "hi"'; }`, `'say "hi"'`, ""},
		{"escaped quote left untouched", config.QuoteStyleDouble, "a { content: 'a\\'b'; }", "'a\\'b'", ""},
		{"import normalized", config.QuoteStyleAuto, "@import 'x.css';", `@import "x.css"`, ""},
		{"charset left untouched", config.QuoteStyleSingle, `@charset "utf-8";`, `@charset "utf-8"`, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.QuoteStyle = tc.quoteStyle

			got := mustFormat(t, []byte(tc.input), cfg)

			if !bytes.Contains(got, []byte(tc.wantContains)) {
				t.Fatalf("output missing %q:\n%s", tc.wantContains, got)
			}

			if tc.wantAbsent != "" && bytes.Contains(got, []byte(tc.wantAbsent)) {
				t.Fatalf("output unexpectedly contains %q:\n%s", tc.wantAbsent, got)
			}

			if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
				t.Fatalf("quote normalization is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
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

func TestFormatMultiLineCommentKeepsItsShape(t *testing.T) {
	// The block is re-homed under the Writer's indent, so its absolute columns
	// move; only the 2-column step between the continuation lines must survive.
	src := []byte("a {\n    /* one\n  two\n    three */\n    color: red;\n}\n")

	want := "a {\n    /* one\n    two\n      three */\n    color: red;\n}\n"

	cfg := config.Default()
	got := mustFormat(t, src, cfg)

	if string(got) != want {
		t.Fatalf("comment shape changed.\n=== got ===\n%q\n=== want ===\n%q", got, want)
	}

	if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
	}
}

func TestFormatAcceptsEscapesInSelectorsAndValues(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"escaped brace in a class", `.a\} { color: red; }` + "\n", `.a\} {` + "\n    color: red;\n}\n"},
		{"escaped semicolon in a value", `:root{--semi: a\;b;}` + "\n", ":root {\n    " + `--semi: a\;b;` + "\n}\n"},
		{"tailwind fraction", `.w-1\/2{width:50%}` + "\n", `.w-1\/2 {` + "\n    width: 50%;\n}\n"},
		{"escaped comma in a class", `.a\,b{color:red}` + "\n", `.a\,b {` + "\n    color: red;\n}\n"},
		{"escaped combinators in a class", `.a\+b, .c\>d, .e\~f{color:red}` + "\n", `.a\+b,` + "\n" + `.c\>d,` + "\n" + `.e\~f {` + "\n    color: red;\n}\n"},
		{"escaped space before a comma", `.a\ ,.b{color:red}` + "\n", `.a\ ,` + "\n.b {\n    color: red;\n}\n"},
		{"escaped space before a descendant", `.a\  b{color:red}` + "\n", `.a\  b {` + "\n    color: red;\n}\n"},
		{"hex escape keeps its terminating space", `.\31  a{color:red}` + "\n", `.\31  a {` + "\n    color: red;\n}\n"},
		{"combinator after a hex escape", `.\31+a{color:red}` + "\n", `.\31 + a {` + "\n    color: red;\n}\n"},
		{"escaped paren does not hide a comma", `.a\(,.b{color:red}` + "\n", `.a\(,` + "\n.b {\n    color: red;\n}\n"},
		{"escaped comma in a value", `:root{--label: a\,b;}` + "\n", ":root {\n    " + `--label: a\,b;` + "\n}\n"},
		{"escaped comma in a prelude", `@layer a\,b;` + "\n", `@layer a\,b;` + "\n"},
		{
			"tailwind arbitrary url value",
			`.bg-\[url\(\'data\:image\/svg\+xml\'\)\]{color:red}` + "\n",
			`.bg-\[url\(\'data\:image\/svg\+xml\'\)\] {` + "\n    color: red;\n}\n",
		},
	}

	cfg := config.Default()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), cfg)

			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent: %q then %q", got, twice)
			}
		})
	}
}

func TestEscapedQuoteDoesNotOpenAString(t *testing.T) {
	const bs = `\`

	cases := []string{
		":" + bs + "'''",
		":" + bs + "'''0",
		"a { content: '" + bs + "''; }\n",
		`a { content: "\"x"; }` + "\n",
		"a { content: '" + bs + "'' 'y'; }\n",
		"a { content: 'plain'; }\n",
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())

			if twice := mustFormat(t, once, config.Default()); !bytes.Equal(twice, once) {
				t.Fatalf("not a fixpoint.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}

			if q := unescapedQuoteParity(once); q {
				t.Fatalf("an unescaped quote was left unpaired:\n%q", once)
			}
		})
	}
}

func unescapedQuoteParity(b []byte) bool {
	var open byte

	for i := 0; i < len(b); i++ {
		switch {
		case b[i] == '\\':
			i++
		case open != 0 && b[i] == open:
			open = 0
		case open == 0 && (b[i] == '\'' || b[i] == '"'):
			open = b[i]
		}
	}

	return open != 0
}

func TestTrailingEscapeDoesNotSwallowTheSemicolon(t *testing.T) {
	const bs = `\`

	cases := []string{
		"a { b: " + bs + " }\n",
		"a { b: c" + bs + " }\n",
		"a { b: " + bs + " ; }\n",
		"a { b: c" + bs + bs + " }\n",
		"a { b: c }\n",
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())

			if twice := mustFormat(t, once, config.Default()); !bytes.Equal(twice, once) {
				t.Fatalf("not a fixpoint.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}

			// An odd run of backslashes before a `;` or `:` escapes it into
			// the token before; an even run is an escaped backslash.
			for i, c := range once {
				if c != ';' && c != ':' {
					continue
				}

				runLen := 0
				for j := i - 1; j >= 0 && once[j] == '\\'; j-- {
					runLen++
				}

				if runLen%2 == 1 {
					t.Fatalf("the terminating semicolon was escaped into the value:\n%q", once)
				}
			}
		})
	}
}

type unknownNode struct{ span parser.Span }

func (n unknownNode) Span() parser.Span {
	return n.span
}

func TestEmitNodeKeepsAnUnknownNodeVerbatim(t *testing.T) {
	src := []byte("@future thing;")
	node := unknownNode{span: parser.Span{
		Start: parser.Position{Offset: 0, Line: 1},
		End:   parser.Position{Offset: len(src), Line: 1},
	}}

	w := formatter.NewWriter(config.Default())
	emitNode(w, src, node, config.Default())

	if got := w.Bytes(); !bytes.Equal(got, src) {
		t.Fatalf("got %q, want %q", got, src)
	}
}
