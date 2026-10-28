package detector

import (
	"path/filepath"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/tailwind"
	"github.com/JacobJoergensen/laravel-fmt/internal/walk"
)

func monorepo(t *testing.T) (workspace, file string) {
	t.Helper()

	workspace = t.TempDir()
	app := filepath.Join(workspace, "packages", "app")

	seedFileAt(t, workspace, ".prettierrc", `{"printWidth":120,"plugins":["@prettier/plugin-php"]}`)
	seedFileAt(t, app, "composer.json", "{}")
	seedFileAt(t, app, "artisan", "")
	seedFileAt(t, app, ".prettierrc", `{"printWidth":40,"plugins":["@prettier/plugin-php"]}`)
	seedFileAt(t, app, "src/x.php", "<?php")

	return workspace, filepath.Join(app, "src", "x.php")
}

func widthFor(t *testing.T, resolver *Resolver, file string) int {
	t.Helper()

	project, err := resolver.For(file)
	if err != nil {
		t.Fatalf("resolve %s: %v", file, err)
	}

	resolved, err := ResolveFileConfig(&project.Cfg, file, project.Ctx, project.UserKeys)
	if err != nil {
		t.Fatalf("ResolveFileConfig: %v", err)
	}

	return resolved.PrintWidth
}

func TestResolverGivesEverySurfaceTheFilesOwnProject(t *testing.T) {
	workspace, file := monorepo(t)

	asked := map[string]string{
		"a directory run from the workspace": workspace,
		"a directory run from the package":   filepath.Join(workspace, "packages"),
		"the file's own directory":           filepath.Dir(file),
	}

	const want = 40

	for name, dir := range asked {
		resolver := NewResolver(LoadConfig)

		if _, err := resolver.ForDir(dir); err != nil {
			t.Fatalf("%s: ForDir(%s): %v", name, dir, err)
		}

		if got := widthFor(t, resolver, file); got != want {
			t.Fatalf("%s resolved printWidth %d, want the application's %d", name, got, want)
		}
	}
}

func TestResolverOutsideAnyProjectUsesTheFilesOwnDirectory(t *testing.T) {
	workspace := t.TempDir()
	seedFileAt(t, workspace, ".prettierrc", `{"printWidth":120,"plugins":["@prettier/plugin-php"]}`)
	seedFileAt(t, workspace, "loose/.prettierrc", `{"printWidth":40,"plugins":["@prettier/plugin-php"]}`)
	seedFileAt(t, workspace, "loose/x.php", "<?php")

	file := filepath.Join(workspace, "loose", "x.php")

	walked := NewResolver(LoadConfig)

	if _, err := walked.ForDir(workspace); err != nil {
		t.Fatalf("ForDir(workspace): %v", err)
	}

	if got := widthFor(t, walked, file); got != 40 {
		t.Fatalf("a walk from the workspace resolved printWidth %d, want the nested config's 40", got)
	}

	if got := widthFor(t, NewResolver(LoadConfig), file); got != 40 {
		t.Fatalf("a run that names the file resolved printWidth %d, want 40", got)
	}
}

func TestResolverOutsideAnyProjectChainsUpToAnAncestorConfig(t *testing.T) {
	workspace := t.TempDir()
	seedFileAt(t, workspace, ".prettierrc", `{"printWidth":120,"plugins":["@prettier/plugin-php"]}`)
	seedFileAt(t, workspace, "loose/x.php", "<?php")

	file := filepath.Join(workspace, "loose", "x.php")

	if got := widthFor(t, NewResolver(LoadConfig), file); got != 120 {
		t.Fatalf("printWidth = %d, want the ancestor's 120", got)
	}
}

func TestResolverCachesAProjectAcrossDirectories(t *testing.T) {
	workspace, file := monorepo(t)
	app := filepath.Join(workspace, "packages", "app")

	resolver := NewResolver(LoadConfig)

	first, err := resolver.For(file)
	if err != nil {
		t.Fatalf("For: %v", err)
	}

	second, err := resolver.ForDir(app)
	if err != nil {
		t.Fatalf("ForDir: %v", err)
	}

	if first != second {
		t.Fatal("the same project was detected twice; the cache is keyed wrong")
	}
}

func TestResolverGivesEachProjectItsOwnTheme(t *testing.T) {
	root := t.TempDir()

	projects := map[string]string{
		"app":              "--color-brand: #f00;",
		"app/packages/kit": "--text-brand: 2rem;",
		"sibling":          "--text-brand: 2rem;",
	}

	for dir, token := range projects {
		seedFileAt(t, root, dir+"/composer.json", "{}")
		seedFileAt(t, root, dir+"/artisan", "")
		seedFileAt(t, root, dir+"/resources/css/app.css", `@import "tailwindcss";`+"\n@theme { "+token+" }")
	}

	want := map[string]string{
		"app":              "font-bold text-brand",
		"app/packages/kit": "text-brand font-bold",
		"sibling":          "text-brand font-bold",
	}

	resolver := NewResolver(LoadConfig)

	for dir, sorted := range want {
		view := filepath.Join(root, filepath.FromSlash(dir), "resources", "views", "x.blade.php")

		project, err := resolver.For(view)
		if err != nil {
			t.Fatalf("For(%s): %v", dir, err)
		}

		if got := tailwind.Sort(project.Cfg.TailwindTheme, "text-brand font-bold"); got != sorted {
			t.Errorf("%s sorted %q, want its own theme's %q", dir, got, sorted)
		}
	}
}

func TestNestedPackageUsesTheApplicationsIgnores(t *testing.T) {
	app := t.TempDir()
	seedFileAt(t, app, "composer.json", "{}")
	seedFileAt(t, app, "artisan", "")
	seedFileAt(t, app, ".prettierignore", "packages/foo/src/Ignored.php\n")
	seedFileAt(t, app, "pint.json", `{"preset":"laravel","exclude":["packages/foo/src/excluded"]}`)
	seedFileAt(t, app, "packages/foo/composer.json", `{"name":"acme/foo"}`)

	loaders := map[string]func(string, Context) (config.Loaded, error){
		"default load": LoadConfig,
		"--no-config":  func(_ string, ctx Context) (config.Loaded, error) { return LoadDefaults(ctx) },
	}

	files := []string{"packages/foo/src/Ignored.php", "packages/foo/src/excluded/E.php"}

	for name, load := range loaders {
		for _, rel := range files {
			t.Run(name+" "+rel, func(t *testing.T) {
				path := filepath.Join(app, filepath.FromSlash(rel))

				project, err := NewResolver(load).For(path)
				if err != nil {
					t.Fatalf("For: %v", err)
				}

				ignored, err := walk.IgnoresFile(project.IgnoreAnchor(), &project.Cfg, path)
				if err != nil {
					t.Fatalf("IgnoresFile: %v", err)
				}

				if !ignored {
					t.Fatalf("%s is not ignored; the application's ignores were read from %s", rel, project.IgnoreAnchor())
				}
			})
		}
	}
}
