// Package alpine canonicalizes the whitespace between the JavaScript tokens
// of Alpine.js attribute values; string, template, regex, and comment
// contents are atomic and never altered.
package alpine

import (
	"bytes"
	"unicode/utf8"
)

var jsAttrNames = []string{
	"x-data", "x-init", "x-show", "x-text", "x-html", "x-model",
	"x-modelable", "x-effect", "x-if", "x-id", "x-for",
}

// Normalize returns value with canonical spacing between its JavaScript
// tokens. Token bytes are kept verbatim and the result is re-lexed to prove
// the token stream survived, so a fused or split token cannot be emitted.
// ok is false when the value must stay verbatim instead: it is empty or
// multi-line, may hold Blade or HTML-entity syntax, or does not tokenize
// with confidence.
func Normalize(value []byte, spaceAfterNot bool) ([]byte, bool) {
	if !normalizable(value) {
		return nil, false
	}

	toks, ok := lex(value)

	if !ok || len(toks) == 0 {
		return nil, false
	}

	out := render(value, toks, spaceAfterNot)

	if !sameTokens(value, toks, out) {
		return nil, false
	}

	return out, true
}

// BreaksInTemplate reports whether a line break in value falls inside a
// template literal. A value that does not tokenize is assumed to when it
// holds a backtick.
func BreaksInTemplate(value []byte) bool {
	toks, ok := lex(value)
	if !ok {
		return bytes.IndexByte(value, '`') >= 0
	}

	for _, t := range toks {
		if t.kind == kindTemplate && bytes.ContainsAny(value[t.start:t.end], "\n\r") {
			return true
		}
	}

	return false
}

// IsAlpineJSAttr reports whether the attribute name carries a JavaScript
// value Alpine evaluates. The list is a closed allowlist: unknown x-*
// attributes (plugins like x-mask define non-JS value syntax) and the
// directives whose values are not JavaScript (x-ref, x-teleport,
// x-transition class lists) stay out, as does the `:` shorthand, which
// collides with Blade component bound props. Modifier chains (.prevent,
// .lazy) and arguments (x-on:click, x-bind:class, @click) are accepted.
func IsAlpineJSAttr(name []byte) bool {
	base := name

	if i := bytes.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}

	if len(base) > 0 && base[0] == '@' {
		return len(base) > 1
	}

	switch {
	case hasPrefixFold(base, "x-on:"):
		return len(base) > len("x-on:")
	case hasPrefixFold(base, "x-bind:"):
		return len(base) > len("x-bind:")
	}

	for _, n := range jsAttrNames {
		if equalFold(base, n) {
			return true
		}
	}

	return false
}

// normalizable rejects values the lexer must not see: `@` (Blade directives and
// escaped echoes), HTML-entity-shaped sequences (the source may mean
// `&amp;&amp;`, which only the browser decodes), and every ECMAScript whitespace
// character isSpace does not handle.
//
// Non-ASCII whitespace, \v and \f would glue onto an adjacent identifier:
// isSpace knows only space, tab, LF and CR, while isIdentStart admits every
// byte from 0x80. A keyword carrying one stops being that keyword, and
// sameTokens cannot notice, since it re-lexes with the same lexer. LF and CR
// are refused because layout and ASI stay the author's.
func normalizable(value []byte) bool {
	// ECMA-262 Annex B makes `<!--` a single-line comment opener. The lexer
	// reads it as `<`, `!` and `--`, and spacing those apart turns `a<!--b`
	// (`a`, the rest commented out) into `a < !--b`, which decrements b. The
	// other half, `-->`, needs a line terminator first, which is refused below.
	if bytes.Contains(value, []byte("<!--")) {
		return false
	}

	for i := 0; i < len(value); i++ {
		c := value[i]

		switch {
		case c == '@':
			return false
		case c == '&' && isEntityStart(value[i+1:]):
			return false
		case c >= utf8.RuneSelf:
			r, n := utf8.DecodeRune(value[i:])
			if jsWhitespace(r) {
				return false
			}

			i += n - 1
		case c != ' ' && c != '\t' && jsWhitespace(rune(c)):
			return false
		}
	}

	return true
}

// jsWhitespace reports whether r is ECMAScript WhiteSpace or a LineTerminator:
// the Unicode Space_Separator category (space, U+00A0, U+1680, U+2000..U+200A,
// U+202F, U+205F, U+3000) plus tab, vertical tab, form feed, ZWNBSP, and the four
// line terminators CR, LF, U+2028 and U+2029.
func jsWhitespace(r rune) bool {
	switch r {
	case '\t', '\v', '\f', ' ', '\n', '\r',
		0x00A0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}

	return r >= 0x2000 && r <= 0x200A
}

func isEntityStart(rest []byte) bool {
	i := 0

	if i < len(rest) && rest[i] == '#' {
		i++

		if i < len(rest) && (rest[i] == 'x' || rest[i] == 'X') {
			i++
		}
	}

	start := i

	for i < len(rest) && isAlnum(rest[i]) {
		i++
	}

	return i > start && i < len(rest) && rest[i] == ';'
}

func sameTokens(src []byte, toks []token, out []byte) bool {
	outToks, ok := lex(out)

	if !ok || len(outToks) != len(toks) {
		return false
	}

	for i, t := range toks {
		o := outToks[i]

		if o.kind != t.kind || !bytes.Equal(src[t.start:t.end], out[o.start:o.end]) {
			return false
		}
	}

	return true
}

func isAlnum(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func equalFold(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}

	for i := range len(s) {
		if lowerByte(b[i]) != lowerByte(s[i]) {
			return false
		}
	}

	return true
}

func hasPrefixFold(b []byte, s string) bool {
	return len(b) >= len(s) && equalFold(b[:len(s)], s)
}

func lowerByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}

	return c
}
