package php

import (
	"bytes"
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
