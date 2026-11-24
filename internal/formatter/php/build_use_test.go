package php

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestCommentsLeftAfterAMovedRunTailKeepTheirOwnLine(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "directive at end of file",
			src:  "<?php use B;#0\nuse A;//laravel-fmt-ignore",
			want: "<?php\n\nuse A;\nuse B; #0\n//laravel-fmt-ignore\n",
		},
		{
			name: "directive and comment before a statement",
			src:  "<?php use B;#0\nuse A;/*laravel-fmt-ignore*/ /*z*/\nfoo( 1 );\n",
			want: "<?php\n\nuse A;\nuse B; #0\n\n/*laravel-fmt-ignore*/ /*z*/\nfoo(1);\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			once := mustFormat(t, []byte(tt.src), config.Default())
			if string(once) != tt.want {
				t.Errorf("got  %q\nwant %q", once, tt.want)
			}

			if twice := mustFormat(t, once, config.Default()); !bytes.Equal(twice, once) {
				t.Errorf("not idempotent:\n%q\n---\n%q", once, twice)
			}
		})
	}
}

// A kept braced group is one import: Pint's ordered_imports sorts the names in
// its braces and places it by its prefix, while a comma list still splits.
func TestAKeptUseGroupSortsInsideAndAmongImports(t *testing.T) {
	cfg := config.Default()
	cfg.PHP.KeepBracedUseGroups = true

	src := `<?php

namespace App;

use Zed\One, Alpha\Two;
use Knuckles\Attributes\{QueryParam,BodyParam};
use Illuminate\Http\Request;
`
	want := `<?php

namespace App;

use Alpha\Two;
use Illuminate\Http\Request;
use Knuckles\Attributes\{BodyParam, QueryParam};
use Zed\One;
`

	got := mustFormat(t, []byte(src), cfg)
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	if twice := mustFormat(t, got, cfg); !bytes.Equal(twice, got) {
		t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
	}
}

func TestImportCommentOnACloseTagLineStaysBeforeTheTag(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"line comment follows its import up the run", "<?php use B; use A // note ?>\n<p>x</p>\n", "<?php use A; // note\nuse B; ?>\n<p>x</p>\n"},
		{"comment on the import sorted last ends its line before the tag", "<?php use A; use B // note ?>\n<p>x</p>\n", "<?php use A; use B; // note\n?>\n<p>x</p>\n"},
		{"comment on its own line moves with its import", "<?php\nuse B; // b\nuse A; // a\n?>\n<p>x</p>\n", "<?php\nuse A; // a\nuse B; // b\n?>\n<p>x</p>\n"},
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
