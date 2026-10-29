package markdown

import "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"

// Position is php.Position, shared so the whole parsing stack uses one
// coordinate type.
type Position = php.Position

// Span is a half-open byte range [Start, End) in source.
type Span = php.Span

// Error is a parse diagnostic.
type Error = php.Error

// Node is any Markdown block node.
type Node interface {
	Span() Span
}

// File is a parsed Markdown source.
type File struct {
	Src    []byte
	Blocks []Node
}

// Frontmatter is an opaque metadata block fenced by --- (YAML) or +++ (TOML).
type Frontmatter struct {
	FenceChar  byte
	OpenFence  Span
	Content    []byte
	CloseFence Span
}

// Span returns the source extent of the frontmatter block.
func (f *Frontmatter) Span() Span {
	return Span{Start: f.OpenFence.Start, End: f.CloseFence.End}
}

// Paragraph is a run of text lines; Content is their raw source.
type Paragraph struct {
	Start   Position
	End     Position
	Content []byte
}

// Span returns the source extent of the paragraph.
func (p *Paragraph) Span() Span {
	return Span{Start: p.Start, End: p.End}
}

// ATXHeading is a `#` to `######` heading on one line.
type ATXHeading struct {
	Start   Position
	End     Position
	Level   int
	Content []byte
}

// Span returns the source extent of the heading.
func (h *ATXHeading) Span() Span {
	return Span{Start: h.Start, End: h.End}
}

// SetextHeading is paragraph text underlined by = (level 1) or - (level 2).
type SetextHeading struct {
	Start         Position
	End           Position
	Level         int
	Content       []byte
	UnderlineSpan Span
}

// Span returns the source extent of the heading.
func (h *SetextHeading) Span() Span {
	return Span{Start: h.Start, End: h.End}
}

// ThematicBreak is a line of three or more matching `-`, `_` or `*`.
type ThematicBreak struct {
	Start Position
	End   Position
	Char  byte
}

// Span returns the source extent of the thematic break.
func (t *ThematicBreak) Span() Span {
	return Span{Start: t.Start, End: t.End}
}

// FencedCodeBlock is a backtick or tilde fenced block; CloseFence is zero when
// the block runs unclosed to the end of input.
type FencedCodeBlock struct {
	FenceChar  byte
	OpenFence  Span
	Info       []byte
	Content    []byte
	CloseFence Span
	End        Position
}

// Span returns the source extent of the fenced code block.
func (b *FencedCodeBlock) Span() Span {
	return Span{Start: b.OpenFence.Start, End: b.End}
}

// IndentedCodeBlock is a block of lines indented four or more columns.
type IndentedCodeBlock struct {
	Start   Position
	End     Position
	Content []byte
}

// Span returns the source extent of the indented code block.
func (b *IndentedCodeBlock) Span() Span {
	return Span{Start: b.Start, End: b.End}
}

// HTMLBlock is a run of lines matching a CommonMark HTML block; Content is kept
// opaque.
type HTMLBlock struct {
	Start   Position
	End     Position
	Content []byte
}

// Span returns the source extent of the HTML block.
func (b *HTMLBlock) Span() Span {
	return Span{Start: b.Start, End: b.End}
}

// LinkRefDef is a one-line `[label]: destination "title"` definition; Title is
// nil when none is given.
type LinkRefDef struct {
	Start Position
	End   Position
	Label []byte
	Dest  []byte
	Title []byte
}

// Span returns the source extent of the link reference definition.
func (d *LinkRefDef) Span() Span {
	return Span{Start: d.Start, End: d.End}
}

// Blockquote is a `>` block; Content is the body with its markers stripped and
// Body is Content parsed, with offsets into Content.
type Blockquote struct {
	Start   Position
	End     Position
	Content []byte
	Body    *File
}

// Span returns the source extent of the blockquote.
func (b *Blockquote) Span() Span {
	return Span{Start: b.Start, End: b.End}
}

// List is a run of items sharing a marker; Loose is set when a blank line
// separates two items or any item is loose.
type List struct {
	Start       Position
	End         Position
	Ordered     bool
	Marker      byte
	StartNumber int
	Items       []*ListItem
	Loose       bool
}

// Span returns the source extent of the list.
func (l *List) Span() Span {
	return Span{Start: l.Start, End: l.End}
}

// TableAlign is a GFM column alignment.
type TableAlign int

const (
	TableAlignNone TableAlign = iota
	TableAlignLeft
	TableAlignRight
	TableAlignCenter
)

// TableCell is one GFM table cell; Content is trimmed, with \| escapes decoded
// to a literal |.
type TableCell struct {
	Content []byte
}

// Table is a GFM table; Aligns and every row match Header's length, with short
// rows padded and long ones truncated.
type Table struct {
	Start  Position
	End    Position
	Aligns []TableAlign
	Header []TableCell
	Rows   [][]TableCell
}

// Span returns the source extent of the table.
func (t *Table) Span() Span {
	return Span{Start: t.Start, End: t.End}
}

// ListItem is one List entry; Body is Content parsed like a Blockquote's.
// Loose is set only by a blank line directly between two of Body's own blocks
// (mdast `spread`).
type ListItem struct {
	Start       Position
	End         Position
	MarkerBytes []byte
	Content     []byte
	Body        *File
	Loose       bool
}

// Span returns the source extent of the list item.
func (i *ListItem) Span() Span {
	return Span{Start: i.Start, End: i.End}
}
