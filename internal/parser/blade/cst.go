package blade

import (
	"github.com/JacobJoergensen/laravel-fmt/internal/parser/html"
	"github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

// Position is html.Position, shared so PHP, HTML, and Blade nodes use one
// coordinate type.
type Position = html.Position

// Span is a half-open byte range [Start, End) in source.
type Span = html.Span

// Error is a parse diagnostic.
type Error = html.Error

// Node is any Blade template node; the interface is only Span() so html.* nodes
// can sit in the tree alongside Blade ones.
type Node interface {
	Span() Span
}

// File is a parsed Blade template.
type File struct {
	Src   []byte
	Nodes []Node
	// Directives is the set of block directives the parse recognized, which
	// the formatter reads to classify a directive in attribute position.
	Directives *Directives
}

// EchoNode is `{{ args }}`, an escaped echo. Blade compiles it to `echo e(args)`,
// so Args is a call argument list: `{{ $v, false }}` passes e()'s doubleEncode.
type EchoNode struct {
	Open   Span
	Args   []php.Expr
	Tokens []php.Token
	Close  Span
}

// RawEchoNode is `{!! args !!}`, an unescaped echo. Blade compiles it to
// `echo args;`, so Args is echo's expression list.
type RawEchoNode struct {
	Open   Span
	Args   []php.Expr
	Tokens []php.Token
	Close  Span
}

// CommentNode is {{-- text --}}, a Blade comment stripped from output.
type CommentNode struct {
	Open  Span
	Text  []byte
	Close Span
}

// DirectiveNode is @name or @name(args). Raw holds the bytes between the
// parens when Args is not parsed: always for @foreach, @forelse and @for,
// and for a package directive whose argument is not PHP (see Opaque).
type DirectiveNode struct {
	At      Position
	Name    []byte
	NameEnd Position
	LParen  Span
	Args    []php.Expr
	Tokens  []php.Token
	Raw     []byte
	// Opaque marks a Raw that is a package directive's argument and not PHP;
	// its handler reads it as a string, so it goes out byte for byte.
	Opaque bool
	// Loop is a @foreach, @forelse or @for argument parsed inside the statement
	// Blade compiles it to (`foreach (<Raw>) {}` or `for (<Raw>) {}`), with the
	// tokens it was lexed into and that wrapped source; nil when the argument
	// does not parse so.
	Loop       php.Stmt
	LoopTokens []php.Token
	LoopSrc    []byte
	RParen     Span
}

// PhpBlockNode is `@php ... @endphp`, an embedded sequence of PHP
// statements.
type PhpBlockNode struct {
	Open   Span
	Stmts  []php.Stmt
	Tokens []php.Token
	Close  Span
}

// DirectiveBlockNode is a directive-opened block: Body, then Branches, then
// the End closer, which is never nil because an unmatched opener does not
// produce a block.
type DirectiveBlockNode struct {
	Opener   *DirectiveNode
	Body     []Node
	Branches []*DirectiveBranch
	End      *DirectiveNode
}

// DirectiveBranch is one middle section of a DirectiveBlockNode, such as
// @else, @elseif, @case or @empty.
type DirectiveBranch struct {
	Directive *DirectiveNode
	Body      []Node
}

// VerbatimNode is @verbatim ... @endverbatim, a region where Blade syntax is
// not interpreted.
type VerbatimNode struct {
	Open  Span
	Text  []byte
	Close Span
}

// EscapedAtNode is `@@`, the escape that emits a literal `@` without
// starting a directive.
type EscapedAtNode struct {
	Start Position
	End   Position
}

// EscapedEchoNode is @{{ text }}: Blade strips the @ and emits {{ text }}
// verbatim for a client-side framework (Vue, Alpine), never parsing it as PHP.
type EscapedEchoNode struct {
	At    Position
	Open  Span
	Text  []byte
	Close Span
}

// RawPhpNode is a `<?php … ?>` or `<?= … ?>` island in template content, kept
// as a structural boundary and never reflowed as text.
type RawPhpNode struct {
	Start Position
	End   Position

	// Body spans the PHP between the open tag and `?>`.
	Body Span
	// Stmts is Body parsed, set only for a `<?php` island whose body is a
	// complete statement list; any other island goes out verbatim, so a failed
	// parse here is not an error.
	Stmts []php.Stmt
	// Tokens are the lexed Body, set with Stmts.
	Tokens []php.Token

	// Opens, Closes and Branch place the island in an alt-syntax control
	// structure (`if ($a):`, `endif;`, `else:`) so the emitter can indent a
	// run of islands; all are false for brace syntax and for a parsed Stmts.
	Opens  bool
	Closes bool
	Branch bool
}

func (n *EchoNode) Span() Span        { return Span{Start: n.Open.Start, End: n.Close.End} }
func (n *RawEchoNode) Span() Span     { return Span{Start: n.Open.Start, End: n.Close.End} }
func (n *CommentNode) Span() Span     { return Span{Start: n.Open.Start, End: n.Close.End} }
func (n *PhpBlockNode) Span() Span    { return Span{Start: n.Open.Start, End: n.Close.End} }
func (n *VerbatimNode) Span() Span    { return Span{Start: n.Open.Start, End: n.Close.End} }
func (n *EscapedAtNode) Span() Span   { return Span{Start: n.Start, End: n.End} }
func (n *EscapedEchoNode) Span() Span { return Span{Start: n.At, End: n.Close.End} }
func (n *RawPhpNode) Span() Span      { return Span{Start: n.Start, End: n.End} }

func (n *DirectiveNode) Span() Span {
	end := n.NameEnd

	if n.RParen.End.Offset > 0 {
		end = n.RParen.End
	}

	return Span{Start: n.At, End: end}
}

func (n *DirectiveBlockNode) Span() Span {
	return Span{Start: n.Opener.Span().Start, End: n.End.Span().End}
}
