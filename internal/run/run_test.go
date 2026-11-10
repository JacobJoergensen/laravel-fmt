package run

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/detector"
	"github.com/JacobJoergensen/laravel-fmt/internal/terminal"
	"github.com/JacobJoergensen/laravel-fmt/internal/upward"
)

// A fixture must not pick up the host's files above the temp directory.
func init() {
	upward.SetCeiling(os.TempDir())
}

// failingWriter stands in for a stdout whose reader went away: a closed pipe
// or a full disk.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("broken pipe")
}

func writeProject(t *testing.T, files map[string]string) {
	t.Helper()

	// A run reads the CI it runs in from the environment; under a real job it
	// would annotate, append to the job summary, or rebase report paths.
	for _, name := range []string{"GITHUB_ACTIONS", "GITHUB_STEP_SUMMARY", "CI_PROJECT_DIR"} {
		t.Setenv(name, "")
	}

	root := t.TempDir()

	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Chdir(root)
}

func TestFailedDiffWriteFailsTheFile(t *testing.T) {
	writeProject(t, map[string]string{"app/Dirty.php": "<?php\nclass Dirty{}\n"})

	var stderr bytes.Buffer

	err := Format(&Options{Diff: true}, Streams{Out: failingWriter{}, Err: &stderr})

	if !errors.Is(err, ErrSilentFailure) || errors.Is(err, ErrDrift) {
		t.Fatalf("Format = %v, want a failure, not drift", err)
	}

	if !strings.Contains(stderr.String(), "write diff: broken pipe") {
		t.Fatalf("stderr %q does not report the failed write", stderr.String())
	}
}

func TestShowConfigReportsAFailedWrite(t *testing.T) {
	writeProject(t, map[string]string{"app/A.php": "<?php\n"})

	err := Format(&Options{ShowConfig: "app/A.php"}, Streams{Out: failingWriter{}, Err: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "write stdout") {
		t.Fatalf("Format(--show-config) = %v, want the write error", err)
	}
}

func TestDiffColorFollowsStdoutNotStderr(t *testing.T) {
	terminal.Enable()
	t.Cleanup(terminal.Disable)

	for _, outColor := range []bool{false, true} {
		writeProject(t, map[string]string{"app/Dirty.php": "<?php\nclass Dirty{}\n"})

		var stdout bytes.Buffer

		err := Format(&Options{Diff: true}, Streams{Out: &stdout, Err: &bytes.Buffer{}, OutColor: outColor})
		if !errors.Is(err, ErrDrift) {
			t.Fatalf("Format(--diff) = %v, want ErrDrift", err)
		}

		if got := strings.Contains(stdout.String(), "\033["); got != outColor {
			t.Fatalf("OutColor=%v: patch has escapes = %v:\n%q", outColor, got, stdout.String())
		}
	}
}

func TestGitLabReportGoesToStdoutAndKeepsTheDriftExit(t *testing.T) {
	writeProject(t, map[string]string{"app/Dirty.php": "<?php\n\nclass Dirty{}\n"})

	var stdout bytes.Buffer

	err := Format(&Options{Check: true, Reporter: ReporterGitLab}, Streams{Out: &stdout, Err: &bytes.Buffer{}})
	if !errors.Is(err, ErrDrift) {
		t.Fatalf("Format = %v, want ErrDrift", err)
	}

	for _, want := range []string{`"path": "app/Dirty.php"`, `"begin": 3`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("report missing %s:\n%s", want, stdout.String())
		}
	}
}

func TestFailedGitLabReportWriteFailsTheRun(t *testing.T) {
	writeProject(t, map[string]string{"app/Clean.php": "<?php\n"})

	err := Format(&Options{Check: true, Reporter: ReporterGitLab}, Streams{Out: failingWriter{}, Err: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "write GitLab report: broken pipe") {
		t.Fatalf("Format = %v, want the write error", err)
	}
}

func TestStdinFormatsForTheProjectOfItsPath(t *testing.T) {
	const unparseable = "<?php\nfunction (\n"

	cases := []struct {
		name    string
		config  string
		path    string
		src     string
		want    string
		wantErr error
	}{
		{"ignored file is echoed", "", "app/skip.php", unparseable, unparseable, nil},
		{"pinned config's ignore", "pinned/laravel-fmt.json", "pinned/sub/a.php", unparseable, unparseable, nil},
		{
			"nested project's config",
			"",
			"sub/a.php",
			"<?php\n\n$x = foo($aaaa, $bbbb, $cccc);\n",
			"<?php\n\n$x = foo(\n    $aaaa,\n    $bbbb,\n    $cccc,\n);\n",
			nil,
		},
		{"parse error writes nothing", "", "app/a.php", unparseable, "", ErrSilentFailure},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writeProject(t, map[string]string{
				"laravel-fmt.json":        `{"ignore": ["app/skip.php"]}`,
				"sub/laravel-fmt.json":    `{"printWidth": 20}`,
				"pinned/laravel-fmt.json": `{"ignore": ["sub/**"]}`,
			})

			var stdout bytes.Buffer

			streams := Streams{In: strings.NewReader(tc.src), Out: &stdout, Err: &bytes.Buffer{}}

			err := Format(&Options{Config: tc.config, StdinFilepath: tc.path}, streams)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Format = %v, want %v", err, tc.wantErr)
			}

			if stdout.String() != tc.want {
				t.Fatalf("stdout = %q, want %q", stdout.String(), tc.want)
			}
		})
	}
}

func TestStdinPathRelativeToASubfolderKeepsItsLanguageRules(t *testing.T) {
	const guideline = "## Rules\n\n<div><p>Use @{{ $x }}</p></div>\n"

	writeProject(t, map[string]string{
		"composer.json": `{"name":"acme/pkg"}`,
		"resources/boost/guidelines/core.blade.php": guideline,
	})
	t.Chdir("resources")

	var stdout bytes.Buffer

	streams := Streams{In: strings.NewReader(guideline), Out: &stdout, Err: &bytes.Buffer{}}

	if err := Format(&Options{StdinFilepath: "boost/guidelines/core.blade.php"}, streams); err != nil {
		t.Fatalf("Format = %v", err)
	}

	if stdout.String() != guideline {
		t.Fatalf("stdout = %q, want the guideline unchanged", stdout.String())
	}
}

func TestStdinPathNoFileCanHaveFormatsAsAnyOther(t *testing.T) {
	// app/ must exist: under a missing parent Windows reports not-exist, and
	// only under a real one does it answer ERROR_INVALID_NAME.
	writeProject(t, map[string]string{
		"composer.json": `{"name":"acme/pkg"}`,
		"app/B.php":     "<?php\n",
	})

	for _, path := range []string{"app/**/A.php", "app/what?/A.php"} {
		t.Run(path, func(t *testing.T) {
			var stdout bytes.Buffer

			streams := Streams{In: strings.NewReader("<?php\n$a=1;\n"), Out: &stdout, Err: &bytes.Buffer{}}

			if err := Format(&Options{StdinFilepath: path}, streams); err != nil {
				t.Fatalf("Format = %v", err)
			}

			if want := "<?php\n\n$a = 1;\n"; stdout.String() != want {
				t.Fatalf("stdout = %q, want %q", stdout.String(), want)
			}
		})
	}
}

func TestDiffHeadersNameFilesFromWhereThePatchApplies(t *testing.T) {
	tests := []struct {
		name  string
		dir   string
		paths []string
		want  string
	}{
		{"a walk from a subfolder covers the app root", "sub", nil, "--- a/app/a.php"},
		{"a named path stays relative to where it was named", "app", []string{"a.php"}, "--- a/a.php"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeProject(t, map[string]string{
				"artisan":       "",
				"composer.json": `{"require":{"laravel/framework":"^12.0"}}`,
				"app/a.php":     "<?php\n$a=1;\n",
				"sub/keep.txt":  "",
			})
			t.Chdir(tt.dir)

			var stdout bytes.Buffer

			err := Format(&Options{Diff: true, Paths: tt.paths}, Streams{Out: &stdout, Err: &bytes.Buffer{}})
			if !errors.Is(err, ErrDrift) {
				t.Fatalf("Format(--diff) = %v, want ErrDrift", err)
			}

			var headers []string

			for line := range strings.SplitSeq(stdout.String(), "\n") {
				if strings.HasPrefix(line, "--- ") {
					headers = append(headers, line)
				}
			}

			if !slices.Contains(headers, tt.want) {
				t.Fatalf("headers %q, want %q among them", headers, tt.want)
			}

			for _, header := range headers {
				if strings.Contains(header, "../") {
					t.Fatalf("header %q climbs out of the directory", header)
				}
			}
		})
	}
}

// --since lists files from the git root, but its patch applies where the walk's
// does: a git root above the app would name each file from a folder git apply
// does not run in.
func TestSinceDiffIsRelativeToTheAppRoot(t *testing.T) {
	appRoot := filepath.Join(t.TempDir(), "web")
	r := &runner{opts: Options{Since: new("HEAD")}, workDir: filepath.Join(appRoot, "app")}

	got := r.diffBase(detector.Context{Laravel: &detector.Laravel{Root: appRoot}})
	if got != appRoot {
		t.Fatalf("diffBase = %q, want the app root %q", got, appRoot)
	}
}

// The line diff pairs the blank lines, so re-indenting <p>b</p> becomes a
// delete in one change and an insert in the next.
func TestStdinRangeKeepsWhatAPartialChangeWouldDrop(t *testing.T) {
	writeProject(t, map[string]string{})

	src := "<div>\n\n<p>a</p>\n\n\n<p>b</p>\n\n</div>\n"

	var stdout bytes.Buffer

	streams := Streams{In: strings.NewReader(src), Out: &stdout, Err: &bytes.Buffer{}}
	opts := &Options{StdinFilepath: "page.html", RangeStart: new(7), RangeEnd: new(15)}

	if err := Format(opts, streams); err != nil {
		t.Fatalf("Format = %v", err)
	}

	if !strings.Contains(stdout.String(), "<p>b</p>") {
		t.Fatalf("range output lost <p>b</p>:\n%s", stdout.String())
	}
}

func TestUnreadableFolderFailsTheRunButNotTheOtherFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits")
	}

	if os.Geteuid() == 0 {
		t.Skip("root reads every folder")
	}

	writeProject(t, map[string]string{
		"ok.php":       "<?php\n$a=[1,2];\n",
		"locked/x.php": "<?php\n",
	})

	if err := os.Chmod("locked", 0); err != nil {
		t.Fatal(err)
	}

	//nolint:gosec // restores the folder so t.TempDir can remove it
	t.Cleanup(func() { _ = os.Chmod("locked", 0o755) })

	var stderr bytes.Buffer

	err := Format(&Options{}, Streams{Out: &bytes.Buffer{}, Err: &stderr})
	if !errors.Is(err, ErrSilentFailure) {
		t.Fatalf("Format = %v, want a failed run", err)
	}

	got, readErr := os.ReadFile("ok.php")
	if readErr != nil {
		t.Fatal(readErr)
	}

	if string(got) != "<?php\n\n$a = [1, 2];\n" {
		t.Fatalf("ok.php = %q, want it formatted beside the unreadable folder", got)
	}

	for _, want := range []string{"locked" + string(filepath.Separator), "read: permission denied\n", "1 folder could not be read"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr lacks %q:\n%s", want, stderr.String())
		}
	}
}
