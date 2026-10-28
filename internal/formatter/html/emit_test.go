package html

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
)

func TestFormatMatchesGolden(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".html", true) {
		t.Run(name, func(t *testing.T) {
			input := formattertest.ReadFixture(t, name+".html")
			want := formattertest.ReadFixture(t, name+".formatted.html")

			got := mustFormat(t, input, config.Default())

			if !bytes.Equal(got, want) {
				t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
			}
		})
	}
}

func TestFormatIsIdempotent(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".html", true) {
		t.Run(name, func(t *testing.T) {
			input := formattertest.ReadFixture(t, name+".html")

			once := mustFormat(t, input, config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("Format is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
			}
		})
	}
}

func TestLongAttrsOverflowTriggersMultiline(t *testing.T) {
	cfg := config.Default()
	cfg.PrintWidth = 40

	input := formattertest.ReadFixture(t, "long_attrs_input.html")
	want := formattertest.ReadFixture(t, "long_attrs.formatted.html")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent under narrow printWidth.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestStyleMultiLineBodyIsIdempotent(t *testing.T) {
	src := []byte("<style>\n.a {\n/* line one\nline two */\ncontent: \"AAA\nBBB\";\n}\n</style>\n")

	once := mustFormat(t, src, config.Default())
	twice := mustFormat(t, once, config.Default())

	if !bytes.Equal(once, twice) {
		t.Fatalf("multi-line <style> body not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
	}
}

// These edge bytes are whitespace to unicode.IsSpace but content to the CSS
// parser.
func TestFormatKeepsRawTextEdgeBytesTheCSSParserWeighs(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"vertical tab keeps the body verbatim",
			"<stYle>;\n\v</stYle>\n",
			"<stYle>\n    ;\n    \v\n</stYle>\n",
		},
		{
			"vertical tab after a rule keeps it unformatted",
			"<style>a{color:red}\n\v</style>\n",
			"<style>\n    a{color:red}\n    \v\n</style>\n",
		},
		{
			"no-break space survives",
			"<style>\n \n</style>\n",
			"<style>\n     \n</style>\n",
		},
		{
			"an ordinary blank edge line is still dropped",
			"<style>\n\n  a{color:red}\n   \n</style>\n",
			"<style>\n    a {\n        color: red;\n    }\n</style>\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.want {
				t.Fatalf("Format output differs.\n=== got ===\n%q\n=== want ===\n%q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

// Expected break counts follow the HTML spec's input stream preprocessing,
// which turns a CRLF pair or a lone CR into one LF.
func TestFormatNormalizesLineTerminatorsInVerbatimBody(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"pre keeps the break a lone CR makes", "<pre>x\r</pre>\n", "<pre>x\n</pre>\n"},
		{"textarea keeps the break a lone CR makes", "<textarea>x\r</textarea>\n", "<textarea>x\n</textarea>\n"},
		{"a CRLF inside still normalizes", "<pre>x\r\ny\r</pre>\n", "<pre>x\ny\n</pre>\n"},
		{"two lone CRs are two breaks", "<pre>x\r\r</pre>\n", "<pre>x\n\n</pre>\n"},
		{"a CR run before a CRLF keeps every break", "<pre>x\r\r\n</pre>\n", "<pre>x\n\n</pre>\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.want {
				t.Fatalf("Format output differs.\n=== got ===\n%q\n=== want ===\n%q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestFormatReadsALoneCRAsItsLFTwin(t *testing.T) {
	cases := map[string]string{
		"text":                          "<p>a\rb</p>\n",
		"a blank line between children": "<div>\r<p>a</p>\r\r<p>b</p>\r</div>\n",
		"a multi-line attribute value":  "<div x-data=\"{\r  a: 1 }\"></div>\n",
		"a script body":                 "<script>\r  a();\r  b();\r</script>\n",
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())
			lf := strings.ReplaceAll(src, "\r", "\n")

			if want := mustFormat(t, []byte(lf), config.Default()); !bytes.Equal(got, want) {
				t.Fatalf("a lone CR formats unlike its LF twin.\n=== CR ===\n%q\n=== LF ===\n%q", got, want)
			}
		})
	}
}

func TestFormatWritesTextBreaksAsTheConfiguredLineEnding(t *testing.T) {
	got := mustFormat(t, []byte("<p>a\r\nb</p>\r\n"), config.Default())

	if bytes.IndexByte(got, '\r') >= 0 {
		t.Fatalf("a CR survived into LF output: %q", got)
	}
}

func TestFormatNeverEndsATagInAPHPCloseTag(t *testing.T) {
	cases := map[string]string{
		"an attribute name ending ?": "<p a? >x</p>\n",
		"an unquoted echo ending ?":  "<p a={{ $x }}? >x</p>\n",
		"a doctype ending ?":         "<!DOCTYPE html? >\n",
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if got := mustFormat(t, []byte(src), config.Default()); bytes.Contains(got, []byte("?>")) {
				t.Fatalf("the formatter wrote a `?>`: %q", got)
			}
		})
	}
}

func TestFormatTailwindEnabled(t *testing.T) {
	input := formattertest.ReadFixture(t, "tailwind_class_input.html")
	want := formattertest.ReadFixture(t, "tailwind_class.formatted.html")

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

func TestFormatTailwindTrimsClassValueEdges(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"several classes", `<div class=" p-4  flex "></div>`, `<div class="flex p-4"></div>`},
		{"one class", `<div class=" flex "></div>`, `<div class="flex"></div>`},
		{"whitespace only", `<div class="  "></div>`, `<div class=""></div>`},
	}

	cfg := config.Default()
	cfg.Tailwind = true

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src+"\n"), cfg)
			if string(got) != tc.want+"\n" {
				t.Fatalf("got %q, want %q", got, tc.want+"\n")
			}

			if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent:\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestFormatRejectsInvalidSource(t *testing.T) {
	out, err := Format([]byte("<!-- unclosed"), config.Default())

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
		wantAbsent   string
	}{
		{"auto canonicalizes to double", config.QuoteStyleAuto, "<div class='x'></div>", `class="x"`, `class='x'`},
		{"single forces single", config.QuoteStyleSingle, `<div class="x"></div>`, `class='x'`, `class="x"`},
		{"conflicting inner quote left untouched", config.QuoteStyleDouble, `<a title='say "hi"'></a>`, `title='say "hi"'`, ""},
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

func TestFormatKeepsATextBodyOnTheTagLineOnlyWhenWrittenThereAndFitting(t *testing.T) {
	long := strings.Repeat("word ", 25) + "end."
	link := `<a href="https://example.com/a/very/long/path/that/keeps/going/and/going/past/the/print/width" class="link link-primary" target="_blank">`

	cases := []struct {
		name string
		src  string
		want string
	}{
		{"author-broken long text", "<p>\n" + long + "\n</p>", "<p>\n    " + long + "\n</p>"},
		{"author-broken short text", "<p>\nHi\n</p>", "<p>\n    Hi\n</p>"},
		{"one-line text that overflows", "<p>" + long + "</p>", "<p>\n    " + long + "\n</p>"},
		{"short one-line text", "<p>Hi</p>", "<p>Hi</p>"},
		{"whitespace-only body", "<div>\n</div>", "<div></div>"},
		{
			"glued edge of a wrapped inline element",
			link + "Read more </a>",
			"<a\n    href=\"https://example.com/a/very/long/path/that/keeps/going/and/going/past/the/print/width\"\n    class=\"link link-primary\"\n" +
				"    target=\"_blank\"\n>Read more </a>",
		},
	}

	cfg := config.Default()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src+"\n"), cfg)
			if want := tc.want + "\n"; string(got) != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent:\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatClosesAParagraphBeforeDialogAndSearch(t *testing.T) {
	src := []byte("<div>\n<p>a\n<dialog>b</dialog>\n<p>c\n<search>d</search>\n</div>\n")
	want := "<div>\n    <p>\n        a\n    </p>\n    <dialog>b</dialog>\n    <p>\n        c\n    </p>\n    <search>d</search>\n</div>\n"

	got := mustFormat(t, src, config.Default())
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	twice := mustFormat(t, got, config.Default())
	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent: %q then %q", got, twice)
	}
}

func TestFormatKeepsUnquotedAttrValues(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"charset", "<meta charset=utf-8>\n", "<meta charset=\"utf-8\">\n"},
		{"input value", "<input type=text value=hello>\n", "<input type=\"text\" value=\"hello\">\n"},
		{"spaced equals", "<a href = x>y</a>\n", "<a href=\"x\">y</a>\n"},
		{"spaced equals with a quoted value", "<a href = \"x\">y</a>\n", "<a href=\"x\">y</a>\n"},
		{"bare equals before a quoted attribute", "<a rel= data-x=\"a b\">y</a>\n", "<a rel data-x=\"a b\">y</a>\n"},
		{"several on one tag", "<a href=x class=y id=z>t</a>\n", "<a href=\"x\" class=\"y\" id=\"z\">t</a>\n"},
		{"numeric", "<td colspan=2>c</td>\n", "<td colspan=\"2\">c</td>\n"},
		{"flag attribute keeps no value", "<a href>x</a>\n", "<a href>x</a>\n"},
		{"flag among unquoted", "<a rel=nofollow noopener>x</a>\n", "<a rel=\"nofollow\" noopener>x</a>\n"},
		{"empty value drops the equals", "<a href=>x</a>\n", "<a href>x</a>\n"},
		{"quoted is unchanged", "<a href=\"x\">y</a>\n", "<a href=\"x\">y</a>\n"},
		{"slash inside the value", "<a href=/docs>y</a>\n", "<a href=\"/docs\">y</a>\n"},
		{"slash-gt closes the tag", "<img src=a.png/>\n", "<img src=\"a.png\" />\n"},
		{"form feed separates attributes", "<a\fhref=x>y</a>\n", "<a href=\"x\">y</a>\n"},
	}

	cfg := config.Default()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), cfg)

			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			twice := mustFormat(t, got, cfg)

			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent: %q then %q", got, twice)
			}
		})
	}
}

func TestFormatMultiLineCommentKeepsItsShape(t *testing.T) {
	src := []byte("<div>\n    <section>\n        <!--\n      <p>legacy</p>\n    -->\n    </section>\n</div>\n")

	want := "<div>\n    <section>\n        <!--\n          <p>legacy</p>\n        -->\n    </section>\n</div>\n"

	cfg := config.Default()
	got := mustFormat(t, src, cfg)

	if string(got) != want {
		t.Fatalf("comment shape changed.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatKeepsTrailingSpaceInMultiLineAttrValue(t *testing.T) {
	src := []byte("<div>\n    <span title=\"one   \n  two   \n  three\">x</span>\n</div>\n")

	cfg := config.Default()
	got := mustFormat(t, src, cfg)

	if !bytes.Contains(got, []byte("one   \n")) || !bytes.Contains(got, []byte("two   \n")) {
		t.Fatalf("trailing spaces deleted from the attribute value:\n%q", got)
	}

	if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
	}
}

func TestFormatGluedInlineBodySurvivesWrappedOpenTag(t *testing.T) {
	long := "aaaaaaaaaaaa bbbbbbbbbbbb cccccccccccc dddddddddddd eeeeeeeeeeee ffffffffffff gggggggggggg hhhhhhhhhhhh"

	cfg := config.Default()

	for _, tag := range []string{"b", "span", "em", "code", "strong", "a", "small", "i", "mark"} {
		t.Run(tag, func(t *testing.T) {
			src := []byte("<p>head<" + tag + " class=\"" + long + "\">body</" + tag + ">tail</p>\n")

			got := mustFormat(t, src, cfg)

			if !bytes.Contains(got, []byte(">body</"+tag+">tail")) {
				t.Fatalf("glued inline body lost its hug when the open tag wrapped:\n%s", got)
			}

			if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}

	// A block element is not whitespace-sensitive, so its body still moves to its
	// own line when the tag wraps.
	block := mustFormat(t, []byte("<div><section class=\""+long+"\">body</section></div>\n"), cfg)
	if bytes.Contains(block, []byte(">body</section>")) {
		t.Fatalf("block element body was hugged:\n%s", block)
	}
}

func TestFormatKeepsInlineElementEdgeWhitespace(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"inline keeps both edges", "<p>foo<span> bar </span>baz</p>\n", "<p>foo<span> bar </span>baz</p>\n"},
		{"glued stays glued", "<p>foo<span>bar</span>baz</p>\n", "<p>foo<span>bar</span>baz</p>\n"},
		{"a run collapses to one space", "<p>foo<span>  bar  </span>baz</p>\n", "<p>foo<span> bar </span>baz</p>\n"},
		{"leading edge only", "<p>foo<span> bar</span>baz</p>\n", "<p>foo<span> bar</span>baz</p>\n"},
		{"trailing edge only", "<p>foo<span>bar </span>baz</p>\n", "<p>foo<span>bar </span>baz</p>\n"},
		{"block container still trims", "<div> block </div>\n", "<div>block</div>\n"},
		{"several inline siblings", "<p>x<b> y </b><em> z </em>w</p>\n", "<p>x<b> y </b><em> z </em>w</p>\n"},
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

func TestFormatKeepsGlueInMixedBlockAndInlineChildren(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"glued comma stays with its link",
			"<div>\n<h2>See also</h2>\n<a href=\"a.htm\">SetX</a>,\n<a href=\"b.htm\">GetY</a>\n</div>\n",
			"<div>\n    <h2>See also</h2>\n    <a href=\"a.htm\">SetX</a>,\n    <a href=\"b.htm\">GetY</a>\n</div>\n",
		},
		{
			"loose siblings still take a line each",
			"<div>\n<h2>T</h2>\n<a href=\"a\">X</a>\n<a href=\"b\">Y</a>\n</div>\n",
			"<div>\n    <h2>T</h2>\n    <a href=\"a\">X</a>\n    <a href=\"b\">Y</a>\n</div>\n",
		},
		{
			"glued element after text",
			"<div>\n<h2>T</h2>\nlead<a href=\"a\">X</a>\n</div>\n",
			"<div>\n    <h2>T</h2>\n    lead<a href=\"a\">X</a>\n</div>\n",
		},
		{
			"block children are unaffected",
			"<div>\n<h2>T</h2>\n<p>para</p>\n</div>\n",
			"<div>\n    <h2>T</h2>\n    <p>para</p>\n</div>\n",
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

// Form controls and embedded content are inline-level per the rendering
// section of the HTML standard.
func TestFormatTreatsInlineLevelElementsAsInline(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"input", "<p><input>:<input></p>\n", "<p><input>:<input></p>\n"},
		{"button", "<li>aaaa<button type=\"button\">press</button>tail</li>\n", "<li>aaaa<button type=\"button\">press</button>tail</li>\n"},
		{"progress", "<p>a<progress value=\"1\"></progress>b</p>\n", "<p>a<progress value=\"1\"></progress>b</p>\n"},
		{"select", "<p>a<select><option>x</option></select>b</p>\n", "<p>a<select><option>x</option></select>b</p>\n"},
		{"embedded video", "<p>a<video src=\"v.mp4\"></video>b</p>\n", "<p>a<video src=\"v.mp4\"></video>b</p>\n"},
		{"glued around a control", "<p>SetX<input>, GetY</p>\n", "<p>SetX<input>, GetY</p>\n"},
		// A block element still forces the structural path, and a line break beside
		// a block box is not painted, so its glue is free to break.
		{"block child still explodes", "<div>a<h2>T</h2>b</div>\n", "<div>\n    a\n    <h2>T</h2>\n    b\n</div>\n"},
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

// In HTML's attribute-name state `"`, `'` and `<` are parse errors that still
// append to the name, so `<button a=0 "">` has two attributes.
func TestQuoteNamedAttrIsNotDropped(t *testing.T) {
	cases := []string{
		`<button a=0 ""></button>` + "\n",
		`<button a=0 'x'></button>` + "\n",
		`<button a=0 b=1></button>` + "\n",
		`<button a=x y></button>` + "\n",
		`<button a=0""></button>` + "\n",
	}

	for _, src := range cases {
		t.Run(strings.TrimSpace(src), func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			for _, tok := range []string{`""`, `'x'`, "b=", "y", "a"} {
				if strings.Contains(src, tok) && !bytes.Contains(got, []byte(tok)) {
					t.Fatalf("attribute %q was dropped:\n%q", tok, got)
				}
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestGluedCommentAfterVerbatimContentIsAFixpoint(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"pre", "<div>\n<pre>a\n  b</pre><!-- c1\n  c2 -->\n</div>\n"},
		{"textarea", "<div>\n<textarea>a\n  b</textarea><!-- c1\n  c2 -->\n</div>\n"},
		{"pre with tabs", "<div>\n<pre>a\n\tb</pre><!-- c1\n\tc2 -->\n</div>\n"},
		{"pre single-line comment", "<div>\n<pre>a\n  b</pre><!-- c -->\n</div>\n"},
		{"comment on its own line", "<div>\n<pre>a\n  b</pre>\n<!-- c1\n  c2 -->\n</div>\n"},
		{"ordinary element", "<div>\n<b>x</b><!-- c1\n  c2 -->\n</div>\n"},
		{"script", "<div>\n<script>a\n  b</script><!-- c1\n  c2 -->\n</div>\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			for i := 2; i <= 6; i++ {
				next := mustFormat(t, got, config.Default())
				if !bytes.Equal(next, got) {
					t.Fatalf("pass %d changed the output.\n=== pass %d ===\n%q\n=== pass %d ===\n%q",
						i, i-1, got, i, next)
				}

				got = next
			}
		})
	}
}

func TestTopLevelGlueIsPreserved(t *testing.T) {
	glued := []string{
		"a<b>x</b>c\n",
		"pre<span>x</span>post\n",
		"pre<br />post\n",
		"a<!-- c -->b\n",
	}

	for _, src := range glued {
		t.Run(strings.TrimSpace(src), func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if bytes.Contains(bytes.TrimRight(got, "\n"), []byte("\n")) {
				t.Fatalf("a glued boundary gained a line break:\n%q", got)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestVoidElementKeepsTheAuthorsSlash(t *testing.T) {
	const wide = "src=\"a-rather-long-source-path-that-pushes-this-tag-over-the-print-width.png\" " +
		"alt=\"some alternative text that is also long\" loading=\"lazy\""

	cases := []struct {
		name string
		src  string
		want string
	}{
		{"slashed", "<div><img src=\"a.png\" /></div>", "<img src=\"a.png\" />"},
		{"bare", "<div><img src=\"a.png\"></div>", "<img src=\"a.png\">"},
		{"slashed, attributes broken", "<div><img " + wide + " /></div>", "\n    />"},
		{"bare, attributes broken", "<div><img " + wide + "></div>", "\n    >"},
		{"both spellings in one document", "<div><br /><br></div>", "<br /><br>"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(mustFormat(t, []byte(tc.src), config.Default()))

			if !strings.Contains(got, tc.want) {
				t.Fatalf("void spelling not preserved: want %q in\n%s", tc.want, got)
			}

			if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

// The division line makes the script lexer decline, so the body goes out
// verbatim.
func TestVerbatimBodyKeepsTrailingWhitespace(t *testing.T) {
	cases := []struct {
		name string
		src  string
		keep string
	}{
		{"template literal in a declined script", "<script>\nconst half = (a + b) / 2;\nconst t = `two  \nspaces`;\n</script>\n", "`two  \n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if !strings.Contains(string(got), tc.keep) {
				t.Fatalf("lost the trailing whitespace of %q:\n%q", tc.keep, got)
			}
		})
	}
}

func TestMultiLineTextIsWrittenLineByLine(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"continuation line follows the indent",
			"<section>\n<div>\n<p>x</p>\nline one\n      line two\n</div>\n</section>\n",
			"<section>\n    <div>\n        <p>x</p>\n        line one\n        line two\n    </div>\n</section>\n",
		},
		{
			"frozen text after an ignore comment moves with its line once",
			"0\n 0<!--laravel-fmt-ignore-->0\n  0",
			"0\n0<!--laravel-fmt-ignore-->0\n 0\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent: %q then %q", got, twice)
			}
		})
	}
}

func TestFormatNeverTurnsAnAuthoredSpaceIntoALineBreak(t *testing.T) {
	prose := "This sentence was written on one line and it runs well past the default print width of one hundred and twenty."
	mixed := "Some <b>bold</b> words and <a href=\"#\">a link</a> in a run of words long enough to overflow the default print width."

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"inline elements among words",
			"<p>" + mixed + "</p>\n",
			"<p>\n    " + mixed + "\n</p>\n",
		},
		{
			"an authored break stays where it was",
			"<p>\n    Some <b>bold</b>\n    " + prose + "\n</p>\n",
			"<p>\n    Some <b>bold</b>\n    " + prose + "\n</p>\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatInlineRunKeepsAnAuthoredBlankLine(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"after a glued element",
			"<div>\n    a<br/> x\n\n    y\n</div>\n",
			"<div>\n    a<br /> x\n\n    y\n</div>\n",
		},
		{
			"after a glued word",
			"<p>\n    a <b>x</b>z\n\n    y\n</p>\n",
			"<p>\n    a <b>x</b>z\n\n    y\n</p>\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.in), config.Default())

			if string(got) != tc.want {
				t.Fatalf("blank line not kept.\n=== got ===\n%s\n=== want ===\n%s", got, tc.want)
			}

			twice := mustFormat(t, got, config.Default())
			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatSpacesAlpineNotOperatorOnlyWithNotOperatorSpace(t *testing.T) {
	src := []byte("<button @click=\"open = ! open\">Toggle</button>\n")

	cases := []struct {
		notOperatorSpace bool
		want             string
	}{
		{true, "<button @click=\"open = ! open\">Toggle</button>\n"},
		{false, "<button @click=\"open = !open\">Toggle</button>\n"},
	}

	for _, c := range cases {
		cfg := config.Default()
		cfg.PHP.NotOperatorSpace = c.notOperatorSpace

		if got := mustFormat(t, src, cfg); string(got) != c.want {
			t.Errorf("NotOperatorSpace=%v: got %q, want %q", c.notOperatorSpace, got, c.want)
		}
	}
}
