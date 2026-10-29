// Package markdown is a block-level Markdown parser; inline content stays raw
// bytes.
package markdown

import "bytes"

// maxNestingDepth caps how deep containers nest, as the JSON, PHP and YAML
// parsers cap theirs. Each level's body is parsed again on its own, so a
// document thousands of levels deep costs far more than its size.
const maxNestingDepth = 500

// Parse parses src as Markdown. File is always non-nil. The only error is
// containers nested past maxNestingDepth: CommonMark has no other invalid input.
func Parse(src []byte) (*File, []Error) {
	if pos, ok := tooDeep(src); ok {
		return &File{Src: src}, []Error{{Pos: pos, Msg: "maximum nesting depth exceeded"}}
	}

	return parse(src, true), nil
}

// parseBody parses the body of a list item or blockquote, which unlike a
// document cannot open with frontmatter.
func (p *parser) parseBody(src []byte) *File {
	return &File{Src: src, Blocks: p.child(src, false).parseFile(false)}
}

func parse(src []byte, document bool) *File {
	p := &parser{src: src, pos: Position{Line: 1, Column: 1}, lazy: map[[2]uint64]bool{}}

	return &File{Src: src, Blocks: p.parseFile(document)}
}

func tooDeep(src []byte) (Position, bool) {
	for off, line := 0, 1; off < len(src); line++ {
		text, next := lineAt(src, off)

		if nestingBound(text) > maxNestingDepth {
			return Position{Offset: off, Line: line, Column: 1}, true
		}

		off = next
	}

	return Position{}, false
}

// nestingBound is the most container levels line can sit in: a level opened on
// it takes a marker, and one opened above it at least two columns of indentation,
// the narrowest list item content column.
func nestingBound(line []byte) int {
	depth := leadingIndentColumns(line) / 2

	for {
		line = bytes.TrimLeft(line, " \t")

		if len(line) > 0 && line[0] == '>' {
			depth++
			line = line[1:]

			continue
		}

		if isThematicBreak(line) {
			return depth
		}

		indent, marker, _, _, contentColumn, ok := listItemStart(line)
		if !ok {
			return depth
		}

		depth++
		line = listFirstLineContent(line, indent+len(marker), contentColumn)
	}
}
