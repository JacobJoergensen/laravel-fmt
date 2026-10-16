package php

import "testing"

func TestCloseTagOffsetFollowsPHPRules(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"plain close", ` echo 1; ?>`, 9},
		{"inside a double-quoted string", ` echo "?> <b>"; ?>`, 16},
		{"inside a single-quoted string", ` echo 'a ?> b'; ?>`, 16},
		{"inside a heredoc", " echo <<<EOT\n?>\nEOT;\n?>", 21},
		{"inside a nowdoc", " echo <<<'EOT'\n?>\nEOT;\n?>", 23},
		{"inside a block comment", ` /* ?> */ echo 1; ?>`, 18},
		{"ends a line comment", ` // note ?> <b>`, 9},
		{"ends a hash comment", ` # note ?>`, 8},
		{"after a method named __halt_compiler", ` $a->__halt_compiler(); ?>`, 24},
		{"none", ` echo "?>`, -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CloseTagOffset([]byte(tc.src)); got != tc.want {
				t.Fatalf("CloseTagOffset(%q) = %d, want %d", tc.src, got, tc.want)
			}
		})
	}
}

func TestYieldFromLexesAsOneToken(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want Kind
	}{
		{"yield from", "<?php yield from $g;", KindYieldFrom},
		{"YIELD FROM uppercase", "<?php YIELD FROM $g;", KindYieldFrom},
		{"yield alone", "<?php yield $v;", KindYield},
		{"yield as a member name", "<?php $a->yield;", KindYield},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tokens, _ := Lex([]byte(tc.src))

			found := false

			for _, tok := range tokens {
				if tok.Kind == tc.want {
					found = true

					break
				}
			}

			if !found {
				t.Fatalf("lexing %q produced no %v token: %v", tc.src, tc.want, kinds(tokens))
			}
		})
	}
}

func kinds(tokens []Token) []Kind {
	out := make([]Kind, len(tokens))
	for i, tok := range tokens {
		out[i] = tok.Kind
	}

	return out
}

func TestEnumBeforeACloseTagIsAnIdentifier(t *testing.T) {
	tokens, _ := Lex([]byte("<?php enum // c ?>\nFoo {}"))

	for _, tok := range tokens {
		if string(tok.Text) == "enum" && tok.Kind != KindIdent {
			t.Fatalf("enum lexed as %v, want KindIdent: the comment ends at ?>", tok.Kind)
		}
	}
}

// Every literal here passes `php -l`.
func TestDoubleQuotedStringKeepsInterpolatedCodeWhole(t *testing.T) {
	cases := []struct {
		name    string
		literal string
	}{
		{"string argument", `"{$a->f("x")}"`},
		{"dollar brace", `"${a["k"]}"`},
		{"brace inside a nested string", `"{$a["}"]}"`},
		{"quote inside a single-quoted key", `"{$a['}"']}"`},
		{"quote inside a block comment", `"{$a /* " */ }"`},
		{"quote inside a line comment", "\"{$a // \"\n}\""},
		{"quote inside a hash comment", "\"{$a # \"\n}\""},
		{"attribute, not a hash comment", `"{$f(#[A] fn () => "}")}"`},
		{"closure body", `"{$f(function () { return 1; }, "x")}"`},
		{"interpolation inside a nested string", `"{$a->f("{$b["}"]}")}"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tokens, errs := Lex([]byte("<?php $x = " + tc.literal + ";"))
			if len(errs) > 0 {
				t.Fatalf("lex errors: %v", errs)
			}

			for _, tok := range tokens {
				if tok.Kind == KindString {
					if string(tok.Text) != tc.literal {
						t.Fatalf("string token = %q, want %q", tok.Text, tc.literal)
					}

					return
				}
			}

			t.Fatalf("no string token in %v", kinds(tokens))
		})
	}
}

func TestDoubleQuotedStringTreatsASpacedBraceAsText(t *testing.T) {
	tokens, _ := Lex([]byte(`<?php $x = "{ " . "}";`))

	var literals []string

	for _, tok := range tokens {
		if tok.Kind == KindString {
			literals = append(literals, string(tok.Text))
		}
	}

	if len(literals) != 2 || literals[0] != `"{ "` || literals[1] != `"}"` {
		t.Fatalf("strings = %q, want two: %q and %q", literals, `"{ "`, `"}"`)
	}
}

func TestDoubleQuotedStringWithAnUnclosedInterpolationIsAnError(t *testing.T) {
	_, errs := Lex([]byte(`<?php $x = "{$a";`))

	if len(errs) == 0 {
		t.Fatal("no error for an unclosed {$, which PHP rejects")
	}
}
