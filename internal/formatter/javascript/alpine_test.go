package javascript

import (
	"bytes"
	"fmt"
	"testing"

	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/javascript"
)

var corpus = []string{
	"{ open: false }",
	"{open:false,toggle(){this.open=!this.open}}",
	"dropdown(true)",
	"open = !open",
	"count++",
	"--count",
	"items.filter(i => i.active).map(i => i.id)",
	"{ get isOpen() { return this.open }, set value(v) { this.v = v } }",
	"`Hello ${name} (${count})`",
	"{ msg: `a ${x ? 'y' : 'z'} b` }",
	"/^a[/b]+$/i.test(value)",
	"total / count / 2",
	"await fetch('/api/items')",
	"x = 0xFF_AA + 1_000n - .5e-2",
	"a?.b?.[c] ?? d",
	"open = false; $dispatch('closed'); count += 1",
	"x ? a : b ? c : d",
	"x ? { a: 1 } : { b: 2 }",
	"{ [key]: value, ...rest }",
	"!visible && items.length > 0",
	"tab === 'home' || tab === 'profile'",
	"new Date(2026, 0, 1).getTime()",
	"typeof value === 'string'",
	"s.replace(/\\s+/g, ' ')",
	"run(a, /* keep */ b)",
	"for (let i = 0; i < 10; i++) { step(i) }",
	"msg = 'it\\'s here'",
	"name = \"say \\\"hi\\\"\"",
	"(item, index) in items",
	"setTimeout(() => ({ done: true }), 50)",
	"a + +b - -c",
	"flags = mask & ~dirty | seen ^ bit",
}

func TestNormalizeCanonicalSpacing(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"flat object", "{open:false,toggle(){this.open=!this.open}}", "{ open: false, toggle() { this.open = ! this.open } }"},
		{"binary and unary", "a&&b||!c", "a && b || ! c"},
		{"ternary vs key colon", "{label:x?'on':'off'}", "{ label: x ? 'on' : 'off' }"},
		{"keyword before paren", "if(x){y=1}", "if (x) { y = 1 }"},
		{"call stays tight", "toggle( a , b )", "toggle(a, b)"},
		{"statement list", "open=false;$dispatch('closed')", "open = false; $dispatch('closed')"},
		{"arrow function", "items.filter( i=>i.active )", "items.filter(i => i.active)"},
		{"template interior untouched", "`a  ${ x }  b`", "`a  ${ x }  b`"},
		{"string interior untouched", "'a  b'+x", "'a  b' + x"},
		{"unary minus after assign", "x=-1", "x = -1"},
		{"postfix increment tight", "count ++", "count++"},
		{"optional chain tight", "a ?. b ?? c", "a?.b ?? c"},
		{"spread tight", "f( ...args )", "f(...args)"},
		{"surrounding whitespace trimmed", "  open = !open  ", "open = ! open"},
		{"division spaced", "total /count/ 2", "total / count / 2"},
		{"regex interior untouched", "s.replace(/  +/g,' ')", "s.replace(/  +/g, ' ')"},
		{"await call", "posts=await fetch('/posts')", "posts = await fetch('/posts')"},
		{"empty brackets tight", "f({}, [])", "f({}, [])"},
		{"fuse-risk space kept", "a - -b", "a - -b"},
		// Alpine matches x-for with a regex requiring whitespace after `in`, so
		// gluing the bracket to it makes the loop render nothing.
		{"keyword before bracket", "i in [1,2]", "i in [1, 2]"},
		{"destructured x-for", "(i,k) in [...Array(3).keys()]", "(i, k) in [...Array(3).keys()]"},
		{"typeof before bracket", "typeof [1]", "typeof [1]"},
		{"index stays tight", "items[ 0 ]", "items[0]"},
		{
			"compound operators stay whole",
			"a??=b;c**=2;d===e;f!==g;h&&=i;j||=k;l<<=1;m>>=2;n>>>=3;o>>>p;s%=2;t^=1;u&=1;v|=1;w<=x;y>=z",
			"a ??= b; c **= 2; d === e; f !== g; h &&= i; j ||= k; l <<= 1; m >>= 2; n >>>= 3; o >>> p; s %= 2; t ^= 1; u &= 1; v |= 1; w <= x; y >= z",
		},
		{"question dot before a digit is a ternary", "x=a?.5:1", "x = a ? .5 : 1"},
		{"declaration keyword before a bracket", "for (const [k,v] of e) {}", "for (const [k, v] of e) {}"},
		{"decrement before a greater-than stays apart", "-- >b", "-- > b"},
		{"ASI guard starting a line stays glued", "a=b\n;[c,d]=e", "a = b\n;[c, d] = e"},
		{"line breaks and their indentation kept", "{\n    open:false,\n        close(){}\n}", "{\n    open: false,\n        close() {}\n}"},
		{"value opening and closing with a line break", "\n  a=1\n  b=2\n", "\n  a = 1\n  b = 2\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := Normalize([]byte(c.in), true)

			if !ok {
				t.Fatalf("Normalize(%q) refused, want %q", c.in, c.want)
			}

			if string(got) != c.want {
				t.Fatalf("Normalize(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestNormalizeSpacesTheNotOperatorOnlyWhenAsked(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		spaced string
		glued  string
	}{
		{"glued operand", "open=!open", "open = ! open", "open = !open"},
		{"before a group", "!(a&&b)", "! (a && b)", "!(a && b)"},
		{"boolean cast", "!!open", "!!open", "!!open"},
		{"strict inequality", "a!==b", "a !== b", "a !== b"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			settings := []struct {
				spaceAfterNot bool
				want          string
			}{
				{true, c.spaced},
				{false, c.glued},
			}

			for _, s := range settings {
				got, ok := Normalize([]byte(c.in), s.spaceAfterNot)
				if !ok || string(got) != s.want {
					t.Fatalf("Normalize(%q, %v) = %q, %v; want %q", c.in, s.spaceAfterNot, got, ok, s.want)
				}
			}
		})
	}
}

func TestNormalizeIsIdempotent(t *testing.T) {
	for _, spaceAfterNot := range []bool{true, false} {
		for _, src := range corpus {
			t.Run(fmt.Sprint(spaceAfterNot, "/", src), func(t *testing.T) {
				once, ok := Normalize([]byte(src), spaceAfterNot)

				if !ok {
					t.Fatalf("Normalize(%q) refused; corpus entries must normalize", src)
				}

				twice, ok := Normalize(once, spaceAfterNot)

				if !ok {
					t.Fatalf("Normalize refused its own output %q", once)
				}

				if !bytes.Equal(twice, once) {
					t.Fatalf("not idempotent: %q became %q", once, twice)
				}
			})
		}
	}
}

func TestNormalizePreservesTokenStream(t *testing.T) {
	for _, src := range corpus {
		t.Run(src, func(t *testing.T) {
			input := []byte(src)
			before, ok := parser.Lex(input)

			if !ok {
				t.Fatalf("Lex(%q) failed", src)
			}

			out, ok := Normalize(input, true)

			if !ok {
				t.Fatalf("Normalize(%q) refused", src)
			}

			after, ok := parser.Lex(out)

			if !ok {
				t.Fatalf("output %q does not lex", out)
			}

			if len(after) != len(before) {
				t.Fatalf("token count changed from %d to %d in %q", len(before), len(after), out)
			}

			for i, b := range before {
				a := after[i]

				if a.Kind != b.Kind || !bytes.Equal(input[b.Start:b.End], out[a.Start:a.End]) {
					t.Fatalf("token %d changed from %q to %q", i, input[b.Start:b.End], out[a.Start:a.End])
				}
			}
		})
	}
}

func TestNormalizeRefusesUnsafeValues(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"blade directive", "toasterHub(@js($toasts))"},
		{"escaped blade echo", "@{{ count }}"},
		{"blade comment", "a = 1 {{-- why --}}"},
		{"raw blade echo", "{ a: {!! $a !!} }"},
		{"named html entity", "a &amp;&amp; b"},
		{"numeric html entity", "x &#39; y"},
		{"value that does not lex", "'open"},
		{"empty value", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if out, ok := Normalize([]byte(c.in), true); ok {
				t.Fatalf("Normalize(%q) = %q, want refusal", c.in, out)
			}
		})
	}
}

func TestIsAlpineJSAttrAllowlist(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"x-data", true},
		{"X-DATA", true},
		{"x-init", true},
		{"x-show", true},
		{"x-effect", true},
		{"x-for", true},
		{"x-id", true},
		{"x-model.lazy", true},
		{"x-on:click", true},
		{"x-on:keydown.window.ctrl.1.prevent", true},
		{"x-bind:class", true},
		{"@click", true},
		{"@click.prevent", true},
		{"class", false},
		{":class", false},
		{"x-ref", false},
		{"x-teleport", false},
		{"x-transition:enter", false},
		{"x-transition.duration.500ms", false},
		{"x-mask", false},
		{"x-intersect", false},
		{"x-cloak", false},
		{"x-on:", false},
		{"x-bind:", false},
		{"@", false},
		{"wire:model", false},
		{"", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsAlpineJSAttr([]byte(c.name)); got != c.want {
				t.Fatalf("IsAlpineJSAttr(%q) = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

func TestNormalizeRefusesEveryJSWhitespace(t *testing.T) {
	// ECMAScript WhiteSpace and LineTerminator, minus the four isSpace handles.
	runes := []rune{
		'\v', '\f',
		0x00A0, 0x1680,
		0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005,
		0x2006, 0x2007, 0x2008, 0x2009, 0x200A,
		0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF,
	}

	for _, r := range runes {
		t.Run(fmt.Sprintf("U+%04X", r), func(t *testing.T) {
			for _, tmpl := range []string{"return%c/re/.test(x)", "i%cin [1, 2]", "a%cb"} {
				in := fmt.Sprintf(tmpl, r)

				if out, ok := Normalize([]byte(in), true); ok {
					t.Fatalf("Normalize(%q) = %q, want refusal", in, out)
				}
			}
		})
	}
}

func TestNormalizeStillRunsOnASCIIWhitespace(t *testing.T) {
	cases := map[string]string{
		"return /re/.test(x)": "return /re/.test(x)",
		"i in [1, 2]":         "i in [1, 2]",
		"{open:false}":        "{ open: false }",
		"a\tb":                "a b",
	}

	for in, want := range cases {
		out, ok := Normalize([]byte(in), true)
		if !ok {
			t.Fatalf("Normalize(%q) refused; want %q", in, want)
		}

		if string(out) != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, out, want)
		}
	}
}

func TestNormalizeRefusesHTMLLikeComment(t *testing.T) {
	// `a < !--b` is deliberately absent: with the space it holds no `<!--` at all
	// and really is a comparison against a decrement, so normalizing it is correct.
	for _, in := range []string{"a<!--b", "x<!-- rest", "count<!--", "a<!--", "a\n--> rest"} {
		if out, ok := Normalize([]byte(in), true); ok {
			t.Errorf("Normalize(%q) = %q, want refusal", in, out)
		}
	}
}

func TestBreaksInTemplate(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{"break inside a template", "{ msg: `one\ntwo` }", true},
		{"break inside a substitution's template", "{ a: `x${`y\nz`}` }", true},
		{"break between tokens", "{\n  msg: `one`,\n}", false},
		{"no template", "{\n  open: false\n}", false},
		{"untokenizable with a backtick", "{ a: 'x\ny', b: `z` }", true},
		{"untokenizable without one", "{ a: 'x\ny' }", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BreaksInTemplate([]byte(tc.value)); got != tc.want {
				t.Fatalf("BreaksInTemplate(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}
