// Package xml parses XML documents, such as phpunit.xml, into a lossless tree.
package xml

// Whitespace is XML's S production; bytes.TrimSpace would also strip NBSP and NEL.
const Whitespace = " \t\r\n"

// Parse parses src as an XML document. File is always non-nil; a non-empty
// error slice means malformed input whose partial tree must not be re-emitted.
func Parse(src []byte) (*File, []Error) {
	return parse(src, true)
}

// ParseFragment is Parse without the one-root rule, for snippets in documentation.
func ParseFragment(src []byte) (*File, []Error) {
	return parse(src, false)
}

func parse(src []byte, document bool) (*File, []Error) {
	p := &xmlParser{Src: src, Pos: Position{Line: 1, Column: 1}}
	p.checkChars()
	nodes := p.parseNodes(nil)

	if document {
		p.checkDocument(nodes)
	}

	return &File{Src: src, Nodes: nodes}, p.errors
}
