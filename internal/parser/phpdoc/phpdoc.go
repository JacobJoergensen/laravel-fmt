// Package phpdoc parses the PHPStan/Psalm type expressions in PHP docblocks,
// a superset of PHP's own types that needs its own lexer.
package phpdoc

// ParseType parses src as a single PHPDoc type, returning *Invalid holding all
// of src when it is not a complete, well-formed type.
func ParseType(src []byte) Type {
	p := &typeParser{toks: lex(src)}
	t := p.parseType()

	if p.failed || !p.at(tokEOF) {
		return &Invalid{Raw: src}
	}

	return t
}

// ParseTypePrefix parses a leading type from src and returns it with the number
// of bytes consumed, or (*Invalid, 0) when src does not begin with a complete
// type. The type ends at a top-level space where PHPStan or Psalm starts a
// tag's description.
func ParseTypePrefix(src []byte) (Type, int) {
	return parsePrefix(src, true)
}

// ParseBoundPrefix is ParseTypePrefix for tags such as @template bounds, whose
// whole remainder Psalm reads as the type, so only PHPStan's boundary applies.
func ParseBoundPrefix(src []byte) (Type, int) {
	return parsePrefix(src, false)
}

func parsePrefix(src []byte, psalmSplit bool) (Type, int) {
	p := &typeParser{toks: lex(src), psalmSplit: psalmSplit}
	t := p.parseType()

	if p.failed || p.pos == 0 {
		return &Invalid{Raw: src}, 0
	}

	return t, p.toks[p.pos-1].end
}
