package ci

import (
	"strings"
	"testing"
)

func TestEscapeData(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"clean", "all good", "all good"},
		{"percent", "100%", "100%25"},
		{"carriage return", "a\rb", "a%0Db"},
		{"newline", "a\nb", "a%0Ab"},
		{"comma and colon kept", "a, b: c", "a, b: c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeData(tt.in); got != tt.want {
				t.Errorf("escapeData(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestEscapeProperty(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"clean", "app/User.php", "app/User.php"},
		{"percent", "100%", "100%25"},
		{"comma", "a,b", "a%2Cb"},
		{"colon", "C:/path", "C%3A/path"},
		{"newline", "a\nb", "a%0Ab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeProperty(tt.in); got != tt.want {
				t.Errorf("escapeProperty(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestGitHubError(t *testing.T) {
	tests := []struct {
		name string
		path string
		line int
		col  int
		msg  string
		want string
	}{
		{
			name: "with position",
			path: "app/User.php",
			line: 3,
			col:  5,
			msg:  "unexpected token",
			want: "::error file=app/User.php,line=3,col=5,title=laravel-fmt::unexpected token\n",
		},
		{
			name: "file level",
			path: "app/User.php",
			line: 0,
			col:  0,
			msg:  "File is not formatted; run laravel-fmt to fix",
			want: "::error file=app/User.php,title=laravel-fmt::File is not formatted; run laravel-fmt to fix\n",
		},
		{
			name: "escapes path and message",
			path: "a,b.php",
			line: 1,
			col:  1,
			msg:  "got 100%",
			want: "::error file=a%2Cb.php,line=1,col=1,title=laravel-fmt::got 100%25\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			GitHubError(&buf, tt.path, tt.line, tt.col, tt.msg)

			if got := buf.String(); got != tt.want {
				t.Errorf("GitHubError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOnGitHubActions(t *testing.T) {
	tests := []struct {
		name string
		set  bool
		val  string
		want bool
	}{
		{"true", true, "true", true},
		{"false", true, "false", false},
		{"unset", false, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("GITHUB_ACTIONS", tt.val)
			} else {
				t.Setenv("GITHUB_ACTIONS", "")
			}

			if got := OnGitHubActions(); got != tt.want {
				t.Errorf("OnGitHubActions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEscapeCell(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"clean", "app/User.php", "app/User.php"},
		{"pipe", "a|b", "a\\|b"},
		{"newline", "a\nb", "a b"},
		{"crlf", "a\r\nb", "a b"},
		{"carriage return", "a\rb", "a b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeCell(tt.in); got != tt.want {
				t.Errorf("escapeCell(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestWriteGitHubSummary(t *testing.T) {
	tests := []struct {
		name     string
		summary  Summary
		contains []string
		absent   []string
	}{
		{
			name: "check drift and failure",
			summary: Summary{
				Check:     true,
				Changed:   []Change{{Path: "app/Models/User.php", Line: 1}},
				Unchanged: 12,
				Failures:  []Failure{{Path: "app/Broken.php", Line: 3, Column: 5, Message: "unexpected token"}},
			},
			contains: []string{
				"#### ✗ Failed to format (1)",
				"| `app/Broken.php` | 3:5 | unexpected token |",
				"#### ⚠ Would be reformatted (1) · 12 already formatted",
				"| `app/Models/User.php` |",
			},
		},
		{
			name: "write mode",
			summary: Summary{
				Changed:   []Change{{Path: "app/Models/User.php", Line: 1}},
				Unchanged: 2,
			},
			contains: []string{"#### ✓ Formatted (1) · 2 already formatted"},
			absent:   []string{"Would be reformatted", "Failed to format"},
		},
		{
			name:     "all formatted",
			summary:  Summary{Unchanged: 5},
			contains: []string{"✓ All files already formatted"},
			absent:   []string{"####"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			WriteGitHubSummary(&buf, tt.summary)
			got := buf.String()

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("summary missing %q\ngot:\n%s", want, got)
				}
			}

			for _, bad := range tt.absent {
				if strings.Contains(got, bad) {
					t.Errorf("summary should not contain %q\ngot:\n%s", bad, got)
				}
			}
		})
	}
}

func TestGitHubSummaryPath(t *testing.T) {
	t.Setenv("GITHUB_STEP_SUMMARY", "/tmp/summary")

	if got := GitHubSummaryPath(); got != "/tmp/summary" {
		t.Errorf("GitHubSummaryPath() = %q, want %q", got, "/tmp/summary")
	}

	t.Setenv("GITHUB_STEP_SUMMARY", "")

	if got := GitHubSummaryPath(); got != "" {
		t.Errorf("GitHubSummaryPath() = %q, want empty", got)
	}
}
