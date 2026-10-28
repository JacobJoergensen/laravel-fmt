package detector

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestDetectProbesFromLaravelRoot(t *testing.T) {
	root := t.TempDir()
	seedFileAt(t, root, "composer.json", "{}")
	seedFileAt(t, root, "artisan", "")
	seedFileAt(t, root, "pint.json", `{"preset":"laravel"}`)

	deep := filepath.Join(root, "app", "Models")
	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ctx, err := Detect(deep)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	if ctx.Laravel == nil || ctx.Laravel.Root != root {
		t.Fatalf("Laravel = %+v, want root %q", ctx.Laravel, root)
	}

	if ctx.Pint == nil {
		t.Fatal("Pint = nil, want it detected from the Laravel root above workDir")
	}
}

func TestDetectFindsMultipleTools(t *testing.T) {
	root := t.TempDir()
	seedFileAt(t, root, "composer.json", "{}")
	seedFileAt(t, root, "artisan", "")
	seedFileAt(t, root, "pint.json", `{"preset":"laravel"}`)
	seedFileAt(t, root, "mago.toml", "[formatter]\nprint-width = 100\n")
	seedFileAt(t, root, "resources/css/app.css", `@import "tailwindcss";`)

	ctx, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	if ctx.Pint == nil || ctx.Mago == nil || ctx.Tailwind == nil {
		t.Fatalf("want Pint+Mago+Tailwind set, got Pint=%t Mago=%t Tailwind=%t",
			ctx.Pint != nil, ctx.Mago != nil, ctx.Tailwind != nil)
	}
}

func TestDetectPropagatesProbeError(t *testing.T) {
	root := t.TempDir()
	seedFileAt(t, root, "pint.json", "{not valid json")

	if _, err := Detect(root); err == nil {
		t.Fatal("Detect = nil error, want the pint.json parse error propagated")
	}
}

func laravelTree(t *testing.T) (root, nested string) {
	t.Helper()

	root = t.TempDir()
	app := filepath.Join(root, "app")

	if err := os.MkdirAll(app, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	for _, f := range []struct{ path, body string }{
		{filepath.Join(root, "composer.json"), `{"name":"acme/app"}`},
		{filepath.Join(root, "artisan"), "<?php"},
		{filepath.Join(root, config.FileName), `{"printWidth":120}`},
		{filepath.Join(app, config.FileName), `{"printWidth":40}`},
	} {
		if err := os.WriteFile(f.path, []byte(f.body), 0o600); err != nil {
			t.Fatalf("write %s: %v", f.path, err)
		}
	}

	return root, filepath.Join(app, config.FileName)
}

func TestDetectResolvesTheProjectConfigWhateverThePathsAre(t *testing.T) {
	root, nested := laravelTree(t)
	rootConfig := filepath.Join(root, config.FileName)

	for _, from := range []string{root, filepath.Join(root, "app")} {
		ctx, err := Detect(from)
		if err != nil {
			t.Fatalf("Detect(%s): %v", from, err)
		}

		if ctx.ConfigPath != rootConfig {
			t.Fatalf("Detect(%s) used %q, want the project's %q", from, ctx.ConfigPath, rootConfig)
		}
	}

	ctx, err := Detect(filepath.Join(root, "app"))
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	if ctx.NestedConfigPath != nested {
		t.Fatalf("NestedConfigPath = %q, want %q so the run can report it", ctx.NestedConfigPath, nested)
	}
}

func TestDetectReportsNoNestedConfigFromTheProjectRoot(t *testing.T) {
	root, _ := laravelTree(t)

	ctx, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	if ctx.NestedConfigPath != "" {
		t.Fatalf("NestedConfigPath = %q, want empty", ctx.NestedConfigPath)
	}
}

func TestDetectOutsideAProjectKeepsTheNearestConfig(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "app")

	if err := os.MkdirAll(app, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	appConfig := filepath.Join(app, config.FileName)

	for _, path := range []string{filepath.Join(root, config.FileName), appConfig} {
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	ctx, err := Detect(app)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	if ctx.ConfigPath != appConfig {
		t.Fatalf("ConfigPath = %q, want the nearest %q", ctx.ConfigPath, appConfig)
	}

	if ctx.NestedConfigPath != "" {
		t.Fatalf("NestedConfigPath = %q, want empty when that config is the one in use", ctx.NestedConfigPath)
	}
}

func TestDetectFindsAnAncestorConfigOutsideAnApplication(t *testing.T) {
	root := t.TempDir()
	seedFileAt(t, root, ".prettierrc", `{"printWidth":40}`)
	seedFileAt(t, root, "pint.json", `{"preset":"laravel"}`)
	seedFileAt(t, root, "src/deep/x.php", "<?php")

	for _, from := range []string{root, filepath.Join(root, "src"), filepath.Join(root, "src", "deep")} {
		ctx, err := Detect(from)
		if err != nil {
			t.Fatalf("Detect(%s): %v", from, err)
		}

		if ctx.Prettier == nil {
			t.Fatalf("Detect(%s) missed the package's .prettierrc", from)
		}

		if ctx.Pint == nil {
			t.Fatalf("Detect(%s) missed the package's pint.json", from)
		}
	}
}

func TestDetectInsideAnApplicationProbesItsRootOnly(t *testing.T) {
	root := t.TempDir()
	seedFileAt(t, root, "composer.json", "{}")
	seedFileAt(t, root, "artisan", "")
	seedFileAt(t, root, "app/.prettierrc", `{"printWidth":40}`)

	ctx, err := Detect(filepath.Join(root, "app"))
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	if ctx.Prettier != nil {
		t.Fatal("a .prettierrc below the application root was adopted; the run's result would depend on the paths it was given")
	}
}

func TestDetectDoesNotProbeAboveTheApplicationRoot(t *testing.T) {
	outer := t.TempDir()
	seedFileAt(t, outer, ".prettierrc", `{"printWidth":40}`)

	app := filepath.Join(outer, "app")
	seedFileAt(t, app, "composer.json", "{}")
	seedFileAt(t, app, "artisan", "")

	ctx, err := Detect(app)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	if ctx.Prettier != nil {
		t.Fatal("a .prettierrc above the application root was adopted")
	}
}

func TestConfigDirIsWhereTheConfigIsRead(t *testing.T) {
	app := t.TempDir()
	seedFileAt(t, app, "composer.json", "{}")
	seedFileAt(t, app, "artisan", "")

	outside := t.TempDir()

	cases := []struct {
		name    string
		workDir string
		want    string
	}{
		{"application subfolder", filepath.Join(app, "app", "Models"), app},
		{"outside an application", outside, outside},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.MkdirAll(tc.workDir, 0o750); err != nil {
				t.Fatalf("mkdir: %v", err)
			}

			seedFileAt(t, tc.want, config.FileName, "{}")

			ctx, err := Detect(tc.workDir)
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}

			if got := ctx.ConfigDir(); got != tc.want {
				t.Fatalf("ConfigDir() = %q, want %q", got, tc.want)
			}

			if want := filepath.Join(tc.want, config.FileName); ctx.ConfigPath != want {
				t.Fatalf("ConfigPath = %q, want %q", ctx.ConfigPath, want)
			}
		})
	}
}

func seedFileAt(t *testing.T, dir, rel, body string) {
	t.Helper()

	full := filepath.Join(dir, filepath.FromSlash(rel))

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", full, err)
	}

	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatalf("seed %s: %v", rel, err)
	}
}
