package formatter

import (
	"bytes"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

// MarkupQuoteTarget maps the configured quote style to a delimiter for CSS,
// HTML and XML, where "auto" means double quotes, their convention.
func MarkupQuoteTarget(cfg *config.File) byte {
	if cfg.QuoteStyle == config.QuoteStyleSingle {
		return '\''
	}

	return '"'
}

// AttrQuote returns the delimiter for a markup attribute value: the configured
// style when the value can adopt it, else src unchanged. Markup attributes have
// no backslash escape, so a value already holding the target quote cannot be
// redelimited without entity-encoding and is kept as-is.
func AttrQuote(src byte, value []byte, cfg *config.File) byte {
	target := MarkupQuoteTarget(cfg)

	if src != target && bytes.IndexByte(value, target) < 0 {
		return target
	}

	return src
}

// UnquotedAttrDelimiter returns the quote an unquoted attribute value can adopt:
// the configured one, the other when the value holds that, or 0 when it holds
// both and markup has no way to delimit it. A value that cannot adopt a
// delimiter has to keep its exact bytes, since a pass that introduces a space
// would split it into two attributes.
func UnquotedAttrDelimiter(value []byte, cfg *config.File) byte {
	target := MarkupQuoteTarget(cfg)
	if bytes.IndexByte(value, target) < 0 {
		return target
	}

	alt := byte('"')
	if target == '"' {
		alt = '\''
	}

	if bytes.IndexByte(value, alt) < 0 {
		return alt
	}

	return 0
}

// WriteAttrValue writes a markup attribute value delimited by the quote
// AttrQuote picks for it. Pass src 0 for a value the source left unquoted: it
// gains the configured delimiter, unless it holds that delimiter already, in
// which case there is no quote to adopt and the bytes go out bare.
func WriteAttrValue(w *Writer, src byte, value []byte, cfg *config.File) {
	quote := AttrQuote(src, value, cfg)

	if quote == 0 {
		w.WriteBytes(value)

		return
	}

	w.Write(string(quote))
	w.WriteBytes(value)
	w.Write(string(quote))
}
