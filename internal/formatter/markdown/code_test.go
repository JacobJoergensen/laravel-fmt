package markdown

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestFormatCodeBlocks(t *testing.T) {
	tabs := config.Default()
	tabs.IndentStyle = config.IndentStyleTab

	crlf := config.Default()
	crlf.EndOfLine = config.EndOfLineCRLF

	cases := []struct {
		name string
		cfg  *config.File
		src  string
		want string
	}{
		{
			"lone close tag stays as written",
			config.Default(),
			"```php\n?>\n```\n",
			"```php\n?>\n```\n",
		},
		{
			"member whose string line starts like an indent stays as written",
			config.Default(),
			"```php\npublic function message(): string {return 'first\n    second';}\n```\n",
			"```php\npublic function message(): string {return 'first\n    second';}\n```\n",
		},
		{
			"members outdent by one tab",
			tabs,
			"```php\npublic function ready(): bool {return true;}\n```\n",
			"```php\npublic function ready(): bool\n{\n\treturn true;\n}\n```\n",
		},
		{
			"code takes the file's line endings",
			crlf,
			"```php\r\nif($ready){start();}\r\n```\r\n",
			"```php\r\nif ($ready) {\r\n    start();\r\n}\r\n```\r\n",
		},
		{
			"a tag spelling a file's extension formats as that file",
			config.Default(),
			"```blade.php\n<div>{{$a}}</div>\n```\n",
			"```blade.php\n<div>{{ $a }}</div>\n```\n",
		},
		{
			"a lone CR in code is laid out as the line break the fence writes",
			config.Default(),
			"```php\n0('0000000\r" + strings.Repeat("0", 100) + "')%([])",
			"```php\n0('0000000\n" + strings.Repeat("0", 100) + "')\n    % ([])\n```\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.src), tc.cfg)
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if twice := mustFormat(t, got, tc.cfg); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent: %q -> %q", got, twice)
			}
		})
	}
}
