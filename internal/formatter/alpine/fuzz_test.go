package alpine

import (
	"bytes"
	"testing"
)

func FuzzNormalize(f *testing.F) {
	for _, seed := range []string{
		"open = ! open",
		"{ count: 0, items: [1,2,3] }",
		"$dispatch('saved', { id: item.id })",
		"x => x.value > 10 ? 'big' : 'small'",
		"`template ${value} string`",
		"",
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, value []byte) {
		out, ok := Normalize(value)
		if !ok {
			return
		}

		if !bytes.Equal(withoutSpaces(value), withoutSpaces(out)) {
			t.Fatalf("changed more than spacing: %q -> %q", value, out)
		}

		out2, ok := Normalize(out)
		if !ok {
			t.Fatalf("normalized value rejected on second pass: %q", out)
		}

		if !bytes.Equal(out, out2) {
			t.Fatalf("not idempotent: %q -> %q -> %q", value, out, out2)
		}
	})
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
