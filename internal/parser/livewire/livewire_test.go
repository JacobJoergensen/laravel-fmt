package livewire

import (
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/parser/blade"
)

func TestSFCSplitsAtFirstCloseTag(t *testing.T) {
	src := "<?php $x = 1; ?>\n<div>hi</div>\n"

	file, _ := Parse([]byte(src), nil)

	if file.PHP == nil {
		t.Fatalf("PHP sub-tree is nil; frontmatter was not routed to the php parser")
	}

	if file.Template == nil {
		t.Fatalf("Template sub-tree is nil; body after ?> was not routed to the blade parser")
	}

	if len(file.PHP.Stmts) == 0 {
		t.Fatalf("PHP sub-tree has no statements")
	}

	if len(file.Template.Nodes) == 0 {
		t.Fatalf("Template sub-tree has no nodes")
	}
}

func TestFileWithoutCloseTagIsAllPHP(t *testing.T) {
	src := "<?php $x = 1;"

	file, _ := Parse([]byte(src), nil)

	if file.PHP == nil {
		t.Fatalf("PHP sub-tree is nil")
	}

	if file.Template != nil {
		t.Fatalf("Template sub-tree is non-nil on a file with no ?>")
	}
}

func TestTemplateNodeSpansAreAbsoluteToOriginalSource(t *testing.T) {
	src := "<?php $x = 1; ?>\n{{ $x }}\n"
	echoOffset := len("<?php $x = 1; ?>\n")

	file, _ := Parse([]byte(src), nil)

	if file.Template == nil {
		t.Fatalf("Template sub-tree is nil")
	}

	var echo *blade.EchoNode

	for _, n := range file.Template.Nodes {
		if e, ok := n.(*blade.EchoNode); ok {
			echo = e

			break
		}
	}

	if echo == nil {
		t.Fatalf("no blade.EchoNode in template nodes: %v", file.Template.Nodes)
		return
	}

	if got := echo.Open.Start.Offset; got != echoOffset {
		t.Fatalf("echo Open.Start.Offset=%d, want %d (template spans regressed to slice-relative offsets)", got, echoOffset)
	}

	if got := echo.Open.Start.Line; got != 2 {
		t.Fatalf("echo Open.Start.Line=%d, want 2 (line did not carry across the split)", got)
	}
}

func TestParseMergesErrorsFromBothSubtrees(t *testing.T) {
	src := "<?php $x = ?>\n@if("
	templateStart := len("<?php $x = ?>")

	_, errs := Parse([]byte(src), nil)

	if len(errs) == 0 {
		t.Fatalf("got 0 errors, want diagnostics from both sub-parsers")
	}

	var sawPhpSide, sawBladeSide bool

	for _, e := range errs {
		if e.Pos.Offset < templateStart {
			sawPhpSide = true
		} else {
			sawBladeSide = true
		}
	}

	if !sawPhpSide {
		t.Fatalf("no diagnostic with offset in the PHP region; errors=%v", errs)
	}

	if !sawBladeSide {
		t.Fatalf("no diagnostic with offset in the Blade region; errors=%v", errs)
	}
}

func TestSplitIgnoresCloseTagInsideStringOrHeredoc(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"double-quoted string", "<?php $x = \"a ?> b\"; ?>\n{{ $x }}\n"},
		{"heredoc", "<?php\n$x = <<<HTML\n<p>not ?> a tag</p>\nHTML;\n?>\n{{ $x }}\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src), nil)

			if len(errs) != 0 {
				t.Fatalf("unexpected errors (split truncated a literal?): %v", errs)
			}

			if file.Template == nil {
				t.Fatalf("Template sub-tree is nil; split did not find the real ?>")
			}

			var echo *blade.EchoNode

			for _, n := range file.Template.Nodes {
				if e, ok := n.(*blade.EchoNode); ok {
					echo = e

					break
				}
			}

			wantOffset := strings.Index(tc.src, "{{")

			if echo == nil {
				t.Fatalf("no blade.EchoNode in template nodes: %v", file.Template.Nodes)
			} else if got := echo.Open.Start.Offset; got != wantOffset {
				t.Fatalf("echo Open.Start.Offset=%d, want %d (split was fooled by the ?> inside the literal)", got, wantOffset)
			}
		})
	}
}
