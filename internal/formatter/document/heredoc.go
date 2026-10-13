package document

import "strings"

// PHP 7.3+ strips the closing label's leading whitespace from every heredoc
// body line, so swapping that prefix for targetWS keeps the value identical.
// A line not carrying the prefix cannot shift, and reports false.
func reindentHeredoc(text, targetWS string) (string, bool) {
	openerEnd := strings.IndexByte(text, '\n')
	if openerEnd < 0 {
		return "", false
	}

	opener := text[:openerEnd+1]
	rest := text[openerEnd+1:]

	closeStart := strings.LastIndexByte(rest, '\n')

	var body, closeLine string

	if closeStart < 0 {
		closeLine = rest
	} else {
		body = rest[:closeStart+1]
		closeLine = rest[closeStart+1:]
	}

	closeWS := leadingWhitespace(closeLine)
	if len(closeWS) == len(closeLine) {
		return "", false
	}

	var b strings.Builder

	b.WriteString(opener)

	if !reindentHeredocBody(&b, body, closeWS, targetWS) {
		return "", false
	}

	b.WriteString(targetWS)
	b.WriteString(closeLine[len(closeWS):])

	return b.String(), true
}

func reindentHeredocBody(b *strings.Builder, body, closeWS, targetWS string) bool {
	for len(body) > 0 {
		line, term, rest := cutHeredocLine(body)
		body = rest

		switch {
		case len(line) == 0:
		case strings.HasPrefix(line, closeWS):
			if tail := line[len(closeWS):]; len(tail) > 0 {
				b.WriteString(targetWS)
				b.WriteString(tail)
			}
		default:
			return false
		}

		b.WriteString(term)
	}

	return true
}

// cutHeredocLine treats a lone CR as a terminator: PHP ends a heredoc body
// line on CR as well as LF and CRLF, and strips the closer's indentation from
// each (checked against PHP).
func cutHeredocLine(body string) (line, term, rest string) {
	for i := range len(body) {
		switch body[i] {
		case '\n':
			return body[:i], "\n", body[i+1:]
		case '\r':
			if i+1 < len(body) && body[i+1] == '\n' {
				return body[:i], "\r\n", body[i+2:]
			}

			return body[:i], "\r", body[i+1:]
		}
	}

	return body, "", ""
}

func leadingWhitespace(line string) string {
	i := 0

	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}

	return line[:i]
}

// indentWhitespace must mirror the printer's writeIndent so a heredoc's re-home
// aligns with the surrounding code.
func indentWhitespace(col int, useTabs bool, tabWidth int) string {
	if useTabs {
		return strings.Repeat("\t", col/tabWidth)
	}

	return strings.Repeat(" ", col)
}
