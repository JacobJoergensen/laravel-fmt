package php

// Doc-native PHPDoc layout: a well-formed `*`-per-line docblock is rebuilt as a
// Doc, so its stars follow the construct's indent and an array shape in a tag
// type breaks on width. The `*` opening each line comes from a
// document.LinePrefix; any other block stays verbatim.

import (
	"bytes"
	"slices"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/document"
	"github.com/JacobJoergensen/laravel-fmt/internal/parser/phpdoc"
)

// docCommentDoc rebuilds a well-formed `*`-per-line docblock as a Doc, and a
// one-line block through singleLineDocComment. A tag type an earlier reflow
// split across lines is rejoined, so the result is idempotent. The second
// result is false for any other block (emit it verbatim).
func docCommentDoc(text []byte) (*document.Doc, bool) {
	// A bare carriage return (a CR that is not part of a CRLF) is a hidden line
	// break the reflow would mangle; leave such a block verbatim so it is stable.
	if hasBareCR(text) {
		return nil, false
	}

	if bytes.IndexByte(text, '\n') < 0 {
		return singleLineDocComment(text)
	}

	rawLines := bytes.Split(text, []byte("\n"))

	if !bytes.Equal(bytes.TrimRight(rawLines[0], " \t\r"), []byte("/**")) {
		return nil, false
	}

	contents := make([][]byte, 0, len(rawLines)-1)

	for _, line := range rawLines[1:] {
		trimmed := bytes.TrimLeft(bytes.TrimRight(line, " \t\r"), " \t")

		if len(trimmed) == 0 || trimmed[0] != '*' {
			return nil, false
		}

		contents = append(contents, trimmed[1:])
	}

	inner := make([]*document.Doc, 0, len(contents)*2)

	for i := 0; i < len(contents); i++ {
		lineDoc, consumed, shape := docTagLine(contents, i)

		if shape {
			inner = append(inner, document.HardLine(), lineDoc)
			i += consumed - 1

			continue
		}

		inner = append(inner, document.HardLine(), document.Text(string(normalizeDocContent(contents[i]))))
	}

	return document.Concat(document.Text("/**"), document.LinePrefix(" *", document.Concat(inner...))), true
}

// singleLineDocComment normalizes the tag type in a one-line `/** ... */` block,
// keeping the rest; a non-canonical or empty block is left verbatim.
func singleLineDocComment(text []byte) (*document.Doc, bool) {
	if !bytes.HasPrefix(text, []byte("/**")) || !bytes.HasSuffix(text, []byte("*/")) || len(text) < 5 {
		return nil, false
	}

	inner := bytes.TrimSpace(text[3 : len(text)-2])
	if len(inner) == 0 {
		return nil, false
	}

	// Only rewrite when the normalization is a fixed point; otherwise the block
	// would keep changing across passes, so leave it verbatim.
	norm := normalizeDocContent(inner)
	if !bytes.Equal(norm, normalizeDocContent(norm)) {
		return nil, false
	}

	return document.Text("/** " + string(norm) + " */"), true
}

func hasBareCR(text []byte) bool {
	for i := range text {
		if text[i] == '\r' && (i+1 >= len(text) || text[i+1] != '\n') {
			return true
		}
	}

	return false
}

// docTagLine reconstructs a type-first tag's type from line i, joining
// continuation lines until brackets balance. It returns the laid-out line, lines
// consumed, and whether the type carries a breakable shape (false = emit line i
// alone).
func docTagLine(contents [][]byte, i int) (*document.Doc, int, bool) {
	content := contents[i]

	prefixEnd := tagTypeStart(content)
	if prefixEnd < 0 {
		return nil, 1, false
	}

	typeText := append([]byte(nil), content[prefixEnd:]...)
	consumed := 1

	// Lines join on a newline, so the parser sees where the author broke the
	// type. A literal still open at a join would carry that newline into its
	// value, so such a type stays as written.
	for i+consumed < len(contents) {
		depth, inLiteral := bracketDepth(typeText)
		if depth <= 0 {
			break
		}

		next := bytes.TrimLeft(contents[i+consumed], " \t")
		if len(next) > 0 && next[0] == '@' {
			break
		}

		if inLiteral {
			return nil, 1, false
		}

		typeText = append(typeText, '\n')
		typeText = append(typeText, next...)
		consumed++
	}

	if depth, _ := bracketDepth(typeText); depth != 0 {
		return nil, 1, false
	}

	parsed, n := phpdoc.ParseTypePrefix(typeText)
	if n == 0 || !typeHasBreakableShape(parsed) {
		return nil, 1, false
	}

	// The joined lines belong to the type only if the type runs onto them; a
	// type that ends on its own line leaves the rest of the block as written.
	suffix := bytes.TrimRight(typeText[n:], " \t")
	if bytes.IndexByte(suffix, '\n') >= 0 {
		return nil, 1, false
	}

	lineDoc := document.Concat(
		document.Text(string(content[:prefixEnd])),
		buildDocType(parsed, 0),
		document.Text(string(suffix)),
	)

	return lineDoc, consumed, true
}

// tagTypeStart returns the offset at which a type-first tag's type begins, or -1
// when the content is not a type-first tag with a type.
func tagTypeStart(content []byte) int {
	i := skipSpaceTab(content, 0)
	if i >= len(content) || content[i] != '@' {
		return -1
	}

	tagEnd := i + 1
	for tagEnd < len(content) && isTagByte(content[tagEnd]) {
		tagEnd++
	}

	tag := content[i:tagEnd]

	// A type-alias tag (@phpstan-type, @psalm-type) carries its type after an alias
	// name and an optional `=`, not directly after the tag.
	if typeAliasTags[string(tag)] {
		nameStart := skipSpaceTab(content, tagEnd)
		nameEnd := skipName(content, nameStart)

		if nameEnd == nameStart {
			return -1
		}

		typeStart := skipSpaceTab(content, nameEnd)
		if typeStart < len(content) && content[typeStart] == '=' {
			typeStart = skipSpaceTab(content, typeStart+1)
		}

		if typeStart >= len(content) {
			return -1
		}

		return typeStart
	}

	if !typeFirstTags[string(tag)] {
		return -1
	}

	typeStart := skipSpaceTab(content, tagEnd)
	if typeStart == tagEnd || typeStart >= len(content) {
		return -1
	}

	return typeStart
}

// bracketDepth returns the net nesting of `{`, `[`, `(`, and `<` over `}`, `]`,
// `)`, and `>` in a type fragment, skipping bytes inside string literals so a
// bracket in a literal type does not unbalance the count. The second result
// reports a literal still open at the end of s.
func bracketDepth(s []byte) (int, bool) {
	depth := 0
	quote := byte(0)

	for i := 0; i < len(s); i++ {
		c := s[i]

		if quote != 0 {
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}

			continue
		}

		switch c {
		case '\'', '"':
			quote = c
		case '{', '[', '(', '<':
			depth++
		case '}', ']', ')', '>':
			depth--
		}
	}

	return depth, quote != 0
}

// buildDocType lays out a docblock type, breaking an array shape it carries
// (directly or in a generic argument) when its single-line form overflows. depth
// is the shape nesting level, setting the indent of broken members.
func buildDocType(t phpdoc.Type, depth int) *document.Doc {
	switch n := t.(type) {
	case *phpdoc.ShapeType:
		return buildDocShape(n, depth)
	case *phpdoc.NullableType:
		return document.Concat(document.Text("?"), buildDocType(n.Inner, depth))
	case *phpdoc.ParenType:
		return document.Concat(document.Text("("), buildDocType(n.Inner, depth), document.Text(")"))
	case *phpdoc.UnionType:
		return buildDocTypeList(n.Members, "|", depth)
	case *phpdoc.IntersectionType:
		return buildDocTypeList(n.Members, "&", depth)
	case *phpdoc.GenericType:
		parts := []*document.Doc{buildDocType(n.Base, depth), document.Text("<")}

		for i, arg := range n.Args {
			if i > 0 {
				parts = append(parts, document.Text(", "))
			}

			if arg.Variance != nil {
				parts = append(parts, document.Text(string(arg.Variance)+" "))
			}

			parts = append(parts, buildDocType(arg.Type, depth))
		}

		parts = append(parts, document.Text(">"))

		return document.Concat(parts...)
	default:
		return document.Text(string(printPHPDocType(t)))
	}
}

// buildDocTypeList joins union or intersection members with sep (no surrounding
// spaces, matching the printer), so a shape carried by a member still breaks while
// the `|null` or `&Other` alternatives stay attached.
func buildDocTypeList(members []phpdoc.Type, sep string, depth int) *document.Doc {
	shared := countShapes(members) >= 2
	parts := make([]*document.Doc, 0, len(members)*2-1)

	for i, m := range members {
		if i > 0 {
			parts = append(parts, document.Text(sep))
		}

		if shape, ok := m.(*phpdoc.ShapeType); ok && shared {
			parts = append(parts, docShapeBody(shape, depth))

			continue
		}

		parts = append(parts, buildDocType(m, depth))
	}

	if shared {
		return document.Group(document.Concat(parts...))
	}

	return document.Concat(parts...)
}

func countShapes(members []phpdoc.Type) int {
	n := 0

	for _, m := range members {
		if _, ok := m.(*phpdoc.ShapeType); ok {
			n++
		}
	}

	return n
}

func buildDocShape(s *phpdoc.ShapeType, depth int) *document.Doc {
	return document.Group(docShapeBody(s, depth))
}

// The spaces after each line's LinePrefix `*` are baked in here so no
// document.Indent shifts the star column.
func docShapeBody(s *phpdoc.ShapeType, depth int) *document.Doc {
	memberPad := document.IfBreak(document.Text(strings.Repeat(" ", docStarGap+docIndentStep*(depth+1))), document.Text(""))
	closePad := document.IfBreak(document.Text(strings.Repeat(" ", docStarGap+docIndentStep*depth)), document.Text(""))

	parts := []*document.Doc{document.Text(string(s.Keyword)), document.Text("{")}

	for i, it := range s.Items {
		if i > 0 {
			parts = append(parts, document.Text(","), document.Line(), memberPad)
		} else {
			parts = append(parts, document.SoftLine(), memberPad)
		}

		if it.Key != nil {
			key := string(it.Key)
			if it.Optional {
				key += "?"
			}

			parts = append(parts, document.Text(key+": "))
		}

		parts = append(parts, buildDocType(it.Value, depth+1))
	}

	if s.HasRest {
		if len(s.Items) > 0 {
			parts = append(parts, document.Text(","), document.Line(), memberPad)
		} else {
			parts = append(parts, document.SoftLine(), memberPad)
		}

		parts = append(parts, document.Text("..."))
	}

	// A trailing comma rides each broken member (PHPStan style); the flat form
	// keeps the comma-space separators printed above with no trailing comma.
	parts = append(parts, document.IfBreak(document.Text(","), document.Text("")), document.SoftLine(), closePad, document.Text("}"))

	if s.Broken {
		parts = append(parts, document.BreakParent())
	}

	return document.Concat(parts...)
}

// PHPStan shape layout: one space after the `*`, four per nesting level.
const (
	docStarGap    = 1
	docIndentStep = 4
)

// normalizeDocContent rewrites the type expression in a docblock line's content
// (the text after the line's star, or the body of a single-line block) into
// canonical PHPDoc spacing. The tag, the $variable, and the description are kept
// byte-for-byte; only the type tokens change. Content that is not a recognized
// type-bearing tag, or whose type does not fully parse, is returned unchanged.
func normalizeDocContent(content []byte) []byte {
	i := skipSpaceTab(content, 0)

	if i >= len(content) || content[i] != '@' {
		return content
	}

	tagEnd := i + 1
	for tagEnd < len(content) && isTagByte(content[tagEnd]) {
		tagEnd++
	}

	tag := content[i:tagEnd]

	switch {
	case methodTags[string(tag)]:
		return normalizeMethodTag(content, tagEnd)
	case typeAliasTags[string(tag)]:
		return normalizeTypeAliasTag(content, tagEnd)
	case templateTags[string(tag)]:
		return normalizeTemplateTag(content, tagEnd)
	case assertTags[string(tag)]:
		return normalizeAssertTag(content, tagEnd)
	case typeFirstTags[string(tag)]:
		typeStart := skipSpaceTab(content, tagEnd)

		if typeStart == tagEnd || typeStart >= len(content) {
			return content
		}

		return normalizeTypeAt(content, typeStart)
	default:
		return content
	}
}

var typeFirstTags = map[string]bool{
	"@param":             true,
	"@param-out":         true,
	"@return":            true,
	"@var":               true,
	"@throws":            true,
	"@property":          true,
	"@property-read":     true,
	"@property-write":    true,
	"@mixin":             true,
	"@phpstan-param":     true,
	"@phpstan-param-out": true,
	"@phpstan-return":    true,
	"@phpstan-var":       true,
	"@psalm-param":       true,
	"@psalm-param-out":   true,
	"@psalm-return":      true,
	"@psalm-var":         true,
}

var typeAliasTags = map[string]bool{
	"@phpstan-type": true,
	"@psalm-type":   true,
}

var templateTags = map[string]bool{
	"@template":                   true,
	"@template-covariant":         true,
	"@template-contravariant":     true,
	"@phpstan-template":           true,
	"@phpstan-template-covariant": true,
	"@psalm-template":             true,
	"@psalm-template-covariant":   true,
}

var methodTags = map[string]bool{
	"@method":         true,
	"@phpstan-method": true,
	"@psalm-method":   true,
}

var assertTags = map[string]bool{
	"@phpstan-assert":          true,
	"@phpstan-assert-if-true":  true,
	"@phpstan-assert-if-false": true,
	"@psalm-assert":            true,
	"@psalm-assert-if-true":    true,
	"@psalm-assert-if-false":   true,
}

// normalizeTypeAt rewrites the type beginning at content[start] into canonical
// spacing, returning content unchanged when no complete type parses there.
func normalizeTypeAt(content []byte, start int) []byte {
	out, _ := rewriteType(content, start)

	return out
}

// rewriteType normalizes the type at content[start] and returns the resulting
// content plus the offset just past that type in it. after is -1 when no
// complete, whitespace-terminated type parses (content is then unchanged), so a
// parse that stopped mid-type is never partially re-spaced.
func rewriteType(content []byte, start int) (out []byte, after int) {
	parsed, n := phpdoc.ParseTypePrefix(content[start:])

	if n == 0 {
		return content, -1
	}

	tail := content[start+n:]
	if len(tail) > 0 && tail[0] != ' ' && tail[0] != '\t' {
		return content, -1
	}

	normalized := printPHPDocType(parsed)

	if bytes.Equal(normalized, content[start:start+n]) {
		return content, start + n
	}

	out = make([]byte, 0, len(content)-n+len(normalized))
	out = append(out, content[:start]...)
	out = append(out, normalized...)
	out = append(out, content[start+n:]...)

	return out, start + len(normalized)
}

// normalizeTemplateTag canonicalizes the bound (after of/as) and the default
// (after =) of a @template declaration, leaving the parameter name verbatim.
func normalizeTemplateTag(content []byte, afterTag int) []byte {
	i := skipSpaceTab(content, afterTag)
	nameEnd := skipName(content, i)

	if nameEnd == i {
		return content
	}

	out := content
	i = skipSpaceTab(out, nameEnd)

	if hasWord(out, i, "of") || hasWord(out, i, "as") {
		var after int
		out, after = rewriteType(out, skipSpaceTab(out, i+len("of")))

		if after < 0 {
			return out
		}

		i = after
	}

	j := skipSpaceTab(out, i)

	if j < len(out) && out[j] == '=' {
		out, _ = rewriteType(out, skipSpaceTab(out, j+1))
	}

	return out
}

// normalizeTypeAliasTag canonicalizes the type definition of a @phpstan-type or
// @psalm-type alias, skipping the alias name and an optional `=`.
func normalizeTypeAliasTag(content []byte, afterTag int) []byte {
	i := skipSpaceTab(content, afterTag)
	nameEnd := skipName(content, i)

	if nameEnd == i {
		return content
	}

	i = skipSpaceTab(content, nameEnd)

	if i < len(content) && content[i] == '=' {
		i = skipSpaceTab(content, i+1)
	}

	if i >= len(content) {
		return content
	}

	return normalizeTypeAt(content, i)
}

// normalizeMethodTag canonicalizes the return type of an @method tag, skipping
// an optional `static` modifier. The method name and its parameter list are
// left verbatim.
func normalizeMethodTag(content []byte, afterTag int) []byte {
	i := skipSpaceTab(content, afterTag)

	if hasWord(content, i, "static") {
		i = skipSpaceTab(content, i+len("static"))
	}

	if i >= len(content) {
		return content
	}

	return normalizeTypeAt(content, i)
}

// normalizeAssertTag canonicalizes the asserted type, preserving an optional
// leading ! negation and the trailing $variable.
func normalizeAssertTag(content []byte, afterTag int) []byte {
	i := skipSpaceTab(content, afterTag)

	if i < len(content) && content[i] == '!' {
		i = skipSpaceTab(content, i+1)
	}

	if i >= len(content) {
		return content
	}

	return normalizeTypeAt(content, i)
}

func typeHasBreakableShape(t phpdoc.Type) bool {
	switch n := t.(type) {
	case *phpdoc.ShapeType:
		return true
	case *phpdoc.NullableType:
		return typeHasBreakableShape(n.Inner)
	case *phpdoc.ParenType:
		return typeHasBreakableShape(n.Inner)
	case *phpdoc.GenericType:
		for _, arg := range n.Args {
			if typeHasBreakableShape(arg.Type) {
				return true
			}
		}
	case *phpdoc.UnionType:
		return slices.ContainsFunc(n.Members, typeHasBreakableShape)
	case *phpdoc.IntersectionType:
		return slices.ContainsFunc(n.Members, typeHasBreakableShape)
	}

	return false
}

func hasWord(b []byte, i int, w string) bool {
	if i+len(w) > len(b) || string(b[i:i+len(w)]) != w {
		return false
	}

	end := i + len(w)

	return end == len(b) || b[end] == ' ' || b[end] == '\t'
}

func isTagByte(c byte) bool {
	return c == '-' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func skipName(b []byte, i int) int {
	if i >= len(b) || !isNameStart(b[i]) {
		return i
	}

	for i < len(b) && (isNameStart(b[i]) || (b[i] >= '0' && b[i] <= '9')) {
		i++
	}

	return i
}

func skipSpaceTab(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\t') {
		i++
	}

	return i
}
