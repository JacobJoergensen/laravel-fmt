// Package html parses HTML into a CST whose node types the Blade parser
// reuses.
package html

// Parse parses src as a pure HTML document.
func Parse(src []byte) (*File, []Error) {
	p := &htmlParser{Src: src, Pos: Position{Line: 1, Column: 1}}
	nodes := p.parseNodes(nil)

	return &File{Src: src, Nodes: nodes}, p.errors
}
