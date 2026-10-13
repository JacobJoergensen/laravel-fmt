package formatter

import "testing"

func TestIsIgnore(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{"php line comment", "// laravel-fmt-ignore", true},
		{"php hash comment", "# laravel-fmt-ignore", true},
		{"php block comment", "/* laravel-fmt-ignore */", true},
		{"php doc comment", "/** laravel-fmt-ignore */", true},
		{"html comment", "<!-- laravel-fmt-ignore -->", true},
		{"blade comment", "{{-- laravel-fmt-ignore --}}", true},

		{"trailing whitespace tolerated", "// laravel-fmt-ignore   ", true},
		{"trailing colon self-doc", "// laravel-fmt-ignore: hand-aligned", true},
		{"trailing tab self-doc", "// laravel-fmt-ignore\tnote", true},
		{"surrounding whitespace tolerated", "/*   laravel-fmt-ignore   */", true},

		{"block closer in a hash comment is text", "# laravel-fmt-ignore*/", false},
		{"block closer in a hash comment, space after", "# laravel-fmt-ignore*/ ", false},
		{"block closer in a line comment is text", "// laravel-fmt-ignore*/", false},
		{"block closer after whitespace still closes a block", "/* laravel-fmt-ignore */ ", true},
		{"html closer in a line comment is text", "// laravel-fmt-ignore-->", false},
		{"blade closer in a line comment is text", "// laravel-fmt-ignore--}}", false},
		{"html comment keeps its own closer", "<!-- laravel-fmt-ignore -->  ", true},
		{"blade comment keeps its own closer", "{{-- laravel-fmt-ignore --}}  ", true},

		{"word boundary required", "// laravel-fmt-ignore-not-really", false},
		{"case sensitive", "// Laravel-Fmt-Ignore", false},
		{"unrelated comment", "// just a comment", false},
		{"empty after marker", "// ", false},
		{"empty string", "", false},
		{"whitespace only", "   ", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsIgnore([]byte(tc.text))

			if got != tc.want {
				t.Fatalf("IsIgnore(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}
