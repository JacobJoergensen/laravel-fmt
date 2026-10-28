package xml

import (
	"strings"
	"testing"
)

func TestAttributeMustBeQuoted(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr bool
	}{
		{"double quoted", `<a x="y"/>`, false},
		{"single quoted", `<a x='y'/>`, false},
		{"bare name", `<a x/>`, true},
		{"unquoted value", `<a x=y/>`, true},
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

func TestDoctypeHoldsItsInternalSubset(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		wantText string
	}{
		{"no subset", "<!DOCTYPE a SYSTEM \"a.dtd\"><a/>", " a SYSTEM \"a.dtd\""},
		{"declaration in subset", "<!DOCTYPE a [<!ENTITY b \"c\">]><a/>", " a [<!ENTITY b \"c\">]"},
		{"bracket and angle in a literal", "<!DOCTYPE a [<!ENTITY b \"]>\">]><a/>", " a [<!ENTITY b \"]>\">]"},
		{"bracket in a comment", "<!DOCTYPE a [<!-- ]> -->]><a/>", " a [<!-- ]> -->]"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src))
			if len(errs) > 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			doctype, ok := file.Nodes[0].(*Doctype)
			if !ok {
				t.Fatalf("node 0 is %T, want *Doctype", file.Nodes[0])
			}

			if string(doctype.Text) != tc.wantText {
				t.Fatalf("Text = %q, want %q", doctype.Text, tc.wantText)
			}

			if len(file.Nodes) != 2 {
				t.Fatalf("got %d nodes, want 2", len(file.Nodes))
			}
		})
	}
}

func TestUnclosedDoctypeSubsetIsAnError(t *testing.T) {
	_, errs := Parse([]byte("<!DOCTYPE a [<!ENTITY b \"c\">"))

	if len(errs) == 0 {
		t.Fatal("want an error for a DOCTYPE left open")
	}
}

func TestRawLessThanInTextIsAnError(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr bool
	}{
		{"escaped", "<a>1 &lt; 2</a>", false},
		{"inside CDATA", "<a><![CDATA[1 < 2]]></a>", false},
		{"before a space", "<a>1 < 2</a>", true},
		{"before a digit", "<a>1<2</a>", true},
		{"at the end", "<a/><", true},
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

func TestStrayClosingTagIsAnError(t *testing.T) {
	cases := []struct{ name, src string }{
		{"alone", "</a>"},
		{"inside an element", "<r></a></r>"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, errs := Parse([]byte(tc.src)); len(errs) == 0 {
				t.Fatal("want an error for a closing tag with no open element")
			}
		})
	}
}

func TestParseCapsNestingDepth(t *testing.T) {
	cases := []struct {
		name       string
		depth      int
		wantErr    bool
		wantColumn int
	}{
		{"at the cap", maxNestingDepth, false, 0},
		{"past the cap", maxNestingDepth + 1, true, len("<a>")*maxNestingDepth + 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.Repeat("<a>", tc.depth) + strings.Repeat("</a>", tc.depth)
			_, errs := Parse([]byte(src))

			if !tc.wantErr {
				if len(errs) != 0 {
					t.Fatalf("errors = %v, want none", errs)
				}

				return
			}

			if len(errs) != 1 || errs[0].Msg != "maximum nesting depth exceeded" || errs[0].Pos.Column != tc.wantColumn {
				t.Fatalf("errors = %v, want one depth error at column %d", errs, tc.wantColumn)
			}
		})
	}
}
