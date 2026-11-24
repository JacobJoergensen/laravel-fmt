package php

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestValueStaysOnItsArrowLineWhenPintJoinsIt(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"array value",
			"$a = ['google' => filled(config('services.google.picker.api_key')) && filled(config('services.google.client_id_value_string'))];",
			"$a = [\n    'google' => filled(config('services.google.picker.api_key'))\n" +
				"        && filled(config('services.google.client_id_value_string')),\n];",
		},
		{
			"match result",
			"$a = match ($p) {\n    self::GoogleDrive => filled(config('services.google.picker.api_key')) && filled(config('services.google.client_id')),\n};",
			"$a = match ($p) {\n    self::GoogleDrive => filled(config('services.google.picker.api_key'))\n" +
				"        && filled(config('services.google.client_id')),\n};",
		},
	}

	cfg := config.Default()
	cfg.PHP.ArrowValueOnKeyLine = true

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

func TestBrokenTrailingArgumentKeepsItsIndentAfterAHuggedArray(t *testing.T) {
	cases := []struct {
		name   string
		callee string
	}{
		{"this chain", "$this->repository->get"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "<?php\n\nfunction g()\n{\n" +
				"    f([\n        'a' => 1,\n    ], " + tc.callee + "([\n        'x',\n    ]));\n}\n"
			want := "<?php\n\nfunction g()\n{\n" +
				"    f(\n        [\n            'a' => 1,\n        ],\n" +
				"        " + tc.callee + "([\n            'x',\n        ]),\n    );\n}\n"

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

// Pint's psr12 keeps the arguments on the call's line and hugs the closure.
func TestEmptyClosureOnTwoLinesHugsAsAnArgument(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.InlineEmptyClosureBodies = false

	src := "<?php\n\n$app->bind(Foo::class, function () {});\n"
	want := "<?php\n\n$app->bind(Foo::class, function () {\n});\n"

	got := mustFormat(t, []byte(src), cfg)
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if again := mustFormat(t, got, cfg); !bytes.Equal(again, got) {
		t.Fatalf("not idempotent:\n%s\nthen:\n%s", got, again)
	}
}

func TestCommentOnlyClosureHugsAsAnArgument(t *testing.T) {
	src := "<?php\n\n$items->each(function ($value, $key) {\n    // Silence is golden!\n});\n"

	got := mustFormat(t, []byte(src), config.Default())
	if string(got) != src {
		t.Fatalf("got:\n%s\nwant:\n%s", got, src)
	}
}

func TestCommentOnlyArrayHugsAsAnArgument(t *testing.T) {
	tests := []string{
		"<?php\n\nreturn Response::json([\n    // ...\n]);\n",
		"<?php\n\n$batch = Bus::batch([\n    // ...\n])\n    ->then(function (Batch $batch) {\n        // ...\n    })\n    ->dispatch();\n",
	}

	for _, src := range tests {
		got := mustFormat(t, []byte(src), config.Default())
		if string(got) != src {
			t.Fatalf("got:\n%s\nwant:\n%s", got, src)
		}
	}
}

// The kept break would make the hug a width-forced one does; un-hugged, the
// arrow added a level without saving a line.
func TestArrowWithAuthorBrokenBodyCallHugs(t *testing.T) {
	tests := []string{
		"<?php\n\n$users->each(fn ($user) => $user->notify(\n    new InvoicePaid($invoice),\n));\n",
		"<?php\n\nHead::defaults(fn (HeadBuilder $head) => $head->twitter(\n    card: TwitterCard::SummaryWithLargeImage,\n));\n",
		"<?php\n\nAttribute::get(fn (): string => new Name(\n    $this->name,\n));\n",
	}

	for _, src := range tests {
		got := mustFormat(t, []byte(src), config.Default())
		if string(got) != src {
			t.Fatalf("got:\n%s\nwant:\n%s", got, src)
		}
	}
}

func TestCommentsBeforeAListCloserKeepTheItemsIndent(t *testing.T) {
	tests := []string{
		"<?php\n\nfoo(\n    $a, // y\n    // z\n);\n",
		"<?php\n\nfunction f(\n    $a, // y\n    /* z */\n) {}\n",
	}

	for _, want := range tests {
		got := mustFormat(t, []byte(want), config.Default())
		if string(got) != want {
			t.Fatalf("got\n%s\nwant\n%s", got, want)
		}
	}
}

func TestAlwaysBreakingChainArgumentHugsOnlyAsAFirstOrLastArgument(t *testing.T) {
	long := "$" + strings.Repeat("a", 80)

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"hugged while the line fits",
			"$owner->assign($owner->items()->create([\n    'key' => $value,\n]));\n",
			"$owner->assign($owner->items()->create([\n    'key' => $value,\n]));\n",
		},
		{
			"one per line when every argument is named",
			"$owner->assign(user: $user, team: $owner->items()->create([\n    'key' => $value,\n]));\n",
			"$owner->assign(\n    user: $user,\n    team: $owner->items()->create([\n        'key' => $value,\n    ]),\n);\n",
		},
		{
			"one per line with two such arguments",
			"pair($a->x()->y([\n    'k' => 1,\n]), $b->x()->y([\n    'k' => 2,\n]));\n",
			"pair(\n    $a->x()->y([\n        'k' => 1,\n    ]),\n    $b->x()->y([\n        'k' => 2,\n    ]),\n);\n",
		},
		{
			"one per line as a middle argument",
			"$owner->assign($first, $owner->items()->create([\n    'key' => $value,\n]), $last);\n",
			"$owner->assign(\n    $first,\n    $owner->items()->create([\n        'key' => $value,\n    ]),\n    $last,\n);\n",
		},
		{
			"hugged as a first argument",
			"$owner->assign($owner->items()->create([\n    'key' => $value,\n]), $last);\n",
			"$owner->assign($owner->items()->create([\n    'key' => $value,\n]), $last);\n",
		},
		{
			"one per line once the hugged line overflows",
			"$owner->assign(" + long + ", $owner->items()->create([\n    'key' => $value,\n]));\n",
			"$owner->assign(\n    " + long + ",\n    $owner->items()->create([\n        'key' => $value,\n    ]),\n);\n",
		},
		{
			"hugged with a block comment before the closer",
			"$owner->assign($owner->items()->create([\n    'key' => $value,\n]) /* note */);\n",
			"$owner->assign($owner->items()->create([\n    'key' => $value,\n]) /* note */);\n",
		},
		{
			"one per line under a line comment before the closer",
			"$owner->assign($owner->items()->create([\n    'key' => $value,\n]) // note\n);\n",
			"$owner->assign(\n    $owner->items()->create([\n        'key' => $value,\n    ]), // note\n);\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte("<?php\n\n"+tc.src), config.Default())
			if want := "<?php\n\n" + tc.want; string(got) != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestArrayHugOnALinkCountsItsOpener(t *testing.T) {
	ladder := "$x\n    ->one()\n    ->two()\n    ->three()\n    ->each("
	fits := "$" + strings.Repeat("c", 105)
	over := "$" + strings.Repeat("c", 106)
	namedFits := "$" + strings.Repeat("c", 98)
	namedOver := "$" + strings.Repeat("c", 99)

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"hugged at the print width",
			ladder + fits + ", ['k' => 1, 'l' => 2]);\n",
			ladder + fits + ", [\n        'k' => 1,\n        'l' => 2,\n    ]);\n",
		},
		{
			"one per line one column past it",
			ladder + over + ", ['k' => 1, 'l' => 2]);\n",
			ladder + "\n        " + over + ",\n        ['k' => 1, 'l' => 2],\n    );\n",
		},
		{
			"named array hugged at the print width",
			ladder + namedFits + ", items: ['k' => 1, 'l' => 2]);\n",
			ladder + namedFits + ", items: [\n        'k' => 1,\n        'l' => 2,\n    ]);\n",
		},
		{
			"named array one per line one column past it",
			ladder + namedOver + ", items: ['k' => 1, 'l' => 2]);\n",
			ladder + "\n        " + namedOver + ",\n        items: ['k' => 1, 'l' => 2],\n    );\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte("<?php\n\n"+tc.src), config.Default())
			if want := "<?php\n\n" + tc.want; string(got) != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestArrowValueLeavesItsKeyLineOnlyWhenItsOpenerOverflows(t *testing.T) {
	fits := strings.Repeat("c", 91)
	over := strings.Repeat("c", 92)

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"body call splits on the key line while its opener fits",
			"$a = ['key' => fn ($i) => $i->" + fits + "($i), 'b' => 2];\n",
			"$a = [\n    'key' => fn ($i) => $i->" + fits + "(\n        $i,\n    ),\n    'b' => 2,\n];\n",
		},
		{
			"arrow moves below once the opener overflows",
			"$a = ['key' => fn ($i) => $i->" + over + "($i), 'b' => 2];\n",
			"$a = [\n    'key' =>\n        fn ($i) => $i->" + over + "($i),\n    'b' => 2,\n];\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte("<?php\n\n"+tc.src), config.Default())
			if want := "<?php\n\n" + tc.want; string(got) != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestFirstArgumentHugSkipsATrailingArgumentHoldingALineComment(t *testing.T) {
	src := "<?php\n\nfoo('one\ntwo', bar((# note\n$a)));\n"
	want := "<?php\n\nfoo(\n    'one\ntwo',\n    bar(\n        (# note\n        $a),\n    ),\n);\n"

	got := mustFormat(t, []byte(src), config.Default())
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestKeyedBinaryValueWithABrokenOperandStaysBesideItsKey(t *testing.T) {
	src := "$x = ['key' => foo(\n    $a,\n    $b,\n)->%s === $a];"
	want := "$x = [\n    'key' => foo(\n        $a,\n        $b,\n    )->%s === $a,\n];"

	assertLayoutAtLength(t, src, want, 87)
}
