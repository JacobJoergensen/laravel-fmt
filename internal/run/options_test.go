package run

import (
	"strings"
	"testing"
)

func TestValidateRefusesFlagsThatCannotCombine(t *testing.T) {
	cases := []struct {
		name string
		opts Options
		want string
	}{
		{"config and no-config", Options{Config: "a.json", NoConfig: true}, "mutually exclusive"},
		{"show-config with a path", Options{ShowConfig: "x.php", Paths: []string{"app"}}, "--show-config takes one file"},
		{"show-config with check", Options{ShowConfig: "x.php", Check: true}, "--show-config takes one file"},
		{"show-config with since", Options{ShowConfig: "x.php", Since: new("HEAD")}, "--show-config takes one file"},
		{"staged with stdin", Options{Staged: true, StdinFilepath: "x.php"}, "--staged does not combine with --stdin-filepath"},
		{"staged with since", Options{Staged: true, Since: new("HEAD")}, "--staged does not combine with --since"},
		{"staged with a path", Options{Staged: true, Paths: []string{"app"}}, "--staged does not accept path arguments"},
		{"staged with check", Options{Staged: true, Check: true}, "--staged does not combine with --check"},
		{"staged with diff", Options{Staged: true, Diff: true}, "--staged does not combine with --diff"},
		{"empty since", Options{Since: new("")}, "--since needs a git ref"},
		{"since with stdin", Options{Since: new("HEAD"), StdinFilepath: "x.php"}, "--since does not combine with --stdin-filepath"},
		{"since with a path", Options{Since: new("HEAD"), Paths: []string{"app"}}, "--since does not accept path arguments"},
		{"stdin with a path", Options{StdinFilepath: "x.php", Paths: []string{"app"}}, "--stdin-filepath does not accept path arguments"},
		{"stdin with check", Options{StdinFilepath: "x.php", Check: true}, "--stdin-filepath does not combine with --check"},
		{"stdin with diff", Options{StdinFilepath: "x.php", Diff: true}, "--stdin-filepath does not combine with --diff"},
		{"range without stdin", Options{RangeStart: new(1)}, "require --stdin-filepath"},
		{"negative range", Options{StdinFilepath: "x.php", RangeStart: new(-1)}, "must not be negative"},
		{"range end before start", Options{StdinFilepath: "x.php", RangeStart: new(5), RangeEnd: new(2)}, "--range-end (2) must not come before --range-start (5)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validate() = %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

func TestValidateAcceptsTheSupportedCombinations(t *testing.T) {
	cases := []struct {
		name string
		opts Options
	}{
		{"no flags", Options{}},
		{"check with paths", Options{Check: true, Paths: []string{"app"}}},
		{"check and diff since a ref", Options{Check: true, Diff: true, Since: new("origin/main")}},
		{"staged alone", Options{Staged: true}},
		{"show-config alone", Options{ShowConfig: "x.php"}},
		{"stdin with a range end only", Options{StdinFilepath: "x.php", RangeEnd: new(0)}},
		{"stdin with a start past the end", Options{StdinFilepath: "x.php", RangeStart: new(9), RangeEnd: new(9)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.opts.validate(); err != nil {
				t.Fatalf("validate() = %v, want nil", err)
			}
		})
	}
}

func TestResolveRangeClampsToTheInput(t *testing.T) {
	cases := []struct {
		name       string
		opts       Options
		start, end int
	}{
		{"unset end is the end of the input", Options{RangeStart: new(3)}, 3, 10},
		{"unset start is the beginning", Options{RangeEnd: new(4)}, 0, 4},
		{"end past the input", Options{RangeStart: new(2), RangeEnd: new(50)}, 2, 10},
		{"start past the input", Options{RangeStart: new(50)}, 10, 10},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end := tc.opts.resolveRange(10)
			if start != tc.start || end != tc.end {
				t.Fatalf("resolveRange(10) = (%d, %d), want (%d, %d)", start, end, tc.start, tc.end)
			}
		})
	}
}
