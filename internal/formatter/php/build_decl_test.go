package php

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

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
