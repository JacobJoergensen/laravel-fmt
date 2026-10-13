package php

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

func TestRawDirectiveArgRehomable(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{"single-line array", "[Cat::A, Cat::B] as $x", true},
		{"multi-line array, single-line strings", "[\n    Cat::A => 'a',\n    Cat::B => 'b',\n] as $k => $v", true},
		{"for clauses", "\n    $i = 0;\n    $i < 10;\n    $i++\n", true},
		{"multi-line single-quoted string", "[\n    'sql' => 'SELECT *\n    FROM t',\n] as $k => $v", false},
		{"multi-line double-quoted string", "[\n    \"a\nb\",\n] as $x", false},
		{"heredoc (always multi-line)", "[<<<EOT\nbody\nEOT] as $x", false},
		{"newline only between elements, not in a string", "['a'\n=>\n'b'] as $x", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RawDirectiveArgRehomable([]byte(tc.raw)); got != tc.want {
				t.Fatalf("RawDirectiveArgRehomable(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestEndsInLineComment(t *testing.T) {
	cases := map[string]struct {
		src  string
		want bool
	}{
		"a hash in a double-quoted string": {`"#" `, false},
		"a hash in a single-quoted string": {`'#'`, false},
		"an attribute opener":              {`#[ `, false},
		"a slash comment":                  {`$i++ // c`, true},
		"a hash comment":                   {`$a as $b # c`, true},
		"a hash inside a block comment":    {`$a as $b /* # */`, false},
		"a comment the line then closes":   {"$a // c\n$b", false},
		"a comment ended by a lone CR":     {"$a // c\r", false},
		"trivia only":                      {"// c", true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := EndsInLineComment([]byte(tc.src)); got != tc.want {
				t.Fatalf("EndsInLineComment(%q) = %v, want %v", tc.src, got, tc.want)
			}
		})
	}
}

func TestDirectiveArgWrappedKeepsACommentItCannotPlace(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"comment before the call's parens", "name: run#\n()", "name: run#\n()"},
		{"no comment", "name: run()", "name: run()"},
		{"comment before the argument too", "/*a*/ name: run#\n()", "/*a*/ name: run#\n()"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args, tokens, errs := parser.ParseCallArgs([]byte(tc.src))
			if len(errs) > 0 || len(args) != 1 {
				t.Fatalf("parse %q: %v", tc.src, errs)
			}

			if got := string(BuildDirectiveArgWrapped([]byte(tc.src), tokens, args[0], config.Default(), 4)); got != tc.want {
				t.Fatalf("BuildDirectiveArgWrapped(%q) = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

func TestStmtsLayoutIsStableAtEveryIndent(t *testing.T) {
	src := []byte("$rows()->filter($total % $count() % $limit());")

	for indent := range 101 {
		stmts, tokens, _ := parser.ParseStatements(src)
		first := BuildStmts(src, tokens, stmts, config.Default(), indent)

		reStmts, reTokens, _ := parser.ParseStatements(first)
		if second := BuildStmts(first, reTokens, reStmts, config.Default(), indent); !bytes.Equal(second, first) {
			t.Fatalf("indent %d:\n%s\nthen:\n%s", indent, first, second)
		}
	}
}

func TestDirectiveArgWrappedLayoutDependsOnlyOnTheWidthLeft(t *testing.T) {
	src := []byte("$rows()->filter($total % $count() % $limit())")
	args, tokens, _ := parser.ParseCallArgs(src)

	for indent := range 101 {
		rest := indent % config.Default().TabWidth

		narrower := config.Default()
		narrower.PrintWidth -= rest

		got := BuildDirectiveArgWrapped(src, tokens, args[0], config.Default(), indent)
		want := BuildDirectiveArgWrapped(src, tokens, args[0], narrower, indent-rest)

		if !bytes.Equal(got, want) {
			t.Fatalf("indent %d:\n%s\nwant, with the same width left at indent %d:\n%s", indent, got, indent-rest, want)
		}
	}
}
