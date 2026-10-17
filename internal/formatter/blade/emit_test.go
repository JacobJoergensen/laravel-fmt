package blade

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
)

func TestFormatMatchesGolden(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".blade.php", true) {
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
	for _, name := range formattertest.GoldenFixtureNames(t, ".blade.php", true) {
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

func TestFormatBlockInlineBodyKeepsInnerSpace(t *testing.T) {
	pad := bytes.Repeat([]byte("0"), 57)
	src := append([]byte("<div "), pad...)
	src = append(src, []byte("@if(A%'b')c=\"d\"e>@ A0!</div>")...)

	once := mustFormat(t, src, config.Default())
	twice := mustFormat(t, once, config.Default())

	if !bytes.Equal(twice, once) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
	}

	if !bytes.Contains(once, []byte("@ A0!")) {
		t.Fatalf("inner space dropped from wrapped-tag body:\n%s", once)
	}
}

func TestFormatInlineElementLooseChildrenExplode(t *testing.T) {
	src := []byte(`<div>
    <span class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border bg-surface-sunken text-zinc-500">
        <flux:icon.fire class="size-4" />
        {{ $label }}
    </span>
</div>
`)

	want := `<div>
    <span class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border bg-surface-sunken text-zinc-500">
        <flux:icon.fire class="size-4" />
        {{ $label }}
    </span>
</div>
`

	got := mustFormat(t, src, config.Default())

	if bytes.Contains(got, []byte("> <")) {
		t.Fatalf("inline children collapsed onto the open tag's `>` line:\n%s", got)
	}

	if string(got) != want {
		t.Fatalf("unexpected layout.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, config.Default())
	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatInlineElementTightChildrenStayFlat(t *testing.T) {
	src := []byte(`<div>
    <a class="font-medium text-text no-underline hover:text-primary-600 focus-visible:outline-none">before<b>x</b>after</a>
</div>
`)

	got := mustFormat(t, src, config.Default())

	if !bytes.Contains(got, []byte("before<b>x</b>after")) {
		t.Fatalf("glued inline run was split (whitespace injected):\n%s", got)
	}

	twice := mustFormat(t, got, config.Default())
	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatGluedInlineBodySurvivesWrappedOpenTag(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"echo body",
			"<div>\n" +
				"    <a\n" +
				"        href=\"{{ route('accounts.show', $account) }}\"\n" +
				"        wire:navigate\n" +
				"        class=\"font-medium text-primary-600 underline-offset-2 hover:underline dark:text-primary-400\"\n" +
				"    >{{ $account->full_name }}</a>\n" +
				"</div>\n",
			">{{ $account->full_name }}</a>",
		},
		{
			"text body in a component",
			"<div>\n" +
				"    <flux:button\n" +
				"        variant=\"ghost\"\n" +
				"        size=\"sm\"\n" +
				"        icon=\"document-arrow-down\"\n" +
				"        icon:variant=\"outline\"\n" +
				"        class=\"font-medium text-primary-600 underline-offset-2 hover:underline\"\n" +
				"    >Export PDF</flux:button>\n" +
				"</div>\n",
			">Export PDF</flux:button>",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if !bytes.Contains(got, []byte(tc.want)) {
				t.Fatalf("glued inline body lost its `>` hug when the open tag wrapped:\n%s", got)
			}

			twice := mustFormat(t, got, config.Default())
			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatComponentKeepsEdgeWhitespace(t *testing.T) {
	cases := map[string]string{
		"<flux:badge><div/>0 </flux:badge>\n": "<flux:badge><div />0 </flux:badge>\n",
		"<flux:badge><c/>0 </flux:badge>\n":   "<flux:badge><c />0 </flux:badge>\n",
		"<div><div/>0 </div>\n":               "<div><div />0</div>\n",
	}

	for src, want := range cases {
		got := mustFormat(t, []byte(src), config.Default())

		if string(got) != want {
			t.Errorf("Format(%q) = %q, want %q", src, got, want)
		}

		twice := mustFormat(t, got, config.Default())
		if !bytes.Equal(twice, got) {
			t.Errorf("not idempotent for %q.\n=== once ===\n%s\n=== twice ===\n%s", src, got, twice)
		}
	}
}

func TestFormatRawPhpAttrIslandNormalizesLineEndings(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"CRLF inside a short tag", "<?php ?><p><A <?\r\n?>/>0", "<?php ?>\n<p><A <?\n?> />0</p>\n"},
		{"CRLF inside a full tag", "<?php ?><p><A <?php\r\n?>/>0", "<?php ?>\n<p><A <?php\n?> />0</p>\n"},
		{"LF is already canonical", "<?php ?><p><A <?\n?>/>0", "<?php ?>\n<p><A <?\n?> />0</p>\n"},
		{"a CR run before the break collapses in one pass", "<?php ?><p><A <?\r\r\n?>/>0", "<?php ?>\n<p><A <?\n?> />0</p>\n"},
		{"a lone CR is content, not a terminator", "<?php ?><p><A <?='a\rb'?>/>0", "<?php ?>\n<p><A <?='a\rb'?> />0</p>\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.want {
				t.Fatalf("Format output differs.\n=== got ===\n%q\n=== want ===\n%q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestFormatRawPhpIslandsStructural(t *testing.T) {
	src := []byte(`<div>
<?php if ($x): ?>
<p>a</p>
<?php elseif ($y): ?>
<p>b</p>
<?php else: ?>
<p>c</p>
<?php endif; ?>
</div>
`)

	want := `<div>
    <?php if ($x): ?>
        <p>a</p>
    <?php elseif ($y): ?>
        <p>b</p>
    <?php else: ?>
        <p>c</p>
    <?php endif; ?>
</div>
`

	got := mustFormat(t, src, config.Default())

	if bytes.Contains(got, []byte("?> <?php")) {
		t.Fatalf("raw php islands jammed onto one line:\n%s", got)
	}

	if string(got) != want {
		t.Fatalf("unexpected layout.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, config.Default())
	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatRawPhpIslandKeepsTheCommentAfterItsLastStatement(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"on one line", "<p>x</p>\n<?php $a = 1; // note ?>\n", "<p>x</p>\n<?php $a = 1; // note ?>\n"},
		{"over lines", "<p>x</p>\n<?php\n$a = 1;\n// note\n?>\n", "<p>x</p>\n<?php\n    $a = 1;\n    // note\n?>\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent: %q -> %q", got, twice)
			}
		})
	}
}

func TestFormatLivewireBlockDirectivesIndentBody(t *testing.T) {
	src := []byte(`<div>
@island(name: 'x')
<p>a</p>
@endisland
@placeholder
<p>b</p>
@endplaceholder
@teleport('body')
<p>c</p>
@endteleport
</div>
`)

	want := `<div>
    @island(name: 'x')
        <p>a</p>
    @endisland
    @placeholder
        <p>b</p>
    @endplaceholder
    @teleport('body')
        <p>c</p>
    @endteleport
</div>
`

	got := mustFormat(t, src, config.Default())

	if string(got) != want {
		t.Fatalf("livewire block bodies not indented.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, config.Default())
	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatSectionBodyIndentsForEveryCloser(t *testing.T) {
	for _, closer := range []string{"endsection", "show", "stop", "append", "overwrite"} {
		t.Run(closer, func(t *testing.T) {
			src := []byte("@section('side')\n<p>x</p>\n@" + closer + "\n")
			want := "@section('side')\n    <p>x</p>\n@" + closer + "\n"

			got := mustFormat(t, src, config.Default())
			if string(got) != want {
				t.Fatalf("got %q, want %q", got, want)
			}

			twice := mustFormat(t, got, config.Default())
			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent: %q then %q", got, twice)
			}
		})
	}
}

func TestFormatQuotesAnUnquotedValueWithASlash(t *testing.T) {
	src := []byte("<a href=/docs>d</a>\n<x-icon name=check/>\n")
	want := "<a href=\"/docs\">d</a>\n<x-icon name=\"check\" />\n"

	got := mustFormat(t, src, config.Default())
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	twice := mustFormat(t, got, config.Default())
	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent: %q then %q", got, twice)
	}
}

func TestFormatMultilineInlineTextReindented(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"plain multi-line text",
			"<div>\nCreated here\nand more text\n</div>\n",
			"<div>\n    Created here\n    and more text\n</div>\n",
		},
		{
			"multi-line text with interior echoes",
			"<div class=\"meta-line\">\nRow {{ $r }}\n· {{ $col }}\n</div>\n",
			"<div class=\"meta-line\">\n    Row {{ $r }}\n    · {{ $col }}\n</div>\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.in), config.Default())

			if string(got) != tc.want {
				t.Fatalf("continuation not re-homed.\n=== got ===\n%s\n=== want ===\n%s", got, tc.want)
			}

			twice := mustFormat(t, got, config.Default())
			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

// HTML's input stream turns a lone CR into LF, so a template with one formats
// like its LF twin.
func TestFormatReadsALoneCRAsItsLFTwin(t *testing.T) {
	cases := map[string]string{
		"an inline element's body":           "0<strong>0\r</strong>\n",
		"an unclosed element in an inline":   "0<strong>0<p>\r</strong>\n",
		"text in an inline element":          "<div><span>a\rb</span></div>\n",
		"text beside an echo":                "<p>a\rb {{ $x }}</p>\n",
		"a blank line in text":               "<p>a\r\rb</p>\n",
		"a blank line between children":      "<div>\r<p>a</p>\r\r<p>b</p>\r</div>\n",
		"a blank line in a directive body":   "@if($a)\r<p>x</p>\r\r<p>y</p>\r@endif\n",
		"an attribute value with an echo":    "<div class=\"a\r  {{ $x }} b\"></div>\n",
		"a multi-line attribute value":       "<div x-data=\"{\r  a: 1 }\"></div>\n",
		"a glued body led by text":           "<div>\r!@A</div>\n",
		"a glued body ending in text":        "<div >@A!\r</div>\n",
		"a re-homed glued body":              "<div><a>\r0</a>@A@</div>\n",
		"an attribute in a re-homed body":    "<div>\n<a title=\"x\r   y\">0</a>@A@\n</div>\n",
		"an escaped echo body":               "<p>@{{\r}}</p>\n",
		"an escaped echo beside a directive": "<li>@A!<p>@{{\r}}",
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())
			lf := strings.ReplaceAll(src, "\r", "\n")

			if want := mustFormat(t, []byte(lf), config.Default()); !bytes.Equal(got, want) {
				t.Fatalf("a lone CR formats unlike its LF twin.\n=== CR ===\n%q\n=== LF ===\n%q", got, want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

// A lone CR right after an echo renders as nothing: Blade doubles a following
// LF or CRLF but not a lone CR, and PHP eats it after the `?>`. Checked against
// illuminate/view's BladeCompiler and PHP's lexer.
func TestFormatLoneCRAfterAnEchoRendersAsNothing(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"in text":                         {"<p>{{ $a }}\rb</p>", "<p>{{ $a }}b</p>\n"},
		"after a raw echo":                {"<p>{!! $a !!}\rb</p>", "<p>{!! $a !!}b</p>\n"},
		"in an inline run":                {"<div>x {{ $a }}\rb <b>y</b></div>", "<div>x {{ $a }}b <b>y</b></div>\n"},
		"in an attribute":                 {"<div class=\"a {{ $x }}\r  b\"></div>", "<div class=\"a {{ $x }}  b\"></div>\n"},
		"in an unquoted value":            {"<span a={{''}}\rb>x</span>", "<span a={{''}}b>x</span>\n"},
		"ending an unquoted value":        {"<span a={{''}}\r>x\ny</span>", "<span a={{''}}\n    >x\n    y</span>\n"},
		"two echoes in an unquoted value": {"<span a={{''}}\r{{''}}\rb>x</span>", "<span a={{''}}{{''}}b>x</span>\n"},
		"ending a script":                 {"<script>let a = {{ $a }}\r</script>", "<script>\n    let a = {{ $a }}\n</script>\n"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

// Blade compiles echoes, directive arguments, `@php` and `<?php ?>` into PHP,
// where a lone CR is a byte of data: `md5("a<CR>b")` and `md5("a<LF>b")`
// differ.
func TestFormatKeepsALoneCRInsidePHP(t *testing.T) {
	const php = `md5("a` + "\r" + `b")`

	cases := map[string]string{
		"an echo in pre":                 "<pre>{{ " + php + " }}</pre>",
		"an echo in textarea":            "<textarea>{{ " + php + " }}</textarea>",
		"a raw PHP block in pre":         "<div><pre>\n<?php echo " + php + "; ?></pre></div>",
		"an echo in a glued body":        "<p>a<b>\n{{ " + php + " }}</b></p>",
		"an echo in script":              "<script>\n  var h = \"{{ " + php + " }}\";\n</script>",
		"a directive argument in script": "<script>\n  @json(" + php + ")\n</script>",
		"an echo in style":               "<style>\n  {{ " + php + " }}\n  .a { color: red; }\n</style>",
		"a component attribute":          "<x-card title=\"a\rb\">x</x-card>",
		"an echo in a re-homed body":     "<div>{{ " + php + " }}<b>0</b>@A@\n</div>",
		"a component in a re-homed body": "<div>\n<x-a title=\"a\rb\">0</x-a>@A@\n</div>",
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if !bytes.Contains(got, []byte("a\rb")) {
				t.Fatalf("the CR inside PHP was rewritten: %q", got)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestFormatCRInsideAnEchoIsNotALineBreak(t *testing.T) {
	const want = "<div>{{ 'x\ry' }}@A</div>\n"

	if got := mustFormat(t, []byte("<div>{{ \"x\ry\" }}@A</div>"), config.Default()); string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormatWordGluedToAWrappedTagStaysGlued(t *testing.T) {
	cfg := config.Default()
	cfg.PrintWidth = 40

	src := `Total:<strong><span class="font-semibold text-gray-900">{{ $order->total }}</span>EUR</strong>`

	once := mustFormat(t, []byte(src), cfg)

	if !bytes.HasPrefix(once, []byte("Total:<strong")) {
		t.Fatalf("the word came off the element:\n%s", once)
	}

	if twice := mustFormat(t, once, cfg); !bytes.Equal(twice, once) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
	}
}

func TestFormatOmittedEndTagFormatsLikeItsTwin(t *testing.T) {
	cases := map[string]struct{ omitted, closed string }{
		"dd at the end":            {"<dd><a>0</a>@A@", "<dd><a>0</a>@A@</dd>"},
		"dd with a broken body":    {"<dd><a>\n0</a>@A@", "<dd><a>\n0</a>@A@</dd>"},
		"li at the end":            {"<li><a>0</a>@A@", "<li><a>0</a>@A@</li>"},
		"li closed by the next li": {"<ul><li><a>0</a>@A@<li>b</ul>", "<ul><li><a>0</a>@A@</li><li>b</li></ul>"},
		"p closed by a block":      {"<div><p><a>0</a>@A@<div>x</div></div>", "<div><p><a>0</a>@A@</p><div>x</div></div>"},
		"li with no glue to keep":  {"<ul><li>a<li>b</ul>", "<ul><li>a</li><li>b</li></ul>"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.omitted), config.Default())

			if want := mustFormat(t, []byte(tc.closed), config.Default()); !bytes.Equal(got, want) {
				t.Fatalf("formats unlike its closed twin.\n=== omitted ===\n%s\n=== closed ===\n%s", got, want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatRawDirectiveArgKeepsItsParenUnlessAComment(t *testing.T) {
	cases := map[string]string{
		"a hash in a string":          `@for($s = "#"; $s; )`,
		"a hash in a padded argument": `@for( $s = "#"; $s; )`,
		"an attribute opener":         `@for($f = #[A] fn () => 1; $f; )`,
		"a hash in a foreach":         `@foreach("#" as $x)`,
		"a real line comment":         "@for($i = 0; $i < 3; $i++ // c\n)x@endfor",
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())
			comment := strings.Contains(src, "//")

			if inline := !bytes.Contains(got, []byte("\n)")); inline == comment {
				t.Fatalf("the paren moved wrongly:\n%s", got)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

// An unquoted value that holds both quotes cannot be quoted, so a `/` ending it
// stays spaced off the tag's end: written against the `>` it would read as a
// self-closing tag on the next parse.
func TestFormatKeepsAnUnquotedSlashOffTheTagEnd(t *testing.T) {
	got := mustFormat(t, []byte(`<td title=a'b"/ >x</td>`), config.Default())

	if want := "<td title=a'b\"/ >x</td>\n"; string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
	}
}

func TestFormatKeepsAGluedBodyEndingInAnOpenChildOfAnotherName(t *testing.T) {
	got := mustFormat(t, []byte("<option>Small<dd>x"), config.Default())

	if want := "<option>Small<dd>x</option>\n"; string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
	}
}

func TestFormatKeepsAnAuthorBrokenEchoThatDoesNotFitOnOneLine(t *testing.T) {
	src := "<div>\n{{\nbar($aaaaaaaaaaaaaaaaaaaa, $bbbbbbbbbbbbbbbbbbbbbbbbbb, $cccccccccccccccccccccccccc, $dddddddddddddddddddddddddd, $eeee)\n}}\n</div>\n"
	want := "<div>\n    {{\n        bar($aaaaaaaaaaaaaaaaaaaa, $bbbbbbbbbbbbbbbbbbbbbbbbbb, $cccccccccccccccccccccccccc, $dddddddddddddddddddddddddd, $eeee)\n    }}\n</div>\n"

	got := mustFormat(t, []byte(src), config.Default())

	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
	}
}

func TestFormatGlueDoesNotFreezeItsSiblings(t *testing.T) {
	src := "<div>\n<div class=\"aaaaaaaaaaaa bbbbbbbbbbbbbbbbbb ccccccccccccccccc dddddddddddddddddddd eeeeeeeeeeeeeeeeeeee fffffffffff\" id=\"foo\">\n      <span>x</span>\n  @if($a)\n<i>y</i>\n     @endif\n</div>\nRead the <a href=\"#\">docs</a>.\n</div>\n"
	want := "<div>\n    <div\n        class=\"aaaaaaaaaaaa bbbbbbbbbbbbbbbbbb ccccccccccccccccc dddddddddddddddddddd eeeeeeeeeeeeeeeeeeee fffffffffff\"\n        id=\"foo\"\n    >\n        <span>x</span>\n        @if ($a)\n            <i>y</i>\n        @endif\n    </div>\n    Read the <a href=\"#\">docs</a>.\n</div>\n"

	got := mustFormat(t, []byte(src), config.Default())

	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
	}
}

func TestFormatKeepsTheLinesOfAValueWhoseWhitespaceRenders(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"plain value": {
			"<div>\n<a title=\"one\n  two\">k</a>\n</div>\n",
			"<div>\n    <a\n        title=\"one\n  two\"\n    >k</a>\n</div>\n",
		},
		"value with an echo": {
			"<div>\n<a title=\"one\n{{ $two }} three\">k</a>\n</div>\n",
			"<div>\n    <a\n        title=\"one\n{{ $two }} three\"\n    >k</a>\n</div>\n",
		},
		"component prop": {
			"<div>\n<x-foo title=\"one\n   two\">k</x-foo>\n</div>\n",
			"<div>\n    <x-foo\n        title=\"one\n   two\"\n    >k</x-foo>\n</div>\n",
		},
		"class list follows the tag": {
			"<div>\n<a class=\"one\ntwo\">k</a>\n</div>\n",
			"<div>\n    <a\n        class=\"one\n        two\"\n    >k</a>\n</div>\n",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestFormatKeepsADirectiveBodyGluedToItsMarkers(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"at the top level": {
			"Day {{ $day }}@if($rev)(Revision {{ $rev }})@endif\n",
			"Day {{ $day }}@if ($rev)(Revision {{ $rev }})@endif\n",
		},
		"glued to a branch": {
			"@if ($a)<b>x</b>@else{{ $b }}@endif\n",
			"@if ($a)<b>x</b>@else{{ $b }}@endif\n",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestFormatKeepsAnAuthoredOneLineAttributeBlockInABrokenTag(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"kept on its line": {
			"<div class=\"aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd eeeeeeeeee ffffffffff gggggggggg\" @if ($sortable) wire:sort=\"reorder\" @else data-fixed @endif></div>\n",
			"<div\n    class=\"aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd eeeeeeeeee ffffffffff gggggggggg\"\n    @if ($sortable) wire:sort=\"reorder\" @else data-fixed @endif\n></div>\n",
		},
		"broken as authored": {
			"<div class=\"aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd eeeeeeeeee ffffffffff gggggggggg\" @if ($sortable)\nwire:sort=\"reorder\" @endif></div>\n",
			"<div\n    class=\"aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd eeeeeeeeee ffffffffff gggggggggg\"\n    @if ($sortable)\n        wire:sort=\"reorder\"\n    @endif\n></div>\n",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestFormatNeverEndsATagInAPHPCloseTag(t *testing.T) {
	cases := map[string]string{
		"an attribute named ?":       "<?<p ? >x</p>\n",
		"an attribute name ending ?": "<p a? >x</p>\n",
		"a doctype ending ?":         "<!DOCTYPE html? >\n",
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if bytes.Contains(got, []byte("?>")) {
				t.Fatalf("the formatter wrote a `?>`: %q", got)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestFormatUnclosedPHPOpenWithAHiddenCloseTagStaysVerbatim(t *testing.T) {
	cases := map[string]string{
		"a comment ended by a lone CR": "<?@#\r\"?>0\n",
		"a string holding markup":      "<? \"\n<p   class=\"b a\">x</p> ?>\n",
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if !bytes.Equal(got, []byte(src)) {
				t.Fatalf("the island was rewritten.\n=== got ===\n%q\n=== want ===\n%q", got, src)
			}
		})
	}

	implied := mustFormat(t, []byte("<p><?\"?>\n"), config.Default())
	if twice := mustFormat(t, implied, config.Default()); !bytes.Equal(twice, implied) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", implied, twice)
	}

	// With no `?>` ahead nothing can be exposed, so what follows is formatted.
	got := mustFormat(t, []byte("<? <p   class=\"x\">y</p>\n"), config.Default())
	if !bytes.Contains(got, []byte(`<p class="x">y</p>`)) {
		t.Fatalf("the markup after the `<?` was not formatted: %q", got)
	}
}

func TestFormatEmptinessSensitiveElementStaysGlued(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"empty-gated div keeps glued content (short tag)",
			`<div class="[&:not(:empty)]:pb-2 flex"><slot name="heading"></slot></div>` + "\n",
			`<div class="[&:not(:empty)]:pb-2 flex"><slot name="heading"></slot></div>` + "\n",
		},
		{
			"empty-gated div keeps glued content while the open tag wraps",
			`<div id="heading" class="[&:not(:empty)]:pb-2 text-sm font-medium text-zinc-800 dark:text-white padding-classes-here [&:not(:empty)+div]:font-normal"><slot name="heading"></slot></div>` + "\n",
			"<div\n    id=\"heading\"\n    class=\"[&:not(:empty)]:pb-2 text-sm font-medium text-zinc-800 dark:text-white padding-classes-here [&:not(:empty)+div]:font-normal\"\n><slot name=\"heading\"></slot></div>\n",
		},
		{
			"plain div (no :empty gate) still expands its child",
			`<div class="flex gap-2"><slot name="heading"></slot></div>` + "\n",
			"<div class=\"flex gap-2\">\n    <slot name=\"heading\"></slot>\n</div>\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.in), config.Default())

			if string(got) != tc.want {
				t.Fatalf("output mismatch.\n=== got ===\n%s\n=== want ===\n%s", got, tc.want)
			}

			twice := mustFormat(t, got, config.Default())
			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatWhitespaceSensitiveBodyReHome(t *testing.T) {
	t.Run("structural body re-homes under a re-indenting wrapper", func(t *testing.T) {
		in := "@if ($cond)\n<div class=\"flex\">\n    <span>a</span>@if ($x)<b>c</b>@endif\n    @if ($y)\n        <b>d</b>\n    @endif\n</div>\n@endif\n"
		want := "@if ($cond)\n    <div class=\"flex\">\n        <span>a</span>@if ($x)<b>c</b>@endif\n        @if ($y)\n            <b>d</b>\n        @endif\n    </div>\n@endif\n"

		got := mustFormat(t, []byte(in), config.Default())
		if string(got) != want {
			t.Fatalf("body not re-homed.\n=== got ===\n%s\n=== want ===\n%s", got, want)
		}

		twice := mustFormat(t, got, config.Default())
		if !bytes.Equal(twice, got) {
			t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
		}
	})

	t.Run("body with a multi-line string stays verbatim", func(t *testing.T) {
		in := "@if ($cond)\n<div class=\"flex\">\n<span>a</span>@if ($x)<b>c</b>@endif\n<a data-x=\"l1\nl2\">k</a>\n</div>\n@endif\n"

		got := mustFormat(t, []byte(in), config.Default())
		if !bytes.Contains(got, []byte("data-x=\"l1\nl2\"")) {
			t.Fatalf("multi-line string was corrupted by re-home:\n%s", got)
		}

		twice := mustFormat(t, got, config.Default())
		if !bytes.Equal(twice, got) {
			t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
		}
	})
}

func TestFormatRawPhpIslandGluedStaysVerbatim(t *testing.T) {
	src := []byte("<div><span>x<?php echo $y; ?></span></div>\n")

	got := mustFormat(t, src, config.Default())

	if !bytes.Contains(got, []byte("<span>x<?php echo $y; ?></span>")) {
		t.Fatalf("glued raw php island was split (whitespace injected):\n%s", got)
	}

	twice := mustFormat(t, got, config.Default())
	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// In CSS a `\` before a line break inside a string drops the break, and the
// next line's leading whitespace is part of the string.
func TestFormatStyleStringContinuationKeepsItsLine(t *testing.T) {
	const continuation = "b\"; }\n"

	src := "<div>\n<style>\n{{ $x }}\n.a::before { content: \"a\\\n" + continuation + "</style>\n</div>\n"
	got := mustFormat(t, []byte(src), config.Default())

	if !bytes.Contains(got, []byte("\n"+continuation)) {
		t.Fatalf("the continuation line was re-indented:\n%s", got)
	}
}

func TestFormatStyleBladeDirectivesStayVerbatim(t *testing.T) {
	src := []byte("<style>\n" +
		"@if($x === '}')\n" +
		".a { color: red; }\n" +
		"@endif\n" +
		".b { color: blue; }\n" +
		"</style>\n")

	got := mustFormat(t, src, config.Default())

	if !bytes.Contains(got, []byte("@if($x === '}')")) {
		t.Fatalf("Blade @if condition mangled (space or quote rewrite):\n%s", got)
	}

	if bytes.Contains(got, []byte(`"}"`)) {
		t.Fatalf("CSS formatter rewrote the PHP quotes inside @if:\n%s", got)
	}

	if bytes.Contains(got, []byte("@if($x === '}') .a")) || bytes.Contains(got, []byte("@endif .b")) {
		t.Fatalf("directive merged onto the selector line:\n%s", got)
	}

	twice := mustFormat(t, got, config.Default())
	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatStylePureCSSStillFormats(t *testing.T) {
	src := []byte("<style>\n@page{margin:0}\n.a{color:red}\n</style>\n")

	got := mustFormat(t, src, config.Default())

	if !bytes.Contains(got, []byte("    @page {")) || !bytes.Contains(got, []byte("    .a {")) {
		t.Fatalf("pure CSS <style> body was not formatted:\n%s", got)
	}
}

func TestPageStyleScanNoPanicOnInvalidUTF8(t *testing.T) {
	src := append([]byte("<style>x"), bytes.Repeat([]byte{0x8d}, 30)...)
	src = append(src, []byte("-a{white-space:pre}</style><div class=a></div>")...)

	got := mustFormat(t, src, config.Default())

	if again := mustFormat(t, got, config.Default()); !bytes.Equal(again, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, again)
	}
}

func TestPageStyleWhitespaceClassPreservesContent(t *testing.T) {
	src := []byte("<style>\n.notes { white-space: pre }\n</style>\n<div class=\"notes\">a\n    b</div>\n")

	got := mustFormat(t, src, config.Default())

	if !bytes.Contains(got, []byte("a\n    b")) {
		t.Fatalf("white-space:pre body was reflowed (scan failed to detect the class):\n%s", got)
	}
}

func TestFormatBoundAttrMultilineArrayRehomed(t *testing.T) {
	src := []byte(`<div>
    <x-thing :items="[
        'a' => 1,
        'b' => 2,
    ]" class="p-4">x</x-thing>
</div>
`)

	got := mustFormat(t, src, config.Default())

	if bytes.Contains(got, []byte("\n'a' =>")) || bytes.Contains(got, []byte("\n]\"")) {
		t.Fatalf("bound-attr array dumped flush-left:\n%s", got)
	}

	if !bytes.Contains(got, []byte("\n        'a' => 1,")) || !bytes.Contains(got, []byte("\n    ]\" class=")) {
		t.Fatalf("bound-attr array not re-homed under the tag:\n%s", got)
	}

	twice := mustFormat(t, got, config.Default())
	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatBoundAttrMultilineStringLiteralRaw(t *testing.T) {
	src := []byte("<div>\n    <x-thing :items=\"[\n        'a' => 'line one\nline two',\n        'b' => 2,\n    ]\">x</x-thing>\n</div>\n")

	got := mustFormat(t, src, config.Default())

	if !bytes.Contains(got, []byte("'line one\nline two'")) {
		t.Fatalf("multi-line string literal was mutated (re-indented):\n%s", got)
	}

	twice := mustFormat(t, got, config.Default())
	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatMultilineAttrEchoPartsRehomed(t *testing.T) {
	t.Run("continuation and closing quote re-home under the tag", func(t *testing.T) {
		in := `@if ($cond)
<div
x-on:drop="
    save('{{ $id }}')
    done()
"
class="p-2"
>
<p>x</p>
</div>
@endif
`
		want := `@if ($cond)
    <div
        x-on:drop="
            save('{{ $id }}')
            done()
        "
        class="p-2"
    >
        <p>x</p>
    </div>
@endif
`
		got := mustFormat(t, []byte(in), config.Default())
		if string(got) != want {
			t.Fatalf("value not re-homed under the tag.\n=== got ===\n%s\n=== want ===\n%s", got, want)
		}

		twice := mustFormat(t, got, config.Default())
		if !bytes.Equal(twice, got) {
			t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
		}
	})

	t.Run("preserves a significant space before an interior echo", func(t *testing.T) {
		in := `@if ($cond)
<div
x-on:click="
    label = 'item {{ $n }}: keep me'
    go('{{ $url }}')
"
class="p-2"
>
<p>x</p>
</div>
@endif
`
		got := mustFormat(t, []byte(in), config.Default())

		if !bytes.Contains(got, []byte("label = 'item {{ $n }}: keep me'")) {
			t.Fatalf("space before an interior echo was corrupted:\n%s", got)
		}

		if !bytes.Contains(got, []byte("go('{{ $url }}')")) {
			t.Fatalf("trailing echo run altered:\n%s", got)
		}

		twice := mustFormat(t, got, config.Default())
		if !bytes.Equal(twice, got) {
			t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
		}
	})

	t.Run("already-indented value stays byte-identical", func(t *testing.T) {
		in := `<div
    x-on:drop="
        save('{{ $id }}')
        done()
    "
    class="p-2"
>
    <p>x</p>
</div>
`
		got := mustFormat(t, []byte(in), config.Default())
		if string(got) != in {
			t.Fatalf("already-indented value changed.\n=== got ===\n%s\n=== want ===\n%s", got, in)
		}
	})
}

func TestFormatForeachRawArgRehomed(t *testing.T) {
	t.Run("multi-line @foreach array re-homes under the directive", func(t *testing.T) {
		in := `@if (true)
@foreach ([
    Cat::A => 'a',
    Cat::B => 'b',
] as $k => $v)
<span>{{ $v }}</span>
@endforeach
@endif
`
		want := `@if (true)
    @foreach ([
        Cat::A => 'a',
        Cat::B => 'b',
    ] as $k => $v)
        <span>{{ $v }}</span>
    @endforeach
@endif
`
		got := mustFormat(t, []byte(in), config.Default())
		if string(got) != want {
			t.Fatalf("@foreach array not re-homed.\n=== got ===\n%s\n=== want ===\n%s", got, want)
		}

		twice := mustFormat(t, got, config.Default())
		if !bytes.Equal(twice, got) {
			t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
		}
	})

	t.Run("multi-line @for clauses lay out as a PHP for header", func(t *testing.T) {
		in := "@if (true)\n@for (\n    $i = 0;\n    $i < 10;\n    $i++\n)\n<span>{{ $i }}</span>\n@endfor\n@endif\n"
		want := "@if (true)\n    @for ($i = 0; $i < 10; $i++)\n        <span>{{ $i }}</span>\n    @endfor\n@endif\n"

		got := mustFormat(t, []byte(in), config.Default())
		if string(got) != want {
			t.Fatalf("@for clause not laid out.\n=== got ===\n%s\n=== want ===\n%s", got, want)
		}
	})

	t.Run("multi-line string in the argument stays verbatim", func(t *testing.T) {
		in := "@if (true)\n        @foreach ([\n            'sql' => 'SELECT *\n                      FROM t',\n        ] as $k => $v)\n<span>{{ $v }}</span>\n        @endforeach\n@endif\n"

		got := mustFormat(t, []byte(in), config.Default())
		if !bytes.Contains(got, []byte("'SELECT *\n                      FROM t'")) {
			t.Fatalf("multi-line string in the raw argument was corrupted:\n%s", got)
		}

		twice := mustFormat(t, got, config.Default())
		if !bytes.Equal(twice, got) {
			t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
		}
	})

	t.Run("already-indented argument stays byte-identical", func(t *testing.T) {
		in := `@foreach ([
    Cat::A => 'a',
    Cat::B => 'b',
] as $k => $v)
    <span>{{ $v }}</span>
@endforeach
`
		got := mustFormat(t, []byte(in), config.Default())
		if string(got) != in {
			t.Fatalf("already-indented argument changed.\n=== got ===\n%s\n=== want ===\n%s", got, in)
		}
	})
}

func TestFormatGluedBlockWithPreStaysVerbatim(t *testing.T) {
	src := []byte("<table>\n<tr>\n<td>label<br>\n<pre>  indented two\n      indented six</pre>\n</td>\n</tr>\n</table>\n")

	got := mustFormat(t, src, config.Default())

	if !bytes.Contains(got, []byte("<pre>  indented two\n      indented six</pre>")) {
		t.Fatalf("nested <pre> whitespace was shifted by a re-home:\n%s", got)
	}

	twice := mustFormat(t, got, config.Default())
	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatTailwindEnabled(t *testing.T) {
	input := formattertest.ReadFixture(t, "tailwind_class_input.blade.php")
	want := formattertest.ReadFixture(t, "tailwind_class.formatted.blade.php")

	cfg := config.Default()
	cfg.Tailwind = true

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("Format is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatTailwindClassWithEcho(t *testing.T) {
	input := formattertest.ReadFixture(t, "tailwind_class_with_echo_input.blade.php")
	want := formattertest.ReadFixture(t, "tailwind_class_with_echo.formatted.blade.php")

	cfg := config.Default()
	cfg.Tailwind = true

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("Format is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatTailwindClassWithDirective(t *testing.T) {
	input := formattertest.ReadFixture(t, "tailwind_class_with_directive_input.blade.php")
	want := formattertest.ReadFixture(t, "tailwind_class_with_directive.formatted.blade.php")

	cfg := config.Default()
	cfg.Tailwind = true

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("Format is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatTailwindSortsClassesInsideVerbatimBodies(t *testing.T) {
	input := formattertest.ReadFixture(t, "tailwind_class_glued_body_input.blade.php")
	want := formattertest.ReadFixture(t, "tailwind_class_glued_body.formatted.blade.php")

	cfg := config.Default()
	cfg.Tailwind = true

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("Format is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// Laravel's compiler turns `@empty` and `@if (...)` into PHP and leaves
// `@md:flex` as text, so only the former stay pinned.
func TestFormatTailwindContainerQuery(t *testing.T) {
	input := formattertest.ReadFixture(t, "tailwind_container_query_input.blade.php")
	want := formattertest.ReadFixture(t, "tailwind_container_query.formatted.blade.php")

	cfg := config.Default()
	cfg.Tailwind = true

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("Format is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatTailwindAtClassDirective(t *testing.T) {
	input := formattertest.ReadFixture(t, "tailwind_atclass_input.blade.php")
	want := formattertest.ReadFixture(t, "tailwind_atclass.formatted.blade.php")

	cfg := config.Default()
	cfg.Tailwind = true

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("Format is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatTailwindBindClass(t *testing.T) {
	input := formattertest.ReadFixture(t, "tailwind_bindclass_input.blade.php")
	want := formattertest.ReadFixture(t, "tailwind_bindclass.formatted.blade.php")

	cfg := config.Default()
	cfg.Tailwind = true

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("Format is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatTailwindKeepsAParenAwayFromABareDirective(t *testing.T) {
	cfg := config.Default()
	cfg.Tailwind = true

	src := "<div class=\"p-4 @a flex ( m-2\"></div>\n"

	if got := mustFormat(t, []byte(src), cfg); string(got) != src {
		t.Fatalf("got:\n%s\nwant:\n%s", got, src)
	}
}

func TestFormatTailwindSortsAttributeBagClasses(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"class() sorts its list but not its conditions",
			`<div {{ $attributes->class(['text-white flex', 'font-bold bg-red-500' => $a === 'p-4 flex']) }}></div>`,
			`<div {{ $attributes->class(['flex text-white', 'bg-red-500 font-bold' => $a === 'p-4 flex']) }}></div>`,
		},
		{
			"merge() class value only",
			`<div {{ $attributes->merge(['class' => 'text-white flex', 'title' => 'text-white flex']) }}></div>`,
			`<div {{ $attributes->merge(['class' => 'flex text-white', 'title' => 'text-white flex']) }}></div>`,
		},
		{
			"through a bag-returning chain",
			`<div {{ $attributes->except('a')->class(['text-white flex']) }}></div>`,
			`<div {{ $attributes->except('a')->class(['flex text-white']) }}></div>`,
		},
		{
			"a slot's attributes",
			`<div {{ $a->attributes->class('text-white flex') }}></div>`,
			`<div {{ $a->attributes->class('flex text-white') }}></div>`,
		},
		{
			"another property's class()",
			`<div {{ $a->b->class('text-white flex') }}></div>`,
			`<div {{ $a->b->class('text-white flex') }}></div>`,
		},
		{
			"another object's class()",
			`<div {{ $a->class('text-white flex') }}></div>`,
			`<div {{ $a->class('text-white flex') }}></div>`,
		},
		{
			"a chain through a non-bag result",
			`<div {{ $attributes->get('a')->class('text-white flex') }}></div>`,
			`<div {{ $attributes->get('a')->class('text-white flex') }}></div>`,
		},
		{"class() without arguments", `<div {{ $attributes->class() }}></div>`, `<div {{ $attributes->class() }}></div>`},
	}

	cfg := config.Default()
	cfg.Tailwind = true

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src+"\n"), cfg)
			if string(got) != tc.want+"\n" {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}

			if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}

	t.Run("Tailwind off", func(t *testing.T) {
		src := cases[0].src + "\n"
		if got := mustFormat(t, []byte(src), config.Default()); string(got) != src {
			t.Fatalf("got:\n%s\nwant:\n%s", got, src)
		}
	})
}

func TestFormatTailwindGluedHole(t *testing.T) {
	input := formattertest.ReadFixture(t, "tailwind_class_glued_hole_input.blade.php")
	want := formattertest.ReadFixture(t, "tailwind_class_glued_hole.formatted.blade.php")

	cfg := config.Default()
	cfg.Tailwind = true

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("Format is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatTailwindKeepsAVerticalTabAtTheClassEdge(t *testing.T) {
	cfg := config.Default()
	cfg.Tailwind = true

	got := mustFormat(t, []byte("<p class=\"z \vz 0\"></p>"), cfg)

	if want := "<p class=\"\vz 0 z\"></p>\n"; string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
	}
}

func TestFormatRejectsInvalidSource(t *testing.T) {
	out, err := Format([]byte("<div>{{ new Foo(...) }}</div>\n"), config.Default())

	if err == nil {
		t.Fatal("Format accepted invalid source; want a parse error")
	}

	if out != nil {
		t.Fatalf("Format returned non-nil bytes on parse error: %q", out)
	}
}

// The complement of the tolerated deferred-close pattern (deferred_close
// fixture): genuinely malformed HTML must still fail loudly.
func TestFormatRejectsMalformedHTML(t *testing.T) {
	cases := map[string]string{
		"mismatched close":  "<div>text</span></div>\n",
		"unclosed at EOF":   "<div><p>hi\n",
		"stray close in el": "<div></span></div>\n",
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Format([]byte(src), config.Default()); err == nil {
				t.Fatalf("Format accepted malformed HTML %q; want a parse error", src)
			}
		})
	}
}

func TestAttrQuoteStyle(t *testing.T) {
	cases := []struct {
		name         string
		quoteStyle   string
		input        string
		wantContains string
	}{
		{"plain attr canonicalizes to double", config.QuoteStyleAuto, "<div class='x'></div>", `class="x"`},
		{"plain attr forced single", config.QuoteStyleSingle, `<div class="x"></div>`, `class='x'`},
		{"bound expression keeps source quote", config.QuoteStyleSingle, `<x-card :foo="$bar"></x-card>`, `:foo="$bar"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.QuoteStyle = tc.quoteStyle

			got := mustFormat(t, []byte(tc.input), cfg)

			if !bytes.Contains(got, []byte(tc.wantContains)) {
				t.Fatalf("output missing %q:\n%s", tc.wantContains, got)
			}

			if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
				t.Fatalf("attribute quoting is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
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

// Blade escapes an echo with a leading @ and emits the original text minus that
// @, so the bytes between the markers are rendered verbatim and the formatter
// must not touch them. Laravel applies the same (@)? prefix to {{ }} and to
// {!! !!}, over the whole template, so attribute position is not special.
func TestFormatPreservesBladeEscapes(t *testing.T) {
	unchanged := []string{
		`<div>a@{{$z}}b</div>`,
		`<div>a@{!!$z!!}b</div>`,
		`<div class="a@{{$z}}b"></div>`,
		`<div class="a@{!!$z!!}b"></div>`,
		`<div class="text-white @{{ tone }} bg-blue-500"></div>`,
		// No close marker ahead, so it stays literal instead of swallowing the rest.
		`<div class="a@{{$z b"></div>`,
	}

	for _, src := range unchanged {
		t.Run(src, func(t *testing.T) {
			got := mustFormat(t, []byte(src+"\n"), config.Default())

			if !bytes.Equal(got, []byte(src+"\n")) {
				t.Fatalf("escape was rewritten.\n=== got ===\n%s\n=== want ===\n%s", got, src+"\n")
			}

			twice := mustFormat(t, got, config.Default())
			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatEscapedEchoBesideLiveEcho(t *testing.T) {
	src := `<div class="{{$a}} x@{{$b}}"></div>` + "\n"
	want := `<div class="{{ $a }} x@{{$b}}"></div>` + "\n"

	got := mustFormat(t, []byte(src), config.Default())

	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatKeepsUnquotedAttrValues(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"unquoted href", "<a href=index.html>go</a>\n", "<a href=\"index.html\">go</a>\n"},
		{"spaced equals", "<a href = x>y</a>\n", "<a href=\"x\">y</a>\n"},
		{"spaced equals with a quoted value", "<a href = \"x\">y</a>\n", "<a href=\"x\">y</a>\n"},
		// The run after the `=` is the next attribute's quoted value, not this
		// attribute's, so consuming it would orphan that attribute's closing quote.
		{"bare equals before a quoted attribute", "<a rel= data-x=\"a b\">y</a>\n", "<a rel data-x=\"a b\">y</a>\n"},
		{"several on one tag", "<a href=x class=y id=z>t</a>\n", "<a href=\"x\" class=\"y\" id=\"z\">t</a>\n"},
		{"flag attribute keeps no value", "<a href>x</a>\n", "<a href>x</a>\n"},
		{"flag among unquoted", "<a rel=nofollow noopener>x</a>\n", "<a rel=\"nofollow\" noopener>x</a>\n"},
		{"empty value drops the equals", "<a href=>x</a>\n", "<a href>x</a>\n"},
		// A directive-shaped name keeps it instead: without the `=` the name is a bare
		// directive again and the run the scanner declined binds to it as arguments.
		{"a directive name keeps the equals", "<a @a= (\"x\">y</a>\n", "<a @a=\"\" (\"x\">y</a>\n"},
		{"bound unquoted value", "<x-card :title=$post>c</x-card>\n", "<x-card :title=\"$post\">c</x-card>\n"},
		{"quoted is unchanged", "<a href=\"x\">y</a>\n", "<a href=\"x\">y</a>\n"},
	}

	cfg := config.Default()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), cfg)

			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			twice := mustFormat(t, got, cfg)

			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent: %q then %q", got, twice)
			}
		})
	}
}

// A `:`-bound value on a plain element is Alpine's x-bind shorthand, so it is
// JavaScript and has to come back unchanged.
func TestFormatKeepsNonPhpBoundValues(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"typeof ternary",
			"<div :class=\"typeof x === 'string' ? 'a' : 'b'\">A</div>\n",
			"<div :class=\"typeof x === 'string' ? 'a' : 'b'\">A</div>\n",
		},
		{"await call", "<div :x=\"await save()\">B</div>\n", "<div :x=\"await save()\">B</div>\n"},
		{"arrow function", "<div :c=\"a => a + 1\">C</div>\n", "<div :c=\"a => a + 1\">C</div>\n"},
		{"in operator", "<div :f=\"a in b\">D</div>\n", "<div :f=\"a in b\">D</div>\n"},
		{"juxtaposition", "<div :f=\"foo() bar()\">E</div>\n", "<div :f=\"foo() bar()\">E</div>\n"},
		// JS that also parses as PHP: `TRUE` is an undefined name in JS, not true.
		{"php-shaped constants", "<div :hidden=\"TRUE\" :a=\"NULL\">G</div>\n", "<div :hidden=\"TRUE\" :a=\"NULL\">G</div>\n"},
		{"php-shaped spacing", "<div :class=\"a?'x':'y'\">H</div>\n", "<div :class=\"a?'x':'y'\">H</div>\n"},
		{
			"author-broken ternary keeps its lines",
			"<div>\n    <button\n        :class=\"a\n            ? 'x'\n            : 'y'\"\n    >I</button>\n</div>\n",
			"<div>\n    <button\n        :class=\"a\n            ? 'x'\n            : 'y'\"\n    >I</button>\n</div>\n",
		},
		// A component binding really is PHP and still routes through the grammar.
		{"component binding stays php", "<x-card :title=\"$post->title\">F</x-card>\n", "<x-card :title=\"$post->title\">F</x-card>\n"},
	}

	cfg := config.Default()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), cfg)

			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			twice := mustFormat(t, got, cfg)

			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent: %q then %q", got, twice)
			}
		})
	}
}

func TestFormatMultiLineHtmlCommentKeepsItsShape(t *testing.T) {
	src := []byte("<div>\n    <section>\n        <!--\n      <p>legacy</p>\n    -->\n    </section>\n</div>\n")

	want := "<div>\n    <section>\n        <!--\n          <p>legacy</p>\n        -->\n    </section>\n</div>\n"

	cfg := config.Default()
	got := mustFormat(t, src, cfg)

	if string(got) != want {
		t.Fatalf("comment shape changed.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatKeepsTrailingSpaceInMultiLineAttrValue(t *testing.T) {
	src := []byte("<div>\n    <span title=\"one   \n  two   \n  three\">x</span>\n</div>\n")

	cfg := config.Default()
	got := mustFormat(t, src, cfg)

	if !bytes.Contains(got, []byte("one   \n")) || !bytes.Contains(got, []byte("two   \n")) {
		t.Fatalf("trailing spaces deleted from the attribute value:\n%q", got)
	}

	if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
	}
}

func TestFormatNestedBlockOpenersAreStable(t *testing.T) {
	cfg := config.Default()

	for _, levels := range []int{1, 2, 8, 26} {
		t.Run(fmt.Sprintf("levels_%d", levels), func(t *testing.T) {
			src := []byte(strings.Repeat("@if(1)", levels) + "\n")

			got := mustFormat(t, src, cfg)

			if n := bytes.Count(got, []byte("@if (1)")); n != levels {
				t.Fatalf("kept %d openers, want %d:\n%s", n, levels, got)
			}

			if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent:\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}

	// A closed block still parses as a block, so the memo did not swallow a real
	// closer.
	closed := mustFormat(t, []byte("@if (1)\n<p>x</p>\n@endif\n"), cfg)
	if !bytes.Contains(closed, []byte("    <p>x</p>")) {
		t.Fatalf("closed block lost its indented body:\n%s", closed)
	}
}

func TestFormatDirectiveWithBreakingArrayIsAFixpoint(t *testing.T) {
	cfg := config.Default()

	entries := make([]string, 0, 14)
	for i := range 14 {
		entries = append(entries, fmt.Sprintf("'key%02d' => %d", i, i))
	}

	for _, name := range []string{"if", "while", "isset", "unless"} {
		for n := 6; n <= 12; n++ {
			t.Run(fmt.Sprintf("%s_%d", name, n), func(t *testing.T) {
				src := []byte("@" + name + "([" + strings.Join(entries[:n], ", ") + "])\nx\n@end" + name + "\n")

				once := mustFormat(t, src, cfg)

				if twice := mustFormat(t, once, cfg); !bytes.Equal(twice, once) {
					t.Fatalf("not a fixpoint.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
				}
			})
		}
	}
}

// Laravel's statement regex starts `\B@`, so a `@` glued to the end of a word
// is literal text: `support@gmail.com` is an address, not a directive.
func TestDirectiveNeedsNonWordCharBefore(t *testing.T) {
	literal := []string{
		"Contact support@gmail.com today.\n",
		"Contact noreply@error.com for help.\n",
		"x@endforeach\n",
		"a@if($b)c\n",
		"1@include(\"p\")\n",
		"_@auth\n",
		"Reply to hi@auth.dev or q@js.org.\n",
	}

	for _, src := range literal {
		t.Run(strings.TrimSpace(src), func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if string(got) != src {
				t.Fatalf("glued @word was treated as a directive.\n=== in ===\n%q\n=== out ===\n%q", src, got)
			}
		})
	}
}

// The raw-block and escaped-echo passes run before Laravel's statement pass and
// match without `\B`, so these four openers still count after a word character.
// Blade compiles `x@php $a = 1; @endphp` to `x<?php $a = 1; ?>`.
func TestGluedRawBlockAndEscapeStillParse(t *testing.T) {
	cases := []struct {
		name         string
		src          string
		wantContains string
	}{
		{"php block", "x@php\n$a = 1;\n@endphp\n", "$a = 1;"},
		{"verbatim block", "x@verbatim\n{{ raw }}\n@endverbatim\n", "{{ raw }}"},
		{"escaped echo", "<div>x@{{ $a }}</div>\n", "x@{{ $a }}"},
		{"escaped raw echo", "<div>x@{!! $a !!}</div>\n", "x@{!! $a !!}"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if !bytes.Contains(got, []byte(tc.wantContains)) {
				t.Fatalf("output missing %q:\n%q", tc.wantContains, got)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

// A `@` demoted to text by the `\B` rule must stay glued to the node before it.
// Emitting the following text run on its own line replaces the word character
// that demoted it, which promotes the text back to a directive: `@csrf@auth`
// would compile two directives where Blade compiles one plus literal text.
func TestGluedLiteralAtStaysOnTheSameLine(t *testing.T) {
	cases := []string{
		"@A@A!\n",
		"@csrf@auth\n",
		"@csrf@if($a)\n",
		"@php\n$a = 1;\n@endphp@auth\n",
		"@verbatim\nx\n@endverbatim@auth\n",
	}

	for _, src := range cases {
		t.Run(strings.TrimSpace(src), func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if bytes.Contains(got, []byte("\n@A")) || bytes.Contains(got, []byte("\n@auth")) ||
				bytes.Contains(got, []byte("\n@if")) {
				t.Fatalf("a demoted @ was promoted by a line break:\n%q", got)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

// A `@` that follows another `@` is a directive opener in Blade (`x@@if($a)`
// compiles the `@if`), and a directive after any non-word byte is unaffected.
func TestDirectiveAfterNonWordCharStillOpens(t *testing.T) {
	cases := []string{
		"@if ($a)\n    <p>x</p>\n@endif\n",
		"<div>\n    @if ($a)\n        <p>x</p>\n    @endif\n</div>\n",
		"] @if ($a)\n    <p>x</p>\n@endif\n",
	}

	for _, src := range cases {
		t.Run(strings.TrimSpace(src), func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if !bytes.Contains(got, []byte("@if ($a)")) || !bytes.Contains(got, []byte("@endif")) {
				t.Fatalf("directive after a non-word byte stopped opening:\n%q", got)
			}
		})
	}
}

// HTML5 ends an unquoted attribute value at whitespace, but a Blade echo,
// escape or directive argument is one unit that may hold whitespace and `>`.
func TestUnquotedAttrValueKeepsBladeRegionsWhole(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"echo", "<img src={{ $url }} alt=hi>\n", `<img src="{{ $url }}" alt="hi">`},
		{"echo no inner space", "<div a={{$v}}>t</div>\n", `<div a="{{ $v }}">t</div>`},
		// A raw echo prints unescaped, so its runtime value can hold a quote and
		// break out of a delimiter the formatter added; the bytes stay as authored.
		{"raw echo", "<div a={!! $v !!}>t</div>\n", `<div a={!! $v !!}>t</div>`},
		// An escaped echo is rendered, not executed, so `{{ $v }}` reaches the
		// browser and an HTML tokenizer already reads three attributes there.
		// Quoting would merge them, so the bytes stay exactly as authored.
		{"escaped echo", "<div a=@{{ $v }}>t</div>\n", `<div a=@{{ $v }}>t</div>`},
		// A directive expands to unescaped markup (`@class([…])` becomes a whole
		// `class="…"` attribute), so the value keeps its bytes rather than gaining a
		// delimiter the expansion would break out of. What matters is that the `>` of
		// `=>` no longer ends the start tag and reparses the rest as content.
		{"directive arg with fat arrow", `<div a=@class(["x" => $y])>t</div>` + "\n", `<div a=@class(["x" => $y])>t</div>`},
		{"anchor", "<a href={{ $u }}>go</a>\n", `<a href="{{ $u }}">go</a>`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if !bytes.Contains(got, []byte(tc.want)) {
				t.Fatalf("output missing %q:\n%q", tc.want, got)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestUnquotedAttrValueNormalizesInOnePass(t *testing.T) {
	cases := []string{
		"<span x-show=%0>x</span>\n",
		"<div x-data={open:false}>x</div>\n",
		"<span x-show=open>x</span>\n",
		"<span @click=count++>x</span>\n",
		"<a href=index.html>go</a>\n",
		// Already several attributes to an HTML tokenizer, so left exactly as is.
		"<div x-data={ open: false }>x</div>\n",
		"<div a=@{{ $v }}>x</div>\n",
	}

	for _, src := range cases {
		t.Run(strings.TrimSpace(src), func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())

			if twice := mustFormat(t, once, config.Default()); !bytes.Equal(twice, once) {
				t.Fatalf("not a fixpoint on pass 1.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}
}

func TestTopLevelGlueIsPreserved(t *testing.T) {
	glued := []string{
		"a<b>x</b>c\n",
		"pre<span>x</span>post\n",
		"pre<br />post\n",
		"Thanks,<br>\n",
		"a{{ $x }}b\n",
		"@if ($a)\nx<b>y</b>z\n@endif\n",
	}

	for _, src := range glued {
		t.Run(strings.TrimSpace(src), func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if bytes.Contains(bytes.TrimRight(got, "\n"), []byte("\n")) &&
				!strings.Contains(src, "@if") {
				t.Fatalf("a glued boundary gained a line break:\n%q", got)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestInlineBlockRunKeepsTheAuthorsLineBreaks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"lone CR inside a glued run's text", "<div><button></button><a0/>0\r0</div>\n", "<div>\n    <button></button><a0 />0\n    0\n</div>\n"},
		{
			"break between words of prose",
			"<p>Click <button>Save</button> to keep\nyour changes.</p>\n",
			"<p>\n    Click <button>Save</button> to keep\n    your changes.\n</p>\n",
		},
		{
			"controls on their own lines",
			"<div>\n<button>A</button>\n<button>B</button>\n</div>\n",
			"<div>\n    <button>A</button>\n    <button>B</button>\n</div>\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent: %q then %q", got, twice)
			}
		})
	}
}

// A component, a custom element and a legacy inline element like <font> render
// inline, so a line break beside one renders as a space.
func TestGlueBesideAnInlineByDefaultElementIsPreserved(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"component in a directive body",
			"@if ($a)\n    Due:<x-datetime :value=\"$d\" />!\n@endif\n",
			"@if ($a)\n    Due:<x-datetime :value=\"$d\" />!\n@endif\n",
		},
		{"component at the top level", "Save<x-icon name=\"check\"/>now\n", "Save<x-icon name=\"check\" />now\n"},
		{
			"legacy inline element beside a block",
			"<div><p>x</p>old<font color=\"red\">red</font>text</div>\n",
			"<div>\n    <p>x</p>\n    old<font color=\"red\">red</font>text\n</div>\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent: %q then %q", got, twice)
			}
		})
	}
}

func TestGluedDirectiveRunLaysOutLikeItsEchoTwin(t *testing.T) {
	got := mustFormat(t, []byte("<p>\n\n@A@\n</p>\n"), config.Default())
	echo := mustFormat(t, []byte("<p>\n\n{{ $a }}@\n</p>\n"), config.Default())

	if want := bytes.Replace(echo, []byte("{{ $a }}"), []byte("@A"), 1); !bytes.Equal(got, want) {
		t.Fatalf("the glued run laid out unlike its echo twin:\n%q\nwant\n%q", got, want)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
	}
}

func TestBodyEndBlankIsNotKeptAboveTheCloseTag(t *testing.T) {
	got := mustFormat(t, []byte("<p>@A@\n\n"), config.Default())

	if bytes.Contains(got, []byte("\n\n</p>")) {
		t.Fatalf("a blank line was left above the close tag:\n%q", got)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
	}
}

func TestBareDirectiveAttrKeepsAParenAttrSeparate(t *testing.T) {
	got := mustFormat(t, []byte("<div @if\n(x)></div>\n"), config.Default())

	if bytes.Contains(got, []byte("@if (x)")) {
		t.Fatalf("the attribute became the directive's arguments:\n%q", got)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
	}
}

func TestVerbatimBodyCloseTakesTheWritersIndent(t *testing.T) {
	fixpoints := []string{
		"<p><strong>0\n</strong>0<strong></strong>",
		"<p><span>0\n</span>0",
		"<p>pre<em>one\ntwo</em>post</p>\n",
		"<div><pre>a\nb\n</pre></div>\n",
		"<div><textarea>a\nb\n</textarea></div>\n",
	}

	for _, src := range fixpoints {
		t.Run(strings.ReplaceAll(src, "\n", "~"), func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}

	// The `<pre>` body is content: its last line must stay where the author left it.
	pre := mustFormat(t, []byte("<div><pre>a\nb\n</pre></div>\n"), config.Default())

	if !bytes.Contains(pre, []byte("b\n</pre>")) {
		t.Fatalf("indent was added inside <pre>:\n%q", pre)
	}
}

func TestAttrNameComposedWithAnEchoStaysOneAttribute(t *testing.T) {
	// Wide enough that the open tag has to break, which is where the split showed.
	const pad = ` class="rounded border border-gray-300 bg-white p-4 shadow-sm ring-1"` +
		` role="region" id="panel-main"`

	names := []string{
		`wire:model{{ $live ? '.live' : '' }}`,
		`data-{{ $k }}`,
		`wire:poll.{{ $interval }}`,
		`x-on:{{ $event }}`,
		`data-{{ $a }}-{{ $b }}`,
		`{{ $whole }}`,
		`wire:model{!! $raw !!}`,
	}

	for _, name := range names {
		for _, tail := range []string{"", `="name"`} {
			for _, extra := range []string{"", pad} {
				src := "<div " + name + tail + extra + ">x</div>\n"

				t.Run(name+tail+extra[:min(len(extra), 6)], func(t *testing.T) {
					got := mustFormat(t, []byte(src), config.Default())

					if !bytes.Contains(got, []byte(name+tail)) {
						t.Fatalf("the attribute was split.\n=== want to contain ===\n%s\n=== got ===\n%s", name+tail, got)
					}

					if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
						t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
					}
				})
			}
		}
	}
}

func TestAttributeBagIsNotReadAsAName(t *testing.T) {
	cases := []string{
		`<div {{ $attributes->merge(['class' => 'x']) }}>y</div>` + "\n",
		`<div {{ $attributes }}>y</div>` + "\n",
		`<div {!! $attributes !!}>y</div>` + "\n",
	}

	for _, src := range cases {
		t.Run(strings.TrimSpace(src), func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if string(got) != src {
				t.Fatalf("the attribute bag changed.\n=== want ===\n%s\n=== got ===\n%s", src, got)
			}
		})
	}
}

func TestValuelessDashedEventAttrKeepsItsName(t *testing.T) {
	cases := []string{
		"<div @my-event></div>\n",
		"<div @my-event data-id=\"1\"></div>\n",
		"<div @my-event.window></div>\n",
		"<div @my-event=\"handle()\"></div>\n",
		"<div @0=\"\" @a0\"></div>\n",
	}

	for _, src := range cases {
		t.Run(strings.TrimSpace(src), func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if string(got) != src {
				t.Fatalf("the attribute changed.\n=== want ===\n%q\n=== got ===\n%q", src, got)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestGlueAfterABrokenItemIsAFixpoint(t *testing.T) {
	cases := []string{
		"<p><span>0\n</span>0",
		"<p>x<span>0\n</span>0",
		"<p>x<span>0\n</span>0</p>\n",
		"0<A><p></p></A>\n",
		"<?php ?>0<A><p></A>",
	}

	for _, src := range cases {
		t.Run(strings.TrimSpace(strings.ReplaceAll(src, "\n", "~")), func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestTopLevelBlankLineIsPreserved(t *testing.T) {
	cases := []string{
		"Line one.\n\nLine two.\n",
		"# Head\n\nBody text.\n",
		"@component('mail::message')\n# Head\n\nBody.\n@endcomponent\n",
		"<div>\nLine one.\n\nLine two.\n</div>\n",
	}

	for _, src := range cases {
		t.Run(strings.TrimSpace(strings.ReplaceAll(src, "\n", "~")), func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if !bytes.Contains(got, []byte("\n\n")) {
				t.Fatalf("the authored blank line was dropped:\n%q", got)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestMarkdownMailTemplateIsUnchanged(t *testing.T) {
	src := []byte("@component('mail::message')\n" +
		"# Order shipped\n\n" +
		"Your order has shipped.\n\n" +
		"If you have questions, reply to this email or contact support@example.com.\n\n" +
		"@component('mail::button', ['url' => $url])\n" +
		"View Order\n" +
		"@endcomponent\n\n" +
		"Thanks,<br>\n" +
		"{{ config('app.name') }}\n" +
		"@endcomponent\n")

	got := mustFormat(t, src, config.Default())

	if !bytes.Equal(got, src) {
		t.Fatalf("the template changed.\n=== in ===\n%s\n=== out ===\n%s", src, got)
	}
}

// illuminate/view still ships compileEscapedEchos, so `{{{ $v }}}` compiles to
// `<?php echo e($v); ?>`, the same as `{{ }}` (checked against v13.23.0).
func TestTripleBraceEchoIsSupported(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"spaced", "{{{ $v }}}\n", "{{{ $v }}}"},
		{"tight", "{{{$v}}}\n", "{{{ $v }}}"},
		{"in an attribute", "<div title=\"{{{ $v }}}\">t</div>\n", "{{{ $v }}}"},
		{"with an expression", "{{{ $a ? 'y' : 'n' }}}\n", "{{{ $a ? 'y' : 'n' }}}"},
		{"double stays double", "{{ $v }}\n", "{{ $v }}"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if !bytes.Contains(got, []byte(tc.want)) {
				t.Fatalf("output missing %q:\n%q", tc.want, got)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestCommentInDirectiveArgKeepsAStringCR(t *testing.T) {
	cr := "\r"

	cases := []string{
		"@if ($a /* note */ && $x === 'a" + cr + "b')\ny\n@endif\n",
		"@if ($a // note\n&& $x === 'a" + cr + "b')\ny\n@endif\n",
		"@if ($a # note\n&& $x === 'a" + cr + "b')\ny\n@endif\n",
		// A CR the literal closes with a line feed: the line-ending strip would
		// delete it.
		"@A(0#" + cr + "%'" + cr + "\n')",
		"@if ($x === 'a" + cr + "\nb')\ny\n@endif\n",
		"@class(['x' => $a === \"p" + cr + "\nq\"])",
	}

	for _, src := range cases {
		t.Run(strings.ReplaceAll(strings.ReplaceAll(src, "\r", "<CR>"), "\n", "~"), func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if bytes.Count(got, []byte(cr)) != strings.Count(src, cr) {
				t.Fatalf("the string literal's CR was rewritten:\n=== in ===\n%q\n=== out ===\n%q", src, got)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestVoidElementKeepsTheAuthorsSlash(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"slashed", "<div><img src=\"a.png\" /></div>\n", "<img src=\"a.png\" />"},
		{"bare", "<div><img src=\"a.png\"></div>\n", "<img src=\"a.png\">"},
		{"slashed with an echo", "<div><img src=\"{{ $u }}\" /></div>\n", "<img src=\"{{ $u }}\" />"},
		{"bare with an echo", "<div><img src=\"{{ $u }}\"></div>\n", "<img src=\"{{ $u }}\">"},
		{"both spellings in one document", "<div><br /><br></div>\n", "<br /><br>"},
		{"inside a directive", "@if ($a)\n    <br />\n@endif\n", "<br />"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(mustFormat(t, []byte(tc.src), config.Default()))

			if !strings.Contains(got, tc.want) {
				t.Fatalf("void spelling not preserved: want %q in\n%s", tc.want, got)
			}

			if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatRawPhpIslandKeepsOpenTagCase(t *testing.T) {
	src := []byte("<div><?PHP $x=1+2; ?></div>\n")
	want := "<div>\n    <?PHP $x = 1 + 2; ?>\n</div>\n"

	got := mustFormat(t, src, config.Default())

	if string(got) != want {
		t.Fatalf("unexpected layout.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, config.Default())
	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatKeepsCommentBeforeDirectiveCloser(t *testing.T) {
	bodies := map[string]string{
		"if":      "@if($a %s)\nx\n@endif\n",
		"unless":  "@unless($a %s)\nx\n@endunless\n",
		"inline":  "@php($x = 1 %s)\n",
		"include": "@include('v', $a %s)\n",
	}

	comments := map[string]string{
		"block comment": "/* note */",
		"line comment":  "// note\n",
		"hash comment":  "# note\n",
	}

	for name, body := range bodies {
		for kind, comment := range comments {
			t.Run(name+" "+kind, func(t *testing.T) {
				src := []byte(fmt.Sprintf(body, comment))

				got := mustFormat(t, src, config.Default())

				if !bytes.Contains(got, []byte("note")) {
					t.Fatalf("the comment was deleted.\n=== in ===\n%s\n=== out ===\n%s", src, got)
				}

				twice := mustFormat(t, got, config.Default())
				if !bytes.Equal(twice, got) {
					t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
				}
			})
		}
	}
}

func TestFormatLoopClauseCommentEndedByALoneCR(t *testing.T) {
	for name, src := range map[string]string{
		"hash":  "@for(;;\n#\r)",
		"slash": "@for(;;\n//x\r)",
	} {
		t.Run(name, func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if !bytes.Contains(got, []byte("\n)")) {
				t.Fatalf("the `)` shares the comment's line: %q", got)
			}

			twice := mustFormat(t, got, config.Default())
			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestFormatDirectiveEmptyArgList(t *testing.T) {
	cases := map[string]string{
		"@A()\n":                       "@A()\n",
		"@if()\nx\n@endif\n":           "@if ()\n    x\n@endif\n",
		"@if(/* note */)\nx\n@endif\n": "@if (/* note */)\n    x\n@endif\n",
	}

	for src, want := range cases {
		t.Run(strings.TrimSpace(src), func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if string(got) != want {
				t.Fatalf("got %q, want %q", got, want)
			}

			twice := mustFormat(t, got, config.Default())
			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestFormatDirectiveTrailingCommentsRehome(t *testing.T) {
	for name, src := range map[string]string{
		"empty arg list": "@A(#\n#\n)\n",
		"after an arg":   "@if($a // one\n// two\n)\nx\n@endif\n",
	} {
		t.Run(name, func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			twice := mustFormat(t, got, config.Default())
			if !bytes.Equal(twice, got) {
				t.Fatalf("indent compounds across passes.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatRawPhpIslandKeepsMultilineString(t *testing.T) {
	for name, src := range map[string]string{
		"single quoted": "<div><?php $a = 'one\ntwo'; ?></div>\n",
		"double quoted": "<div><?php $a = \"one\ntwo\"; ?></div>\n",
	} {
		t.Run(name, func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if !bytes.Contains(got, []byte("one\ntwo")) {
				t.Fatalf("the literal's own newline was re-indented:\n%q", got)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}

func TestFormatGluedBlockBodyRehomed(t *testing.T) {
	src := []byte(`<table>
    <tr>
        <td class="meta">
        <strong>{{ $n }}</strong> element{{ $n === 1 ? '' : 's' }}
        across <strong>{{ $m }}</strong> cats<br>
        Generated {{ $date }}
    </td>
    </tr>
</table>
`)

	got := mustFormat(t, src, config.Default())

	if !bytes.Contains(got, []byte("\n            <strong>{{ $n }}</strong> element{{")) {
		t.Fatalf("glued block body not re-homed under the tag:\n%s", got)
	}

	if !bytes.Contains(got, []byte("\n        </td>")) {
		t.Fatalf("glued block closer not aligned with its opener:\n%s", got)
	}

	if !bytes.Contains(got, []byte("cats<br>")) || !bytes.Contains(got, []byte("</strong> element{{")) {
		t.Fatalf("glued boundary altered (whitespace injected):\n%s", got)
	}

	twice := mustFormat(t, got, config.Default())
	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// Asserts the invariant, not a shape: the layout must not depend on whether
// the author closed the element.
func TestFormatGluedBodyIgnoresWhetherTagWasClosed(t *testing.T) {
	pairs := map[string][2]string{
		"glued run over two lines": {
			"<p><strong>0</strong>0<strong>0\n0</strong>",
			"<p><strong>0</strong>0<strong>0\n0</strong></p>",
		},
		"glued run after a break": {
			"<p><strong>\n0</strong>0<strong></strong>",
			"<p><strong>\n0</strong>0<strong></strong></p>",
		},
	}

	for name, pair := range pairs {
		t.Run(name, func(t *testing.T) {
			open := mustFormat(t, []byte(pair[0]), config.Default())
			closed := mustFormat(t, []byte(pair[1]), config.Default())

			if !bytes.Equal(open, closed) {
				t.Fatalf("closing the tag changed the layout.\n=== unclosed ===\n%s\n=== closed ===\n%s", open, closed)
			}

			// The boundary the author wrote glued must survive both.
			if !bytes.Contains(open, []byte("</strong>0<strong>")) {
				t.Fatalf("a newline landed at a glued boundary, which renders as a space:\n%s", open)
			}

			if twice := mustFormat(t, open, config.Default()); !bytes.Equal(twice, open) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", open, twice)
			}
		})
	}
}

func TestFormatGluedInlineElementHangsItsOpenBracket(t *testing.T) {
	src := []byte("<div><div><div><a href=\"/terms\" class=\"link\">" +
		"Our terms and conditions apply to absolutely everyone here today<b>always</b>ok" +
		"</a></div></div></div>\n")

	got := mustFormat(t, src, config.Default())

	if !bytes.Contains(got, []byte("class=\"link\"\n")) {
		t.Fatalf("open bracket did not hang:\n%s", got)
	}

	if !bytes.Contains(got, []byte(">Our terms")) {
		t.Fatalf("body does not follow the hung bracket:\n%s", got)
	}

	if !bytes.Contains(got, []byte("ok</a>\n")) {
		t.Fatalf("close tag was not joined:\n%s", got)
	}

	if !bytes.Contains(got, []byte("here today<b>always</b>ok")) {
		t.Fatalf("a glued boundary was broken:\n%s", got)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// A block body that cannot fold onto the open-tag line keeps its authored lines
// when each fits, inline elements and all; only an overflowing line explodes.
func TestFormatBlockBodyKeepsFittingAuthoredLines(t *testing.T) {
	open := "<div>\n    <h2 class=\"aaaaaaaaa bbbbbbbbbbbbb cccccccccccccc dddddddddddd eeeeeeeeeeee ffffffff\">\n"
	closeTag := "    </h2>\n</div>\n"

	cases := []struct {
		name string
		body string
		want string
	}{
		{
			"mixed line that fits",
			"        {{ $a->b }} <span class=\"x\">/</span> {{ $a->c }}\n",
			"        {{ $a->b }} <span class=\"x\">/</span> {{ $a->c }}\n",
		},
		{
			"authored lines kept apart",
			"        {{ $a->b }} <span class=\"x\">/</span>\n        {{ $a->c }}\n",
			"        {{ $a->b }} <span class=\"x\">/</span>\n        {{ $a->c }}\n",
		},
		{
			"overflowing line keeps its spaces",
			"        {{ $a->bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb }} <span class=\"x\">/</span> {{ $a->cccccccccccccccccccccccccccccccc }}\n",
			"        {{ $a->bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb }} <span class=\"x\">/</span> {{ $a->cccccccccccccccccccccccccccccccc }}\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(open+tc.body+closeTag), config.Default())

			if want := open + tc.want + closeTag; string(got) != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

// An overflowing glued run breaks inside tag punctuation next to the overflow:
// a text-first body hangs its open bracket, and an element too long for its
// line hangs its close tag rather than leaving the break to the glued sibling.
func TestFormatGluedRunBreaksAtTheOverflow(t *testing.T) {
	long := "$aaaaaaaaaaaaaaaaaaaaaaaaaaaaa->bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	echo := "{{ __('aaaa.bbbb.cccccccccccc', ['count' => number_format($this->dddddddddd['eeeeeeeeeeeeeeeeeeee'], 1)]) }}"

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"text-first body hangs its open bracket",
			"<div>\n    <span class=\"a\">· <x-foo :value=\"" + long + "\" format=\"date\" /></span>\n</div>\n",
			"<div>\n    <span class=\"a\"\n        >· <x-foo :value=\"" + long + "\" format=\"date\"\n    /></span>\n</div>\n",
		},
		{
			"text-first body that fits stays flat",
			"<div>\n    <span class=\"a\">· <x-foo :value=\"$b\" format=\"date\" /></span>\n</div>\n",
			"<div>\n    <span class=\"a\">· <x-foo :value=\"$b\" format=\"date\" /></span>\n</div>\n",
		},
		{
			"overflowing element hangs before a glued element",
			"<div>\n    <span>" + echo + "</span><span>·</span>\n</div>\n",
			"<div>\n    <span>" + echo + "</span\n    ><span>·</span>\n</div>\n",
		},
		{
			"overflowing element keeps glued text on its line",
			"<div>\n    <strong>" + echo + "</strong>.\n</div>\n",
			"<div>\n    <strong>" + echo + "</strong>.\n</div>\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

// A blank line above a glued element sits outside it, so the element keeps the
// layout it gets without one instead of being joined past the print width.
func TestFormatGluedElementLayoutIgnoresABlankLineAbove(t *testing.T) {
	element := "    <x-button\n        variant=\"primary\"\n        size=\"sm\"\n        icon=\"plus\"\n" +
		"        icon:variant=\"outline\"\n        wire:click=\"openSomethingModal\"\n    >{{ $a }}</x-button>\n"

	for _, above := range []string{"    <p>x</p>\n", "    <p>x</p>\n\n"} {
		src := "<div>\n" + above + element + "</div>\n"

		got := mustFormat(t, []byte(src), config.Default())

		if string(got) != src {
			t.Fatalf("got:\n%s\nwant:\n%s", got, src)
		}
	}
}

// Livewire's @persist and @volt and Laravel's @pushIf are markup blocks Blade
// already knows; any other directive nests only once declared.
func TestFormatBlockDirectiveDeclaredOrBuiltIn(t *testing.T) {
	declared := config.Default()
	declared.Blade.BlockDirectives = []config.BladeBlockDirective{
		{Open: "feature", Branches: []string{"elsefeature", "else"}},
	}

	cases := []struct {
		name string
		cfg  *config.File
		src  string
		want string
	}{
		{
			"declared block with branches",
			declared,
			"@feature('a')\n<p>A</p>\n@elsefeature('b')\n<p>B</p>\n@else\n<p>C</p>\n@endfeature\n",
			"@feature('a')\n    <p>A</p>\n@elsefeature('b')\n    <p>B</p>\n@else\n    <p>C</p>\n@endfeature\n",
		},
		{
			"declared block in attribute position",
			declared,
			"<div class=\"aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd eeeeeeeeee ffffffffff gggggggggg\" @feature('x')\ndata-a=\"1\" @endfeature></div>\n",
			"<div\n    class=\"aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd eeeeeeeeee ffffffffff gggggggggg\"\n    @feature('x')\n        data-a=\"1\"\n    @endfeature\n></div>\n",
		},
		{
			"undeclared block stays flat",
			config.Default(),
			"@feature('a')\n<p>A</p>\n@endfeature\n",
			"@feature('a')\n<p>A</p>\n@endfeature\n",
		},
		{
			"a non-mail @component nests as markup",
			config.Default(),
			"@component('components.alert')\n@slot('title')\nOops\n@endslot\n<p>Broke.</p>\n@endcomponent\n",
			"@component('components.alert')\n    @slot('title')\n        Oops\n    @endslot\n    <p>Broke.</p>\n@endcomponent\n",
		},
		{
			"built-in Livewire and Laravel blocks",
			config.Default(),
			"@persist('player')\n<audio></audio>\n@endpersist\n@volt('counter')\n<div></div>\n@endvolt\n@pushIf($x, 'scripts')\n<script src=\"/x.js\"></script>\n@endPushIf\n",
			"@persist('player')\n    <audio></audio>\n@endpersist\n@volt('counter')\n    <div></div>\n@endvolt\n@pushIf($x, 'scripts')\n    <script src=\"/x.js\"></script>\n@endPushIf\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), tc.cfg)

			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if twice := mustFormat(t, got, tc.cfg); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent: %q then %q", got, twice)
			}
		})
	}
}

// Two trailing spaces are a Markdown hard line break, so a mail body keeps
// them.
func TestMailBodyKeepsAMarkdownHardBreak(t *testing.T) {
	for _, src := range []string{
		"<x-mail::message>\nLine one  \nLine two\n</x-mail::message>\n",
		"@component('mail::message')\nLine one  \nLine two\n@endcomponent\n",
	} {
		if got := mustFormat(t, []byte(src), config.Default()); string(got) != src {
			t.Errorf("mail body changed:\n%q\nwant\n%q", got, src)
		}
	}
}

// A form feed or vertical tab at the edge of an inline run is painted, not
// collapsed, so it stays the byte it is.
func TestInlineRunKeepsAPaintedEdgeCharacter(t *testing.T) {
	for _, c := range []string{"\f", "\v"} {
		src := "000<span>0000{!! 0 !!}0000" + c + "</span>\n"

		got := mustFormat(t, []byte(src), config.Default())

		if !strings.Contains(string(got), "0000"+c+"</span>") {
			t.Errorf("%q at the run's edge did not survive:\n%q", c, got)
		}
	}
}

func TestEchoDocFormatting(t *testing.T) {
	cfg := config.Default()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "<p>{{ $name }}</p>\n", "<p>{{ $name }}</p>\n"},
		{"bool casing", "{{ TRUE ? 'a' : 'b' }}\n", "{{ true ? 'a' : 'b' }}\n"},
		{"cast alias", "{{ (integer) $x }}\n", "{{ (int) $x }}\n"},
		{"arrow spacing", "{{ $c->map(fn($i)=>$i->id) }}\n", "{{ $c->map(fn ($i) => $i->id) }}\n"},
		{"interior comment kept", "{{ $a/* keep */+$b }}\n", "{{ $a/* keep */+$b }}\n"},
		{"raw echo", "{!! $html !!}\n", "{!! $html !!}\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Format([]byte(tc.in), cfg)
			if err != nil {
				t.Fatalf("Format(%q): %v", tc.in, err)
			}

			if string(got) != tc.want {
				t.Fatalf("Format(%q) = %q, want %q", tc.in, got, tc.want)
			}

			again, err := Format(got, cfg)
			if err != nil {
				t.Fatalf("re-format: %v", err)
			}

			if !bytes.Equal(again, got) {
				t.Fatalf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

func TestPhpBlockDocFormatting(t *testing.T) {
	cfg := config.Default()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"normalizes body",
			"@php\n$v=(integer)$x;\nif($ready){$z=TRUE;}\n@endphp\n",
			"@php\n    $v = (int) $x;\n    if ($ready) {\n        $z = true;\n    }\n@endphp\n",
		},
		{
			"keeps comment",
			"@php\n// keep me\n$a = 1;\n@endphp\n",
			"@php\n    // keep me\n    $a = 1;\n@endphp\n",
		},
		{
			"keeps a block holding only a comment",
			"@php\n// only\n@endphp\n",
			"@php\n    // only\n@endphp\n",
		},
		{
			"one-line block stays on its line",
			"@php $x = 1 @endphp\n",
			"@php $x = 1; @endphp\n",
		},
		{
			"one-line block with two statements becomes a block",
			"@php $x = 1; $y = 2; @endphp\n",
			"@php\n    $x = 1;\n    $y = 2;\n@endphp\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Format([]byte(tc.in), cfg)
			if err != nil {
				t.Fatalf("Format(%q): %v", tc.in, err)
			}

			if string(got) != tc.want {
				t.Fatalf("Format(%q) =\n%q\nwant\n%q", tc.in, got, tc.want)
			}

			again, err := Format(got, cfg)
			if err != nil {
				t.Fatalf("re-format: %v", err)
			}

			if !bytes.Equal(again, got) {
				t.Fatalf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

func TestPhpBlockVerbatimStatementIdempotent(t *testing.T) {
	cfg := config.Default()

	cases := []struct {
		name string
		in   string
	}{
		{
			"chain with step comment",
			"@php\n    $c = $x // comment\n        ->b();\n@endphp\n",
		},
		{
			"comment between chain steps",
			"@php\n    $c = $x\n        ->a()\n        // note\n        ->b();\n@endphp\n",
		},
		{
			"alternate syntax if",
			"@php\n    if ($a):\n        b();\n    endif;\n@endphp\n",
		},
		{
			"laravel-fmt-ignore frozen statement",
			"@php\n    // laravel-fmt-ignore\n    $c = $x\n        ->b();\n@endphp\n",
		},
		{
			"multi-line block comment",
			"@php\n    /* one\n       two */\n    $a = 1;\n@endphp\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			once, err := Format([]byte(tc.in), cfg)
			if err != nil {
				t.Fatalf("Format: %v", err)
			}

			twice, err := Format(once, cfg)
			if err != nil {
				t.Fatalf("re-format: %v", err)
			}

			if !bytes.Equal(twice, once) {
				t.Fatalf("not idempotent:\n=== once ===\n%s=== twice ===\n%s", once, twice)
			}
		})
	}
}

// Regression for the blade-fuzz heredoc-directive-arg-cr seed.
func TestHeredocDirectiveArgIdempotent(t *testing.T) {
	cfg := config.Default()

	cases := []struct {
		name string
		in   string
	}{
		{"literal CR in body (fuzz seed)", "@A(<<<A\n\r\r\nA)\n"},
		{"CRLF heredoc, blank middle line", "@A(<<<HTML\r\n<p>hi</p>\r\n\r\n<p>bye</p>\r\nHTML)\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			once, err := Format([]byte(tc.in), cfg)
			if err != nil {
				t.Fatalf("Format: %v", err)
			}

			twice, err := Format(once, cfg)
			if err != nil {
				t.Fatalf("re-format: %v", err)
			}

			if !bytes.Equal(twice, once) {
				t.Fatalf("not idempotent:\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}
}

// Each case pairs a fragment with its all-CRLF twin, which must format to the
// same bytes; wantLines pins which side of the width boundary the pair sits
// on, so a change moving both twins together still fails. Regression for the
// blade-fuzz e4e3456a seed.
// The directive's `)` shares the line with a flat argument, so an argument
// that reaches the width only with it breaks instead of printing at 121.
func TestDirectiveArgReservesItsClosingParen(t *testing.T) {
	arg := "foo(A % B() % ['" + strings.Repeat("a", 40) + "', '0000000000000000000000', '0000000000000000000000'])"

	got := mustFormat(t, []byte("@elseif("+arg+")\n"), config.Default())

	for line := range strings.SplitSeq(string(got), "\n") {
		if len(line) > config.Default().PrintWidth {
			t.Fatalf("line is %d columns:\n%s", len(line), got)
		}
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestCarriageReturnDoesNotChangeLayout(t *testing.T) {
	cfg := config.Default()

	zeros := func(n int) string { return strings.Repeat("0", n) }

	cases := []struct {
		name      string
		lf        string
		wantLines int
	}{
		// Directive argument: the host writes the built bytes as they are, and the
		// directive's `)` counts toward the width.
		{"arg fits flat", "@A(0%[" + zeros(91) + "\n               ,,])\n", 2},
		{"arg on the width boundary", "@A(0%[" + zeros(92) + "\n               ,,])\n", 2},
		{"arg breaks at the operator", "@A(0%[" + zeros(93) + "\n               ,,])\n", 3},

		// Control-flow condition, single line: the host re-homes the built bytes
		// under the opener, and the width decision that a CR could tip lives here.
		{"condition fits on the opener line", "@if(0%[" + zeros(106) + ",,])\nx\n@endif\n", 3},
		{"condition wraps under the opener", "@if(0%[" + zeros(108) + ",,])\nx\n@endif\n", 5},

		// Control-flow condition spanning lines: a condition carrying its own
		// newlines can never be laid out flat, so it wraps whatever its width. Both
		// widths land on 7 because there is no width decision left to tip, which is
		// the point: an interior CR cannot move this path at all anymore.
		{"multi-line condition wraps regardless", "@if(0%[" + zeros(100) + "\n     ,,])\nx\n@endif\n", 7},
		{"multi-line condition, wider", "@if(0%[" + zeros(102) + "\n     ,,])\nx\n@endif\n", 7},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lf, err := Format([]byte(tc.lf), cfg)
			if err != nil {
				t.Fatalf("format LF: %v", err)
			}

			crlf, err := Format([]byte(strings.ReplaceAll(tc.lf, "\n", "\r\n")), cfg)
			if err != nil {
				t.Fatalf("format CRLF: %v", err)
			}

			if !bytes.Equal(crlf, lf) {
				t.Fatalf("CR changed the layout:\n=== LF ===\n%q\n=== CRLF ===\n%q", lf, crlf)
			}

			if got := bytes.Count(lf, []byte("\n")); got != tc.wantLines {
				t.Fatalf("laid out on %d lines, want %d:\n%q", got, tc.wantLines, lf)
			}
		})
	}
}

func TestDirectiveAndAttrDocFormatting(t *testing.T) {
	cfg := config.Default()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"directive array arg cast", "@include('v', ['c' => (integer) $d])\n", "@include('v', ['c' => (int) $d])\n"},
		{"directive multi-arg", "@can('edit',$post)\nx\n@endcan\n", "@can('edit', $post)\n    x\n@endcan\n"},
		{"directive arg comment", "@if ($a /* keep */ && $b)\nz\n@endif\n", "@if ($a /* keep */ && $b)\n    z\n@endif\n"},
		{"bound attr array spacing", "<x-link :href=\"route('x', ['id'=>$id])\">l</x-link>\n", "<x-link :href=\"route('x', ['id' => $id])\">l</x-link>\n"},
		{"bound attr ternary", "<x-card :class=\"$on?'a':'b'\"></x-card>\n", "<x-card :class=\"$on ? 'a' : 'b'\"></x-card>\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Format([]byte(tc.in), cfg)
			if err != nil {
				t.Fatalf("Format(%q): %v", tc.in, err)
			}

			if string(got) != tc.want {
				t.Fatalf("Format(%q) = %q, want %q", tc.in, got, tc.want)
			}

			again, err := Format(got, cfg)
			if err != nil {
				t.Fatalf("re-format: %v", err)
			}

			if !bytes.Equal(again, got) {
				t.Fatalf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

func TestBoundAttrQuoteConflict(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"single_delim_double_value", "<x-a :a='$active ? \"on\" : \"off\"'></x-a>\n", "<x-a :a=\"$active ? 'on' : 'off'\"></x-a>\n"},
		{"double_delim_single_value", "<x-a :b=\"$active ? 'on' : 'off'\"></x-a>\n", "<x-a :b=\"$active ? 'on' : 'off'\"></x-a>\n"},
		{"no_conflict_kept", "<x-a :c='$value'></x-a>\n", "<x-a :c='$value'></x-a>\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if again := mustFormat(t, got, config.Default()); !bytes.Equal(again, got) {
				t.Fatalf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

func TestStyleBlockFormatting(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"pure_css_formatted",
			"<div>\n<style>\n.a  {  color : red ;}\n</style>\n</div>\n",
			"<div>\n    <style>\n        .a {\n            color: red;\n        }\n    </style>\n</div>\n",
		},
		{
			"blade_echo_rehomed",
			"<style>.a{color:{{ $c }}}</style>\n",
			"<style>\n    .a{color:{{ $c }}}\n</style>\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if again := mustFormat(t, got, config.Default()); !bytes.Equal(again, got) {
				t.Fatalf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

func TestFormatIgnoresABreakInsideADirectiveArgument(t *testing.T) {
	body := strings.Repeat("0", 78) + "<p><span> @foo(" + strings.Repeat("A", 31) + "!=" + strings.Repeat("A", 39) + ")000</span>"
	src := []byte(strings.Repeat("<div>", 6) + "<span @class(['a'])>" + body + "</span>" + strings.Repeat("</div>", 6) + "\n")

	once := mustFormat(t, src, config.Default())
	if twice := mustFormat(t, once, config.Default()); !bytes.Equal(twice, once) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
	}
}

func TestScriptShiftsAMultiLineBladeConstructButNotItsPHPLiterals(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"echo moves as one block",
			"<div>\n<script type=\"application/ld+json\">\n            {!! json_encode([\n                'a' => 1,\n            ]) !!}\n</script>\n</div>\n",
			"<div>\n    <script type=\"application/ld+json\">\n        {!! json_encode([\n            'a' => 1,\n        ]) !!}\n    </script>\n</div>\n",
		},
		{
			"multi-line string keeps its lines",
			"<div>\n<script type=\"application/ld+json\">\n            {!! json_encode([\n                'a' => 'one\ntwo',\n            ]) !!}\n</script>\n</div>\n",
			"<div>\n    <script type=\"application/ld+json\">\n        {!! json_encode([\n            'a' => 'one\ntwo',\n        ]) !!}\n    </script>\n</div>\n",
		},
		{
			"heredoc keeps its lines",
			"<div>\n<script>\n            @php\n                $x = <<<TXT\n                one\n                TXT;\n            @endphp\n</script>\n</div>\n",
			"<div>\n    <script>\n        @php\n            $x = <<<TXT\n                one\n                TXT;\n        @endphp\n    </script>\n</div>\n",
		},
	}

	cfg := config.Default()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), cfg)
			if string(got) != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}

			if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent:\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatTrimmedSlotTakesBlockLayout(t *testing.T) {
	long := "{{ __('enums.schedule_units.'.$unit->value.'.and.some.more.words.to.overflow.the.line') }}"
	block := "    <flux:select.option value=\"{{ $unit->value }}\">\n        " + long + "\n    </flux:select.option>\n"

	cases := []struct {
		name string
		src  string
		want string
	}{
		{"glued body that overflows", "    <flux:select.option value=\"{{ $unit->value }}\">" + long + "</flux:select.option>\n", block},
		{"break after the open tag only", "    <flux:select.option value=\"{{ $unit->value }}\">\n        " + long + "</flux:select.option>\n", block},
		{
			"break before the close tag only",
			"    <flux:button size=\"sm\">{{ __('common.cancel') }}\n    </flux:button>\n",
			"    <flux:button size=\"sm\">\n        {{ __('common.cancel') }}\n    </flux:button>\n",
		},
		{"glued body that fits", "    <x-badge>{{ $a }} and {{ $b }}</x-badge>\n", "    <x-badge>{{ $a }} and {{ $b }}</x-badge>\n"},
		{"break inside a glued body", "    <x-badge>{{ $a }}\n        {{ $b }}</x-badge>\n", "    <x-badge>{{ $a }}\n        {{ $b }}</x-badge>\n"},
		{"plain inline element", "    <span>\n        " + long + "</span>\n", "    <span>\n        " + long + "</span>\n"},
		{"livewire tag", "    <livewire:row>\n        " + long + "</livewire:row>\n", "    <livewire:row>\n        " + long + "</livewire:row>\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(mustFormat(t, []byte("<div>\n"+tc.src+"</div>\n"), config.Default()))

			if want := "<div>\n" + tc.want + "</div>\n"; got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatKeepsTheQuoteOfAValueHoldingOutput(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"json directive", `<div data-variants='@json($variantPrices)'></div>`, `<div data-variants='@json($variantPrices)'></div>`},
		{"php tag", `<div data-x='<?= $x ?>'></div>`, `<div data-x='<?= $x ?>'></div>`},
		{
			"quote inside the directive's arguments",
			`<v-x :channels='@json($c->map(fn ($c) => ['id' => $c->id])->values())'></v-x>`,
			`<v-x :channels='@json($c->map(fn ($c) => ['id' => $c->id])->values())'></v-x>`,
		},
		{"quote inside a php tag", `<div data-p="<?= "q" ?>"></div>`, `<div data-p="<?= "q" ?>"></div>`},
		{"plain value", `<div data-x='plain'></div>`, `<div data-x="plain"></div>`},
		{"at sign inside a word", `<a data-x='mail@x(y)'></a>`, `<a data-x="mail@x(y)"></a>`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(mustFormat(t, []byte(tc.src+"\n"), config.Default()))

			if want := tc.want + "\n"; got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatNeverTurnsAnAuthoredSpaceIntoALineBreak(t *testing.T) {
	prose := "This sentence was written on one line and runs well past the print width of one hundred and twenty characters."

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"echoes around an arrow",
			"<div>\n    <div>\n        <div>\n            <flux:heading size=\"lg\">{{ $alpha['first']?->record?->group?->label ?? '-' }} → {{ $alpha['last']?->record?->group?->label ?? '-' }}</flux:heading>\n        </div>\n    </div>\n</div>\n",
			"<div>\n    <div>\n        <div>\n            <flux:heading size=\"lg\">{{ $alpha['first']?->record?->group?->label ?? '-' }} → {{ $alpha['last']?->record?->group?->label ?? '-' }}</flux:heading>\n        </div>\n    </div>\n</div>\n",
		},
		{
			"text after a link",
			"<ul>\n    <li><a href=\"#\">Link</a> " + prose + "</li>\n</ul>\n",
			"<ul>\n    <li>\n        <a href=\"#\">Link</a> " + prose + "\n    </li>\n</ul>\n",
		},
		{
			"an authored break stays where it was",
			"<div>\n    <p>\n        Short line\n        {{ $name }} " + prose + "\n    </p>\n</div>\n",
			"<div>\n    <p>\n        Short line\n        {{ $name }} " + prose + "\n    </p>\n</div>\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())

			if string(got) != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatKeepsAComponentsAuthoredCloseName(t *testing.T) {
	src := "<div>\n    <flux:icon.check variant=\"outline\"></flux:icon>\n    <x-table.body.row>\n        <x-table.cell>a</x-table.cell>\n    </x-table.head.row>\n</div>\n"

	got := mustFormat(t, []byte(src), config.Default())
	if string(got) != src {
		t.Fatalf("got:\n%s\nwant:\n%s", got, src)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatPairsPhpBlocksAsBladeDoes(t *testing.T) {
	src := "@foreach ($rows as $row)\n    @php ($data = $row->options) @endphp\n    @php ($marked = $row->marked)\n    <p>{{ $data }}</p>\n@endforeach\n"
	want := "@foreach ($rows as $row)\n    @php ($data = $row->options); @endphp\n    @php($marked = $row->marked)\n    <p>{{ $data }}</p>\n@endforeach\n"

	got := mustFormat(t, []byte(src), config.Default())
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestFormatInlineRunKeepsAnAuthoredBlankLine(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"between lines broken beside a glued element",
			"<div>\n    a<br/>\n\n    ---<br/>\n\n    @lang('x') : {{ $y }}\n</div>\n",
			"<div>\n    a<br />\n\n    ---<br />\n\n    @lang('x') : {{ $y }}\n</div>\n",
		},
		{
			"between text atoms after a glued element",
			"<div>\n    a<br/> x\n\n    y\n</div>\n",
			"<div>\n    a<br /> x\n\n    y\n</div>\n",
		},
		{
			"inside an inline container",
			"<span>\n    a<b>x</b>\n\n    y</span>\n",
			"<span>\n    a<b>x</b>\n\n    y</span>\n",
		},
		{
			"CRLF line breaks",
			"<div>\r\n    a<br/>\r\n\r\n    b\r\n</div>\r\n",
			"<div>\n    a<br />\n\n    b\n</div>\n",
		},
		{
			"several blank lines collapse to one",
			"<div>\n    a<br/>\n\n\n\n    b\n</div>\n",
			"<div>\n    a<br />\n\n    b\n</div>\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.in), config.Default())

			if string(got) != tc.want {
				t.Fatalf("blank line not kept.\n=== got ===\n%s\n=== want ===\n%s", got, tc.want)
			}

			twice := mustFormat(t, got, config.Default())
			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatSingleAttributeFoldKeepsTheBodyLineBreak(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"double quote in the name",
			"<span a\"b>@lang('x')\n></span>\n",
			"<span a\"b>@lang('x')\n></span>\n",
		},
		{
			"single quote in the name",
			"<span a'b>@lang('x')\n></span>\n",
			"<span a'b>@lang('x')\n></span>\n",
		},
		{
			"hung bracket still folds",
			"<a\n    href=\"#\"\n>Text</a>\n",
			"<a href=\"#\">Text</a>\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.in), config.Default())

			if string(got) != tc.want {
				t.Fatalf("fold changed the body.\n=== got ===\n%s\n=== want ===\n%s", got, tc.want)
			}

			twice := mustFormat(t, got, config.Default())
			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatLoneAttributeFoldLaysTheValueOutOnItsOwnLine(t *testing.T) {
	name := strings.Repeat("a", 66)
	attr := strings.Repeat("b", 41)

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"echo broken by its author past the width",
			"<x-" + name + " " + attr + "={{ $a\n}} />\n",
			"<x-" + name + " " + attr + "=\"{{ $a }}\" />\n",
		},
		{
			"a chain that breaks on its own line stays broken",
			"<div class=\"{{ $this->aaaaaaaaaaa()\n    ->bbbbbbbbbbbbbbbb()->ccccccccccccccccc()->ddddddddddddddddd() }}\">x</div>\n",
			"<div\n    class=\"{{\n        $this->aaaaaaaaaaa()\n            ->bbbbbbbbbbbbbbbb()->ccccccccccccccccc()->ddddddddddddddddd()\n    }}\"\n>\n    x\n</div>\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.in), config.Default())

			if string(got) != tc.want {
				t.Fatalf("lone attribute laid out wrong.\n=== got ===\n%s\n=== want ===\n%s", got, tc.want)
			}

			twice := mustFormat(t, got, config.Default())
			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatGluedBodyTailNeedsARenderedGlue(t *testing.T) {
	long := strings.Repeat("a", 80)

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"component with an unknown self-closing child",
			"<x-foo aaa bbbbb><C c=\"" + long + "\" d=\"0\"/></x-foo>\n",
			"<x-foo aaa bbbbb>\n    <C c=\"" + long + "\" d=\"0\" />\n</x-foo>\n",
		},
		{
			"custom element with an unknown self-closing child",
			"<my-el aaa bbbbb><C c=\"" + long + "\" d=\"0\"/></my-el>\n",
			"<my-el aaa bbbbb>\n    <C c=\"" + long + "\" d=\"0\" />\n</my-el>\n",
		},
		{
			"an inline child keeps the hung layout",
			"<x-foo aaa bbbbb><img c=\"" + long + "\" d=\"0\"></x-foo>\n",
			"<x-foo\n    aaa\n    bbbbb\n><img c=\"" + long + "\" d=\"0\"></x-foo>\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.in), config.Default())

			if string(got) != tc.want {
				t.Fatalf("glued body layout wrong.\n=== got ===\n%s\n=== want ===\n%s", got, tc.want)
			}

			twice := mustFormat(t, got, config.Default())
			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatDirectiveArgumentLaysOutWhereItLands(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"line comments",
			"@A(0\n,0 // x\n.A // y\n.A)\n",
			"@A(0, 0 // x\n    .A  // y\n    .A)\n",
		},
		{
			"hash comments",
			"@A(0\n,0#\n.A#\n.A)\n",
			"@A(0, 0 #\n    .A  #\n    .A)\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.in), config.Default())

			if string(got) != tc.want {
				t.Fatalf("argument laid out at the wrong column.\n=== got ===\n%s\n=== want ===\n%s", got, tc.want)
			}

			twice := mustFormat(t, got, config.Default())
			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatKeepsOneBlankLineAboveConfiguredDirectives(t *testing.T) {
	cfg := config.Default()
	cfg.Blade.BlockDirectives = []config.BladeBlockDirective{{Open: "feature"}}
	cfg.Blade.BlankLineBeforeDirectives = []string{"if", "foreach", "empty", "section", "php", "feature"}

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"after a sibling element",
			"<div>\n<p>A</p>\n@if ($a)\n<p>B</p>\n@endif\n</div>\n",
			"<div>\n    <p>A</p>\n\n    @if ($a)\n        <p>B</p>\n    @endif\n</div>\n",
		},
		{
			"an authored blank line counts",
			"<p>A</p>\n\n\n@if ($a)\n<p>B</p>\n@endif\n",
			"<p>A</p>\n\n@if ($a)\n    <p>B</p>\n@endif\n",
		},
		{
			"between consecutive blocks",
			"@if ($a)\n<p>A</p>\n@endif\n@foreach ($xs as $x)\n<p>{{ $x }}</p>\n@endforeach\n",
			"@if ($a)\n    <p>A</p>\n@endif\n\n@foreach ($xs as $x)\n    <p>{{ $x }}</p>\n@endforeach\n",
		},
		{
			"forelse empty branch stays, standalone empty block gets one",
			"<p>A</p>\n\n@forelse ($xs as $x)\n<p>{{ $x }}</p>\n@empty\n<p>None</p>\n@endforelse\n<p>B</p>\n@empty($ys)\n<p>No ys</p>\n@endempty\n",
			"<p>A</p>\n\n@forelse ($xs as $x)\n    <p>{{ $x }}</p>\n@empty\n    <p>None</p>\n@endforelse\n<p>B</p>\n\n@empty($ys)\n    <p>No ys</p>\n@endempty\n",
		},
		{
			"php block",
			"<p>A</p>\n@php\n$a = 1;\n@endphp\n",
			"<p>A</p>\n\n@php\n    $a = 1;\n@endphp\n",
		},
		{
			"declared block",
			"<p>A</p>\n@feature('x')\n<p>B</p>\n@endfeature\n",
			"<p>A</p>\n\n@feature('x')\n    <p>B</p>\n@endfeature\n",
		},
		{
			"the blank goes above an attached comment",
			"<p>A</p>\n{{-- Filters --}}\n@if ($a)\n<p>B</p>\n@endif\n",
			"<p>A</p>\n\n{{-- Filters --}}\n@if ($a)\n    <p>B</p>\n@endif\n",
		},
		{"first child of an element", "<div>\n@if ($a)\n<p>B</p>\n@endif\n</div>\n", ""},
		{"first item in a directive body", "@foreach ($xs as $x)\n@if ($x)\n<p>B</p>\n@endif\n@endforeach\n", ""},
		{"branches", "<p>A</p>\n\n@if ($a)\n<p>B</p>\n@elseif ($b)\n<p>C</p>\n@else\n<p>D</p>\n@endif\n", ""},
		{"comment separated by its own blank line", "<p>A</p>\n{{-- note --}}\n\n@if ($a)\n<p>B</p>\n@endif\n", ""},
		{"inline forms", "<p>A</p>\n@section('title', 'Home')\n@php($x = 1)\n", ""},
		{"a text run above", "Hello {{ $name }}\n@if ($a)\n<p>B</p>\n@endif\n", ""},
		{"attribute position", "<div class=\"a\" @if ($a) data-a=\"1\" @endif></div>\n", ""},
		{"glued", "<span>A</span>@if ($a)<span>B</span>@endif\n", ""},
		{"pre", "<pre>\n<b>A</b>\n@if ($a)\nB\n@endif\n</pre>\n", ""},
		{"textarea", "<textarea>\nA\n@if ($a)\nB\n@endif\n</textarea>\n", ""},
		{"whitespace-pre-line class", "<div class=\"whitespace-pre-line\">\n<p>A</p>\n@if ($a)\n<p>B</p>\n@endif\n</div>\n", ""},
		{"white-space inline style", "<div style=\"white-space: pre-line\">\n<p>A</p>\n@if ($a)\n<p>B</p>\n@endif\n</div>\n", ""},
		{"white-space page style", "<style>.notes { white-space: pre-line }</style>\n<div class=\"notes\">\n<p>A</p>\n@if ($a)\n<p>B</p>\n@endif\n</div>\n", ""},
		{"mail component", "<x-mail::message>\n# Hi\n<p>A</p>\n@if ($a)\nBody\n@endif\n</x-mail::message>\n", ""},
		{"mail directive", "@component('mail::message')\n# Hi\n<p>A</p>\n@if ($a)\nBody\n@endif\n@endcomponent\n", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.want
			if want == "" {
				want = string(mustFormat(t, []byte(tc.src), config.Default()))
			}

			got := mustFormat(t, []byte(tc.src), cfg)
			if string(got) != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatKeepsAnEchoGluedToTheTagName(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"attributes break onto their own lines",
			`<img{!! $attributes !!}@if($a) loading="lazy"@endif src="{{ $b->cccccccccccc('dddddddddddd') }}" alt="{{ $b->eeeeeeee }}">` + "\n",
			"<img{!! $attributes !!}\n    @if ($a) loading=\"lazy\" @endif\n    src=\"{{ $b->cccccccccccc('dddddddddddd') }}\"\n    alt=\"{{ $b->eeeeeeee }}\"\n>\n",
		},
		{"echo spacing normalized", "<img{!!$attributes!!} src=\"a\">\n", "<img{!! $attributes !!} src=\"a\">\n"},
		{"glued comment", "<img{{-- note --}} src=\"a\">\n", "<img{{-- note --}} src=\"a\">\n"},
		{"dynamic tag pairs and normalizes both echoes", "<h{{$a}} class=\"b\">{{ $c }}</h{{$a}}>\n", "<h{{ $a }} class=\"b\">{{ $c }}</h{{ $a }}>\n"},
		{
			"tag named wholly by an echo lays out its attributes and body",
			"<{{ $as }}\n{{ $attributes->class(['a']) }} >\n@if ($b)\n<span>c</span>\n@endif\n</{{ $as }}>\n",
			"<{{ $as }} {{ $attributes->class(['a']) }}>\n    @if ($b)\n        <span>c</span>\n    @endif\n</{{ $as }}>\n",
		},
		{
			"tag named wholly by an echo keeps its glued edges past the width",
			"<div><{{ $as }}>" + strings.Repeat("word ", 30) + "end</{{ $as }}></div>\n",
			"<div>\n    <{{ $as }}>" + strings.Repeat("word ", 30) + "end</{{ $as }}>\n</div>\n",
		},
		{"unclaimed close still normalizes its echo", "a </{{$as}}> b\n", "a </{{ $as }}> b\n"},
		{"tag named wholly by an echo flows in its paragraph", "<p><{{ $as }} class=\"b\">x</{{ $as }}> y</p>\n", "<p><{{ $as }} class=\"b\">x</{{ $as }}> y</p>\n"},
		{"block child still closes the paragraph", "<div><p{!! $a !!}>x<div>y</div></div>\n", "<div>\n    <p{!! $a !!}>x</p>\n    <div>y</div>\n</div>\n"},
		{"close echo inside pre keeps its lone CR", "<pre><b{{ \"a\rb\" }}>x</b{{ \"a\rb\" }}></pre>\n", "<pre><b{{ \"a\rb\" }}>x</b{{ \"a\rb\" }}></pre>\n"},
		{"echo inside pre keeps its lone CR", "<pre><span{{ \"a\rb\" }}>x</span></pre>\n", "<pre><span{{ \"a\rb\" }}>x</span></pre>\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), config.Default())
			if string(got) != tc.want {
				t.Fatalf("got:\n%q\nwant:\n%q", got, tc.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatSpacesAlpineNotOperatorOnlyWithNotOperatorSpace(t *testing.T) {
	src := []byte("<button @click=\"open = ! open\">Toggle</button>\n")

	cases := []struct {
		notOperatorSpace bool
		want             string
	}{
		{true, "<button @click=\"open = ! open\">Toggle</button>\n"},
		{false, "<button @click=\"open = !open\">Toggle</button>\n"},
	}

	for _, c := range cases {
		cfg := config.Default()
		cfg.PHP.NotOperatorSpace = c.notOperatorSpace

		if got := mustFormat(t, src, cfg); string(got) != c.want {
			t.Errorf("NotOperatorSpace=%v: got %q, want %q", c.notOperatorSpace, got, c.want)
		}
	}
}
