package formatter

import "bytes"

// ignoreDirective is the payload of a laravel-fmt-ignore comment; it lives in
// source because skipping a region is the author's call, not project style.
const ignoreDirective = "laravel-fmt-ignore"

// IsIgnore reports whether text is a laravel-fmt-ignore directive comment.
// text may be a whole comment or just the text between its markers. The
// payload must be ignoreDirective followed by end-of-string, whitespace, or
// `:`, so `laravel-fmt-ignore: reason` self-documents while
// `laravel-fmt-ignore-not-really` does not match.
func IsIgnore(text []byte) bool {
	payload := bytes.TrimSpace(stripCommentMarkers(text))

	if !bytes.HasPrefix(payload, []byte(ignoreDirective)) {
		return false
	}

	rest := payload[len(ignoreDirective):]

	if len(rest) == 0 {
		return true
	}

	c := rest[0]

	return c == ' ' || c == '\t' || c == ':'
}

// commentMarkers lists longest opener first, so `/**` is consumed whole rather
// than as `/` + `**`.
var commentMarkers = []struct{ open, close string }{
	{"/**", "*/"},
	{"/*", "*/"},
	{"<!--", "-->"},
	{"{{--", "--}}"},
	{"//", ""},
	{"#", ""},
}

// The closer has to be the opener's own: a `*/` ending a `#` line comment is
// text. Read as a marker, it matched or not depending on trailing whitespace
// the emitter trims, so a comment could become a directive on the next pass.
func stripCommentMarkers(text []byte) []byte {
	for _, marker := range commentMarkers {
		if !bytes.HasPrefix(text, []byte(marker.open)) {
			continue
		}

		text = text[len(marker.open):]

		if marker.close == "" {
			return text
		}

		if bytes.HasSuffix(text, []byte(marker.close)) {
			return text[:len(text)-len(marker.close)]
		}

		return text
	}

	return text
}
