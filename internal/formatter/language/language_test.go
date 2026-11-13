package language

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
)

// Laravel renders a mail component inside the mail's Markdown, so an indent
// added after a blank line turns the rest of the component into a code block.
func TestFormatKeepsTheLayoutOfAMailComponentTemplate(t *testing.T) {
	src := []byte("<table class=\"panel\">\n" +
		"<tr>\n" +
		"<td class=\"panel-item\">\n" +
		"{{ Illuminate\\Mail\\Markdown::parse($slot) }}\n" +
		"</td>\n" +
		"</tr>\n" +
		"</table>\n")

	tests := []struct {
		name string
		path string
		kept bool
	}{
		{"a published app template", "resources/views/vendor/mail/html/panel.blade.php", true},
		{"a Windows path", `C:\app\resources\views\vendor\mail\html\panel.blade.php`, true},
		{"the framework's own template", "src/Illuminate/Mail/resources/views/html/panel.blade.php", true},
		{"an ordinary component", "resources/views/components/panel.blade.php", false},
		{"an app folder named mail", "resources/views/mail/html/panel.blade.php", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Format(formatter.LangBlade, tc.path, src, config.Default())
			if err != nil {
				t.Fatalf("format: %v", err)
			}

			if kept := bytes.Equal(got, src); kept != tc.kept {
				t.Fatalf("kept as written = %v, want %v\n=== out ===\n%s", kept, tc.kept, got)
			}

			twice, err := Format(formatter.LangBlade, tc.path, got, config.Default())
			if err != nil {
				t.Fatalf("second format: %v", err)
			}

			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}
