package markdown

import "bytes"

// InlineWrapUnits splits content at whitespace into the units a wrap must keep
// whole: code spans, links, images and autolinks are atomic, but emphasis is
// not, since CommonMark pairs its delimiters across line breaks.
func InlineWrapUnits(content []byte) [][]byte {
	var units [][]byte

	n := len(content)
	i := 0

	for i < n {
		for i < n && isInlineWhitespace(content[i]) {
			i++
		}

		if i >= n {
			break
		}

		start := i

		for i < n {
			if isInlineWhitespace(content[i]) {
				break
			}

			if content[i] == '\\' && i+1 < n {
				i += 2

				continue
			}

			if end, ok := scanInlineAtom(content, i); ok {
				i = end

				continue
			}

			i++
		}

		units = append(units, content[start:i])
	}

	return units
}

// SplitHardBreaks cuts content at CommonMark hard breaks and returns the exact
// break bytes, one fewer than segments; the last line never breaks.
func SplitHardBreaks(content []byte) (segments, breaks [][]byte) {
	start := 0

	for off := 0; off < len(content); {
		line, next := lineAt(content, off)

		if next >= len(content) {
			break
		}

		width := hardBreakWidth(line)
		if width == 0 {
			off = next

			continue
		}

		lineEnd := off + len(line)
		segments = append(segments, content[start:lineEnd-width])
		breaks = append(breaks, content[lineEnd-width:lineEnd])
		start = next
		off = next
	}

	return append(segments, content[start:]), breaks
}

func hardBreakWidth(line []byte) int {
	if spaces := len(line) - len(bytes.TrimRight(line, " ")); spaces >= 2 {
		return spaces
	}

	if backslashes := len(line) - len(bytes.TrimRight(line, `\`)); backslashes%2 == 1 {
		return 1
	}

	return 0
}

func isInlineWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func scanInlineAtom(content []byte, i int) (int, bool) {
	if i >= len(content) {
		return 0, false
	}

	switch content[i] {
	case '`':
		return scanInlineCodeSpan(content, i)
	case '[':
		return scanInlineLinkLike(content, i)
	case '!':
		if i+1 < len(content) && content[i+1] == '[' {
			return scanInlineLinkLike(content, i+1)
		}
	case '<':
		return scanInlineAutolink(content, i)
	}

	return 0, false
}

func scanInlineCodeSpan(content []byte, i int) (int, bool) {
	n := len(content)

	if i >= n || content[i] != '`' {
		return 0, false
	}

	openLen := 0

	for i+openLen < n && content[i+openLen] == '`' {
		openLen++
	}

	j := i + openLen

	for j < n {
		if content[j] != '`' {
			j++

			continue
		}

		runStart := j

		for j < n && content[j] == '`' {
			j++
		}

		if j-runStart == openLen {
			return j, true
		}
	}

	return 0, false
}

// A bare [label] is kept whole too: whether it is a shortcut link depends on
// definitions this scan cannot see.
func scanInlineLinkLike(content []byte, i int) (int, bool) {
	n := len(content)

	if i >= n || content[i] != '[' {
		return 0, false
	}

	j := i + 1
	depth := 1

	for j < n && depth > 0 {
		c := content[j]

		if c == '\\' && j+1 < n {
			j += 2

			continue
		}

		switch c {
		case '[':
			depth++
		case ']':
			depth--

			if depth == 0 {
				j++
			}
		}

		if depth > 0 {
			j++
		}
	}

	if depth > 0 {
		return 0, false
	}

	if j < n && content[j] == '(' {
		if end, ok := scanInlineLinkDest(content, j); ok {
			return end, true
		}
	}

	if j < n && content[j] == '[' {
		if end, ok := scanInlineRefBracket(content, j); ok {
			return end, true
		}
	}

	return j, true
}

func scanInlineLinkDest(content []byte, i int) (int, bool) {
	n := len(content)

	if i >= n || content[i] != '(' {
		return 0, false
	}

	j := i + 1
	depth := 1

	for j < n && depth > 0 {
		c := content[j]

		if c == '\\' && j+1 < n {
			j += 2

			continue
		}

		switch c {
		case '(':
			depth++
		case ')':
			depth--

			if depth == 0 {
				j++
			}
		}

		if depth > 0 {
			j++
		}
	}

	if depth > 0 {
		return 0, false
	}

	return j, true
}

func scanInlineRefBracket(content []byte, i int) (int, bool) {
	n := len(content)

	if i >= n || content[i] != '[' {
		return 0, false
	}

	j := i + 1

	for j < n {
		c := content[j]

		if c == '\\' && j+1 < n {
			j += 2

			continue
		}

		if c == ']' {
			return j + 1, true
		}

		if c == '[' {
			return 0, false
		}

		j++
	}

	return 0, false
}

func scanInlineAutolink(content []byte, i int) (int, bool) {
	n := len(content)

	if i >= n || content[i] != '<' {
		return 0, false
	}

	j := i + 1
	sawColon := false
	sawAt := false

	for j < n {
		c := content[j]

		if c == '>' {
			if !sawColon && !sawAt {
				return 0, false
			}

			return j + 1, true
		}

		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '<' {
			return 0, false
		}

		if c == ':' {
			sawColon = true
		}

		if c == '@' {
			sawAt = true
		}

		j++
	}

	return 0, false
}
