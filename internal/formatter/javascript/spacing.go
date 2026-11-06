package javascript

import (
	"bytes"

	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/javascript"
)

type tokenClass struct {
	prefixUnary bool
	postfix     bool
	ternary     bool
}

var nonOperandKeywords = map[string]bool{
	"await": true, "case": true, "catch": true, "const": true,
	"delete": true, "do": true, "else": true, "for": true, "function": true,
	"if": true, "in": true, "instanceof": true, "let": true, "new": true,
	"of": true, "return": true, "switch": true, "throw": true,
	"typeof": true, "var": true, "void": true, "while": true, "yield": true,
}

var parenSpaceKeywords = map[string]bool{
	"async": true, "await": true, "case": true, "catch": true,
	"delete": true, "do": true, "else": true, "for": true, "function": true,
	"if": true, "in": true, "instanceof": true, "new": true, "of": true,
	"return": true, "switch": true, "throw": true, "typeof": true,
	"void": true, "while": true, "yield": true,
	"const": true, "let": true, "var": true,
}

type normalizer struct {
	src           []byte
	toks          []parser.Token
	classes       []tokenClass
	spaceAfterNot bool
}

func render(src []byte, toks []parser.Token, spaceAfterNot bool) []byte {
	n := &normalizer{src: src, toks: toks, classes: classify(src, toks), spaceAfterNot: spaceAfterNot}
	out := make([]byte, 0, len(src))

	if lead := src[:toks[0].Start]; bytes.IndexByte(lead, '\n') >= 0 {
		out = append(out, lead...)
	}

	// A `;` starting a line guards it against ASI joining it to the line above
	// (`;[a, b] = pair`), so it stays glued to what it guards.
	lineStart := true

	for i, t := range toks {
		if i > 0 {
			gap := src[toks[i-1].End:t.Start]
			guard := lineStart && n.is(i-1, ";")
			lineStart = bytes.IndexByte(gap, '\n') >= 0

			switch {
			case lineStart:
				out = append(out, gap...)
			case !guard && n.gap(i-1, i):
				out = append(out, ' ')
			}
		}

		out = append(out, src[t.Start:t.End]...)
	}

	if tail := src[toks[len(toks)-1].End:]; bytes.IndexByte(tail, '\n') >= 0 {
		out = append(out, tail...)
	}

	return out
}

func classify(src []byte, toks []parser.Token) []tokenClass {
	classes := make([]tokenClass, len(toks))

	// Open `?` positions are remembered with their bracket depth so the
	// matching `:` at the same depth is recognized as the conditional
	// operator while object-key colons in between stay tight.
	var ternaryOpens []int

	depth := 0
	prev := -1

	for i, t := range toks {
		if t.Kind == parser.KindComment {
			continue
		}

		if t.Kind == parser.KindPunct {
			switch string(src[t.Start:t.End]) {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--

				for len(ternaryOpens) > 0 && ternaryOpens[len(ternaryOpens)-1] > depth {
					ternaryOpens = ternaryOpens[:len(ternaryOpens)-1]
				}
			case "?":
				classes[i].ternary = true
				ternaryOpens = append(ternaryOpens, depth)
			case ":":
				if len(ternaryOpens) > 0 && ternaryOpens[len(ternaryOpens)-1] == depth {
					classes[i].ternary = true
					ternaryOpens = ternaryOpens[:len(ternaryOpens)-1]
				}
			case "+", "-", "!", "~", "++", "--":
				text := string(src[t.Start:t.End])
				prefix := text == "!" || text == "~" ||
					prev < 0 || !operandEnd(src, toks, classes, prev)

				switch {
				case prefix:
					classes[i].prefixUnary = true
				case text == "++" || text == "--":
					classes[i].postfix = true
				}
			}
		}

		prev = i
	}

	return classes
}

func operandEnd(src []byte, toks []parser.Token, classes []tokenClass, i int) bool {
	t := toks[i]

	switch t.Kind {
	case parser.KindNumber, parser.KindString, parser.KindTemplate, parser.KindRegex:
		return true
	case parser.KindIdent:
		return !nonOperandKeywords[string(src[t.Start:t.End])]
	case parser.KindPunct:
		switch string(src[t.Start:t.End]) {
		case ")", "]", "}":
			return true
		}

		return classes[i].postfix
	default:
		return false
	}
}

// gap reports whether one space separates token b from token a before it.
// The cases overlap, so their order decides the spacing.
func (n *normalizer) gap(a, b int) bool {
	if n.toks[a].Kind == parser.KindComment || n.toks[b].Kind == parser.KindComment {
		return true
	}

	switch {
	case n.is(a, "--") && n.src[n.toks[b].Start] == '>':
		// Glued, `-->` starts an HTML-like comment at the start of a line.
		return true
	case n.is(a, "{") && n.is(b, "}"):
		return false
	case n.is(b, ")"), n.is(b, "]"):
		return false
	case n.is(a, "("), n.is(a, "["):
		return false
	case n.is(a, "{"), n.is(b, "}"):
		return true
	case n.is(b, ","), n.is(b, ";"):
		return false
	case n.is(a, ","), n.is(a, ";"):
		return true
	case n.is(a, "."), n.is(b, "."), n.is(a, "?."), n.is(b, "?."):
		return false
	case n.is(a, "!"):
		// `!!x` is a boolean cast and stays one unit.
		return n.spaceAfterNot && !n.is(b, "!") && (a == 0 || !n.is(a-1, "!"))
	case n.classes[a].prefixUnary:
		// `- -x` must keep its space or the pair re-lexes as `--`.
		return n.fuses(a, b)
	case n.classes[b].postfix:
		return false
	case n.is(b, ":"):
		return n.classes[b].ternary
	case n.is(a, ":"):
		return true
	case n.is(a, "?"), n.is(b, "?"):
		return true
	case n.binary(a), n.binary(b):
		return true
	case n.is(a, "..."):
		return false
	case n.is(b, "("), n.is(b, "["):
		// A keyword keeps its space: `foo(x)` and `foo[0]` are a call and an
		// index, but `in [1, 2]` is not. Alpine matches x-for with a regex that
		// requires the space, so `i in[...]` makes the loop render nothing.
		t := n.toks[a]

		return t.Kind == parser.KindIdent && parenSpaceKeywords[string(n.src[t.Start:t.End])]
	default:
		return true
	}
}

func (n *normalizer) is(i int, s string) bool {
	return isPunctText(n.src, n.toks[i], s)
}

func isPunctText(src []byte, t parser.Token, s string) bool {
	return t.Kind == parser.KindPunct && string(src[t.Start:t.End]) == s
}

func (n *normalizer) fuses(a, b int) bool {
	ca := n.src[n.toks[a].Start]

	return (ca == '+' || ca == '-') && n.src[n.toks[b].Start] == ca
}

func (n *normalizer) binary(i int) bool {
	return isBinary(n.src, n.toks, n.classes, i)
}

func isBinary(src []byte, toks []parser.Token, classes []tokenClass, i int) bool {
	t := toks[i]

	if t.Kind != parser.KindPunct {
		return false
	}

	switch string(src[t.Start:t.End]) {
	case "=", "+=", "-=", "*=", "/=", "%=", "**=", "<<=", ">>=", ">>>=",
		"&=", "|=", "^=", "&&=", "||=", "??=", "=>",
		"==", "!=", "===", "!==", "<", ">", "<=", ">=",
		"&&", "||", "??", "**", "<<", ">>", ">>>",
		"%", "&", "|", "^", "*", "/":
		return true
	case "+", "-":
		return !classes[i].prefixUnary
	default:
		return false
	}
}
