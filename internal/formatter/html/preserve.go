package html

import (
	"bytes"

	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/html"
)

var whitespacePreservingClasses = map[string]bool{
	"whitespace-pre":          true,
	"whitespace-pre-wrap":     true,
	"whitespace-pre-line":     true,
	"whitespace-break-spaces": true,
}

// CSS Text 4 lets the white-space shorthand take white-space-collapse
// keywords too (`white-space: preserve nowrap`).
var whitespacePreservingValues = map[string]bool{
	"pre":             true,
	"pre-wrap":        true,
	"pre-line":        true,
	"break-spaces":    true,
	"preserve":        true,
	"preserve-breaks": true,
	"preserve-spaces": true,
}

// IsWhitespacePreservingElement reports whether el's white-space keeps source
// newlines, through its class, its inline style or a class in pageClasses, so
// its body has to go out verbatim. A bound or echoed value that mentions a
// preserving white-space counts, since it cannot be evaluated.
func IsWhitespacePreservingElement(el *parser.Element, pageClasses map[string]bool) bool {
	for _, a := range el.Attributes {
		if a.Value == nil {
			continue
		}

		name, bound := boundAttrTarget(a)
		isStyle := bytes.EqualFold(name, []byte("style"))

		if !isStyle && !IsClassAttr(name) {
			continue
		}

		if bound {
			if MentionsPreservingWhitespace(a.Value) {
				return true
			}

			continue
		}

		if isStyle {
			if stylePreservesWhitespace(a.Value) {
				return true
			}

			continue
		}

		if classListPreservesWhitespace(a.Value) || anyClassIn(a.Value, pageClasses) {
			return true
		}

		if a.Parts != nil && MentionsPreservingWhitespace(a.Value) {
			return true
		}
	}

	return false
}

// boundAttrTarget returns the attribute an Alpine or Blade binding sets, and
// whether the value is an expression: Blade parses `:class` as Bound, while the
// HTML parser and `x-bind:class` keep the prefix in the name.
func boundAttrTarget(a *parser.Attr) ([]byte, bool) {
	if a.Bound {
		return a.Name, true
	}

	const xBind = "x-bind:"

	if len(a.Name) > len(xBind) && bytes.EqualFold(a.Name[:len(xBind)], []byte(xBind)) {
		return a.Name[len(xBind):], true
	}

	if len(a.Name) > 1 && a.Name[0] == ':' {
		return a.Name[1:], true
	}

	return a.Name, false
}

// MentionsPreservingWhitespace reports whether a class or style expression
// names a preserving white-space utility or property, counting any value it
// cannot evaluate as preserving. Whitespace in expr is ignored: the formatter
// re-spaces operators (`white-space` becomes `white - space`), and the answer
// must not change between passes.
func MentionsPreservingWhitespace(expr []byte) bool {
	lower := asciiLower(withoutWhitespace(expr))

	if bytes.Contains(lower, []byte("white-space")) {
		return true
	}

	for i := 0; ; {
		at := bytes.Index(lower[i:], []byte("whitespace"))
		if at < 0 {
			return false
		}

		i += at + len("whitespace")
		rest := lower[i:]

		if !bytes.HasPrefix(rest, []byte("-")) ||
			bytes.HasPrefix(rest, []byte("-pre")) ||
			bytes.HasPrefix(rest, []byte("-break-spaces")) {
			return true
		}
	}
}

// IsEmptinessSensitiveElement reports whether el's own class gates on `:empty`
// (`[&:not(:empty)]:pb-2`), so a whitespace text node added around its content
// would change whether it matches. A page <style> `:empty` rule is not checked.
func IsEmptinessSensitiveElement(el *parser.Element) bool {
	for _, a := range el.Attributes {
		if a.Bound || a.Value == nil {
			continue
		}

		if IsClassAttr(a.Name) && bytes.Contains(a.Value, []byte(":empty")) {
			return true
		}
	}

	return false
}

// ScanPageStyleWhitespaceClasses returns the class names the page's <style>
// blocks give a preserving white-space, including rules nested in an at-rule
// or another rule.
func ScanPageStyleWhitespaceClasses(src []byte) map[string]bool {
	out := map[string]bool{}

	lower := asciiLower(src)
	pos := 0

	for {
		open := bytes.Index(lower[pos:], []byte("<style"))
		if open < 0 {
			break
		}

		open += pos

		gt := bytes.IndexByte(lower[open:], '>')
		if gt < 0 {
			break
		}

		bodyStart := open + gt + 1

		rel := bytes.Index(lower[bodyStart:], []byte("</style"))
		if rel < 0 {
			break
		}

		scanCSSPreservingClasses(src[bodyStart:bodyStart+rel], lower[bodyStart:bodyStart+rel], out)
		pos = bodyStart + rel
	}

	return out
}

func anyClassIn(value []byte, classes map[string]bool) bool {
	if len(classes) == 0 {
		return false
	}

	for tok := range bytes.FieldsSeq(value) {
		if classes[string(tok)] {
			return true
		}
	}

	return false
}

// asciiLower lowercases ASCII letters only, so unlike bytes.ToLower it keeps
// every byte at its offset.
func withoutWhitespace(src []byte) []byte {
	return bytes.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' {
			return -1
		}

		return r
	}, src)
}

func asciiLower(src []byte) []byte {
	out := make([]byte, len(src))

	for i, c := range src {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}

		out[i] = c
	}

	return out
}

func scanCSSPreservingClasses(body, bodyLower []byte, out map[string]bool) {
	var selectors [][]byte

	runStart := 0

	for i, c := range bodyLower {
		if c != '{' && c != '}' {
			continue
		}

		run := bodyLower[runStart:i]
		if len(selectors) > 0 && stylePreservesWhitespace(run) {
			collectSelectorClasses(selectors[len(selectors)-1], out)
		}

		if c == '{' {
			selectorStart := runStart + bytes.LastIndexByte(run, ';') + 1
			selectors = append(selectors, body[selectorStart:i])
		} else if len(selectors) > 0 {
			selectors = selectors[:len(selectors)-1]
		}

		runStart = i + 1
	}

	if len(selectors) > 0 && stylePreservesWhitespace(bodyLower[runStart:]) {
		collectSelectorClasses(selectors[len(selectors)-1], out)
	}
}

func collectSelectorClasses(selector []byte, out map[string]bool) {
	for {
		dot := bytes.IndexByte(selector, '.')
		if dot < 0 {
			return
		}

		selector = selector[dot+1:]

		n := 0
		for n < len(selector) && isClassNameByte(selector[n]) {
			n++
		}

		if n > 0 {
			out[string(selector[:n])] = true
		}

		selector = selector[n:]
	}
}

func isClassNameByte(c byte) bool {
	return c == '-' || c == '_' ||
		c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// classListPreservesWhitespace reports whether a class value carries a
// whitespace-preserving utility. A variant prefix only limits where the
// utility applies, so it still counts.
func classListPreservesWhitespace(value []byte) bool {
	for tok := range bytes.FieldsSeq(value) {
		name := bytes.Trim(utilityName(tok), "!")

		if whitespacePreservingClasses[string(name)] {
			return true
		}

		if len(name) > 2 && name[0] == '[' && name[len(name)-1] == ']' {
			decl := bytes.ReplaceAll(name[1:len(name)-1], []byte("_"), []byte(" "))
			if stylePreservesWhitespace(decl) {
				return true
			}
		}
	}

	return false
}

// utilityName strips a class token's variant prefixes: everything up to the
// last colon outside brackets, since an arbitrary variant (`[&:hover]:`) or
// property (`[white-space:pre]`) carries colons of its own.
func utilityName(tok []byte) []byte {
	depth, start := 0, 0

	for i, c := range tok {
		switch c {
		case '[':
			depth++
		case ']':
			depth = max(depth-1, 0)
		case ':':
			if depth == 0 {
				start = i + 1
			}
		}
	}

	return tok[start:]
}

// stylePreservesWhitespace reports whether any white-space or
// white-space-collapse declaration in a CSS declaration list preserves. Any, not
// the one the cascade picks: a collapsed body kept verbatim renders the same,
// while a preserved body reflowed does not.
func stylePreservesWhitespace(decls []byte) bool {
	lower := asciiLower(decls)
	if !bytes.Contains(lower, []byte("white-space")) {
		return false
	}

	for decl := range bytes.SplitSeq(stripCSSComments(lower), []byte(";")) {
		prop, value, ok := bytes.Cut(decl, []byte(":"))
		if !ok {
			continue
		}

		switch string(bytes.TrimSpace(prop)) {
		case "white-space", "white-space-collapse":
			if valuePreservesWhitespace(value) {
				return true
			}
		}
	}

	return false
}

// unresolvedValueMarkers start a value the source does not spell out: a custom
// property, a Blade echo or a PHP tag, any of which may resolve to a preserving
// keyword.
var unresolvedValueMarkers = [][]byte{
	[]byte("var("),
	[]byte("{{"),
	[]byte("{!!"),
	[]byte("<?"),
}

func valuePreservesWhitespace(value []byte) bool {
	for _, marker := range unresolvedValueMarkers {
		if bytes.Contains(value, marker) {
			return true
		}
	}

	keywords, _, _ := bytes.Cut(value, []byte("!"))

	for word := range bytes.FieldsSeq(keywords) {
		if whitespacePreservingValues[string(word)] {
			return true
		}
	}

	return false
}

func stripCSSComments(css []byte) []byte {
	if !bytes.Contains(css, []byte("/*")) {
		return css
	}

	var out []byte

	for {
		before, after, found := bytes.Cut(css, []byte("/*"))
		out = append(out, before...)

		if !found {
			return out
		}

		out = append(out, ' ')

		if _, css, found = bytes.Cut(after, []byte("*/")); !found {
			return out
		}
	}
}
