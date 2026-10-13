package php

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

func FuzzFormat(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".php")
	cfg := config.Default()

	f.Fuzz(func(t *testing.T, src []byte) {
		formattertest.CheckIdempotent(t, func(b []byte) ([]byte, error) {
			return Format(b, cfg)
		}, src)

		checkCommentsKept(t, src, cfg)
	})
}

// A project running Pint with aligned arrows reaches layout paths the
// default config never does.
func FuzzFormatPint(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".php")
	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true
	cfg.PHP.PintAlignment = true
	cfg.PHP.ArrowValueOnKeyLine = true

	f.Fuzz(func(t *testing.T, src []byte) {
		formattertest.CheckIdempotent(t, func(b []byte) ([]byte, error) {
			return Format(b, cfg)
		}, src)
	})
}

// checkCommentsKept fails when formatting src loses a comment. Kinds are
// counted rather than texts compared, since a block comment's continuation
// lines re-home and a docblock's types normalize.
func checkCommentsKept(t *testing.T, src []byte, cfg *config.File) {
	t.Helper()

	out, err := Format(src, cfg)
	if err != nil {
		return
	}

	before, after := commentKinds(src), commentKinds(out)
	if before != after {
		t.Fatalf("comments lost: %v before, %v after.\n=== src ===\n%s\n=== out ===\n%s", before, after, src, out)
	}
}

func commentKinds(src []byte) [3]int {
	tokens, _ := parser.Lex(src)

	var counts [3]int

	for _, tok := range tokens {
		for _, tr := range tok.Leading {
			switch tr.Kind {
			case parser.TriviaLineComment:
				counts[0]++
			case parser.TriviaBlockComment:
				counts[1]++
			case parser.TriviaDocComment:
				counts[2]++
			case parser.TriviaWhitespace:
			}
		}
	}

	return counts
}

// mixedFuzzSeeds are templates that interleave inline HTML with PHP islands, to
// steer the fuzzer into buildMixed (the mixed HTML/PHP layout path).
var mixedFuzzSeeds = []string{
	`<a href="<?= $link ?>" class="btn">Go</a>`,
	"<title><?= $t ?></title>\n<meta content=\"<?= $d ?>\">",
	"<?php foreach ($x as $y): ?>\n<li><?= $y ?></li>\n<?php endforeach; ?>",
	"<?php if ($a) { ?><span><?= $b ?></span><?php } ?>",
	"<pre>\n    <?= $code ?>\n</pre>",
	"<?php $x = 1; ?>\n<div><?php echo $x; ?></div>",
	"<?= 1 + 2 ?><?= 3 * 4 ?>",
	"text\n<?php echo 'mid'; ?>\ntext",
	"<input value=\"<?php echo htmlspecialchars($v); ?>\">",
	"<?php\n// c\n$a = 1;\n?>\n<b><?= $a ?></b>",
	"<?php switch ($x) { case 1: ?>one<?php break; } ?>",
	"<option <?php if ($s) echo 'selected'; ?>>x</option>",
	"<?php declare(strict_types=1); ?>\n<html><?= $body ?></html>",
	"<script>var x = <?= json_encode($d) ?>;</script>",
	"<?php while ($i--): ?>a<?php endwhile; ?>",
}

// FuzzFormatMixed skips non-mixed inputs so the fuzzer's budget stays on the
// mixed HTML/PHP path.
func FuzzFormatMixed(f *testing.F) {
	for _, s := range mixedFuzzSeeds {
		f.Add([]byte(s))
	}

	cfg := config.Default()

	f.Fuzz(func(t *testing.T, src []byte) {
		file, errs := parser.Parse(src)
		if len(errs) > 0 || !isMixedMode(file.Stmts) {
			return
		}

		formattertest.CheckIdempotent(t, func(b []byte) ([]byte, error) {
			return Format(b, cfg)
		}, src)
	})
}

func FuzzBuildDoc(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".php")
	cfg := config.Default()

	f.Fuzz(func(t *testing.T, src []byte) {
		file, errs := parser.Parse(src)
		if len(errs) > 0 {
			return
		}

		first := BuildFile(file, cfg)

		reparsed, errs := parser.Parse(first)
		if len(errs) > 0 {
			t.Fatalf("doc output does not parse:\n%s", first)
		}

		second := BuildFile(reparsed, cfg)
		if !bytes.Equal(first, second) {
			t.Fatalf("doc path not idempotent:\n--- first ---\n%s\n--- second ---\n%s", first, second)
		}
	})
}

func FuzzBuildExpr(f *testing.F) {
	for _, seed := range []string{"$a + $b", "$x->y()", "(int) $z", "$a /* c */ + $b", "fn ($i) => $i->id", "['k' => $v]", "$a ?? 'd'"} {
		f.Add([]byte(seed))
	}

	cfg := config.Default()

	f.Fuzz(func(t *testing.T, src []byte) {
		expr, tokens, errs := parser.ParseFullExpression(src)
		if len(errs) > 0 || expr == nil {
			return
		}

		first := BuildExpr(src, tokens, expr, cfg, 0)

		reExpr, reTokens, errs := parser.ParseFullExpression(first)
		if len(errs) > 0 || reExpr == nil {
			t.Fatalf("BuildExpr output does not parse as an expression:\n%q", first)
		}

		second := BuildExpr(first, reTokens, reExpr, cfg, 0)
		if !bytes.Equal(first, second) {
			t.Fatalf("BuildExpr not idempotent:\n--- first ---\n%q\n--- second ---\n%q", first, second)
		}
	})
}

func FuzzBuildStmts(f *testing.F) {
	for _, seed := range []string{"$a = 1;", "if ($x) {\n$y = 2;\n}", "use App\\Model;\n$c = (int) $d;", "// c\n$e = 1;"} {
		f.Add([]byte(seed), 4)
	}

	cfg := config.Default()

	f.Fuzz(func(t *testing.T, src []byte, indent int) {
		if indent < 0 || indent > 100 {
			return
		}

		stmts, tokens, errs := parser.ParseStatements(src)
		if len(errs) > 0 || len(stmts) == 0 || HasMultilineStringLiteral(tokens) {
			return
		}

		first := BuildStmts(src, tokens, stmts, cfg, indent)

		reStmts, reTokens, errs := parser.ParseStatements(first)
		if len(errs) > 0 {
			t.Fatalf("BuildStmts output does not parse:\n%s", first)
		}

		second := BuildStmts(first, reTokens, reStmts, cfg, indent)
		if !bytes.Equal(first, second) {
			t.Fatalf("BuildStmts not idempotent at indent %d:\n--- first ---\n%s\n--- second ---\n%s", indent, first, second)
		}
	})
}

// FuzzBuildDocAligned exercises the width-sensitive config the default seed leaves
// off (=> alignment, next-line braces), guarding idempotence and against panics.
func FuzzBuildDocAligned(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".php")

	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true
	cfg.PHP.ControlBraceStyle = config.BraceStyleNextLine

	f.Fuzz(func(t *testing.T, src []byte) {
		file, errs := parser.Parse(src)
		if len(errs) > 0 {
			return
		}

		first := BuildFile(file, cfg)

		reparsed, errs := parser.Parse(first)
		if len(errs) > 0 {
			t.Fatalf("doc output does not parse:\n%s", first)
		}

		if second := BuildFile(reparsed, cfg); !bytes.Equal(first, second) {
			t.Fatalf("aligned doc path not idempotent:\n--- first ---\n%s\n--- second ---\n%s", first, second)
		}
	})
}
