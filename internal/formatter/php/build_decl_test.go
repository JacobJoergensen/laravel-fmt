package php

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestAnonymousClassArgumentsStayOnOneLineWhenPintJoinsThem(t *testing.T) {
	const flat = "<?php\n\n$handler = new class($this->originalExceptionHandler, $exceptionsToReport, $more) " +
		"implements ExceptionHandler, WithoutExceptionHandlingHandler\n{\n};\n"
	const broken = "<?php\n\n$handler = new class(\n    $this->originalExceptionHandler,\n    $exceptionsToReport,\n    $more,\n) " +
		"implements ExceptionHandler, WithoutExceptionHandlingHandler\n{\n};\n"

	tests := []struct {
		name   string
		inline bool
		src    string
		want   string
	}{
		{"an over-wide list stays on one line", true, flat, flat},
		{"an author-broken list joins", true, broken, flat},
		{"without the rule a wide list breaks", false, flat, broken},
		{
			"a closure inside a chain argument keeps its indent",
			true,
			"<?php\n\nfunction g()\n{\n    return new class($q->where(1)->each(function ($i) { return $i; })) {};\n}\n",
			"<?php\n\nfunction g()\n{\n    return new class($q->where(1)->each(function ($i) {\n            return $i;\n        }))\n    {\n    };\n}\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.PHP.InlineAnonClassArgs = tc.inline
			cfg.PHP.InlineEmptyAnonClassBodies = false

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

// Laravel's preset keeps `use A, B;` grouped, and Pint flags a split list under
// a docblock, so only a project whose rules split trait uses gets one per line.
func TestTraitUsesSplitOnlyWhenTheProjectSplitsThem(t *testing.T) {
	const head = "<?php\n\nclass Account\n{\n"
	const tail = "}\n"

	grouped := "    /** @use HasRecords<RecordFactory> */\n    use HasRecords, Notifies, Archives;\n"
	split := "    /** @use HasRecords<RecordFactory> */\n    use HasRecords;\n    use Notifies;\n    use Archives;\n"
	adapted := "    use Notifies, Archives {\n        Notifies::send insteadof Archives;\n    }\n"

	tests := []struct {
		name  string
		split bool
		body  string
		want  string
	}{
		{"a grouped list stays grouped", false, grouped, grouped},
		{"a split list stays split", false, split, split},
		{"the project's rules split a grouped list", true, grouped, split},
		{"a list with adaptations stays grouped", true, adapted, adapted},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.PHP.SplitTraitUses = tc.split

			got := mustFormat(t, []byte(head+tc.body+tail), cfg)
			if want := head + tc.want + tail; string(got) != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}

// PHP-CS-Fixer's braces_position without single_line_empty_body puts an empty
// body's braces on lines of their own; laravel-fmt follows when told to.
func TestEmptyBodiesFollowTheProjectsLayout(t *testing.T) {
	tests := []struct {
		name string
		set  func(*config.PHP)
		src  string
		want string
	}{
		{
			"an empty function",
			func(p *config.PHP) { p.InlineEmptyFunctionBodies = false },
			"<?php\n\nfunction run() {}\n",
			"<?php\n\nfunction run()\n{\n}\n",
		},
		{
			"an empty method beside a wrapped signature",
			func(p *config.PHP) { p.InlineEmptyMethodBodies = false },
			"<?php\n\nclass Job\n{\n    public function run(string $aVeryLongFirstArgumentName, string $aVeryLongSecondArgumentName, string $aVeryLongThirdArgumentName) {}\n}\n",
			"<?php\n\nclass Job\n{\n    public function run(\n        string $aVeryLongFirstArgumentName,\n        string $aVeryLongSecondArgumentName,\n        string $aVeryLongThirdArgumentName,\n    ) {\n    }\n}\n",
		},
		{
			"an empty class",
			func(p *config.PHP) { p.InlineEmptyClassBodies = false },
			"<?php\n\nclass Job {}\n",
			"<?php\n\nclass Job\n{\n}\n",
		},
		{
			"an empty anonymous class",
			func(p *config.PHP) { p.InlineEmptyAnonClassBodies = false },
			"<?php\n\n$job = new class {};\n",
			"<?php\n\n$job = new class\n{\n};\n",
		},
		{
			"a space before an anonymous class's arguments",
			func(p *config.PHP) { p.AnonClassParenSpace = true },
			"<?php\n\n$job = new class(1) {};\n",
			"<?php\n\n$job = new class (1) {};\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			tc.set(&cfg.PHP)

			got := mustFormat(t, []byte(tc.src), cfg)
			if string(got) != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}

			if again := mustFormat(t, got, cfg); !bytes.Equal(again, got) {
				t.Fatalf("not idempotent:\n%s\nthen:\n%s", got, again)
			}
		})
	}
}

// PHP 8.6 gives a parameter the last doc comment read before the token after
// it, so a comment never crosses a comma, and a trailing comma is never added
// or dropped between a comment and the `)`.
func TestParameterCommentsKeepTheirSideOfEachComma(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{
			"a doc comment above a parameter stays above it",
			"<?php\nfunction f(\n    /** @param Book[] */\n    array $books,\n) {}\n",
			"<?php\n\nfunction f(\n    /** @param Book[] */\n    array $books,\n) {}\n",
		},
		{
			"a doc comment before a comma stays before it",
			"<?php\nfunction f(  array   $a   /** x */  ,  array $b ) {}\n",
			"<?php\n\nfunction f(array $a /** x */, array $b) {}\n",
		},
		{
			"an added trailing comma goes after the comment",
			"<?php\nfunction f(\n    array $a /** x */\n) {}\n",
			"<?php\n\nfunction f(\n    array $a /** x */,\n) {}\n",
		},
		{
			"a comment after a trailing comma keeps the list broken",
			"<?php\nfunction f(array $a, /** x */) {}\n",
			"<?php\n\nfunction f(\n    array $a, /** x */\n) {}\n",
		},
		{
			"a comment after a moved line comment goes on its own line",
			"<?php\nfunction f(\n    $a // y\n    , /** x */\n    $b\n) {}\n",
			"<?php\n\nfunction f(\n    $a, // y\n    /** x */\n    $b,\n) {}\n",
		},
		{
			"a line comment before a trailing comma moves after it",
			"<?php\nfunction f(\n    $a // y\n    // z\n    ,\n) {}\n",
			"<?php\n\nfunction f(\n    $a, // y\n    // z\n) {}\n",
		},
		{
			"two moved line comments stay on two lines",
			"<?php\nfunction f(\n    $a // y\n    // z\n    , $b\n) {}\n",
			"<?php\n\nfunction f(\n    $a, // y\n    // z\n    $b,\n) {}\n",
		},
		{
			"two doc comments before a parameter keep their order",
			"<?php\nfunction f($a, /** p */ /** q */\n    $b) {}\n",
			"<?php\n\nfunction f(\n    $a,\n    /** p */ /** q */\n    $b,\n) {}\n",
		},
		{
			"a line comment before a doc comment stays before it",
			"<?php\nfunction f(\n    $a // y\n    /** z */\n) {}\n",
			"<?php\n\nfunction f(\n    $a // y\n    /** z */,\n) {}\n",
		},
		{
			"a comment after a comma trails it",
			"<?php\nfunction f(\n    array $a, /** x */\n    array $b,\n) {}\n",
			"<?php\n\nfunction f(\n    array $a, /** x */\n    array $b,\n) {}\n",
		},
		{
			"inline doc comments stay inline",
			"<?php\nfunction f(/** @param int */ $a, /** @param string */ $b) {}\n",
			"<?php\n\nfunction f(/** @param int */ $a, /** @param string */ $b) {}\n",
		},
		{
			"a promoted property keeps its trailing doc comment",
			"<?php\nclass K { public function __construct(public int $a /** x */) {} }\n",
			"<?php\n\nclass K\n{\n    public function __construct(\n        public int $a /** x */,\n    ) {}\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tt.in), config.Default())
			if string(got) != tt.want {
				t.Fatalf("got\n%s\nwant\n%s", got, tt.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent:\n%s\n---\n%s", got, twice)
			}
		})
	}
}
