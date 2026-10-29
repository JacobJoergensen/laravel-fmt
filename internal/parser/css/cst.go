package css

import parserpkg "github.com/JacobJoergensen/laravel-fmt/internal/parser"

// Position is parser.Position.
type Position = parserpkg.Position

// Span is a half-open byte range [Start, End) in source.
type Span = parserpkg.Span

// Error is a parse diagnostic.
type Error = parserpkg.Error

// Node is any CSS CST node.
type Node interface {
	Span() Span
}

// File is a parsed CSS source.
type File struct {
	Src   []byte
	Nodes []Node
}

// Rule is a qualified rule, selector { children }. Selector is the raw bytes,
// not sub-parsed; Children may nest (Declaration, Rule, AtRule, Comment).
type Rule struct {
	SelectorStart Position
	SelectorEnd   Position
	Selector      []byte
	LBrace        Span
	Children      []Node
	RBrace        Span
}

func (r *Rule) Span() Span {
	end := r.RBrace.End

	if end.Offset == 0 {
		end = r.SelectorEnd
	}

	return Span{Start: r.SelectorStart, End: end}
}

// AtRule is an at-rule in block form (@name prelude { ... }), with Block,
// LBrace, RBrace and Children set, or statement form (@name prelude;),
// whose Semicolon is a zero span when a closing } ends it instead.
type AtRule struct {
	At           Span
	Name         []byte
	PreludeStart Position
	PreludeEnd   Position
	Prelude      []byte
	Block        bool
	LBrace       Span
	Children     []Node
	RBrace       Span
	Semicolon    Span
}

func (a *AtRule) Span() Span {
	end := a.Semicolon.End

	if a.Block {
		end = a.RBrace.End
	}

	if end.Offset == 0 {
		end = a.PreludeEnd
	}

	if end.Offset == 0 {
		end = a.At.End
	}

	return Span{Start: a.At.Start, End: end}
}

// Declaration is property: value [!important][;]. Semicolon is a zero span when
// the terminator was omitted (the last declaration in a block).
type Declaration struct {
	PropertyStart Position
	PropertyEnd   Position
	Property      []byte
	Colon         Span
	ValueStart    Position
	ValueEnd      Position
	Value         []byte
	Important     bool
	ImportantSpan Span
	Semicolon     Span
}

func (d *Declaration) Span() Span {
	end := d.ValueEnd

	if d.Important {
		end = d.ImportantSpan.End
	}

	if d.Semicolon.End.Offset > d.Semicolon.Start.Offset {
		end = d.Semicolon.End
	}

	return Span{Start: d.PropertyStart, End: end}
}

// Comment is /* text */.
type Comment struct {
	Open  Span
	Text  []byte
	Close Span
}

func (c *Comment) Span() Span {
	return Span{Start: c.Open.Start, End: c.Close.End}
}
