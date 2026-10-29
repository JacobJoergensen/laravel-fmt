package detector

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestParseEditorConfigSections(t *testing.T) {
	raw := []byte(`root = true

[*]
indent_style = space
indent_size = 4
end_of_line = lf

[*.php]
indent_size = 4
max_line_length = 120

[*.md]
indent_size = 2
`)

	var ec EditorConfig

	if err := parseEditorConfig(raw, &ec); err != nil {
		t.Fatalf("parseEditorConfig: %v", err)
	}

	want := []EditorConfigSection{
		{Pattern: "*", IndentStyle: "space", IndentSize: 4, EndOfLine: "lf"},
		{Pattern: "*.php", IndentSize: 4, MaxLineLength: 120},
		{Pattern: "*.md", IndentSize: 2},
	}

	if !reflect.DeepEqual(ec.Sections, want) {
		t.Fatalf("Sections = %#v, want %#v", ec.Sections, want)
	}
}

func TestEditorConfigResolveForFile(t *testing.T) {
	ec := EditorConfig{
		Sections: []EditorConfigSection{
			{Pattern: "*", IndentStyle: "space", IndentSize: 4, EndOfLine: "lf"},
			{Pattern: "*.php", IndentSize: 4, MaxLineLength: 120},
			{Pattern: "*.md", IndentSize: 2},
		},
	}

	tests := []struct {
		name string
		rel  string
		want EditorConfigSection
	}{
		{
			"php file picks up [*] and [*.php]",
			"app/Models/User.php",
			EditorConfigSection{IndentStyle: "space", IndentSize: 4, EndOfLine: "lf", MaxLineLength: 120},
		},
		{
			"md file picks up [*] and [*.md]",
			"README.md",
			EditorConfigSection{IndentStyle: "space", IndentSize: 2, EndOfLine: "lf"},
		},
		{
			"unmatched extension picks up only [*]",
			"public/robots.txt",
			EditorConfigSection{IndentStyle: "space", IndentSize: 4, EndOfLine: "lf"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ec.ResolveForFile(tc.rel)

			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ResolveForFile(%q) = %#v, want %#v", tc.rel, got, tc.want)
			}
		})
	}
}

// Expected values match the EditorConfig core (npm editorconfig).
func TestEditorConfigUnsetClearsInheritedValue(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want EditorConfigSection
	}{
		{
			"unset clears every key it names",
			"[*]\nindent_style = space\nindent_size = 2\ntab_width = 8\nend_of_line = crlf\nmax_line_length = 100\n\n[*.php]\nindent_style = unset\nindent_size = unset\ntab_width = unset\nend_of_line = unset\nmax_line_length = unset\n",
			EditorConfigSection{
				IndentStyle:   editorConfigUnset,
				IndentSize:    editorConfigCleared,
				TabWidth:      editorConfigCleared,
				EndOfLine:     editorConfigUnset,
				MaxLineLength: editorConfigCleared,
			},
		},
		{
			"unset only clears the keys it names",
			"[*]\nindent_size = 2\nmax_line_length = 100\n\n[*.php]\nindent_size = unset\n",
			EditorConfigSection{IndentSize: editorConfigCleared, MaxLineLength: 100},
		},
		{
			"clearing indent_size leaves an explicit tab_width standing",
			"[*]\nindent_size = 2\ntab_width = 8\n\n[*.php]\nindent_size = unset\n",
			EditorConfigSection{IndentSize: editorConfigCleared, TabWidth: 8},
		},
		{
			"UNSET is case-insensitive",
			"[*]\nindent_size = 2\n\n[*.php]\nindent_size = UNSET\n",
			EditorConfigSection{IndentSize: editorConfigCleared},
		},
		{
			"a later section sets a cleared key again",
			"[*]\nindent_size = 2\n\n[*.php]\nindent_size = unset\n\n[User.php]\nindent_size = 6\n",
			EditorConfigSection{IndentSize: 6},
		},
		{
			"indent_size = tab defers the width to tab_width",
			"[*]\nindent_size = 2\ntab_width = 8\n\n[*.php]\nindent_size = tab\n",
			EditorConfigSection{IndentSize: editorConfigCleared, TabWidth: 8},
		},
		{
			"max_line_length = off clears the limit",
			"[*]\nmax_line_length = 100\n\n[*.php]\nmax_line_length = off\n",
			EditorConfigSection{MaxLineLength: editorConfigCleared},
		},
		{
			"an unparseable value is absent, so it still inherits",
			"[*]\nindent_size = 2\n\n[*.php]\nindent_size = wide\n",
			EditorConfigSection{IndentSize: 2},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var ec EditorConfig

			if err := parseEditorConfig([]byte(tc.raw), &ec); err != nil {
				t.Fatalf("parseEditorConfig: %v", err)
			}

			got := ec.ResolveForFile("app/Models/User.php")

			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ResolveForFile = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestEditorConfigGlobalMergesMultipleStarSections(t *testing.T) {
	ec := EditorConfig{
		Sections: []EditorConfigSection{
			{Pattern: "*", IndentSize: 4, EndOfLine: "lf"},
			{Pattern: "*.php", IndentSize: 2},
			{Pattern: "*", IndentSize: 8},
		},
	}

	got := ec.Global()
	want := EditorConfigSection{Pattern: "*", IndentSize: 8, EndOfLine: "lf"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Global() = %#v, want %#v", got, want)
	}
}

func TestEditorConfigGlobMatches(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{"star matches root file", "*", "README.md", true},
		{"star matches nested file via basename", "*", "app/Models/User.php", true},
		{"star.ext matches by basename", "*.php", "app/Models/User.php", true},
		{"star.ext does not cross dot wrong", "*.php", "User.txt", false},
		{"brace alternatives match either", "*.{php,blade.php}", "User.php", true},
		{"brace alternatives match the second", "*.{php,blade.php}", "show.blade.php", true},
		{"brace mismatch falls through", "*.{php,blade.php}", "README.md", false},
		{"question matches single char", "?.php", "a.php", true},
		{"question rejects two chars", "?.php", "ab.php", false},
		{"slash in pattern enables full-path mode", "app/*.php", "app/User.php", true},
		{"slash in pattern, basename match disallowed", "app/*.php", "User.php", false},
		{"slash single-star does not cross /", "app/*.php", "app/Models/User.php", false},
		{"double-star crosses directories", "app/**/*.php", "app/Models/User.php", true},
		{"double-star matches zero directories", "app/**/*.php", "app/User.php", true},
		{"double-star anywhere", "**/*.php", "any/deep/User.php", true},
		{"trailing star matches non-empty", "User.*", "User.php", true},
		{"nested braces expand", "*.{php,blade.{php,html}}", "x.blade.html", true},
		{"leading slash anchors to the config directory", "/top.php", "top.php", true},
		{"leading slash does not match deeper", "/top.php", "sub/top.php", false},
		{"double-star crosses / inside a basename pattern", "a**z.php", "a/z.php", true},
		{"single-alternative braces are literal", "{a}.php", "{a}.php", true},
		{"single-alternative braces do not expand", "{a}.php", "a.php", false},
		{"character class picks one of its members", "*.[ch]", "main.c", true},
		{"character class rejects a non-member", "*.[ch]", "main.o", false},
		{"numeric range matches a number in it", "v{1..9}.php", "v3.php", true},
		{"numeric range rejects a number outside it", "v{1..9}.php", "v12.php", false},
		{"caret negates a class like the references do", "[^abc].php", "d.php", true},
		{"caret negation excludes its members", "[^abc].php", "a.php", false},
		{"a single-character range is literal text", "{a..c}.php", "{a..c}.php", true},
		{"a single-character range does not expand", "{a..c}.php", "b.php", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := editorConfigGlobMatches(tc.pattern, tc.path)

			if got != tc.want {
				t.Fatalf("editorConfigGlobMatches(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
			}
		})
	}
}

// The cases are editorconfig-core-test's brackets.in and braces.in, kept
// verbatim: the suite is the spec's authority, even where the JavaScript
// core disagrees.
func TestEditorConfigGlobMatchesSpecSuite(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		// brackets.in
		{"[ab].a", "a.a", true},
		{"[ab].a", "c.a", false},
		{"[!ab].b", "c.b", true},
		{"[!ab].b", "a.b", false},
		{"[d-g].c", "f.c", true},
		{"[d-g].c", "h.c", false},
		{"[!d-g].d", "h.d", true},
		{"[!d-g].d", "f.d", false},
		{"[abd-g].e", "e.e", true},
		{"[-ab].f", "-.f", true},

		{`[\]ab].g`, "].g", true},
		{`[\]ab].g`, "c.g", false},
		{"[ab]].g", "b].g", true},
		{`[!\]ab].g`, "].g", false},
		{`[!\]ab].g`, "c.g", true},
		{"[!ab]].g", "c].g", true},

		{"ab[e/]cd.i", "ab/cd.i", false},
		{"ab[e/]cd.i", "abecd.i", false},
		{"ab[e/]cd.i", "ab[e/]cd.i", true},
		{"ab[/c", "ab[/c", true},

		// braces.in
		{"*.{py,js,html}", "test.py", true},
		{"*.{py,js,html}", "test.js", true},
		{"*.{py,js,html}", "test.html", true},
		{"*.{py,js,html}", "test.pyc", false},

		{"{single}.b", "{single}.b", true},
		{"{single}.b", "single.b", false},
		{"{single}.b", ".b", false},
		{"{}.c", "{}.c", true},
		{"{}.c", ".c", false},
		{"{.f", "{.f", true},
		{"{.f", ".f", false},

		{"a{b,c,}.d", "a.d", true},
		{"a{b,c,}.d", "ab.d", true},
		{"a{b,c,}.d", "ac.d", true},
		{"a{b,c,}.d", "a,.d", false},
		{"a{,b,,c,}.e", "a.e", true},
		{"a{,b,,c,}.e", "ab.e", true},
		{"a{,b,,c,}.e", "ac.e", true},
		{"a{,b,,c,}.e", "a,.e", false},

		{"{word,{also},this}.g", "word,this}.g", false},
		{"{word,{also},this}.g", "{also,this}.g", false},
		{"{word,{also},this}.g", "word.g", true},
		{"{word,{also},this}.g", "{also}.g", true},
		{"{word,{also},this}.g", "this.g", true},
		{"{{a,b},c}.k", "{{a,b},c}.k", false},
		{"{{a,b},c}.k", "{a,b}.k", false},
		{"{{a,b},c}.k", "a.k", true},
		{"{{a,b},c}.k", "b.k", true},
		{"{{a,b},c}.k", "c.k", true},
		{"{a,{b,c}}.l", "{a,{b,c}}.l", false},
		{"{a,{b,c}}.l", "{b,c}.l", false},
		{"{a,{b,c}}.l", "a.l", true},
		{"{a,{b,c}}.l", "b.l", true},
		{"{a,{b,c}}.l", "c.l", true},

		{"{},b}.h", "{},b}.h", true},

		{"{{,b,c{d}.i", "{{,b,c{d}.i", true},
		{"{{,b,c{d}.i", "{.i", false},
		{"{{,b,c{d}.i", "b.i", false},
		{"{{,b,c{d}.i", "c{d.i", false},
		{"{{,b,c{d}.i", ".i", false},

		{`{a\,b,cd}.txt`, "a,b.txt", true},
		{`{a\,b,cd}.txt`, "a.txt", false},
		{`{a\,b,cd}.txt`, "cd.txt", true},
		{`{e,\},f}.txt`, "e.txt", true},
		{`{e,\},f}.txt`, "}.txt", true},
		{`{e,\},f}.txt`, "f.txt", true},
		{`{g,\\,i}.txt`, "g.txt", true},
		{`{g,\\,i}.txt`, `\.txt`, true},
		{`{g,\\,i}.txt`, "i.txt", true},

		{"{some,a{*c,b}[ef]}.j", "some.j", true},
		{"{some,a{*c,b}[ef]}.j", "abe.j", true},
		{"{some,a{*c,b}[ef]}.j", "abf.j", true},
		{"{some,a{*c,b}[ef]}.j", "abg.j", false},
		{"{some,a{*c,b}[ef]}.j", "ace.j", true},
		{"{some,a{*c,b}[ef]}.j", "acf.j", true},
		{"{some,a{*c,b}[ef]}.j", "acg.j", false},
		{"{some,a{*c,b}[ef]}.j", "abce.j", true},
		{"{some,a{*c,b}[ef]}.j", "abcf.j", true},
		{"{some,a{*c,b}[ef]}.j", "abcg.j", false},
		{"{some,a{*c,b}[ef]}.j", "ae.j", false},
		{"{some,a{*c,b}[ef]}.j", ".j", false},

		{"{3..120}", "1", false},
		{"{3..120}", "3", true},
		{"{3..120}", "15", true},
		{"{3..120}", "60", true},
		{"{3..120}", "5a", false},
		{"{3..120}", "120", true},
		{"{3..120}", "121", false},
		{"{3..120}", "060", false},

		{"{aardvark..antelope}", "{aardvark..antelope}", true},
		{"{aardvark..antelope}", "a", false},
		{"{aardvark..antelope}", "aardvark", false},
		{"{aardvark..antelope}", "agreement", false},
		{"{aardvark..antelope}", "antelope", false},
		{"{aardvark..antelope}", "antimatter", false},
	}

	for _, tc := range tests {
		t.Run(tc.pattern+" vs "+tc.path, func(t *testing.T) {
			got := editorConfigGlobMatches(tc.pattern, tc.path)

			if got != tc.want {
				t.Fatalf("editorConfigGlobMatches(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
			}
		})
	}
}

func TestParseEditorConfigRoot(t *testing.T) {
	var ec EditorConfig

	if err := parseEditorConfig([]byte("root = true\n\n[*]\nindent_size = 2\n"), &ec); err != nil {
		t.Fatalf("parseEditorConfig: %v", err)
	}

	if !ec.Root {
		t.Fatal("Root = false, want true for `root = true` preamble")
	}
}

func TestParseEditorConfigIgnoresLeadingBOM(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantRoot bool
	}{
		{"root preamble", "\uFEFFroot = true\n\n[*]\nindent_size = 2\n", true},
		{"section first", "\uFEFF[*]\nindent_size = 2\n", false},
	}

	want := []EditorConfigSection{{Pattern: "*", IndentSize: 2}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ec EditorConfig

			if err := parseEditorConfig([]byte(tc.raw), &ec); err != nil {
				t.Fatalf("parseEditorConfig: %v", err)
			}

			if ec.Root != tc.wantRoot {
				t.Fatalf("Root = %v, want %v", ec.Root, tc.wantRoot)
			}

			if !reflect.DeepEqual(ec.Sections, want) {
				t.Fatalf("Sections = %#v, want %#v", ec.Sections, want)
			}
		})
	}
}

func TestNewEditorConfigResolverReadErrorIsEager(t *testing.T) {
	dir := t.TempDir()

	if err := os.Mkdir(filepath.Join(dir, ".editorconfig"), 0o755); err != nil {
		t.Fatalf("mkdir .editorconfig: %v", err)
	}

	if _, err := newEditorConfigResolver(dir, &probeMemo{}); err == nil {
		t.Fatal("newEditorConfigResolver: want error for an unreadable .editorconfig, got nil")
	}
}

func TestResolverAncestorAppliesToSubdir(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".editorconfig", "root = true\n[*]\nindent_size = 2\n")

	resolver, err := newEditorConfigResolver(dir, &probeMemo{})
	if err != nil {
		t.Fatalf("newEditorConfigResolver: %v", err)
	}

	got := resolveForFile(t, resolver, filepath.Join(dir, "apps", "api", "User.php"))
	if got.IndentSize != 2 {
		t.Fatalf("IndentSize = %d, want 2 (ancestor config should apply in subdir)", got.IndentSize)
	}
}

func TestResolverNearestWins(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".editorconfig", "root = true\n[*]\nindent_size = 2\nmax_line_length = 200\n")
	seedFileAt(t, dir, "apps/web/.editorconfig", "[*]\nindent_size = 8\n")

	resolver, err := newEditorConfigResolver(dir, &probeMemo{})
	if err != nil {
		t.Fatalf("newEditorConfigResolver: %v", err)
	}

	web := resolveForFile(t, resolver, filepath.Join(dir, "apps", "web", "Page.php"))
	if web.IndentSize != 8 {
		t.Errorf("web IndentSize = %d, want 8 (nearest config wins)", web.IndentSize)
	}

	if web.MaxLineLength != 200 {
		t.Errorf("web MaxLineLength = %d, want 200 (inherited from ancestor)", web.MaxLineLength)
	}

	sibling := resolveForFile(t, resolver, filepath.Join(dir, "apps", "api", "User.php"))
	if sibling.IndentSize != 2 {
		t.Errorf("sibling IndentSize = %d, want 2 (the web override must not leak)", sibling.IndentSize)
	}
}

func TestResolverNearestUnsetClearsAncestor(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".editorconfig", "root = true\n[*]\nindent_size = 2\nmax_line_length = 200\n")
	seedFileAt(t, dir, "apps/web/.editorconfig", "[*]\nindent_size = unset\n")

	resolver, err := newEditorConfigResolver(dir, &probeMemo{})
	if err != nil {
		t.Fatalf("newEditorConfigResolver: %v", err)
	}

	web := resolveForFile(t, resolver, filepath.Join(dir, "apps", "web", "Page.php"))
	if web.IndentSize != editorConfigCleared {
		t.Errorf("web IndentSize = %d, want cleared (a nearer config unset it)", web.IndentSize)
	}

	if web.MaxLineLength != 200 {
		t.Errorf("web MaxLineLength = %d, want 200 (unset must not clear an untouched key)", web.MaxLineLength)
	}

	sibling := resolveForFile(t, resolver, filepath.Join(dir, "apps", "api", "User.php"))
	if sibling.IndentSize != 2 {
		t.Errorf("sibling IndentSize = %d, want 2 (the web unset must not leak)", sibling.IndentSize)
	}
}

func TestResolverRootTrueStopsWalk(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".editorconfig", "[*]\nmax_line_length = 200\n")
	seedFileAt(t, dir, "inner/.editorconfig", "root = true\n[*]\nindent_size = 2\n")

	resolver, err := newEditorConfigResolver(dir, &probeMemo{})
	if err != nil {
		t.Fatalf("newEditorConfigResolver: %v", err)
	}

	got := resolveForFile(t, resolver, filepath.Join(dir, "inner", "file.php"))
	if got.IndentSize != 2 {
		t.Errorf("IndentSize = %d, want 2 (inner config)", got.IndentSize)
	}

	if got.MaxLineLength != 0 {
		t.Errorf("MaxLineLength = %d, want 0 (root=true must stop the walk before the outer config)", got.MaxLineLength)
	}
}

func TestResolverReadErrorInSubdirIsLazy(t *testing.T) {
	dir := t.TempDir()

	resolver, err := newEditorConfigResolver(dir, &probeMemo{})
	if err != nil {
		t.Fatalf("newEditorConfigResolver: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "sub", ".editorconfig"), 0o755); err != nil {
		t.Fatalf("mkdir sub/.editorconfig: %v", err)
	}

	if _, err := resolver.ResolveForFile(filepath.Join(dir, "sub", "file.php")); err == nil {
		t.Fatal("ResolveForFile: want error for an unreadable subdir .editorconfig, got nil")
	}
}

func TestResolverConcurrentResolveForFile(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".editorconfig", "root = true\n[*]\nindent_size = 2\n")
	seedFileAt(t, dir, "a/.editorconfig", "[*]\nindent_size = 4\n")
	seedFileAt(t, dir, "b/.editorconfig", "[*]\nindent_size = 8\n")

	resolver, err := newEditorConfigResolver(dir, &probeMemo{})
	if err != nil {
		t.Fatalf("newEditorConfigResolver: %v", err)
	}

	paths := []string{
		filepath.Join(dir, "a", "x.php"),
		filepath.Join(dir, "b", "y.php"),
		filepath.Join(dir, "c", "z.php"),
	}

	var wg sync.WaitGroup

	for range 8 {
		for _, path := range paths {
			wg.Go(func() {
				if _, err := resolver.ResolveForFile(path); err != nil {
					t.Errorf("ResolveForFile(%s): %v", path, err)
				}
			})
		}
	}

	wg.Wait()
}

func resolveForFile(t *testing.T, resolver *EditorConfigResolver, abs string) EditorConfigSection {
	t.Helper()

	section, err := resolver.ResolveForFile(abs)
	if err != nil {
		t.Fatalf("ResolveForFile(%s): %v", abs, err)
	}

	return section
}

func TestEditorConfigIndentSetsExplicit(t *testing.T) {
	cfg := config.Default()
	applyEditorConfigSection(cfg, EditorConfigSection{IndentSize: 2}, map[string]bool{})

	if !cfg.IndentExplicit {
		t.Fatal("IndentExplicit not set when EditorConfig provides indent_size")
	}

	if cfg.IndentWidth != 2 {
		t.Fatalf("IndentWidth = %d, want 2", cfg.IndentWidth)
	}
}
