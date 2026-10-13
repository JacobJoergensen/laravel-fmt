package html

import "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"

// Position is php.Position, shared so the whole parsing stack uses one
// coordinate type.
type Position = php.Position

// Span is a half-open byte range [Start, End) in source.
type Span = php.Span

// Error is a parse diagnostic.
type Error = php.Error

// Node is any HTML CST node; the interface is only Span() so Blade/Livewire
// nodes can mix into Element.Children.
type Node interface {
	Span() Span
}

// File is a parsed HTML document.
type File struct {
	Src   []byte
	Nodes []Node
}

// Element is <name attrs /> or <name attrs>children</name>. A void element
// (<br>, <img>) gets SelfClose without a /, unless a matching close tag in
// scope pairs it.
type Element struct {
	Open       Span
	Name       []byte
	Attributes []*Attr
	OpenClose  Span
	SelfClose  bool
	// SlashClosed records that the author wrote `/>`. On a void element the
	// slash means nothing to HTML5 and SelfClose is set either way, so this
	// flag is what keeps the author's spelling.
	SlashClosed bool
	Children    []Node
	// CloseOpen is the `</` of the close tag, and its Start is where the body
	// ends. For an end tag the author omitted it is zero-width at that point, so
	// the body reads the same with the tag written or not; CloseFinal stays zero.
	CloseOpen  Span
	CloseName  []byte
	CloseFinal Span
	// Loose marks a deferred-open element: opened inside a Blade block whose
	// matching close tag lives in a sibling branch or later iteration. The
	// emitter renders the open tag alone and must not synthesize a close.
	Loose bool
}

func (e *Element) Span() Span {
	end := e.OpenClose.End

	if !e.SelfClose && e.CloseFinal.End.Offset > 0 {
		end = e.CloseFinal.End
	}

	return Span{Start: e.Open.Start, End: end}
}

// Attr is one attribute on an HTML element (Blade reuses it):
//   - flag      `name`         : Eq, Quote, Value and Expr zero
//   - plain     `name="value"` : Value set; Quote is 0 when unquoted
//   - bound     `:name="expr"` : Bound set; Expr only when the value parses
//     as PHP (Alpine/Vue JS values leave Expr nil and keep Value verbatim)
//   - shorthand `:$name`       : Bound and Shorthand set
//
// Parts holds the html.Text and echo nodes when a Blade echo is embedded in
// the value. Directive holds a Blade directive, echo or comment standing in
// attribute position (@class([...]), {{ $attributes }}), with the name and
// value fields zero. Only the Blade parser sets Expr, Parts and Directive.
type Attr struct {
	Start     Position
	End       Position
	Bound     bool
	Shorthand bool
	Name      []byte
	Eq        Span
	Quote     byte
	Value     []byte
	// ValueStart is the source offset of the value's first byte, after any
	// opening quote.
	ValueStart int
	Parts      []Node
	Expr       php.Expr
	ExprTokens []php.Token
	Directive  Node
	// RawPhp is a `<?php … ?>` or `<?= … ?>` tag in attribute position, kept
	// whole so its PHP is not read as attribute names; only Blade sets it.
	RawPhp []byte
	// KeepUnquoted reports that an unquoted value has to be emitted exactly as
	// authored rather than gaining quotes, because its rendered bytes can carry
	// whitespace, a `>` or a quote of their own. That covers literal whitespace or
	// `>` (an HTML tokenizer already reads `x-data={ open: false }` as four
	// attributes, and quoting would merge them), an escaped echo or a Blade comment
	// (rendered literally, not executed), and a raw `{!! !!}` echo or a `@name(…)`
	// argument list, which expand to unescaped markup. A `{{ }}` echo is the one
	// safe case: e() escapes both quote characters, so it cannot break out.
	KeepUnquoted bool
	// KeepQuote reports that a quoted value has to keep the quote it was written
	// with, because it holds output that can print either quote: a `@name(…)`
	// directive (`@json` prints raw JSON), a raw `{!! !!}` echo or a `<?` tag.
	KeepQuote bool
}

func (a *Attr) Span() Span {
	return Span{Start: a.Start, End: a.End}
}

// Text is a literal text run between HTML constructs.
//
// PHP holds the ranges of a raw body (`<script>`, `<textarea>`, a mail
// component) that Blade compiles into PHP: echoes, directive arguments,
// `@php` blocks and `<?php ?>` islands. Blade compiles them in any HTML
// context, so a CR inside one is PHP data rather than a line of the body.
// PHPLiterals holds the parts of those ranges whose line starts are data: the
// multi-line string literals, or a whole range the PHP lexer could not read.
// Only the Blade parser sets them.
type Text struct {
	Start       Position
	End         Position
	Text        []byte
	PHP         []Span
	PHPLiterals []Span
}

func (t *Text) Span() Span {
	return Span{Start: t.Start, End: t.End}
}

// Comment is `<!-- text -->`.
type Comment struct {
	Open  Span
	Text  []byte
	Close Span
}

func (c *Comment) Span() Span {
	return Span{Start: c.Open.Start, End: c.Close.End}
}

// Doctype is `<!DOCTYPE text>`.
type Doctype struct {
	Open  Span
	Text  []byte
	Close Span
}

func (d *Doctype) Span() Span {
	return Span{Start: d.Open.Start, End: d.Close.End}
}
