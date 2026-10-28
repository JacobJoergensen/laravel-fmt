package yaml

import (
	"fmt"
	"strings"
	"testing"
)

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
		{"bracket in a comment inside a flow", "a: [b,\n  # see [ here\n  d]\n", false},
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

func TestParseBlockScalarSpanHoldsExactlyItsText(t *testing.T) {
	cases := []struct{ name, src, wantRaw string }{
		{"anchored keep", "k: &a |+\n  a\n\n\nj: 1\n", "&a |+\n  a\n\n"},
		{"plus in the header comment", "k: | # c++\n  a\n\n\nj: 1\n", "| # c++\n  a"},
		{"hash line only", "k: |\n  ### Thanks to all contributors!\n\nj: 1\n", "|\n  ### Thanks to all contributors!"},
		{"hash line last", "k: |\n  text\n  # not a comment\nj: 1\n", "|\n  text\n  # not a comment"},
		{"line of spaces past the body indent", "k: |\n  x\n   \nj: 1\n", "|\n  x\n   "},
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

func TestParseOpensAQuotedOrFlowKeyOnlyAtTheNodeStart(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		items int
	}{
		{"apostrophe inside a plain item", "- it's\n- 'a: b'\n- c\n", 3},
		{"flow mapping as an item", "- { a: { b: 1 }, c: 2 }\n- d\n", 2},
		{"apostrophe inside a flow item", "- [it's, b]\n- c\n", 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src))
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}

			seq, ok := file.Docs[0].Body.(*Sequence)
			if !ok || len(seq.Items) != tc.items {
				t.Fatalf("body = %T %+v, want a %d-item *Sequence", file.Docs[0].Body, seq, tc.items)
			}

			for _, item := range seq.Items {
				if _, isMapping := item.Value.(*Mapping); isMapping {
					t.Fatalf("item %+v read as a mapping", item)
				}
			}
		})
	}
}

func TestParseRejectsADuplicateKey(t *testing.T) {
	var manyKeys strings.Builder

	for i := range 40 {
		fmt.Fprintf(&manyKeys, "k%d: %d\n", i, i)
	}

	cases := []struct {
		name     string
		src      string
		wantErr  bool
		wantLine int
	}{
		{"plain key twice", "a: 1\nb: 2\na: 3\n", true, 3},
		{"quoted styles of one key", "'a': 1\n\"a\": 2\n", true, 2},
		{"repeat past the key set threshold", manyKeys.String() + "k3: again\n", true, 41},
		{"plain and quoted are different keys", "1: a\n'1': b\n", false, 0},
		{"same key in sibling mappings", "x:\n  a: 1\ny:\n  a: 2\n", false, 0},
		{"repeated merge key", "a: &x {b: 1}\nc:\n  <<: *x\n  <<: *x\n", false, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := Parse([]byte(tc.src))

			if !tc.wantErr {
				if len(errs) != 0 {
					t.Fatalf("errors = %v, want none", errs)
				}

				return
			}

			if len(errs) != 1 || !strings.HasPrefix(errs[0].Msg, "duplicate key") || errs[0].Pos.Line != tc.wantLine {
				t.Fatalf("errors = %v, want one duplicate key error on line %d", errs, tc.wantLine)
			}
		})
	}
}

func TestParseRejectsAKeyIndentedToNoOpenMapping(t *testing.T) {
	cases := []struct {
		name              string
		src               string
		wantErr           bool
		wantLine, wantCol int
	}{
		{"key under a list item's dash", "body:\n    - type: input\n    id: x\n", true, 3, 5},
		{"key below a scalar value", "a: one\n  two: x\n", true, 2, 3},
		{"key dedented between two mappings", "a:\n    b: 1\n  c: 2\n", true, 3, 3},
		{"list after an anchored key", "seq: &anchor\n- a\n", false, 0, 0},
		{"explicit key's value", "a:\n  ? sky\n  : blue\n", false, 0, 0},
		{"explicit key holding a mapping", "? first: Sammy\n  last: Sosa\n: x\n", false, 0, 0},
		{"list item holding a flow mapping", "x:\n  y: &a\n  - {c: d}\n", false, 0, 0},
		{"key after a block the parser ended early", "x:\n  y: &a\n  - b\n  z: 1\n", false, 0, 0},
		{"anchored list item holding a mapping", "in:\n  - &any\n    go: t\n", false, 0, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := Parse([]byte(tc.src))

			if !tc.wantErr {
				if len(errs) != 0 {
					t.Fatalf("errors = %v, want none", errs)
				}

				return
			}

			if len(errs) != 1 || errs[0].Msg != "key is indented to no open mapping" || errs[0].Pos.Line != tc.wantLine || errs[0].Pos.Column != tc.wantCol {
				t.Fatalf("errors = %v, want one stranded key error at %d:%d", errs, tc.wantLine, tc.wantCol)
			}
		})
	}
}
