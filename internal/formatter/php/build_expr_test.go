package php

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestCommentInsideAnOperandKeepsTheLayout(t *testing.T) {
	const comment = "            // note\n"

	cases := []struct {
		name, expr string
	}{
		{"coalesce", "$contents ?? "},
		{"concatenation", "$contents . "},
		{"ternary", "$contents ? $fallbackValue : "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "<?php\n\nfunction g()\n{\n    $m = json_encode(" + tc.expr + "[\n" +
				"        'a' => [\n" + comment + "            'b' => 1,\n        ],\n    ], JSON_PRETTY_PRINT);\n}\n"

			got := string(mustFormat(t, []byte(src), config.Default()))
			bare := string(mustFormat(t, []byte(strings.Replace(src, comment, "", 1)), config.Default()))

			line := strings.TrimLeft(comment, " ")
			if !strings.Contains(got, line) || removeLine(got, line) != bare {
				t.Fatalf("the comment changed the layout.\n=== with ===\n%s\n=== without ===\n%s", got, bare)
			}

			if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestCommentInAnOperandIsLaidOutInEveryLayoutChoice(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{
			"single operator",
			"<?php\n\n$x = $y % f(\"a\" #c\n. \"b\");\n",
			"<?php\n\n$x = $y % f(\n    'a' #c\n        .'b',\n);\n",
		},
		{
			"operator before a block",
			"<?php\n\n$x = $y ?? f('a', [\n    // c\n    'b' => 1,\n]);\n",
			"<?php\n\n$x = $y ?? f('a', [\n    // c\n    'b' => 1,\n]);\n",
		},
		{
			"operator glued to a chain",
			"<?php\n\n$x = $a & B::c($d)->e(function () { return 1; }) % f(function (#c\nA $b) {});\n",
			"<?php\n\n$x = $a\n    & B::c($d)->e(function () {\n        return 1;\n    }) % f(\n" +
				"        function (\n            #c\n            A $b,\n        ) {},\n    );\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(mustFormat(t, []byte(tc.src), config.Default()))
			if got != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}

			if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestOperandGluedToTheHeadsClosingLine(t *testing.T) {
	const fn = "<?php\n\nfunction g()\n{\n"
	const end = "}\n"
	const long = "$this->pages()->map(fn ($page) => similar_text($page, $search))" +
		"->filter(fn ($score) => $score >= 3)->sortDesc()->first()"

	chainHead := "    return $this->pages()\n" +
		"        ->filter(fn ($page) => str_starts_with(\n" +
		"            Str::slug($page['title'], ' '),\n" +
		"            Str::slug($search, ' '),\n" +
		"        ))\n" +
		"        ->keys()\n" +
		"        ->first() ?? $this->pages()\n"
	links := []string{
		"->map(fn ($page) => similar_text($page, $search))\n",
		"->filter(fn ($score) => $score >= 3)\n",
		"->sortDesc()\n",
		"->first();\n",
	}
	ladder := func(indent string) string {
		var sb strings.Builder
		for _, l := range links {
			sb.WriteString(indent)
			sb.WriteString(l)
		}

		return sb.String()
	}
	closerHead := "    $x = collect([\n        'one',\n        'two',\n    ]) ?? "
	closureLink := "$this->pages(function ($page) { return similar_text($page, $search); })" +
		"->filter(fn ($score) => $score >= 3)->first();\n"
	closureLinkBroken := "$this->pages(function ($page) {\n            return similar_text($page, $search);\n" +
		"        })->filter(fn ($score) => $score >= 3)->first();\n"

	cases := []struct {
		name string
		pint bool
		src  string
		want string
	}{
		{
			"a chain after a closing bracket",
			false,
			fn + closerHead + long + ";\n" + end,
			fn + closerHead + "$this->pages()\n" + ladder("        ") + end,
		},
		{
			"a call after a closing bracket",
			false,
			fn + closerHead + "$this->pages($aaaaaaaaaaaaaaaaaaaaaaaaaaaaa, " +
				"$bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb);\n" + end,
			fn + closerHead + "$this->pages(\n        $aaaaaaaaaaaaaaaaaaaaaaaaaaaaa,\n" +
				"        $bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb,\n    );\n" + end,
		},
		{
			"a call after a closure's closing brace",
			false,
			"<?php\n\n$x = $a & B::c($d)->e(function () { return 1; }) % f(function (A $b) { return 2; }, $second);\n",
			"<?php\n\n$x = $a\n    & B::c($d)->e(function () {\n        return 1;\n    }) % f(function (A $b) {\n" +
				"        return 2;\n    }, $second);\n",
		},
		{
			"a chain after a chain",
			false,
			fn + strings.TrimSuffix(chainHead, "$this->pages()\n") + long + ";\n" + end,
			fn + chainHead + ladder("            ") + end,
		},
		{
			"a chain after a chain, with Pint's method_chaining_indentation",
			true,
			fn + strings.TrimSuffix(chainHead, "$this->pages()\n") + long + ";\n" + end,
			fn + chainHead + ladder("        ") + end,
		},
		{
			"a closure in a glued chain's first link",
			false,
			fn + strings.TrimSuffix(chainHead, "$this->pages()\n") + closureLink + end,
			fn + strings.TrimSuffix(chainHead, "$this->pages()\n") + closureLinkBroken + end,
		},
		{
			"a closure in a glued chain's first link, with Pint's method_chaining_indentation",
			true,
			fn + strings.TrimSuffix(chainHead, "$this->pages()\n") + closureLink + end,
			fn + strings.TrimSuffix(chainHead, "$this->pages()\n") + closureLinkBroken + end,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.PHP.ChainIndentFromLine = tc.pint

			got := string(mustFormat(t, []byte(tc.src), cfg))
			if got != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}

			if twice := string(mustFormat(t, []byte(got), cfg)); twice != got {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestCommentBetweenOperandsIsKeptAsWritten(t *testing.T) {
	src := "<?php\n\n$m = $contents ?? // note\n    $fallback;\n"

	if got := string(mustFormat(t, []byte(src), config.Default())); got != src {
		t.Fatalf("got:\n%s\nwant:\n%s", got, src)
	}
}

func TestYieldFrom(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"operand is formatted", "yield from   foo( $a,[1,2] );", "yield from foo($a, [1, 2]);"},
		{"keyword is normalized", "$x = YIELD \t FROM bar( 1 );", "$x = yield from bar(1);"},
		{"comment glued to yield", "yield/*c*/ from foo( 1 );", "yield/*c*/ from foo( 1 );"},
		{"comment glued to from", "yield /*c*/from foo( 1 );", "yield /*c*/from foo( 1 );"},
		{"line comment glued to yield", "yield//c\n    from foo( 1 );", "yield//c\n    from foo( 1 );"},
		{"comment before the operand", "yield from /* c */ foo( 1 );", "yield from /* c */ foo( 1 );"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "<?php\n\nfunction g()\n{\n    " + tc.body + "\n}\n"
			want := "<?php\n\nfunction g()\n{\n    " + tc.want + "\n}\n"

			got := mustFormat(t, []byte(src), config.Default())
			if string(got) != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if again := mustFormat(t, got, config.Default()); string(again) != want {
				t.Fatalf("not idempotent:\n%s", again)
			}
		})
	}
}

func TestClosureAfterOperatorStaysOnTheOperatorsLine(t *testing.T) {
	long := "$this->someVeryLongPropertyName->anotherVeryLongPropertyNameThatIsLongerStillAndMore"
	arg := "$this->someVeryLongPropertyName->anotherVeryLongPropertyNameHereLonger"

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"short ternary",
			"$c = $a->set\n    ?: function ($v) use ($k) {\n        return 1;\n    };\n",
			"$c = $a->set ?: function ($v) use ($k) {\n    return 1;\n};\n",
		},
		{
			"coalesce run",
			"$c = $a ?? $b ?? function () {\n    return 1;\n};\n",
			"$c = $a ?? $b ?? function () {\n    return 1;\n};\n",
		},
		{
			"argument hugs",
			"$c = f($a ?: function ($v) {\n    return $v;\n});\n",
			"$c = f($a ?: function ($v) {\n    return $v;\n});\n",
		},
		{
			"array item with a comment-only body",
			"$c = [\n    $a[$k] ?? static function () {\n        //\n    },\n];\n",
			"$c = [\n    $a[$k] ?? static function () {\n        //\n    },\n];\n",
		},
		{
			"too wide moves the operator down",
			"$c = " + long + " ?? function (string $name) use ($k) {\n    return 1;\n};\n",
			"$c = " + long + "\n    ?? function (string $name) use ($k) {\n        return 1;\n    };\n",
		},
		{
			"too wide as an argument breaks the list",
			"$c = call_user_func(" + arg + " ?: function ($value) use ($key) {\n    return 1;\n});\n",
			"$c = call_user_func(\n    " + arg + " ?: function ($value) use ($key) {\n        return 1;\n    },\n);\n",
		},
		{
			"closure before the last operand",
			"$c = $a ?? $b->c(function () {\n    return 1;\n}) ?? function () {\n    return 2;\n};\n",
			"$c = $a\n    ?? $b->c(function () {\n        return 1;\n    })\n    ?? function () {\n        return 2;\n    };\n",
		},
		{
			"nested past the bound keeps the operator down",
			"$c = $a ?? function () {\n    return $b ?? function () {\n        return $d ?? function () {\n" +
				"            return $e ?? function () {\n                return 1;\n            };\n        };\n    };\n};\n",
			"$c = $a ?? function () {\n    return $b ?? function () {\n        return $d ?? function () {\n" +
				"            return $e\n                ?? function () {\n                    return 1;\n                };\n" +
				"        };\n    };\n};\n",
		},
		{
			"match is left alone",
			"$c = $a\n    ?? match ($x) {\n        1 => 'a',\n    };\n",
			"$c = $a\n    ?? match ($x) {\n        1 => 'a',\n    };\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := "<?php\n\n" + tc.want

			got := mustFormat(t, []byte("<?php\n\n"+tc.src), config.Default())
			if string(got) != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if again := mustFormat(t, got, config.Default()); string(again) != want {
				t.Fatalf("not idempotent:\n%s", again)
			}
		})
	}
}

func removeLine(s, line string) string {
	i := strings.Index(s, line)
	start := strings.LastIndexByte(s[:i], '\n') + 1

	return s[:start] + s[i+len(line):]
}

func TestHeredocIndentationModes(t *testing.T) {
	src := "<?php\n\nfunction f()\n{\n    $sql = <<<'SQL'\nSELECT 1;\nSQL;\n}\n"

	cases := []struct {
		mode string
		body string
	}{
		{config.HeredocIndentStartPlusOne, "        SELECT 1;\n        SQL;\n"},
		{config.HeredocIndentSameAsStart, "    SELECT 1;\n    SQL;\n"},
		{config.HeredocIndentNone, "SELECT 1;\nSQL;\n"},
	}

	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			cfg := config.Default()
			cfg.PHP.HeredocIndentation = tc.mode

			want := "<?php\n\nfunction f()\n{\n    $sql = <<<'SQL'\n" + tc.body + "}\n"

			if got := string(mustFormat(t, []byte(src), cfg)); got != want {
				t.Errorf("got\n%s\nwant\n%s", got, want)
			}
		})
	}
}

func TestBinaryKeepsItsOperatorOnTheLineOpeningABlockThatBreaksAnyway(t *testing.T) {
	wide := "['xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx' => 1, 'yyyyyyyyyyyyyyyyyyyyyyyyyy' => 2, 'zzzzzzzzzzzzzzzzzzzzzz' => 3]"
	long := "$" + strings.Repeat("v", 103)

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"author-broken array",
			"$a = $b ?? Foo::bar([\n    'x' => 1,\n]);",
			"$a = $b ?? Foo::bar([\n    'x' => 1,\n]);",
		},
		{
			"array too wide for its own line",
			"$a = $b ?? Foo::bar(" + wide + ");",
			"$a = $b ?? Foo::bar([\n    'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx' => 1,\n    'yyyyyyyyyyyyyyyyyyyyyyyyyy' => 2,\n" +
				"    'zzzzzzzzzzzzzzzzzzzzzz' => 3,\n]);",
		},
		{
			"operand that fits its own line moves down",
			"$aaaaaaaaaaaaaaaaaaaaaaaa = $this->reference($query, $order->number) ?? $this->score($query, $order->number, [$v, $order->notes]);",
			"$aaaaaaaaaaaaaaaaaaaaaaaa = $this->reference($query, $order->number)\n    ?? $this->score($query, $order->number, [$v, $order->notes]);",
		},
		{
			"operand that fits its own line only without the statement's closers",
			"$a = (($b ?? score(" + long + ")));",
			"$a = (($b ?? score(\n    " + long + ",\n)));",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(mustFormat(t, []byte("<?php\n\n"+tc.src+"\n"), config.Default()))
			want := "<?php\n\n" + tc.want + "\n"

			if got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestBinaryOperandChainKeepsItsAuthorLadder(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"head before a short tail", "$max = Foo::query()\n    ->where('sheet_id', $sheet->id)\n    ->max('sort_order') ?? -1;"},
		{"head before a breakable tail", "$date = $this->sheet\n    ->day\n    ?->date\n    ?->format('M j, Y') ?? __('common.tbd');"},
		{"property read off the ladder", "return $this->items\n    ->firstWhere('id', $user->id)\n    ?->pivot\n    ?->pinned_at !== null;"},
		{"later operand of a broken run", "$ok = $aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n" +
			"    && $event->user->teams()\n        ->where('teams.id', $id)\n        ->exists();"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "<?php\n\n" + tc.src + "\n"
			if got := string(mustFormat(t, []byte(src), config.Default())); got != src {
				t.Fatalf("got:\n%s\nwant:\n%s", got, src)
			}
		})
	}
}

func TestOperatorRidesTheCloserOfABrokenBlock(t *testing.T) {
	long := "$" + strings.Repeat("a", 110)
	match := "match ($x) {\n    1 => 'one',\n    default => null,\n}"
	window := "$item->window?->between(now()->addSeconds(4), now()->addSeconds(6)"

	cases := []struct {
		name string
		src  string
		want string
	}{
		{"coalesce after a match", "return " + match + " ?? 0;", "return " + match + " ?? 0;"},
		{"elvis after a call ending in an array", "return foo([\n    'a' => 1,\n]) ?: null;", "return foo([\n    'a' => 1,\n]) ?: null;"},
		{"elvis after a flat condition that fits", "return $a ?: null;", "return $a ?: null;"},
		{"elvis after a long flat condition", "return " + long + " ?: null;", "return " + long + "\n    ?: null;"},
		{"full ternary after a match", "return " + match + " ? 'yes' : 'no';", "return " + match + "\n    ? 'yes'\n    : 'no';"},
		{"comparison after a property of a broken call", "return foo(\n    $alpha,\n)->key === $role;", "return foo(\n    $alpha,\n)->key === $role;"},
		{
			"comparison with a negative literal breaks the call before it",
			"$f = fn (Item $item): bool => " + window + ", $marginOfError) === -1;",
			"$f = fn (Item $item): bool => $item->window?->between(\n    now()->addSeconds(4),\n    now()->addSeconds(6),\n    $marginOfError,\n) === -1;",
		},
		{
			"comparison with an operand of its own leads its line",
			"$f = fn (Item $item): bool => " + window + ", $margin) === $expectedValue;",
			"$f = fn (Item $item): bool => " + window + ", $margin)\n    === $expectedValue;",
		},
		{
			"comparison with a short string breaks the call before it",
			"$f = fn (Item $item): bool => " + window + ", $margin) === '" + strings.Repeat("x", 18) + "';",
			"$f = fn (Item $item): bool => $item->window?->between(\n    now()->addSeconds(4),\n    now()->addSeconds(6),\n    $margin,\n) === '" + strings.Repeat("x", 18) + "';",
		},
		{
			"comparison with a string one byte longer leads its line",
			"$f = fn (Item $item): bool => " + window + ", $margin) === '" + strings.Repeat("x", 19) + "';",
			"$f = fn (Item $item): bool => " + window + ", $margin)\n    === '" + strings.Repeat("x", 19) + "';",
		},
		{
			"comparison with a literal after a parenthesised assignment leads its line",
			"while (($line = fgets($this->" + strings.Repeat("h", 50) + ", $this->maximumLineLengthInBytes)) !== false) {\n    foo();\n}",
			"while (\n    ($line = fgets($this->" + strings.Repeat("h", 50) + ", $this->maximumLineLengthInBytes))\n    !== false\n) {\n    foo();\n}",
		},
		{
			"arithmetic with a literal leads its line",
			"$f = fn (Item $item): bool => " + window + ", $margin) + 1000000000000;",
			"$f = fn (Item $item): bool => " + window + ", $margin)\n    + 1000000000000;",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(mustFormat(t, []byte("<?php\n\n"+tc.src+"\n"), config.Default()))

			if want := "<?php\n\n" + tc.want + "\n"; got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestCoalesceKeepsTheLineOpeningACallsArguments(t *testing.T) {
	const long = "SomeLongFactoryName::makeFromTheseArguments("
	const args = "$firstArgumentValue, $secondArgumentValue, $thirdArgumentValue"

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"throw before a call the author broke",
			"return $version ?? throw StaleRecordException::unverifiable(\n    self::keyToString($this->getKey())\n);",
			"return $version ?? throw StaleRecordException::unverifiable(\n    self::keyToString($this->getKey()),\n);",
		},
		{
			"call the author broke",
			"return $version ?? StaleRecordException::unverifiable(\n    self::keyToString($this->getKey())\n);",
			"return $version ?? StaleRecordException::unverifiable(\n    self::keyToString($this->getKey()),\n);",
		},
		{
			"throw before a call ending in an array",
			"return $version ?? throw StaleRecordException::unverifiable([\n    'key' => $key,\n]);",
			"return $version ?? throw StaleRecordException::unverifiable([\n    'key' => $key,\n]);",
		},
		{
			"throw new with broken arguments",
			"return $version ?? throw new RuntimeException(\n    'The record has no version.'\n);",
			"return $version ?? throw new RuntimeException(\n    'The record has no version.',\n);",
		},
		{"short call", "return $short ?? Foo::bar($a);", "return $short ?? Foo::bar($a);"},
		{
			"call that fits on its own line",
			"return $value ?? " + long + args + ");",
			"return $value\n    ?? " + long + args + ");",
		},
		{
			"call too long for its own line",
			"return $value ?? " + long + args + ", $fourthArgumentValue);",
			"return $value ?? " + long + "\n    $firstArgumentValue,\n    $secondArgumentValue,\n" +
				"    $thirdArgumentValue,\n    $fourthArgumentValue,\n);",
		},
		{
			"ternary default before new with broken arguments",
			"return $x === null ? null : new Definition(\n    $column\n);",
			"return $x === null ? null : new Definition(\n    $column,\n);",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(mustFormat(t, []byte("<?php\n\nfunction f()\n{\n    "+indentBody(tc.src)+"\n}\n"), config.Default()))

			if want := "<?php\n\nfunction f()\n{\n    " + indentBody(tc.want) + "\n}\n"; got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func indentBody(s string) string {
	return strings.ReplaceAll(s, "\n", "\n    ")
}

func TestGluedConcatOperandLaysOutTheSameInEveryPass(t *testing.T) {
	src := "<?php\n\nf(g((h(\n1) % ($a .\n$b))(3)));\n"
	want := "<?php\n\nf(\n    g(\n        (\n            h(\n                1,\n            ) % ($a.$b)\n        )(3),\n    ),\n);\n"

	got := string(mustFormat(t, []byte(src), config.Default()))
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestConcatBrokenAfterABrokenHeadGluesToItsCloser(t *testing.T) {
	for _, src := range []string{"<?php\n$x = f([\n1]).\nh([\n2]);\n", "<?php\n$x = f([\n1])\n. h([\n2]);\n"} {
		want := "<?php\n\n$x = f([\n    1,\n]).h([\n    2,\n]);\n"

		got := string(mustFormat(t, []byte(src), config.Default()))
		if got != want {
			t.Fatalf("got:\n%s\nwant:\n%s", got, want)
		}

		if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
			t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
		}
	}
}

func TestBinaryBreaksAroundAChainKeptBrokenInsideAnArrowFunction(t *testing.T) {
	want := "<?php\n\n$first\n    != $second\n        % (fn (Item $item, Collection $collection, Repository $repository): int => (int) $query\n" +
		"            ->where(fn ($inner) => $inner % $limit)\n            ->count());\n"
	src := "<?php\n$first != $second % (fn (Item $item, Collection $collection, Repository $repository): int => (int) $query->where(fn ($inner) => $inner % $limit)->count());\n"

	for _, in := range []string{src, want} {
		got := string(mustFormat(t, []byte(in), config.Default()))
		if got != want {
			t.Fatalf("got:\n%s\nwant:\n%s", got, want)
		}
	}
}

func TestOperandThatCannotHugMovedDownBreaksItsListGlued(t *testing.T) {
	long := "$" + strings.Repeat("a", 95)
	want := "<?php\n\n$fallbackValueName ?? build(\n    " + long + ",\n    [\n        1,\n    ],\n);\n"

	for _, src := range []string{"<?php\n$fallbackValueName ?? build(" + long + ", [\n1]);\n", want} {
		got := string(mustFormat(t, []byte(src), config.Default()))
		if got != want {
			t.Fatalf("got:\n%s\nwant:\n%s", got, want)
		}
	}
}

func TestKeptPipeIndentsFromTheContinuationLineItOpensOn(t *testing.T) {
	want := "<?php $a\n    .$b;?><?php $c\n            .$d\n        |> f(...)\n        |> g(...);?>x\n"

	for _, src := range []string{"<?php $a\n.$b?><?php $c\n.$d|>f(...)|>g(...)?>x", want} {
		got := string(mustFormat(t, []byte(src), config.Default()))
		if got != want {
			t.Fatalf("got:\n%s\nwant:\n%s", got, want)
		}
	}
}

func TestChainedAssignmentMeasuresFromItsRealColumn(t *testing.T) {
	long := "$" + strings.Repeat("a", 113)
	src := "<?php\n\nf([] = [" + long + "] = $b);\n"
	want := "<?php\n\nf(\n    [] = [\n        " + long + ",\n    ] = $b,\n);\n"

	got := string(mustFormat(t, []byte(src), config.Default()))
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestValueAssignedToAListKeepsItsLayoutOnTheNextRun(t *testing.T) {
	inner, value := "$"+strings.Repeat("c", 40), "$"+strings.Repeat("a", 70)
	chain := "($q = " + inner + " = " + value + " + $m);"

	cases := []struct{ name, src, want string }{
		{
			"list that can break",
			"[$p] = " + chain,
			"[$p] =\n    ($q = " + inner + " = " + value + "\n        + $m);",
		},
		{
			"list that stays flat",
			"[] = " + chain,
			"[] = ($q = " + inner + "\n    = " + value + " + $m);",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(mustFormat(t, []byte("<?php\n\n"+tc.src+"\n"), config.Default()))
			if want := "<?php\n\n" + tc.want + "\n"; got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestChainedAssignmentMeasuresABrokenListTargetFlat(t *testing.T) {
	cases := []struct {
		name string
		long string
		tail string
		want string
	}{
		{"tail stays beside the closing line", "$" + strings.Repeat("a", 97), "[] = [][]", "] = [] = [][],"},
		{"tail the first pass put below stays below", "$" + strings.Repeat("a", 103), "[] = []", "] = [] =\n        [],"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "<?php\n\nf($b = [" + tc.long + "] = " + tc.tail + ");\n"
			want := "<?php\n\nf(\n    $b = [\n        " + tc.long + ",\n    " + tc.want + "\n);\n"

			got := string(mustFormat(t, []byte(src), config.Default()))
			if got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestAssignmentAsABinaryOperandKeepsItsLayoutOnTheNextPass(t *testing.T) {
	a, b := strings.Repeat("a", 32), strings.Repeat("b", 46)
	src := "<?php\n\n$q->a()->b([\n[]], '') % $c = run(fn ($x, $y) => $x % " + a + "() % " + b + "(\n$z));\n"
	want := "<?php\n\n$q->a()->b([\n    [],\n], '') % $c = run(\n    fn ($x, $y) => $x\n        % " + a + "()\n        % " + b +
		"(\n            $z,\n        ),\n);\n"

	got := string(mustFormat(t, []byte(src), config.Default()))
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestCommentBeforeADestructuredArrayValueIsWrittenOnce(t *testing.T) {
	src := "<?php\n\n[$a] = /* c */ [0];\n"

	if got := string(mustFormat(t, []byte(src), config.Default())); got != src {
		t.Fatalf("got:\n%s\nwant:\n%s", got, src)
	}
}

func TestOperandCallingACallResultGluesItsBrokenListOnTheFirstPass(t *testing.T) {
	long := strings.Repeat("0", 64)
	wrap := func(body string) string {
		return "<?php\n\nclass Foo\n{\n    public function run()\n    {\n        " + body + "\n    }\n}\n"
	}

	callee := "make($" + strings.Repeat("b", 34) + ")"
	src := wrap("$a % " + callee + "($f . '" + long + "', ['x']);")
	want := wrap("$a % " + callee + "(\n            $f.'" + long + "',\n            ['x'],\n        );")

	got := string(mustFormat(t, []byte(src), config.Default()))
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestTernaryElseBlockCountsTheEntryTerminator(t *testing.T) {
	long := "$" + strings.Repeat("a", 107)
	src := "<?php\n\nA([0 ? 0 : [" + long + "]]);\n"
	want := "<?php\n\nA([\n    0 ? 0 : [\n        " + long + ",\n    ],\n]);\n"

	got := string(mustFormat(t, []byte(src), config.Default()))
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestTernaryKeepsTheLineOpeningABlockBesideADefault(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"default else",
			"return [\n    'options' => extension_loaded('pdo_mysql') ? array_filter([\n        Mysql::ATTR_SSL_CA => env('MYSQL_ATTR_SSL_CA'),\n    ]) : [],\n];",
			"return [\n    'options' => extension_loaded('pdo_mysql') ? array_filter([\n        Mysql::ATTR_SSL_CA => env('MYSQL_ATTR_SSL_CA'),\n    ]) : [],\n];",
		},
		{
			"meaningful else",
			"return [\n    'options' => extension_loaded('pdo_mysql') ? array_filter([\n        Mysql::ATTR_SSL_CA => env('MYSQL_ATTR_SSL_CA'),\n    ]) : $other,\n];",
			"return [\n    'options' => extension_loaded('pdo_mysql')\n        ? array_filter([\n            Mysql::ATTR_SSL_CA => env('MYSQL_ATTR_SSL_CA'),\n        ])\n        : $other,\n];",
		},
		{
			"null else",
			"return [\n    'options' => extension_loaded('pdo_mysql') ? array_filter([\n        Mysql::ATTR_SSL_CA => env('MYSQL_ATTR_SSL_CA'),\n    ]) : null,\n];",
			"return [\n    'options' => extension_loaded('pdo_mysql') ? array_filter([\n        Mysql::ATTR_SSL_CA => env('MYSQL_ATTR_SSL_CA'),\n    ]) : null,\n];",
		},
		{
			"default then before a cast block",
			"return $item === null ? null : (object) [\n    'kind' => 'item',\n    'model' => $item,\n];",
			"return $item === null ? null : (object) [\n    'kind' => 'item',\n    'model' => $item,\n];",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(mustFormat(t, []byte("<?php\n\n"+tc.src+"\n"), config.Default()))

			if want := "<?php\n\n" + tc.want + "\n"; got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestTernaryElseBlockHugsBesideAnUnbreakableThen(t *testing.T) {
	long := strings.Repeat("a", 95)

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"variable then, array else",
			"return is_array($item) ? $item : [\n    'name' => $key,\n    'value' => $item,\n];",
			"return is_array($item) ? $item : [\n    'name' => $key,\n    'value' => $item,\n];",
		},
		{
			"variable then, cast block else",
			"return $item === null ? $fallback : (object) [\n    'kind' => 'item',\n    'model' => $item,\n];",
			"return $item === null ? $fallback : (object) [\n    'kind' => 'item',\n    'model' => $item,\n];",
		},
		{
			"property then, call hugging an array",
			"$x = $cond ? $a->b : foo([\n    'name' => $key,\n]);",
			"$x = $cond ? $a->b : foo([\n    'name' => $key,\n]);",
		},
		{
			"opening line too wide",
			"return is_array($" + long + ") ? $item : [\n    'name' => $key,\n];",
			"return is_array($" + long + ")\n    ? $item\n    : [\n        'name' => $key,\n    ];",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(mustFormat(t, []byte("<?php\n\n"+tc.src+"\n"), config.Default()))
			if want := "<?php\n\n" + tc.want + "\n"; got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestPintAlignmentPadsEveryEntryOfARunWhateverTheWidth(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"entry past the width once padded",
			"$a = [\n    'budget_line_item_id' => $line->id,\n    'description' => sprintf('Payroll %s: %s', $period->name, $timesheet->contact?->full_name_with_title ?? 'Crew'),\n];",
			"$a = [\n    'budget_line_item_id' => $line->id,\n" +
				"    'description'         => sprintf('Payroll %s: %s', $period->name, $timesheet->contact?->full_name_with_title ?? 'Crew'),\n];",
		},
		{
			"entry with a multi-line value",
			"$a = match ($this) {\n    self::Indie => ['monthly' => 'pri_sandbox_indie_monthly', 'annual' => 'pri_sandbox_indie_annual_billing_cycle'],\n    self::ProductionHouse => [\n        'monthly' => 'house',\n    ],\n};",
			"$a = match ($this) {\n    self::Indie           => ['monthly' => 'pri_sandbox_indie_monthly', 'annual' => 'pri_sandbox_indie_annual_billing_cycle'],\n    self::ProductionHouse => [\n        'monthly' => 'house',\n    ],\n};",
		},
	}

	cfg := config.Default()
	cfg.PHP.AlignFatArrow = true
	cfg.PHP.PintAlignment = true

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(mustFormat(t, []byte("<?php\n\n"+tc.src+"\n"), cfg))

			if want := "<?php\n\n" + tc.want + "\n"; got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := string(mustFormat(t, []byte(got), cfg)); twice != got {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestEmptyArrayHeadGluesNoOperator(t *testing.T) {
	long := "A" + strings.Repeat("0", 108)

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"statement",
			"[$p]=[].$z=" + long + ";",
			"[$p] = []\n    .$z = " + long + ";",
		},
		{
			"call argument",
			"f([$p]=[].[]=" + long + ");",
			"f(\n    [$p] = []\n        .[] = " + long + ",\n);",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := "<?php\n\n" + tc.want + "\n"

			got := string(mustFormat(t, []byte("<?php\n"+tc.src+"\n"), config.Default()))
			if got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestEmptyClosureBodyFollowsTheProjectsLayout(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.InlineEmptyClosureBodies = false

	src := "<?php\n\n$noop = function () {};\n"
	want := "<?php\n\n$noop = function () {\n};\n"

	got := mustFormat(t, []byte(src), cfg)
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if again := mustFormat(t, got, cfg); !bytes.Equal(again, got) {
		t.Fatalf("not idempotent:\n%s\nthen:\n%s", got, again)
	}
}

func TestClosureSignatureTooWideForAnyLineBreaksItsSingleParameter(t *testing.T) {
	sole := "$q->where(static function (\\Acme\\%s $inner) use ($first, $second): void {\n    $inner->run();\n});"
	leading := "assert($user, Code::class, function (\\Acme\\%s $item) use ($first, $second): bool {\n    return true;\n});"

	tests := []struct {
		name   string
		src    string
		want   string
		length int
	}{
		{
			name:   "sole argument un-hugs while the signature fits a line of its own",
			src:    sole,
			want:   "$q->where(\n    static function (\\Acme\\%s $inner) use ($first, $second): void {\n        $inner->run();\n    },\n);",
			length: 55,
		},
		{
			name:   "sole argument breaks its parameter once the signature fits no line",
			src:    sole,
			want:   "$q->where(static function (\n    \\Acme\\%s $inner,\n) use ($first, $second): void {\n    $inner->run();\n});",
			length: 56,
		},
		{
			name:   "after other arguments it stays hugged past the width while the signature fits a line",
			src:    leading,
			want:   leading,
			length: 63,
		},
		{
			name:   "after other arguments it breaks its parameter once the signature fits no line",
			src:    leading,
			want:   "assert($user, Code::class, function (\n    \\Acme\\%s $item,\n) use ($first, $second): bool {\n    return true;\n});",
			length: 64,
		},
		{
			name:   "a long use list breaks instead",
			src:    "$q->where(static function (\\Acme\\%s $inner) use ($first, $second, $third): void {\n    $inner->run();\n});",
			want:   "$q->where(static function (\\Acme\\%s $inner) use (\n    $first,\n    $second,\n    $third,\n): void {\n    $inner->run();\n});",
			length: 60,
		},
		{
			name:   "a long use list too wide to open on the line breaks the parameter",
			src:    "$q->where(static function (\\Acme\\%s $inner) use ($first, $second, $third): void {\n    $inner->run();\n});",
			want:   "$q->where(static function (\n    \\Acme\\%s $inner,\n) use ($first, $second, $third): void {\n    $inner->run();\n});",
			length: 80,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertLayoutAtLength(t, tt.src, tt.want, tt.length)
		})
	}
}

func TestCommentBeforeSemicolonInsideAClosureArgumentSurvives(t *testing.T) {
	src := "<?php\n\n$o = f(function ($n) {\n    return $n instanceof A\n        || $n instanceof B\n        // keep me\n    ;\n});\n"

	got := mustFormat(t, []byte(src), config.Default())
	if !bytes.Contains(got, []byte("// keep me")) {
		t.Fatalf("comment dropped:\n%s", got)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent:\n%s\n---\n%s", got, twice)
	}
}

func TestPartialApplicationPlaceholdersFormatAsArguments(t *testing.T) {
	in := "<?php\n$f = User::make( ? ,'guest');\n$g = foo(a:?, ...);\n$h = clone(...);\n" +
		"$i = someLongFunctionName($firstArgumentValue, ?, $thirdArgumentValue, $fourthArgumentValue, $fifth, $sixthArgumentValue, named: ?, ...);\n"
	want := "<?php\n\n$f = User::make(?, 'guest');\n$g = foo(a: ?, ...);\n$h = clone(...);\n" +
		"$i = someLongFunctionName(\n    $firstArgumentValue,\n    ?,\n    $thirdArgumentValue,\n    $fourthArgumentValue,\n    $fifth,\n    $sixthArgumentValue,\n    named: ?,\n    ...,\n);\n"

	got := mustFormat(t, []byte(in), config.Default())
	if string(got) != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent:\n%s\n---\n%s", got, twice)
	}
}

func TestDeeplyNestedHugsFormatWithoutExponentialCost(t *testing.T) {
	const depth = 18
	const budget = 2 * time.Second

	tests := []struct {
		name  string
		level func(inner string) string
	}{
		{
			name: "arrow functions and calls",
			level: func(inner string) string {
				return "$x->each(fn ($i) => outer($x->map(fn ($j) => " + inner + ", ['k' => 1])))"
			},
		},
		{
			name: "closures",
			level: func(inner string) string {
				return "$x->each(function ($i) { return outer($x->map(function ($j) { return " + inner + "; }, ['k' => 1])); })"
			},
		},
		{
			name:  "calls",
			level: func(inner string) string { return "outer($x->map(" + inner + ", ['k' => 1]))" },
		},
		{
			name:  "new",
			level: func(inner string) string { return "new Outer(new Inner(" + inner + ", ['k' => 1]))" },
		},
		{
			name:  "arrays",
			level: func(inner string) string { return "foo(['k' => bar(" + inner + ", [$a])])" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := "$x->" + strings.Repeat("c", 60) + "($i)"
			for range depth {
				inner = tt.level(inner)
			}

			src := []byte("<?php\n\nif (" + inner + ") {\n    foo();\n}\n")

			start := time.Now()
			got := mustFormat(t, src, config.Default())

			if took := time.Since(start); took > budget {
				t.Fatalf("formatting %d nested levels took %v, want under %v", depth, took, budget)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestCommentInsideAWidthProbeKeepsTheLayout(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "comment in an arrow function's parameters inside a closure",
			src:  "<?php\nt(function(){n(fn(#\n$s)=>f>e);});\n",
			want: "<?php\n\nt(function () {\n    n(fn (\n        #\n        $s,\n    ) => f > e);\n});\n",
		},
		{
			name: "trailing comment in a chained call's argument",
			src: "<?php class A{function a(){f(0)->a(f('00000000000000000000000000000000'#$width%'second-part-of-the-label'\n" +
				".($kind%2?'odd':'even')));}}\n",
			want: "<?php\n\nclass A\n{\n    function a()\n    {\n        f(0)->a(f(\n" +
				"            '00000000000000000000000000000000' #$width%'second-part-of-the-label'\n" +
				"                .($kind % 2 ? 'odd' : 'even'),\n        ));\n    }\n}\n",
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

func TestThrownChainKeepsItsCallOnAKeptBrokenArray(t *testing.T) {
	src := "<?php\n\nthrow Failure::withItems([\n    'key' => trans('A message long enough to need its own line.'),\n])->inBag('primary');\n"

	got := string(mustFormat(t, []byte(src), config.Default()))
	if got != src {
		t.Fatalf("got:\n%s\nwant:\n%s", got, src)
	}
}

func TestConcatOperandGluedToABrokenCloserKeepsItsIndentOnTheNextPass(t *testing.T) {
	left := "wrap(fn ($a, $b): Shape => (object) ['' => (int) $a000, '0000' => $a0000[A0000000] ?? '0', '00000' => $a00 > a0000()])"
	element := "'00' % (int) A0000000 % '0000' % A00000[A0000000] ?? '0' % '00000' % a000000000000()"
	src := "<?php\n\n$v > " + left + ".wrap(fn ($a, $b): A00 => [" + element + "]()[]);\n"

	got := mustFormat(t, []byte(src), config.Default())

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestAuthorBrokenPipeKeepsTheLadderOfItsLastOperand(t *testing.T) {
	want := "<?php\n\n$a\n    |> b()\n    |> c()\n        ->d()\n        ->e();\n"

	for _, src := range []string{"<?php\n$a |> b() |> c()\n->d()\n->e();\n", want} {
		if got := string(mustFormat(t, []byte(src), config.Default())); got != want {
			t.Fatalf("got:\n%s\nwant:\n%s", got, want)
		}
	}
}

func TestNestedArrowBodyMovesAnArrowThatFitsAloneToItsOwnLine(t *testing.T) {
	src := "outer(fn () => $x->each(fn ($y) => $y->%s($y, $z)));"
	want := "outer(fn () => $x->each(\n    fn ($y) => $y->%s($y, $z),\n));"

	tests := []struct {
		name   string
		length int
	}{
		{"once the outer arrow no longer fits a line of its own", 74},
		{"several columns past that", 83},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertLayoutAtLength(t, src, want, tt.length)
		})
	}
}

func TestArrowWithBrokenParametersMeasuresItsBodyFromTheClosingLine(t *testing.T) {
	brokenParams := func(name string) string {
		return "fn (\n    string $" + name + "1,\n    string $" + name + "2,\n    string $" + name + "3,\n) => "
	}

	short, long := strings.Repeat("p", 14), strings.Repeat("p", 30)
	shortParams := brokenParams(short)
	wideParams := brokenParams(long)
	flatParams := "fn (string $" + long + "1, string $" + long + "2, string $" + long + "3) => "
	compactParams := "fn (string $" + short + "1, string $" + short + "2, string $" + short + "3) => "

	tests := []struct {
		name   string
		src    string
		want   string
		length int
	}{
		{
			name:   "hug fits on the closing line",
			src:    "$f = " + shortParams + "$this->engine->%s($this->target, [\n    'a' => $alpha,\n]);",
			want:   "$f = " + shortParams + "$this->engine->%s($this->target, [\n    'a' => $alpha,\n]);",
			length: 83,
		},
		{
			name:   "hug one column past the closing line un-hugs",
			src:    "$f = " + shortParams + "$this->engine->%s($this->target, [\n    'a' => $alpha,\n]);",
			want:   "$f = " + shortParams + "$this->engine->%s(\n    $this->target,\n    [\n        'a' => $alpha,\n    ],\n);",
			length: 84,
		},
		{
			name:   "signature too wide to stay flat",
			src:    "$f = " + flatParams + "$this->engine->%s($this->target, [\n    'a' => $alpha,\n]);",
			want:   "$f = " + wideParams + "$this->engine->%s($this->target, [\n    'a' => $alpha,\n]);",
			length: 10,
		},
		{
			name:   "compact signature the printer breaks for its body",
			src:    "$f = " + compactParams + "$this->engine->%s($this->target, [\n    'a' => $alpha,\n]);",
			want:   "$f = " + shortParams + "$this->engine->%s($this->target, [\n    'a' => $alpha,\n]);",
			length: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertLayoutAtLength(t, tt.src, tt.want, tt.length)
		})
	}
}

func TestOperandWhoseHugFitsNowhereIsGluedWithItsListBroken(t *testing.T) {
	class := "class A\n{\n    public function a()\n    {\n        $r = %s;\n    }\n}"
	tests := []struct {
		name   string
		value  string
		want   string
		length int
	}{
		{
			name:   "ternary",
			value:  "$c ? %s($a, [$b, $c]) : null",
			want:   "$c ? %s(\n            $a,\n            [$b, $c],\n        ) : null",
			length: 101,
		},
		{
			name:   "coalesce",
			value:  "$a ?? %s($a, [$b, $c])",
			want:   "$a ?? %s(\n            $a,\n            [$b, $c],\n        )",
			length: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := strings.Replace(class, "%s", tt.value, 1)
			want := strings.Replace(class, "%s", tt.want, 1)

			assertLayoutAtLength(t, src, want, tt.length)
		})
	}
}

func TestArrowBodyCallUnhugsItsClosureBeforeBreakingItsHead(t *testing.T) {
	src := "$f = fn () => foo($a)->%s(function ($x) { return $x; });"
	want := "$f = fn () => foo($a)->%s(\n    function ($x) {\n        return $x;\n    },\n);"

	assertLayoutAtLength(t, src, want, 82)
}

func TestNewArgumentUnhugsLikeACall(t *testing.T) {
	src := "$result = outer(new %s($a, [$b]));"
	tests := []struct {
		name   string
		want   string
		length int
	}{
		{
			name:   "un-hugs flat",
			want:   "$result = outer(\n    new %s($a, [$b]),\n);",
			length: 94,
		},
		{
			name:   "un-hugs with its array broken",
			want:   "$result = outer(\n    new %s($a, [\n        $b,\n    ]),\n);",
			length: 102,
		},
		{
			name:   "un-hugs with its own list broken",
			want:   "$result = outer(\n    new %s(\n        $a,\n        [$b],\n    ),\n);",
			length: 110,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertLayoutAtLength(t, src, tt.want, tt.length)
		})
	}
}

func TestOperandAfterACallBreaksBeforeTheOperator(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		want   string
		length int
	}{
		{
			name:   "logical operand",
			src:    "return (bool) config()->%s('alpha.beta.gamma.delta.enabled') && $this->collection->count() > 1;",
			want:   "return (bool) config()->%s('alpha.beta.gamma.delta.enabled')\n    && $this->collection->count() > 1;",
			length: 40,
		},
		{
			name:   "literal comparison",
			src:    "return config()->%s('section.key') == 'with_the_rate';",
			want:   "return config()->%s('section.key')\n    == 'with_the_rate';",
			length: 86,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertLayoutAtLength(t, tt.src, tt.want, tt.length)
		})
	}
}
