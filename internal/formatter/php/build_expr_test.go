package php

import (
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

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
		{
			"meaningful then before a cast block",
			"return $item === null ? $fallback : (object) [\n    'kind' => 'item',\n    'model' => $item,\n];",
			"return $item === null\n    ? $fallback\n    : (object) [\n        'kind' => 'item',\n        'model' => $item,\n    ];",
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
