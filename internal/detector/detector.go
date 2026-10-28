// Package detector finds the project settings that shape formatting.
package detector

import "github.com/JacobJoergensen/laravel-fmt/internal/upward"

// Context is a project's detected sources; a missing one is a nil field.
type Context struct {
	WorkDir    string
	ConfigPath string
	// NestedConfigPath is a laravel-fmt.json below the project root. It does
	// not apply, and is reported so its author is not left wondering why.
	NestedConfigPath string
	Laravel          *Laravel
	Pint             *Pint
	// PhpCsFixer uses the Pint type: Pint wraps PHP-CS-Fixer, so the rules
	// share names and shapes.
	PhpCsFixer   *Pint
	Mago         *Mago
	EditorConfig *EditorConfigResolver
	Prettier     *Prettier
	// PrettierResolver finds the Prettier config nearest each file; Prettier is
	// its answer for the project root.
	PrettierResolver *PrettierResolver
	Tailwind         *Tailwind
}

// ConfigDir returns the directory laravel-fmt.json belongs in.
func (c Context) ConfigDir() string {
	if c.Laravel != nil {
		return c.Laravel.Root
	}

	return c.WorkDir
}

// Detect probes workDir, or the Laravel root above it when there is one.
func Detect(workDir string) (Context, error) {
	return detectWith(workDir, &probeMemo{})
}

// The memo lets a run that detects several roots probe each directory once.
func detectWith(workDir string, memo *probeMemo) (Context, error) {
	ctx := Context{WorkDir: workDir}

	laravel, laravelFound, err := detectLaravel(workDir, memo)
	if err != nil {
		return Context{}, err
	}

	if laravelFound {
		ctx.Laravel = &laravel
	}

	// Inside a project the config comes from its root alone, so no
	// laravel-fmt.json further down can make a file format differently
	// depending on which paths the run was given.
	configFrom, configStopAt := ctx.ConfigDir(), ""

	if laravelFound {
		configStopAt = laravel.Root
	}

	configPath, err := findConfig(configFrom, configStopAt, memo)
	if err != nil {
		return Context{}, err
	}

	ctx.ConfigPath = configPath

	nested, err := nestedConfig(workDir, configFrom, configPath, memo)
	if err != nil {
		return Context{}, err
	}

	ctx.NestedConfigPath = nested

	// Inside an application the probes answer from its root. Outside one they
	// walk up from the given paths, since a package keeps its .prettierrc at
	// the top.
	probeRoot, probeStop := workDir, ""

	if laravelFound {
		probeRoot, probeStop = laravel.Root, laravel.Root
	}

	if err := detectUp(&ctx.Pint, probeRoot, probeStop, memo, probePint, detectPint); err != nil {
		return Context{}, err
	}

	if err := detectUp(&ctx.PhpCsFixer, probeRoot, probeStop, memo, probePhpCsFixer, detectPhpCsFixer); err != nil {
		return Context{}, err
	}

	if err := detectUp(&ctx.Mago, probeRoot, probeStop, memo, probeMago, detectMago); err != nil {
		return Context{}, err
	}

	editorConfig, err := newEditorConfigResolver(probeRoot, memo)
	if err != nil {
		return Context{}, err
	}

	ctx.EditorConfig = editorConfig

	if err := detectUp(&ctx.Prettier, probeRoot, probeStop, memo, probePrettier, detectPrettier); err != nil {
		return Context{}, err
	}

	ctx.PrettierResolver = newPrettierResolver(ctx.Prettier, probeRoot, probeStop, memo)

	if err := detectUp(&ctx.Tailwind, probeRoot, probeStop, memo, probeTailwind, detectTailwind); err != nil {
		return Context{}, err
	}

	return ctx, nil
}

func detectUp[T any](dst **T, root, stopAt string, memo *probeMemo, kind probeKind, detect func(string) (T, bool, error)) error {
	return upward.Walk(root, func(dir string) (bool, error) {
		value, found, err := probeDir(memo, kind, dir, detect)
		if err != nil {
			return false, err
		}

		if found {
			*dst = &value

			return true, nil
		}

		return stopAt != "" && dir == stopAt, nil
	})
}
