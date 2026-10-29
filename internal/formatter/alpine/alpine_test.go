package alpine

import (
	"bytes"
	"fmt"
	"testing"
)

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
			before, ok := lex(input)

			if !ok {
				t.Fatalf("lex(%q) failed", src)
			}

			out, ok := Normalize(input, true)

			if !ok {
				t.Fatalf("Normalize(%q) refused", src)
			}

			after, ok := lex(out)

			if !ok {
				t.Fatalf("output %q does not lex", out)
			}

			if len(after) != len(before) {
				t.Fatalf("token count changed from %d to %d in %q", len(before), len(after), out)
			}

			for i, b := range before {
				a := after[i]

				if a.kind != b.kind || !bytes.Equal(input[b.start:b.end], out[a.start:a.end]) {
					t.Fatalf("token %d changed from %q to %q", i, input[b.start:b.end], out[a.start:a.end])
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
		{"multi-line value", "{\n open: false }"},
		{"blade directive", "toasterHub(@js($toasts))"},
		{"escaped blade echo", "@{{ count }}"},
		{"named html entity", "a &amp;&amp; b"},
		{"numeric html entity", "x &#39; y"},
		{"unterminated string", "'open"},
		{"unterminated template", "`a ${open"},
		{"unterminated block comment", "a /* b"},
		{"unterminated regex", "x = /ab"},
		{"exponent without a digit", "x = 1e+"},
		{"slash after brace", "{} /re/.test(s)"},
		// A `)` closing a control-flow head is followed by a regex, not a
		// division: spacing `if (b) /b/.test(b)` would change it from true to
		// false.
		{"slash after paren", "if (b) /b/.test(b)"},
		{"unknown byte", "a # b"},
		{"identifier glued to number", "2px"},
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
	for _, in := range []string{"a<!--b", "x<!-- rest", "count<!--", "a<!--"} {
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
