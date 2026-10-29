package xml

import parserpkg "github.com/JacobJoergensen/laravel-fmt/internal/parser"

// Position is parser.Position.
type Position = parserpkg.Position

// Span is a half-open byte range [Start, End) in source.
type Span = parserpkg.Span

// Error is a parse diagnostic.
type Error = parserpkg.Error

// Node is any XML node.
type Node interface {
	Span() Span
}

// File is a parsed XML document.
type File struct {
	Src   []byte
	Nodes []Node
}

// Element is an XML element; CloseFinal is zero when the end tag is missing,
// which the parser reports as an error.
type Element struct {
	Open       Span
	Name       []byte
	Attributes []*Attr
	OpenClose  Span
	SelfClose  bool
	Children   []Node
	CloseOpen  Span
	CloseName  []byte
	CloseFinal Span
}

// Span returns the source extent of the element.
func (e *Element) Span() Span {
	end := e.OpenClose.End

	if !e.SelfClose && e.CloseFinal.End.Offset > 0 {
		end = e.CloseFinal.End
	}

	return Span{Start: e.Open.Start, End: end}
}

// Attr is one `name="value"` attribute; Quote is the quote the source used.
type Attr struct {
	Start Position
	End   Position
	Name  []byte
	Quote byte
	Value []byte
}

// Span returns the source extent of the attribute.
func (a *Attr) Span() Span {
	return Span{Start: a.Start, End: a.End}
}

// Text is a run of character data.
type Text struct {
	Start Position
	End   Position
	Text  []byte
}

// Span returns the source extent of the text run.
func (t *Text) Span() Span {
	return Span{Start: t.Start, End: t.End}
}

// Comment is `<!-- text -->`.
type Comment struct {
	Open  Span
	Text  []byte
	Close Span
}

// Span returns the source extent of the comment.
func (c *Comment) Span() Span {
	return Span{Start: c.Open.Start, End: c.Close.End}
}

// CDATA is a `<![CDATA[ ... ]]>` section.
type CDATA struct {
	Open  Span
	Text  []byte
	Close Span
}

// Span returns the source extent of the CDATA section.
func (c *CDATA) Span() Span {
	return Span{Start: c.Open.Start, End: c.Close.End}
}

// PI is a processing instruction `<?target ... ?>`.
type PI struct {
	Open  Span
	Text  []byte
	Close Span
}

// Span returns the source extent of the processing instruction.
func (p *PI) Span() Span {
	return Span{Start: p.Open.Start, End: p.Close.End}
}

// Doctype is `<!DOCTYPE text>`.
type Doctype struct {
	Open  Span
	Text  []byte
	Close Span
}

// Span returns the source extent of the doctype declaration.
func (d *Doctype) Span() Span {
	return Span{Start: d.Open.Start, End: d.Close.End}
}
