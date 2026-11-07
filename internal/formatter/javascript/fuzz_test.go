package javascript

import (
	"bytes"
	"testing"

	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/javascript"
)

func FuzzNormalize(f *testing.F) {
	for _, seed := range []string{
		"open = ! open",
		"{ count: 0, items: [1,2,3] }",
		"$dispatch('saved', { id: item.id })",
		"x => x.value > 10 ? 'big' : 'small'",
		"`template ${value} string`",
		"{\n    open:false,\n    close(){ this.open=!this.open }\n}",
		"",
	} {
		f.Add([]byte(seed), true)
		f.Add([]byte(seed), false)
	}

	f.Fuzz(func(t *testing.T, value []byte, spaceAfterNot bool) {
		out, ok := Normalize(value, spaceAfterNot)
		if !ok {
			return
		}

		if !bytes.Equal(withoutSpaces(value), withoutSpaces(out)) {
			t.Fatalf("changed more than spacing: %q -> %q", value, out)
		}

		out2, ok := Normalize(out, spaceAfterNot)
		if !ok {
			t.Fatalf("normalized value rejected on second pass: %q", out)
		}

		if !bytes.Equal(out, out2) {
			t.Fatalf("not idempotent: %q -> %q -> %q", value, out, out2)
		}
	})
}

func FuzzLineDepths(f *testing.F) {
	for _, seed := range []string{
		"{\n count: 0,\n inc() {\n this.count++\n },\n}",
		"\nopen = false\n$dispatch('closed')\n",
		"items\n.filter(i => i.on)\n.map(i => i.id)",
		"ok = a &&\nb ?\nc :\nd",
		"{\nmsg: `a\n  b`\n}",
		"{\n    /**\n  * a\n     */\nb() {}\n}",
		"",
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, value []byte) {
		depths, ok := LineDepths(value)
		if !ok {
			return
		}

		out := atDepths(value, depths)

		if !bytes.Equal(trimmedLines(value), trimmedLines(out)) {
			t.Fatalf("changed more than leading whitespace: %q -> %q", value, out)
		}

		if !sameTokensMovingComments(value, out) {
			t.Fatalf("moved a token: %q -> %q", value, out)
		}

		depths2, ok := LineDepths(out)
		if !ok {
			t.Fatalf("reindented value rejected on second pass: %q", out)
		}

		if out2 := atDepths(out, depths2); !bytes.Equal(out, out2) {
			t.Fatalf("not idempotent: %q -> %q -> %q", value, out, out2)
		}
	})
}

func atDepths(value []byte, depths []Depth) []byte {
	lines := bytes.Split(value, []byte("\n"))

	for i := 1; i < len(lines); i++ {
		indent := bytes.Repeat([]byte("    "), depths[i].Level)
		if depths[i].InComment {
			indent = append(indent, ' ')
		}

		lines[i] = append(indent, bytes.TrimLeft(lines[i], " \t")...)
	}

	return bytes.Join(lines, []byte("\n"))
}

func sameTokensMovingComments(src, out []byte) bool {
	srcToks, ok := parser.Lex(src)
	if !ok {
		return false
	}

	outToks, ok := parser.Lex(out)
	if !ok || len(srcToks) != len(outToks) {
		return false
	}

	for i, s := range srcToks {
		o := outToks[i]
		a, b := src[s.Start:s.End], out[o.Start:o.End]

		if s.Kind == parser.KindComment {
			a, b = trimmedLines(a), trimmedLines(b)
		}

		if o.Kind != s.Kind || !bytes.Equal(a, b) {
			return false
		}
	}

	return true
}

func trimmedLines(b []byte) []byte {
	lines := bytes.Split(b, []byte("\n"))

	for i := range lines {
		lines[i] = bytes.TrimLeft(lines[i], " \t")
	}

	return bytes.Join(lines, []byte("\n"))
}

func withoutSpaces(b []byte) []byte {
	out := make([]byte, 0, len(b))

	for _, c := range b {
		if c != ' ' && c != '\t' {
			out = append(out, c)
		}
	}

	return out
}
