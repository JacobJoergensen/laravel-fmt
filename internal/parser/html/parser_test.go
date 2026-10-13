package html

import (
	"bytes"
	"testing"
)

func TestVoidElementsCloseWithoutEndTag(t *testing.T) {
	cases := []struct {
		src  string
		name string
	}{
		{"<br>", "br"},
		{"<img src=\"x.png\">", "img"},
		{"<input />", "input"},
	}

	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))

			el, ok := file.Nodes[0].(*Element)

			if !ok {
				t.Fatalf("node is %T, want *Element", file.Nodes[0])
			}

			if !el.SelfClose {
				t.Fatalf("SelfClose=false, want true for void element %q", tc.name)
			}

			if string(el.Name) != tc.name {
				t.Fatalf("Name=%q, want %q", el.Name, tc.name)
			}
		})
	}
}

func TestVoidElementParsesAsPairedWhenClosedInScope(t *testing.T) {
	src := "<channel><link>https://example.com</link></channel>"

	file, errs := Parse([]byte(src))

	if len(errs) > 0 {
		t.Fatalf("Parse returned errors: %v", errs)
	}

	channel, ok := file.Nodes[0].(*Element)

	if !ok {
		t.Fatalf("outer node is %T, want *Element", file.Nodes[0])
	}

	link, ok := channel.Children[0].(*Element)

	if !ok {
		t.Fatalf("inner node is %T, want *Element", channel.Children[0])
	}

	if string(link.Name) != linkTag {
		t.Fatalf("Name=%q, want %q", link.Name, linkTag)
	}

	if link.SelfClose {
		t.Fatal("SelfClose=true, want false for paired void element")
	}

	if string(link.CloseName) != linkTag {
		t.Fatalf("CloseName=%q, want %q", link.CloseName, linkTag)
	}

	if len(link.Children) != 1 {
		t.Fatalf("got %d children, want 1 text child", len(link.Children))
	}
}

func TestVoidElementStaysVoidWhenCloseIsOutOfScope(t *testing.T) {
	src := "<channel><link>x</channel></link>"

	file, _ := Parse([]byte(src))

	channel, ok := file.Nodes[0].(*Element)

	if !ok {
		t.Fatalf("outer node is %T, want *Element", file.Nodes[0])
	}

	link, ok := channel.Children[0].(*Element)

	if !ok {
		t.Fatalf("inner node is %T, want *Element", channel.Children[0])
	}

	if string(link.Name) != linkTag {
		t.Fatalf("Name=%q, want %q", link.Name, linkTag)
	}

	if len(link.Children) != 0 {
		t.Fatalf("got %d children, want 0: <link> must stay void when </link> is out of scope", len(link.Children))
	}
}

func TestRawTextElementBodyIsSingleTextNode(t *testing.T) {
	cases := []string{
		"<script>if (a < b && c > d) {}</script>",
		"<style>.foo { color: red; }</style>",
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			file, _ := Parse([]byte(src))
			el, ok := file.Nodes[0].(*Element)

			if !ok {
				t.Fatalf("node is %T, want *Element", file.Nodes[0])
			}

			if len(el.Children) != 1 {
				t.Fatalf("got %d children, want 1", len(el.Children))
			}

			if _, ok := el.Children[0].(*Text); !ok {
				t.Fatalf("child is %T, want *Text", el.Children[0])
			}
		})
	}
}

func TestRcdataElementBodyIsSingleTextNode(t *testing.T) {
	cases := []string{
		"<textarea><b>not</b> markup</textarea>",
		"<title>Plain <b>title</b></title>",
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			file, _ := Parse([]byte(src))

			el, ok := file.Nodes[0].(*Element)

			if !ok {
				t.Fatalf("node is %T, want *Element", file.Nodes[0])
			}

			if len(el.Children) != 1 {
				t.Fatalf("got %d children, want 1", len(el.Children))
			}

			if _, ok := el.Children[0].(*Text); !ok {
				t.Fatalf("child is %T, want *Text", el.Children[0])
			}
		})
	}
}

func TestPreElementParsesChildren(t *testing.T) {
	src := "<pre><b>x</b></pre>"
	file, _ := Parse([]byte(src))

	el, ok := file.Nodes[0].(*Element)

	if !ok {
		t.Fatalf("node is %T, want *Element", file.Nodes[0])
	}

	if len(el.Children) == 0 {
		t.Fatalf("pre has no children; body was not parsed")
	}

	if _, ok := el.Children[0].(*Element); !ok {
		t.Fatalf("first child is %T, want *Element (pre was raw-captured?)", el.Children[0])
	}
}

func TestOptionalEndTagsImplicitlyClose(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		parent string
		child  string
		want   int
	}{
		{"li peers and list close", "<ul><li>a<li>b</ul>", "ul", "li", 2},
		{"option peers and select close", "<select><option>a<option>b</select>", "select", "option", 2},
		{"paragraph peer and parent close", "<div><p>a<p>b</div>", "div", "p", 2},
		{"table cells and row close", "<table><tr><td>a<td>b</tr></table>", "tr", "td", 2},
		{"paragraph closed by dialog", "<div><p>a<dialog>b</dialog></div>", "div", "dialog", 1},
		{"paragraph closed by search", "<div><p>a<search>b</search></div>", "div", "search", 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src))

			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			parent := findElement(file.Nodes, tc.parent)

			if parent == nil {
				t.Fatalf("no <%s> element found", tc.parent)
			}

			got := countChildElements(parent, tc.child)

			if got != tc.want {
				t.Fatalf("got %d <%s> children of <%s>, want %d", got, tc.child, tc.parent, tc.want)
			}
		})
	}
}

func findElement(nodes []Node, name string) *Element {
	for _, n := range nodes {
		el, ok := n.(*Element)

		if !ok {
			continue
		}

		if bytes.EqualFold(el.Name, []byte(name)) {
			return el
		}

		if found := findElement(el.Children, name); found != nil {
			return found
		}
	}

	return nil
}

func countChildElements(el *Element, name string) int {
	count := 0

	for _, c := range el.Children {
		if child, ok := c.(*Element); ok && bytes.EqualFold(child.Name, []byte(name)) {
			count++
		}
	}

	return count
}

func TestAttributeFormsRoundTripThroughParse(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		attrName  string
		bound     bool
		shorthand bool
	}{
		{"flag", "<x disabled>", "disabled", false, false},
		{"plain", `<x class="foo">`, "class", false, false},
		{"bound", `<x :type="$v">`, "type", true, false},
		{"shorthand", "<x :$message>", "message", true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src))
			el, ok := file.Nodes[0].(*Element)

			if !ok {
				t.Fatalf("node is %T, want *Element", file.Nodes[0])
			}

			if len(el.Attributes) != 1 {
				t.Fatalf("got %d attributes, want 1", len(el.Attributes))
			}

			a := el.Attributes[0]

			if string(a.Name) != tc.attrName {
				t.Fatalf("Name=%q, want %q", a.Name, tc.attrName)
			}

			if a.Bound != tc.bound {
				t.Fatalf("Bound=%v, want %v", a.Bound, tc.bound)
			}

			if a.Shorthand != tc.shorthand {
				t.Fatalf("Shorthand=%v, want %v", a.Shorthand, tc.shorthand)
			}
		})
	}
}

func TestBoundAttrAlpineJSParsesWithoutErrors(t *testing.T) {
	src := `<div :class="{ open: isOpen }" :style="{ width: w + 'px' }"></div>`

	if _, errs := Parse([]byte(src)); len(errs) != 0 {
		t.Fatalf("Alpine :bound JS produced parse errors: %v", errs)
	}
}

// Each outsider comes from a different table, so a predicate wired to the
// wrong one fails; inline and inline-block must stay disjoint for the
// formatter's display:inline boundary.
func TestElementPredicatesReadTheirOwnTable(t *testing.T) {
	tests := []struct {
		name     string
		fn       func([]byte) bool
		member   string
		outsider string
	}{
		{"void", IsVoidElement, "IMG", "script"},
		{"raw text", IsRawTextElement, "ScRiPt", "img"},
		{"rcdata", IsRcdataElement, "TextArea", "script"},
		{"inline", IsInlineElement, "SPAN", "button"},
		{"inline block", IsInlineBlockElement, "BuTToN", "span"},
		{"preformatted", IsPreformattedElement, "PRE", "span"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.fn([]byte(tt.member)) {
				t.Errorf("%q reported as not a member; tag names are case-insensitive", tt.member)
			}

			if tt.fn([]byte(tt.outsider)) {
				t.Errorf("%q reported as a member; it belongs to a different table", tt.outsider)
			}
		})
	}
}

func TestUnquotedAttrValueEndsAtWhitespaceOrTagEnd(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		attrName  string
		value     string
		selfClose bool
	}{
		{"slash inside the value", "<a href=/docs>d</a>", "href", "/docs", false},
		{"slash-gt closes the tag", "<img src=a.png/>", "src", "a.png", true},
		{"form feed separates attributes", "<a\fhref=\"x\">d</a>", "href", "x", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src))
			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			el, ok := file.Nodes[0].(*Element)
			if !ok {
				t.Fatalf("node is %T, want *Element", file.Nodes[0])
			}

			a := el.Attributes[0]

			if string(a.Name) != tc.attrName || string(a.Value) != tc.value {
				t.Fatalf("attr = %q=%q, want %q=%q", a.Name, a.Value, tc.attrName, tc.value)
			}

			if el.SelfClose != tc.selfClose {
				t.Fatalf("SelfClose = %v, want %v", el.SelfClose, tc.selfClose)
			}
		})
	}
}

func TestParseRejectsAByteGluedToATagName(t *testing.T) {
	const noError = -1

	cases := []struct {
		src  string
		want int
	}{
		{`<span">x</span>`, 5},
		{`<span'>x</span>`, 5},
		{`<div{{ $a }}>x</div>`, 4},
		{`<div@click="a">x</div>`, 4},
		{`<p!a>x</p>`, 2},
		{`<p=a>x</p>`, 2},
		{`<x-a.b:c_d>x</x-a.b:c_d>`, noError},
		{`<br/>`, noError},
		{"<p\ta>x</p>", noError},
	}

	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			_, errs := Parse([]byte(tc.src))

			got := noError

			for _, e := range errs {
				if e.Msg == "unexpected character after tag name" {
					got = e.Pos.Offset
				}
			}

			if got != tc.want {
				t.Fatalf("glued-name error at %d, want %d (errors: %v)", got, tc.want, errs)
			}
		})
	}
}
