package diff

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/terminal"
)

// Snippet renders a one-line source excerpt with a caret under col (1-based
// byte offset), or "" when line is out of range.
func Snippet(src []byte, line, col int) string {
	text, ok := lineAt(src, line)
	if !ok {
		return ""
	}

	number := strconv.Itoa(line)
	gutter := number + " │ "
	caretGutter := strings.Repeat(" ", len(number)) + " │ "

	var out strings.Builder

	fmt.Fprintf(&out, "%s%s%s%s\n", terminal.Dim, gutter, terminal.Reset, text)
	fmt.Fprintf(&out, "%s%s%s%s%s^%s\n", terminal.Dim, caretGutter, terminal.Reset, caretIndent(text, col), terminal.Red, terminal.Reset)

	return out.String()
}

func lineAt(src []byte, line int) (string, bool) {
	if len(src) == 0 || line < 1 {
		return "", false
	}

	// Lines end as the parsers count them: at LF, CRLF or a lone CR.
	for range line - 1 {
		end := bytes.IndexAny(src, "\r\n")
		if end < 0 {
			return "", false
		}

		if src[end] == '\r' && end+1 < len(src) && src[end+1] == '\n' {
			end++
		}

		src = src[end+1:]
	}

	if end := bytes.IndexAny(src, "\r\n"); end >= 0 {
		src = src[:end]
	}

	return string(src), true
}

// Tabs in the prefix are kept so the caret aligns at any tab width; other
// runes collapse to one space. col is clamped into the line so a
// past-the-end column (unexpected-EOF) points just after the last character.
func caretIndent(text string, col int) string {
	n := min(max(col-1, 0), len(text))

	var b strings.Builder

	for _, r := range text[:n] {
		if r == '\t' {
			b.WriteByte('\t')
		} else {
			b.WriteByte(' ')
		}
	}

	return b.String()
}
