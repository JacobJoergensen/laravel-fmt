package alpine

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
}

type normalizer struct {
	src           []byte
	toks          []token
	classes       []tokenClass
	spaceAfterNot bool
}

func render(src []byte, toks []token, spaceAfterNot bool) []byte {
	n := &normalizer{src: src, toks: toks, classes: classify(src, toks), spaceAfterNot: spaceAfterNot}
	out := make([]byte, 0, len(src))

	for i, t := range toks {
		if i > 0 && n.gap(i-1, i) {
			out = append(out, ' ')
		}

		out = append(out, src[t.start:t.end]...)
	}

	return out
}

func classify(src []byte, toks []token) []tokenClass {
	classes := make([]tokenClass, len(toks))

	// Open `?` positions are remembered with their bracket depth so the
	// matching `:` at the same depth is recognized as the conditional
	// operator while object-key colons in between stay tight.
	var ternaryOpens []int

	depth := 0
	prev := -1

	for i, t := range toks {
		if t.kind == kindComment {
			continue
		}

		if t.kind == kindPunct {
			switch string(src[t.start:t.end]) {
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
				text := string(src[t.start:t.end])
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

func operandEnd(src []byte, toks []token, classes []tokenClass, i int) bool {
	t := toks[i]

	switch t.kind {
	case kindNumber, kindString, kindTemplate, kindRegex:
		return true
	case kindIdent:
		return !nonOperandKeywords[string(src[t.start:t.end])]
	case kindPunct:
		switch string(src[t.start:t.end]) {
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
	if n.toks[a].kind == kindComment || n.toks[b].kind == kindComment {
		return true
	}

	switch {
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

		return t.kind == kindIdent && parenSpaceKeywords[string(n.src[t.start:t.end])]
	default:
		return true
	}
}

func (n *normalizer) is(i int, s string) bool {
	t := n.toks[i]

	return t.kind == kindPunct && string(n.src[t.start:t.end]) == s
}

func (n *normalizer) fuses(a, b int) bool {
	ca := n.src[n.toks[a].start]

	return (ca == '+' || ca == '-') && n.src[n.toks[b].start] == ca
}

func (n *normalizer) binary(i int) bool {
	t := n.toks[i]

	if t.kind != kindPunct {
		return false
	}

	switch string(n.src[t.start:t.end]) {
	case "=", "+=", "-=", "*=", "/=", "%=", "**=", "<<=", ">>=", ">>>=",
		"&=", "|=", "^=", "&&=", "||=", "??=", "=>",
		"==", "!=", "===", "!==", "<", ">", "<=", ">=",
		"&&", "||", "??", "**", "<<", ">>", ">>>",
		"%", "&", "|", "^", "*", "/":
		return true
	case "+", "-":
		return !n.classes[i].prefixUnary
	default:
		return false
	}
}
