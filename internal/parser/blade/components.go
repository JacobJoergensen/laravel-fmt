package blade

import (
	"bytes"
	"slices"
)

var rawBodyComponents = map[string]struct{}{
	"x-mail::message": {},
	"x-mail::button":  {},
	"x-mail::panel":   {},
	"x-mail::subcopy": {},
	"x-mail::table":   {},
	"x-mail::header":  {},
	"x-mail::footer":  {},
	"x-mail::layout":  {},
}

// IsRawBodyComponent reports whether name's body must be preserved verbatim:
// the Laravel mail markdown components, whose body is markdown, not HTML, and
// would be wrecked by HTML re-flow.
func IsRawBodyComponent(name []byte) bool {
	_, ok := rawBodyComponents[string(name)]

	return ok
}

// componentTagPrefixes are the tags a Blade tag compiler turns into a component
// call: illuminate/view's ComponentTagCompiler matches `x-` and `x:`, and
// Livewire's and Flux's tag compilers extend it for `livewire:` and `flux:`.
// All three match the prefix case-sensitively.
var componentTagPrefixes = [][]byte{[]byte("x-"), []byte("x:"), []byte("livewire:"), []byte("flux:")}

// IsComponentTag reports whether name is a component tag, whose attribute
// values compile into PHP: a plain value into a quoted string, a bound one
// into an expression.
func IsComponentTag(name []byte) bool {
	return slices.ContainsFunc(componentTagPrefixes, func(prefix []byte) bool {
		return bytes.HasPrefix(name, prefix)
	})
}

// componentFamily names the tags whose close tags a Blade tag compiler cannot
// tell apart: it matches `</x-…>`, `</flux:…>` and `</livewire:…>` by prefix
// alone, so any close tag of a family ends the innermost open one. Slots
// compile on their own pass. A tag outside every family returns "".
func componentFamily(name []byte) string {
	switch {
	case isSlotName(name, "x-slot"), isSlotName(name, "x:slot"):
		return "x-slot"
	case bytes.HasPrefix(name, []byte("x-")), bytes.HasPrefix(name, []byte("x:")):
		return "x"
	case isSlotName(name, "livewire:slot"):
		return "livewire-slot"
	case bytes.HasPrefix(name, []byte("livewire:")):
		return "livewire"
	case bytes.HasPrefix(name, []byte("flux:")):
		return "flux"
	}

	return ""
}

func isSlotName(name []byte, slot string) bool {
	rest, ok := bytes.CutPrefix(name, []byte(slot))

	return ok && (len(rest) == 0 || rest[0] == ':')
}

// trimmedSlotPrefixes are the component tags that render through Laravel's
// ManagesComponents, which trims every slot it captures. Livewire's
// `livewire:` tags render elsewhere and are not trimmed.
var trimmedSlotPrefixes = [][]byte{[]byte("x-"), []byte("x:"), []byte("flux:")}

// TrimsSlot reports whether whitespace at the edges of name's body never renders.
func TrimsSlot(name []byte) bool {
	return slices.ContainsFunc(trimmedSlotPrefixes, func(prefix []byte) bool {
		return bytes.HasPrefix(name, prefix)
	})
}
