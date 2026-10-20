package yaml

import "testing"

func TestUnterminatedConstructIsAnError(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr bool
	}{
		{"closed double quote", "a: \"b\"\n", false},
		{"closed single quote", "a: 'b''c'\n", false},
		{"quote spanning lines", "a: \"b\n  c\"\n", false},
		{"apostrophe in a plain scalar", "a: it's\nb: c\n", false},
		{"closed flow", "a: [b, {c: d}]\n", false},
		{"open double quote", "a: \"b\nc: d\n", true},
		{"open single quote", "a: 'b\nc: d\n", true},
		{"open flow", "a: [b, c\nd: e\n", true},
		{"open quote inside a flow", "a: [\"b]\n", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := Parse([]byte(tc.src))

			if gotErr := len(errs) > 0; gotErr != tc.wantErr {
				t.Fatalf("errors = %v, want error %v", errs, tc.wantErr)
			}
		})
	}
}

func TestParseReadsADashBlockScalarAgainstTheDash(t *testing.T) {
	for _, src := range []string{"- |\n  a\n- b\n", "- !!str |\n  a\n- b\n"} {
		t.Run(src, func(t *testing.T) {
			file, errs := Parse([]byte(src))
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}

			if len(file.Docs) != 1 {
				t.Fatalf("got %d documents, want 1", len(file.Docs))
			}

			seq, ok := file.Docs[0].Body.(*Sequence)
			if !ok || len(seq.Items) != 2 {
				t.Fatalf("body = %T with items %v, want a two-item *Sequence", file.Docs[0].Body, seq)
			}

			scalar, ok := seq.Items[0].Value.(*BlockScalar)
			if !ok || scalar.ContentIndent != 2 {
				t.Fatalf("first item = %+v, want a *BlockScalar with ContentIndent 2", seq.Items[0].Value)
			}
		})
	}
}

func TestParseKeepsOnlyKeptBreaksInABlockScalarsSpan(t *testing.T) {
	cases := []struct{ name, src, wantRaw string }{
		{"anchored keep", "k: &a |+\n  a\n\n\nj: 1\n", "&a |+\n  a\n\n"},
		{"plus in the header comment", "k: | # c++\n  a\n\n\nj: 1\n", "| # c++\n  a"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src))
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}

			mapping, ok := file.Docs[0].Body.(*Mapping)
			if !ok || len(mapping.Entries) != 2 {
				t.Fatalf("body = %T, want a two-entry *Mapping", file.Docs[0].Body)
			}

			scalar, ok := mapping.Entries[0].Value.(*BlockScalar)
			if !ok || string(scalar.Raw) != tc.wantRaw {
				t.Fatalf("first value = %#v, want a *BlockScalar with Raw %q", mapping.Entries[0].Value, tc.wantRaw)
			}
		})
	}
}

func TestParseRecordsABlockScalarsContentIndent(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"first body line", "a: |\n    b\n      c\n", 4},
		{"leading empty lines skipped", "a: >\n\n  \n   b\n", 3},
		{"crlf body", "a: |-\r\n    b\r\n", 4},
		{"indentation indicator", "a: |2\n    b\n", 0},
		{"empty body", "a: |\nb: c\n", 0},
		{"tab after the indent", "a: |\n  \tb\n", 0},
		{"whitespace-only tab line skipped", "a: >\n\t\n  b\n", 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src))
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}

			mapping, ok := file.Docs[0].Body.(*Mapping)
			if !ok {
				t.Fatalf("body is %T, want *Mapping", file.Docs[0].Body)
			}

			scalar, ok := mapping.Entries[0].Value.(*BlockScalar)
			if !ok {
				t.Fatalf("value is %T, want *BlockScalar", mapping.Entries[0].Value)
			}

			if scalar.ContentIndent != tc.want {
				t.Fatalf("ContentIndent = %d, want %d", scalar.ContentIndent, tc.want)
			}
		})
	}
}
