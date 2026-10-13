package php

import (
	"bytes"
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
