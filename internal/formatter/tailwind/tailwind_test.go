package tailwind

import (
	"sync"
	"testing"
)

func TestSortCanonical(t *testing.T) {
	got := Sort(nil, "text-white bg-blue-500 p-4")
	want := "bg-blue-500 p-4 text-white"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Expected values are Tailwind's getClassOrder ranks, where `container` sorts
// before margins and `space-y` after sizing, not at the properties they set.
func TestSortLayoutSpacingOrder(t *testing.T) {
	cases := map[string]string{
		"mx-auto container max-w-7xl": "container mx-auto max-w-7xl",
		"space-y-3 mt-4":              "mt-4 space-y-3",
		"mx-auto space-y-4 max-w-6xl": "mx-auto max-w-6xl space-y-4",
	}

	for in, want := range cases {
		if got := Sort(nil, in); got != want {
			t.Errorf("Sort(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSortCanonicalBeatsAlphabetical(t *testing.T) {
	got := Sort(nil, "border rounded-full")
	want := "rounded-full border"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSortVariantsAfterBase(t *testing.T) {
	got := Sort(nil, "hover:bg-red-500 bg-blue-500")
	want := "bg-blue-500 hover:bg-red-500"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSortBreakpointsByWidth(t *testing.T) {
	got := Sort(nil, "lg:p-8 sm:p-2 md:p-4")
	want := "sm:p-2 md:p-4 lg:p-8"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSortUnknownClassFirst(t *testing.T) {
	got := Sort(nil, "bg-blue-500 custom-foo")
	want := "custom-foo bg-blue-500"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Every want is what Tailwind v4.3.5's getClassOrder gives.
func TestSortBareStemIsKnownOnlyWhenItCompiles(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"a stem that needs a value is unknown", "z-10 row", "row z-10"},
		{"a color or size stem alone is unknown", "p-4 text", "text p-4"},
		{"a stem with a default value is known", "rounded custom-foo", "custom-foo rounded"},
		{"a side stem with a default value is known", "rounded-t custom-foo", "custom-foo rounded-t"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Sort(nil, tc.in); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Every want is what Tailwind v4.3.5's getClassOrder gives. A theme is loaded,
// since without one any value under a stem counts as known.
func TestSortRanksUtilitiesKeptForOldMarkup(t *testing.T) {
	theme := mustTheme(t, nil, nil, nil, nil)

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"a static one", "break-words flex custom-foo", "custom-foo flex break-words"},
		{"a static one with a direction", "bg-gradient-to-r p-4 custom-foo", "custom-foo bg-gradient-to-r p-4"},
		{"a functional one with a breakpoint", "max-w-screen-lg w-full custom-foo", "custom-foo w-full max-w-screen-lg"},
		{"a functional one with none", "filter-none blur custom-foo", "custom-foo blur filter-none"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Sort(theme, tc.in); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Every want is what Tailwind v4.3.5's getClassOrder gives.
func TestSortReadsTheLeadingImportantMark(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"before the utility", "!p-3 z-10 custom-foo", "custom-foo z-10 !p-3"},
		{"after a variant", "hover:!opacity-100 opacity-50 custom-foo", "custom-foo opacity-50 hover:!opacity-100"},
		{"before a negative utility", "!-mt-2 z-10 custom-foo", "custom-foo z-10 !-mt-2"},
		{"on both ends is no class", "z-10 !p-3!", "!p-3! z-10"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Sort(nil, tc.in); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Every want is what Tailwind v4.3.5's getClassOrder gives with these tokens.
func TestSortPlacesAnUnlistedValueAmongItsSiblingsByName(t *testing.T) {
	theme := mustTheme(t, []ThemeToken{
		{Namespace: "color", Name: "brand"},
		{Namespace: "color", Name: "accent"},
		{Namespace: "spacing", Name: "gutter"},
	}, nil, nil, nil)

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"a number between listed ones", "rotate-12 rotate-7 rotate-1", "rotate-1 rotate-7 rotate-12"},
		{"a number past the listed ones", "z-24 z-10", "z-10 z-24"},
		{"a value of a stem that is also a static utility", "flex-24 max-w-25 flex", "flex max-w-25 flex-24"},
		{"a theme color among the palette", "bg-zinc-500 bg-brand bg-amber-500", "bg-amber-500 bg-brand bg-zinc-500"},
		{"a theme color before the whole palette", "bg-amber-500 bg-accent", "bg-accent bg-amber-500"},
		{"a theme spacing among the scale", "p-gutter p-px p-4", "p-4 p-gutter p-px"},
		{"a sibling that is a stem of its own", "inset-gutter inset-full inset-auto", "inset-auto inset-full inset-gutter"},
		{"a number on a stem that also takes colors", "border-13 border-none border-4", "border-4 border-13 border-none"},
		{"a color set in a custom property", "drop-shadow-brand drop-shadow-none drop-shadow-lg", "drop-shadow-lg drop-shadow-none drop-shadow-brand"},
		{"a bare stem before its arbitrary values", "rounded-(--x) rounded", "rounded rounded-(--x)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Sort(theme, tc.in); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSortPreservesZeroOrSingleToken(t *testing.T) {
	cases := []string{"", "   ", "only-one"}

	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			got := Sort(nil, input)

			if got != input {
				t.Fatalf("Sort(%q)=%q, want unchanged", input, got)
			}
		})
	}
}

func TestSortImportantSuffixStaysLast(t *testing.T) {
	got := Sort(nil, "text-white bg-blue-500 !important")
	want := "bg-blue-500 text-white !important"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSortImportantSuffixModifierDoesNotSkew(t *testing.T) {
	got := Sort(nil, "text-white bg-blue-500! p-4")
	want := "bg-blue-500! p-4 text-white"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSortAmbiguousPrefixNamedValues(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"border width before border color", "border-red-500 border-2", "border-2 border-red-500"},
		{"shadow size keyword routes to box-shadow", "shadow-red-500 shadow-md", "shadow-md shadow-red-500"},
		{"ring size routes to ring-shadow", "ring-red-500 ring-2", "ring-2 ring-red-500"},
		{"decoration color before decoration thickness", "decoration-2 decoration-red-500", "decoration-red-500 decoration-2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Sort(nil, tc.in)

			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Every want is what Tailwind v4.3.4's getClassOrder gives.
func TestSortArbitraryValueShapeDisambiguation(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"border length vs color", "p-4 border-[2px] border-[#fff]", "border-[2px] border-[#fff] p-4"},
		{"text length vs color", "text-[#fff] p-4 text-[14px]", "p-4 text-[14px] text-[#fff]"},
		{"shadow length vs color", "shadow-[#fff] p-4 shadow-[10px]", "p-4 shadow-[10px] shadow-[#fff]"},
		{"unit added in v4.3.4 is a length", "text-[#fff] p-4 text-[10dvmin]", "p-4 text-[10dvmin] text-[#fff]"},
		{"math function is a length", "text-[#fff] p-4 text-[clamp(1rem,2vw,3rem)]", "p-4 text-[clamp(1rem,2vw,3rem)] text-[#fff]"},
		{"spacing function is a length", "border-[#fff] p-4 border-[--spacing(2)]", "border-[--spacing(2)] border-[#fff] p-4"},
		{"exponent number is a length", "text-[#fff] p-4 text-[1e1px]", "p-4 text-[1e1px] text-[#fff]"},
		{"text takes a time as a color", "text-[#fff] p-4 text-[2s]", "p-4 text-[#fff] text-[2s]"},
		{"shadow takes a time as a shadow", "shadow-[#fff] p-4 shadow-[2s]", "p-4 shadow-[2s] shadow-[#fff]"},
		{"shadow takes a shadow list as a shadow", "shadow-[#fff] p-4 shadow-[0_0_2px_#000]", "p-4 shadow-[0_0_2px_#000] shadow-[#fff]"},
		{"border takes a bare number as a width", "border-[#fff] p-4 border-[2]", "border-[2] border-[#fff] p-4"},
		{"border takes a percentage as a color", "border-[#fff] p-4 border-[50%]", "border-[#fff] border-[50%] p-4"},
		{"border takes a width keyword as a width", "border-[#fff] p-4 border-[thin]", "border-[thin] border-[#fff] p-4"},
		{"text takes a relative size as a size", "text-[#fff] p-4 text-[larger]", "p-4 text-[larger] text-[#fff]"},
		{"a type hint decides the branch", "text-[#fff] p-4 text-[length:var(--x)]", "p-4 text-[length:var(--x)] text-[#fff]"},
		{"a type hint in parentheses decides the branch", "border-[#fff] p-4 border-(length:--x)", "border-(length:--x) border-[#fff] p-4"},
		{"bg takes a url as an image", "bg-cover bg-[url(x)] bg-red-500", "bg-red-500 bg-[url(x)] bg-cover"},
		{"font takes a number as a weight", "font-bold font-[500] font-[Inter]", "font-[Inter] font-[500] font-bold"},
		{"a value the utility takes sorts by its rank", "inset-0 -bg-linear-[45deg]", "inset-0 -bg-linear-[45deg]"},
		{"a value the utility rejects is unknown", "inset-0 -bg-linear-[#000]", "-bg-linear-[#000] inset-0"},
		{"an empty type hint is unknown", "inset-0 text-[:x]", "text-[:x] inset-0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Sort(nil, tc.in)

			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Classes setting the same properties sort by name, so `[` after a digit puts
// `p-[3px]` after `p-4`, and `(` before one puts `w-(--x)` ahead of `w-0`.
func TestSortPlacesArbitraryValuesAmongNamedOnes(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"p-4 p-[3px] p-1", "p-1 p-4 p-[3px]"},
		{"w-0 w-(--x) w-[10px]", "w-(--x) w-0 w-[10px]"},
	}

	for _, tc := range cases {
		if got := Sort(nil, tc.in); got != tc.want {
			t.Errorf("Sort(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSortRanksNegativeAndFractionClassesAsTheirOwn(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"translate-x-2/3 translate-x-2.5", "translate-x-2.5 translate-x-2/3"},
		{"mt-2 -mt-[3px] mt-[3px]", "-mt-[3px] mt-2 mt-[3px]"},
	}

	for _, tc := range cases {
		if got := Sort(nil, tc.in); got != tc.want {
			t.Errorf("Sort(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Every want is what Tailwind v4.3.4's getClassOrder gives.
func TestSortOrdersVariantsAsTheEngineDoes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"min widths join the named breakpoints", "min-[50rem]:flex sm:flex min-[30rem]:flex", "min-[30rem]:flex sm:flex min-[50rem]:flex"},
		{"decimal widths compare as numbers", "min-[40.5rem]:flex min-[40.25rem]:flex", "min-[40.25rem]:flex min-[40.5rem]:flex"},
		{"widths group by unit first", "min-[30rem]:flex min-[500px]:flex", "min-[500px]:flex min-[30rem]:flex"},
		{"max widths sort widest first", "max-[30rem]:flex max-sm:flex", "max-sm:flex max-[30rem]:flex"},
		{"container widths join the named sizes", "@min-[40rem]:flex @min-[9rem]:flex @md:flex", "@min-[9rem]:flex @md:flex @min-[40rem]:flex"},
		{"container max widths sort widest first", "@max-sm:flex @max-[50rem]:flex", "@max-[50rem]:flex @max-sm:flex"},
		{"a variant value decides before the utility", "data-[b]:p-4 data-[a]:m-4", "data-[a]:m-4 data-[b]:p-4"},
		{"named values precede arbitrary ones", "aria-[busy=true]:flex aria-checked:flex", "aria-checked:flex aria-[busy=true]:flex"},
		{"a compound variant compares its inner one", "group-focus:flex group-hover:flex", "group-hover:flex group-focus:flex"},
		{"a compound modifier sorts after none", "group-hover/item:flex group-hover:flex", "group-hover:flex group-hover/item:flex"},
		{"numbers in a class compare by value", "w-[10px] w-[9px]", "w-[9px] w-[10px]"},
		{"a static variant with a value is unknown", "p-4 hover-foo:text-sm", "hover-foo:text-sm p-4"},
		{"a compound variant without one is unknown", "p-4 group:text-sm", "group:text-sm p-4"},
		{"a compound variant cannot wrap an at-rule", "p-4 group-md:text-sm", "group-md:text-sm p-4"},
		{"a compound variant cannot wrap a pseudo-element", "p-4 group-[&::after]:text-sm", "group-[&::after]:text-sm p-4"},
		{"not wraps an at-rule", "p-4 not-md:text-sm", "p-4 not-md:text-sm"},
		{"group cannot wrap a relative selector", "p-4 group-[>img]:text-sm", "group-[>img]:text-sm p-4"},
		{"has wraps a relative selector", "p-4 has-[>img]:text-sm", "p-4 has-[>img]:text-sm"},
		{"a breakpoint with no width is unknown", "p-4 max-foo:text-sm", "max-foo:text-sm p-4"},
		{"a container modifier keeps the width", "p-4 @lg/main:text-sm", "p-4 @lg/main:text-sm"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Sort(nil, tc.in); got != tc.want {
				t.Fatalf("Sort(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSortContainerQueryVariants(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"bare container query sorts by width", "@xl:p-4 @md:p-4", "@md:p-4 @xl:p-4"},
		{"width beats alphabetical order", "@2xl:p-4 @sm:p-4", "@sm:p-4 @2xl:p-4"},
		{"width orders regardless of utility", "@xl:m-2 @sm:p-4", "@sm:p-4 @xl:m-2"},
		{"named container query sorts by container width", "@xl/main:p-4 @md/sidebar:p-4", "@md/sidebar:p-4 @xl/main:p-4"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Sort(nil, tc.in)

			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSortCustomThemeTokensUseStemFallback(t *testing.T) {
	got := Sort(nil, "text-dark-text bg-dark-bg inline-flex border-dark-border")
	want := "inline-flex border-dark-border bg-dark-bg text-dark-text"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSortValueMissingFromProjectThemeIsUnknown(t *testing.T) {
	theme := mustTheme(t, []ThemeToken{
		{Namespace: "color", Name: "surface-raised"},
		{Namespace: "color", Name: "text-muted"},
		{Namespace: "radius", Name: "card"},
	}, nil, nil, nil)

	tests := []struct {
		name  string
		theme *Theme
		in    string
		want  string
	}{
		{
			name:  "undefined names float first in authored order",
			theme: theme,
			in:    "size-5 text-coral-500 p-2 bg-surface-muted hover:border-coral-300",
			want:  "text-coral-500 bg-surface-muted hover:border-coral-300 size-5 p-2",
		},
		{
			name:  "project tokens in any namespace sort as utilities",
			theme: theme,
			in:    "text-text-muted rounded-card bg-surface-raised p-2",
			want:  "rounded-card bg-surface-raised p-2 text-text-muted",
		},
		{
			name:  "bare numbers and var shorthands sort as utilities",
			theme: theme,
			in:    "opacity-37 p-13 bg-(--brand) grid-cols-13",
			want:  "grid-cols-13 bg-(--brand) p-13 opacity-37",
		},
		{
			name:  "without a project theme every value is taken to exist",
			theme: nil,
			in:    "size-5 text-coral-500 p-2",
			want:  "size-5 p-2 text-coral-500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Sort(tt.theme, tt.in); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// These utilities take only arbitrary values, so no listed class reaches their
// stem; the generator probes one so they sort at their property.
func TestSortArbitraryOnlyUtilitiesSortAtTheirProperty(t *testing.T) {
	tests := []struct{ in, want string }{
		{"after:content-['x'] font-medium", "font-medium after:content-['x']"},
		{"content-[var(--x)] font-medium block", "block font-medium content-[var(--x)]"},
		{"will-change-[top] cursor-[grab] p-1", "cursor-[grab] p-1 will-change-[top]"},
	}

	for _, tt := range tests {
		if got := Sort(nil, tt.in); got != tt.want {
			t.Errorf("Sort(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSortArbitraryPropertySortsByDeclaredProperty(t *testing.T) {
	got := Sort(nil, "[text-shadow:0_0_red] bg-blue-500 [color:red] [-webkit-text-fill-color:transparent]")
	want := "bg-blue-500 [color:red] [-webkit-text-fill-color:transparent] [text-shadow:0_0_red]"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSortStableOnDuplicates(t *testing.T) {
	// The repeated tokens are the test subject, so dupword is suppressed.
	got := Sort(nil, "p-4 p-4 m-2 m-2") //nolint:dupword
	want := "m-2 m-2 p-4 p-4"           //nolint:dupword

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSortIsIdempotent(t *testing.T) {
	inputs := []string{
		"text-white bg-blue-500 p-4",
		"hover:bg-red-500 bg-blue-500 md:p-8 p-4",
		"custom-foo bg-blue-500 p-4",
		"@md:p-4 @xl:p-4 p-4",
		"text-[14px] text-[#fff] text-base text-red-500",
		"shadow-[10px] shadow-[#fff] shadow-md shadow-red-500",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			once := Sort(nil, input)
			twice := Sort(nil, once)

			if once != twice {
				t.Fatalf("not idempotent: once=%q twice=%q", once, twice)
			}
		})
	}
}

// No class list in the other tests holds 64 distinct variants, so a swapped
// word order would pass them.
func TestVariantMaskCmp(t *testing.T) {
	tests := []struct {
		name string
		a, b variantMask
		want int
	}{
		{"equal zero", variantMask{0, 0}, variantMask{0, 0}, 0},
		{"low bit beats none", variantMask{1, 0}, variantMask{0, 0}, 1},
		{"high word outranks full low word", variantMask{^uint64(0), 0}, variantMask{0, 1}, -1},
		{"larger low word", variantMask{2, 0}, variantMask{1, 0}, 1},
		{"equal nonzero", variantMask{5, 9}, variantMask{5, 9}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.cmp(tt.b); got != tt.want {
				t.Fatalf("cmp(%v, %v) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// The registry is reset so the goroutines race its first build, and the two
// themes disagree about `text-brand` (a color in one, a font size in the
// other), so a goroutine that saw the wrong theme sorts differently.
func TestSortConcurrentAcrossThemes(t *testing.T) {
	colorTheme := mustTheme(t, []ThemeToken{{Namespace: "color", Name: "brand"}}, nil, nil, nil)
	sizeTheme := mustTheme(t, []ThemeToken{{Namespace: "text", Name: "brand"}}, nil, nil, nil)

	registryOnce = sync.Once{}
	utilityRegistry = nil
	stemRegistry = nil
	variantRegistry = nil
	propertyIndex = nil
	customVariantOrder = 0
	breakpointGroups = nil

	const goroutines = 16
	const iterations = 50

	input := "text-brand font-bold"
	themes := []*Theme{colorTheme, sizeTheme}
	want := []string{"font-bold text-brand", "text-brand font-bold"}
	results := make([]string, goroutines)

	var wg sync.WaitGroup

	for i := range goroutines {
		wg.Go(func() {
			var got string
			for range iterations {
				got = Sort(themes[i%2], input)
			}

			results[i] = got
		})
	}

	wg.Wait()

	for i, got := range results {
		if got != want[i%2] {
			t.Fatalf("goroutine %d: Sort = %q, want %q", i, got, want[i%2])
		}
	}
}

func mustTheme(t *testing.T, tokens []ThemeToken, utilities []CustomUtility, variants []CustomVariant, prefixes []string) *Theme {
	t.Helper()

	return newTheme(tokens, utilities, variants, prefixes)
}

func TestSortPinnedHoldsBoundaryToken(t *testing.T) {
	cases := []struct {
		in       string
		pinFirst bool
		pinLast  bool
		want     string
	}{
		{"rounded-full ring-offset-2 bg-", false, true, "rounded-full ring-offset-2 bg-"},
		{"p-4 flex bg-", false, true, "flex p-4 bg-"},
		{"-500 m-2", true, false, "-500 m-2"},
		{"text-white bg-blue-500 p-4", false, false, "bg-blue-500 p-4 text-white"},
		{"bg- text-white p-4", true, false, "bg- p-4 text-white"},
		{"p-4 text-white bg-", true, true, "p-4 text-white bg-"},
		{"bg- p-4", true, true, "bg- p-4"},
		{"bg-", true, true, "bg-"},
	}

	for _, c := range cases {
		if got := SortPinned(nil, c.in, c.pinFirst, c.pinLast); got != c.want {
			t.Errorf("SortPinned(%q, %v, %v) = %q, want %q", c.in, c.pinFirst, c.pinLast, got, c.want)
		}
	}
}

// Every want string here is prettier-plugin-tailwindcss's verbatim output.
func TestSortArbitraryVariantGrouping(t *testing.T) {
	cases := []struct{ in, want string }{
		{
			"[&::-webkit-datetime-edit]:justify-center [&::-webkit-calendar-picker-indicator]:hidden [&::-webkit-datetime-edit]:flex",
			"[&::-webkit-calendar-picker-indicator]:hidden [&::-webkit-datetime-edit]:flex [&::-webkit-datetime-edit]:justify-center",
		},
		{
			"[&>svg]:size-4 [&:hover]:underline [&>svg]:p-0 [&:hover]:font-bold",
			"[&:hover]:font-bold [&:hover]:underline [&>svg]:size-4 [&>svg]:p-0",
		},
		{"[&>svg]:p-0 flex hover:block p-4", "flex p-4 hover:block [&>svg]:p-0"},
	}

	for _, c := range cases {
		if got := Sort(nil, c.in); got != c.want {
			t.Errorf("Sort(%q)\n = %q\nwant %q", c.in, got, c.want)
		}
	}
}

// Every `@`-prefixed directive Laravel ships is listed, since a wrong true
// would move a directive inside a class list. `@container` is Tailwind v4's
// own utility; the v3-era `container` must not answer for it.
func TestIsKnownUtilityRejectsBladeDirectiveNames(t *testing.T) {
	directives := []string{
		"@if", "@elseif", "@else", "@endif", "@unless", "@endunless",
		"@isset", "@endisset", "@empty", "@endempty", "@switch", "@case",
		"@break", "@default", "@endswitch", "@for", "@endfor", "@foreach",
		"@endforeach", "@forelse", "@endforelse", "@while", "@endwhile",
		"@continue", "@each", "@include", "@includeIf", "@extends", "@section",
		"@endsection", "@yield", "@parent", "@show", "@stop", "@append",
		"@push", "@endpush", "@prepend", "@stack", "@once", "@endonce",
		"@csrf", "@method", "@error", "@enderror", "@json", "@js", "@class",
		"@style", "@checked", "@disabled", "@readonly", "@required",
		"@selected", "@props", "@aware", "@slot", "@endslot", "@verbatim",
		"@endverbatim", "@php", "@endphp", "@inject", "@auth", "@endauth",
		"@guest", "@endguest", "@can", "@cannot", "@canany", "@endcan",
		"@lang", "@choice", "@dd", "@dump", "@env", "@endenv", "@production",
		"@fragment", "@endfragment", "@use", "@session", "@endsession",
		"@vite", "@viteReactRefresh", "@persist", "@teleport", "@script",
		"@endscript", "@assets", "@entangle", "@livewireStyles",
		"@livewireScripts", "@bool", "@hasSection", "@sectionMissing",
	}

	for _, d := range directives {
		if IsKnownUtility(d) {
			t.Errorf("IsKnownUtility(%q) = true; a Blade directive must not read as a class", d)
		}
	}

	known := []string{"@container", "@container-normal"}

	for _, u := range known {
		if !IsKnownUtility(u) {
			t.Errorf("IsKnownUtility(%q) = false; want true", u)
		}
	}

	if IsKnownUtility("@some-project-class") {
		t.Error(`IsKnownUtility("@some-project-class") = true; want false`)
	}
}

// A DOMTokenList splits on ASCII whitespace only, so a vertical tab or a
// non-breaking space is part of the class it sits in.
func TestSortSplitsOnlyOnHTMLWhitespace(t *testing.T) {
	const nbsp = string(rune(0xa0))

	cases := map[string]struct{ in, want string }{
		"vertical tab":       {"p-4 x\vy m-2", "x\vy m-2 p-4"},
		"non-breaking space": {"p-4 x" + nbsp + "y m-2", "x" + nbsp + "y m-2 p-4"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := Sort(nil, tc.in); got != tc.want {
				t.Fatalf("Sort(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
