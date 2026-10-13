package php

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
)

func TestFormatMatchesGolden(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".php", true) {
		t.Run(name, func(t *testing.T) {
			input := formattertest.ReadFixture(t, name+".php")
			want := formattertest.ReadFixture(t, name+".formatted.php")

			got := mustFormat(t, input, config.Default())

			if !bytes.Equal(got, want) {
				t.Fatalf("Format output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
			}
		})
	}
}

func TestFormatIsIdempotent(t *testing.T) {
	for _, name := range formattertest.GoldenFixtureNames(t, ".php", true) {
		t.Run(name, func(t *testing.T) {
			input := formattertest.ReadFixture(t, name+".php")

			once := mustFormat(t, input, config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(twice, once) {
				t.Fatalf("Format is not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
			}
		})
	}
}

func TestKeepTrailingCloseTag(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.RemoveTrailingCloseTag = false

	input := formattertest.ReadFixture(t, "keep_trailing_close_tag_input.php")
	want := formattertest.ReadFixture(t, "keep_trailing_close_tag.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent with RemoveTrailingCloseTag disabled.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestSingleQuotePreferDouble(t *testing.T) {
	cfg := config.Default()
	cfg.QuoteStyle = config.QuoteStyleDouble

	input := formattertest.ReadFixture(t, "single_quote_prefer_double_input.php")
	want := formattertest.ReadFixture(t, "single_quote_prefer_double.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent with SingleQuote=false.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestControlBraceNextLine(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.ControlBraceStyle = config.BraceStyleNextLine

	input := formattertest.ReadFixture(t, "control_brace_next_line_input.php")
	want := formattertest.ReadFixture(t, "control_brace_next_line.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent with ControlBraceStyle=next-line.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestBraceAlwaysNextLine(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.FunctionBraceStyle = config.BraceStyleAlwaysNextLine
	cfg.PHP.MethodBraceStyle = config.BraceStyleAlwaysNextLine

	input := formattertest.ReadFixture(t, "brace_always_next_line_input.php")
	want := formattertest.ReadFixture(t, "brace_always_next_line.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent with brace style=always-next-line.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// Expected order is PER Coding Style 3.0 §4.6.
func TestModifierOrder(t *testing.T) {
	cfg := config.Default()

	input := formattertest.ReadFixture(t, "modifier_order_input.php")
	want := formattertest.ReadFixture(t, "modifier_order.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("modifier ordering not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestConcatSpaceNone(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.ConcatSpace = false

	input := formattertest.ReadFixture(t, "concat_space_none_input.php")
	want := formattertest.ReadFixture(t, "concat_space_none.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent with ConcatSpace=false.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// Each `!` is its own UnaryExpr, so `!!` becomes `! ! `.
func TestNotOperatorSpaceEnabled(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.NotOperatorSpace = true

	input := formattertest.ReadFixture(t, "not_operator_space_input.php")
	want := formattertest.ReadFixture(t, "not_operator_space.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent with NotOperatorSpace=true.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestClosureFnSpaceEnabled(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.ClosureFnSpace = true

	input := formattertest.ReadFixture(t, "closure_fn_space_input.php")
	want := formattertest.ReadFixture(t, "closure_fn_space.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent with ClosureFnSpace=true.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// Alignment whitespace is non-newline trivia the parser drops, so the padded
// output re-formats to itself.
func TestAlignFatArrowEnabled(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true

	input := formattertest.ReadFixture(t, "align_fat_arrow_input.php")
	want := formattertest.ReadFixture(t, "align_fat_arrow.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent with AlignFatArrow=true.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestChainPropertyStepGluedToItsCall(t *testing.T) {
	src := []byte("<?php\n\nit('checks', function (): void {\n" +
		"    expect($atlAuto)->not->toBeNull()->and((float) $atlAuto->estimated_amount)" +
		"->toBe(1000.0)->and($btlAuto)->not->toBeNull();\n});\n")

	got := mustFormat(t, src, config.Default())

	if bytes.Contains(got, []byte("->not\n")) {
		t.Fatalf("property step stranded on its own line:\n%s", got)
	}

	if n := bytes.Count(got, []byte("->not->toBeNull()")); n != 2 {
		t.Fatalf("property step glued %d of 2 times:\n%s", n, got)
	}

	twice := mustFormat(t, got, config.Default())

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// A value that fits only short of its run's padding ends the run instead of
// breaking, and a multi-line value ends its `=>` run, as under PHP-CS-Fixer:
// the block splits into two columns rather than a padded line running past 120.
func TestAlignFatArrowWidthMarginal(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true

	input := formattertest.ReadFixture(t, "align_fat_arrow_width_marginal_input.php")
	want := formattertest.ReadFixture(t, "align_fat_arrow_width_marginal.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// An entry is only broken for padding it is printed with: one that fits short
// of a later, longer key ends its run there and stays flat, while padding from
// a key before it is real and still breaks it.
func TestAlignFatArrowBreaksOnlyForPaddingItGets(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true

	value := "$iiiiiiiiiiiiiiii === null || $iiiiiiiiiiiiiiii === '' ? null : (float) "
	short := value + "$iiiiiiiiiiiiiiiiiii,\n"
	long := value + "$iiiiiiiiiiiiiiiiiiiiiiiiiiiiiii,\n"
	wideEntry := "    '" + strings.Repeat("k", 39) + "' => $f->" + strings.Repeat("v", 65) + ",\n"
	firstOperand := "! $f->" + strings.Repeat("a", 70)
	lastOperand := "! $f->" + strings.Repeat("b", 50)

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"longer key after the entry",
			"<?php\n\n$x = [\n    'a' => $a,\n    'dddd' => " + short + "    'eeeee' => $e,\n    'a_very_long_key_name' => $k,\n];\n",
			"<?php\n\n$x = [\n    'a'    => $a,\n    'dddd' => " + short + "    'eeeee'                => $e,\n    'a_very_long_key_name' => $k,\n];\n",
		},
		{
			"padding before the entry still overflows",
			"<?php\n\n$x = [\n    'bbbbbbbbbbbb' => $a,\n    'dddd' => " + long + "    'a_very_long_key_name' => $k,\n];\n",
			"<?php\n\n$x = [\n    'bbbbbbbbbbbb' => $a,\n    'dddd' => " + long + "    'a_very_long_key_name' => $k,\n];\n",
		},
		{
			"longer key before the entry",
			"<?php\n\n$x = [\n    'a_very_long_key_name' => $k,\n    'dddd' => " + short + "    'eeeee' => $e,\n];\n",
			"<?php\n\n$x = [\n    'a_very_long_key_name' => $k,\n    'dddd' => " + short + "    'eeeee' => $e,\n];\n",
		},
		{
			"overflowing unbreakable value above the entry",
			"<?php\n\n$x = [\n    '" + strings.Repeat("k", 52) + "' => $f->" + strings.Repeat("v", 54) + ",\n    'dddd' => " + short + "];\n",
			"<?php\n\n$x = [\n    '" + strings.Repeat("k", 52) + "' => $f->" + strings.Repeat("v", 54) + ",\n    'dddd' => " + short + "];\n",
		},
		{
			"entry breaking anyway whose first line fits only unpadded",
			"<?php\n\n$x = [\n" + wideEntry + "    'cccc' => " + firstOperand + " && " + lastOperand + ",\n];\n",
			"<?php\n\n$x = [\n" + wideEntry + "    'cccc' => " + firstOperand + "\n        && " + lastOperand + ",\n];\n",
		},
		{
			"key too wide to align above the entry",
			"<?php\n\n$x = [\n    '" + strings.Repeat("k", 112) + "' => $k,\n    'dddd' => " + short + "];\n",
			"<?php\n\n$x = [\n    '" + strings.Repeat("k", 112) + "' => $k,\n    'dddd' => " + short + "];\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), cfg)

			if string(got) != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}

			if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestAlignFatArrowNestedKeyedArrays(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true

	input := formattertest.ReadFixture(t, "align_fat_arrow_scoped_input.php")
	want := formattertest.ReadFixture(t, "align_fat_arrow_scoped.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestAlignFatArrowKeylessInnerListAndLongString(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true

	input := formattertest.ReadFixture(t, "align_fat_arrow_keyless_value_input.php")
	want := formattertest.ReadFixture(t, "align_fat_arrow_keyless_value.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestAlignFatArrowShortKeyedInnerArray(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true

	input := formattertest.ReadFixture(t, "align_fat_arrow_short_keyed_inner_input.php")
	want := formattertest.ReadFixture(t, "align_fat_arrow_short_keyed_inner.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestAlignFatArrowSeparatorEndsScope(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true

	input := formattertest.ReadFixture(t, "align_fat_arrow_separator_input.php")
	want := formattertest.ReadFixture(t, "align_fat_arrow_separator.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// The `default` keyword counts toward the alignment width.
func TestAlignFatArrowMatchArms(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true

	input := formattertest.ReadFixture(t, "align_fat_arrow_match_input.php")
	want := formattertest.ReadFixture(t, "align_fat_arrow_match.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent with AlignFatArrow=true on match.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// The expected output is Pint's under binary_operator_spaces
// align_single_space_minimal: an element with no `=>` of its own ends the
// alignment scope.
func TestAlignFatArrowKeylessElementEndsScope(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true

	input := formattertest.ReadFixture(t, "align_fat_arrow_keyless_ends_scope_input.php")
	want := formattertest.ReadFixture(t, "align_fat_arrow_keyless_ends_scope.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// The expected output is Pint's: only the result's first line sits right of
// the `=>`, so the arm stays in the scope.
func TestAlignFatArrowMatchBlockResult(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true

	input := formattertest.ReadFixture(t, "align_fat_arrow_match_block_result_input.php")
	want := formattertest.ReadFixture(t, "align_fat_arrow_match_block_result.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// The expected output is Pint's.
func TestAlignFatArrowIndentedContext(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true

	input := formattertest.ReadFixture(t, "align_fat_arrow_indented_context_input.php")
	want := formattertest.ReadFixture(t, "align_fat_arrow_indented_context.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestAlignFatArrowTrailingComments(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true

	input := formattertest.ReadFixture(t, "align_fat_arrow_trailing_comments_input.php")
	want := formattertest.ReadFixture(t, "align_fat_arrow_trailing_comments.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestAlignFatArrowMatchTrailingComments(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true

	input := formattertest.ReadFixture(t, "align_fat_arrow_match_trailing_comments_input.php")
	want := formattertest.ReadFixture(t, "align_fat_arrow_match_trailing_comments.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestAlignFatArrowMatchArmBlockEndsScope(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true

	input := formattertest.ReadFixture(t, "align_fat_arrow_match_block_input.php")
	want := formattertest.ReadFixture(t, "align_fat_arrow_match_block.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// A broken condition list aligns by its last condition, the line its `=>`
// rides on.
func TestAlignFatArrowMatchConds(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true

	input := formattertest.ReadFixture(t, "align_fat_arrow_match_conds_input.php")
	want := formattertest.ReadFixture(t, "align_fat_arrow_match_conds.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestAlignFatArrowMatchCondLists(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true

	input := formattertest.ReadFixture(t, "align_fat_arrow_match_cond_lists_input.php")
	want := formattertest.ReadFixture(t, "align_fat_arrow_match_cond_lists.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestAlignFatArrowReservesPaddingPerRun(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true

	input := formattertest.ReadFixture(t, "align_fat_arrow_runs_input.php")
	want := formattertest.ReadFixture(t, "align_fat_arrow_runs.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// The expected output is Pint's on this exact input: the arm's `=>` sits on its
// wrapped line, which is what the run aligns to.
func TestAlignFatArrowMatchBrokenSingleCond(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true

	input := formattertest.ReadFixture(t, "align_fat_arrow_match_broken_cond_input.php")
	want := formattertest.ReadFixture(t, "align_fat_arrow_match_broken_cond.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// A return alone in its block gets no blank above it (the block-start
// exception).
func TestBlankLineBeforeReturnEnabled(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.BlankLineBeforeStatements = []string{config.StmtReturn}

	input := formattertest.ReadFixture(t, "blank_line_before_return_input.php")
	want := formattertest.ReadFixture(t, "blank_line_before_return.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent with blank line before return.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// A configured statement that opens its block gets no blank above it.
func TestBlankLineBeforeStatements(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.BlankLineBeforeStatements = []string{
		config.StmtContinue, config.StmtIf, config.StmtReturn, config.StmtThrow,
	}

	input := formattertest.ReadFixture(t, "blank_line_before_statements_input.php")
	want := formattertest.ReadFixture(t, "blank_line_before_statements.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestSortAndExpandUsesDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.SortUses = false
	cfg.PHP.ExpandUseGroups = false

	input := formattertest.ReadFixture(t, "no_sort_no_expand_input.php")
	want := formattertest.ReadFixture(t, "no_sort_no_expand.formatted.php")

	got := mustFormat(t, input, cfg)

	if !bytes.Equal(got, want) {
		t.Fatalf("output differs.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent with sort/expand disabled.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestUseSortBailsAtIgnoreDirective(t *testing.T) {
	input := "<?php\nuse Zebra\\A;\nuse Yak\\B;\n// laravel-fmt-ignore\nuse Camel\\C;\nuse Banana\\D;\nuse Apple\\E;\n"
	want := "<?php\n\nuse Yak\\B;\nuse Zebra\\A;\n// laravel-fmt-ignore\nuse Camel\\C;\nuse Apple\\E;\nuse Banana\\D;\n"

	got := mustFormat(t, []byte(input), config.Default())

	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	twice := mustFormat(t, got, config.Default())

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestImportOrderConfigurable(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.ImportOrder = []string{config.ImportKindClass, config.ImportKindFunction, config.ImportKindConst}

	input := "<?php\nuse const App\\CONST_A;\nuse function App\\func_a;\nuse App\\ClassA;\n"
	want := "<?php\n\nuse App\\ClassA;\n\nuse function App\\func_a;\n\nuse const App\\CONST_A;\n"

	got := mustFormat(t, []byte(input), cfg)

	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	twice := mustFormat(t, got, cfg)

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

// The width-driven path explodes the list one per line, so the assertion is on
// the break, not a hug shape.
func TestFormatWideCallArgBreaks(t *testing.T) {
	wide := strings.Repeat("a", 110)
	src := []byte("<?php\nf([$" + wide + " % 0], 0);\n")

	once := mustFormat(t, src, config.Default())
	twice := mustFormat(t, once, config.Default())

	if !bytes.Equal(once, twice) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
	}

	if !bytes.Contains(once, []byte("\n")) || !bytes.Contains(once, []byte("f(")) {
		t.Fatalf("wide call argument did not break:\n%s", once)
	}
}

func TestFormatCommentInPrefixedElementAfterHole(t *testing.T) {
	cases := []string{"<?php\n[, &#c\n$b];\n", "<?php\n[, ...#c\n$b];\n"}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(once, twice) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
			}

			if !bytes.Contains(once, []byte("#c")) {
				t.Fatalf("comment dropped:\n%s", once)
			}
		})
	}
}

func TestFormatChainCommentInEmptyArgsIsStable(t *testing.T) {
	cases := []string{
		"<?php\nit()->A->A(#\n)->A0;\n",
		"<?php\nit()->A->A(/*c*/)->A0;\n",
		"<?php\na->b(#\n)->c->d(#\n)->e;\n",
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())
			twice := mustFormat(t, once, config.Default())

			if !bytes.Equal(once, twice) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
			}
		})
	}
}

func TestFormatRejectsInvalidSource(t *testing.T) {
	out, err := Format([]byte("<?php\nnew Foo(...);\n"), config.Default())

	if err == nil {
		t.Fatal("Format accepted invalid source; want a parse error")
	}

	if out != nil {
		t.Fatalf("Format returned non-nil bytes on parse error: %q", out)
	}
}

// A CR inside heredoc/string content is part of the string's runtime value and
// must never be normalized away, so a CRLF heredoc body keeps its `\r`.
func TestFormatPreservesHeredocCR(t *testing.T) {
	cfg := config.Default()
	cfg.EndOfLine = config.EndOfLineCRLF

	input := []byte("<?php\r\n$x = <<<EOT\r\nline1\r\nline2\r\nEOT;\r\n")

	out := mustFormat(t, input, cfg)

	if !bytes.Contains(out, []byte("line1\r\n")) || !bytes.Contains(out, []byte("line2\r\n")) {
		t.Fatalf("heredoc CR not preserved.\n=== got ===\n%q", out)
	}

	if twice := mustFormat(t, out, cfg); !bytes.Equal(twice, out) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", out, twice)
	}
}

func TestATrailingCommentStaysInsideItsPhpIsland(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"block comment before a close tag", "<?php $a = 1; /* note */ ?>\n<p>x</p>\n"},
		{"line comment before a close tag", "<?php #note\n?>\n<p>x</p>\n"},
		{"close tag written inside a line comment", "<?php #0<?php?>\n0"},
		{"comment written in front of its statement", "0<?php /* note */;\n0;"},
		{"comment inside a one-line island", "<div><?php /* note */ echo $a; ?></div>\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := mustFormat(t, []byte(tc.src), config.Default())

			if i := bytes.Index(out, []byte("?>")); i >= 0 && bytes.Contains(out[i:], []byte("note")) {
				t.Fatalf("the comment was carried past the close tag:\n%s", out)
			}

			if twice := mustFormat(t, out, config.Default()); !bytes.Equal(twice, out) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", out, twice)
			}
		})
	}
}

func TestLineCommentInAnEmptyParameterListBreaksTheList(t *testing.T) {
	cases := []string{
		"<?php\n\nfunction f(// nothing yet\n) {}\n",
		"<?php\n\nfunction f(# nothing yet\n) {}\n",
		"<?php\n\nfunction f(/* intentionally no args */) {}\n",
	}

	for _, src := range cases {
		t.Run(strings.ReplaceAll(src, "\n", "~"), func(t *testing.T) {
			out := mustFormat(t, []byte(src), config.Default())

			if !bytes.Contains(out, []byte("no args")) && !bytes.Contains(out, []byte("nothing yet")) {
				t.Fatalf("the comment was dropped:\n%s", out)
			}

			// mustFormat parses what it is given, so a second run proves the first
			// output is still valid PHP as well as a fixpoint.
			if twice := mustFormat(t, out, config.Default()); !bytes.Equal(twice, out) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", out, twice)
			}
		})
	}
}

func TestAStatementCommentIsPrintedOnce(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"island comment above a call taking a closure", "0<?php # note\n0(function () {\n    ;\n});\n"},
		{"comment above a call taking a closure", "<?php\n\n// note\nf(function () {\n    return 1;\n});\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := mustFormat(t, []byte(tc.src), config.Default())

			if n := bytes.Count(out, []byte("note")); n != 1 {
				t.Fatalf("the comment appears %d times:\n%s", n, out)
			}

			if twice := mustFormat(t, out, config.Default()); !bytes.Equal(twice, out) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", out, twice)
			}
		})
	}
}

// A file whose `<?php` does not start at column zero is where a comment's
// column and its line's indent differ.
func TestBlockCommentContinuationDoesNotCreepLeft(t *testing.T) {
	cases := []string{
		" <?php /*\n  */ 0;\n",
		"<?=/*\n    */ 0;\n",
		"<?php\n\n    /* one\n       two */\n$y = 2;\n",
		"<?php\n\nfunction f()\n{\n    /* one\n       two */\n    return 1;\n}\n",
	}

	for _, src := range cases {
		t.Run(strings.ReplaceAll(src, "\n", "~"), func(t *testing.T) {
			out := mustFormat(t, []byte(src), config.Default())

			if twice := mustFormat(t, out, config.Default()); !bytes.Equal(twice, out) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", out, twice)
			}
		})
	}
}

func TestCommentAfterALoneCRStaysOnItsOwnLine(t *testing.T) {
	cases := []string{
		"<?php\n\n$x = [0 # first\r/* second */];\n",
		"<?php\n\n$a = 1; # first\r// second\n$b = 2;\n",
	}

	for _, src := range cases {
		t.Run(strings.ReplaceAll(src, "\n", "~"), func(t *testing.T) {
			out := mustFormat(t, []byte(src), config.Default())

			for line := range bytes.SplitSeq(out, []byte("\n")) {
				comment := bytes.IndexAny(line, "#/")
				if comment >= 0 && bytes.Contains(line[comment+1:], []byte("/*")) {
					t.Fatalf("a comment was swallowed by the one before it:\n%s", out)
				}
			}

			if twice := mustFormat(t, out, config.Default()); !bytes.Equal(twice, out) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", out, twice)
			}
		})
	}
}

func TestLineCommentEndsAtLoneCR(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "statement after the CR is kept at end of file",
			src:  "<?php\n$GLOBALS[1] = 1; # note\r$GLOBALS[2] = 2;\n",
			want: []string{"# note", "$GLOBALS[2] = 2;"},
		},
		{
			name: "double slash spelling",
			src:  "<?php\n$GLOBALS[1] = 1; // note\r$GLOBALS[2] = 2;\n",
			want: []string{"// note", "$GLOBALS[2] = 2;"},
		},
		{
			name: "code after the CR is formatted",
			src:  "<?php\n$a = 0; # note\r$b=5;   $c   =  6;\necho $b;\n",
			want: []string{"$b = 5;", "$c = 6;"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := mustFormat(t, []byte(tc.src), config.Default())

			for _, want := range tc.want {
				if !bytes.Contains(out, []byte(want)) {
					t.Fatalf("output is missing %q:\n%q", want, out)
				}
			}

			if twice := mustFormat(t, out, config.Default()); !bytes.Equal(twice, out) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", out, twice)
			}
		})
	}
}

func mustFormat(t *testing.T, src []byte, cfg *config.File) []byte {
	t.Helper()

	out, err := Format(src, cfg)
	if err != nil {
		t.Fatalf("format: %v", err)
	}

	return out
}

func TestMixedInlineHTMLNormalizesLineEndings(t *testing.T) {
	src := []byte("<div>\r\n    <?php echo htmlspecialchars($content) ?>\r\n</div>\r\n")

	out := mustFormat(t, src, config.Default())

	if bytes.Contains(out, []byte("\r")) {
		t.Fatalf("default config left a CR in mixed output:\n%q", out)
	}

	// A heredoc's CRLF bytes are part of the string value, not layout, and must not
	// be normalized away.
	withHeredoc := []byte("<?php\r\n$x = <<<EOT\r\nline\r\nEOT;\r\n")

	got := mustFormat(t, withHeredoc, config.Default())
	if !bytes.Contains(got, []byte("line\r\n")) {
		t.Fatalf("heredoc value CR was stripped:\n%q", got)
	}
}

func TestCastExprPreservesInteriorComment(t *testing.T) {
	cases := []string{
		"<?php\n$x = (/* a */ int) $y;\n",
		"<?php\n$x = (int /* b */) $y;\n",
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			out, err := Format([]byte(src), config.Default())
			if err != nil {
				t.Fatalf("Format returned error: %v", err)
			}

			if !bytes.Contains(out, []byte("/*")) {
				t.Fatalf("cast interior comment was dropped:\n%s", out)
			}
		})
	}
}

func TestInlineCommentsInExpressionPositionsSurvive(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"if condition", "<?php\nif (/* explain */ $a) {\n    $b = 1;\n}\n", "explain"},
		{"while condition", "<?php\nwhile (/* forever */ true) {\n    break;\n}\n", "forever"},
		{"foreach var hint", "<?php\nforeach ($users as /** @var User $user */ $user) {\n    echo 1;\n}\n", "@var"},
		{"after return", "<?php\nfunction f()\n{\n    return /* early out */ null;\n}\n", "early out"},
		{"assignment value", "<?php\n$total = /* cents */ 1000;\n", "cents"},
		{"cast operand", "<?php\n$x = (int) /* force */ $raw;\n", "force"},
		{"phpstan ignore in a condition", "<?php\nif (/* @phpstan-ignore-next-line */ $a instanceof B) {\n    echo 1;\n}\n", "@phpstan-ignore-next-line"},
		{"match arm body", "<?php\n$v = match ($a) {\n    1 => /* one */ 2,\n    default => 3,\n};\n", "one"},
		{"empty parameter list", "<?php\nfunction f(/* intentionally no args */) {}\n", "intentionally no args"},
		{"before a parameter", "<?php\nfunction f(/* the id */ int $id) {}\n", "the id"},
		{"argument list, already worked", "<?php\n$x = foo(/* first */ 1, 2);\n", "first"},
		{"array literal, already worked", "<?php\n$y = [/* empty on purpose */];\n", "empty on purpose"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := mustFormat(t, []byte(tc.src), config.Default())

			if !bytes.Contains(out, []byte(tc.want)) {
				t.Fatalf("comment %q was deleted:\n%s", tc.want, out)
			}

			if twice := mustFormat(t, out, config.Default()); !bytes.Equal(twice, out) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", out, twice)
			}
		})
	}
}

func TestCommentSurvivesADiscardedArgumentLayout(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"line comment before a rejected hug", "<?php\n\nf(!#c\n[], [\n[]]);\n", "#c"},
		{"block comment before a rejected hug", "<?php\n\nf(/* why */ $a, [\n    1,\n]);\n", "why"},
		{"comment on a middle argument", "<?php\n\nf($a, /* mid */ $b, [\n    1,\n]);\n", "mid"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := mustFormat(t, []byte(tc.src), config.Default())

			if !bytes.Contains(out, []byte(tc.want)) {
				t.Fatalf("comment %q was deleted:\n%s", tc.want, out)
			}

			if twice := mustFormat(t, out, config.Default()); !bytes.Equal(twice, out) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", out, twice)
			}
		})
	}
}

func TestCommentsWithNoStatementToAttachToSurvive(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"comment-only file", "<?php\n// lonely\n", "lonely"},
		{"hash comment-only file", "<?php\n# lonely\n", "lonely"},
		{"block comment-only file", "<?php\n/* lonely */\n", "lonely"},
		{"trailing comment in an island", "<div>\n    <?php $a = 1; // note ?>\n</div>\n", "note"},
		{"trailing hash comment in an island", "<div>\n    <?php $a = 1; # note\n$b = 2; ?>\n</div>\n", "note"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := mustFormat(t, []byte(tc.src), config.Default())

			if !bytes.Contains(out, []byte(tc.want)) {
				t.Fatalf("comment %q was deleted:\n%q", tc.want, out)
			}

			if twice := mustFormat(t, out, config.Default()); !bytes.Equal(twice, out) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", out, twice)
			}
		})
	}
}

// The last two cases are alternative-syntax blocks, copied from their source,
// where the `?>` is the terminator and must stay.
func TestTrailingCloseTagSettlesInOnePass(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		keepTag bool
	}{
		{"expression statement", "<?php 0?>", false},
		{"trailing space after the tag", "<?php 0?> ", false},
		{"echo", "<?php echo 1;?> ", false},
		{"assignment with no semicolon", "<?php $a = 1 ?>", false},
		// No tag inside the block, so the trailing one is the only `?>` the
		// assertion can see.
		{"alt syntax terminated by the tag", "<?php if ($a): endif ?>", true},
		{"alt syntax with its own semicolon", "<?php if ($a): endif; ?>", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			once := mustFormat(t, []byte(tc.src), config.Default())

			if twice := mustFormat(t, once, config.Default()); !bytes.Equal(twice, once) {
				t.Fatalf("not a fixpoint.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}

			if got := bytes.Contains(once, []byte("?>")); got != tc.keepTag {
				t.Fatalf("close tag kept=%v, want %v:\n%q", got, tc.keepTag, once)
			}
		})
	}
}

func TestHeredocWithLoneCRStaysParseable(t *testing.T) {
	cr := "\r"

	cases := []string{
		"<?php\n$s = <<<EOT\na" + cr + "b\nEOT;\necho strlen($s);\n",
		"<?php\n$s = <<<'EOT'\na" + cr + "b\nEOT;\necho strlen($s);\n",
		// Already indented, so the line after the CR carries the closer's indent too;
		// that is the spelling PHP accepts, and its value is the same `a<CR>b`.
		"<?php\nfunction f()\n{\n    $s = <<<EOT\n    a" + cr + "    b\n    EOT;\n\n    return $s;\n}\n",
	}

	for _, src := range cases {
		t.Run(strings.ReplaceAll(strings.ReplaceAll(src, "\r", "<CR>"), "\n", "~"), func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if bytes.Count(got, []byte(cr)) != strings.Count(src, cr) {
				t.Fatalf("the heredoc's CR was rewritten:\n=== in ===\n%q\n=== out ===\n%q", src, got)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}

			assertHeredocBodyClearsCloser(t, got)
		})
	}
}

// assertHeredocBodyClearsCloser checks the rule PHP enforces on a flexible
// heredoc: every body line is indented at least as far as the closing marker,
// and a lone CR ends a line as LF does. It is checked here rather than by
// running php, which a test must not need.
func assertHeredocBodyClearsCloser(t *testing.T, out []byte) {
	t.Helper()

	lines := bytes.FieldsFunc(out, func(r rune) bool { return r == '\n' || r == '\r' })

	indentOf := func(line []byte) int {
		return len(line) - len(bytes.TrimLeft(line, " \t"))
	}

	openerAt := -1

	for i, line := range lines {
		switch {
		case openerAt < 0:
			if bytes.Contains(line, []byte("<<<")) {
				openerAt = i
			}
		case bytes.HasSuffix(bytes.TrimSpace(line), []byte("EOT;")):
			closer := indentOf(line)

			for _, body := range lines[openerAt+1 : i] {
				if len(bytes.TrimSpace(body)) > 0 && indentOf(body) < closer {
					t.Fatalf("heredoc body line %q is indented %d, below the closer's %d:\n%q",
						body, indentOf(body), closer, out)
				}
			}

			return
		}
	}
}

func TestClassBodyTagPairFormatsTheRestOfTheFile(t *testing.T) {
	src := "<?php\n\nfunction  before( )  {\n  return   1;\n}\n\n" +
		"class A { public $x = 1 ?><?php }\n\nfunction  after( )  {\n  return   2;\n}\n"

	want := "<?php\n\nfunction before()\n{\n    return 1;\n}\n\n" +
		"class A { public $x = 1 ?><?php }\n\nfunction after()\n{\n    return 2;\n}\n"

	got := mustFormat(t, []byte(src), config.Default())
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent: %q -> %q", got, twice)
	}
}

func TestSyntheticTerminatorKeepsTheSpaceBeforeTheTag(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"space is kept", "x<?php echo $a ?>y", "x<?php echo $a; ?>y\n"},
		{"no space is not invented", "x<?php echo $a?>y", "x<?php echo $a;?>y\n"},
		{"a run collapses to one", "x<?php echo $a  ?>y", "x<?php echo $a; ?>y\n"},
		// An explicit `;` leaves nothing to stand in for, either way.
		{"explicit terminator, no space", "x<?php echo $a;?>y", "x<?php echo $a;?>y\n"},
		{"explicit terminator, space", "x<?php echo $a; ?>y", "x<?php echo $a; ?>y\n"},
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

func TestFormatKeepsCommentBeforeClosingParen(t *testing.T) {
	bodies := map[string]string{
		"if":          "if ($a %s) {\n}",
		"elseif":      "if ($a) {\n} elseif ($b %s) {\n}",
		"while":       "while ($a %s) {\n}",
		"do while":    "do {\n} while ($a %s);",
		"for":         "for ($i = 0; $i < 3; $i++ %s) {\n}",
		"foreach":     "foreach ($a as $b %s) {\n}",
		"switch":      "switch ($a %s) {\n    case 1:\n        break;\n}",
		"match":       "$x = match ($a %s) {\n    1 => 2,\n};",
		"catch":       "try {\n} catch (E $e %s) {\n}",
		"parens":      "$x = ($a %s);",
		"new":         "$x = new C($a %s);",
		"function":    "function f($a %s) {\n}",
		"method":      "class C\n{\n    function m($a %s) {}\n}",
		"closure use": "$f = function () use ($a %s) {\n};",
		"arrow fn":    "$f = fn ($a %s) => 1;",
		"call":        "$x = foo($a %s);",
		"array":       "$x = [1 %s];",
	}

	// A line comment runs to the end of its line, so a `)` or a `,` left on that
	// line is commented away and the result no longer parses. Both kinds have to
	// survive, and the output has to stay a fixed point.
	comments := map[string]string{
		"block comment": "/* note */",
		"line comment":  "// note\n",
		"hash comment":  "# note\n",
	}

	for name, body := range bodies {
		for kind, comment := range comments {
			t.Run(name+" "+kind, func(t *testing.T) {
				src := []byte("<?php\n\n" + fmt.Sprintf(body, comment) + "\n")

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

func TestFormatKeepsSeveralCommentsBeforeClosingParen(t *testing.T) {
	cases := map[string]string{
		"block comments": "if ($a /* one */ /* two */) {\n}",
		"line comments":  "if ($a // one\n// two\n) {\n}",
		"hash comments":  "if ($a # one\n# two\n) {\n}",
		"mixed":          "if ($a /* one */ // two\n) {\n}",
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			src := []byte("<?php\n\n" + body + "\n")

			got := mustFormat(t, src, config.Default())

			for _, want := range []string{"one", "two"} {
				if !bytes.Contains(got, []byte(want)) {
					t.Fatalf("comment %q was lost.\n=== out ===\n%s", want, got)
				}
			}

			twice := mustFormat(t, got, config.Default())
			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFormatKeepsHaltCompilerPayloadExact(t *testing.T) {
	payload := "RAW BYTES NOT PHP {[(\n\x00\xff bin\r\r\n end"
	src := []byte("<?php\n$x=1;\n__halt_compiler();\n" + payload)

	got := mustFormat(t, src, config.Default())

	head := []byte("<?php\n\n$x = 1;\n__halt_compiler();\n")
	if !bytes.HasPrefix(got, head) {
		t.Fatalf("the code before the call was not formatted:\n%q", got)
	}

	if tail := string(got[len(head):]); tail != payload {
		t.Fatalf("payload altered.\n=== got ===\n%q\n=== want ===\n%q", tail, payload)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
	}
}

// The construct is only the construct at statement position. PHP 7 let reserved
// words name members, so these are ordinary calls and must not swallow the file.
func TestFormatHaltCompilerAsMemberIsAnOrdinaryCall(t *testing.T) {
	for name, src := range map[string]string{
		"method": "<?php\n$a->__halt_compiler();\n$b = 2;\n",
		"static": "<?php\nPhar::__halt_compiler();\n$b = 2;\n",
		"const":  "<?php\n\nclass C\n{\n    const __halt_compiler = 1;\n}\n",
	} {
		t.Run(name, func(t *testing.T) {
			got := mustFormat(t, []byte(src), config.Default())

			if !bytes.Contains(got, []byte("$b = 2;")) && !bytes.Contains(got, []byte("const __halt_compiler = 1;")) {
				t.Fatalf("the code after the call was swallowed:\n%s", got)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

// The call itself goes out as written. Phar reads the stub looking for this
// token and php-src has an open bug where that read is case-sensitive, so
// lowercasing a `__HALT_COMPILER();` could stop an archive being readable.
func TestFormatHaltCompilerCallIsVerbatim(t *testing.T) {
	for _, call := range []string{"__HALT_COMPILER();", "__halt_compiler();", "__Halt_Compiler ( ) ;"} {
		t.Run(call, func(t *testing.T) {
			got := mustFormat(t, []byte("<?php\n"+call+"\nDATA"), config.Default())

			if !bytes.Contains(got, []byte(call)) {
				t.Fatalf("the call was rewritten:\n%q", got)
			}
		})
	}
}

func TestFormatIgnoreDirectiveClassifiesTheSameEachPass(t *testing.T) {
	for name, src := range map[string]string{
		"hash comment ending in a block closer": "<?php ;#laravel-fmt-ignore*/ \n0;",
		"line comment ending in a block closer": "<?php\n$a=1+2;// laravel-fmt-ignore*/ \n$b=3+4;\n",
	} {
		t.Run(name, func(t *testing.T) {
			once := mustFormat(t, []byte(src), config.Default())

			if twice := mustFormat(t, once, config.Default()); !bytes.Equal(twice, once) {
				t.Fatalf("not idempotent.\n=== once ===\n%q\n=== twice ===\n%q", once, twice)
			}
		})
	}
}

// The directive still freezes the node it precedes, in each comment form.
func TestFormatIgnoreDirectiveStillFreezesItsNode(t *testing.T) {
	for _, comment := range []string{
		"# laravel-fmt-ignore",
		"// laravel-fmt-ignore",
		"/* laravel-fmt-ignore */",
		"/** laravel-fmt-ignore */",
	} {
		t.Run(comment, func(t *testing.T) {
			got := mustFormat(t, []byte("<?php\n"+comment+"\n$x=1+2;\n"), config.Default())

			if !bytes.Contains(got, []byte("$x=1+2;")) {
				t.Fatalf("the ignored statement was reformatted:\n%s", got)
			}
		})
	}
}

// Both shapes force the width path with an author-broken array in a chain that
// would otherwise fit.
func TestFormatChainNoBreakRunSharesOneIndent(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"call after a property": {
			"<?php 0()->A->A(([\n0]))->A([\n1])->A;",
			"<?php\n\n0()->A->A(\n    ([\n        0,\n    ]),\n)\n    ->A([\n        1,\n    ])\n    ->A;\n",
		},
		"bare call after a hug": {
			"<?php A->A()([\n0])->A([\n1])->A;",
			"<?php\n\nA\n    ->A()([\n        0,\n    ])\n    ->A([\n        1,\n    ])\n    ->A;\n",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			once := mustFormat(t, []byte(tc.src), config.Default())

			// The run's closing bracket lands back at the run's own indent, one
			// level out from its contents, rather than two.
			if string(once) != tc.want {
				t.Fatalf("unexpected layout.\n=== got ===\n%q\n=== want ===\n%q", once, tc.want)
			}

			if twice := mustFormat(t, once, config.Default()); !bytes.Equal(twice, once) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
			}
		})
	}
}

func TestFormatConcatSpacesOnlyTheSideThatTouchesANumber(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"integer before", "$x = $n - 32 . '';", "$x = $n - 32 .'';"},
		{"integer after", "$x = 'a' . 32;", "$x = 'a'. 32;"},
		{"integer inside a run", "$x = $y . 32 . 'b';", "$x = $y. 32 .'b';"},
		{"octal before", "$x = 017 . 'a';", "$x = 017 .'a';"},
		{"int cast", "$x = (int) 1 . $y;", "$x = (int) 1 .$y;"},
		{"float cast", "$x = (float) 0 . 'a';", "$x = (float) 0 .'a';"},
		{"float before", "$x = 1.5 . 'a';", "$x = 1.5.'a';"},
		// Pint drops this space and breaks the code; `.1` would lex as a float.
		{"float after", "$x = 'a' . 1.5;", "$x = 'a'. 1.5;"},
		{"nothing numeric", "$x = (bool) $a . $b;", "$x = (bool) $a.$b;"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte("<?php\n\n"+tc.src+"\n"), config.Default())

			if want := "<?php\n\n" + tc.want + "\n"; string(got) != want {
				t.Fatalf("got %q, want %q", got, want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent: %q then %q", got, twice)
			}
		})
	}
}

func TestArraySyntaxShort(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"long array to short", "<?php\n\n$x = array(1, 2);\n", "<?php\n\n$x = [1, 2];\n"},
		{"empty long array to short", "<?php\n\n$x = array();\n", "<?php\n\n$x = [];\n"},
		{"nested long arrays to short", "<?php\n\n$x = array(array(1), 2);\n", "<?php\n\n$x = [[1], 2];\n"},
		{"short array stays short", "<?php\n\n$x = [1, 2];\n", "<?php\n\n$x = [1, 2];\n"},
		{"list destructuring is untouched", "<?php\n\nlist($a, $b) = $c;\n", "<?php\n\nlist($a, $b) = $c;\n"},
		{"short-list destructuring is untouched", "<?php\n\n[$a, $b] = $c;\n", "<?php\n\n[$a, $b] = $c;\n"},
		{"comment before array keyword is preserved", "<?php\n\n$x = /* c */ array(1);\n", "<?php\n\n$x = /* c */ [1];\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.input), config.Default())

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

func TestArraySyntaxLongPreservedWhenDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.ShortArray = false

	input := []byte("<?php\n\n$x = array(1, 2);\n")
	want := "<?php\n\n$x = array(1, 2);\n"

	got := mustFormat(t, input, cfg)

	if string(got) != want {
		t.Fatalf("output mismatch.\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}
}

func TestDocCommentReindent(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single-line docblock is left verbatim",
			input: "<?php\n\n/** Single line. */\nfunction foo() {}\n",
			want:  "<?php\n\n/** Single line. */\nfunction foo() {}\n",
		},
		{
			name:  "stars realign to the construct indent",
			input: "<?php\n\nfunction foo()\n{\n/**\n* note\n*\n* @return void\n*/\n    return;\n}\n",
			want:  "<?php\n\nfunction foo()\n{\n    /**\n     * note\n     *\n     * @return void\n     */\n    return;\n}\n",
		},
		{
			name:  "over-indented stars collapse to the canonical column",
			input: "<?php\n\n/**\n        * over-indented\n */\nfunction foo() {}\n",
			want:  "<?php\n\n/**\n * over-indented\n */\nfunction foo() {}\n",
		},
		{
			name:  "a line without a leading star bails to verbatim",
			input: "<?php\n\n/**\n * Diagram:\n   raw line\n */\nfunction foo() {}\n",
			want:  "<?php\n\n/**\n * Diagram:\n   raw line\n */\nfunction foo() {}\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.input), config.Default())

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

func TestLastArgHugArrayBudgetFollowsWidth(t *testing.T) {
	src := []byte("<?php\n\n" +
		"$a = $this->log($file, EventAction::FileLocked, FileAccessChannel::Direct, " +
		"['file_name' => $file->name, 'size' => $file->size]);\n" +
		"$b = $this->log($file, EventAction::FileRestored, FileAccessChannel::Direct, " +
		"['file_name' => $file->name, 'size' => $file->size]);\n")

	got := mustFormat(t, src, config.Default())

	if n := bytes.Count(got, []byte("Direct, [\n")); n != 2 {
		t.Fatalf("array last-argument hugged %d of 2 times:\n%s", n, got)
	}

	twice := mustFormat(t, got, config.Default())

	if !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestLastArgHugHeadWidthGuard(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"wide-callee call last-arg explodes instead of hugging over-width",
			"<?php\n\n$out[] = $service->record($firstArgumentValueHere, $secondArgumentValueHere, $this->buildDescription($alpha, $bravo, $charlie, $delta));\n",
			"<?php\n\n$out[] = $service->record(\n    $firstArgumentValueHere,\n    $secondArgumentValueHere,\n    $this->buildDescription($alpha, $bravo, $charlie, $delta),\n);\n",
		},
		{
			"array last-arg with the same leading still hugs",
			"<?php\n\n$out[] = $service->record($firstArgumentValueHere, $secondArgumentValueHere, ['a' => $alpha, 'b' => $bravo, 'c' => $charlie]);\n",
			"<?php\n\n$out[] = $service->record($firstArgumentValueHere, $secondArgumentValueHere, [\n    'a' => $alpha,\n    'b' => $bravo,\n    'c' => $charlie,\n]);\n",
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

func configForFixture(base string) *config.File {
	cfg := config.Default()

	switch {
	case strings.HasPrefix(base, "align_fat_arrow"):
		cfg.PHP.AlignFatArrow = true
	case base == "blank_line_before_return":
		cfg.PHP.BlankLineBeforeStatements = []string{config.StmtReturn}
	case base == "blank_line_before_statements":
		cfg.PHP.BlankLineBeforeStatements = []string{config.StmtContinue, config.StmtIf, config.StmtReturn, config.StmtThrow}
	case base == "brace_always_next_line":
		cfg.PHP.FunctionBraceStyle = config.BraceStyleAlwaysNextLine
		cfg.PHP.MethodBraceStyle = config.BraceStyleAlwaysNextLine
	case base == "concat_space_none":
		cfg.PHP.ConcatSpace = false
	case base == "control_brace_next_line":
		cfg.PHP.ControlBraceStyle = config.BraceStyleNextLine
	case base == "keep_trailing_close_tag":
		cfg.PHP.RemoveTrailingCloseTag = false
	case base == "no_sort_no_expand":
		cfg.PHP.SortUses = false
		cfg.PHP.ExpandUseGroups = false
	case base == "single_quote_prefer_double":
		cfg.QuoteStyle = config.QuoteStyleDouble
	}

	return cfg
}

// TestRegenerateGoldens rewrites every `.formatted.php` fixture from the doc path
// (the production formatter). Gated by REGEN=1; run once after an intentional
// formatting change, then review. A default `X.php` regenerates with the default
// config; a config fixture `X_input.php` uses configForFixture.
func TestRegenerateGoldens(t *testing.T) {
	if os.Getenv("REGEN") != "1" {
		t.Skip("set REGEN=1 to rewrite golden fixtures")
	}

	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".php") || strings.HasSuffix(name, ".formatted.php") {
			continue
		}

		base := strings.TrimSuffix(name, ".php")

		var golden string

		cfg := config.Default()

		switch {
		case strings.HasSuffix(base, "_input"):
			b := strings.TrimSuffix(base, "_input")
			golden = b + ".formatted.php"
			cfg = configForFixture(b)
		default:
			golden = base + ".formatted.php"
		}

		goldenPath := filepath.Join("testdata", golden)
		if _, err := os.Stat(goldenPath); err != nil {
			continue // no existing golden (round-trip/fuzz-only input): don't create one
		}

		src := formattertest.ReadFixture(t, name)

		out, ferr := Format(src, cfg)
		if ferr != nil {
			continue // invalid-source fixture; leave its golden alone
		}

		if err := os.WriteFile(goldenPath, out, 0o644); err != nil {
			t.Fatalf("write %s: %v", golden, err)
		}
	}
}
