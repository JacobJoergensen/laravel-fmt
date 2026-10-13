package tailwind

import (
	"bytes"
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestExtractThemeReturnsRecognisedTokens(t *testing.T) {
	src := []byte(`@theme {
  --color-brand: #123456;
  --color-brand-soft: #abcdef;
  --text-display: 1.5rem;
  --shadow-glow: 0 0 8px red;
  --font-weight-bold: 700;
  --radius-tight: 2px;
}`)

	got := ExtractTheme(src)
	want := []ThemeToken{
		{Namespace: "color", Name: "brand", Value: "#123456"},
		{Namespace: "color", Name: "brand-soft", Value: "#abcdef"},
		{Namespace: "text", Name: "display", Value: "1.5rem"},
		{Namespace: "shadow", Name: "glow", Value: "0 0 8px red"},
		{Namespace: "font-weight", Name: "bold", Value: "700"},
		{Namespace: "radius", Name: "tight", Value: "2px"},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d tokens, want %d: %+v", len(got), len(want), got)
	}

	for i, w := range want {
		if got[i] != w {
			t.Fatalf("token %d: got %+v, want %+v", i, got[i], w)
		}
	}
}

func TestExtractThemeLongestPrefixMatchesNamespace(t *testing.T) {
	src := []byte(`@theme {
  --font-weight-bold: 700;
  --font-display: "Inter", sans-serif;
}`)

	got := ExtractTheme(src)

	if len(got) != 2 {
		t.Fatalf("got %d tokens, want 2: %+v", len(got), got)
	}

	if got[0].Namespace != "font-weight" || got[0].Name != "bold" {
		t.Fatalf("token 0: got %+v, want {font-weight, bold}", got[0])
	}

	if got[1].Namespace != "font" || got[1].Name != "display" {
		t.Fatalf("token 1: got %+v, want {font, display}", got[1])
	}
}

func TestExtractThemeIgnoresNonThemeAtRules(t *testing.T) {
	src := []byte(`@import "tailwindcss";

@layer base {
  --color-not-a-theme-token: red;
}

@theme {
  --color-brand: #fff;
}`)

	got := ExtractTheme(src)

	if len(got) != 1 {
		t.Fatalf("got %d tokens, want 1: %+v", len(got), got)
	}

	if got[0] != (ThemeToken{Namespace: "color", Name: "brand", Value: "#fff"}) {
		t.Fatalf("got %+v, want {color, brand, #fff}", got[0])
	}
}

func TestExtractThemeSkipsNonCustomDeclarations(t *testing.T) {
	src := []byte(`@theme {
  color: red;
  --color-brand: #fff;
  --not-a-namespace: 1;
}`)

	got := ExtractTheme(src)

	if len(got) != 1 || got[0].Name != "brand" {
		t.Fatalf("got %+v, want one {color, brand}", got)
	}
}

func TestExtractThemeStatementFormIgnored(t *testing.T) {
	src := []byte(`@theme inline;`)

	if got := ExtractTheme(src); len(got) != 0 {
		t.Fatalf("got %+v, want no tokens", got)
	}
}

func TestExtractThemeEmpty(t *testing.T) {
	for _, src := range [][]byte{nil, {}, []byte(`body { color: red; }`)} {
		if got := ExtractTheme(src); len(got) != 0 {
			t.Fatalf("ExtractTheme(%q): got %+v, want empty", string(src), got)
		}
	}
}

func TestSortThemeRoutesTextToFontSize(t *testing.T) {
	theme := mustTheme(t, []ThemeToken{
		{Namespace: "color", Name: "brand"},
		{Namespace: "text", Name: "display"},
	}, nil, nil, nil)

	got := Sort(theme, "text-brand text-display p-4")
	want := "p-4 text-display text-brand"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSortThemeRoutesShadowToBoxShadow(t *testing.T) {
	theme := mustTheme(t, []ThemeToken{
		{Namespace: "color", Name: "muted"},
		{Namespace: "shadow", Name: "glow"},
	}, nil, nil, nil)

	got := Sort(theme, "shadow-muted shadow-glow p-4")
	want := "p-4 shadow-glow shadow-muted"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSortThemeIsIdempotent(t *testing.T) {
	theme := mustTheme(t, []ThemeToken{
		{Namespace: "color", Name: "brand"},
		{Namespace: "text", Name: "display"},
		{Namespace: "shadow", Name: "glow"},
	}, nil, nil, nil)

	inputs := []string{
		"text-display text-brand p-4",
		"shadow-glow shadow-muted text-display",
		"text-[14px] text-[#fff] text-display text-brand",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			once := Sort(theme, input)
			twice := Sort(theme, once)

			if once != twice {
				t.Fatalf("not idempotent: once=%q twice=%q", once, twice)
			}
		})
	}
}

func TestLoadTheme(t *testing.T) {
	src := []byte(`@import "tailwindcss";

@theme {
  --color-brand: #123;
  --text-display: 1.5rem;
}`)

	path := filepath.Join(t.TempDir(), "app.css")

	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatalf("write temp css: %v", err)
	}

	theme, err := LoadTheme(path)
	if err != nil {
		t.Fatalf("LoadTheme: %v", err)
	}

	if got := theme.branchFor("text", "display"); got != branchSize {
		t.Fatalf("branchFor(text, display) = %d, want size", got)
	}

	if got := theme.branchFor("text", "brand"); got != branchColor {
		t.Fatalf("branchFor(text, brand) = %d, want color", got)
	}
}

func TestLoadThemeMissingPathErrors(t *testing.T) {
	_, err := LoadTheme(filepath.Join(t.TempDir(), "does-not-exist.css"))

	if err == nil {
		t.Fatalf("LoadTheme(missing): err = nil, want non-nil")
	}
}

func TestContainerDefaultsMatchRank(t *testing.T) {
	if len(defaultContainerWidths) != len(containerSizeRank) {
		t.Fatalf("defaultContainerWidths has %d entries, containerSizeRank has %d",
			len(defaultContainerWidths), len(containerSizeRank))
	}

	names := make([]string, 0, len(defaultContainerWidths))
	for name := range defaultContainerWidths {
		names = append(names, name)
	}

	slices.SortFunc(names, func(a, b string) int {
		return cmp.Compare(defaultContainerWidths[a], defaultContainerWidths[b])
	})

	for i, name := range names {
		if containerSizeRank[name] != i {
			t.Fatalf("container %q: width order %d != containerSizeRank %d", name, i, containerSizeRank[name])
		}
	}
}

func TestSortCustomContainerSizesByWidth(t *testing.T) {
	theme := mustTheme(t, []ThemeToken{
		{Namespace: "container", Name: "tiny", Value: "10rem"},
		{Namespace: "container", Name: "huge", Value: "100rem"},
	}, nil, nil, nil)

	got := Sort(theme, "@huge:flex @md:flex @tiny:flex")
	want := "@tiny:flex @md:flex @huge:flex"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if twice := Sort(theme, got); twice != got {
		t.Fatalf("not idempotent: %q -> %q", got, twice)
	}
}

func TestSortCustomContainerOverridesDefaultWidth(t *testing.T) {
	theme := mustTheme(t, []ThemeToken{
		{Namespace: "container", Name: "sm", Value: "90rem"},
	}, nil, nil, nil)

	got := Sort(theme, "@sm:flex @xl:flex")
	want := "@xl:flex @sm:flex"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSortContainerUnchangedWithoutCustomSizes(t *testing.T) {
	theme := mustTheme(t, []ThemeToken{
		{Namespace: "color", Name: "brand", Value: "#fff"},
		{Namespace: "container", Name: "weird", Value: "calc(100% - 2rem)"},
	}, nil, nil, nil)

	got := Sort(theme, "@2xl:p-4 @sm:p-4")
	want := "@sm:p-4 @2xl:p-4"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestParseThemeWidth(t *testing.T) {
	tests := []struct {
		value  string
		want   float64
		wantOK bool
	}{
		{"10rem", 160, true},
		{"1.5rem", 24, true},
		{"768px", 768, true},
		{"20em", 320, true},
		{" 32rem ", 512, true},
		{"calc(100% - 2rem)", 0, false},
		{"var(--x)", 0, false},
		{"100", 0, false},
		{"", 0, false},
		{"rem", 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			got, ok := parseThemeWidth(tc.value)

			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("parseThemeWidth(%q) = (%v, %v), want (%v, %v)",
					tc.value, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestBreakpointDefaultsMatchRegistry(t *testing.T) {
	ensureRegistry()

	names := make([]string, 0, len(defaultBreakpointWidths))
	for name := range defaultBreakpointWidths {
		names = append(names, name)
	}

	slices.SortFunc(names, func(a, b string) int {
		return cmp.Compare(defaultBreakpointWidths[a], defaultBreakpointWidths[b])
	})

	for i := 1; i < len(names); i++ {
		prev := variantRegistry[names[i-1]].Priority
		cur := variantRegistry[names[i]].Priority

		if cur != prev+1 {
			t.Fatalf("breakpoint %q priority %d is not one above %q priority %d; width order and generated order have drifted",
				names[i], cur, names[i-1], prev)
		}
	}
}

func TestSortCustomBreakpointSizesByWidth(t *testing.T) {
	theme := mustTheme(t, []ThemeToken{
		{Namespace: "breakpoint", Name: "xs", Value: "20rem"},
		{Namespace: "breakpoint", Name: "3xl", Value: "120rem"},
	}, nil, nil, nil)

	got := Sort(theme, "sm:flex 3xl:flex xs:flex md:flex")
	want := "xs:flex sm:flex md:flex 3xl:flex"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if twice := Sort(theme, got); twice != got {
		t.Fatalf("not idempotent: %q -> %q", got, twice)
	}
}

func TestSortCustomBreakpointOverridesDefaultWidth(t *testing.T) {
	theme := mustTheme(t, []ThemeToken{
		{Namespace: "breakpoint", Name: "md", Value: "20rem"},
	}, nil, nil, nil)

	got := Sort(theme, "sm:flex md:flex")
	want := "md:flex sm:flex"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSortCustomBreakpointPreservesMixedChainOrder(t *testing.T) {
	const input = "2xl:flex lg:hover:flex"
	const want = "lg:hover:flex 2xl:flex"

	if got := Sort(nil, input); got != want {
		t.Fatalf("default: got %q, want %q", got, want)
	}

	theme := mustTheme(t, []ThemeToken{
		{Namespace: "breakpoint", Name: "xs", Value: "20rem"},
	}, nil, nil, nil)

	if got := Sort(theme, input); got != want {
		t.Fatalf("with custom breakpoint: got %q, want %q", got, want)
	}
}

func TestSortBreakpointUnchangedWithoutCustomSizes(t *testing.T) {
	theme := mustTheme(t, []ThemeToken{
		{Namespace: "color", Name: "brand", Value: "#fff"},
		{Namespace: "breakpoint", Name: "weird", Value: "calc(100% - 2rem)"},
	}, nil, nil, nil)

	got := Sort(theme, "md:flex sm:flex")
	want := "sm:flex md:flex"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExtractUtilities(t *testing.T) {
	src := []byte(`@utility tab-4 {
  tab-size: 4;
}

@utility card {
  display: flex;
  gap: 1rem;
}

@utility scrollbar-* {
  scrollbar-width: var(--value);
}`)

	got := ExtractUtilities(src)

	want := []CustomUtility{
		{Name: "tab-4", Properties: []string{"tab-size"}},
		{Name: "card", Properties: []string{"display", "gap"}},
		{Name: "scrollbar-*", Properties: []string{"scrollbar-width"}},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d utilities, want %d: %+v", len(got), len(want), got)
	}

	for i, w := range want {
		if got[i].Name != w.Name || !slices.Equal(got[i].Properties, w.Properties) {
			t.Fatalf("utility %d: got %+v, want %+v", i, got[i], w)
		}
	}
}

func TestSortCustomUtilityResolvesAtProperty(t *testing.T) {
	theme := mustTheme(t, nil, []CustomUtility{
		{Name: "myflex", Properties: []string{"display"}},
	}, nil, nil)

	// A custom @utility setting `display` sorts at the arbitrary-display slot, which
	// getClassOrder places before the builtin named display utilities (`flex`).
	got := Sort(theme, "custom-x myflex flex")
	want := "custom-x myflex flex"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if twice := Sort(theme, got); twice != got {
		t.Fatalf("not idempotent: %q -> %q", got, twice)
	}
}

func TestSortUtilityUnchangedWithoutRegistration(t *testing.T) {
	theme := mustTheme(t, []ThemeToken{
		{Namespace: "color", Name: "brand", Value: "#fff"},
	}, nil, nil, nil)

	got := Sort(theme, "flex card-fancy")
	want := "card-fancy flex"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExtractCustomVariants(t *testing.T) {
	src := []byte(`@custom-variant pointer-fine (@media (pointer: fine));

@custom-variant theme-midnight {
  &:where([data-theme="midnight"] *) {
    @slot;
  }
}

@custom-variant sidebar-* (&:where([data-sidebar="*"]));`)

	got := ExtractCustomVariants(src)
	want := []string{"pointer-fine", "theme-midnight", "sidebar-*"}

	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExtractPrefixes(t *testing.T) {
	cases := map[string]struct {
		src  string
		want []string
	}{
		"with prefix":    {`@import "tailwindcss" prefix(tw);`, []string{"tw"}},
		"without prefix": {`@import "tailwindcss";`, []string{""}},
		"source qualifier and no prefix": {
			`@import 'tailwindcss' source(none);`,
			[]string{""},
		},
		// Entry points merge, so a project can present both at once.
		"one bundle prefixed, one not": {
			"@import \"tailwindcss\" prefix(tw);\n@import \"tailwindcss\";",
			[]string{"tw", ""},
		},
		"repeats collapse": {
			"@import \"tailwindcss\" prefix(tw);\n@import \"tailwindcss\" prefix(tw);",
			[]string{"tw"},
		},
		// prefix() belongs to Tailwind's own import; another import is not one.
		"unrelated import is not a bundle": {`@import "bootstrap" prefix(bs);`, nil},
		"no tailwind at all":               {`body { color: red; }`, nil},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ExtractPrefixes([]byte(tc.src)); !slices.Equal(got, tc.want) {
				t.Fatalf("ExtractPrefixes = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSortCustomVariantOrder(t *testing.T) {
	theme := mustTheme(t, nil, nil, []string{"theme-a", "theme-b"}, nil)

	got := Sort(theme, "[&_p]:flex theme-b:flex dark:flex theme-a:flex flex")
	want := "flex dark:flex theme-a:flex theme-b:flex [&_p]:flex"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if twice := Sort(theme, got); twice != got {
		t.Fatalf("not idempotent: %q -> %q", got, twice)
	}
}

func TestSortPrefixStripped(t *testing.T) {
	theme := mustTheme(t, nil, nil, nil, []string{"tw"})

	got := Sort(theme, "tw:hover:underline flex tw:flex")
	want := "flex tw:flex tw:hover:underline"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if twice := Sort(theme, got); twice != got {
		t.Fatalf("not idempotent: %q -> %q", got, twice)
	}
}

func TestSortFunctionalUtility(t *testing.T) {
	theme := mustTheme(t, nil, []CustomUtility{
		{Name: "tab-*", Properties: []string{"display"}},
	}, nil, nil)

	got := Sort(theme, "custom-x tab-[13] flex tab-7")
	want := "custom-x tab-7 tab-[13] flex"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if twice := Sort(theme, got); twice != got {
		t.Fatalf("not idempotent: %q -> %q", got, twice)
	}
}

func TestSortFunctionalCustomVariant(t *testing.T) {
	theme := mustTheme(t, nil, nil, []string{"theme-*"}, nil)

	got := Sort(theme, "[&_p]:flex theme-dawn:flex dark:flex theme-midnight:flex flex")
	want := "flex dark:flex theme-dawn:flex theme-midnight:flex [&_p]:flex"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if twice := Sort(theme, got); twice != got {
		t.Fatalf("not idempotent: %q -> %q", got, twice)
	}
}

func TestSortFunctionalDoesNotHijackBuiltin(t *testing.T) {
	theme := mustTheme(t, nil, nil, []string{"group-*"}, nil)

	got := Sort(theme, "dark:flex group-hover:flex flex")
	want := "flex group-hover:flex dark:flex"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLoadThemeFollowsImports(t *testing.T) {
	cases := map[string][2]string{
		"relative import":   {`@import "tailwindcss";` + "\n" + `@import "./parts/theme.css";`, "parts/theme.css"},
		"url() form":        {`@import url("./parts/theme.css");`, "parts/theme.css"},
		"layer qualifier":   {`@import "./parts/theme.css" layer(base);`, "parts/theme.css"},
		"uppercase at-rule": {`@IMPORT "./parts/theme.css";`, "parts/theme.css"},
		"no leading dot":    {`@import "parts/theme.css";`, "parts/theme.css"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeCSS(t, dir, "app.css", tc[0])
			writeCSS(t, dir, tc[1], "@theme { --color-brand: #f00; }\n@utility card-pad { padding: 1rem; }")

			theme := loadApp(t, dir)

			if !hasUtility(theme, "card-pad") {
				t.Fatal("the imported @utility was not registered")
			}
		})
	}
}

func TestLoadThemeSkipsNonLocalImports(t *testing.T) {
	dir := t.TempDir()
	writeCSS(t, dir, "app.css", `@import "tailwindcss";`+"\n"+`@import "some-pkg/theme.css";`+"\n@utility card-pad { padding: 1rem; }")

	theme := loadApp(t, dir)

	if !hasUtility(theme, "card-pad") {
		t.Fatal("a non-local import should be skipped, not abandon the file")
	}
}

// Asserted on the expanded source, because the extractors skip comments and
// the registry could not tell: the hazard is spliced content carrying `*/`
// that closes the comment early.
func TestReadWithImportsIgnoresCommentedImport(t *testing.T) {
	dir := t.TempDir()
	writeCSS(t, dir, "app.css", `/* @import "./parts/theme.css"; */`)
	writeCSS(t, dir, "parts/theme.css", "@utility card-pad { padding: 1rem; }")

	src, err := readWithImports(filepath.Join(dir, "app.css"), map[string]bool{}, map[string]fileStamp{})
	if err != nil {
		t.Fatalf("readWithImports: %v", err)
	}

	if bytes.Contains(src, []byte("card-pad")) {
		t.Fatalf("a commented-out import was followed:\n%s", src)
	}
}

func TestLoadThemeSurvivesImportCycle(t *testing.T) {
	dir := t.TempDir()
	writeCSS(t, dir, "app.css", `@import "./a.css";`)
	writeCSS(t, dir, "a.css", `@import "./app.css";`+"\n@utility card-pad { padding: 1rem; }")

	theme := loadApp(t, dir)

	if !hasUtility(theme, "card-pad") {
		t.Fatal("the cycle lost the definitions after it")
	}
}

func TestLoadThemeMergesEntryPoints(t *testing.T) {
	dir := t.TempDir()
	writeCSS(t, dir, "app.css", `@import "tailwindcss";`+"\n@utility card-pad { padding: 1rem; }")
	writeCSS(t, dir, "admin.css", `@import "tailwindcss";`+"\n@utility admin-pad { padding: 2rem; }")

	theme, err := LoadTheme(filepath.Join(dir, "admin.css"), filepath.Join(dir, "app.css"))
	if err != nil {
		t.Fatalf("LoadTheme: %v", err)
	}

	for _, name := range []string{"card-pad", "admin-pad"} {
		if !hasUtility(theme, name) {
			t.Errorf("@utility %s from its own entry point was not registered", name)
		}
	}

	got := Sort(theme, "card-pad admin-pad p-4 flex")
	want := "flex admin-pad card-pad p-4"

	if got != want {
		t.Fatalf("Sort = %q, want %q", got, want)
	}
}

// The entry points merge in the order given, and CSS gives a later definition
// the win. The two here differ only in the property they set, which is what the
// sorter ranks them by, so the order of the merge is visible in the result.
func TestLoadThemeLaterEntryPointWins(t *testing.T) {
	dir := t.TempDir()
	writeCSS(t, dir, "first.css", "@utility ambiguous { margin: 1rem; }")
	writeCSS(t, dir, "second.css", "@utility ambiguous { padding: 1rem; }")

	first := filepath.Join(dir, "first.css")
	second := filepath.Join(dir, "second.css")

	sortWith := func(paths ...string) string {
		t.Helper()

		theme, err := LoadTheme(paths...)
		if err != nil {
			t.Fatalf("LoadTheme: %v", err)
		}

		return Sort(theme, "ambiguous m-1 p-1")
	}

	firstAlone := sortWith(first)
	secondAlone := sortWith(second)

	if firstAlone == secondAlone {
		t.Fatalf("the two definitions sort alike, so the case proves nothing: %q", firstAlone)
	}

	if got := sortWith(first, second); got != secondAlone {
		t.Fatalf("merged = %q, want the later entry point's %q", got, secondAlone)
	}

	if got := sortWith(second, first); got != firstAlone {
		t.Fatalf("reversed merge = %q, want the later entry point's %q", got, firstAlone)
	}
}

func writeCSS(t *testing.T, dir, rel, body string) {
	t.Helper()

	full := filepath.Join(dir, filepath.FromSlash(rel))

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", full, err)
	}

	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

func TestLooksLikeTailwindV4Import(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"double-quoted", `@import "tailwindcss";`, true},
		{"single-quoted", `@import 'tailwindcss';`, true},
		{"subpath-preflight", `@import "tailwindcss/preflight";`, true},
		{"subpath-theme", `@import 'tailwindcss/theme';`, true},
		{"no-semicolon", `@import "tailwindcss"`, true},
		{"source-qualifier", `@import 'tailwindcss' source(none);`, true},
		{"mid-file", "body { color: red; }\n@import \"tailwindcss\";\n", true},
		{"unrelated-import", `@import "bootstrap";`, false},
		{"bare-mention", `/* tailwindcss is great */`, false},
		{"url-form", `@import url("tailwindcss");`, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := looksLikeTailwindV4Import([]byte(tc.in))

			if got != tc.want {
				t.Fatalf("looksLikeTailwindV4Import(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// The fixture mirrors Filament, whose resources/css/filament/<panel>/theme.css
// is a single @import of a vendored stylesheet that holds the Tailwind import.
func TestImportsTailwindFollowsImports(t *testing.T) {
	dir := t.TempDir()
	writeCSS(t, dir, "theme.css", `@import '../vendor/pkg/theme.css';`)
	writeCSS(t, dir, "../vendor/pkg/theme.css", "@import 'tailwindcss' source(none);\n@import './index.css';")
	writeCSS(t, dir, "../vendor/pkg/index.css", ".fi-btn { color: red; }")
	writeCSS(t, dir, "partial.css", "@utility card-pad { padding: 1rem; }")
	writeCSS(t, dir, "cycle.css", `@import './cycle.css';`)

	cases := map[string]struct {
		file string
		want bool
	}{
		"reaches tailwind through an import": {"theme.css", true},
		"a partial is not an entry point":    {"partial.css", false},
		"a cycle terminates":                 {"cycle.css", false},
		"a missing file is not one":          {"nope.css", false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ImportsTailwind(filepath.Join(dir, tc.file)); got != tc.want {
				t.Fatalf("ImportsTailwind(%s) = %v, want %v", tc.file, got, tc.want)
			}
		})
	}
}

// 100 files clears the old 64-file bound; Filament's panel theme alone reaches
// 129.
func TestLoadThemeReadsPastTheOldFileBound(t *testing.T) {
	const files = 100

	dir := t.TempDir()

	var index strings.Builder

	for i := range files {
		name := fmt.Sprintf("parts/c%03d.css", i)
		fmt.Fprintf(&index, "@import './%s';\n", name)
		writeCSS(t, dir, name, "")
	}

	writeCSS(t, dir, fmt.Sprintf("parts/c%03d.css", files-1), "@utility card-pad { padding: 1rem; }")
	writeCSS(t, dir, "app.css", `@import "tailwindcss";`+"\n"+index.String())

	theme := loadApp(t, dir)

	if !hasUtility(theme, "card-pad") {
		t.Fatalf("a definition %d files into the graph was dropped", files)
	}
}

func TestReadWithImportsReportsExhaustedBudget(t *testing.T) {
	dir := t.TempDir()

	var index strings.Builder

	for i := range maxImportedFiles + 1 {
		name := fmt.Sprintf("parts/c%04d.css", i)
		fmt.Fprintf(&index, "@import './%s';\n", name)
		writeCSS(t, dir, name, "")
	}

	writeCSS(t, dir, "app.css", index.String())

	_, err := readWithImports(filepath.Join(dir, "app.css"), map[string]bool{}, map[string]fileStamp{})

	if err == nil {
		t.Fatal("readWithImports silently truncated an oversized graph")
	}
}

// The edit is to an imported file rather than the entry point, since the
// cache stamps the whole graph.
func TestLoadThemeNoticesAnImportedFileChanging(t *testing.T) {
	dir := t.TempDir()
	writeCSS(t, dir, "app.css", `@import "tailwindcss";`+"\n"+`@import "./parts/theme.css";`)
	writeCSS(t, dir, "parts/theme.css", "@utility card-pad { padding: 1rem; }")

	theme := loadApp(t, dir)

	if !hasUtility(theme, "card-pad") {
		t.Fatal("the imported @utility was not registered")
	}

	writeCSS(t, dir, "parts/theme.css", "@utility deck-gap { gap: 2rem; }")
	theme = loadApp(t, dir)

	if hasUtility(theme, "card-pad") {
		t.Error("a utility the file no longer defines survived")
	}

	if !hasUtility(theme, "deck-gap") {
		t.Error("a utility the file now defines was not picked up")
	}
}

// An edit that keeps the length: only the modification time gives it away. The
// stamp is set explicitly because two writes in one test land inside the
// filesystem's timestamp resolution, which a person editing a file does not.
func TestLoadThemeNoticesASameSizeEdit(t *testing.T) {
	dir := t.TempDir()
	writeCSS(t, dir, "app.css", `@import "tailwindcss";`+"\n"+`@import "./parts/theme.css";`)
	writeCSS(t, dir, "parts/theme.css", "@utility aaaa-pad { padding: 1rem; }")

	theme := loadApp(t, dir)

	if !hasUtility(theme, "aaaa-pad") {
		t.Fatal("the imported @utility was not registered")
	}

	edited := filepath.Join(dir, "parts", "theme.css")
	writeCSS(t, dir, "parts/theme.css", "@utility bbbb-pad { padding: 1rem; }")

	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(edited, later, later); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	theme = loadApp(t, dir)

	if !hasUtility(theme, "bbbb-pad") {
		t.Fatal("a same-size edit went unnoticed")
	}
}

func TestLoadThemeNoticesADeletedImport(t *testing.T) {
	dir := t.TempDir()
	writeCSS(t, dir, "app.css", `@import "tailwindcss";`+"\n"+`@import "./parts/theme.css";`)
	writeCSS(t, dir, "parts/theme.css", "@utility card-pad { padding: 1rem; }")

	loadApp(t, dir)

	if err := os.Remove(filepath.Join(dir, "parts", "theme.css")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	theme := loadApp(t, dir)

	if hasUtility(theme, "card-pad") {
		t.Fatal("a definition from a deleted file survived")
	}
}

func TestLoadThemeKeepsEachProjectsTheme(t *testing.T) {
	app, admin := t.TempDir(), t.TempDir()
	writeCSS(t, app, "app.css", `@import "tailwindcss";`+"\n@utility card-pad { padding: 1rem; }")
	writeCSS(t, admin, "app.css", `@import "tailwindcss";`+"\n@utility admin-pad { padding: 2rem; }")

	first := loadApp(t, app)
	loadApp(t, admin)

	if again := loadApp(t, app); again != first {
		t.Fatal("loading another project's stylesheets evicted this one's theme")
	}
}

func TestNewThemeRejectsMoreVariantsThanTheMaskHolds(t *testing.T) {
	mustTheme(t, nil, nil, []string{"small"}, nil)

	variants := make([]string, 0, variantMaskSlots)
	for i := range variantMaskSlots - arbitraryVariantSlot {
		variants = append(variants, fmt.Sprintf("v%d", i))
	}

	if _, err := newTheme(nil, nil, variants, nil); err == nil {
		t.Fatalf("newTheme accepted %d custom variants, which overflow the variant mask", len(variants))
	}

	// One fewer fits, and its unknown-variant slot is the mask's last bit.
	fits := variants[:len(variants)-1]
	theme := mustTheme(t, nil, nil, fits, nil)

	input := "unknown:flex " + fits[len(fits)-1] + ":flex"
	want := fits[len(fits)-1] + ":flex unknown:flex"

	if got := Sort(theme, input); got != want {
		t.Fatalf("Sort = %q, want %q", got, want)
	}
}

func TestSortAcceptsEveryBundlesPrefix(t *testing.T) {
	theme := mustTheme(t, nil, nil, nil, []string{"tw", ""})

	cases := map[string]struct{ in, want string }{
		"the prefixed bundle's classes":   {"tw:p-4 tw:flex", "tw:flex tw:p-4"},
		"the unprefixed bundle's classes": {"p-4 flex", "flex p-4"},
		"variants under either":           {"hover:underline tw:hover:underline p-4", "p-4 hover:underline tw:hover:underline"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := Sort(theme, tc.in)

			if got != tc.want {
				t.Fatalf("Sort(%q) = %q, want %q", tc.in, got, tc.want)
			}

			if twice := Sort(theme, got); twice != got {
				t.Fatalf("not idempotent: %q -> %q", got, twice)
			}
		})
	}
}

// Accepting unprefixed classes is what a recorded "" means, and no bundle here
// records one.
func TestSortStillRejectsUnprefixedWhenEveryBundlePrefixes(t *testing.T) {
	theme := mustTheme(t, nil, nil, nil, []string{"tw"})

	got := Sort(theme, "flex p-4 tw:p-4 tw:flex")
	want := "flex p-4 tw:flex tw:p-4"

	if got != want {
		t.Fatalf("Sort = %q, want %q; unprefixed classes must stay Unknown", got, want)
	}
}

func loadApp(t *testing.T, dir string) *Theme {
	t.Helper()

	theme, err := LoadTheme(filepath.Join(dir, "app.css"))
	if err != nil {
		t.Fatalf("LoadTheme: %v", err)
	}

	return theme
}

func hasUtility(theme *Theme, name string) bool {
	_, ok := theme.customUtility(name)

	return ok
}
