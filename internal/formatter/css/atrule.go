package css

import (
	"bytes"

	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/css"
)

// cssAtRules holds the standard CSS and Tailwind at-rule names. A <style> body
// with any other @name, such as a Blade @if, is left verbatim.
var cssAtRules = map[string]bool{
	"charset": true, "import": true, "namespace": true, "media": true,
	"supports": true, "container": true, "layer": true, "scope": true,
	"starting-style": true, "page": true, "font-face": true, "keyframes": true,
	"counter-style": true, "font-feature-values": true, "font-palette-values": true,
	"property": true, "position-try": true, "view-transition": true,
	"document": true, "color-profile": true,
	// The @page margin boxes (CSS Paged Media 3). A PDF template nests these
	// inside @page, and without them the whole <style> body reads as foreign.
	"top-left-corner": true, "top-left": true, "top-center": true,
	"top-right": true, "top-right-corner": true,
	"bottom-left-corner": true, "bottom-left": true, "bottom-center": true,
	"bottom-right": true, "bottom-right-corner": true,
	"left-top": true, "left-middle": true, "left-bottom": true,
	"right-top": true, "right-middle": true, "right-bottom": true,
	"tailwind": true, "apply": true, "theme": true, "utility": true,
	"variant": true, "custom-variant": true, "source": true, "reference": true,
	"plugin": true, "config": true, "screen": true, "responsive": true,
	"variants": true,
}

func hasForeignAtRule(nodes []parser.Node) bool {
	for _, n := range nodes {
		switch nn := n.(type) {
		case *parser.AtRule:
			if !isCSSAtRule(nn.Name) || hasForeignAtRule(nn.Children) {
				return true
			}
		case *parser.Rule:
			if hasForeignAtRule(nn.Children) {
				return true
			}
		}
	}

	return false
}

func isCSSAtRule(name []byte) bool {
	stripped := stripVendorPrefix(bytes.TrimSpace(name))

	return cssAtRules[string(bytes.ToLower(stripped))]
}

func stripVendorPrefix(name []byte) []byte {
	if len(name) > 0 && name[0] == '-' {
		if i := bytes.IndexByte(name[1:], '-'); i >= 0 {
			return name[i+2:]
		}
	}

	return name
}
