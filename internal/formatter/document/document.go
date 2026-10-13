// Package document is a Wadler/Leijen-style document model: a builder
// describes layout intent (groups, lines, indents) as a Doc tree, and Print
// resolves it against a print width.
package document

import (
	"slices"
	"strings"
)

type kind uint8

const (
	kindText kind = iota
	kindConcat
	kindLine
	kindSoftLine
	kindHardLine
	kindGroup
	kindIndent
	kindLineSuffix
	kindBreakParent
	kindIfBreak
	kindLinePrefix
	kindHeredoc
	kindVerbatim
	kindAlignGap
	kindCondGroup
	kindOpener
)

// Doc is a node in the document tree. Its fields are ordered and groupID is
// 32-bit so a node fits the 48-byte size class: a format allocates tens of
// thousands of them.
type Doc struct {
	// text: alignGap: the padding the gap may grow by, as spaces
	text  string
	parts []*Doc
	// group: IfBreak identity; ifBreak: tracked group; alignGap: scope
	groupID  int32
	kind     kind
	mustWrap bool
}

var (
	line        = &Doc{kind: kindLine}
	softLine    = &Doc{kind: kindSoftLine}
	hardLine    = &Doc{kind: kindHardLine}
	breakParent = &Doc{kind: kindBreakParent}
)

// Shared texts for the syntax formatters emit most: across a Laravel app these
// strings were two thirds of all text.
var (
	textEmpty        = &Doc{kind: kindText}
	textSpace        = &Doc{kind: kindText, text: " "}
	textComma        = &Doc{kind: kindText, text: ","}
	textCommaSpace   = &Doc{kind: kindText, text: ", "}
	textSemicolon    = &Doc{kind: kindText, text: ";"}
	textColonSpace   = &Doc{kind: kindText, text: ": "}
	textArrow        = &Doc{kind: kindText, text: "->"}
	textDoubleArrow  = &Doc{kind: kindText, text: " => "}
	textDoubleColon  = &Doc{kind: kindText, text: "::"}
	textAssign       = &Doc{kind: kindText, text: " = "}
	textBackslash    = &Doc{kind: kindText, text: `\`}
	textParenOpen    = &Doc{kind: kindText, text: "("}
	textParenClose   = &Doc{kind: kindText, text: ")"}
	textParens       = &Doc{kind: kindText, text: "()"}
	textBraceOpen    = &Doc{kind: kindText, text: "{"}
	textBraceClose   = &Doc{kind: kindText, text: "}"}
	textBracketOpen  = &Doc{kind: kindText, text: "["}
	textBracketClose = &Doc{kind: kindText, text: "]"}
	textThis         = &Doc{kind: kindText, text: "$this"}
	textUse          = &Doc{kind: kindText, text: "use "}
	textFunction     = &Doc{kind: kindText, text: "function "}
	textPublic       = &Doc{kind: kindText, text: "public"}
	textReturn       = &Doc{kind: kindText, text: "return "}
)

// Text is a literal string with no internal line breaks.
func Text(s string) *Doc {
	// A switch, not a map: hashing every string cost more than sharing saved.
	switch s {
	case "":
		return textEmpty
	case " ":
		return textSpace
	case ",":
		return textComma
	case ", ":
		return textCommaSpace
	case ";":
		return textSemicolon
	case ": ":
		return textColonSpace
	case "->":
		return textArrow
	case " => ":
		return textDoubleArrow
	case "::":
		return textDoubleColon
	case " = ":
		return textAssign
	case `\`:
		return textBackslash
	case "(":
		return textParenOpen
	case ")":
		return textParenClose
	case "()":
		return textParens
	case "{":
		return textBraceOpen
	case "}":
		return textBraceClose
	case "[":
		return textBracketOpen
	case "]":
		return textBracketClose
	case "$this":
		return textThis
	case "use ":
		return textUse
	case "function ":
		return textFunction
	case "public":
		return textPublic
	case "return ":
		return textReturn
	}

	return &Doc{kind: kindText, text: s}
}

// AlignGap marks the gap before an aligned operator; scope names the array or
// match it belongs to, so adjacent constructs are not aligned together. pad is
// the most the alignment pass can widen it by, which the layout reserves so
// an aligned line cannot run past the print width.
func AlignGap(scope int32, pad int) *Doc {
	return &Doc{kind: kindAlignGap, groupID: scope, text: strings.Repeat(" ", max(pad, 0))}
}

func alignGapWidth(d *Doc) int {
	return 1 + len(d.text)
}

// Concat sequences docs with no separator.
func Concat(parts ...*Doc) *Doc { return &Doc{kind: kindConcat, parts: parts} }

// Join sequences docs with sep between each pair.
func Join(sep *Doc, docs []*Doc) *Doc {
	if len(docs) == 0 {
		return Concat()
	}

	parts := make([]*Doc, 0, len(docs)*2-1)
	for i, d := range docs {
		if i > 0 {
			parts = append(parts, sep)
		}

		parts = append(parts, d)
	}

	return &Doc{kind: kindConcat, parts: parts}
}

// Line is a space when its group is flat, a line break when broken.
func Line() *Doc { return line }

// SoftLine is nothing when flat, a line break when broken.
func SoftLine() *Doc { return softLine }

// HardLine always breaks and forces every enclosing group to break.
func HardLine() *Doc { return hardLine }

// BreakParent forces every enclosing group to break without emitting anything.
func BreakParent() *Doc { return breakParent }

// Group lays doc out flat, or breaks it if it does not fit the print width.
func Group(doc *Doc) *Doc { return &Doc{kind: kindGroup, parts: []*Doc{doc}} }

// GroupBreak is a Group that always breaks.
func GroupBreak(doc *Doc) *Doc {
	return &Doc{kind: kindGroup, parts: []*Doc{doc}, mustWrap: true}
}

// GroupID is a Group with an identity that IfBreakOf can track.
func GroupID(id int32, doc *Doc) *Doc {
	return &Doc{kind: kindGroup, parts: []*Doc{doc}, groupID: id}
}

// ConditionalGroup prints the first variant whose first line fits, else the
// last; unlike Group, a forced break further down a variant does not rule it
// out.
func ConditionalGroup(variants ...*Doc) *Doc {
	return &Doc{kind: kindCondGroup, parts: variants}
}

// Opener prints doc unchanged; only its own ConditionalGroup measures it as
// width columns then a break.
func Opener(width int, doc *Doc) *Doc {
	if width > ForcedBreakWidth {
		width = ForcedBreakWidth
	}

	return &Doc{kind: kindOpener, parts: []*Doc{doc}, groupID: int32(width)}
}

// Indent increases the indentation of any line breaks inside doc by one level.
func Indent(doc *Doc) *Doc { return &Doc{kind: kindIndent, parts: []*Doc{doc}} }

// LineSuffix defers doc to the next line break so a trailing comment rides the
// code line instead of splitting it.
func LineSuffix(doc *Doc) *Doc { return &Doc{kind: kindLineSuffix, parts: []*Doc{doc}} }

// IfBreak emits brk when the enclosing group breaks and flat when it stays flat.
func IfBreak(brk, flat *Doc) *Doc {
	return &Doc{kind: kindIfBreak, parts: []*Doc{brk, flat}}
}

// IfBreakOf is IfBreak keyed on a specific GroupID rather than the enclosing one.
func IfBreakOf(groupID int32, brk, flat *Doc) *Doc {
	return &Doc{kind: kindIfBreak, parts: []*Doc{brk, flat}, groupID: groupID}
}

// LinePrefix emits prefix after the indent on each line break inside doc (a
// comment block's leading `*`), leaving the opening line's opener to the caller.
func LinePrefix(prefix string, doc *Doc) *Doc {
	return &Doc{kind: kindLinePrefix, text: prefix, parts: []*Doc{doc}}
}

// ForcedBreakWidth is the effectively-infinite width FlatWidth reports for a doc
// with an unconditional break, so a budget comparison treats it as "will wrap".
const ForcedBreakWidth = 1 << 29

// HasForcedBreak reports whether d renders on multiple lines at any width,
// counting GroupBreak groups FlatWidth ignores; it marks d's groups as Print
// does.
func HasForcedBreak(d *Doc) bool {
	return propagateBreaks(d)
}

// CanBreak reports whether d holds any line-break opportunity.
func CanBreak(d *Doc) bool {
	switch d.kind {
	case kindLine, kindSoftLine, kindHardLine, kindHeredoc, kindVerbatim, kindBreakParent:
		return true
	case kindText, kindAlignGap:
		return false
	case kindConcat, kindGroup, kindIndent, kindLineSuffix, kindIfBreak, kindLinePrefix:
		return slices.ContainsFunc(d.parts, CanBreak)
	case kindCondGroup, kindOpener:
		return CanBreak(d.parts[0])
	}

	return false
}

// FlatWidth is d's one-line visual width with every group flat, or
// ForcedBreakWidth if it holds an unconditional break.
func FlatWidth(d *Doc, tabWidth int) int {
	switch d.kind {
	case kindText:
		// Text carrying its own newlines cannot be laid out flat, so it has no
		// one-line width: summing its lines would answer with a number no line
		// ever has, and the sum counts whitespace the emitter strips off a blank
		// line, letting bytes that are never written decide a break.
		if strings.IndexByte(d.text, '\n') >= 0 {
			return ForcedBreakWidth
		}

		return visualWidth(d.text, tabWidth)
	case kindLine:
		return 1
	case kindAlignGap:
		return alignGapWidth(d)
	case kindSoftLine, kindLineSuffix, kindBreakParent:
		return 0
	case kindHardLine, kindHeredoc, kindVerbatim:
		return ForcedBreakWidth
	case kindGroup, kindIndent, kindLinePrefix, kindCondGroup, kindOpener:
		return FlatWidth(d.parts[0], tabWidth)
	case kindIfBreak:
		return FlatWidth(d.parts[1], tabWidth)
	case kindConcat:
		sum := 0

		for _, c := range d.parts {
			sum += FlatWidth(c, tabWidth)
			if sum >= ForcedBreakWidth {
				return ForcedBreakWidth
			}
		}

		return sum
	}

	return 0
}

// FlatPrefixWidth is d's flat width up to its first unconditional break, and
// whether it reached one.
func FlatPrefixWidth(d *Doc, tabWidth int) (int, bool) {
	return prefixWidth(d, tabWidth, false, false)
}

// OpeningLineWidth is FlatPrefixWidth where a group that must break (an
// author-broken array) also ends the line at its first line break. Only a
// decision that is itself keyed on the author's breaks may use it: elsewhere
// it would read back a break the formatter emitted on width as authored.
func OpeningLineWidth(d *Doc, tabWidth int) (int, bool) {
	return prefixWidth(d, tabWidth, false, true)
}

// BrokenPrefixWidth is d's narrowest possible first line: every group may
// break, and a ConditionalGroup takes its narrowest variant.
func BrokenPrefixWidth(d *Doc, tabWidth int) (int, bool) {
	switch d.kind {
	case kindText, kindAlignGap:
		return FlatWidth(d, tabWidth), false
	case kindLineSuffix, kindBreakParent:
		return 0, false
	case kindLine, kindSoftLine, kindHardLine, kindHeredoc, kindVerbatim:
		return 0, true
	case kindGroup, kindIndent, kindLinePrefix, kindOpener, kindIfBreak:
		return BrokenPrefixWidth(d.parts[0], tabWidth)
	case kindCondGroup:
		narrowest, broken := BrokenPrefixWidth(d.parts[0], tabWidth)

		for _, v := range d.parts[1:] {
			if w, hard := BrokenPrefixWidth(v, tabWidth); w < narrowest {
				narrowest, broken = w, hard
			}
		}

		return narrowest, broken
	case kindConcat:
		sum := 0

		for _, c := range d.parts {
			w, done := BrokenPrefixWidth(c, tabWidth)
			sum += w

			if done {
				return sum, true
			}
		}

		return sum, false
	}

	return 0, false
}

// ClosingLineWidth is OpeningLineWidth for d's last line; run
// HasForcedBreak first.
func ClosingLineWidth(d *Doc, tabWidth int) (int, bool) {
	return suffixWidth(d, tabWidth, false)
}

func suffixWidth(d *Doc, tabWidth int, broken bool) (int, bool) {
	switch d.kind {
	case kindText:
		if i := strings.LastIndexByte(d.text, '\n'); i >= 0 {
			return visualWidth(d.text[i+1:], tabWidth), true
		}

		return FlatWidth(d, tabWidth), false
	case kindAlignGap:
		return FlatWidth(d, tabWidth), false
	case kindLine:
		if broken {
			return 0, true
		}

		return 1, false
	case kindSoftLine:
		return 0, broken
	case kindLineSuffix, kindBreakParent:
		return 0, false
	case kindVerbatim:
		// The last line starts at the indent of the line the text began on,
		// as a line after a hard break does.
		if n := len(d.parts); n > 0 {
			return visualWidth(d.parts[n-1].text, tabWidth), true
		}

		return visualWidth(d.text, tabWidth), false
	case kindHardLine, kindHeredoc:
		return 0, true
	case kindGroup:
		return suffixWidth(d.parts[0], tabWidth, d.mustWrap)
	case kindOpener:
		return suffixWidth(d.parts[0], tabWidth, broken)
	case kindCondGroup:
		return suffixWidth(d.parts[0], tabWidth, false)
	case kindIndent, kindLinePrefix:
		return suffixWidth(d.parts[0], tabWidth, broken)
	case kindIfBreak:
		if broken {
			return suffixWidth(d.parts[0], tabWidth, broken)
		}

		return suffixWidth(d.parts[1], tabWidth, broken)
	case kindConcat:
		sum := 0

		for _, c := range slices.Backward(d.parts) {
			w, hard := suffixWidth(c, tabWidth, broken)
			sum += w

			if hard {
				return sum, true
			}
		}

		return sum, false
	}

	return 0, false
}

// prefixWidth measures d's opening line. broken is set inside a group that
// must break, where each of its lines is a break; wrap says whether to honor
// such groups at all.
func prefixWidth(d *Doc, tabWidth int, broken, wrap bool) (int, bool) {
	switch d.kind {
	case kindText, kindAlignGap:
		return FlatWidth(d, tabWidth), false
	case kindLine:
		if broken {
			return 0, true
		}

		return 1, false
	case kindSoftLine:
		return 0, broken
	case kindLineSuffix, kindBreakParent:
		return 0, false
	case kindHardLine, kindHeredoc, kindVerbatim:
		return 0, true
	case kindGroup:
		return prefixWidth(d.parts[0], tabWidth, wrap && d.mustWrap, wrap)
	case kindOpener:
		return prefixWidth(d.parts[0], tabWidth, broken, wrap)
	case kindCondGroup:
		return prefixWidth(d.parts[0], tabWidth, false, wrap)
	case kindIndent, kindLinePrefix:
		return prefixWidth(d.parts[0], tabWidth, broken, wrap)
	case kindIfBreak:
		if broken {
			return prefixWidth(d.parts[0], tabWidth, broken, wrap)
		}

		return prefixWidth(d.parts[1], tabWidth, broken, wrap)
	case kindConcat:
		sum := 0

		for _, c := range d.parts {
			w, hard := prefixWidth(c, tabWidth, broken, wrap)
			sum += w

			if hard {
				return sum, true
			}
		}

		return sum, false
	}

	return 0, false
}

// Heredoc reindents the raw `<<<LABEL ... LABEL` text to the print indentation
// (plusOne = one level past the code), writing the body verbatim to preserve its
// value and falling back to the source when it cannot shift safely.
func Heredoc(text string, plusOne bool) *Doc {
	return &Doc{kind: kindHeredoc, text: text, mustWrap: plusOne}
}

// Verbatim prints first in place and starts each line of rest, which carries
// construct-relative indentation only, at the indent of the line it began on.
func Verbatim(first string, rest []string) *Doc {
	parts := make([]*Doc, len(rest))
	for i, ln := range rest {
		parts[i] = &Doc{kind: kindText, text: ln}
	}

	return &Doc{kind: kindVerbatim, text: first, parts: parts}
}
