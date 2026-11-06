package javascript

import (
	"bytes"

	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/javascript"
)

var lineStartContinuations = map[string]bool{
	".": true, "?.": true, "?": true, ":": true, "&&": true, "||": true, "??": true,
}

var (
	conditionKeywords = map[string]bool{keywordIf: true, keywordFor: true, keywordWhile: true}
	bodyKeywords      = map[string]bool{keywordElse: true, keywordDo: true}
	labelKeywords     = map[string]bool{keywordCase: true, keywordDefault: true}
)

const (
	keywordIf      = "if"
	keywordFor     = "for"
	keywordWhile   = "while"
	keywordElse    = "else"
	keywordDo      = "do"
	keywordCase    = "case"
	keywordDefault = "default"
)

type bracket struct {
	level    int
	line     int
	exprLine int
	head     bool
}

// LineDepths returns each line's indent level by bracket depth, or false when
// the value's layout must stay as written.
func LineDepths(value []byte) ([]int, bool) {
	levels, ok := BlockDepths(value)
	if !ok {
		return nil, false
	}

	if first, _, _ := bytes.Cut(value, []byte("\n")); len(bytes.TrimLeft(first, " \t")) == 0 {
		for i := 1; i < len(levels); i++ {
			levels[i]++
		}
	}

	return levels, true
}

// BlockDepths is LineDepths for a block whose first line is laid out like the
// rest, such as a script body.
func BlockDepths(body []byte) ([]int, bool) {
	lines := bytes.Split(body, []byte("\n"))

	for _, line := range lines {
		if !normalizable(line) {
			return nil, false
		}
	}

	toks, ok := parser.Lex(body)
	if !ok {
		return nil, false
	}

	for _, t := range toks {
		if bytes.IndexByte(body[t.Start:t.End], '\n') >= 0 {
			return nil, false
		}
	}

	// No token crosses a line, so moving the whitespace that starts one cannot
	// fuse or split a token; FuzzLineDepths holds the layout to that.
	levels, ok := lineLevels(body, toks, lines)
	if !ok {
		return nil, false
	}

	return levels, true
}

type leveler struct {
	src     []byte
	toks    []parser.Token
	classes []tokenClass
	levels  []int
	stack   []bracket

	prevTok   int
	prevEnd   int
	headClose int
}

func lineLevels(src []byte, toks []parser.Token, lines [][]byte) ([]int, bool) {
	l := &leveler{
		src: src, toks: toks, classes: classify(src, toks),
		levels: make([]int, len(lines)), prevTok: -1, prevEnd: -1, headClose: -1,
	}

	next := 0
	lineStart := 0

	for i, line := range lines {
		lineEnd := lineStart + len(line)
		first := next
		closedLevel := -1

		if i > 0 {
			if next < len(toks) && toks[next].Start < lineEnd {
				level, ok := l.lineLevel(next)
				if !ok {
					return nil, false
				}

				l.levels[i] = level
			} else {
				l.levels[i] = l.innerLevel()
			}
		}

		for ; next < len(toks) && toks[next].Start < lineEnd; next++ {
			if toks[next].Kind == parser.KindComment {
				continue
			}

			if !l.read(next, i, &closedLevel) {
				return nil, false
			}
		}

		if l.prevTok >= first && l.prevTok < next {
			l.prevEnd = l.prevTok
		}

		lineStart = lineEnd + 1
	}

	return l.levels, len(l.stack) == 0
}

func (l *leveler) lineLevel(first int) (int, bool) {
	t := l.toks[first]
	text := string(l.src[t.Start:t.End])

	if t.Kind == parser.KindPunct && closesBracket(text) {
		if len(l.stack) == 0 {
			return 0, false
		}

		return l.stack[len(l.stack)-1].level, true
	}

	level := l.innerLevel()

	switch {
	case l.endsHead():
		if t.Kind != parser.KindPunct || text != "{" {
			level++
		}
	case l.continues(first) && !l.huggedExpr():
		level++
	}

	return level, true
}

func (l *leveler) read(i, line int, closedLevel *int) bool {
	t := l.toks[i]
	text := string(l.src[t.Start:t.End])

	if top := l.top(); top != nil && top.exprLine < 0 {
		top.exprLine = line
	}

	isPunct := t.Kind == parser.KindPunct

	switch {
	case isPunct && opensBracket(text):
		level := l.levels[line]
		if *closedLevel >= 0 {
			level = *closedLevel
		}

		head := text == "(" && l.prevTok >= 0 && isControlKeyword(l.src, l.toks[l.prevTok])
		l.stack = append(l.stack, bracket{level: level, line: line, exprLine: -1, head: head})
	case isPunct && closesBracket(text):
		if len(l.stack) == 0 {
			return false
		}

		closed := l.stack[len(l.stack)-1]
		l.stack = l.stack[:len(l.stack)-1]

		if closed.head {
			l.headClose = i
		}

		if closed.line < line {
			*closedLevel = closed.level
		}
	case isPunct && (text == "," || text == ";"):
		if top := l.top(); top != nil {
			top.exprLine = -1
		}
	}

	l.prevTok = i

	return true
}

func (l *leveler) top() *bracket {
	if len(l.stack) == 0 {
		return nil
	}

	return &l.stack[len(l.stack)-1]
}

func (l *leveler) innerLevel() int {
	if top := l.top(); top != nil {
		return top.level + 1
	}

	return 0
}

// huggedExpr reports whether the bracket around the expression being continued
// already sets it apart: the expression started on the line the bracket opened
// on (`if (a &&`, `.map((el) =>`), or the bracket holds a condition.
func (l *leveler) huggedExpr() bool {
	top := l.top()

	return top != nil && (top.exprLine == top.line || top.head)
}

func (l *leveler) endsHead() bool {
	if l.prevEnd < 0 {
		return false
	}

	end := l.toks[l.prevEnd]

	return l.prevEnd == l.headClose || (end.Kind == parser.KindIdent && bodyKeywords[string(l.src[end.Start:end.End])])
}

func (l *leveler) continues(first int) bool {
	t := l.toks[first]
	if t.Kind == parser.KindPunct && lineStartContinuations[string(l.src[t.Start:t.End])] {
		return true
	}

	if l.prevEnd < 0 {
		return false
	}

	end := l.prevEnd

	return isBinary(l.src, l.toks, l.classes, end) || l.classes[end].ternary ||
		(isPunctText(l.src, l.toks[end], ":") && !l.endsLabel(end))
}

func (l *leveler) endsLabel(i int) bool {
	for j := i - 1; j >= 0; j-- {
		t := l.toks[j]
		text := string(l.src[t.Start:t.End])

		if t.Kind == parser.KindIdent && labelKeywords[text] {
			return true
		}

		if t.Kind == parser.KindPunct && (text == ";" || text == "{" || text == "}" || text == ",") {
			return false
		}
	}

	return false
}

func opensBracket(text string) bool {
	return text == "(" || text == "[" || text == "{"
}

func closesBracket(text string) bool {
	return text == ")" || text == "]" || text == "}"
}

func isControlKeyword(src []byte, t parser.Token) bool {
	return t.Kind == parser.KindIdent && conditionKeywords[string(src[t.Start:t.End])]
}
