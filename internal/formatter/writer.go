package formatter

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/textwidth"
)

// Writer accumulates formatted bytes while tracking the visual column so
// emitters can make width-aware decisions. The primitives never error and
// leave the Writer in a consistent state, so emitters can chain them without
// checking after each call.
type Writer struct {
	buf           bytes.Buffer
	column        int
	indent        int
	align         int
	pendingIndent bool
	pendingBlank  bool

	tabWidth   int
	useTabs    bool
	eol        string
	printWidth int

	flatDepth    int
	flatOverflow bool
	// lineTrailing reserves columns for content that will share the current line
	// after the construct being measured (a statement `;`, a trailing `,`, a
	// hugged call's `callee(`). It is read only inside a flat probe, where the
	// whole construct is on one line so the trailing token genuinely trails it; a
	// real Newline clears it (a broken construct's trailer moves to a later line).
	lineTrailing int
}

type snapshot struct {
	bufLen        int
	column        int
	indent        int
	align         int
	pendingIndent bool
	pendingBlank  bool
	flatOverflow  bool
	lineTrailing  int
}

// NewWriter returns an empty Writer configured from cfg.
func NewWriter(cfg *config.File) *Writer {
	return &Writer{
		tabWidth:   cfg.TabWidth,
		useTabs:    cfg.UseTabs(),
		eol:        eolBytes(cfg.EndOfLine),
		printWidth: cfg.PrintWidth,
	}
}

// Bytes returns the formatted output accumulated so far. The returned
// slice is owned by the Writer and must not be modified.
func (w *Writer) Bytes() []byte {
	return w.buf.Bytes()
}

// Column returns the current visual column on the line being emitted.
// Tabs count tabWidth columns, a carriage return counts none, and every other
// rune counts the columns textwidth.Rune reports for it.
func (w *Writer) Column() int {
	return w.column
}

// Write appends s to the buffer, flushing any pending indent first. s must not
// contain newlines; emitters use Newline so the Writer can track column and
// pending indent.
func (w *Writer) Write(s string) {
	if w.flatOverflow {
		return
	}

	w.consumeBlank()
	w.flushIndent()
	w.buf.WriteString(s)
	w.column += visualWidth(s, w.tabWidth)

	if w.flatDepth > 0 && w.column+w.lineTrailing > w.printWidth {
		w.flatOverflow = true
	}
}

// WriteBytes appends b like Write but without the copy Write(string(b)) would
// force: the zero-copy path for a token's source bytes. b must not contain
// newlines.
func (w *Writer) WriteBytes(b []byte) {
	if w.flatOverflow {
		return
	}

	w.consumeBlank()
	w.flushIndent()
	w.buf.Write(b)
	w.column += VisualWidthBytes(b, w.tabWidth)

	if w.flatDepth > 0 && w.column+w.lineTrailing > w.printWidth {
		w.flatOverflow = true
	}
}

// SetPrintWidth overrides the print width used for fit decisions.
func (w *Writer) SetPrintWidth(printWidth int) {
	w.printWidth = printWidth
}

// PrintWidth reports the print width currently used for fit decisions.
func (w *Writer) PrintWidth() int {
	return w.printWidth
}

// WithTrailing runs fn with n extra columns reserved for content that will share
// the current line after fn (a statement `;`, a trailing `,`). The reserve only
// tightens fit decisions inside a flat probe, where fn is provably on one line
// with that trailing content; a real Newline inside fn clears it, so a construct
// that breaks does not over-reserve on its interior lines. Reserves nest: an
// inner flat element accounts for every ancestor trailer that shares its line.
func (w *Writer) WithTrailing(n int, fn func()) {
	saved := w.lineTrailing
	w.lineTrailing += n
	fn()
	w.lineTrailing = saved
}

// Space appends a single space.
func (w *Writer) Space() {
	w.Write(" ")
}

// Newline emits the configured line terminator and arms a pending indent.
// Inside TryFlat it marks the block overflowed instead; a flat emission
// produces no line breaks, so a newline forces the caller to retry broken.
//
// A Newline that leaves two or more trailing terminators satisfies a pending
// RequestBlankLine: the break it just emitted is the blank line requested.
func (w *Writer) Newline() {
	if w.flatOverflow {
		return
	}

	if w.flatDepth > 0 {
		w.flatOverflow = true

		return
	}

	w.buf.WriteString(w.eol)
	w.column = 0
	w.pendingIndent = true
	w.lineTrailing = 0

	if w.pendingBlank && countTrailingNewlines(w.buf.Bytes()) >= 2 {
		w.pendingBlank = false
	}
}

// RequestBlankLine asks for a blank line before the next non-blank emission,
// consumed by the next Write or Newline. Repeated requests collapse into one
// blank, so a rule-driven and a trivia-driven request may stack without
// producing a double blank. Inside TryFlat it marks the block overflowed, as
// Newline does.
func (w *Writer) RequestBlankLine() {
	if w.flatOverflow {
		return
	}

	// A blank requested mid-line (pendingIndent cleared) can't be flattened, so
	// it fails the fit attempt like Newline. A blank at the start of a fresh
	// line is a statement-separator carried on leading trivia; it sits above
	// the construct, not within its width, so it is deferred and the construct
	// can still emit flat.
	if w.flatDepth > 0 && !w.pendingIndent {
		w.flatOverflow = true

		return
	}

	w.pendingBlank = true
}

func (w *Writer) consumeBlank() {
	if !w.pendingBlank {
		return
	}

	w.pendingBlank = false

	if w.buf.Len() == 0 {
		return
	}

	trailing := countTrailingNewlines(w.buf.Bytes())

	if trailing >= 2 {
		return
	}

	for range 2 - trailing {
		w.buf.WriteString(w.eol)
	}

	w.column = 0
	w.pendingIndent = true
}

// Both LF and CRLF end in `\n`, so counting `\n` covers either terminator.
func countTrailingNewlines(b []byte) int {
	n := 0

	for i := len(b) - 1; i >= 0 && b[i] == '\n'; i-- {
		n++
	}

	return n
}

// Indent increases the indent level by one. Indent bytes are emitted lazily on
// the first Write after the next Newline.
func (w *Writer) Indent() {
	w.indent++
}

// IndentColumn returns the visual column the next Write would land at after a
// Newline flushed the pending indent. Tabs and spaces both count tabWidth per
// level, so callers need not branch on useTabs.
func (w *Writer) IndentColumn() int {
	return w.indent*w.tabWidth + w.align
}

// CursorColumn returns the column the next byte will occupy: the indent column
// when a newline is pending (the indent has not flushed yet, so Column is still
// 0), else the current column.
func (w *Writer) CursorColumn() int {
	if w.pendingIndent {
		return w.IndentColumn()
	}

	return w.Column()
}

// EOL returns the line terminator this Writer emits, for a caller that has to
// normalize terminators inside a span it writes as one piece.
func (w *Writer) EOL() string {
	return w.eol
}

// Remaining returns how many visual columns are left before printWidth,
// counting a pending indent and any columns WithTrailing reserved. A construct
// whose flat width is at most this fits on the current line, which is the test
// TryFlat applies after emitting speculatively; an emitter that can measure its
// own flat width uses this instead and skips the speculative emission.
func (w *Writer) Remaining() int {
	return w.printWidth - w.CursorColumn() - w.lineTrailing
}

// Dedent decreases the indent level by one. Calls beyond zero are
// ignored so emitters can pair Indent/Dedent defensively.
func (w *Writer) Dedent() {
	if w.indent > 0 {
		w.indent--
	}
}

// WithoutIndent runs fn with the indent level temporarily zero, restored on
// return. For emitters embedding verbatim content (an HTML <script> body)
// where re-indenting would corrupt the source.
func (w *Writer) WithoutIndent(fn func()) {
	saved, savedAlign := w.indent, w.align
	w.indent, w.align = 0, 0
	fn()
	w.indent, w.align = saved, savedAlign
}

// WithAlign runs fn with lines starting n columns past the indent level.
func (w *Writer) WithAlign(n int, fn func()) {
	w.align += n
	fn()
	w.align -= n
}

// TryFlat runs fn in flat-emission mode and reports whether it fit. A Newline
// or a Write past printWidth marks the block overflowed. On fit the bytes stay
// and it returns true; otherwise the buffer rolls back to its pre-call state
// and it returns false. Nested calls compose; a nested rollback does not taint
// an outer flat block still within budget.
func (w *Writer) TryFlat(fn func()) bool {
	save, fit := w.probe(fn)

	if !fit {
		w.rollback(save)
	}

	w.flatOverflow = save.flatOverflow
	w.lineTrailing = save.lineTrailing

	return fit
}

// TryOneLine runs fn and keeps its output only if it wrote no line break,
// reporting whether it did. Unlike TryFlat it lets the line run past the width.
func (w *Writer) TryOneLine(fn func()) bool {
	save := w.saveState()
	start := w.startOfOutput()

	fn()

	if !bytes.ContainsAny(w.buf.Bytes()[start:], "\r\n") {
		// With nothing written, the blank stays deferred for whatever comes next.
		if w.buf.Len() == start {
			w.rollback(save)
		}

		return true
	}

	w.rollback(save)
	w.flatOverflow = save.flatOverflow
	w.lineTrailing = save.lineTrailing

	return false
}

// MeasureOneLine runs fn as it would really emit and always rolls back,
// reporting whether it wrote no line break. Width plays no part.
func (w *Writer) MeasureOneLine(fn func()) bool {
	save := w.saveState()
	start := w.startOfOutput()

	fn()

	oneLine := !bytes.ContainsAny(w.buf.Bytes()[start:], "\r\n")

	w.rollback(save)
	w.flatOverflow = save.flatOverflow
	w.lineTrailing = save.lineTrailing

	return oneLine
}

// startOfOutput writes a deferred blank line now and returns the offset the
// next emission starts at. The blank sits above that emission, so a one-line
// check must not read its line break as one of the emission's own.
func (w *Writer) startOfOutput() int {
	w.consumeBlank()

	return w.buf.Len()
}

// Measure is TryFlat that always rolls back, reporting only whether the
// output fit the print width.
func (w *Writer) Measure(fn func()) bool {
	save, fit := w.probe(fn)

	w.rollback(save)
	w.flatOverflow = save.flatOverflow
	w.lineTrailing = save.lineTrailing

	return fit
}

// InFlatProbe reports whether a TryFlat or Measure is running. Such a probe
// stops writing once its line overflows, so output read back inside one can
// be cut short.
func (w *Writer) InFlatProbe() bool {
	return w.flatDepth > 0
}

// probe runs fn in flat mode and reports whether it fit, leaving its output and
// the flat state for the caller to keep or roll back.
func (w *Writer) probe(fn func()) (snapshot, bool) {
	save := w.saveState()

	w.flatDepth++
	w.flatOverflow = false
	fn()
	w.flatDepth--

	return save, !w.flatOverflow && w.column+w.lineTrailing <= w.printWidth
}

func (w *Writer) saveState() snapshot {
	return snapshot{
		bufLen:        w.buf.Len(),
		column:        w.column,
		indent:        w.indent,
		align:         w.align,
		pendingIndent: w.pendingIndent,
		pendingBlank:  w.pendingBlank,
		flatOverflow:  w.flatOverflow,
		lineTrailing:  w.lineTrailing,
	}
}

func (w *Writer) rollback(s snapshot) {
	w.buf.Truncate(s.bufLen)
	w.column = s.column
	w.indent = s.indent
	w.align = s.align
	w.pendingIndent = s.pendingIndent
	w.pendingBlank = s.pendingBlank
}

func (w *Writer) flushIndent() {
	if !w.pendingIndent {
		return
	}

	w.pendingIndent = false

	if w.useTabs {
		for range w.indent {
			w.buf.WriteByte('\t')
			w.column += w.tabWidth
		}

		w.buf.WriteString(strings.Repeat(" ", w.align))
		w.column += w.align

		return
	}

	spaces := w.IndentColumn()
	w.buf.WriteString(strings.Repeat(" ", spaces))
	w.column += spaces
}

// eolBytes maps config.EndOfLine to the bytes emitted per Newline. Auto falls
// through to `\n`: run and the LSP settle it with config.ResolveEndOfLine
// before building a Writer, so this only covers direct callers.
func eolBytes(setting string) string {
	switch setting {
	case config.EndOfLineCRLF:
		return "\r\n"
	default:
		return "\n"
	}
}

func visualWidth(s string, tabWidth int) int {
	width := 0

	for _, c := range s {
		switch c {
		case '\t':
			width += tabWidth
		case '\r':
			// No visual column, and the text emitters trim it off a line end, so
			// counting it would decide a break on a byte that never survives.
		default:
			width += textwidth.Rune(c)
		}
	}

	return width
}

// VisualWidthBytes is visualWidth over b, without copying it to a string.
func VisualWidthBytes(b []byte, tabWidth int) int {
	width := 0

	for len(b) > 0 {
		c, size := utf8.DecodeRune(b)

		switch c {
		case '\t':
			width += tabWidth
		case '\r':
		default:
			width += textwidth.Rune(c)
		}

		b = b[size:]
	}

	return width
}
