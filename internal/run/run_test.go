package run

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
