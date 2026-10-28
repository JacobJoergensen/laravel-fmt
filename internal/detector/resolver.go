package detector

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/upward"
)

// composer.json marks a PHP project; laravel-fmt.json marks a tree
// configured without one.
var projectMarkers = []string{"composer.json", config.FileName}

// Project is one project's detected context and loaded configuration.
type Project struct {
	Root     string
	Ctx      Context
	Cfg      config.File
	UserKeys map[string]bool

	ignoreAnchor string
}

// IgnoreAnchor returns the directory the project's ignore patterns were read from.
func (p *Project) IgnoreAnchor() string {
	return p.ignoreAnchor
}

// Resolver maps a file to its project and that project's configuration.
//
// CONTRACT: a file formats by its own project's configuration, never the
// invocation's, so a walk, a named path, stdin and an editor agree, in a
// monorepo too. Caches keep it cheap: the root walk per directory, detection
// per project, and each probe per directory.
type Resolver struct {
	load func(root string, ctx Context) (config.Loaded, error)

	roots    sync.Map
	projects sync.Map
	memo     probeMemo
}

// NewResolver returns a Resolver that loads each project's config with load,
// so --config can pin one.
func NewResolver(load func(root string, ctx Context) (config.Loaded, error)) *Resolver {
	return &Resolver{load: load}
}

// For returns the project the file at path belongs to.
func (r *Resolver) For(path string) (*Project, error) {
	return r.ForDir(filepath.Dir(path))
}

// ForDir returns the project dir belongs to.
func (r *Resolver) ForDir(dir string) (*Project, error) {
	root, err := r.rootFor(dir)
	if err != nil {
		return nil, err
	}

	if cached, ok := r.projects.Load(root); ok {
		project, _ := cached.(*Project)

		return project, nil
	}

	ctx, err := detectWith(root, &r.memo)
	if err != nil {
		return nil, err
	}

	loaded, err := r.load(root, ctx)
	if err != nil {
		return nil, err
	}

	project := &Project{Root: root, Ctx: ctx, Cfg: loaded.File, UserKeys: loaded.UserKeys, ignoreAnchor: loaded.IgnoreAnchor}
	stored, _ := r.projects.LoadOrStore(root, project)
	project, _ = stored.(*Project)

	return project, nil
}

// rootFor falls back to dir itself, not the invocation's directory, so a
// file gets the same root however it was named.
func (r *Resolver) rootFor(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir, nil //nolint:nilerr // an unresolvable path is not this layer's error to raise
	}

	if cached, ok := r.roots.Load(abs); ok {
		root, _ := cached.(string)

		return root, nil
	}

	root := abs

	err = upward.Walk(abs, func(at string) (bool, error) {
		for _, marker := range projectMarkers {
			candidate := filepath.Join(at, marker)

			exists, statErr := r.memo.statExists(candidate)
			if statErr != nil {
				return false, fmt.Errorf("stat %s: %w", candidate, statErr)
			}

			if exists {
				root = at

				return true, nil
			}
		}

		return false, nil
	})
	if err != nil {
		return "", err
	}

	r.roots.Store(abs, root)

	return root, nil
}
