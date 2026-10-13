// Package xml parses XML documents, such as phpunit.xml, into a lossless tree.
package xml

// Parse parses src as an XML document. File is always non-nil; a non-empty
// error slice means malformed input whose partial tree must not be re-emitted.
func Parse(src []byte) (*File, []Error) {
	p := &xmlParser{Src: src, Pos: Position{Line: 1, Column: 1}}
	nodes := p.parseNodes(nil)

	return &File{Src: src, Nodes: nodes}, p.errors
}
