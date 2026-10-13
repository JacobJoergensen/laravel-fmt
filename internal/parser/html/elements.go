package html

import (
	"bytes"
	"slices"
)

const linkTag = "link"

var voidElements = [...][]byte{
	[]byte("area"), []byte("base"), []byte("br"), []byte("col"),
	[]byte("embed"), []byte("hr"), []byte("img"), []byte("input"),
	[]byte(linkTag), []byte("meta"), []byte("source"), []byte("track"),
	[]byte("wbr"),
}

var rawTextElements = [...][]byte{
	[]byte("script"), []byte("style"),
}

var rcdataElements = [...][]byte{
	[]byte("textarea"), []byte("title"),
}

var preformattedElements = [...][]byte{
	[]byte("pre"),
}

var inlineElements = [...][]byte{
	[]byte("a"), []byte("abbr"), []byte("b"), []byte("bdi"), []byte("bdo"),
	[]byte("br"), []byte("cite"), []byte("code"), []byte("data"), []byte("del"),
	[]byte("dfn"), []byte("em"), []byte("i"), []byte("img"), []byte("ins"),
	[]byte("kbd"), []byte("label"), []byte("mark"), []byte("output"),
	[]byte("q"), []byte("rp"), []byte("rt"), []byte("ruby"), []byte("s"),
	[]byte("samp"), []byte("small"), []byte("span"), []byte("strong"),
	[]byte("sub"), []byte("sup"), []byte("time"), []byte("u"), []byte("var"),
	[]byte("wbr"),
	// Replaced and embedded content is display:inline in the HTML standard's
	// rendering section, so whitespace around it is painted the same as around
	// a span.
	[]byte("audio"), []byte("canvas"), []byte("embed"), []byte("iframe"),
	[]byte("map"), []byte("object"), []byte("picture"), []byte("svg"),
	[]byte("video"),
}

// inlineBlockElements are inline-level but not display:inline: their body edges
// keep whitespace, but their content is not forced flat. option is here because
// its text is a control label, not flowed content, and the form controls because
// the standard renders them inline-block.
var inlineBlockElements = [...][]byte{
	[]byte("button"), []byte("input"), []byte("meter"), []byte("option"),
	[]byte("progress"), []byte("select"), []byte("textarea"),
}

// blockElements are the elements the HTML standard's rendering section gives a
// block-like display (block, list-item, a table part), plus the document
// scaffolding around them. Whitespace beside such a box is never painted. Any
// other element, including an unknown or custom one, renders display:inline by
// default, so this list is the only safe test for where a line may break.
var blockElements = [...][]byte{
	[]byte("address"), []byte("article"), []byte("aside"), []byte("blockquote"),
	[]byte("body"), []byte("caption"), []byte("center"), []byte("col"),
	[]byte("colgroup"), []byte("dd"), []byte("details"), []byte("dialog"),
	[]byte("dir"), []byte("div"), []byte("dl"), []byte("dt"),
	[]byte("fieldset"), []byte("figcaption"), []byte("figure"), []byte("footer"),
	[]byte("form"), []byte("h1"), []byte("h2"), []byte("h3"), []byte("h4"),
	[]byte("h5"), []byte("h6"), []byte("head"), []byte("header"),
	[]byte("hgroup"), []byte("hr"), []byte("html"), []byte("legend"),
	[]byte("li"), []byte("listing"), []byte("main"), []byte("menu"),
	[]byte("nav"), []byte("ol"), []byte("p"), []byte("plaintext"),
	[]byte("pre"), []byte("search"), []byte("section"), []byte("summary"),
	[]byte("table"), []byte("tbody"), []byte("td"), []byte("tfoot"),
	[]byte("th"), []byte("thead"), []byte("tr"), []byte("ul"), []byte("xmp"),
}

func containsFold(set [][]byte, name []byte) bool {
	return slices.ContainsFunc(set, func(e []byte) bool {
		return bytes.EqualFold(name, e)
	})
}

// IsVoidElement reports whether name is an HTML5 void element (no children or
// closing tag).
func IsVoidElement(name []byte) bool {
	return containsFold(voidElements[:], name)
}

// IsRawTextElement reports whether name is script or style (content is raw
// text, not HTML).
func IsRawTextElement(name []byte) bool {
	return containsFold(rawTextElements[:], name)
}

// IsRcdataElement reports whether name is textarea or title (body captured
// verbatim, not interpreted as HTML).
func IsRcdataElement(name []byte) bool {
	return containsFold(rcdataElements[:], name)
}

// IsInlineElement reports whether name renders inline (CSS display:inline), so
// whitespace around it is significant to layout.
func IsInlineElement(name []byte) bool {
	return containsFold(inlineElements[:], name)
}

// IsInlineBlockElement reports whether name is inline-level but not display:inline.
func IsInlineBlockElement(name []byte) bool {
	return containsFold(inlineBlockElements[:], name)
}

// IsInlineLevelElement reports whether name is laid out inline, either
// display:inline or inline-block. It is the test for whitespace around and at
// the edges of an element, since a browser paints both the same way; the
// narrower IsInlineElement is the test for whether a run must stay flat.
func IsInlineLevelElement(name []byte) bool {
	return IsInlineElement(name) || IsInlineBlockElement(name)
}

// IsBlockElement reports whether name renders block-like, so a line break
// beside it is not painted as a space.
func IsBlockElement(name []byte) bool {
	return containsFold(blockElements[:], name)
}

// IsPreformattedElement reports whether name is pre: its children are parsed
// HTML (unlike RCDATA) but the formatter emits them verbatim.
func IsPreformattedElement(name []byte) bool {
	return containsFold(preformattedElements[:], name)
}

// optionalEndTags maps an element whose end tag may be omitted to the start
// tags that implicitly close it (the WHATWG optional-tags subset). Table
// sections (thead/tbody/tfoot) are excluded on purpose: doing them right needs
// the table insertion modes, and a partial rule would silently mis-nest.
var optionalEndTags = map[string][]string{
	"li":       {"li"},
	"dt":       {"dt", "dd"},
	"dd":       {"dt", "dd"},
	"option":   {"option", "optgroup"},
	"optgroup": {"optgroup"},
	"tr":       {"tr"},
	"td":       {"td", "th", "tr"},
	"th":       {"td", "th", "tr"},
	"p": {
		"address", "article", "aside", "blockquote", "details", "dialog",
		"div", "dl", "fieldset", "figcaption", "figure", "footer", "form",
		"h1", "h2", "h3", "h4", "h5", "h6", "header", "hgroup", "hr",
		"main", "menu", "nav", "ol", "p", "pre", "search", "section",
		"table", "ul",
	},
}

// HasOptionalEndTag reports whether name is one of the elements in
// optionalEndTags, whose omitted end tag the parser infers.
func HasOptionalEndTag(name []byte) bool {
	for k := range optionalEndTags {
		if bytes.EqualFold(name, []byte(k)) {
			return true
		}
	}

	return false
}

// StartTagClosesCurrent reports whether the opener start tag implicitly closes
// the open current element (a second <li>, or a block element after a <p>).
func StartTagClosesCurrent(current, opener []byte) bool {
	for k, closers := range optionalEndTags {
		if !bytes.EqualFold(current, []byte(k)) {
			continue
		}

		for _, c := range closers {
			if bytes.EqualFold(opener, []byte(c)) {
				return true
			}
		}

		return false
	}

	return false
}
