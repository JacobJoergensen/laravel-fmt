package json

import parserpkg "github.com/JacobJoergensen/laravel-fmt/internal/parser"

// Position is parser.Position.
type Position = parserpkg.Position

// Span is a half-open byte range [Start, End) in source.
type Span = parserpkg.Span

// Error is a parse diagnostic.
type Error = parserpkg.Error

// Value is any JSON value node.
type Value interface {
	Span() Span
}

// Comment is a `//` or `/* */` comment captured in JSONC mode. Text is the raw
// bytes including the delimiters, trailing whitespace trimmed, so re-emission is
// verbatim. Strict JSON never produces one.
type Comment struct {
	Range       Span
	Text        []byte
	BlankBefore bool
}

func (c *Comment) Span() Span { return c.Range }

// File is a parsed JSON document; Root is nil when src holds no value.
// Leading and Trailing hold JSONC comments before and after the root
// value (both empty for strict JSON); RootTrailing is a same-line comment
// after the root.
type File struct {
	Src          []byte
	Root         Value
	Leading      []Comment
	RootTrailing *Comment
	Trailing     []Comment
}

// Object is { members } in source order. Duplicate keys aren't enforced; the
// formatter preserves whatever the source carried. Dangling holds JSONC comments
// after the last member (or inside an otherwise-empty object) with no member to
// attach to.
type Object struct {
	Range    Span
	Members  []*Member
	Dangling []Comment
}

func (o *Object) Span() Span { return o.Range }

// Member is one "key": value pair inside an Object. Leading holds JSONC comments
// on their own lines before the key; Trailing is a same-line comment after the
// value. Both are empty/nil for strict JSON.
type Member struct {
	Key     *String
	Value   Value
	Leading []Comment
	// BeforeValue holds JSONC comments between the key and the value, on
	// either side of the colon.
	BeforeValue []Comment
	Trailing    *Comment
	BlankBefore bool
}

// Array is `[ elements ]`. Elements is in source order. Dangling mirrors Object.
type Array struct {
	Range    Span
	Elements []*Element
	Dangling []Comment
}

func (a *Array) Span() Span { return a.Range }

// Element is one array entry. Leading and Trailing mirror Member's comment
// fields; both are empty/nil for strict JSON.
type Element struct {
	Value       Value
	Leading     []Comment
	Trailing    *Comment
	BlankBefore bool
}

// String is a JSON string literal. Raw is the bytes between the quotes
// verbatim; escapes are not decoded, so re-emission is byte-for-byte.
type String struct {
	Range Span
	Raw   []byte
}

func (s *String) Span() Span { return s.Range }

// Number is a JSON number literal. Raw is verbatim, so 1.0, 1e5, and -0
// round-trip unchanged rather than being canonicalized.
type Number struct {
	Range Span
	Raw   []byte
}

func (n *Number) Span() Span { return n.Range }

// Bool is a JSON boolean literal.
type Bool struct {
	Range Span
	Value bool
}

func (b *Bool) Span() Span { return b.Range }

// Null is the JSON `null` literal.
type Null struct {
	Range Span
}

func (n *Null) Span() Span { return n.Range }
