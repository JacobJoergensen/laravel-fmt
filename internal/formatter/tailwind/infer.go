package tailwind

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// dataTypes is the set of Tailwind data types an arbitrary value matches, one
// bit per DataType in utils/infer-data-type.ts. A utility picks its branch from
// this set alone, which is what lets one ranked sample answer for every value
// sharing its set.
type dataTypes uint32

const (
	typeColor dataTypes = 1 << iota
	typeLength
	typePercentage
	typeRatio
	typeNumber
	typeInteger
	typeURL
	typePosition
	typeBgSize
	typeLineWidth
	typeImage
	typeFamilyName
	typeGenericName
	typeAbsoluteSize
	typeRelativeSize
	typeAngle
	typeVector
)

var probeLookup = sync.OnceValue(func() map[dataTypes]int {
	lookup := make(map[dataTypes]int, len(arbitrarySamples))

	for i, sample := range arbitrarySamples {
		set := inferDataTypes(decodeArbitrary(sample))

		if _, seen := lookup[set]; !seen {
			lookup[set] = i
		}
	}

	return lookup
})

// arbitraryProbeIndex returns the stemEntry.ProbeOrders index answering for an
// arbitrary `[…]` or `(…)` value, in the layout the generator ranks: the
// samples, each bracketed hint, `(--var)`, then each hint in parentheses.
// found is false for a value no sample shares a set with; invalid marks one
// candidate.ts rejects.
func arbitraryProbeIndex(value string) (index int, found, invalid bool) {
	parenStart := len(arbitrarySamples) + len(arbitraryHintTypes)
	inner := value[1 : len(value)-1]

	if value[0] == '(' {
		parts := segmentBracketAware(inner, ':')

		switch {
		case len(parts) == 2 && strings.HasPrefix(parts[1], "--"):
			return parenStart + 1 + hintIndex(parts[0]), true, false
		case len(parts) != 2 && strings.HasPrefix(inner, "--"):
			return parenStart, true, false
		default:
			return 0, false, true
		}
	}

	hint, rest, hinted := typeHint(inner)

	switch {
	case strings.TrimSpace(rest) == "", hinted && hint == "":
		return 0, false, true
	case hinted:
		return len(arbitrarySamples) + hintIndex(hint), true, false
	}

	index, found = probeLookup()[inferDataTypes(decodeArbitrary(rest))]

	return index, found, false
}

// typeHint splits a leading `type:` off an arbitrary value: lower-case letters
// and dashes up to the first colon, as candidate.ts reads it.
func typeHint(value string) (hint, rest string, hinted bool) {
	for i := range len(value) {
		c := value[i]

		switch {
		case c == ':':
			return value[:i], value[i+1:], true
		case c == '-' || (c >= 'a' && c <= 'z'):
		default:
			return "", value, false
		}
	}

	return "", value, false
}

// A hint Tailwind has no DataType for takes the last, unknown-type probe.
func hintIndex(hint string) int {
	if i := slices.Index(arbitraryHintTypes[:], hint); i >= 0 {
		return i
	}

	return len(arbitraryHintTypes) - 1
}

// decodeArbitrary turns `_` into a space and `\_` into `_`, the part of
// decodeArbitraryValue that changes which data types a value matches.
func decodeArbitrary(value string) string {
	if !strings.Contains(value, "_") {
		return value
	}

	var b strings.Builder

	b.Grow(len(value))

	for i := 0; i < len(value); i++ {
		switch {
		case value[i] == '\\' && i+1 < len(value) && value[i+1] == '_':
			b.WriteByte('_')
			i++
		case value[i] == '_':
			b.WriteByte(' ')
		default:
			b.WriteByte(value[i])
		}
	}

	return b.String()
}

// inferDataTypes runs every check of Tailwind's inferDataType on a decoded
// value. inferDataType gives up on a leading `var(`, so that matches nothing.
func inferDataTypes(value string) dataTypes {
	if strings.HasPrefix(value, "var(") {
		return 0
	}

	var set dataTypes

	add := func(t dataTypes, matches bool) {
		if matches {
			set |= t
		}
	}

	add(typeColor, isColor(value))
	add(typeLength, isLength(value))
	add(typePercentage, isPercentage(value))
	add(typeRatio, isFraction(value))
	add(typeNumber, isNumber(value))
	add(typeInteger, isPositiveInteger(value))
	add(typeURL, isURL(value))
	add(typePosition, isBackgroundPosition(value))
	add(typeBgSize, isBackgroundSize(value))
	add(typeLineWidth, isLineWidth(value))
	add(typeImage, isImage(value))
	add(typeFamilyName, isFamilyName(value))
	add(typeGenericName, slices.Contains(genericNames, value))
	add(typeAbsoluteSize, slices.Contains(absoluteSizes, value))
	add(typeRelativeSize, value == "larger" || value == "smaller")
	add(typeAngle, hasNumberWithUnit(value, angleUnits))
	add(typeVector, isVector(value))

	return set
}

func isColor(value string) bool {
	if strings.HasPrefix(value, "#") || hasAnyPrefixFold(value, colorFunctions) {
		return true
	}

	_, found := slices.BinarySearch(namedColors, strings.ToLower(value))

	return found
}

func isLength(value string) bool {
	return hasNumberWithUnit(value, lengthUnits) || hasAnyPrefixFold(value, spacingFunction) || hasMathFunction(value)
}

func isPercentage(value string) bool {
	return hasNumberWithUnit(value, percentUnit) || hasMathFunction(value)
}

func isNumber(value string) bool {
	end := numberEnd(value)

	return (end > 0 && end == len(value)) || hasMathFunction(value)
}

// isFraction is IS_FRACTION, `number \s* / \s* number`, or a math function.
func isFraction(value string) bool {
	if hasMathFunction(value) {
		return true
	}

	numerator := numberEnd(value)
	if numerator == 0 {
		return false
	}

	rest, ok := strings.CutPrefix(strings.TrimLeft(value[numerator:], regexpWhitespace), "/")
	if !ok {
		return false
	}

	rest = strings.TrimLeft(rest, regexpWhitespace)
	denominator := numberEnd(rest)

	return denominator > 0 && denominator == len(rest)
}

// isPositiveInteger is `String(Number(v)) === v` for a whole v >= 0: only the
// canonical spelling counts, so `02`, `2.0` and `-0` are not integers.
func isPositiveInteger(value string) bool {
	if value == "" || value[0] == '-' || value[0] == '+' {
		return false
	}

	n, err := strconv.ParseFloat(value, 64)
	if err != nil || n >= 1e21 || n != math.Trunc(n) {
		return false
	}

	return strconv.FormatFloat(n, 'f', -1, 64) == value
}

func isURL(value string) bool {
	return strings.HasPrefix(value, "url(") && strings.HasSuffix(value, ")")
}

func isBackgroundPosition(value string) bool {
	count := 0

	for part := range segments(value, ' ') {
		switch {
		case slices.Contains(positionKeywords, part):
			count++
		case strings.HasPrefix(part, "var("):
		case isLength(part) || isPercentage(part):
			count++
		default:
			return false
		}
	}

	return count > 0
}

func isBackgroundSize(value string) bool {
	count := 0

	for size := range segments(value, ',') {
		if size == "cover" || size == "contain" {
			count++

			continue
		}

		values, sized := 0, true

		for v := range segments(size, ' ') {
			values++
			sized = sized && (v == "auto" || isLength(v) || isPercentage(v))
		}

		if values > 2 {
			return false
		}

		if sized {
			count++
		}
	}

	return count > 0
}

func isLineWidth(value string) bool {
	for part := range segments(value, ' ') {
		if !isLength(part) && !isNumber(part) && !slices.Contains(lineWidthKeywords, part) {
			return false
		}
	}

	return true
}

func isImage(value string) bool {
	count := 0

	for part := range segments(value, ',') {
		switch {
		case strings.HasPrefix(part, "var("):
		case isURL(part), hasAnyPrefixFold(part, imageFunctions):
			count++
		default:
			return false
		}
	}

	return count > 0
}

func isFamilyName(value string) bool {
	count := 0

	for part := range segments(value, ',') {
		if part != "" && part[0] >= '0' && part[0] <= '9' {
			return false
		}

		if !strings.HasPrefix(part, "var(") {
			count++
		}
	}

	return count > 0
}

// isVector is IS_VECTOR: three numbers apart by spaces.
func isVector(value string) bool {
	for range 2 {
		end := numberEnd(value)
		if end == 0 || end == len(value) || value[end] != ' ' {
			return false
		}

		value = strings.TrimLeft(value[end:], " ")
	}

	end := numberEnd(value)

	return end > 0 && end == len(value)
}

func hasNumberWithUnit(value string, units []string) bool {
	end := numberEnd(value)

	return end > 0 && slices.Contains(units, value[end:])
}

func hasAnyPrefixFold(value string, prefixes []string) bool {
	return slices.ContainsFunc(prefixes, func(prefix string) bool {
		return len(value) >= len(prefix) && strings.EqualFold(value[:len(prefix)], prefix)
	})
}

// numberEnd returns where Tailwind's HAS_NUMBER, `[+-]?\d*\.?\d+([eE][+-]?\d+)?`,
// ends at the start of s, or 0 without one. An exponent counts only with digits
// after it, so `2em` keeps its unit.
func numberEnd(s string) int {
	start := 0
	if start < len(s) && (s[start] == '+' || s[start] == '-') {
		start++
	}

	end := digitsEnd(s, start)

	if end < len(s) && s[end] == '.' {
		if fraction := digitsEnd(s, end+1); fraction > end+1 {
			end = fraction
		}
	}

	if end == start {
		return 0
	}

	if end < len(s) && (s[end] == 'e' || s[end] == 'E') {
		exponent := end + 1
		if exponent < len(s) && (s[exponent] == '+' || s[exponent] == '-') {
			exponent++
		}

		if digits := digitsEnd(s, exponent); digits > exponent {
			end = digits
		}
	}

	return end
}

func digitsEnd(s string, i int) int {
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}

	return i
}

func hasMathFunction(s string) bool {
	if !strings.Contains(s, "(") {
		return false
	}

	return slices.ContainsFunc(mathFunctions, func(fn string) bool {
		return strings.Contains(s, fn)
	})
}

// The \s a JavaScript regular expression matches, as far as a class holds.
const regexpWhitespace = " \t\n\r\f\v"

var (
	percentUnit     = []string{"%"}
	spacingFunction = []string{"--spacing("}
)

// MATH_FUNCTIONS in utils/math-operators.ts, each with its opening paren.
var mathFunctions = []string{
	"calc(", "min(", "max(", "clamp(", "mod(", "rem(",
	"sin(", "cos(", "tan(", "asin(", "acos(", "atan(", "atan2(",
	"pow(", "sqrt(", "hypot(", "log(", "exp(", "round(",
}

// LENGTH_UNITS in utils/infer-data-type.ts.
var lengthUnits = []string{
	"cm", "mm", "Q", "in", "pc", "pt", "px",
	"em", "ex", "ch", "cap", "ic", "rem", "rex", "rch", "rcap", "ric",
	"lh", "rlh",
	"vw", "vh", "vmin", "vmax", "vb", "vi",
	"svw", "svh", "svi", "svb", "svmin", "svmax",
	"lvw", "lvh", "lvi", "lvb", "lvmin", "lvmax",
	"dvw", "dvh", "dvi", "dvb", "dvmin", "dvmax",
	"cqw", "cqh", "cqi", "cqb", "cqmin", "cqmax",
}

var angleUnits = []string{"deg", "rad", "grad", "turn"}

// medium is both a line width and an absolute size.
const medium = "medium"

var lineWidthKeywords = []string{"thin", medium, "thick"}

var positionKeywords = []string{"center", "top", "right", "bottom", "left"}

var genericNames = []string{
	"serif", "sans-serif", "monospace", "cursive", "fantasy", "system-ui",
	"ui-serif", "ui-sans-serif", "ui-monospace", "ui-rounded", "math", "emoji",
	"fangsong",
}

var absoluteSizes = []string{
	"xx-small", "x-small", "small", medium, "large", "x-large", "xx-large",
	"xxx-large",
}

// IS_COLOR_FN in utils/is-color.ts.
var colorFunctions = []string{
	"rgb(", "rgba(", "hsl(", "hsla(", "hwb(", "color(", "lab(", "lch(",
	"oklab(", "oklch(", "light-dark(", "color-mix(", "--alpha(",
}

// IS_IMAGE_FN and IS_GRADIENT_FN in utils/infer-data-type.ts.
var imageFunctions = []string{
	"element(", "image(", "cross-fade(", "image-set(",
	"linear-gradient(", "radial-gradient(", "conic-gradient(",
	"repeating-linear-gradient(", "repeating-radial-gradient(",
	"repeating-conic-gradient(",
}

// NAMED_COLORS in utils/is-color.ts, sorted for a binary search.
var namedColors = []string{
	"accentcolor", "accentcolortext", "activetext", "aliceblue",
	"antiquewhite", "aqua", "aquamarine", "azure", "beige", "bisque", "black",
	"blanchedalmond", "blue", "blueviolet", "brown", "burlywood",
	"buttonborder", "buttonface", "buttontext", "cadetblue", "canvas",
	"canvastext", "chartreuse", "chocolate", "coral", "cornflowerblue",
	"cornsilk", "crimson", "currentcolor", "cyan", "darkblue", "darkcyan",
	"darkgoldenrod", "darkgray", "darkgreen", "darkgrey", "darkkhaki",
	"darkmagenta", "darkolivegreen", "darkorange", "darkorchid", "darkred",
	"darksalmon", "darkseagreen", "darkslateblue", "darkslategray",
	"darkslategrey", "darkturquoise", "darkviolet", "deeppink", "deepskyblue",
	"dimgray", "dimgrey", "dodgerblue", "field", "fieldtext", "firebrick",
	"floralwhite", "forestgreen", "fuchsia", "gainsboro", "ghostwhite", "gold",
	"goldenrod", "gray", "graytext", "green", "greenyellow", "grey",
	"highlight", "highlighttext", "honeydew", "hotpink", "indianred", "indigo",
	"ivory", "khaki", "lavender", "lavenderblush", "lawngreen", "lemonchiffon",
	"lightblue", "lightcoral", "lightcyan", "lightgoldenrodyellow",
	"lightgray", "lightgreen", "lightgrey", "lightpink", "lightsalmon",
	"lightseagreen", "lightskyblue", "lightslategray", "lightslategrey",
	"lightsteelblue", "lightyellow", "lime", "limegreen", "linen", "linktext",
	"magenta", "mark", "marktext", "maroon", "mediumaquamarine", "mediumblue",
	"mediumorchid", "mediumpurple", "mediumseagreen", "mediumslateblue",
	"mediumspringgreen", "mediumturquoise", "mediumvioletred", "midnightblue",
	"mintcream", "mistyrose", "moccasin", "navajowhite", "navy", "oldlace",
	"olive", "olivedrab", "orange", "orangered", "orchid", "palegoldenrod",
	"palegreen", "paleturquoise", "palevioletred", "papayawhip", "peachpuff",
	"peru", "pink", "plum", "powderblue", "purple", "rebeccapurple", "red",
	"rosybrown", "royalblue", "saddlebrown", "salmon", "sandybrown",
	"seagreen", "seashell", "selecteditem", "selecteditemtext", "sienna",
	"silver", "skyblue", "slateblue", "slategray", "slategrey", "snow",
	"springgreen", "steelblue", "tan", "teal", "thistle", "tomato",
	"transparent", "turquoise", "violet", "visitedtext", "wheat", "white",
	"whitesmoke", "yellow", "yellowgreen",
}
