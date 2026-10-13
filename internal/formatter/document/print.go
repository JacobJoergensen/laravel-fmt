package document

import (
	"bytes"
	"slices"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/textwidth"
)

// Options are the layout parameters Print resolves a Doc against.
type Options struct {
	PrintWidth int
	TabWidth   int
	UseTabs    bool
	EOL        string

	// StartColumn is the column the first line already sits at, for a fragment written
	// into a line the host has begun; later lines carry their own indent.
	StartColumn int
}

type mode uint8

const (
	modeBreak mode = iota
	modeFlat
)

type cmd struct {
	indent int
	mode   mode
	doc    *Doc
	prefix string
}

// Print lays out d against opts and returns bytes the caller owns.
func Print(d *Doc, opts Options) []byte {
	propagateBreaks(d)

	if opts.TabWidth <= 0 {
		opts.TabWidth = 4
	}

	if opts.EOL == "" {
		opts.EOL = "\n"
	}

	p := &printer{opts: opts, groupMode: map[int32]mode{}, pos: opts.StartColumn}
	p.stack = append(p.stack, cmd{0, modeBreak, d, ""})

	// A LineSuffix still pending at the end has no line break left to flush it,
	// so it is pushed and the stack drained again.
	for {
		for len(p.stack) > 0 {
			c := p.stack[len(p.stack)-1]
			p.stack = p.stack[:len(p.stack)-1]
			p.step(c)
		}

		if len(p.lineSuffix) == 0 {
			break
		}

		p.pushReversed(0, modeBreak, "", suffixDocs(p.lineSuffix))
		p.lineSuffix = p.lineSuffix[:0]
	}

	if len(p.gaps) > 0 {
		return expandAlignGaps(p.out.Bytes(), p.gaps)
	}

	return p.out.Bytes()
}

type printer struct {
	opts       Options
	out        bytes.Buffer
	pos        int
	lineIndent int
	stack      []cmd
	lineSuffix []cmd
	groupMode  map[int32]mode
	gaps       []alignGap

	fitStack []cmd
}

func (p *printer) step(c cmd) {
	switch c.doc.kind {
	case kindText:
		p.out.WriteString(c.doc.text)

		if nl := strings.LastIndexByte(c.doc.text, '\n'); nl >= 0 {
			p.pos = visualWidth(c.doc.text[nl+1:], p.opts.TabWidth)
			p.lineIndent = 0
		} else {
			p.pos += visualWidth(c.doc.text, p.opts.TabWidth)
		}

	case kindAlignGap:
		// The padding depends on which lines form a run, which the layout has
		// yet to settle, so the gap reserves the most it can grow by. Where
		// padding from keys below would break what follows, the entry keeps the
		// padding from a wider key above and ends the run; where even that would
		// break it, the entry stands alone unpadded.
		reserve := len(c.doc.text)
		cut, alone := false, false

		if reserve > 0 && !p.restFits(1+reserve) {
			above := max(p.runColumn(c.doc.groupID)-p.pos, 0)

			switch {
			case above < reserve && p.restFits(1+above):
				reserve, cut = above, true
			case p.restFits(1):
				reserve, cut, alone = 0, true, true
			}
		}

		p.gaps = append(p.gaps, alignGap{offset: p.out.Len(), column: p.pos, scope: c.doc.groupID, reserve: reserve, cut: cut, alone: alone})
		p.out.WriteByte(' ')
		p.pos += 1 + reserve

	case kindConcat:
		p.pushReversed(c.indent, c.mode, c.prefix, c.doc.parts)

	case kindIndent:
		p.push(c.indent+p.opts.TabWidth, c.mode, c.prefix, c.doc.parts[0])

	case kindLinePrefix:
		p.push(c.indent, c.mode, c.prefix+c.doc.text, c.doc.parts[0])

	case kindGroup:
		p.stepGroup(c)

	case kindOpener:
		p.push(c.indent, c.mode, c.prefix, c.doc.parts[0])

	case kindCondGroup:
		p.stepCondGroup(c)

	case kindLine, kindSoftLine, kindHardLine:
		p.stepLine(c)

	case kindHeredoc:
		p.stepHeredoc(c)

	case kindVerbatim:
		p.stepVerbatim(c)

	case kindLineSuffix:
		p.lineSuffix = append(p.lineSuffix, cmd{c.indent, c.mode, c.doc.parts[0], c.prefix})

	case kindIfBreak:
		if p.resolvedMode(c) == modeBreak {
			p.push(c.indent, c.mode, c.prefix, c.doc.parts[0])
		} else {
			p.push(c.indent, c.mode, c.prefix, c.doc.parts[1])
		}

	case kindBreakParent:
	}
}

func (p *printer) stepGroup(c cmd) {
	if c.mode == modeFlat && !c.doc.mustWrap {
		p.recordGroup(c.doc, modeFlat)
		p.push(c.indent, modeFlat, c.prefix, c.doc.parts[0])

		return
	}

	if !c.doc.mustWrap && p.fits(cmd{c.indent, modeFlat, c.doc.parts[0], c.prefix}) {
		p.recordGroup(c.doc, modeFlat)
		p.push(c.indent, modeFlat, c.prefix, c.doc.parts[0])

		return
	}

	// A group whose opener was pushed onto a continuation line indents its body
	// from that line, not its structural depth, so the body does not sit level
	// with the line that opened it (PHP-CS-Fixer statement_indentation).
	p.recordGroup(c.doc, modeBreak)
	p.push(max(c.indent, p.lineIndent), modeBreak, c.prefix, c.doc.parts[0])
}

// stepCondGroup pushes the chosen variant in break mode: a compact variant
// has no breaks of its own, so it stays inline while nested content (a hugged
// closure body) still breaks.
func (p *printer) stepCondGroup(c cmd) {
	variants := c.doc.parts
	for i := range len(variants) - 1 {
		if p.fitsWith(cmd{c.indent, modeFlat, variants[i], c.prefix}, fitFlatForced) {
			p.push(c.indent, modeBreak, c.prefix, variants[i])

			return
		}
	}

	p.push(c.indent, modeBreak, c.prefix, variants[len(variants)-1])
}

func (p *printer) stepLine(c cmd) {
	if c.mode == modeFlat && c.doc.kind != kindHardLine {
		if c.doc.kind == kindLine {
			p.out.WriteString(" ")
			p.pos++
		}

		return
	}

	if len(p.lineSuffix) > 0 {
		p.stack = append(p.stack, c)
		p.pushReversed(c.indent, c.mode, c.prefix, suffixDocs(p.lineSuffix))
		p.lineSuffix = p.lineSuffix[:0]

		return
	}

	p.trimTrailingSpace()
	p.out.WriteString(p.opts.EOL)
	p.writeIndent(c.indent)
	p.pos = c.indent
	p.lineIndent = c.indent

	if c.prefix != "" {
		p.out.WriteString(c.prefix)
		p.pos += visualWidth(c.prefix, p.opts.TabWidth)
	}
}

// stepHeredoc writes the body raw, never through stepLine's trailing-space
// trim, so a body line's significant whitespace survives.
func (p *printer) stepHeredoc(c cmd) {
	col := c.indent
	if c.doc.mustWrap { // plusOne
		col += p.opts.TabWidth
	}

	out, ok := reindentHeredoc(c.doc.text, indentWhitespace(col, p.opts.UseTabs, p.opts.TabWidth))
	if !ok {
		out = c.doc.text
	}

	p.out.WriteString(out)

	if nl := strings.LastIndexByte(out, '\n'); nl >= 0 {
		p.pos = visualWidth(out[nl+1:], p.opts.TabWidth)
		p.lineIndent = col
	} else {
		p.pos += visualWidth(out, p.opts.TabWidth)
	}
}

func (p *printer) stepVerbatim(c cmd) {
	p.out.WriteString(c.doc.text)
	p.pos += visualWidth(c.doc.text, p.opts.TabWidth)

	for _, ln := range c.doc.parts {
		p.out.WriteString(p.opts.EOL)

		if ln.text == "" {
			p.pos = 0

			continue
		}

		p.writeIndent(p.lineIndent)
		p.out.WriteString(ln.text)
		p.pos = p.lineIndent + visualWidth(ln.text, p.opts.TabWidth)
	}
}

// restFits reports whether the rest of the current line fits after a gap of
// width columns, measured as it prints when nothing in it has to break.
func (p *printer) restFits(width int) bool {
	p.pos += width
	fits := p.fitsWith(cmd{doc: emptyText}, fitFlatRest)
	p.pos -= width

	return fits
}

var emptyText = Text("")

// runColumn is the widest column among the gaps of scope already printed on
// the lines directly above, the run the next gap of that scope would join.
func (p *printer) runColumn(scope int32) int {
	widest, at := 0, p.out.Len()

	for _, g := range slices.Backward(p.gaps) {
		if g.scope != scope {
			continue
		}

		if g.cut || bytes.Count(p.out.Bytes()[g.offset:at], []byte{'\n'}) != 1 {
			break
		}

		widest, at = max(widest, g.column), g.offset
	}

	return widest
}

func (p *printer) fits(next cmd) bool {
	return p.fitsWith(next, fitAsIs)
}

// fitMode picks how fitsWith measures groups it has not laid out yet.
type fitMode int

const (
	fitAsIs fitMode = iota
	// An author-broken group is measured flat too, so the width of a chain
	// signature does not depend on whether an inner argument is broken. A
	// true hard break (a hugged closure body) still stops the scan.
	fitFlatForced
	// Every group the author did not break is measured flat: the rest of the
	// line as it prints when nothing in it has to break.
	fitFlatRest
)

// CONTRACT: not reentrant. The scan runs on p.fitStack, reused across calls, so
// a fit test started from inside another one would clobber it.
func (p *printer) fitsWith(next cmd, fm fitMode) bool {
	remaining := p.opts.PrintWidth - p.pos
	if remaining < 0 {
		return false
	}

	p.fitStack = append(p.fitStack[:0], next)
	restIdx := len(p.stack)

	for remaining >= 0 {
		if len(p.fitStack) == 0 {
			if restIdx == 0 {
				return true
			}

			restIdx--
			p.fitStack = append(p.fitStack, p.stack[restIdx])

			continue
		}

		c := p.fitStack[len(p.fitStack)-1]
		p.fitStack = p.fitStack[:len(p.fitStack)-1]

		switch c.doc.kind {
		case kindText:
			remaining -= visualWidth(c.doc.text, p.opts.TabWidth)

		case kindAlignGap:
			remaining -= alignGapWidth(c.doc)

		case kindConcat:
			for _, v := range slices.Backward(c.doc.parts) {
				p.fitStack = append(p.fitStack, cmd{c.indent, c.mode, v, c.prefix})
			}

		case kindIndent:
			p.fitStack = append(p.fitStack, cmd{c.indent + p.opts.TabWidth, c.mode, c.doc.parts[0], c.prefix})

		case kindLinePrefix:
			p.fitStack = append(p.fitStack, cmd{c.indent, c.mode, c.doc.parts[0], c.prefix})

		case kindGroup:
			m := c.mode

			switch {
			case c.doc.mustWrap && fm != fitFlatForced:
				m = modeBreak
			case !c.doc.mustWrap && fm == fitFlatRest:
				m = modeFlat
			}

			p.fitStack = append(p.fitStack, cmd{c.indent, m, c.doc.parts[0], c.prefix})

		case kindOpener:
			remaining -= int(c.doc.groupID)

			return remaining >= 0

		case kindCondGroup:
			w, hard := FlatPrefixWidth(c.doc.parts[0], p.opts.TabWidth)
			remaining -= w

			if hard && (remaining < 0 || fm == fitFlatForced) {
				return remaining >= 0
			}

			// What follows the group shares its last line, so measuring goes on
			// from there rather than stopping at its first break.
			if hard {
				closing, _ := ClosingLineWidth(c.doc.parts[0], p.opts.TabWidth)
				remaining = p.opts.PrintWidth - c.indent - closing
			}

		case kindLine:
			if c.mode == modeBreak {
				return true
			}

			remaining--

		case kindSoftLine:
			if c.mode == modeBreak {
				return true
			}

		case kindHardLine, kindHeredoc, kindVerbatim:
			return true

		case kindIfBreak:
			if p.resolvedMode(c) == modeBreak {
				p.fitStack = append(p.fitStack, cmd{c.indent, c.mode, c.doc.parts[0], c.prefix})
			} else {
				p.fitStack = append(p.fitStack, cmd{c.indent, c.mode, c.doc.parts[1], c.prefix})
			}

		case kindLineSuffix, kindBreakParent:
		}
	}

	return false
}

func (p *printer) resolvedMode(c cmd) mode {
	if c.doc.groupID != 0 {
		if m, ok := p.groupMode[c.doc.groupID]; ok {
			return m
		}

		return modeFlat
	}

	return c.mode
}

func (p *printer) recordGroup(d *Doc, m mode) {
	if d.groupID != 0 {
		p.groupMode[d.groupID] = m
	}
}

func (p *printer) push(indent int, m mode, prefix string, d *Doc) {
	p.stack = append(p.stack, cmd{indent, m, d, prefix})
}

func (p *printer) pushReversed(indent int, m mode, prefix string, parts []*Doc) {
	for _, part := range slices.Backward(parts) {
		p.stack = append(p.stack, cmd{indent, m, part, prefix})
	}
}

func suffixDocs(suffixes []cmd) []*Doc {
	docs := make([]*Doc, len(suffixes))
	for i, c := range suffixes {
		docs[i] = c.doc
	}

	return docs
}

func (p *printer) writeIndent(indent int) {
	if p.opts.UseTabs {
		for range indent / p.opts.TabWidth {
			p.out.WriteString("\t")
		}

		return
	}

	p.out.WriteString(strings.Repeat(" ", indent))
}

func (p *printer) trimTrailingSpace() {
	b := p.out.Bytes()
	end := len(b)

	for end > 0 && (b[end-1] == ' ' || b[end-1] == '\t') {
		end--
	}

	p.out.Truncate(end)
}

func propagateBreaks(d *Doc) bool {
	switch d.kind {
	case kindHardLine, kindBreakParent, kindHeredoc, kindVerbatim:
		return true

	case kindText, kindAlignGap, kindLine, kindSoftLine:
		return false

	case kindLineSuffix, kindIfBreak:
		// Deferred / conditional content does not force the current line to break.
		for _, c := range d.parts {
			propagateBreaks(c)
		}

		return false

	case kindGroup:
		if propagateBreaks(d.parts[0]) {
			d.mustWrap = true
		}

		return d.mustWrap

	case kindIndent, kindLinePrefix, kindOpener:
		return propagateBreaks(d.parts[0])

	case kindCondGroup:
		// The group chooses a variant per width, so a forced break inside one
		// (a hugged closure) neither forces an enclosing group to break nor
		// collapses the choice. Each variant is still walked to resolve its own
		// nested groups.
		for _, c := range d.parts {
			propagateBreaks(c)
		}

		return false

	case kindConcat:
		forced := false
		for _, c := range d.parts {
			if propagateBreaks(c) {
				forced = true
			}
		}

		return forced
	}

	return false
}

func visualWidth(s string, tabWidth int) int {
	width := 0

	for _, r := range s {
		switch r {
		case '\t':
			width += tabWidth
		case '\r':
			// A carriage return renders as no column, and every embedded path
			// strips it before emission (stripLineCR, verbatim.go lineEnd), so
			// counting it would let a byte the emitter deletes decide a break.
		default:
			width += textwidth.Rune(r)
		}
	}

	return width
}
