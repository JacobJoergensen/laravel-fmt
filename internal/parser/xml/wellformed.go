package xml

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"unicode"
	"unicode/utf8"

	parserpkg "github.com/JacobJoergensen/laravel-fmt/internal/parser"
)

var predefinedEntities = []string{"lt", "gt", "amp", "apos", "quot"}

// A lookup keeps checkChars' loop tight; it runs once per byte of every file.
var unremarkableByte = func() (table [256]bool) {
	for c := range table {
		table[c] = c >= ' ' && c < utf8.RuneSelf || c == '\t' || c == '\n' || c == '\r'
	}

	return table
}()

// A file is UTF-8 unless its declaration names another encoding. Then only
// ASCII control bytes are checked, since an ASCII-compatible encoding cannot
// make them allowed.
func (p *xmlParser) checkChars() {
	utf8Text := declaresUTF8(p.Src)

	for i := 0; i < len(p.Src); {
		c := p.Src[i]

		if unremarkableByte[c] {
			i++

			continue
		}

		if c < utf8.RuneSelf {
			p.addError(p.positionFrom(fileStart, i), fmt.Sprintf("character %U is not allowed in XML", c))

			return
		}

		if !utf8Text {
			i++

			continue
		}

		r, size := utf8.DecodeRune(p.Src[i:])

		if r == utf8.RuneError && size == 1 {
			p.addError(p.positionFrom(fileStart, i), fmt.Sprintf("invalid UTF-8 byte %#02x", c))

			return
		}

		if !isChar(r) {
			p.addError(p.positionFrom(fileStart, i), fmt.Sprintf("character %U is not allowed in XML", r))

			return
		}

		i += size
	}
}

// XML 1.0 production [23] XMLDecl, between `<?xml` and `?>`.
var xmlDeclaration = regexp.MustCompile(`^[ \t\r\n]+version[ \t\r\n]*=[ \t\r\n]*(?:"1\.[0-9]+"|'1\.[0-9]+')` +
	`(?:[ \t\r\n]+encoding[ \t\r\n]*=[ \t\r\n]*(?:"([A-Za-z][A-Za-z0-9._-]*)"|'([A-Za-z][A-Za-z0-9._-]*)'))?` +
	`(?:[ \t\r\n]+standalone[ \t\r\n]*=[ \t\r\n]*(?:"(?:yes|no)"|'(?:yes|no)'))?[ \t\r\n]*$`)

func declaresUTF8(src []byte) bool {
	rest, hasDecl := bytes.CutPrefix(src, []byte("<?xml"))
	if !hasDecl {
		return true
	}

	decl, _, _ := bytes.Cut(rest, []byte("?>"))

	m := xmlDeclaration.FindSubmatch(decl)
	if m == nil {
		return true
	}

	encoding := slices.Concat(m[1], m[2])

	return len(encoding) == 0 || bytes.EqualFold(encoding, []byte("UTF-8")) || bytes.EqualFold(encoding, []byte("UTF8"))
}

// isChar is XML 1.0 production [2] Char.
func isChar(r rune) bool {
	return r == '\t' || r == '\n' || r == '\r' || r >= 0x20 && r <= 0xD7FF ||
		r >= 0xE000 && r <= 0xFFFD || r >= 0x10000 && r <= 0x10FFFF
}

// A named reference must be declared (XML 1.0 section 4.1), so without a
// DOCTYPE that can declare one only the predefined five are allowed.
func (p *xmlParser) skipReference() {
	start := p.Pos
	p.Advance()

	if p.Peek() == '#' {
		p.Advance()

		if !p.skipCharReference() {
			p.addError(start, "invalid character reference")
		}

		return
	}

	name := p.readName()

	if len(name) == 0 || p.Peek() != ';' {
		p.addError(start, "& must start a reference like &amp;")

		return
	}

	p.Advance()

	if !p.declaresEntities && !slices.Contains(predefinedEntities, string(name)) {
		p.addError(start, "undeclared entity &"+string(name)+";")
	}
}

func (p *xmlParser) skipCharReference() bool {
	base := 10

	if p.Peek() == 'x' {
		base = 16
		p.Advance()
	}

	start := p.Pos.Offset

	for isDigitIn(p.Peek(), base) {
		p.Advance()
	}

	digits := p.Src[start:p.Pos.Offset]

	if len(digits) == 0 || p.Peek() != ';' {
		return false
	}

	p.Advance()

	n, err := strconv.ParseUint(string(digits), base, 32)

	return err == nil && n <= unicode.MaxRune && isChar(rune(n))
}

func isDigitIn(c byte, base int) bool {
	switch {
	case c >= '0' && c <= '9':
		return true
	case base == 16:
		return c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
	}

	return false
}

// After an earlier error it adds none: that error already explains the file.
func (p *xmlParser) checkDocument(nodes []Node) {
	if len(p.errors) > 0 {
		return
	}

	var root *Element

	blank, seenDoctype := true, false

	for _, node := range nodes {
		switch n := node.(type) {
		case *Element:
			if root != nil {
				p.addError(n.Open.Start, "only one root element is allowed")
			}

			root = n
		case *Text:
			if len(bytes.Trim(n.Text, Whitespace)) > 0 {
				p.addError(n.Start, "text is not allowed outside the root element")
			}

			continue
		case *CDATA:
			p.addError(n.Open.Start, "CDATA is not allowed outside the root element")
		case *Doctype:
			if root != nil || seenDoctype {
				p.addError(n.Open.Start, "DOCTYPE must come once, before the root element")
			}

			seenDoctype = true
		}

		blank = false
	}

	if root == nil && !blank {
		p.addError(p.Pos, "missing root element")
	}
}

var fileStart = Position{Line: 1, Column: 1}

// Walking from the file start for every error would make many errors quadratic.
func (p *xmlParser) positionFrom(from Position, offset int) Position {
	c := parserpkg.Cursor{Src: p.Src, Pos: from}
	c.AdvanceN(offset - from.Offset)

	return c.Pos
}

func declaresEntities(body []byte) bool {
	if bytes.IndexByte(body, '[') >= 0 {
		return true
	}

	fields := bytes.Fields(body)

	return len(fields) > 1 && (string(fields[1]) == "SYSTEM" || string(fields[1]) == "PUBLIC")
}

// A target spelled xml in any case is reserved for the declaration, which only
// the exact `<?xml` opening the file can be.
func (p *xmlParser) checkDeclaration(open Position, target, body []byte) {
	switch {
	case open.Offset > 0 || string(target) != "xml":
		p.addError(open, "the XML declaration must be the first thing in the file, as <?xml")
	case !xmlDeclaration.Match(body):
		p.addError(open, `malformed XML declaration; expected <?xml version="1.0"?>`)
	}
}
