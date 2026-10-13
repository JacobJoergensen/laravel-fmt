package css

import (
	"bytes"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/tailwind"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/css"
)

// trimCSSSpace exists because bytes.TrimSpace would also strip a vertical
// tab or NBSP, which CSS keeps as content, so the output would re-parse
// with a different structure.
func trimCSSSpace(b []byte) []byte {
	return bytes.Trim(b, " \t\n\r\f")
}

// collapseCSSSpace only changes a malformed property, keeping it on one line
// so a stray newline cannot compound a <style> body's indentation.
func collapseCSSSpace(b []byte) []byte {
	trimmed := trimCSSSpace(b)
	if !bytes.ContainsAny(trimmed, " \t\n\r\f") {
		return trimmed
	}

	var out bytes.Buffer

	space := false

	for _, c := range trimmed {
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' {
			space = true

			continue
		}

		if space {
			out.WriteByte(' ')

			space = false
		}

		out.WriteByte(c)
	}

	return out.Bytes()
}

// normalizeQuotes swaps a string's delimiters only when it holds no backslash
// and no target quote, so the swap never changes meaning or needs escaping.
func normalizeQuotes(b []byte, target byte) []byte {
	if bytes.IndexByte(b, '\'') < 0 && bytes.IndexByte(b, '"') < 0 {
		return b
	}

	out := make([]byte, 0, len(b))
	i := 0

	for i < len(b) {
		c := b[i]

		if c == '/' && i+1 < len(b) && b[i+1] == '*' {
			end := commentEnd(b, i)
			out = append(out, b[i:end]...)
			i = end

			continue
		}

		if c == '\\' && i+1 < len(b) {
			out = append(out, c, b[i+1])
			i += 2

			continue
		}

		if c != '\'' && c != '"' {
			out = append(out, c)
			i++

			continue
		}

		end, hasBackslash, hasTarget := scanCSSString(b, i, target)

		if end > len(b)-1 {
			out = append(out, b[i:]...)

			break
		}

		if c != target && !hasBackslash && !hasTarget {
			out = append(out, target)
			out = append(out, b[i+1:end]...)
			out = append(out, target)
		} else {
			out = append(out, b[i:end+1]...)
		}

		i = end + 1
	}

	return out
}

func commentEnd(b []byte, i int) int {
	for j := i + 2; j+1 < len(b); j++ {
		if b[j] == '*' && b[j+1] == '/' {
			return j + 2
		}
	}

	return len(b)
}

const maxHexEscapeDigits = 6

// The one whitespace that ends a hex escape belongs to the escape, so it is
// not a separator.
func escapeEnd(b []byte, i int) int {
	j := i + 1
	if j >= len(b) {
		return j
	}

	if !isHexDigit(b[j]) {
		return j + 1
	}

	for j < len(b) && j <= i+maxHexEscapeDigits && isHexDigit(b[j]) {
		j++
	}

	if j+1 < len(b) && b[j] == '\r' && b[j+1] == '\n' {
		return j + 2
	}

	if j < len(b) && isCSSSpace(b[j]) {
		return j + 1
	}

	return j
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func scanCSSString(b []byte, i int, target byte) (end int, hasBackslash, hasTarget bool) {
	quote := b[i]

	for j := i + 1; j < len(b); j++ {
		switch b[j] {
		case '\\':
			hasBackslash = true
			j++
		case quote:
			return j, hasBackslash, hasTarget
		case target:
			hasTarget = true
		}
	}

	return len(b), hasBackslash, hasTarget
}

func emitFile(w *formatter.Writer, file *parser.File, cfg *config.File) {
	var prev parser.Node

	for i, node := range file.Nodes {
		if i > 0 {
			emitInterNodeGap(w, file.Nodes[i-1], node)
		}

		if isIgnoreDirectiveComment(prev) {
			emitNodeVerbatim(w, file.Src, node)
		} else {
			emitNode(w, file.Src, node, cfg)
		}

		prev = node
	}

	if len(file.Nodes) > 0 {
		w.Newline()
	}
}

func emitNode(w *formatter.Writer, src []byte, node parser.Node, cfg *config.File) {
	switch n := node.(type) {
	case *parser.Rule:
		emitRule(w, src, n, cfg)
	case *parser.AtRule:
		emitAtRule(w, src, n, cfg)
	case *parser.Declaration:
		emitDeclaration(w, n, cfg)
	case *parser.Comment:
		emitComment(w, src, n)
	default:
		emitNodeVerbatim(w, src, node)
	}
}

func emitInterNodeGap(w *formatter.Writer, prev, next parser.Node) {
	w.Newline()

	if next.Span().Start.Line-prev.Span().End.Line > 1 {
		w.Newline()
	}
}

func emitRule(w *formatter.Writer, src []byte, r *parser.Rule, cfg *config.File) {
	emitSelector(w, r.Selector)
	w.Space()
	emitBlockBody(w, src, r.Children, cfg)
}

func emitAtRule(w *formatter.Writer, src []byte, a *parser.AtRule, cfg *config.File) {
	w.Write("@")
	w.WriteBytes(a.Name)

	var prelude []byte

	if len(a.Prelude) > 0 {
		prelude = trimCSSSpace(a.Prelude)

		if cfg.Tailwind && bytes.EqualFold(a.Name, []byte("apply")) {
			prelude = []byte(tailwind.Sort(cfg.TailwindTheme, string(prelude)))
		} else if !bytes.EqualFold(a.Name, []byte("charset")) {
			// @charset is spec-locked to double quotes, so it is left untouched.
			prelude = normalizeQuotes(prelude, formatter.MarkupQuoteTarget(cfg))
			prelude = normalizePrelude(prelude, preludeHasFeatureColons(a.Name))
		}

		w.Space()
		w.WriteBytes(prelude)
	}

	if !a.Block {
		terminate(w, prelude)

		return
	}

	w.Space()
	emitBlockBody(w, src, a.Children, cfg)
}

func emitBlockBody(w *formatter.Writer, src []byte, children []parser.Node, cfg *config.File) {
	if len(children) == 0 {
		w.Write("{}")

		return
	}

	w.Write("{")
	w.Indent()

	var prev parser.Node

	for i, child := range children {
		if i > 0 {
			emitInterNodeGap(w, children[i-1], child)
		} else {
			w.Newline()
		}

		if isIgnoreDirectiveComment(prev) {
			emitNodeVerbatim(w, src, child)
		} else {
			emitNode(w, src, child, cfg)
		}

		prev = child
	}

	w.Dedent()
	w.Newline()
	w.Write("}")
}

func emitDeclaration(w *formatter.Writer, d *parser.Declaration, cfg *config.File) {
	property := string(collapseCSSSpace(d.Property))
	value := normalizeQuotes(trimCSSSpace(d.Value), formatter.MarkupQuoteTarget(cfg))
	value = normalizeValue(value)
	parts := splitCommaList(value)

	if shouldEmitDeclInline(w, cfg, property, parts, d.Important) {
		emitInlineDeclaration(w, property, parts, d.Important)

		return
	}

	if allAtomic(parts) {
		emitFilledDeclaration(w, property, parts, d.Important, cfg.PrintWidth)

		return
	}

	emitWrappedDeclaration(w, property, parts, d.Important)
}

func allAtomic(parts [][]byte) bool {
	for _, p := range parts {
		depth := 0
		quote := byte(0)

		for i := 0; i < len(p); i++ {
			c := p[i]

			switch {
			case quote != 0:
				switch c {
				case '\\':
					i++
				case quote:
					quote = 0
				}
			case c == '"' || c == '\'':
				quote = c
			case c == '(':
				depth++
			case c == ')':
				depth--
			case depth == 0 && isCSSSpace(c):
				return false
			}
		}
	}

	return true
}

func emitFilledDeclaration(w *formatter.Writer, property string, parts [][]byte, important bool, width int) {
	w.Write(property)
	separate(w, []byte(property), ":")
	w.Indent()
	w.Newline()

	for i, p := range parts {
		if i > 0 {
			w.Write(",")

			tail := len(",")
			if i == len(parts)-1 && important {
				tail = len(" !important;")
			}

			if w.Column()+len(" ")+len(p)+tail > width {
				w.Newline()
			} else {
				w.Space()
			}
		}

		w.WriteBytes(p)
	}

	w.Dedent()
	terminateAfterValue(w, parts, important)
}

func shouldEmitDeclInline(w *formatter.Writer, cfg *config.File, property string, parts [][]byte, important bool) bool {
	if len(parts) < 2 {
		return true
	}

	inline := len(property) + len(": ") + len(";")

	for i, p := range parts {
		if i > 0 {
			inline += len(", ")
		}

		inline += len(p)
	}

	if important {
		inline += len(" !important")
	}

	return w.Column()+inline <= cfg.PrintWidth
}

func emitInlineDeclaration(w *formatter.Writer, property string, parts [][]byte, important bool) {
	w.Write(property)
	separate(w, []byte(property), ":")
	w.Space()

	for i, p := range parts {
		if i > 0 {
			w.Write(", ")
		}

		w.WriteBytes(p)
	}

	terminateAfterValue(w, parts, important)
}

func emitWrappedDeclaration(w *formatter.Writer, property string, parts [][]byte, important bool) {
	w.Write(property)
	separate(w, []byte(property), ":")
	w.Indent()

	for i, p := range parts {
		w.Newline()
		w.WriteBytes(p)

		if i < len(parts)-1 {
			w.Write(",")
		}
	}

	w.Dedent()
	terminateAfterValue(w, parts, important)
}

func terminateAfterValue(w *formatter.Writer, parts [][]byte, important bool) {
	if important {
		w.Space()
		w.Write("!important")
		w.Write(";")

		return
	}

	last := []byte(nil)
	if len(parts) > 0 {
		last = parts[len(parts)-1]
	}

	terminate(w, last)
}

func separate(w *formatter.Writer, last []byte, delim string) {
	if endsWithUnpairedBackslash(last) {
		w.Space()
	}

	w.Write(delim)
}

func terminate(w *formatter.Writer, last []byte) {
	separate(w, last, ";")
}

func endsWithUnpairedBackslash(b []byte) bool {
	n := 0

	for i := len(b) - 1; i >= 0 && b[i] == '\\'; i-- {
		n++
	}

	return n%2 == 1
}

func emitComment(w *formatter.Writer, src []byte, c *parser.Comment) {
	formatter.EmitVerbatim(w, src, c.Open.Start.Offset, c.Close.End.Offset)
}

func emitSelector(w *formatter.Writer, sel []byte) {
	parts := splitCommaList(sel)

	for i, p := range parts {
		if i > 0 {
			w.Write(",")
			w.Newline()
		}

		w.WriteBytes(normalizeSelectorSpacing(p))
	}
}

func splitCommaList(sel []byte) [][]byte {
	var parts [][]byte

	depth := 0
	inStr := byte(0)
	start := 0

	for i := 0; i < len(sel); i++ {
		c := sel[i]

		if inStr != 0 {
			if c == '\\' && i+1 < len(sel) {
				i++

				continue
			}

			if c == inStr {
				inStr = 0
			}

			continue
		}

		switch c {
		case '\\':
			i = escapeEnd(sel, i) - 1
		case '"', '\'':
			inStr = c
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, trimListPart(sel[start:i]))
				start = i + 1
			}
		}
	}

	parts = append(parts, trimListPart(sel[start:]))

	return parts
}

// trimListPart keeps a whitespace that an unpaired trailing backslash
// escapes: trimmed away, the backslash would escape the following comma.
func trimListPart(b []byte) []byte {
	start := len(b) - len(bytes.TrimLeft(b, " \t\n\r\f"))
	end := start + len(trimCSSSpace(b))

	if end < len(b) && endsWithUnpairedBackslash(b[start:end]) {
		end++
	}

	return b[start:end]
}

func isMathFunc(name []byte) bool {
	switch {
	case bytes.EqualFold(name, []byte("calc")),
		bytes.EqualFold(name, []byte("min")),
		bytes.EqualFold(name, []byte("max")),
		bytes.EqualFold(name, []byte("clamp")):
		return true
	default:
		return false
	}
}

func isUnquotedURL(fn, b []byte, i int) bool {
	if !bytes.EqualFold(fn, []byte("url")) {
		return false
	}

	for ; i < len(b); i++ {
		switch b[i] {
		case ' ', '\t', '\n', '\r', '\f':
			continue
		case '"', '\'':
			return false
		}

		return true
	}

	return false
}

func urlTokenEnd(b []byte, i int) int {
	for ; i < len(b); i++ {
		if b[i] == '\\' && i+1 < len(b) {
			i++

			continue
		}

		if b[i] == ')' {
			return i + 1
		}
	}

	return len(b)
}

func trailingIdent(bs []byte) []byte {
	start := len(bs)

	for start > 0 && isIdentByte(bs[start-1]) {
		start--
	}

	return bs[start:]
}

// normalizeValue leaves + and - alone even in calc: they are also signs and
// identifier characters, and valid calc already spaces them. Top-level commas
// belong to the caller's layout.
func normalizeValue(b []byte) []byte {
	if len(b) == 0 {
		return b
	}

	var out bytes.Buffer

	inStr := byte(0)
	pendingSpace := false

	var mathStack []bool

	flush := func() {
		if pendingSpace {
			out.WriteByte(' ')
			pendingSpace = false
		}
	}

	inMath := func() bool {
		return len(mathStack) > 0 && mathStack[len(mathStack)-1]
	}

	for i := 0; i < len(b); i++ {
		c := b[i]

		if inStr != 0 {
			out.WriteByte(c)

			if c == '\\' && i+1 < len(b) {
				out.WriteByte(b[i+1])
				i++

				continue
			}

			if c == inStr {
				inStr = 0
			}

			continue
		}

		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if out.Len() > 0 {
				pendingSpace = true
			}

			continue
		}

		switch {
		case c == '\\':
			flush()

			end := escapeEnd(b, i)
			out.Write(b[i:end])
			i = end - 1
		case c == '"' || c == '\'':
			flush()
			inStr = c
			out.WriteByte(c)
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			flush()

			end := commentEnd(b, i)
			out.Write(b[i:end])
			i = end - 1
		case c == '(':
			name := trailingIdent(out.Bytes())

			// One space after the `base64,` of a data URI turns the url-token into a
			// bad-url-token, so the browser drops the declaration and the asset
			// silently disappears. The token is opaque, like a string or a comment.
			if isUnquotedURL(name, b, i+1) {
				flush()

				end := urlTokenEnd(b, i+1)
				out.Write(b[i:end])
				i = end - 1

				continue
			}

			math := inMath()
			if len(name) > 0 {
				math = isMathFunc(name)
			}

			flush()
			mathStack = append(mathStack, math)
			out.WriteByte(c)
		case c == ')':
			flush()

			if len(mathStack) > 0 {
				mathStack = mathStack[:len(mathStack)-1]
			}

			out.WriteByte(c)
		case c == ',' && len(mathStack) > 0:
			pendingSpace = false
			out.WriteByte(c)
			pendingSpace = true
		case (c == '*' || c == '/') && inMath():
			spaceBeforeOperator(&out)

			out.WriteByte(c)
			pendingSpace = true
		default:
			flush()
			out.WriteByte(c)
		}
	}

	return out.Bytes()
}

func isIdentByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_'
}

// preludeHasFeatureColons is true for the conditional-group rules whose prelude
// has `feature: value`; selector preludes (`@custom-variant`'s `&:where`) must not
// be spaced.
func preludeHasFeatureColons(name []byte) bool {
	return bytes.EqualFold(name, []byte("media")) ||
		bytes.EqualFold(name, []byte("supports")) ||
		bytes.EqualFold(name, []byte("container"))
}

// normalizePrelude spaces a : only inside a grouping paren, one whose ( is
// not glued to an identifier: in a function call such as selector() or at
// the top level, a : is a pseudo-class.
func normalizePrelude(b []byte, featureColons bool) []byte {
	var out bytes.Buffer

	inStr := byte(0)
	pendingSpace := false

	var parenIsFunc []bool

	flush := func() {
		if pendingSpace {
			out.WriteByte(' ')
			pendingSpace = false
		}
	}

	inGroupingParen := func() bool {
		return len(parenIsFunc) > 0 && !parenIsFunc[len(parenIsFunc)-1]
	}

	for i := 0; i < len(b); i++ {
		c := b[i]

		if inStr != 0 {
			out.WriteByte(c)

			if c == '\\' && i+1 < len(b) {
				out.WriteByte(b[i+1])
				i++

				continue
			}

			if c == inStr {
				inStr = 0
			}

			continue
		}

		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if out.Len() > 0 {
				pendingSpace = true
			}

			continue
		}

		switch {
		case c == '\\':
			flush()

			end := escapeEnd(b, i)
			out.Write(b[i:end])
			i = end - 1
		case c == '"' || c == '\'':
			flush()
			inStr = c
			out.WriteByte(c)
		case c == '(':
			bs := out.Bytes()
			fn := !pendingSpace && len(bs) > 0 && isIdentByte(bs[len(bs)-1])

			if fn && isUnquotedURL(trailingIdent(bs), b, i+1) {
				flush()

				end := urlTokenEnd(b, i+1)
				out.Write(b[i:end])
				i = end - 1

				continue
			}

			flush()
			parenIsFunc = append(parenIsFunc, fn)
			out.WriteByte(c)
		case c == '[':
			flush()
			parenIsFunc = append(parenIsFunc, true)
			out.WriteByte(c)
		case c == ')' || c == ']':
			flush()

			if len(parenIsFunc) > 0 {
				parenIsFunc = parenIsFunc[:len(parenIsFunc)-1]
			}

			out.WriteByte(c)
		case c == ',' || (c == ':' && featureColons && inGroupingParen()):
			pendingSpace = false
			out.WriteByte(c)
			pendingSpace = true
		default:
			flush()
			out.WriteByte(c)
		}
	}

	return out.Bytes()
}

// normalizeSelectorSpacing spaces only top-level combinators: inside parens or
// brackets, >, + and ~ belong to an an+b expression, a pseudo-class argument
// or attribute syntax.
func normalizeSelectorSpacing(sel []byte) []byte {
	var out bytes.Buffer

	depth := 0
	inStr := byte(0)
	pendingSpace := false

	flush := func() {
		if pendingSpace {
			out.WriteByte(' ')
			pendingSpace = false
		}
	}

	for i := 0; i < len(sel); i++ {
		c := sel[i]

		if inStr != 0 {
			out.WriteByte(c)

			if c == '\\' && i+1 < len(sel) {
				out.WriteByte(sel[i+1])
				i++

				continue
			}

			if c == inStr {
				inStr = 0
			}

			continue
		}

		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if out.Len() > 0 {
				pendingSpace = true
			}

			continue
		}

		if c == '\\' {
			flush()

			end := escapeEnd(sel, i)
			out.Write(sel[i:end])
			i = end - 1

			continue
		}

		if depth == 0 && (c == '>' || c == '+' || c == '~') {
			spaceBeforeOperator(&out)

			out.WriteByte(c)
			pendingSpace = true

			continue
		}

		flush()

		switch c {
		case '"', '\'':
			inStr = c
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		}

		out.WriteByte(c)
	}

	return out.Bytes()
}

// A second space would grow on every pass: once inserted, it becomes the
// whitespace that ends a hex escape.
func spaceBeforeOperator(out *bytes.Buffer) {
	b := out.Bytes()
	if len(b) == 0 || isCSSSpace(b[len(b)-1]) {
		return
	}

	out.WriteByte(' ')
}
