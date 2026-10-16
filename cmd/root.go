package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/JacobJoergensen/laravel-fmt/internal/run"
	"github.com/JacobJoergensen/laravel-fmt/internal/terminal"
)

var colorMode = "auto"

type fmtFlags struct {
	check         bool
	diff          bool
	reporter      string
	staged        bool
	since         string
	config        string
	noConfig      bool
	stdinFilepath string
	showConfig    string
	rangeStart    int
	rangeEnd      int
}

const (
	flagRangeStart = "range-start"
	flagRangeEnd   = "range-end"
	flagSince      = "since"
)

var fmtOpts fmtFlags

var rootCmd = &cobra.Command{
	Use:   "laravel-fmt [paths...]",
	Short: "Format PHP, Blade, and Livewire 4 files",
	Long: `laravel-fmt formats source files in place. With no path arguments it walks
the project root discovered by the detector, skipping what .gitignore and the
ignore list in laravel-fmt.json exclude. With path arguments each path is
formatted directly when it is a file, even an ignored one, or walked the same
way when it is a directory. With --stdin-filepath set, source is read from
stdin and formatted output is written to stdout, with the value used as the
filename for language dispatch. The file itself is never opened. Adding
--range-start or --range-end narrows that output to the changes overlapping
those byte offsets, for editors that format a selection by shelling out.`,
	Example:           "  laravel-fmt\n  app/Models/User.php\n  laravel-fmt --diff\n  laravel-fmt --check\n  laravel-fmt --staged\n  laravel-fmt --check --since origin/main\n  laravel-fmt --config path/to/laravel-fmt.json\n  laravel-fmt --stdin-filepath foo.blade.php < input.php\n  laravel-fmt --stdin-filepath foo.php --range-start 40 --range-end 120 < input.php",
	Version:           versionString(),
	Args:              cobra.ArbitraryArgs,
	SilenceUsage:      true,
	SilenceErrors:     true,
	PersistentPreRunE: rootPreRun,
	RunE:              runFmt,
}

// Execute runs the root command and prints a non-silent error to stderr.
func Execute() error {
	// Cobra's cmd.Print falls back to stderr until an out stream is set. A diff is
	// data a caller pipes (`laravel-fmt --diff | patch`), so it belongs on stdout;
	// status lines and diagnostics stay on stderr.
	rootCmd.SetOut(os.Stdout)

	err := rootCmd.Execute()

	if err == nil {
		return nil
	}

	if !errors.Is(err, run.ErrSilentFailure) {
		fmt.Fprintf(os.Stderr, "%s%s %s%s\n", terminal.Red, terminal.CrossMark, err, terminal.Reset)
	}

	return fmt.Errorf("execute root command: %w", err)
}

const (
	exitDrift = 1
	exitError = 2
)

// ExitCode maps an Execute error to Prettier's exit contract.
func ExitCode(err error) int {
	if errors.Is(err, run.ErrDrift) {
		return exitDrift
	}

	return exitError
}

// version and commit are stamped by the release build via -ldflags -X (see
// .goreleaser.yaml).
var (
	version string
	commit  string
)

func releaseVersion() string {
	if version != "" {
		return version
	}

	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
		return info.Main.Version
	}

	return ""
}

func buildCommit() string {
	c := commit

	if c == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" {
					c = s.Value

					break
				}
			}
		}
	}

	return c[:min(len(c), 7)]
}

func versionString() string {
	v := releaseVersion()
	if v == "" {
		v = "(devel)"
	}

	out := "laravel-fmt " + v

	if c := buildCommit(); c != "" {
		out += " " + c
	}

	return fmt.Sprintf("%s %s/%s", out, runtime.GOOS, runtime.GOARCH)
}

func rootPreRun(_ *cobra.Command, _ []string) error {
	switch colorMode {
	case "auto":
		// terminal init has already chosen the right state.
	case "never":
		terminal.Disable()
	case "always":
		terminal.Enable()
	default:
		return fmt.Errorf("--color must be one of: auto, never, always (got %q)", colorMode)
	}

	return nil
}

func init() {
	rootCmd.PersistentFlags().StringVar(&colorMode, "color", "auto", "color output: auto, never, always")
	rootCmd.Flags().BoolVar(&fmtOpts.check, "check", false, "check if files are formatted; print the paths that would change without writing")
	rootCmd.Flags().BoolVar(&fmtOpts.diff, "diff", false, "print a line-level diff of the changes without writing")
	rootCmd.Flags().StringVar(&fmtOpts.reporter, "reporter", "", "with --check, write a report to stdout for CI: gitlab (Code Quality)")
	rootCmd.Flags().BoolVar(&fmtOpts.staged, "staged", false, "format only files staged in git and re-stage them, leaving unstaged changes untouched")
	rootCmd.Flags().StringVar(&fmtOpts.since, flagSince, "", "format only files changed since the given git ref branched off, including uncommitted and untracked ones")
	rootCmd.Flags().StringVar(&fmtOpts.config, "config", "", "path to a laravel-fmt.json; overrides auto-discovery")
	rootCmd.Flags().BoolVar(&fmtOpts.noConfig, "no-config", false, "skip laravel-fmt.json and use defaults plus detector tweaks")
	rootCmd.Flags().StringVar(&fmtOpts.stdinFilepath, "stdin-filepath", "", "read source from stdin and write formatted output to stdout; the value is the filename used for language dispatch")
	rootCmd.Flags().StringVar(&fmtOpts.showConfig, "show-config", "", "print the config resolved for this file as JSON and exit, without formatting anything")
	rootCmd.Flags().IntVar(&fmtOpts.rangeStart, flagRangeStart, 0, "with --stdin-filepath, apply only the changes at or after this byte offset of the input")
	rootCmd.Flags().IntVar(&fmtOpts.rangeEnd, flagRangeEnd, 0, "with --stdin-filepath, apply only the changes before this byte offset of the input, exclusive; defaults to the end of the input")
	rootCmd.SetVersionTemplate("{{.Version}}\n")
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(lspCmd)
}

func runFmt(cmd *cobra.Command, args []string) error {
	opts := fmtOpts.options(cmd, args)
	streams := run.Streams{
		In:       cmd.InOrStdin(),
		Out:      cmd.OutOrStdout(),
		Err:      cmd.ErrOrStderr(),
		ErrIsTTY: terminal.IsTTY(cmd.ErrOrStderr()),
		OutColor: outColor(cmd.OutOrStdout()),
	}

	return run.Format(&opts, streams)
}

// outColor decides color for stdout the way --color decides it for stderr, but
// from stdout's own terminal: a diff piped to a file must stay a plain patch.
func outColor(out io.Writer) bool {
	switch colorMode {
	case "always":
		return true
	case "never":
		return false
	default:
		return terminal.WantsColor(out)
	}
}

func (f fmtFlags) options(cmd *cobra.Command, args []string) run.Options {
	opts := run.Options{
		Paths:         args,
		Check:         f.check,
		Diff:          f.diff,
		Reporter:      f.reporter,
		Staged:        f.staged,
		Config:        f.config,
		NoConfig:      f.noConfig,
		StdinFilepath: f.stdinFilepath,
		ShowConfig:    f.showConfig,
	}

	if cmd.Flags().Changed(flagSince) {
		opts.Since = new(f.since)
	}

	if cmd.Flags().Changed(flagRangeStart) {
		opts.RangeStart = new(f.rangeStart)
	}

	if cmd.Flags().Changed(flagRangeEnd) {
		opts.RangeEnd = new(f.rangeEnd)
	}

	return opts
}
