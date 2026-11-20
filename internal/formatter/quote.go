package formatter

import (
	"bytes"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

// AttrDelimiter is the quote a markup attribute value takes. quoteStyle does
// not reach it: Prettier's singleQuote leaves HTML attributes double-quoted.
const AttrDelimiter = '"'

const attrAltDelimiter = '\''

// CSSQuoteTarget returns the delimiter for CSS strings.
func CSSQuoteTarget(cfg *config.File) byte {
	if cfg.QuoteStyle == config.QuoteStyleSingle {
		return '\''
	}

	return '"'
}

// AttrQuote returns the delimiter for a markup attribute value: AttrDelimiter
// when the value can adopt it, else src unchanged. Markup attributes have no
// backslash escape, so a value already holding a double quote cannot be
// redelimited without entity-encoding and is kept as-is.
func AttrQuote(src byte, value []byte) byte {
	if src != AttrDelimiter && bytes.IndexByte(value, AttrDelimiter) < 0 {
		return AttrDelimiter
	}

	return src
}

// UnquotedAttrDelimiter returns the quote an unquoted attribute value can adopt:
// AttrDelimiter, the single quote when the value holds a double one, or 0 when
// it holds both and markup has no way to delimit it. A value that cannot adopt
// a delimiter has to keep its exact bytes, since a pass that introduces a space
// would split it into two attributes.
func UnquotedAttrDelimiter(value []byte) byte {
	if bytes.IndexByte(value, AttrDelimiter) < 0 {
		return AttrDelimiter
	}

	if bytes.IndexByte(value, attrAltDelimiter) < 0 {
		return attrAltDelimiter
	}

	return 0
}

// WriteAttrValue writes a markup attribute value delimited by the quote
// AttrQuote picks for it. Pass src 0 for a value the source left unquoted: it
// gains AttrDelimiter, unless it holds that delimiter already, in which case
// there is no quote to adopt and the bytes go out bare.
func WriteAttrValue(w *Writer, src byte, value []byte) {
	quote := AttrQuote(src, value)

	if quote == 0 {
		w.WriteBytes(value)

		return
	}

	w.Write(string(quote))
	w.WriteBytes(value)
	w.Write(string(quote))
}
