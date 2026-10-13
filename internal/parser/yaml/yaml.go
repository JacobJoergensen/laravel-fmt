// Package yaml parses the YAML subset found in PHP projects (CI workflows,
// docker-compose, dependabot) into a lossless tree, keeping what it does not
// model as verbatim spans.
package yaml

import (
	"fmt"
	"unicode/utf8"
)

// Parse parses src as a YAML stream. File is always non-nil; a non-empty error
// slice means the input could not be parsed cleanly and its partial tree must
// not be re-emitted.
func Parse(src []byte) (*File, []Error) {
	if pos, ok := firstNonPrintable(src); ok {
		return &File{Src: src}, []Error{{Pos: pos, Msg: nonPrintableMessage(src[pos.Offset:])}}
	}

	p := &yamlParser{Src: src, Pos: Position{Line: 1, Column: 1}}
	docs := p.parseStream()

	return &File{Src: src, Docs: docs}, p.errors
}

func firstNonPrintable(src []byte) (Position, bool) {
	line, col := 1, 1

	for i := 0; i < len(src); {
		r, size := rune(src[i]), 1

		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRune(src[i:])
		}

		if !isPrintable(r, size) {
			return Position{Offset: i, Line: line, Column: col}, true
		}

		if r == '\n' {
			line++
			col = 1
		} else {
			col += size
		}

		i += size
	}

	return Position{}, false
}

// isPrintable follows YAML 1.2 rule c-printable (section 5.1). A RuneError of
// width one is a byte that is not UTF-8, which a YAML stream cannot hold.
func isPrintable(r rune, size int) bool {
	switch {
	case r == utf8.RuneError && size == 1:
		return false
	case r == '\t' || r == '\n' || r == '\r' || r == 0x85:
		return true
	case r >= 0x20 && r <= 0x7e:
		return true
	case r >= 0xa0 && r <= 0xd7ff:
		return true
	case r >= 0xe000 && r <= 0xfffd:
		return true
	default:
		return r >= 0x10000 && r <= utf8.MaxRune
	}
}

func nonPrintableMessage(rest []byte) string {
	r, size := utf8.DecodeRune(rest)

	if r == utf8.RuneError && size == 1 {
		return fmt.Sprintf("invalid UTF-8 byte %#02x", rest[0])
	}

	return fmt.Sprintf("forbidden character %U", r)
}
