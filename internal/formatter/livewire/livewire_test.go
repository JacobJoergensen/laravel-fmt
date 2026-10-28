package livewire

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	bladefmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/blade"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
)

func TestLooksLikeSFCRecognizesPHPOpener(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"bare php opener", "<?php\n$x = 1;", true},
		{"leading whitespace then opener", "\n  <?php\n$x = 1;", true},
		{"uppercase opener", "<?PHP\n$x = 1;", true},
		{"mixed case opener", "<?Php\n$x = 1;", true},
		{"short echo opener", "<?= $x ?>\n<div></div>", false},
		{"truncated opener", "<?ph", false},
		{"html first", "<div>hi</div>", false},
		{"blade directive first", "@if ($x)\nhi\n@endif", false},
		{"empty", "", false},
		{"whitespace only", "   \n\t\n", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LooksLikeSFC([]byte(tc.src)); got != tc.want {
				t.Fatalf("LooksLikeSFC(%q) = %v, want %v", tc.src, got, tc.want)
			}
		})
	}
}

func TestFormatMatchesGolden(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".blade.php", false) {
		t.Run(name, func(t *testing.T) {
			input := formattertest.ReadFixture(t, name+".blade.php")
			want := formattertest.ReadFixture(t, name+".formatted.blade.php")

			got := mustFormat(t, input, config.Default())

			if !bytes.Equal(got, want) {
				t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
			}
		})
	}
}

func TestFormatIsIdempotent(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".blade.php", false) {
		t.Run(name, func(t *testing.T) {
			input := formattertest.ReadFixture(t, name+".blade.php")

			once := mustFormat(t, input, config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("Format is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
			}
		})
	}
}

func TestFormatLeavesAViewWithoutAComponentToBlade(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"xml prolog echo", "<?php echo '<?xml version=\"1.0\"?>'; ?>\n<urlset>\n@foreach ($items as $item)\n<url>{{ $item }}</url>\n@endforeach\n</urlset>\n"},
		{"import only", "<?php use App\\Enums\\Status; ?>\n<div>{{ Status::Active->label() }}</div>\n"},
	}

	cfg := config.Default()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, err := bladefmt.Format([]byte(tc.src), cfg)
			if err != nil {
				t.Fatalf("blade Format returned error: %v", err)
			}

			got := mustFormat(t, []byte(tc.src), cfg)
			if !bytes.Equal(got, want) {
				t.Fatalf("got:\n%s\nwant the Blade output:\n%s", got, want)
			}

			if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent:\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatRejectsInvalidSource(t *testing.T) {
	out, err := Format([]byte("<?php\nnew Foo(...);\n?>\n<div></div>\n"), config.Default())

	if err == nil {
		t.Fatal("Format accepted invalid source; want a parse error")
	}

	if out != nil {
		t.Fatalf("Format returned non-nil bytes on parse error: %q", out)
	}
}

func TestFormatKeepsANoBreakSpaceTemplate(t *testing.T) {
	src := []byte("<?php\n\nnew class extends Component {};\n?>\n\n\u00a0\n")

	got := mustFormat(t, src, config.Default())

	if !bytes.Equal(got, src) {
		t.Fatalf("got %q, want the input unchanged", got)
	}
}

func mustFormat(t *testing.T, src []byte, cfg *config.File) []byte {
	t.Helper()

	out, err := Format(src, cfg)
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}

	return out
}

// livewireLifted lists the <script> and <style> tags starting a line at column
// 0, the candidates Livewire 4's SingleFileParser lifts from a view, after
// removing the @script, @assets and @verbatim blocks it skips.
func livewireLifted(src []byte) []string {
	view := regexp.MustCompile(`(?s)<\?php\s*.*?\s*\?>`).ReplaceAll(src, nil)

	for _, block := range []string{"script", "assets", "verbatim"} {
		view = regexp.MustCompile(`(?s)@`+block+`\s*.*?@end`+block).ReplaceAll(view, nil)
	}

	matches := regexp.MustCompile(`(?:^|\n)<(script|style)\b`).FindAllSubmatch(view, -1)
	names := make([]string, 0, len(matches))

	for _, m := range matches {
		names = append(names, string(m[1]))
	}

	return names
}

func TestLivewireViewKeepsTheColumnLivewireLiftsATagFrom(t *testing.T) {
	const head = "<?php\n\nuse Livewire\\Component;\n\nnew class extends Component\n{\n};\n?>\n\n"

	cases := map[string]string{
		"lifted script nested in markup":      "<div>\n<script>\nrun();\n</script>\n</div>\n",
		"indented root script stays":          "<div>x</div>\n    <script>\n        run();\n    </script>\n",
		"lifted style in a directive":         "<div>\n@if ($a)\n<style>\np { color: red; }\n</style>\n@endif\n</div>\n",
		"root script glued after a directive": "@if ($a)\n@endif<script>run();</script>\n",
	}

	for name, view := range cases {
		t.Run(name, func(t *testing.T) {
			src := []byte(head + view)

			got, err := Format(src, config.Default())
			if err != nil {
				t.Fatalf("format: %v", err)
			}

			if want, have := livewireLifted(src), livewireLifted(got); !slices.Equal(want, have) {
				t.Fatalf("Livewire lifts %v from the source but %v from the output:\n%s", want, have, got)
			}

			formattertest.CheckIdempotent(t, func(b []byte) ([]byte, error) { return Format(b, config.Default()) }, got)
		})
	}
}

// Livewire removes @script and @assets blocks before lifting a <script>, so a
// tag inside one is free to indent.
func TestLivewireScriptBlockBodyIndents(t *testing.T) {
	src := []byte("<?php\n\n$x = 1;\n?>\n\n<div>\n@script\n<script>\nrun();\n</script>\n@endscript\n</div>\n")

	got, err := Format(src, config.Default())
	if err != nil {
		t.Fatalf("format: %v", err)
	}

	if !bytes.Contains(got, []byte("\n    @script\n        <script>\n")) {
		t.Fatalf("the @script body did not indent under its directive:\n%s", got)
	}
}

func TestLivewireViewUsesDeclaredBlockDirectives(t *testing.T) {
	cfg := config.Default()
	cfg.Blade.BlockDirectives = []config.BladeBlockDirective{{Open: "feature"}}

	got, err := Format([]byte("<?php\n\n$x = 1;\n?>\n\n@feature('a')\n<p>A</p>\n@endfeature\n"), cfg)
	if err != nil {
		t.Fatalf("format: %v", err)
	}

	if !bytes.Contains(got, []byte("@feature('a')\n    <p>A</p>\n@endfeature\n")) {
		t.Fatalf("the declared block did not nest in the view:\n%s", got)
	}
}

func TestFormatBlankLineOptionsStayInTheirHalf(t *testing.T) {
	const class = "<?php\n\nuse Livewire\\Component;\n\nnew class extends Component\n{\n    public function go(): void\n    {\n        $a = 1;\n%s        if ($a) {\n            $a = 2;\n        }\n    }\n};\n?>\n\n"
	const view = "<div>\n    <p>A</p>\n%s    @if ($a)\n        <p>B</p>\n    @endif\n</div>\n"

	src := fmt.Sprintf(class, "") + fmt.Sprintf(view, "")

	bladeOnly := config.Default()
	bladeOnly.PHP.BlankLineBeforeStatements = nil
	bladeOnly.Blade.BlankLineBeforeDirectives = []string{"if"}

	phpOnly := config.Default()
	phpOnly.PHP.BlankLineBeforeStatements = []string{config.StmtIf}

	cases := []struct {
		name string
		cfg  *config.File
		want string
	}{
		{"blade option", bladeOnly, fmt.Sprintf(class, "") + fmt.Sprintf(view, "\n")},
		{"php option", phpOnly, fmt.Sprintf(class, "\n") + fmt.Sprintf(view, "")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(src), tc.cfg)
			if string(got) != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}

			formattertest.CheckIdempotent(t, func(b []byte) ([]byte, error) { return Format(b, tc.cfg) }, got)
		})
	}
}
