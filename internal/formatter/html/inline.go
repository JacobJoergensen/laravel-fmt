package html

import (
	"bytes"

	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/html"
)

// The inline-run rules the HTML and Blade emitters share: what a browser paints
// at a boundary does not depend on the template language.

// Gluable reports whether a boundary at node may have to stay on one line,
// since only between inline boxes does a line break render as a space. An
// unknown or custom element counts as inline, as browsers render it.
func Gluable(node any) bool {
	switch n := node.(type) {
	case *parser.Text, *parser.Comment:
		return true
	case *parser.Element:
		return !parser.IsBlockElement(n.Name)
	}

	return false
}

// IsInlineContainer reports whether an element keeps the whitespace at its body
// edges: an inline-level element, or one whose rendering is unknown: a custom
// element, a component, or a Blade tag named wholly by an echo (empty name).
func IsInlineContainer(name []byte) bool {
	return parser.IsInlineLevelElement(name) || bytes.ContainsAny(name, "-:") || len(name) == 0
}

// RunAuthoredOnOneLine reports whether no line break sits beside an element of
// an inline run; breaks at its edges and between words, which the layout
// re-flows, do not count.
func RunAuthoredOnOneLine(children []parser.Node) bool {
	last := len(children) - 1

	for i, c := range children {
		t, ok := c.(*parser.Text)
		if !ok {
			continue
		}

		if len(bytes.Trim(t.Text, CollapsibleSpace)) == 0 {
			if i > 0 && i < last && HasLineBreak(t.Text) {
				return false
			}

			continue
		}

		lead := t.Text[:len(t.Text)-len(bytes.TrimLeft(t.Text, CollapsibleSpace))]
		trail := t.Text[len(bytes.TrimRight(t.Text, CollapsibleSpace)):]

		if i > 0 && HasLineBreak(lead) || i < last && HasLineBreak(trail) {
			return false
		}
	}

	return true
}

// KeepsAuthoredBreak reports whether an inline run keeps its authored lines
// instead of folding onto one: a lone element child on a line of its own, or a
// blank line between two children.
func KeepsAuthoredBreak(children []parser.Node) bool {
	return loneChildOnItsOwnLine(children) || hasBlankLineBetween(children)
}

func loneChildOnItsOwnLine(children []parser.Node) bool {
	lone := -1

	for i, c := range children {
		if t, ok := c.(*parser.Text); ok && len(bytes.Trim(t.Text, CollapsibleSpace)) == 0 {
			continue
		}

		if _, ok := c.(*parser.Element); !ok || lone >= 0 {
			return false
		}

		lone = i
	}

	return lone >= 0 && brokenBefore(children[:lone]) && brokenBefore(children[lone+1:])
}

func brokenBefore(nodes []parser.Node) bool {
	for _, n := range nodes {
		if t, ok := n.(*parser.Text); ok && HasLineBreak(t.Text) {
			return true
		}
	}

	return false
}

// hasBlankLineBetween reports whether a blank line separates two children in a
// way both emitters' structural layouts keep, so blocking the fold is a fixed
// point. A blank at a text child's edge counts only beside an element: next to
// more text, Blade's layout folds it as a gap between words.
func hasBlankLineBetween(children []parser.Node) bool {
	last := len(children) - 1

	for i, c := range children {
		if IsWhitespaceText(c) {
			if i > 0 && i < last && BlankLineInText(c) {
				return true
			}

			continue
		}

		if i > 0 && isElement(children[i-1]) && TextEdgeBlank(c, true) {
			return true
		}

		if i < last && isElement(children[i+1]) && TextEdgeBlank(c, false) {
			return true
		}
	}

	return false
}

func isElement(n parser.Node) bool {
	_, ok := n.(*parser.Element)

	return ok
}

// TextEdgeBlank reports whether node is text whose leading (or trailing)
// whitespace holds a blank line, which the caller must request itself because
// trimming the text drops it.
func TextEdgeBlank(node any, leading bool) bool {
	t, ok := node.(*parser.Text)
	if !ok {
		return false
	}

	edge := t.Text

	if leading {
		edge = edge[:len(edge)-len(bytes.TrimLeft(edge, CollapsibleSpace))]
	} else {
		edge = edge[len(bytes.TrimRight(edge, CollapsibleSpace)):]
	}

	return CountLineBreaks(edge) >= 2
}

// InlineAtom is a whitespace-free word or a whole node in an inline run.
// SpaceBefore is false when the atom is glued to the previous one, so no line
// break may separate them. NewlineBefore records that the author broke the
// line there: only there may a break go, since a space turned into a newline
// changes the rendered HTML that tests and white-space CSS see. BlankBefore
// records a blank line there, which stays as the block layouts keep it.
type InlineAtom struct {
	Text          []byte
	Node          parser.Node
	SpaceBefore   bool
	NewlineBefore bool
	BlankBefore   bool
}

// InlineAtoms splits an inline run into atoms and reports whether it has
// leading and trailing whitespace. A whitespace run becomes one breakable
// boundary, as a browser collapses it.
func InlineAtoms(children []parser.Node) (atoms []InlineAtom, lead, trail bool) {
	pending, pendingBreaks := false, 0
	first := true

	add := func(atom InlineAtom) {
		atom.SpaceBefore, atom.NewlineBefore, atom.BlankBefore = pending, pendingBreaks > 0, pendingBreaks > 1
		if first {
			lead = pending
			first = false
		}

		pending, pendingBreaks = false, 0
		atoms = append(atoms, atom)
	}

	for _, c := range children {
		text, ok := c.(*parser.Text)
		if !ok {
			add(InlineAtom{Node: c})

			continue
		}

		i := 0
		for i < len(text.Text) {
			if IsCollapsibleSpace(text.Text[i]) {
				start := i
				for i < len(text.Text) && IsCollapsibleSpace(text.Text[i]) {
					i++
				}

				pending = true
				pendingBreaks += CountLineBreaks(text.Text[start:i])

				continue
			}

			start := i
			for i < len(text.Text) && !IsCollapsibleSpace(text.Text[i]) {
				i++
			}

			add(InlineAtom{Text: text.Text[start:i]})
		}
	}

	return atoms, lead, pending
}

// InlineRunFullyLoose reports whether an inline run carries whitespace at both
// edges and at every interior boundary, so laying it out on its own lines
// replaces only whitespace the source already had. A single glued boundary
// (`H<sub>2</sub>O`) fails the test, since a newline there renders as a space.
func InlineRunFullyLoose(atoms []InlineAtom, lead, trail bool) bool {
	if !lead || !trail {
		return false
	}

	for _, a := range atoms[1:] {
		if !a.SpaceBefore {
			return false
		}
	}

	return true
}

// InlineRunHasGluedInterior reports whether any boundary between two atoms
// carries no whitespace, so a break there would render as a space.
func InlineRunHasGluedInterior(atoms []InlineAtom) bool {
	for _, a := range atoms[1:] {
		if !a.SpaceBefore {
			return true
		}
	}

	return false
}

// CloseTagWidth is the width of el's close tag, which an inline body shares a
// line with and so has to reserve.
func CloseTagWidth(el *parser.Element) int {
	name := el.CloseName
	if len(name) == 0 {
		name = el.Name
	}

	return len("</>") + len(name)
}

// CollapsibleSpace is the whitespace a browser collapses between words and
// drops at a block's edge. Measured in Chrome, a form feed, vertical tab, NEL
// or no-break space paints a visible gap instead, so those are content.
const CollapsibleSpace = " \t\n\r"

// IsCollapsibleSpace reports whether b is in CollapsibleSpace.
func IsCollapsibleSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
