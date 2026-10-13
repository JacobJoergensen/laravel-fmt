// Package json parses JSON: Parse is strict RFC 8259, and ParseJSONC also
// accepts `//` and `/* */` comments and trailing commas (tsconfig.json and
// friends), keeping the comments on the tree.
package json

// Parse parses src as strict RFC 8259 JSON. File is always non-nil; a non-empty
// error slice means invalid JSON whose partial tree must not be re-emitted.
func Parse(src []byte) (*File, []Error) {
	return parse(src, false)
}

// ParseJSONC parses src as JSONC: JSON plus comments and trailing commas.
func ParseJSONC(src []byte) (*File, []Error) {
	return parse(src, true)
}

func parse(src []byte, jsonc bool) (*File, []Error) {
	p := &jsonParser{
		Src: src, Pos: Position{Line: 1, Column: 1},
		jsonc: jsonc,
	}
	root := p.parseDocument()

	return &File{Src: src, Root: root, Leading: p.fileLeading, Trailing: p.fileTrailing}, p.errors
}
