package yaml

import "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"

// Position is php.Position.
type Position = php.Position

// Span is a half-open byte range [Start, End) in source.
type Span = php.Span

// Error is a parse diagnostic.
type Error = php.Error

// Node is any YAML value node.
type Node interface {
	Span() Span
}

// File is a parsed YAML stream.
type File struct {
	Src  []byte
	Docs []*Document
}

// TriviaKind categorizes a non-structural line.
type TriviaKind uint8

const (
	TriviaBlank TriviaKind = iota
	TriviaComment
)

// Trivia is a blank line or comment.
type Trivia struct {
	Kind  TriviaKind
	Range Span
	Text  []byte
}

// Document is one document in the stream.
type Document struct {
	Range       Span
	Leading     []Trivia
	Directives  []Span
	StartMarker Span
	Body        Node // Mapping, Sequence, Verbatim or nil
	EndMarker   Span
	Trailing    []Trivia
}

// Mapping is a block mapping.
type Mapping struct {
	Range   Span
	Entries []*MappingEntry
}

// Span returns the source extent of the mapping.
func (m *Mapping) Span() Span { return m.Range }

// MappingEntry is one key/value pair; Value is nil for an empty (null) value.
type MappingEntry struct {
	Range   Span
	Leading []Trivia
	Key     *Scalar
	Colon   Span
	Value   Node
	Comment *Trivia
}

// Sequence is a block sequence.
type Sequence struct {
	Range Span
	Items []*SequenceItem
}

// Span returns the source extent of the sequence.
func (s *Sequence) Span() Span { return s.Range }

// SequenceItem is one `- value` entry.
type SequenceItem struct {
	Range   Span
	Leading []Trivia
	Dash    Span
	Value   Node
	Comment *Trivia
}

// Scalar is a plain or quoted scalar; its quoting and escapes are never
// rewritten.
type Scalar struct {
	Range Span
	Raw   []byte
	// Indent is the block indent the scalar was read against. A quoted scalar may
	// span lines, and the emitter re-homes those continuation lines against this
	// distance: the source column alone is not it, since a `- ` marker is structural
	// indent that is not whitespace.
	Indent int
}

// Span returns the source extent of the scalar.
func (s *Scalar) Span() Span { return s.Range }

// BlockScalar is a `|` literal or `>` folded scalar, with any anchor or tag
// that leads its header.
type BlockScalar struct {
	Range Span
	Raw   []byte
	// Indent is the block indent the body was read against: it covers exactly the
	// following lines indented further than this. The emitter re-homes the body
	// against it, since that distance is what keeps the lines part of the scalar.
	Indent int
	// ContentIndent is the column of the first non-empty body line, or 0 when
	// the body must be kept as written: the header has an indent digit, the
	// body is empty, or a tab follows a line's indent.
	ContentIndent int
}

// Span returns the source extent of the block scalar.
func (b *BlockScalar) Span() Span { return b.Range }

// Flow is a {...} or [...] collection. laravel-fmt has no opinion on flow
// style, so it stays verbatim.
type Flow struct {
	Range Span
	Raw   []byte
}

// Span returns the source extent of the flow collection.
func (f *Flow) Span() Span { return f.Range }

// Verbatim is a span the parser does not model: an anchored, aliased or tagged
// value other than a block scalar, or with Bail set, lines it cannot classify,
// which leave the whole file unformatted.
type Verbatim struct {
	Range Span
	Raw   []byte
	Bail  bool
	// Indent is the block indent the span was read against: it absorbed exactly
	// the following lines indented further than this. The emitter needs it to
	// re-home the span, because that distance, not the span's own column, is
	// what keeps those lines part of the node.
	Indent int
}

// Span returns the source extent of the preserved span.
func (v *Verbatim) Span() Span { return v.Range }
