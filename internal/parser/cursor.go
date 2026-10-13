// Package parser holds the shared primitives (Position, Span, Cursor) the
// per-language parsers under internal/parser build on.
package parser

import "bytes"

// Position is a byte offset with 1-based line and column; Column is bytes, not
// runes (the go/token convention).
type Position struct {
	Offset int
	Line   int
	Column int
}

// Span is a half-open byte range [Start, End) in source.
type Span struct {
	Start Position
	End   Position
}

// Cursor walks raw source bytes tracking line and column; its fields are
// exported so embedders can save and restore Pos for lookahead.
type Cursor struct {
	Src []byte
	Pos Position
}

// Eof reports whether the cursor has consumed every byte of Src.
func (c *Cursor) Eof() bool {
	return c.Pos.Offset >= len(c.Src)
}

// Peek returns the byte at the cursor, or 0 at end-of-source, so a NUL byte
// in Src reads the same as the end.
func (c *Cursor) Peek() byte {
	if c.Eof() {
		return 0
	}

	return c.Src[c.Pos.Offset]
}

// PeekAt returns the byte n positions ahead, or 0 past end-of-source.
func (c *Cursor) PeekAt(n int) byte {
	if c.Pos.Offset+n >= len(c.Src) {
		return 0
	}

	return c.Src[c.Pos.Offset+n]
}

// Advance consumes one byte; only LF starts a new line, so a lone CR advances
// the column.
func (c *Cursor) Advance() {
	if c.Eof() {
		return
	}

	if c.Src[c.Pos.Offset] == '\n' {
		c.Pos.Line++
		c.Pos.Column = 1
	} else {
		c.Pos.Column++
	}

	c.Pos.Offset++
}

// AdvanceN consumes up to n bytes, stopping at end-of-source.
func (c *Cursor) AdvanceN(n int) {
	for range n {
		c.Advance()
	}
}

// StartsWith reports whether Src at the cursor begins with s.
func (c *Cursor) StartsWith(s string) bool {
	end := c.Pos.Offset + len(s)

	if end > len(c.Src) {
		return false
	}

	return string(c.Src[c.Pos.Offset:end]) == s
}

// StartsWithFold matches s against Src at the cursor, case-insensitively.
func (c *Cursor) StartsWithFold(s string) bool {
	end := c.Pos.Offset + len(s)

	if end > len(c.Src) {
		return false
	}

	return bytes.EqualFold(c.Src[c.Pos.Offset:end], []byte(s))
}
