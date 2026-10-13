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
