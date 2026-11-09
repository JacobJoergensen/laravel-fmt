package detector

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/tailwind"
)

func TestDetectTailwind(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  string
	}{
		{"double-quoted", "app.css", `@import "tailwindcss";`},
		{"single-quoted", "app.css", `@import 'tailwindcss';`},
		{"subpath", "app.css", `@import "tailwindcss/preflight";`},
		{"non-default-filename", "main.css", `@import "tailwindcss";`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			cssDir := filepath.Join(root, "resources", "css")

			if err := os.MkdirAll(cssDir, 0o750); err != nil {
				t.Fatalf("mkdir %s: %v", cssDir, err)
			}

			path := filepath.Join(cssDir, tc.filename)

			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}

			tw, found, err := detectTailwind(root)
			if err != nil {
				t.Fatalf("detectTailwind: %v", err)
			}

			if !found {
				t.Fatalf("expected v4 import to be detected")
			}

			if !slices.Equal(tw.Paths, []string{path}) {
				t.Fatalf("Paths = %q, want %q", tw.Paths, []string{path})
			}
		})
	}
}

func TestDetectTailwindReturnsEveryEntryPoint(t *testing.T) {
	root := t.TempDir()
	cssDir := filepath.Join(root, "resources", "css")

	if err := os.MkdirAll(cssDir, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", cssDir, err)
	}

	files := map[string]string{
		"admin.css":  `@import "tailwindcss";`,
		"app.css":    `@import "tailwindcss";`,
		"plain.css":  `body { color: red; }`,
		"notes.md":   `not css`,
		"zzzz-a.css": `@import "tailwindcss";`,
	}

	for name, body := range files {
		path := filepath.Join(cssDir, name)

		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	tw, found, err := detectTailwind(root)
	if err != nil {
		t.Fatalf("detectTailwind: %v", err)
	}

	if !found {
		t.Fatal("expected the v4 imports to be detected")
	}

	want := []string{
		filepath.Join(cssDir, "admin.css"),
		filepath.Join(cssDir, "app.css"),
		filepath.Join(cssDir, "zzzz-a.css"),
	}

	if !slices.Equal(tw.Paths, want) {
		t.Fatalf("Paths = %q, want %q in ReadDir order", tw.Paths, want)
	}
}

func TestDetectTailwindNone(t *testing.T) {
	root := t.TempDir()

	tw, found, err := detectTailwind(root)
	if err != nil {
		t.Fatalf("detectTailwind: %v", err)
	}

	if found {
		t.Fatalf("expected no detection, got %+v", tw)
	}
}

func TestDetectTailwindCSSWithoutImport(t *testing.T) {
	root := t.TempDir()
	cssDir := filepath.Join(root, "resources", "css")

	if err := os.MkdirAll(cssDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	path := filepath.Join(cssDir, "app.css")

	if err := os.WriteFile(path, []byte("body { color: red; }\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, found, err := detectTailwind(root)
	if err != nil {
		t.Fatalf("detectTailwind: %v", err)
	}

	if found {
		t.Fatalf("expected no detection (CSS without import marker)")
	}
}

// Filament's panel theme sits below resources/css and imports its vendored
// stylesheet, not tailwindcss itself.
func TestDetectTailwindFindsGeneratedPanelTheme(t *testing.T) {
	root := t.TempDir()

	seed := map[string]string{
		"resources/css/app.css":                  `@import "tailwindcss";`,
		"resources/css/filament/admin/theme.css": `@import '../../../../vendor/pkg/theme.css';`,
		"resources/css/filament/admin/notes.txt": `not css`,
		"vendor/pkg/theme.css":                   "@import 'tailwindcss' source(none);",
		"resources/css/partials/_buttons.css":    `.btn { color: red; }`,
	}

	for rel, body := range seed {
		path := filepath.Join(root, filepath.FromSlash(rel))

		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}

		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	tw, found, err := detectTailwind(root)
	if err != nil {
		t.Fatalf("detectTailwind: %v", err)
	}

	if !found {
		t.Fatal("expected the entry points to be detected")
	}

	want := []string{
		filepath.Join(root, "resources", "css", "app.css"),
		filepath.Join(root, "resources", "css", "filament", "admin", "theme.css"),
	}

	if !slices.Equal(tw.Paths, want) {
		t.Fatalf("Paths = %q, want %q; a partial that reaches no Tailwind is not an entry point", tw.Paths, want)
	}
}

func TestResolveTailwindSilentWithoutTailwind(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, "composer.json", "{}")

	cfg := loadTailwind(t, dir)

	if cfg.Tailwind || cfg.TailwindTheme != nil {
		t.Fatalf("Tailwind = %v, TailwindTheme = %v, want off and nil", cfg.Tailwind, cfg.TailwindTheme)
	}
}

// Each stylesheet defines its own utility and an unknown class sorts first,
// so the sorted probe shows which stylesheets built the theme.
func TestResolveTailwindPrecedence(t *testing.T) {
	dir := t.TempDir()

	seedFileAt(t, dir, "composer.json", "{}")
	seedFileAt(t, dir, "artisan", "")
	seedFileAt(t, dir, "resources/css/app.css", `@import "tailwindcss";`+"\n@utility app-pad { padding: 1rem; }")
	seedFileAt(t, dir, "assets/css/theme.css", `@import "tailwindcss";`+"\n@utility theme-pad { padding: 1rem; }")

	const (
		probe      = "app-pad theme-pad flex"
		discovered = "theme-pad flex app-pad"
		configured = "app-pad flex theme-pad"
		bothStyles = "flex app-pad theme-pad"
	)

	t.Run("discovery when nothing names one", func(t *testing.T) {
		cfg := loadTailwind(t, dir)

		if got := tailwind.Sort(cfg.TailwindTheme, probe); got != discovered {
			t.Fatalf("sorted %q, want the discovered stylesheet's %q", got, discovered)
		}
	})

	t.Run("prettier names one", func(t *testing.T) {
		seedFileAt(t, dir, ".prettierrc", `{"tailwindStylesheet": "./assets/css/theme.css"}`)

		cfg := loadTailwind(t, dir)

		if got := tailwind.Sort(cfg.TailwindTheme, probe); got != configured {
			t.Fatalf("sorted %q, want prettier's stylesheet's %q", got, configured)
		}
	})

	t.Run("laravel-fmt.json outranks prettier", func(t *testing.T) {
		seedFileAt(t, dir, ".prettierrc", `{"tailwindStylesheet": "./resources/css/app.css"}`)
		seedFileAt(t, dir, "laravel-fmt.json", `{"tailwind": {"stylesheet": "assets/css/theme.css"}}`)

		cfg := loadTailwind(t, dir)

		if got := tailwind.Sort(cfg.TailwindTheme, probe); got != configured {
			t.Fatalf("sorted %q, want laravel-fmt.json's stylesheet's %q", got, configured)
		}
	})

	t.Run("laravel-fmt.json names several", func(t *testing.T) {
		seedFileAt(t, dir, "laravel-fmt.json",
			`{"tailwind": {"stylesheet": ["resources/css/app.css", "assets/css/theme.css"]}}`)

		cfg := loadTailwind(t, dir)

		if got := tailwind.Sort(cfg.TailwindTheme, probe); got != bothStyles {
			t.Fatalf("sorted %q, want both stylesheets' %q", got, bothStyles)
		}
	})

	t.Run("a configured path that is not there is an error", func(t *testing.T) {
		seedFileAt(t, dir, "laravel-fmt.json", `{"tailwind": {"stylesheet": "nope.css"}}`)

		ctx, err := Detect(dir)
		if err != nil {
			t.Fatalf("Detect: %v", err)
		}

		if _, err := LoadConfig(dir, ctx); err == nil {
			t.Fatal("LoadConfig accepted a stylesheet that does not exist")
		}
	})
}
