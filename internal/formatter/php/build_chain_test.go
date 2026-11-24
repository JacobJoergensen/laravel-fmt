package php

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

// A nested chain's head whose `.` run wraps on width reads back as author
// broken; the trailing call's placement must not flip on that next pass.
func TestTrailingCallIgnoresAConcatWrappedHead(t *testing.T) {
	src := []byte("<?php class A{function a(){$a&$b%f(g('0000000000000000000000000000'.($c0000%$d000000000%0?" +
		"'00000000000000000':'0000000000000000')))->h('x');}}\n")

	once := mustFormat(t, src, config.Default())

	if twice := mustFormat(t, once, config.Default()); !bytes.Equal(twice, once) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", once, twice)
	}
}

func TestTailRidesTheCloserOfAHuggedArrowCall(t *testing.T) {
	src := []byte("<?php\n\nfunction foo(): void {\n" +
		"    $inner->whereFoo('shareable', [File::class, Folder::class], fn (Builder $s) => $s->whereLike('display_name_of_the_shared_item', $searchTermEnteredByTheUserInTheBox))" +
		"->orWhereBar('creator');\n}\n")
	want := "<?php\n\nfunction foo(): void\n{\n" +
		"    $inner->whereFoo('shareable', [File::class, Folder::class], fn (Builder $s) => $s->whereLike(\n" +
		"        'display_name_of_the_shared_item',\n        $searchTermEnteredByTheUserInTheBox,\n    ))->orWhereBar('creator');\n}\n"

	got := mustFormat(t, src, config.Default())

	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestTailLaddersAfterAHuggedArrowReturningAChainedCall(t *testing.T) {
	src := []byte("<?php\n\nclass A\n{\n    function a()\n    {\n" +
		"        $b->transformWith(fn () => $query->make(['first-column' % one() % 'second-column-key' % CONSTANT])" +
		"->makeHandler()($x % $foo % sixsix() % 'a-string-of-twenty-five-c' % formatter('abcd') % 'abcd'))->x(0 % '');\n" +
		"    }\n}\n")
	want := "<?php\n\nclass A\n{\n    function a()\n    {\n        $b\n" +
		"            ->transformWith(fn () => $query->make(['first-column' % one() % 'second-column-key' % CONSTANT])\n" +
		"                ->makeHandler()($x % $foo % sixsix() % 'a-string-of-twenty-five-c' % formatter('abcd') % 'abcd'))\n" +
		"            ->x(0 % '');\n    }\n}\n"

	got := mustFormat(t, src, config.Default())
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestAuthorBrokenTailLaddersAfterANestedChainHuggingAClosure(t *testing.T) {
	src := []byte("<?php\n\n$x->foo($a->b()->c(function () { x(); }))\n->d();\n")
	want := "<?php\n\n$x\n    ->foo($a->b()->c(function () {\n        x();\n    }))\n    ->d();\n"

	got := mustFormat(t, src, config.Default())
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestArrayLinkMovesDownWhenItFitsFlat(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"array wrapped in a call",
			"$r = $this->foo($this->bar)->get(route(['aaaaaaaaaaaaaaa' => 1, 'eeeeeee' => $this->eeeeeee, 'ffffff' => $this->ffffff]));",
			"$r = $this->foo($this->bar)\n    ->get(route(['aaaaaaaaaaaaaaa' => 1, 'eeeeeee' => $this->eeeeeee, 'ffffff' => $this->ffffff]));",
		},
		{
			"chain in an arrow body",
			"$f = fn ($q) => $q->whereIn('bars.revision_id_long_long_long', $ids)->select(['bars.id', 'bars.scene_number', 'bars.order']);",
			"$f = fn ($q) => $q->whereIn('bars.revision_id_long_long_long', $ids)\n    ->select(['bars.id', 'bars.scene_number', 'bars.order']);",
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

func TestSingleTrailingCallMovesDownWhenTheHeadFits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"head up to the print width",
			"function f() {\n    Foo::test('pages::projects.foo-bars.index', 'project', $this->project, 'fooBarBazQuxxxxxxx', $this->fooBar)->call('send');\n}",
			"function f()\n{\n    Foo::test('pages::projects.foo-bars.index', 'project', $this->project, 'fooBarBazQuxxxxxxx', $this->fooBar)\n" +
				"        ->call('send');\n}",
		},
		{
			"call without arguments",
			"$aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa = FooBarBazQux::createFromTimestampxxxxxxxxx((int) $row->last_activity)->diffForHumans();",
			"$aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa = FooBarBazQux::createFromTimestampxxxxxxxxx((int) $row->last_activity)\n    ->diffForHumans();",
		},
		{
			"new head",
			"$elements = new Classifier($profile)->classify(TextLines::split($content), $abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqr);",
			"$elements = new Classifier($profile)\n    ->classify(TextLines::split($content), $abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqr);",
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

func TestClosureWithALongUseListStaysHugged(t *testing.T) {
	use := "use ($createNextOccurrence, &$totalCountXyz, $someMoreVariablesXyzAbcdefg): void {\n    $a = 1;\n});"
	broken := "use (\n    $createNextOccurrence,\n    &$totalCountXyz,\n    $someMoreVariablesXyzAbcdefg,\n): void {\n    $a = 1;\n});"

	cases := []struct {
		name string
		src  string
		want string
	}{
		{"helper head", "app(Foo::class)->forEachThing(function () " + use, "app(Foo::class)->forEachThing(function () " + broken},
		{
			"breakable leading argument",
			"$q->chunkById((int) $this->option('chunk'), function (Bar $rows) " + use,
			"$q->chunkById((int) $this->option('chunk'), function (Bar $rows) " + broken,
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

func TestPestAndArgumentKeepsItsAuthorLadder(t *testing.T) {
	src := "<?php\n\nexpect($a->b)->toBe($c->d)\n    ->and(Foo::query()\n        ->where('action', Bar::Baaaaaaaaaaaaaaaaaz)\n" +
		"        ->where('auditable_id', $file->id)\n        ->count(),\n    )->toBe(1);\n"
	want := "<?php\n\nexpect($a->b)->toBe($c->d)\n    ->and(\n        Foo::query()\n            ->where('action', Bar::Baaaaaaaaaaaaaaaaaz)\n" +
		"            ->where('auditable_id', $file->id)\n            ->count(),\n    )->toBe(1);\n"

	got := string(mustFormat(t, []byte(src), config.Default()))
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestPestTailRidesTheCloserOfABrokenMatcher(t *testing.T) {
	matcher := "expect($data)->toBe([\n    'day' => '2024-01-15',\n    'moment' => '2024-01-15 10:30:00',\n])"
	pair := "->and($driver->parse($driver->serialize($data)))->toBe($data)"

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"pair after the closer",
			matcher + pair + ";",
			matcher + pair + ";",
		},
		{"lone and", matcher + "->and($x);", matcher + "->and($x);"},
		{
			"tail too wide for the closer line",
			matcher + pair + pair + ";",
			matcher + "\n    " + pair + "\n    " + pair + ";",
		},
		{
			"author ladder",
			matcher + "\n    ->and($x)->toBe($data);",
			matcher + "\n    ->and($x)->toBe($data);",
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

func TestLastArrowEndingInAnArrayUnhugsWhenItFitsAlone(t *testing.T) {
	src := "<?php\n\nwithGuard(fn () => DB::table('contract_events')->where('id', $event->id)->update(['event' => 'envelope_completed_xxxx']));\n"
	want := "<?php\n\nwithGuard(\n    fn () => DB::table('contract_events')->where('id', $event->id)->update(['event' => 'envelope_completed_xxxx']),\n);\n"

	got := string(mustFormat(t, []byte(src), config.Default()))
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestHugMeasuresTheLineItOpensOn(t *testing.T) {
	head := "Table::group(['collection' => [$" + strings.Repeat("a", 71) + "]], function () {\n    foo();\n});\n"
	over := "Table::group(['collection' => [$" + strings.Repeat("a", 72) + "]], function () {\n    foo();\n});\n"
	broken := "return wrap(\n    $alpha,\n    $" + strings.Repeat("b", 110) + ",\n)"
	wide := "return wrap($alpha, $" + strings.Repeat("b", 110) + ")"
	pipeline := "$r = (new Pipeline([\n    'first' => $" + strings.Repeat("a", 40) + ",\n    'second' => $" + strings.Repeat("b", 40) + ",\n    'third' => $" + strings.Repeat("c", 40) + ",\n]))"
	kept := "wrap(\n    $alpha,\n    $" + strings.Repeat("b", 110) + ",\n)->each(function ($item) {\n    return $item->key;\n});\n"

	cases := []struct {
		name string
		src  string
		want string
	}{
		{"closure hugs a link after a multi-line receiver", "return " + kept, "return " + kept},
		{"closure in an arrow body hugs a link after a multi-line receiver", "$f = fn () => " + kept, "$f = fn () => " + kept},
		{"static call hug fits at the print width", head, head},
		{
			"static call hug one column past the print width",
			over,
			"Table::group(\n    ['collection' => [$" + strings.Repeat("a", 72) + "]],\n    function () {\n        foo();\n    },\n);\n",
		},
		{
			"arrow hug on a variable's call fits at the print width",
			"$x->each(fn ($i) => match ($i->" + strings.Repeat("c", 86) + ") {\n    1 => 'a',\n    default => 'b',\n});\n",
			"$x->each(fn ($i) => match ($i->" + strings.Repeat("c", 86) + ") {\n    1 => 'a',\n    default => 'b',\n});\n",
		},
		{
			"arrow hug on a variable's call one column past the print width",
			"$x->each(fn ($i) => match ($i->" + strings.Repeat("c", 87) + ") {\n    1 => 'a',\n    default => 'b',\n});\n",
			"$x->each(\n    fn ($i) => match ($i->" + strings.Repeat("c", 87) + ") {\n        1 => 'a',\n        default => 'b',\n    },\n);\n",
		},
		{
			"arrow hug counts a long receiver",
			"$this->someLongerPropertyName->anotherProperty->each(fn ($i) => match ($i->" + strings.Repeat("c", 43) + ") {\n    1 => 'a',\n    default => 'b',\n});\n",
			"$this->someLongerPropertyName->anotherProperty->each(\n    fn ($i) => match ($i->" + strings.Repeat("c", 43) + ") {\n        1 => 'a',\n        default => 'b',\n    },\n);\n",
		},
		{
			"arrow hug on a head's closer fits at the print width",
			broken + "->each(fn ($item) => inner($" + strings.Repeat("c", 88) + ", [\n    'k' => 1,\n]));\n",
			broken + "->each(fn ($item) => inner($" + strings.Repeat("c", 88) + ", [\n    'k' => 1,\n]));\n",
		},
		{
			"arrow hug on a head's closer one column past the print width",
			broken + "->each(fn ($item) => inner($" + strings.Repeat("c", 89) + ", [\n    'k' => 1,\n]));\n",
			broken + "->each(\n    fn ($item) => inner($" + strings.Repeat("c", 89) + ", [\n        'k' => 1,\n    ]),\n);\n",
		},
		{
			"arrow on a head's closer un-hugs whole while it fits a line of its own",
			broken + "->each(fn ($item) => $item->" + strings.Repeat("c", 84) + "($item));\n",
			broken + "->each(\n    fn ($item) => $item->" + strings.Repeat("c", 84) + "($item),\n);\n",
		},
		{
			"sole link the author put on the next line measures from the closer too",
			broken + "\n    ->each(fn ($item) => $item->" + strings.Repeat("c", 84) + "($item));\n",
			broken + "->each(\n    fn ($item) => $item->" + strings.Repeat("c", 84) + "($item),\n);\n",
		},
		{
			"head too wide for one line measures from the closer it will print",
			wide + "->each(fn ($item) => inner($" + strings.Repeat("c", 88) + ", ['k' => 1]));\n",
			broken + "->each(fn ($item) => inner($" + strings.Repeat("c", 88) + ", [\n    'k' => 1,\n]));\n",
		},
		{
			"head too wide for one line un-hugs an arrow that fits a line of its own",
			wide + "->each(fn ($item) => $item->" + strings.Repeat("c", 84) + "($item));\n",
			broken + "->each(\n    fn ($item) => $item->" + strings.Repeat("c", 84) + "($item),\n);\n",
		},
		{
			"head too wide for one line inside another call measures from its closer",
			"outer(wrap($alpha, $" + strings.Repeat("b", 110) + ")->each(fn ($i) => $i->" + strings.Repeat("c", 93) + "($i)));\n",
			"outer(\n    wrap(\n        $alpha,\n        $" + strings.Repeat(
				"b",
				110,
			) + ",\n    )->each(\n        fn ($i) => $i->" + strings.Repeat(
				"c",
				93,
			) + "(\n            $i,\n        ),\n    ),\n);\n",
		},
		{
			"arrow with a binary body hugs on a variable's call while its first operand fits",
			"$x->each(fn ($i) => $i->" + strings.Repeat("c", 85) + " === $other && $i->ok);\n",
			"$x->each(fn ($i) => $i->" + strings.Repeat("c", 85) + " === $other\n    && $i->ok);\n",
		},
		{
			"arrow with a binary body un-hugs once its first operand overflows",
			"$x->each(fn ($i) => $i->" + strings.Repeat("c", 86) + " === $other && $i->ok);\n",
			"$x->each(\n    fn ($i) => $i->" + strings.Repeat("c", 86) + " === $other\n        && $i->ok,\n);\n",
		},
		{
			"arrow after a head closing on its hugged array un-hugs whole",
			"return wrap($alpha, [\n    'k' => $beta,\n])->each(fn ($i) => $i->" + strings.Repeat("c", 93) + "($i));\n",
			"return wrap($alpha, [\n    'k' => $beta,\n])->each(\n    fn ($i) => $i->" + strings.Repeat("c", 93) + "($i),\n);\n",
		},
		{
			"arrow that overflows only with the terminator un-hugs whole",
			"return wrap($alpha, [\n    'k' => $beta,\n])->each(fn ($i) => inner($" + strings.Repeat("c", 69) + ", ['k' => 1, 'l' => 2]));\n",
			"return wrap($alpha, [\n    'k' => $beta,\n])->each(\n    fn ($i) => inner($" + strings.Repeat("c", 69) + ", ['k' => 1, 'l' => 2]),\n);\n",
		},
		{
			"arrow after a head closing on a wide line un-hugs whole",
			"return wrap([\n    'k' => $" + strings.Repeat("b", 100) + ",\n], $more, $args)->each(fn ($i) => $i->" + strings.Repeat("c", 93) + "($i));\n",
			"return wrap([\n    'k' => $" + strings.Repeat("b", 100) + ",\n], $more, $args)->each(\n    fn ($i) => $i->" + strings.Repeat("c", 93) + "($i),\n);\n",
		},
		{
			"head the printer breaks at its array measures from the closer it will print",
			"return wrap($alpha, ['k' => $" + strings.Repeat("b", 105) + "])->each(fn ($i) => $i->" + strings.Repeat("c", 93) + "($i));\n",
			"return wrap($alpha, [\n    'k' => $" + strings.Repeat("b", 105) + ",\n])->each(\n    fn ($i) => $i->" + strings.Repeat("c", 93) + "($i),\n);\n",
		},
		{
			"head whose break is the printer's choice un-hugs the link",
			"return wrap($alpha->one()->two($" + strings.Repeat("b", 100) + ")->three())->each(fn ($i) => $i->" + strings.Repeat("c", 93) + "($i));\n",
			"return wrap(\n    $alpha->one()\n        ->two($" + strings.Repeat("b", 100) + ")\n        ->three(),\n)->each(\n    fn ($i) => $i->" + strings.Repeat("c", 93) + "($i),\n);\n",
		},
		{
			"arrow after a short head closing on a wide line un-hugs where its line overflows",
			"return wrap([\n    'k' => $beta,\n], $more, $args)->each(fn ($i) => $i->" + strings.Repeat("c", 93) + "($i));\n",
			"return wrap([\n    'k' => $beta,\n], $more, $args)->each(\n    fn ($i) => $i->" + strings.Repeat("c", 93) + "($i),\n);\n",
		},
		{
			"arrow after a closure head un-hugs where its line overflows",
			"return wrap(function () { foo(); }, $more, $args)->each(fn ($i) => $i->" + strings.Repeat("c", 93) + "($i));\n",
			"return wrap(function () {\n    foo();\n}, $more, $args)->each(\n    fn ($i) => $i->" + strings.Repeat("c", 93) + "($i),\n);\n",
		},
		{
			"short head stays on its line when the arrow after it un-hugs",
			"return wrap($alpha, $beta)->each(fn ($i) => $i->" + strings.Repeat("c", 93) + "($i));\n",
			"return wrap($alpha, $beta)->each(\n    fn ($i) => $i->" + strings.Repeat("c", 93) + "($i),\n);\n",
		},
		{
			"array hug after a multi-line head that is no call measures from its closer",
			pipeline + "->through($stages, ['k' => 1, 'l' => 2, 'm' => $" + strings.Repeat("d", 70) + "]);\n",
			pipeline + "->through($stages, [\n    'k' => 1,\n    'l' => 2,\n    'm' => $" + strings.Repeat("d", 70) + ",\n]);\n",
		},
		{
			"arrow inside another call un-hugs where its line overflows",
			"outer(wrap($alpha, [\n    'k' => $beta,\n])->each(fn ($i) => $i->" + strings.Repeat("c", 93) + "($i)));\n",
			"outer(\n    wrap($alpha, [\n        'k' => $beta,\n    ])->each(\n        fn ($i) => $i->" + strings.Repeat("c", 93) + "(\n            $i,\n        ),\n    ),\n);\n",
		},
		{
			"call in an arrow's body keeps its hug when the arrow un-hugs",
			"$x->each(fn ($i) => $i->" + strings.Repeat("c", 91) + "($i, ['k' => 1, 'l' => 2]));\n",
			"$x->each(\n    fn ($i) => $i->" + strings.Repeat("c", 91) + "($i, [\n        'k' => 1,\n        'l' => 2,\n    ]),\n);\n",
		},
		{
			"arrow hug is measured on the ladder when more links follow",
			broken + "->each(fn ($item) => inner($" + strings.Repeat("c", 86) + ", [\n    'k' => 1,\n]))->two()->three()->four();\n",
			broken + "\n    ->each(\n        fn ($item) => inner($" + strings.Repeat("c", 86) + ", [\n            'k' => 1,\n        ]),\n    )\n    ->two()\n    ->three()\n    ->four();\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(mustFormat(t, []byte("<?php\n\n"+tc.src), config.Default()))

			if want := "<?php\n\n" + tc.want; got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := string(mustFormat(t, []byte(got), config.Default())); twice != got {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestLinkAfterAHeadClosingOnAWideLineIsStableOnTheNextPass(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"arrow after a nested call", "return wrap(inner(['k' => $beta]), $more, $args)->each(fn ($i) => $i->" + strings.Repeat("c", 93) + "($i));\n"},
		{"array after a nested call", "return wrap(inner(['k' => $beta]), $more, $args)->each($" + strings.Repeat("c", 93) + ", ['k' => 1, 'l' => 2]);\n"},
		{"arrow with a binary body inside another call", "outer(wrap(['k' => $beta], $more, $args)->each(fn ($i) => $i->" + strings.Repeat("c", 76) + " === $other && $i->ok));\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte("<?php\n\n"+tc.src), config.Default())

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestNestedLinkRidesTheCloserOfABrokenHead(t *testing.T) {
	hugged := "wrap($alpha, [\n    'k' => $beta,\n])"
	long := strings.Repeat("c", 87)
	wide := strings.Repeat("c", 100)

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"after a list's closer",
			"outer(wrap(\n    $alpha,\n    $beta,\n)->each($c));\n",
			"outer(\n    wrap(\n        $alpha,\n        $beta,\n    )->each($c),\n);\n",
		},
		{
			"after a hugged array on a static call",
			"outer(Table::" + hugged + "->each($c));\n",
			"outer(\n    Table::wrap($alpha, [\n        'k' => $beta,\n    ])->each($c),\n);\n",
		},
		{
			"after a new expression",
			"outer(new Thing($alpha, [\n    'k' => $beta,\n])->each($c));\n",
			"outer(\n    new Thing($alpha, [\n        'k' => $beta,\n    ])->each($c),\n);\n",
		},
		{
			"breaking inside its arguments once they overflow",
			"outer(" + hugged + "->each(fn ($i) => $i->" + long + "($i)));\n",
			"outer(\n    wrap($alpha, [\n        'k' => $beta,\n    ])->each(\n        fn ($i) => $i->" + long + "($i),\n    ),\n);\n",
		},
		{
			"several links after a list's closer ladder",
			"outer(wrap(\n    $alpha,\n    $beta,\n)->each($" + wide + ")->all());\n",
			"outer(\n    wrap(\n        $alpha,\n        $beta,\n    )\n        ->each($" + wide + ")\n        ->all(),\n);\n",
		},
		{
			"several links that overflow ladder",
			"outer(" + hugged + "->each($" + wide + ")->all());\n",
			"outer(\n    wrap($alpha, [\n        'k' => $beta,\n    ])\n        ->each($" + wide + ")\n        ->all(),\n);\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := "<?php\n\n" + tc.want

			for _, src := range []string{"<?php\n\n" + tc.src, want} {
				if got := string(mustFormat(t, []byte(src), config.Default())); got != want {
					t.Fatalf("got:\n%s\nwant:\n%s", got, want)
				}
			}
		})
	}
}

func TestNestedCallHugsUntilItsOpeningLineOverflows(t *testing.T) {
	match := func(n int) string { return "match ($i->" + strings.Repeat("c", n) + ")" }
	long := "$this->someLongerPropertyName->anotherProperty->each($"

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"arrow in a condition hugs at the print width",
			"if ($x->each(fn ($i) => " + match(82) + " { 1 => 'a', default => 'b' })) {\n    foo();\n}\n",
			"if ($x->each(fn ($i) => " + match(82) + " {\n    1 => 'a',\n    default => 'b',\n})) {\n    foo();\n}\n",
		},
		{
			"arrow in a condition un-hugs one column past it",
			"if ($x->each(fn ($i) => " + match(83) + " { 1 => 'a', default => 'b' })) {\n    foo();\n}\n",
			"if ($x->each(\n    fn ($i) => " + match(83) + " {\n        1 => 'a',\n        default => 'b',\n    },\n)) {\n    foo();\n}\n",
		},
		{
			"arrow in a plain call hugs at the print width",
			"if (each(fn ($i) => " + match(86) + " { 1 => 'a', default => 'b' })) {\n    foo();\n}\n",
			"if (each(fn ($i) => " + match(86) + " {\n    1 => 'a',\n    default => 'b',\n})) {\n    foo();\n}\n",
		},
		{
			"arrow in a plain call un-hugs one column past it",
			"if (each(fn ($i) => " + match(87) + " { 1 => 'a', default => 'b' })) {\n    foo();\n}\n",
			"if (each(\n    fn ($i) => " + match(87) + " {\n        1 => 'a',\n        default => 'b',\n    },\n)) {\n    foo();\n}\n",
		},
		{
			"array in a static call hugs at the print width",
			"outer(Table::each($" + strings.Repeat("c", 98) + ", ['k' => 1, 'l' => 2]));\n",
			"outer(Table::each($" + strings.Repeat("c", 98) + ", [\n    'k' => 1,\n    'l' => 2,\n]));\n",
		},
		{
			"array in a static call goes one per line one column past it",
			"outer(Table::each($" + strings.Repeat("c", 99) + ", ['k' => 1, 'l' => 2]));\n",
			"outer(Table::each(\n    $" + strings.Repeat("c", 99) + ",\n    ['k' => 1, 'l' => 2],\n));\n",
		},
		{
			"arrow in an assigned arrow's body hugs at the print width",
			"$f = fn () => $this->someLongerPropertyName->anotherProperty->each(fn ($i) => " + match(28) + " { 1 => 'a', default => 'b' });\n",
			"$f = fn () => $this->someLongerPropertyName->anotherProperty->each(fn ($i) => " + match(28) + " {\n    1 => 'a',\n    default => 'b',\n});\n",
		},
		{
			"arrow in an assigned arrow's body un-hugs one column past it",
			"$f = fn () => $this->someLongerPropertyName->anotherProperty->each(fn ($i) => " + match(29) + " { 1 => 'a', default => 'b' });\n",
			"$f = fn () => $this->someLongerPropertyName->anotherProperty->each(\n    fn ($i) => " + match(29) + " {\n        1 => 'a',\n        default => 'b',\n    },\n);\n",
		},
		{
			"arrow in a nested arrow's body hugs at the print width",
			"outer(fn () => $x->each(fn ($i) => " + match(73) + " { 1 => 'a', default => 'b' }));\n",
			"outer(\n    fn () => $x->each(fn ($i) => " + match(73) + " {\n        1 => 'a',\n        default => 'b',\n    }),\n);\n",
		},
		{
			"arrow in a nested arrow's body un-hugs one column past it",
			"outer(fn () => $x->each(fn ($i) => " + match(74) + " { 1 => 'a', default => 'b' }));\n",
			"outer(\n    fn () => $x->each(\n        fn ($i) => " + match(74) + " {\n            1 => 'a',\n            default => 'b',\n        },\n    ),\n);\n",
		},
		{
			"closure too wide for a line of its own keeps its hug",
			"outer($x->each(function (TypeName $" + strings.Repeat("d", 100) + "): bool { return true; }));\n",
			"outer($x->each(function (TypeName $" + strings.Repeat("d", 100) + "): bool {\n    return true;\n}));\n",
		},
		{
			"array after a long receiver hugs at the print width",
			"outer(" + long + strings.Repeat("c", 57) + ", ['k' => 1, 'l' => 2]));\n",
			"outer(" + long + strings.Repeat("c", 57) + ", [\n    'k' => 1,\n    'l' => 2,\n]));\n",
		},
		{
			"array after a long receiver goes one per line one column past it",
			"outer(" + long + strings.Repeat("c", 58) + ", ['k' => 1, 'l' => 2]));\n",
			"outer(" + strings.TrimSuffix(long, "$") + "\n    $" + strings.Repeat("c", 58) + ",\n    ['k' => 1, 'l' => 2],\n));\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := "<?php\n\n" + tc.want

			for _, src := range []string{"<?php\n\n" + tc.src, want} {
				if got := string(mustFormat(t, []byte(src), config.Default())); got != want {
					t.Fatalf("got:\n%s\nwant:\n%s", got, want)
				}
			}
		})
	}
}

func TestNestedTailThatCannotRideLaddersAsTheNextPassReadsIt(t *testing.T) {
	wide := strings.Repeat("x", 51)
	src := "<?php\n\n$r = $a % $b->first(\n'')->second('" + wide + "\n" + wide + "');\n"

	got := mustFormat(t, []byte(src), config.Default())

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestChainArrowMeasuresABodyThatUnhugsItsArrow(t *testing.T) {
	method := "m" + strings.Repeat("a", 64)
	src := "<?php\n\nif ($x->" + method + "(fn () => $y->each(fn ($z) => $z->go($z, $a)))) {\n}\n"
	want := "<?php\n\nif ($x->" + method + "(fn () => $y->each(\n    fn ($z) => $z->go($z, $a),\n))) {\n}\n"

	got := mustFormat(t, []byte(src), config.Default())
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestNestedLinksRideABrokenCallsCloserWhileTheLineFits(t *testing.T) {
	head := "foo(\n    $a,\n    $b,\n)"
	broken := "foo(\n        $a,\n        $b,\n    )"
	at120 := "m" + strings.Repeat("a", 90)
	over := at120 + "a"

	for _, tc := range []struct {
		name, src, want string
	}{
		{
			"argument",
			"$this->set(" + head + "->map($a)->each($x)->all());",
			"$this->set(\n    " + broken + "->map($a)->each($x)->all(),\n);",
		},
		{
			"array value",
			"$x = ['key' => " + head + "->map($a)->each($x)->all()];",
			"$x = [\n    'key' => " + broken + "->map($a)->each($x)->all(),\n];",
		},
		{
			"arrow body",
			"$f = fn () => " + head + "->map($a)->each($x)->all();",
			"$f = fn () => foo(\n    $a,\n    $b,\n)->map($a)->each($x)->all();",
		},
		{
			"line at the print width",
			"$this->set(" + head + "->" + at120 + "($a)->each($x)->all());",
			"$this->set(\n    " + broken + "->" + at120 + "($a)->each($x)->all(),\n);",
		},
		{
			"line past the print width",
			"$this->set(" + head + "->" + over + "($a)->each($x)->all());",
			"$this->set(\n    " + broken + "\n        ->" + over + "($a)\n        ->each($x)\n        ->all(),\n);",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte("<?php\n\n"+tc.src+"\n"), config.Default())
			if want := "<?php\n\n" + tc.want + "\n"; string(got) != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

func TestHeadCallBeforeATailUnhugsAnArrowThatFitsAlone(t *testing.T) {
	method := "m" + strings.Repeat("a", 92)
	src := "<?php\n\n$result = $q->map(fn ($x) => $x->" + method + "($a))->all();\n"
	want := "<?php\n\n$result = $q->map(\n    fn ($x) => $x->" + method + "($a),\n)->all();\n"

	got := mustFormat(t, []byte(src), config.Default())
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestStatementChainLaddersThenUnhugsItsOutermostArrow(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		want   string
		length int
	}{
		{
			name:   "outer arrow un-hugs before the inner one",
			src:    "$result = collect($items)->map(fn ($x) => $x->each(fn ($y) => $y->%s($y, $z)));",
			want:   "$result = collect($items)->map(\n    fn ($x) => $x->each(fn ($y) => $y->%s($y, $z)),\n);",
			length: 70,
		},
		{
			name:   "arrow whose body hugs ladders first",
			src:    "collect($items)->map(fn ($x) => $x->%s($a, $b));",
			want:   "collect($items)\n    ->map(fn ($x) => $x->%s($a, $b));",
			length: 80,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertLayoutAtLength(t, tt.src, tt.want, tt.length)
		})
	}
}

func TestLastCallOnTheHeadLineMeasuresItsArrowAtTheHeadColumn(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		want   string
		length int
	}{
		{
			name:   "arrow fits alone on the head line",
			src:    "$this->query()->map(fn ($x) => $x->%s(fn ($y) => $y));",
			want:   "$this->query()->map(\n    fn ($x) => $x->%s(fn ($y) => $y),\n);",
			length: 83,
		},
		{
			name:   "hug would overflow the head line",
			src:    "$result = $this->query()->map(fn ($x) => $x->%s(fn ($y) => $y));",
			want:   "$result = $this->query()->map(\n    fn ($x) => $x->%s(\n        fn ($y) => $y,\n    ),\n);",
			length: 90,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertLayoutAtLength(t, tt.src, tt.want, tt.length)
		})
	}
}

func TestStatementLastCallLaddersBeforeUnhuggingItsClosure(t *testing.T) {
	src := "$this->query()->%s(function ($x) { return $x; });"
	tests := []struct {
		name   string
		want   string
		length int
	}{
		{
			name:   "ladders where the hug overflows the head line",
			want:   "$this->query()\n    ->%s(function ($x) {\n        return $x;\n    });",
			length: 98,
		},
		{
			name:   "un-hugs on the head line once the ladder overflows",
			want:   "$this->query()->%s(\n    function ($x) {\n        return $x;\n    },\n);",
			length: 99,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertLayoutAtLength(t, src, tt.want, tt.length)
		})
	}
}

func TestSoleLinkAfterABrokenHeadMeasuresItsRideFromTheCloser(t *testing.T) {
	src := "$x = ['key' => foo(\n    $a,\n    $b,\n)->%s($a)];"
	want := "$x = [\n    'key' => foo(\n        $a,\n        $b,\n    )->%s($a),\n];"

	assertLayoutAtLength(t, src, want, 104)
}

func TestHugMeasuresAnAuthorBrokenCalleeToItsFirstBreak(t *testing.T) {
	src := "$this->set(Foo::make([\n    'a' => 1,\n])->%s([$a]));"

	assertLayoutAtLength(t, src, src, 85)
}

func TestSoleCallArgumentUnhugsBeforeBreakingItsHead(t *testing.T) {
	src := "$this->set((new Foo($a))->%s([$a, $b]));"
	want := "$this->set(\n    (new Foo($a))->%s([\n        $a,\n        $b,\n    ]),\n);"

	assertLayoutAtLength(t, src, want, 94)
}

func TestArrowBodyChainOfSeveralLinksUnhugsTheArrow(t *testing.T) {
	src := "return collect(config('items'))\n    ->map(fn (array $list) => collect($list)\n        ->map(fn (array $item) => [\n            'id' => $item['id'],\n            'name' => tr($item['label']),\n        ])\n        ->%s('name')\n        ->unshift(['id' => '', 'name' => '']));"
	want := "return collect(config('items'))->map(\n    fn (array $list) => collect($list)\n        ->map(fn (array $item) => [\n            'id' => $item['id'],\n            'name' => tr($item['label']),\n        ])\n        ->%s('name')\n        ->unshift(['id' => '', 'name' => '']),\n);"

	assertLayoutAtLength(t, src, want, 14)
}

func TestChainWithAMultiLineStringStaysHugged(t *testing.T) {
	src := "$x = outer($a->%s('one\ntwo')->b(''));"

	assertLayoutAtLength(t, src, src, 10)
	assertLayoutAtLength(t, "$x = outer($a\n    ->%s('one\ntwo')->b(''));", src, 10)
}
