package run

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/dispatch"
	"github.com/JacobJoergensen/laravel-fmt/internal/git"
)

// Exit 1 means "run the formatter" and exit 2 "the formatter could not run",
// so a failure must never read as drift.
func TestFinishDecidesHowTheRunEnded(t *testing.T) {
	cases := []struct {
		name                       string
		opts                       Options
		failed, changed, unchanged int
		wantErr                    error
		wantDrift                  bool
		wantSummary                string
	}{
		{"failure outranks drift", Options{Check: true}, 1, 2, 0, ErrSilentFailure, false, "1 file failed to format"},
		{"check with drift", Options{Check: true}, 0, 2, 1, ErrDrift, true, "2 files would be reformatted · 1 already formatted"},
		{"diff with drift", Options{Diff: true}, 0, 1, 0, ErrDrift, true, "1 file would be reformatted · 0 already formatted"},
		{"check with none", Options{Check: true}, 0, 0, 3, nil, false, "All files already formatted"},
		{"write with changes", Options{}, 0, 1, 2, nil, false, "Formatted 1 file · 2 unchanged"},
		{"write with none", Options{}, 0, 0, 1, nil, false, "1 file already formatted"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer

			r := &runner{opts: tc.opts, streams: Streams{Err: &stderr}}

			err := r.finish(tc.failed, tc.changed, tc.unchanged, time.Millisecond)

			if !errors.Is(err, tc.wantErr) || (err == nil) != (tc.wantErr == nil) {
				t.Fatalf("finish() = %v, want %v", err, tc.wantErr)
			}

			if errors.Is(err, ErrDrift) != tc.wantDrift {
				t.Fatalf("finish() = %v, drift = %v, want drift %v", err, errors.Is(err, ErrDrift), tc.wantDrift)
			}

			if !strings.Contains(stderr.String(), tc.wantSummary) {
				t.Fatalf("summary %q does not contain %q", stderr.String(), tc.wantSummary)
			}
		})
	}
}

func TestFinishStagedFailsOnlyOnAFileThatDidNotFormat(t *testing.T) {
	var stderr bytes.Buffer

	r := &runner{streams: Streams{Err: &stderr}}

	partial := git.Result{PartiallyStaged: []string{"app/A.php"}}
	if err := r.finishStaged(partial, t.TempDir(), 0); err != nil {
		t.Fatalf("finishStaged(partially staged) = %v, want nil", err)
	}

	if !strings.Contains(stderr.String(), "re-staged; working tree kept") {
		t.Fatalf("partially staged file not reported: %q", stderr.String())
	}

	failed := git.Result{Failed: []git.FileError{{Path: "app/B.php", Err: errors.New("boom")}}}
	if err := r.finishStaged(failed, t.TempDir(), 0); !errors.Is(err, ErrSilentFailure) {
		t.Fatalf("finishStaged(failed) = %v, want ErrSilentFailure", err)
	}
}

func TestSummaryTimingOnlyOnATerminal(t *testing.T) {
	for _, tty := range []bool{false, true} {
		var stderr bytes.Buffer

		r := &runner{streams: Streams{Err: &stderr, ErrIsTTY: tty}}
		r.renderSummary("", "✓", "done", 1500*time.Microsecond)

		if got := strings.Contains(stderr.String(), "1ms"); got != tty {
			t.Fatalf("ErrIsTTY=%v: summary %q shows timing = %v", tty, stderr.String(), got)
		}
	}
}

func TestShowConfigExplainsItsSourcesOnStderr(t *testing.T) {
	writeProject(t, map[string]string{
		"laravel-fmt.json": `{"printWidth": 100, "ignore": ["app/A.php"]}`,
		"app/A.php":        "<?php\n",
	})

	var stdout, stderr bytes.Buffer

	if err := Format(&Options{ShowConfig: "app/A.php"}, Streams{Out: &stdout, Err: &stderr}); err != nil {
		t.Fatalf("Format(--show-config) = %v", err)
	}

	if !json.Valid(stdout.Bytes()) {
		t.Fatalf("stdout is no longer a config:\n%s", stdout.String())
	}

	sourceOf := map[string]string{}

	for line := range strings.Lines(stderr.String()) {
		if fields := strings.Fields(line); len(fields) == 3 {
			sourceOf[fields[0]] = fields[2]
		}
	}

	if sourceOf["printWidth"] != "laravel-fmt.json" || sourceOf["indentWidth"] != "default" {
		t.Fatalf("sources = %v, want printWidth from laravel-fmt.json and indentWidth default", sourceOf)
	}

	if !strings.Contains(stderr.String(), "app/A.php: ignored") {
		t.Fatalf("stderr %q does not report the file as ignored", stderr.String())
	}
}

func TestUnreadPrettierConfigIsWarnedOnEveryRun(t *testing.T) {
	cases := []struct {
		name string
		file string
		body string
		want string
	}{
		{"JS config", "prettier.config.js", "export default {}\n", "prettier.config.js: not read, laravel-fmt reads only JSON"},
		{"shared config", "package.json", "{\n  \"prettier\": \"@vendor/prettier-config\"\n}\n", `package.json: not read, laravel-fmt can't load the shared config`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writeProject(t, map[string]string{
				tc.file:     tc.body,
				"app/A.php": "<?php\n",
			})

			var stderr bytes.Buffer

			if err := Format(&Options{Check: true}, Streams{Out: &bytes.Buffer{}, Err: &stderr}); err != nil {
				t.Fatalf("Format(--check) = %v", err)
			}

			if !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("stderr %q does not contain %q", stderr.String(), tc.want)
			}
		})
	}
}

func TestRenderFileErrorShowsEachParseDiagnostic(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")

	src := []byte("<?php\n$a = ;\n")

	_, parseErr := dispatch.File("x.php", src, config.Default())
	if parseErr == nil {
		t.Fatal("fixture parsed; want a parse error")
	}

	var stderr bytes.Buffer

	r := &runner{streams: Streams{Err: &stderr}}
	abs := filepath.Join(t.TempDir(), "x.php")
	r.renderFileError(abs, "x.php", src, parseErr)

	got := stderr.String()

	for _, want := range []string{"x.php", "2:", "$a = ;", "::error file=", ",line=2,"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered error %q does not contain %q", got, want)
		}
	}
}
