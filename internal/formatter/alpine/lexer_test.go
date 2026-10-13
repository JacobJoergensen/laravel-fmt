package alpine

import (
	"bytes"
	"testing"
)

// corpus is shared by the round-trip, idempotence, and token-preservation
// properties; every entry must lex and normalize cleanly.
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

func TestLexTokenRoundTrip(t *testing.T) {
	for _, src := range corpus {
		t.Run(src, func(t *testing.T) {
			input := []byte(src)
			toks, ok := lex(input)

			if !ok {
				t.Fatalf("lex(%q) failed; corpus entries must lex", src)
			}

			rebuilt := make([]byte, 0, len(input))
			pos := 0

			for _, tok := range toks {
				if tok.start < pos || tok.end <= tok.start || tok.end > len(input) {
					t.Fatalf("token span [%d, %d) out of order at byte %d", tok.start, tok.end, pos)
				}

				for _, c := range input[pos:tok.start] {
					if !isSpace(c) {
						t.Fatalf("non-whitespace byte %q dropped between tokens", c)
					}
				}

				rebuilt = append(rebuilt, input[pos:tok.start]...)
				rebuilt = append(rebuilt, input[tok.start:tok.end]...)
				pos = tok.end
			}

			for _, c := range input[pos:] {
				if !isSpace(c) {
					t.Fatalf("non-whitespace byte %q dropped after last token", c)
				}
			}

			rebuilt = append(rebuilt, input[pos:]...)

			if !bytes.Equal(rebuilt, input) {
				t.Fatalf("round trip produced %q, want %q", rebuilt, input)
			}
		})
	}
}

func TestLexKeepsTemplatesAtomic(t *testing.T) {
	src := []byte("`a ${ f({ b: '}' }) } c`")
	toks, ok := lex(src)

	if !ok {
		t.Fatal("lex failed on nested template")
	}

	if len(toks) != 1 || toks[0].kind != kindTemplate {
		t.Fatalf("got %d tokens, want one template token", len(toks))
	}

	if toks[0].start != 0 || toks[0].end != len(src) {
		t.Fatalf("template span [%d, %d), want the whole input", toks[0].start, toks[0].end)
	}
}

func TestLexRefusesAmbiguousSlashAfterBrace(t *testing.T) {
	if _, ok := lex([]byte("{} /re/.test(s)")); ok {
		t.Fatal("lex accepted a slash after }, which is ambiguous")
	}
}

func TestMultilineLiterals(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		spans int
		ok    bool
	}{
		{"one-line literals are not reported", "const a = `x`; const b = 'y';", 0, true},
		{"multi-line template", "const a = `x\n  y`;", 1, true},
		{"backslash-continued string", "const a = 'x\\\n  y';", 1, true},
		{"template with a nested template", "const a = `${`in\n`}`;", 1, true},
		{"line comment is not a literal", "// `not\nconst a = 1;", 0, true},
		{"ambiguous slash refuses", "const a = (b) / 2;", 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spans, ok := MultilineLiterals([]byte(tc.src))
			if ok != tc.ok || len(spans) != tc.spans {
				t.Fatalf("got %d spans ok=%v, want %d ok=%v", len(spans), ok, tc.spans, tc.ok)
			}
		})
	}
}
