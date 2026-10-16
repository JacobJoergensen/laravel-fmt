// Package run carries out one formatting run: it selects the files, formats
// each under its own project's config, and writes or reports the result.
package run

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/JacobJoergensen/laravel-fmt/internal/ci"
	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/detector"
	"github.com/JacobJoergensen/laravel-fmt/internal/diff"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/dispatch"
	"github.com/JacobJoergensen/laravel-fmt/internal/git"
	"github.com/JacobJoergensen/laravel-fmt/internal/parallel"
	"github.com/JacobJoergensen/laravel-fmt/internal/rewrite"
	"github.com/JacobJoergensen/laravel-fmt/internal/terminal"
	"github.com/JacobJoergensen/laravel-fmt/internal/walk"
)

// ErrSilentFailure signals a non-zero exit with no extra stderr message.
var ErrSilentFailure = errors.New("silent failure")

// ErrDrift signals that files would be reformatted.
var ErrDrift = fmt.Errorf("drift: %w", ErrSilentFailure)

// batchGCPercent is the GC target for a batch run. Measured on a 1,695-file
// project it kept the run time streaming had lost; 300 and above bought little
// more speed for most of the memory back.
const batchGCPercent = 200

// Streams are the run's standard streams. ErrIsTTY shows the summary's
// timing, left off when piped so CI output stays byte-stable; OutColor decides
// color on Out alone, since Out is often piped while Err is a terminal.
type Streams struct {
	In       io.Reader
	Out      io.Writer
	Err      io.Writer
	ErrIsTTY bool
	OutColor bool
}

type runner struct {
	opts    Options
	streams Streams
	workDir string
	start   time.Time
}

// Format runs one invocation. Only a diff, a report and formatted stdin go to
// Out, so each can be piped. It returns ErrSilentFailure after reporting its own
// failure, and ErrDrift when --check or --diff found unformatted files.
func Format(opts *Options, streams Streams) error {
	if err := opts.validate(); err != nil {
		return err
	}

	start := time.Now()

	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}

	r := &runner{opts: *opts, streams: streams, workDir: workDir, start: start}

	// Config is searched from what is being formatted, not where the process
	// stands; the walk still resolves relative arguments against the working
	// directory, as a relative path means.
	anchor, err := detector.AnchorDir(workDir, opts.configAnchorPaths())
	if err != nil {
		return err
	}

	ctx, err := detector.Detect(anchor)
	if err != nil {
		return fmt.Errorf("detect context: %w", err)
	}

	loaded, err := r.loadConfig(anchor, ctx)
	if err != nil {
		return err
	}

	r.warnNestedConfig(ctx)

	cfg := loaded.File

	projects := detector.NewResolver(r.loadConfig)

	switch {
	case opts.ShowConfig != "":
		return r.showConfig(projects)
	case opts.StdinFilepath != "":
		return r.stdin(projects)
	case opts.Staged:
		return r.staged(&cfg, projects)
	}

	files, err := r.selectFiles(ctx, &cfg, projects)
	if err != nil {
		return err
	}

	return r.batch(files, projects)
}

func (r *runner) outPalette() terminal.Palette {
	if r.streams.OutColor {
		return terminal.Colors()
	}

	return terminal.Palette{}
}

type fileResult struct {
	src       []byte
	formatted []byte
	changed   bool
}

type fileResultOrErr struct {
	res fileResult
	err error
}

func (r *runner) batch(files []string, projects *detector.Resolver) error {
	failed := 0

	var changed []ci.Change

	var failures []ci.Failure

	// Files format on every core and come back in input order, so --check and
	// --diff output is stable and writes never interleave; each result is
	// dropped once handled.
	workers := runtime.GOMAXPROCS(0)

	// Streaming leaves little live data, and at the default GOGC the collector
	// ran two and a half times as often and cost a third of the run. Doubling
	// the target restores the speed and keeps the peak well under what holding
	// every result used to cost. A GOGC the user set wins.
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(batchGCPercent)
	}

	parallel.Ordered(len(files), workers, 2*workers, func(i int) fileResultOrErr {
		return formatInProject(projects, files[i])
	}, func(i int, res fileResultOrErr) {
		path := files[i]
		shown := displayPath(r.workDir, path)

		fail := func(err error) {
			r.renderFileError(path, shown, res.res.src, err)
			failures = append(failures, collectFailures(shown, err)...)
			failed++
		}

		if res.err != nil {
			fail(res.err)

			return
		}

		if !res.res.changed {
			return
		}

		switch {
		case r.opts.Diff:
			// Only the patch goes to stdout, so it can be piped to git apply;
			// the file headers name each file, and the summary goes to stderr.
			// A patch that did not reach its reader fails the file, or a
			// truncated one would pass for ordinary drift.
			patch := diff.Unified(filepath.ToSlash(shown), string(res.res.src), string(res.res.formatted), r.outPalette())
			if _, err := io.WriteString(r.streams.Out, patch); err != nil {
				fail(fmt.Errorf("write diff: %w", err))

				return
			}
		case r.opts.Check:
			r.renderCheckPath(shown)

			if ci.OnGitHubActions() {
				ci.GitHubError(r.streams.Err, path, 0, 0, ci.UnformattedMessage)
			}
		default:
			if err := rewrite.Rewrite(path, res.res.src, res.res.formatted); err != nil {
				fail(fmt.Errorf("write: %w", err))

				return
			}

			r.renderCheckPath(shown)
		}

		changed = append(changed, ci.Change{
			Path: shown,
			Line: diff.FirstChangedLine(res.res.src, res.res.formatted),
		})
	})

	unchanged := len(files) - len(changed) - failed

	summary := ci.Summary{
		Check:     r.opts.Check || r.opts.Diff,
		Changed:   changed,
		Unchanged: unchanged,
		Failures:  failures,
	}

	writeJobSummary(summary)

	// Unlike the job summary, the report is what the user asked for, so a
	// report that did not reach its reader fails the run.
	if r.opts.Reporter == ReporterGitLab {
		if err := ci.WriteGitLabReport(r.streams.Out, summary, r.workDir, ci.GitLabProjectDir()); err != nil {
			return err
		}
	}

	return r.finish(failed, len(changed), unchanged, time.Since(r.start))
}

func resolveFileConfig(projects *detector.Resolver, path string) (*config.File, error) {
	project, err := projects.For(path)
	if err != nil {
		return nil, err
	}

	resolved, err := detector.ResolveFileConfig(&project.Cfg, path, project.Ctx, project.UserKeys)
	if err != nil {
		return nil, err
	}

	return &resolved, nil
}

// formatInProject runs on a worker, so it only reads and formats; the caller
// writes and prints.
func formatInProject(projects *detector.Resolver, path string) fileResultOrErr {
	fileCfg, err := resolveFileConfig(projects, path)
	if err != nil {
		return fileResultOrErr{err: err}
	}

	res, err := formatFile(path, fileCfg)

	return fileResultOrErr{res: res, err: err}
}

// A parser diagnostic surfaces as an error so the caller leaves the
// file untouched on disk instead of writing recovery output.
func formatFile(path string, cfg *config.File) (fileResult, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return fileResult{}, fmt.Errorf("read: %w", err)
	}

	config.ResolveEndOfLine(src, cfg)

	out, err := dispatch.File(path, src, cfg)
	if err != nil {
		// Keep src on the failure path so the caller can render a
		// source snippet under each parse diagnostic.
		return fileResult{src: src}, err
	}

	return fileResult{
		src:       src,
		formatted: out,
		changed:   !bytes.Equal(out, src),
	}, nil
}

func (r *runner) loadConfig(anchorDir string, ctx detector.Context) (config.Loaded, error) {
	switch {
	case r.opts.NoConfig:
		return detector.LoadDefaults(ctx)
	case r.opts.Config != "":
		return detector.LoadConfigFile(r.opts.Config, ctx)
	default:
		return detector.LoadConfig(anchorDir, ctx)
	}
}

// projectIgnore holds each file to its own project's ignore patterns as well
// as the run's. A pinned --config is the one authority, so it adds none.
func (r *runner) projectIgnore(projects *detector.Resolver) walk.ProjectIgnore {
	if r.opts.Config != "" {
		return nil
	}

	return func(dir string) (string, *config.File, error) {
		project, err := projects.ForDir(dir)
		if err != nil {
			return "", nil, err
		}

		return project.IgnoreAnchor(), &project.Cfg, nil
	}
}

// ignoreAnchor names the config file whose ignore patterns are in play, since
// they are written relative to it. --no-config leaves only the defaults, which
// are project-root relative, so it anchors on nothing.
func (r *runner) ignoreAnchor(ctx detector.Context) string {
	switch {
	case r.opts.NoConfig:
		return ""
	case r.opts.Config != "":
		return r.opts.Config
	default:
		return ctx.ConfigPath
	}
}

// runIgnoreAnchor returns where the run's ignore patterns are written from:
// the pinned --config's directory, or that of the project dir belongs to.
func (r *runner) runIgnoreAnchor(dir string, projects *detector.Resolver) (string, error) {
	if r.opts.Config != "" {
		abs, err := filepath.Abs(r.opts.Config)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", r.opts.Config, err)
		}

		return filepath.Dir(abs), nil
	}

	project, err := projects.ForDir(dir)
	if err != nil {
		return "", err
	}

	return project.IgnoreAnchor(), nil
}

func (r *runner) selectFiles(ctx detector.Context, cfg *config.File, projects *detector.Resolver) ([]string, error) {
	if r.opts.Since != nil {
		return r.changedFiles(cfg, projects)
	}

	root := r.workDir

	if ctx.Laravel != nil {
		root = ctx.Laravel.Root
	}

	ignoreRoot := walk.IgnoreRoot(root, r.ignoreAnchor(ctx))

	return walk.Files(root, ignoreRoot, cfg, r.opts.Paths, r.projectIgnore(projects))
}

// Ignore rules apply as they do to --staged, since git, not the user, named
// these files.
func (r *runner) changedFiles(cfg *config.File, projects *detector.Resolver) ([]string, error) {
	repo, err := git.Open(r.workDir)
	if err != nil {
		return nil, fmt.Errorf("--since requires a git repository: %w", err)
	}

	root := repo.Root()
	if err := repo.Close(); err != nil {
		return nil, err
	}

	runAnchor, err := r.runIgnoreAnchor(r.workDir, projects)
	if err != nil {
		return nil, err
	}

	skip, err := walk.IgnoreFilter(root, runAnchor, cfg, r.projectIgnore(projects))
	if err != nil {
		return nil, err
	}

	return git.ChangedFiles(root, *r.opts.Since, skip)
}

// Works through the git index so a file's unstaged working-tree edits
// are never disturbed; the working copy is rewritten only when it still
// matches the pre-format staged content. Exits non-zero only on a
// per-file failure, so a pre-commit hook can still proceed.
func (r *runner) staged(cfg *config.File, projects *detector.Resolver) error {
	repo, err := git.Open(r.workDir)
	if err != nil {
		return fmt.Errorf("--staged requires a git repository: %w", err)
	}

	runAnchor, err := r.runIgnoreAnchor(r.workDir, projects)
	if err != nil {
		return err
	}

	skip, err := walk.IgnoreFilter(repo.Root(), runAnchor, cfg, r.projectIgnore(projects))
	if err != nil {
		return err
	}

	format := func(relPath string, src []byte) ([]byte, error) {
		path := repoPath(repo.Root(), relPath)

		fileCfg, err := resolveFileConfig(projects, path)
		if err != nil {
			return nil, err
		}

		config.ResolveEndOfLine(src, fileCfg)

		return dispatch.File(relPath, src, fileCfg)
	}

	res, err := git.FormatStaged(repo, format, skip)
	if closeErr := repo.Close(); closeErr != nil {
		err = errors.Join(err, closeErr)
	}

	if err != nil {
		return err
	}

	return r.finishStaged(res, repo.Root(), time.Since(r.start))
}

func repoPath(root, relPath string) string {
	return filepath.Join(root, filepath.FromSlash(relPath))
}

// The file at --stdin-filepath is never opened; it only selects the language
// and project. A parse failure writes nothing to Out and returns
// ErrSilentFailure, so an editor keeps its buffer.
func (r *runner) stdin(projects *detector.Resolver) error {
	ignored, err := r.stdinIgnored(projects)
	if err != nil {
		return err
	}

	if ignored {
		// Echoed back unchanged, so an editor keeps its buffer.
		if _, err := io.Copy(r.streams.Out, r.streams.In); err != nil {
			return fmt.Errorf("copy stdin: %w", err)
		}

		return nil
	}

	cfg, err := resolveFileConfig(projects, r.opts.StdinFilepath)
	if err != nil {
		return err
	}

	src, err := io.ReadAll(r.streams.In)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	config.ResolveEndOfLine(src, cfg)

	if r.opts.ranged() {
		config.DisableRelocatingPasses(cfg)
	}

	out, err := dispatch.File(r.opts.StdinFilepath, src, cfg)
	if err != nil {
		// The label is the caller's and names no file read here, so an editor
		// gets back exactly what it sent.
		r.renderFileError(r.opts.StdinFilepath, r.opts.StdinFilepath, src, err)

		return ErrSilentFailure
	}

	if r.opts.ranged() {
		start, end := r.opts.resolveRange(len(src))
		parses := func(b []byte) bool {
			_, err := dispatch.File(r.opts.StdinFilepath, b, cfg)

			return err == nil
		}

		out = diff.ApplyInRange(src, out, start, end, parses)
	}

	if _, err := r.streams.Out.Write(out); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}

	return nil
}

// An editor sends --stdin-filepath on every save, so it is held to the
// project's ignore rules, not the override a typed path gets.
func (r *runner) stdinIgnored(projects *detector.Resolver) (bool, error) {
	path, err := filepath.Abs(r.opts.StdinFilepath)
	if err != nil {
		return false, fmt.Errorf("resolve %s: %w", r.opts.StdinFilepath, err)
	}

	project, err := projects.For(path)
	if err != nil {
		return false, err
	}

	anchor, err := r.runIgnoreAnchor(filepath.Dir(path), projects)
	if err != nil {
		return false, err
	}

	return walk.IgnoresFile(anchor, &project.Cfg, path)
}

// showConfig prints one file's resolved config as a laravel-fmt.json.
func (r *runner) showConfig(projects *detector.Resolver) error {
	cfg, err := resolveFileConfig(projects, r.opts.ShowConfig)
	if err != nil {
		return err
	}

	out, err := config.Generate(cfg)
	if err != nil {
		return fmt.Errorf("show config: %w", err)
	}

	if _, err := r.streams.Out.Write(out); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}

	return nil
}
