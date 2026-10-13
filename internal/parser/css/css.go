// Package css parses CSS into a lossless tree of rules, at-rules,
// declarations and comments, including native nesting.
package css

// Parse parses src as a CSS stylesheet; File is non-nil and holds whatever
// parsed even when errors are returned, which the Tailwind theme readers
// rely on.
func Parse(src []byte) (*File, []Error) {
	p := &cssParser{Src: src, Pos: Position{Line: 1, Column: 1}}
	nodes := p.parseNodes(false)

	return &File{Src: src, Nodes: nodes}, p.errors
}
