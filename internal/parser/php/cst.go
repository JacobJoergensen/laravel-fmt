package php

// Node is any node in the lossless CST.
type Node interface {
	Span() Span
}

// Stmt is any PHP statement.
type Stmt interface {
	Node
	stmtNode()
}

// Expr is any PHP expression.
type Expr interface {
	Node
	exprNode()
}

// File is a parsed PHP source file. Tokens holds the complete lexed stream so
// formatters can consult trivia and tag positions that are not represented in
// the statement tree.
type File struct {
	Src    []byte
	Tokens []Token
	Stmts  []Stmt
}

// InlineHTMLStmt holds text outside of PHP tags.
type InlineHTMLStmt struct {
	Token Token
}

// TagStmt wraps a PHP open or close tag (`<?php`, `<?=`, `?>`).
type TagStmt struct {
	Token Token
}

// HaltCompilerStmt is `__halt_compiler();` and, in Payload, every byte after
// it. Those bytes are a PHAR archive, not source: the engine stops compiling at
// the call and exposes the rest only through __COMPILER_HALT_OFFSET__. They are
// carried whole so the formatter can put them back unchanged.
type HaltCompilerStmt struct {
	Token   Token
	Payload Token
}

// ExpressionStmt is a bare expression used in statement position.
type ExpressionStmt struct {
	Expr      Expr
	Semicolon Token
}

// EchoStmt is `echo expr, ...;`.
type EchoStmt struct {
	Keyword   Token
	Exprs     []Expr
	Semicolon Token
}

// ShortEchoStmt is the head of `<?= expr, ... ?>`. Semicolon is a zero-width
// token when the head ends at `?>` or EOF without one, and further statements
// before the `?>` (`<?= $a; $b ?>`) follow as sibling ExpressionStmts.
type ShortEchoStmt struct {
	OpenTag   Token
	Exprs     []Expr
	Commas    []Token
	Semicolon Token
}

// ReturnStmt is `return [expr];`. Expr is nil for a bare return.
type ReturnStmt struct {
	Keyword   Token
	Expr      Expr
	Semicolon Token
}

// BlockStmt is `{ stmts }`.
type BlockStmt struct {
	LBrace Token
	Stmts  []Stmt
	RBrace Token
}

func (*InlineHTMLStmt) stmtNode()   {}
func (*HaltCompilerStmt) stmtNode() {}
func (*TagStmt) stmtNode()          {}
func (*ExpressionStmt) stmtNode()   {}
func (*EchoStmt) stmtNode()         {}
func (*ShortEchoStmt) stmtNode()    {}
func (*ReturnStmt) stmtNode()       {}
func (*BlockStmt) stmtNode()        {}

// Span returns the source extent of the inline HTML.
func (s *InlineHTMLStmt) Span() Span { return s.Token.Span }

// Span returns the source extent of the tag statement.
func (s *TagStmt) Span() Span { return s.Token.Span }

// Span returns the source extent of the halt compiler statement.
func (s *HaltCompilerStmt) Span() Span {
	if s.Payload.Kind == KindHaltPayload {
		return Span{Start: s.Token.Span.Start, End: s.Payload.Span.End}
	}

	return s.Token.Span
}

// Span returns the source extent of the expression statement.
func (s *ExpressionStmt) Span() Span {
	return Span{Start: s.Expr.Span().Start, End: s.Semicolon.Span.End}
}

// Span returns the source extent of the echo statement.
func (s *EchoStmt) Span() Span {
	return Span{Start: s.Keyword.Span.Start, End: s.Semicolon.Span.End}
}

// Span returns the source extent of the short echo statement.
func (s *ShortEchoStmt) Span() Span {
	end := s.Semicolon.Span.End

	if end.Offset == 0 && len(s.Exprs) > 0 {
		end = s.Exprs[len(s.Exprs)-1].Span().End
	}

	return Span{Start: s.OpenTag.Span.Start, End: end}
}

// Span returns the source extent of the return statement.
func (s *ReturnStmt) Span() Span {
	return Span{Start: s.Keyword.Span.Start, End: s.Semicolon.Span.End}
}

// Span returns the source extent of the block statement.
func (s *BlockStmt) Span() Span {
	return Span{Start: s.LBrace.Span.Start, End: s.RBrace.Span.End}
}

// IntExpr is an integer literal.
type IntExpr struct {
	Token Token
}

// FloatExpr is a floating-point literal.
type FloatExpr struct {
	Token Token
}

// StringExpr is a string literal, heredoc and nowdoc included.
type StringExpr struct {
	Token Token
}

// ShellExecExpr is a backtick shell-exec literal, kept as one Token so its body
// is never inspected.
type ShellExecExpr struct {
	Token Token
}

// BoolExpr is the `true` or `false` literal.
type BoolExpr struct {
	Token Token
	Value bool
}

// NullExpr is the `null` literal.
type NullExpr struct {
	Token Token
}

// VariableExpr is `$name`.
type VariableExpr struct {
	Token Token
}

// VarVarExpr is a variable named by an expression: $$name or ${expr}. LBrace
// is set only for the ${expr} form.
type VarVarExpr struct {
	Dollar Token
	LBrace Token
	Name   Expr
	RBrace Token
}

// IdentExpr is an identifier used as a name (function, constant, class,
// self/static). With LeadingBackslash zero and Parts empty it is a single
// identifier (`Foo`); when either is set it is a qualified name whose Parts
// interleave with Separators to spell `\App\Enums\Permissions`.
type IdentExpr struct {
	Token            Token
	LeadingBackslash Token
	Separators       []Token
	Parts            []Token
}

// BinaryExpr is `lhs op rhs`.
type BinaryExpr struct {
	Left  Expr
	Op    Token
	Right Expr
}

// UnaryExpr is a prefix unary expression (`!expr`, `-expr`, `~expr`, `@expr`).
type UnaryExpr struct {
	Op   Token
	Expr Expr
}

// UpdateExpr is `++` or `--` in prefix or postfix form.
type UpdateExpr struct {
	Op      Token
	Operand Expr
	Prefix  bool
}

// AssignExpr is `lhs = rhs` or any compound-assignment form.
type AssignExpr struct {
	Left  Expr
	Op    Token
	Right Expr
	ByRef bool
}

// ParenExpr is `( expr )`.
type ParenExpr struct {
	LParen Token
	Expr   Expr
	RParen Token
}

// DynamicName is a brace-wrapped expression in a member-name position:
// `$obj->{$propName}` or `Class::{$constName}`. Distinct from ParenExpr
// because the braces denote dynamic-name lookup, not precedence grouping.
type DynamicName struct {
	LBrace Token
	Expr   Expr
	RBrace Token
}

// CallExpr is `callee(args)`. Each arg is an Expr and may be a NamedArgExpr
// or SpreadExpr in addition to any regular expression.
type CallExpr struct {
	Callee Expr
	LParen Token
	Args   []Expr
	RParen Token
}

// FirstClassCallableExpr is the first-class callable syntax `callee(...)`.
type FirstClassCallableExpr struct {
	Callee   Expr
	LParen   Token
	Ellipsis Token
	RParen   Token
}

// TernaryExpr is `cond ? then : else`; Then is nil for the short form
// `cond ?: else`.
type TernaryExpr struct {
	Cond     Expr
	Question Token
	Then     Expr
	Colon    Token
	Else     Expr
}

// MemberExpr is `object->property` or `object?->property`.
// Arrow.Kind distinguishes the two forms.
type MemberExpr struct {
	Object   Expr
	Arrow    Token
	Property Expr
}

// StaticMemberExpr is `class::name`.
type StaticMemberExpr struct {
	Class    Expr
	ScopeRes Token
	Name     Expr
}

// SubscriptExpr is `array[index]`. Index is nil for the append form `arr[]`.
type SubscriptExpr struct {
	Array    Expr
	LBracket Token
	Index    Expr
	RBracket Token
}

// ArrayExpr is a short `[...]` or long `array(...)` literal; KeywordPrefix is
// the zero Token for the short form.
type ArrayExpr struct {
	KeywordPrefix Token
	Open          Token
	Elements      []*ArrayElement
	Close         Token
}

// ArrayElement is one entry of an ArrayExpr.
// When Spread is true, Value carries the spread expression and Key is nil.
// When Key is non-nil, Arrow is the `=>` token.
type ArrayElement struct {
	Spread   bool
	ByRef    bool
	Key      Expr
	Arrow    Token
	Amp      Token // the `&` of a by-reference value
	Ellipsis Token // leading `...` of a spread element
	Value    Expr
}

// Span returns the source extent of the array element.
func (e *ArrayElement) Span() Span {
	if e.Key != nil {
		return Span{Start: e.Key.Span().Start, End: e.Value.Span().End}
	}

	// An empty destructuring slot (`[$a, , $c]`) has no value token, so
	// it has no source span of its own.
	if e.Value == nil {
		return Span{}
	}

	// Start at the leading `&`/`...`, not the value, so a comment between the
	// marker and the value stays interior to this element instead of attaching
	// to the prior.
	if e.ByRef && e.Amp.Kind != KindInvalid {
		return Span{Start: e.Amp.Span.Start, End: e.Value.Span().End}
	}

	if e.Spread && e.Ellipsis.Kind != KindInvalid {
		return Span{Start: e.Ellipsis.Span.Start, End: e.Value.Span().End}
	}

	return e.Value.Span()
}

// SpreadExpr is `...expr` used as a call argument or array element value.
type SpreadExpr struct {
	Ellipsis Token
	Expr     Expr
}

// NamedArgExpr is `name: value` used as a call argument.
type NamedArgExpr struct {
	Name  Token
	Colon Token
	Value Expr
}

// NewExpr is `new Class(args)` or `new Class`. When no parentheses follow,
// LParen and RParen are zero tokens and Args is nil.
type NewExpr struct {
	Keyword Token
	Class   Expr
	LParen  Token
	Args    []Expr
	RParen  Token
}

// AnonClassExpr is `new class (...) { ... }`. Modifiers takes any class
// modifier, since PHP rejects all but readonly only when compiling. LParen,
// Args and RParen are zero without a constructor argument list.
type AnonClassExpr struct {
	New              Token
	Attributes       []*AttributeGroup
	Modifiers        []Modifier
	ClassKeyword     Token
	LParen           Token
	Args             []Expr
	RParen           Token
	Extends          Token
	Parent           *Name
	Implements       Token
	ImplementsNames  []*Name
	ImplementsCommas []Token
	LBrace           Token
	Members          []ClassMember
	RBrace           Token
}

// CloneExpr is `clone expr`, or the PHP 8.5 call form `clone(arg, mods)`, in
// which LParen is set, Args holds the arguments and Expr is nil.
type CloneExpr struct {
	Keyword Token
	Expr    Expr
	LParen  Token
	Args    []Expr
	RParen  Token
}

// InstanceofExpr is `expr instanceof Class`.
type InstanceofExpr struct {
	Left    Expr
	Keyword Token
	Class   Expr
}

// PrintExpr is `print expr`.
type PrintExpr struct {
	Keyword Token
	Expr    Expr
}

// IncludeExpr is `include expr`, `include_once expr`, `require expr`, or
// `require_once expr`. Keyword.Kind distinguishes them.
type IncludeExpr struct {
	Keyword Token
	Expr    Expr
}

// ThrowExpr is `throw expr`.
type ThrowExpr struct {
	Keyword Token
	Expr    Expr
}

// YieldExpr is `yield`, `yield value`, or `yield key => value`.
// Key is nil when no key is given; Value is nil for a bare `yield`.
type YieldExpr struct {
	Keyword Token
	Key     Expr
	Arrow   Token
	Value   Expr
}

// YieldFromExpr is `yield from expr`.
type YieldFromExpr struct {
	Keyword Token
	Expr    Expr
}

// MatchExpr is `match (subject) { arm, ... }`.
type MatchExpr struct {
	Keyword Token
	LParen  Token
	Subject Expr
	RParen  Token
	LBrace  Token
	Arms    []*MatchArm
	RBrace  Token
}

// MatchArm is one arm of a MatchExpr: `cond, cond => result` or
// `default => result`. IsDefault marks the default arm; in that case
// DefaultKeyword is the `default` token and Conds is nil.
type MatchArm struct {
	IsDefault      bool
	DefaultKeyword Token
	Conds          []Expr
	Arrow          Token
	Result         Expr
}

// ClosureExpr is `[static] function [&] (params) [use (uses)] [: return]
// { body }`. Static and ByRef are the zero Token when absent; UseKeyword /
// UseLParen / UseRParen are likewise zero when no `use` clause is given.
type ClosureExpr struct {
	Attributes  []*AttributeGroup
	Static      Token
	Keyword     Token
	ByRef       Token
	LParen      Token
	Params      []*Param
	ParamCommas []Token
	RParen      Token
	UseKeyword  Token
	UseLParen   Token
	Uses        []*ClosureUse
	UseCommas   []Token
	UseRParen   Token
	ReturnColon Token
	ReturnType  TypeHint
	Body        *BlockStmt
}

// ClosureUse is one captured variable inside a closure's `use (...)` list.
// ByRef is the zero Token for capture-by-value.
type ClosureUse struct {
	ByRef Token
	Var   Token
}

// ArrowFnExpr is `[static] fn [&] (params) [: return] => body`.
type ArrowFnExpr struct {
	Attributes  []*AttributeGroup
	Static      Token
	Keyword     Token
	ByRef       Token
	LParen      Token
	Params      []*Param
	ParamCommas []Token
	RParen      Token
	ReturnColon Token
	ReturnType  TypeHint
	Arrow       Token
	Body        Expr
}

// CastExpr is `(type)expr`.
type CastExpr struct {
	LParen Token
	Type   Token
	RParen Token
	Expr   Expr
}

func (*IntExpr) exprNode()                {}
func (*FloatExpr) exprNode()              {}
func (*StringExpr) exprNode()             {}
func (*ShellExecExpr) exprNode()          {}
func (*BoolExpr) exprNode()               {}
func (*NullExpr) exprNode()               {}
func (*VariableExpr) exprNode()           {}
func (*VarVarExpr) exprNode()             {}
func (*IdentExpr) exprNode()              {}
func (*BinaryExpr) exprNode()             {}
func (*UnaryExpr) exprNode()              {}
func (*UpdateExpr) exprNode()             {}
func (*AssignExpr) exprNode()             {}
func (*ParenExpr) exprNode()              {}
func (*DynamicName) exprNode()            {}
func (*CallExpr) exprNode()               {}
func (*FirstClassCallableExpr) exprNode() {}
func (*TernaryExpr) exprNode()            {}
func (*MemberExpr) exprNode()             {}
func (*StaticMemberExpr) exprNode()       {}
func (*SubscriptExpr) exprNode()          {}
func (*ArrayExpr) exprNode()              {}
func (*SpreadExpr) exprNode()             {}
func (*NamedArgExpr) exprNode()           {}
func (*NewExpr) exprNode()                {}
func (*AnonClassExpr) exprNode()          {}
func (*CloneExpr) exprNode()              {}
func (*InstanceofExpr) exprNode()         {}
func (*PrintExpr) exprNode()              {}
func (*IncludeExpr) exprNode()            {}
func (*ThrowExpr) exprNode()              {}
func (*YieldExpr) exprNode()              {}
func (*YieldFromExpr) exprNode()          {}
func (*MatchExpr) exprNode()              {}
func (*ClosureExpr) exprNode()            {}
func (*ArrowFnExpr) exprNode()            {}
func (*CastExpr) exprNode()               {}

// Span returns the source extent of the int expression.
func (e *IntExpr) Span() Span { return e.Token.Span }

// Span returns the source extent of the float expression.
func (e *FloatExpr) Span() Span { return e.Token.Span }

// Span returns the source extent of the string expression.
func (e *StringExpr) Span() Span { return e.Token.Span }

// Span returns the source extent of the shell-exec literal.
func (e *ShellExecExpr) Span() Span { return e.Token.Span }

// Span returns the source extent of the bool expression.
func (e *BoolExpr) Span() Span { return e.Token.Span }

// Span returns the source extent of the null expression.
func (e *NullExpr) Span() Span { return e.Token.Span }

// Span returns the source extent of the variable expression.
func (e *VariableExpr) Span() Span { return e.Token.Span }

// Span returns the source extent of the variable variable.
func (e *VarVarExpr) Span() Span {
	end := e.Name.Span().End
	if e.RBrace.Kind != KindInvalid {
		end = e.RBrace.Span.End
	}

	return Span{Start: e.Dollar.Span.Start, End: end}
}

// Span returns the source extent of the identifier.
func (e *IdentExpr) Span() Span {
	start := e.Token.Span.Start
	end := e.Token.Span.End

	if e.LeadingBackslash.Kind != KindInvalid {
		start = e.LeadingBackslash.Span.Start
	}

	if n := len(e.Parts); n > 0 {
		end = e.Parts[n-1].Span.End
	}

	return Span{Start: start, End: end}
}

// Span returns the source extent of the binary expression.
func (e *BinaryExpr) Span() Span {
	return Span{Start: e.Left.Span().Start, End: e.Right.Span().End}
}

// Span returns the source extent of the unary expression.
func (e *UnaryExpr) Span() Span {
	return Span{Start: e.Op.Span.Start, End: e.Expr.Span().End}
}

// Span returns the source extent of the update expression.
func (e *UpdateExpr) Span() Span {
	if e.Prefix {
		return Span{Start: e.Op.Span.Start, End: e.Operand.Span().End}
	}

	return Span{Start: e.Operand.Span().Start, End: e.Op.Span.End}
}

// Span returns the source extent of the assign expression.
func (e *AssignExpr) Span() Span {
	return Span{Start: e.Left.Span().Start, End: e.Right.Span().End}
}

// Span returns the source extent of the parenthesized expression.
func (e *ParenExpr) Span() Span {
	return Span{Start: e.LParen.Span.Start, End: e.RParen.Span.End}
}

// Span returns the source extent of the dynamic name.
func (e *DynamicName) Span() Span {
	return Span{Start: e.LBrace.Span.Start, End: e.RBrace.Span.End}
}

// Span returns the source extent of the call expression.
func (e *CallExpr) Span() Span {
	return Span{Start: e.Callee.Span().Start, End: e.RParen.Span.End}
}

// Span returns the source extent of the first-class callable.
func (e *FirstClassCallableExpr) Span() Span {
	return Span{Start: e.Callee.Span().Start, End: e.RParen.Span.End}
}

// Span returns the source extent of the ternary expression.
func (e *TernaryExpr) Span() Span {
	return Span{Start: e.Cond.Span().Start, End: e.Else.Span().End}
}

// Span returns the source extent of the member expression.
func (e *MemberExpr) Span() Span {
	return Span{Start: e.Object.Span().Start, End: e.Property.Span().End}
}

// Span returns the source extent of the static member expression.
func (e *StaticMemberExpr) Span() Span {
	return Span{Start: e.Class.Span().Start, End: e.Name.Span().End}
}

// Span returns the source extent of the subscript expression.
func (e *SubscriptExpr) Span() Span {
	return Span{Start: e.Array.Span().Start, End: e.RBracket.Span.End}
}

// Span returns the source extent of the array expression.
func (e *ArrayExpr) Span() Span {
	start := e.Open.Span.Start

	if e.KeywordPrefix.Kind != KindInvalid {
		start = e.KeywordPrefix.Span.Start
	}

	return Span{Start: start, End: e.Close.Span.End}
}

// Span returns the source extent of the spread expression.
func (e *SpreadExpr) Span() Span {
	return Span{Start: e.Ellipsis.Span.Start, End: e.Expr.Span().End}
}

// Span returns the source extent of the named argument.
func (e *NamedArgExpr) Span() Span {
	return Span{Start: e.Name.Span.Start, End: e.Value.Span().End}
}

// Span returns the source extent of the new expression.
func (e *NewExpr) Span() Span {
	if e.RParen.Kind != KindInvalid {
		return Span{Start: e.Keyword.Span.Start, End: e.RParen.Span.End}
	}

	return Span{Start: e.Keyword.Span.Start, End: e.Class.Span().End}
}

// Span returns the source extent of the anonymous class expression.
func (e *AnonClassExpr) Span() Span {
	return Span{Start: e.New.Span.Start, End: e.RBrace.Span.End}
}

// Span returns the source extent of the clone expression.
func (e *CloneExpr) Span() Span {
	if e.LParen.Kind != KindInvalid {
		return Span{Start: e.Keyword.Span.Start, End: e.RParen.Span.End}
	}

	return Span{Start: e.Keyword.Span.Start, End: e.Expr.Span().End}
}

// Span returns the source extent of the instanceof expression.
func (e *InstanceofExpr) Span() Span {
	return Span{Start: e.Left.Span().Start, End: e.Class.Span().End}
}

// Span returns the source extent of the print expression.
func (e *PrintExpr) Span() Span {
	return Span{Start: e.Keyword.Span.Start, End: e.Expr.Span().End}
}

// Span returns the source extent of the include expression.
func (e *IncludeExpr) Span() Span {
	return Span{Start: e.Keyword.Span.Start, End: e.Expr.Span().End}
}

// Span returns the source extent of the throw expression.
func (e *ThrowExpr) Span() Span {
	return Span{Start: e.Keyword.Span.Start, End: e.Expr.Span().End}
}

// Span returns the source extent of the yield expression.
func (e *YieldExpr) Span() Span {
	end := e.Keyword.Span.End

	if e.Value != nil {
		end = e.Value.Span().End
	}

	return Span{Start: e.Keyword.Span.Start, End: end}
}

// Span returns the source extent of the yield from expression.
func (e *YieldFromExpr) Span() Span {
	return Span{Start: e.Keyword.Span.Start, End: e.Expr.Span().End}
}

// Span returns the source extent of the match expression.
func (e *MatchExpr) Span() Span {
	return Span{Start: e.Keyword.Span.Start, End: e.RBrace.Span.End}
}

// Span returns the source extent of the match arm.
func (a *MatchArm) Span() Span {
	if a.IsDefault {
		return Span{Start: a.DefaultKeyword.Span.Start, End: a.Result.Span().End}
	}

	return Span{Start: a.Conds[0].Span().Start, End: a.Result.Span().End}
}

// Span returns the source extent of the closure expression.
func (e *ClosureExpr) Span() Span {
	start := e.Keyword.Span.Start

	if e.Static.Kind != KindInvalid {
		start = e.Static.Span.Start
	}

	if len(e.Attributes) > 0 {
		start = e.Attributes[0].Span().Start
	}

	return Span{Start: start, End: e.Body.Span().End}
}

// Span returns the source extent of the closure use.
func (u *ClosureUse) Span() Span {
	start := u.Var.Span.Start

	if u.ByRef.Kind != KindInvalid {
		start = u.ByRef.Span.Start
	}

	return Span{Start: start, End: u.Var.Span.End}
}

// Span returns the source extent of the arrow function.
func (e *ArrowFnExpr) Span() Span {
	start := e.Keyword.Span.Start

	if e.Static.Kind != KindInvalid {
		start = e.Static.Span.Start
	}

	if len(e.Attributes) > 0 {
		start = e.Attributes[0].Span().Start
	}

	return Span{Start: start, End: e.Body.Span().End}
}

// Span returns the source extent of the cast expression.
func (e *CastExpr) Span() Span {
	return Span{Start: e.LParen.Span.Start, End: e.Expr.Span().End}
}

// EmptyStmt is a bare `;` in statement position.
type EmptyStmt struct {
	Semicolon Token
}

// IfStmt is `if (cond) then [elseif ...] [else ...]` (curly form) or
// `if (cond): body [elseif (cond): body ...] [else: body] endif;`
// (alternative form). When Colon.Kind != KindInvalid the alt form is
// in use: Then is nil, AltBody holds the body statement list, and the
// trailing `endif;` lives in EndKeyword and EndSemi. Each ElseIfClause
// and ElseClause records its own alt-form body inline so the chain is
// one continuous structure with a single terminator.
//
// `else if` (two tokens) ends up as an ElseClause whose Body is an
// inner IfStmt, only meaningful in curly form, since alt form
// requires the one-token `elseif`.
type IfStmt struct {
	Keyword    Token
	LParen     Token
	Cond       Expr
	RParen     Token
	Colon      Token
	Then       Stmt
	AltBody    []Stmt
	ElseIfs    []*ElseIfClause
	Else       *ElseClause
	EndKeyword Token
	EndSemi    Token
}

// ElseIfClause is one `elseif (cond) body` segment of an IfStmt. In
// curly form, Body is the single statement that follows; in alt form
// (parent IfStmt has Colon set), Colon is the `:` after `)` and
// AltBody holds the statement list up to the next `elseif`/`else`/
// `endif` boundary.
type ElseIfClause struct {
	Keyword Token
	LParen  Token
	Cond    Expr
	RParen  Token
	Colon   Token
	Body    Stmt
	AltBody []Stmt
}

// ElseClause is the trailing `else body` of an IfStmt. In curly form,
// Body is the single statement that follows. In alt form (parent
// IfStmt has Colon set), Colon is the `:` after `else` and AltBody
// holds the statement list up to the matching `endif`.
type ElseClause struct {
	Keyword Token
	Colon   Token
	Body    Stmt
	AltBody []Stmt
}

// WhileStmt is `while (cond) body` (curly form) or
// `while (cond): body endwhile;` (alt form). When Colon.Kind !=
// KindInvalid the alt form is in use: Body is nil, AltBody holds the
// body statement list, and EndKeyword / EndSemi carry the closing
// `endwhile;`.
type WhileStmt struct {
	Keyword    Token
	LParen     Token
	Cond       Expr
	RParen     Token
	Colon      Token
	Body       Stmt
	AltBody    []Stmt
	EndKeyword Token
	EndSemi    Token
}

// DoWhileStmt is `do body while (cond);`.
type DoWhileStmt struct {
	DoKeyword    Token
	Body         Stmt
	WhileKeyword Token
	LParen       Token
	Cond         Expr
	RParen       Token
	Semicolon    Token
}

// ForStmt is `for (init; cond; post) body` (curly form) or
// `for (init; cond; post): body endfor;` (alt form). When Colon.Kind
// != KindInvalid the alt form is in use: Body is nil, AltBody holds
// the body statement list, and EndKeyword / EndSemi carry the closing
// `endfor;`. Each header clause may be empty or contain one or more
// comma-separated expressions.
type ForStmt struct {
	Keyword    Token
	LParen     Token
	Init       []Expr
	Semi1      Token
	Cond       []Expr
	Semi2      Token
	Post       []Expr
	RParen     Token
	Colon      Token
	Body       Stmt
	AltBody    []Stmt
	EndKeyword Token
	EndSemi    Token
}

// ForEachStmt is `foreach (expr as [key =>] [&]value) body` (curly
// form) or `foreach (expr as [key =>] [&]value): body endforeach;`
// (alt form). When Colon.Kind != KindInvalid the alt form is in use:
// Body is nil, AltBody holds the body statement list, and EndKeyword
// / EndSemi carry the closing `endforeach;`. ByRef applies to the
// value position; Key is nil when no `key =>` is present.
type ForEachStmt struct {
	Keyword    Token
	LParen     Token
	Expr       Expr
	As         Token
	Key        Expr
	Arrow      Token
	ByRef      bool
	Value      Expr
	RParen     Token
	Colon      Token
	Body       Stmt
	AltBody    []Stmt
	EndKeyword Token
	EndSemi    Token
}

// SwitchStmt is `switch (expr) { cases }` (curly form) or
// `switch (expr): cases endswitch;` (alt form). When Colon.Kind !=
// KindInvalid the alt form is in use: LBrace and RBrace are zero,
// EndKeyword and EndSemi carry the closing `endswitch;`. Cases are
// shared across both forms.
type SwitchStmt struct {
	Keyword    Token
	LParen     Token
	Expr       Expr
	RParen     Token
	Colon      Token
	LBrace     Token
	Cases      []*SwitchCase
	RBrace     Token
	EndKeyword Token
	EndSemi    Token
}

// SwitchCase is one `case expr:` or `default:` arm of a switch.
// Colon records the terminator, which PHP permits as either `:` or `;`.
type SwitchCase struct {
	Keyword   Token
	IsDefault bool
	Expr      Expr
	Colon     Token
	Stmts     []Stmt
}

// TryStmt is `try body [catches...] [finally]`.
type TryStmt struct {
	Keyword Token
	Body    *BlockStmt
	Catches []*CatchClause
	Finally *FinallyClause
}

// CatchClause is one `catch (TypeA|TypeB [$var]) body` of a try.
// Var is nil for PHP 8+ non-capturing catch.
type CatchClause struct {
	Keyword Token
	LParen  Token
	Types   []Expr
	Pipes   []Token
	Var     *VariableExpr
	RParen  Token
	Body    *BlockStmt
}

// FinallyClause is the trailing `finally body` of a try.
type FinallyClause struct {
	Keyword Token
	Body    *BlockStmt
}

// BreakStmt is `break;` or `break expr;`. Level is nil when no level is given.
type BreakStmt struct {
	Keyword   Token
	Level     Expr
	Semicolon Token
}

// ContinueStmt mirrors BreakStmt for the continue keyword.
type ContinueStmt struct {
	Keyword   Token
	Level     Expr
	Semicolon Token
}

// GotoStmt is `goto label;`.
type GotoStmt struct {
	Keyword   Token
	Label     Token
	Semicolon Token
}

// LabelStmt is `name:` used as a goto target. It carries no body, the
// statements that follow it are simply the next entries in the enclosing
// statement list.
type LabelStmt struct {
	Name  Token
	Colon Token
}

// ConstStmt is a file- or namespace-level `const` declaration; class constants
// are ClassConstDecl.
type ConstStmt struct {
	Keyword   Token
	Items     []*ConstItem
	Commas    []Token
	Semicolon Token
}

// GlobalStmt is `global $a, $b, ...;`.
type GlobalStmt struct {
	Keyword   Token
	Vars      []Token
	Commas    []Token
	Semicolon Token
}

// StaticVarStmt is `static $a [= default], ...;`, not the `static` modifier.
type StaticVarStmt struct {
	Keyword   Token
	Vars      []*StaticVarItem
	Commas    []Token
	Semicolon Token
}

// StaticVarItem is one `$name [= default]` entry in a StaticVarStmt.
type StaticVarItem struct {
	Var     Token
	Eq      Token
	Default Expr
}

// UnsetStmt is `unset(expr, ...);`.
type UnsetStmt struct {
	Keyword   Token
	LParen    Token
	Exprs     []Expr
	Commas    []Token
	RParen    Token
	Semicolon Token
}

func (*EmptyStmt) stmtNode()     {}
func (*IfStmt) stmtNode()        {}
func (*WhileStmt) stmtNode()     {}
func (*DoWhileStmt) stmtNode()   {}
func (*ForStmt) stmtNode()       {}
func (*ForEachStmt) stmtNode()   {}
func (*SwitchStmt) stmtNode()    {}
func (*TryStmt) stmtNode()       {}
func (*BreakStmt) stmtNode()     {}
func (*ContinueStmt) stmtNode()  {}
func (*GotoStmt) stmtNode()      {}
func (*LabelStmt) stmtNode()     {}
func (*ConstStmt) stmtNode()     {}
func (*GlobalStmt) stmtNode()    {}
func (*StaticVarStmt) stmtNode() {}
func (*UnsetStmt) stmtNode()     {}

// Span returns the source extent of the empty statement.
func (s *EmptyStmt) Span() Span { return s.Semicolon.Span }

// Span returns the source extent of the if statement.
func (s *IfStmt) Span() Span {
	if s.Colon.Kind != KindInvalid {
		return Span{Start: s.Keyword.Span.Start, End: s.EndSemi.Span.End}
	}

	end := s.Then.Span().End

	if n := len(s.ElseIfs); n > 0 {
		end = s.ElseIfs[n-1].Body.Span().End
	}

	if s.Else != nil {
		end = s.Else.Body.Span().End
	}

	return Span{Start: s.Keyword.Span.Start, End: end}
}

// Span returns the source extent of the elseif clause.
func (c *ElseIfClause) Span() Span {
	if c.Colon.Kind != KindInvalid {
		end := c.Colon.Span.End

		if n := len(c.AltBody); n > 0 {
			end = c.AltBody[n-1].Span().End
		}

		return Span{Start: c.Keyword.Span.Start, End: end}
	}

	return Span{Start: c.Keyword.Span.Start, End: c.Body.Span().End}
}

// Span returns the source extent of the else clause.
func (c *ElseClause) Span() Span {
	if c.Colon.Kind != KindInvalid {
		end := c.Colon.Span.End

		if n := len(c.AltBody); n > 0 {
			end = c.AltBody[n-1].Span().End
		}

		return Span{Start: c.Keyword.Span.Start, End: end}
	}

	return Span{Start: c.Keyword.Span.Start, End: c.Body.Span().End}
}

// Span returns the source extent of the while statement.
func (s *WhileStmt) Span() Span {
	if s.Colon.Kind != KindInvalid {
		return Span{Start: s.Keyword.Span.Start, End: s.EndSemi.Span.End}
	}

	return Span{Start: s.Keyword.Span.Start, End: s.Body.Span().End}
}

// Span returns the source extent of the do-while statement.
func (s *DoWhileStmt) Span() Span {
	return Span{Start: s.DoKeyword.Span.Start, End: s.Semicolon.Span.End}
}

// Span returns the source extent of the for statement.
func (s *ForStmt) Span() Span {
	if s.Colon.Kind != KindInvalid {
		return Span{Start: s.Keyword.Span.Start, End: s.EndSemi.Span.End}
	}

	return Span{Start: s.Keyword.Span.Start, End: s.Body.Span().End}
}

// Span returns the source extent of the foreach statement.
func (s *ForEachStmt) Span() Span {
	if s.Colon.Kind != KindInvalid {
		return Span{Start: s.Keyword.Span.Start, End: s.EndSemi.Span.End}
	}

	return Span{Start: s.Keyword.Span.Start, End: s.Body.Span().End}
}

// Span returns the source extent of the switch statement.
func (s *SwitchStmt) Span() Span {
	if s.Colon.Kind != KindInvalid {
		return Span{Start: s.Keyword.Span.Start, End: s.EndSemi.Span.End}
	}

	return Span{Start: s.Keyword.Span.Start, End: s.RBrace.Span.End}
}

// Span returns the source extent of the switch case.
func (c *SwitchCase) Span() Span {
	end := c.Colon.Span.End

	if n := len(c.Stmts); n > 0 {
		end = c.Stmts[n-1].Span().End
	}

	return Span{Start: c.Keyword.Span.Start, End: end}
}

// Span returns the source extent of the try statement.
func (s *TryStmt) Span() Span {
	end := s.Body.Span().End

	if n := len(s.Catches); n > 0 {
		end = s.Catches[n-1].Body.Span().End
	}

	if s.Finally != nil {
		end = s.Finally.Body.Span().End
	}

	return Span{Start: s.Keyword.Span.Start, End: end}
}

// Span returns the source extent of the catch clause.
func (c *CatchClause) Span() Span {
	return Span{Start: c.Keyword.Span.Start, End: c.Body.Span().End}
}

// Span returns the source extent of the finally clause.
func (c *FinallyClause) Span() Span {
	return Span{Start: c.Keyword.Span.Start, End: c.Body.Span().End}
}

// Span returns the source extent of the break statement.
func (s *BreakStmt) Span() Span {
	return Span{Start: s.Keyword.Span.Start, End: s.Semicolon.Span.End}
}

// Span returns the source extent of the continue statement.
func (s *ContinueStmt) Span() Span {
	return Span{Start: s.Keyword.Span.Start, End: s.Semicolon.Span.End}
}

// Span returns the source extent of the goto statement.
func (s *GotoStmt) Span() Span {
	return Span{Start: s.Keyword.Span.Start, End: s.Semicolon.Span.End}
}

// Span returns the source extent of the label statement.
func (s *LabelStmt) Span() Span {
	return Span{Start: s.Name.Span.Start, End: s.Colon.Span.End}
}

// Span returns the source extent of the const statement.
func (s *ConstStmt) Span() Span {
	return Span{Start: s.Keyword.Span.Start, End: s.Semicolon.Span.End}
}

// Span returns the source extent of the global statement.
func (s *GlobalStmt) Span() Span {
	return Span{Start: s.Keyword.Span.Start, End: s.Semicolon.Span.End}
}

// Span returns the source extent of the static variable statement.
func (s *StaticVarStmt) Span() Span {
	return Span{Start: s.Keyword.Span.Start, End: s.Semicolon.Span.End}
}

// Span returns the source extent of the static variable item.
func (i *StaticVarItem) Span() Span {
	end := i.Var.Span.End
	if i.Default != nil {
		end = i.Default.Span().End
	}

	return Span{Start: i.Var.Span.Start, End: end}
}

// Span returns the source extent of the unset statement.
func (s *UnsetStmt) Span() Span {
	return Span{Start: s.Keyword.Span.Start, End: s.Semicolon.Span.End}
}

// Name is a qualified identifier path:
// `Foo`, `\Foo`, `Foo\Bar`, `\Foo\Bar\Baz`.
// Parts holds the identifier tokens in source order; Separators holds the
// backslash tokens between them. LeadingBackslash is the zero Token when the
// name is not fully qualified.
type Name struct {
	LeadingBackslash Token
	Parts            []Token
	Separators       []Token
}

// Span returns the source extent of the name.
func (n *Name) Span() Span {
	if len(n.Parts) == 0 {
		if n.LeadingBackslash.Kind != KindInvalid {
			return n.LeadingBackslash.Span
		}

		return Span{}
	}

	start := n.Parts[0].Span.Start

	if n.LeadingBackslash.Kind != KindInvalid {
		start = n.LeadingBackslash.Span.Start
	}

	return Span{Start: start, End: n.Parts[len(n.Parts)-1].Span.End}
}

// NamespaceStmt is either `namespace Name;` (unbracketed, governing the rest
// of the file) or `namespace [Name] { body }` (bracketed). Name is nil for
// the global bracketed form `namespace { ... }`.
type NamespaceStmt struct {
	Keyword   Token
	Name      *Name
	Body      *BlockStmt
	Semicolon Token
}

// UseStmt imports names into the current scope. Flat form: `use A, B as C;`.
// Grouped form: `use A\B\{C, D as E};`, Prefix and GroupSeparator are
// non-nil / non-zero, and Items are the entries between LBrace and RBrace.
// UseKind is the `function` or `const` keyword for typed imports, or zero.
type UseStmt struct {
	Keyword        Token
	UseKind        Token
	Prefix         *Name
	GroupSeparator Token
	LBrace         Token
	Items          []*UseItem
	RBrace         Token
	Semicolon      Token
}

// UseItem is one target inside a use statement. Kind is non-zero only when
// the item appears inside a mixed group such as
// `use A\{function foo, const BAR}`.
// AsToken and Alias are zero when no alias is given.
type UseItem struct {
	Kind    Token
	Name    *Name
	AsToken Token
	Alias   Token
}

// Span returns the source extent of the use item.
func (i *UseItem) Span() Span {
	start := i.Name.Span().Start

	if i.Kind.Kind != KindInvalid {
		start = i.Kind.Span.Start
	}

	end := i.Name.Span().End

	if i.Alias.Kind != KindInvalid {
		end = i.Alias.Span.End
	}

	return Span{Start: start, End: end}
}

func (*NamespaceStmt) stmtNode() {}
func (*UseStmt) stmtNode()       {}

// Span returns the source extent of the namespace statement.
func (s *NamespaceStmt) Span() Span {
	end := s.Keyword.Span.End

	switch {
	case s.Body != nil:
		end = s.Body.Span().End
	case s.Semicolon.Kind != KindInvalid:
		end = s.Semicolon.Span.End
	case s.Name != nil:
		end = s.Name.Span().End
	}

	return Span{Start: s.Keyword.Span.Start, End: end}
}

// Span returns the source extent of the use statement.
func (s *UseStmt) Span() Span {
	return Span{Start: s.Keyword.Span.Start, End: s.Semicolon.Span.End}
}

// DeclareStmt is `declare(directives) body`, `declare(directives);`, or
// `declare(directives): body enddeclare;` (alt form). Exactly one of Body,
// Semicolon (with Body nil), or the alt-form trio (Colon/AltBody/EndKeyword +
// EndSemi) is populated. Curly form holds a *BlockStmt in Body.
type DeclareStmt struct {
	Keyword    Token
	LParen     Token
	Directives []*DeclareDirective
	RParen     Token
	Colon      Token
	Body       Stmt
	AltBody    []Stmt
	EndKeyword Token
	EndSemi    Token
	Semicolon  Token
}

// DeclareDirective is one `name=value` pair inside declare().
type DeclareDirective struct {
	Name  Token
	Eq    Token
	Value Expr
}

// Span returns the source extent of the declare directive.
func (d *DeclareDirective) Span() Span {
	return Span{Start: d.Name.Span.Start, End: d.Value.Span().End}
}

func (*DeclareStmt) stmtNode() {}

// Span returns the source extent of the declare statement.
func (s *DeclareStmt) Span() Span {
	end := s.RParen.Span.End

	switch {
	case s.Colon.Kind != KindInvalid:
		end = s.EndSemi.Span.End
	case s.Body != nil:
		end = s.Body.Span().End
	case s.Semicolon.Kind != KindInvalid:
		end = s.Semicolon.Span.End
	}

	return Span{Start: s.Keyword.Span.Start, End: end}
}

// Attribute is one entry inside an attribute group, such as the
// `Route('/users')` in `#[Route('/users'), Middleware('auth')]`. When the
// attribute takes no arguments, LParen and RParen are zero tokens and
// Args is nil.
type Attribute struct {
	Name   *Name
	LParen Token
	Args   []Expr
	RParen Token
}

// Span returns the source extent of the attribute.
func (a *Attribute) Span() Span {
	end := a.Name.Span().End

	if a.RParen.Kind != KindInvalid {
		end = a.RParen.Span.End
	}

	return Span{Start: a.Name.Span().Start, End: end}
}

// AttributeGroup is one `#[ ... ]` cluster preceding a declaration. Multiple
// groups may stack on a single target. Each attribute's arguments share the
// call-argument grammar (positional, named, spread).
type AttributeGroup struct {
	Open       Token
	Attributes []*Attribute
	Commas     []Token
	Close      Token
}

// Span returns the source extent of the attribute group.
func (g *AttributeGroup) Span() Span {
	return Span{Start: g.Open.Span.Start, End: g.Close.Span.End}
}

// FunctionDecl is a named function declaration; unlike MethodDecl it always
// has a body and takes no modifiers.
type FunctionDecl struct {
	Attributes  []*AttributeGroup
	Keyword     Token
	ByRef       Token
	Name        Token
	LParen      Token
	Params      []*Param
	ParamCommas []Token
	RParen      Token
	ReturnColon Token
	ReturnType  TypeHint
	Body        *BlockStmt
}

func (*FunctionDecl) stmtNode() {}

// Span returns the source extent of the function declaration.
func (s *FunctionDecl) Span() Span {
	start := s.Keyword.Span.Start

	if len(s.Attributes) > 0 {
		start = s.Attributes[0].Span().Start
	}

	return Span{Start: start, End: s.Body.Span().End}
}

// Modifier is one entry in a declaration's modifier list. Keyword holds
// the modifier token itself (`public`, `static`, `readonly`, etc.). The
// remaining fields carry the optional PHP 8.4 asymmetric-visibility suffix
// `(set)` that attaches only to visibility keywords; they are the zero
// Token when no suffix is present.
type Modifier struct {
	Keyword Token
	LParen  Token
	Set     Token
	RParen  Token
}

// Span returns the source extent of the modifier.
func (m *Modifier) Span() Span {
	end := m.Keyword.Span.End

	if m.RParen.Kind != KindInvalid {
		end = m.RParen.Span.End
	}

	return Span{Start: m.Keyword.Span.Start, End: end}
}

// ClassStmt is a class declaration.
type ClassStmt struct {
	Attributes       []*AttributeGroup
	Modifiers        []Modifier
	Keyword          Token
	Name             Token
	Extends          Token
	Parent           *Name
	Implements       Token
	ImplementsNames  []*Name
	ImplementsCommas []Token
	LBrace           Token
	Members          []ClassMember
	RBrace           Token
}

// InterfaceStmt is an interface declaration.
type InterfaceStmt struct {
	Attributes    []*AttributeGroup
	Keyword       Token
	Name          Token
	Extends       Token
	ExtendsNames  []*Name
	ExtendsCommas []Token
	LBrace        Token
	Members       []ClassMember
	RBrace        Token
}

// TraitStmt is a trait declaration.
type TraitStmt struct {
	Attributes []*AttributeGroup
	Keyword    Token
	Name       Token
	LBrace     Token
	Members    []ClassMember
	RBrace     Token
}

// EnumStmt is an enum declaration; BackingColon and BackingType are zero for a
// pure enum.
type EnumStmt struct {
	Attributes       []*AttributeGroup
	Keyword          Token
	Name             Token
	BackingColon     Token
	BackingType      TypeHint
	Implements       Token
	ImplementsNames  []*Name
	ImplementsCommas []Token
	LBrace           Token
	Members          []ClassMember
	RBrace           Token
}

// ClassMember is any entry in a class, interface, trait or enum body.
type ClassMember interface {
	Node
	classMemberNode()
}

// PropertyDecl is a property declaration that may define multiple items
// sharing modifiers and a type, e.g. `private int $a = 0, $b = 1;`. When
// PHP 8.4 property hooks are present, LBrace / Hooks / RBrace carry the
// hook block and Semicolon is the zero Token (the `}` terminates the
// declaration in that case).
type PropertyDecl struct {
	Attributes []*AttributeGroup
	Modifiers  []Modifier
	Type       TypeHint
	Items      []*PropertyItem
	Commas     []Token
	LBrace     Token
	Hooks      []*PropertyHook
	RBrace     Token
	Semicolon  Token
}

// PropertyItem is a single `$name` or `$name = default` inside PropertyDecl.
type PropertyItem struct {
	Var     Token
	Eq      Token
	Default Expr
}

// PropertyHook is one `get` or `set` hook inside a property declaration's
// `{ ... }` block. Three forms are represented:
//   - short:    `get => expr;` (Arrow, Expr, and Semicolon are set).
//   - block:    `set(Type $v) { body }` (Body is set).
//   - abstract: `get;` / `set;` (only Semicolon is set).
//
// The optional `final`, the only modifier PHP compiles on a hook, is stored
// in Final and the optional `&` before `get` in Ref; LParen/Params/RParen
// carry the parameter list permitted on setters.
type PropertyHook struct {
	Attributes  []*AttributeGroup
	Final       Token
	Ref         Token
	Name        Token
	LParen      Token
	Params      []*Param
	ParamCommas []Token
	RParen      Token
	Arrow       Token
	Expr        Expr
	Body        *BlockStmt
	Semicolon   Token
}

// MethodDecl is a method inside a class, interface, or trait. Body is nil
// for abstract and interface methods, in which case Semicolon is non-zero.
type MethodDecl struct {
	Attributes  []*AttributeGroup
	Modifiers   []Modifier
	Keyword     Token
	ByRef       Token
	Name        Token
	LParen      Token
	Params      []*Param
	ParamCommas []Token
	RParen      Token
	ReturnColon Token
	ReturnType  TypeHint
	Body        *BlockStmt
	Semicolon   Token
}

// Param is one parameter of a function or method signature; Modifiers is
// non-empty only for a promoted constructor parameter.
type Param struct {
	Attributes []*AttributeGroup
	Modifiers  []Modifier
	Type       TypeHint
	ByRef      Token
	Variadic   Token
	Var        Token
	Eq         Token
	Default    Expr
	// A promoted constructor property may carry PHP 8.4 hooks; LBrace / Hooks /
	// RBrace hold that block when present.
	LBrace Token
	Hooks  []*PropertyHook
	RBrace Token
}

// ClassConstDecl is a class constant declaration; Type is nil unless the
// constant is typed.
type ClassConstDecl struct {
	Attributes []*AttributeGroup
	Modifiers  []Modifier
	Keyword    Token
	Type       TypeHint
	Items      []*ConstItem
	Commas     []Token
	Semicolon  Token
}

// ConstItem is one `NAME = value` of a ConstStmt or ClassConstDecl.
type ConstItem struct {
	Name  Token
	Eq    Token
	Value Expr
}

// TraitUseStmt is `use TraitA, TraitB;` inside a class body. When a
// `{ ... }` adaptation body is present, LBrace / Adaptations / RBrace
// carry the block and Semicolon is the zero Token (the `}` terminates
// the declaration in that case).
type TraitUseStmt struct {
	Keyword     Token
	Traits      []*Name
	Commas      []Token
	LBrace      Token
	Adaptations []TraitAdaptation
	RBrace      Token
	Semicolon   Token
}

// TraitAdaptation is one entry inside a `use TraitA { ... }` body.
// Two concrete shapes exist: TraitInsteadofAdaptation for conflict
// resolution and TraitAliasAdaptation for renames and visibility
// changes.
type TraitAdaptation interface {
	Node
	traitAdaptationNode()
}

// TraitMethodRef is the method reference on the left side of an
// adaptation: `TraitA::method` (qualified) or just `method`
// (unqualified, only legal in `as` adaptations; `insteadof` requires
// qualification). When unqualified, Trait is nil and Separator is the zero
// Token.
type TraitMethodRef struct {
	Trait     *Name
	Separator Token
	Method    Token
}

// TraitInsteadofAdaptation is `Trait::method insteadof Other, ...;`.
type TraitInsteadofAdaptation struct {
	Method    TraitMethodRef
	Keyword   Token
	Traits    []*Name
	Commas    []Token
	Semicolon Token
}

func (*TraitInsteadofAdaptation) traitAdaptationNode() {}

// TraitAliasAdaptation is `method as [visibility] [Alias];`. Visibility and
// Alias are zero when absent, and PHP requires at least one.
type TraitAliasAdaptation struct {
	Method     TraitMethodRef
	Keyword    Token
	Visibility Token
	Alias      Token
	Semicolon  Token
}

func (*TraitAliasAdaptation) traitAdaptationNode() {}

// EnumCase is one `case Name [= value];` of an enum; Value is nil and Eq zero
// in a pure enum.
type EnumCase struct {
	Attributes []*AttributeGroup
	Keyword    Token
	Name       Token
	Eq         Token
	Value      Expr
	Semicolon  Token
}

// TypeHint is any PHP type expression.
type TypeHint interface {
	Node
	typeNode()
}

// NamedType is an identifier-based type like `int`, `Foo`, `\Foo\Bar`.
// Built-in single-word types such as `array` or `callable` are expressed as
// KeywordType instead.
type NamedType struct {
	Name *Name
}

// KeywordType is a type expressed by a single reserved keyword that cannot
// appear as a Name part: `array`, `callable`, `self`, `static`, `null`,
// `true`, `false`.
type KeywordType struct {
	Token Token
}

// NullableType is `?Type`.
type NullableType struct {
	Question Token
	Type     TypeHint
}

// UnionType is `A|B|C`.
type UnionType struct {
	Types []TypeHint
	Pipes []Token
}

// IntersectionType is `A&B`.
type IntersectionType struct {
	Types []TypeHint
	Amps  []Token
}

// ParenType is a parenthesized intersection inside a DNF union, `(A&B)|C`.
type ParenType struct {
	LParen Token
	Type   TypeHint
	RParen Token
}

func (*ClassStmt) stmtNode()     {}
func (*InterfaceStmt) stmtNode() {}
func (*TraitStmt) stmtNode()     {}
func (*EnumStmt) stmtNode()      {}

func (*PropertyDecl) classMemberNode()   {}
func (*MethodDecl) classMemberNode()     {}
func (*ClassConstDecl) classMemberNode() {}
func (*TraitUseStmt) classMemberNode()   {}
func (*EnumCase) classMemberNode()       {}

func (*NamedType) typeNode()        {}
func (*KeywordType) typeNode()      {}
func (*NullableType) typeNode()     {}
func (*UnionType) typeNode()        {}
func (*IntersectionType) typeNode() {}
func (*ParenType) typeNode()        {}

// Span returns the source extent of the class statement.
func (s *ClassStmt) Span() Span {
	start := s.Keyword.Span.Start

	switch {
	case len(s.Attributes) > 0:
		start = s.Attributes[0].Span().Start
	case len(s.Modifiers) > 0:
		start = s.Modifiers[0].Keyword.Span.Start
	}

	return Span{Start: start, End: s.RBrace.Span.End}
}

// Span returns the source extent of the interface statement.
func (s *InterfaceStmt) Span() Span {
	start := s.Keyword.Span.Start
	if len(s.Attributes) > 0 {
		start = s.Attributes[0].Span().Start
	}

	return Span{Start: start, End: s.RBrace.Span.End}
}

// Span returns the source extent of the trait statement.
func (s *TraitStmt) Span() Span {
	start := s.Keyword.Span.Start
	if len(s.Attributes) > 0 {
		start = s.Attributes[0].Span().Start
	}

	return Span{Start: start, End: s.RBrace.Span.End}
}

// Span returns the source extent of the enum statement.
func (s *EnumStmt) Span() Span {
	start := s.Keyword.Span.Start
	if len(s.Attributes) > 0 {
		start = s.Attributes[0].Span().Start
	}

	return Span{Start: start, End: s.RBrace.Span.End}
}

// Span returns the source extent of the property declaration.
func (m *PropertyDecl) Span() Span {
	start := m.Semicolon.Span.Start

	switch {
	case len(m.Attributes) > 0:
		start = m.Attributes[0].Span().Start
	case len(m.Modifiers) > 0:
		start = m.Modifiers[0].Keyword.Span.Start
	case m.Type != nil:
		start = m.Type.Span().Start
	case len(m.Items) > 0:
		start = m.Items[0].Var.Span.Start
	}

	end := m.Semicolon.Span.End

	if m.RBrace.Kind != KindInvalid {
		end = m.RBrace.Span.End
	}

	return Span{Start: start, End: end}
}

// Span returns the source extent of the property item.
func (i *PropertyItem) Span() Span {
	end := i.Var.Span.End

	if i.Default != nil {
		end = i.Default.Span().End
	}

	return Span{Start: i.Var.Span.Start, End: end}
}

// Span returns the source extent of the property hook.
func (h *PropertyHook) Span() Span {
	start := h.Name.Span.Start

	switch {
	case len(h.Attributes) > 0:
		start = h.Attributes[0].Span().Start
	case h.Final.Kind != KindInvalid:
		start = h.Final.Span.Start
	case h.Ref.Kind != KindInvalid:
		start = h.Ref.Span.Start
	}

	end := h.Name.Span.End

	switch {
	case h.Body != nil:
		end = h.Body.Span().End
	case h.Semicolon.Kind != KindInvalid:
		end = h.Semicolon.Span.End
	}

	return Span{Start: start, End: end}
}

// Span returns the source extent of the method declaration.
func (m *MethodDecl) Span() Span {
	start := m.Keyword.Span.Start

	switch {
	case len(m.Attributes) > 0:
		start = m.Attributes[0].Span().Start
	case len(m.Modifiers) > 0:
		start = m.Modifiers[0].Keyword.Span.Start
	}

	end := m.Semicolon.Span.End

	if m.Body != nil {
		end = m.Body.Span().End
	}

	return Span{Start: start, End: end}
}

// Span returns the source extent of the parameter.
func (p *Param) Span() Span {
	start := p.Var.Span.Start

	switch {
	case len(p.Attributes) > 0:
		start = p.Attributes[0].Span().Start
	case len(p.Modifiers) > 0:
		start = p.Modifiers[0].Keyword.Span.Start
	case p.Type != nil:
		start = p.Type.Span().Start
	case p.ByRef.Kind != KindInvalid:
		start = p.ByRef.Span.Start
	case p.Variadic.Kind != KindInvalid:
		start = p.Variadic.Span.Start
	}

	end := p.Var.Span.End

	switch {
	case p.RBrace.Kind != KindInvalid:
		end = p.RBrace.Span.End
	case p.Default != nil:
		end = p.Default.Span().End
	}

	return Span{Start: start, End: end}
}

// Span returns the source extent of the class constant declaration.
func (m *ClassConstDecl) Span() Span {
	start := m.Keyword.Span.Start

	switch {
	case len(m.Attributes) > 0:
		start = m.Attributes[0].Span().Start
	case len(m.Modifiers) > 0:
		start = m.Modifiers[0].Keyword.Span.Start
	}

	return Span{Start: start, End: m.Semicolon.Span.End}
}

// Span returns the source extent of the constant item.
func (i *ConstItem) Span() Span {
	return Span{Start: i.Name.Span.Start, End: i.Value.Span().End}
}

// Span returns the source extent of the trait use statement.
func (m *TraitUseStmt) Span() Span {
	end := m.Keyword.Span.End

	switch {
	case m.Semicolon.Kind != KindInvalid:
		end = m.Semicolon.Span.End
	case m.RBrace.Kind != KindInvalid:
		end = m.RBrace.Span.End
	case len(m.Traits) > 0:
		end = m.Traits[len(m.Traits)-1].Span().End
	}

	return Span{Start: m.Keyword.Span.Start, End: end}
}

// Span returns the source extent of the trait method reference.
func (r *TraitMethodRef) Span() Span {
	if r.Trait != nil {
		return Span{Start: r.Trait.Span().Start, End: r.Method.Span.End}
	}

	return r.Method.Span
}

// Span returns the source extent of the insteadof adaptation.
func (a *TraitInsteadofAdaptation) Span() Span {
	return Span{Start: a.Method.Span().Start, End: a.Semicolon.Span.End}
}

// Span returns the source extent of the alias adaptation.
func (a *TraitAliasAdaptation) Span() Span {
	return Span{Start: a.Method.Span().Start, End: a.Semicolon.Span.End}
}

// Span returns the source extent of the enum case.
func (c *EnumCase) Span() Span {
	start := c.Keyword.Span.Start

	if len(c.Attributes) > 0 {
		start = c.Attributes[0].Span().Start
	}

	return Span{Start: start, End: c.Semicolon.Span.End}
}

// Span returns the source extent of the named type.
func (t *NamedType) Span() Span { return t.Name.Span() }

// Span returns the source extent of the keyword type.
func (t *KeywordType) Span() Span { return t.Token.Span }

// Span returns the source extent of the nullable type.
func (t *NullableType) Span() Span {
	return Span{Start: t.Question.Span.Start, End: t.Type.Span().End}
}

// Span returns the source extent of the union type.
func (t *UnionType) Span() Span {
	return Span{Start: t.Types[0].Span().Start, End: t.Types[len(t.Types)-1].Span().End}
}

// Span returns the source extent of the intersection type.
func (t *IntersectionType) Span() Span {
	return Span{Start: t.Types[0].Span().Start, End: t.Types[len(t.Types)-1].Span().End}
}

// Span returns the source extent of the parenthesized type.
func (t *ParenType) Span() Span {
	return Span{Start: t.LParen.Span.Start, End: t.RParen.Span.End}
}
