package php

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestNormalizeDocContent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"param shape", " @param array{foo:int,bar:string} $x", " @param array{foo: int, bar: string} $x"},
		{"return generic", " @return Foo<int,Bar>", " @return Foo<int, Bar>"},
		{"var generic", " @var array<int,string> $x", " @var array<int, string> $x"},
		{"phpstan param", " @phpstan-param list<Foo> $items", " @phpstan-param list<Foo> $items"},
		{"description and alignment preserved", " @param  array<int,Foo>  $items   The list.", " @param  array<int, Foo>  $items   The list."},
		{"already canonical", " @param array{foo: int} $x", " @param array{foo: int} $x"},
		{"return void", " @return void", " @return void"},
		{"fqn throws", ` @throws \App\Exception`, ` @throws \App\Exception`},
		{"callable normalized", " @param callable(int,string):bool $cb", " @param callable(int, string): bool $cb"},
		{"const fetch canonical", " @return self::VALUES", " @return self::VALUES"},
		{"conditional normalized", " @return ($x is int?string:bool)", " @return ($x is int ? string : bool)"},
		{"method return type", " @method static Collection<int,User> users()", " @method static Collection<int, User> users()"},
		{"method name only unchanged", " @method users()", " @method users()"},
		{"phpstan-type alias", " @phpstan-type UserData array{id:int,name:string}", " @phpstan-type UserData array{id: int, name: string}"},
		{"psalm-type alias with equals", " @psalm-type Row = array{a:int,b:string}", " @psalm-type Row = array{a: int, b: string}"},
		{"type alias canonical", " @phpstan-type Id int", " @phpstan-type Id int"},
		{"type alias no name unchanged", " @phpstan-type", " @phpstan-type"},
		{"type alias unterminated bails", " @phpstan-type Big array{", " @phpstan-type Big array{"},
		{"template bound", " @template T of Collection<int,User>", " @template T of Collection<int, User>"},
		{"template covariant bound", " @template-covariant T of array<int,Foo>", " @template-covariant T of array<int, Foo>"},
		{"template as bound", " @template T as array<int,string>", " @template T as array<int, string>"},
		{"template bound and default", " @template T of Foo = array<int,Bar>", " @template T of Foo = array<int, Bar>"},
		{"template default only", " @template T = array<int,string>", " @template T = array<int, string>"},
		{"template fqn bound unchanged", ` @template T of \App\Model`, ` @template T of \App\Model`},
		{"template bare unchanged", " @template T", " @template T"},
		{"template no name unchanged", " @template", " @template"},
		{"assert type", " @phpstan-assert array<int,Foo> $x", " @phpstan-assert array<int, Foo> $x"},
		{"assert if-true", " @phpstan-assert-if-true Foo<int,Bar> $x", " @phpstan-assert-if-true Foo<int, Bar> $x"},
		{"assert negated preserves bang", " @phpstan-assert !array<int,Foo> $x", " @phpstan-assert !array<int, Foo> $x"},
		{"psalm assert canonical", " @psalm-assert non-empty-string $x", " @psalm-assert non-empty-string $x"},
		{"assert if-false null unchanged", " @phpstan-assert-if-false null $x", " @phpstan-assert-if-false null $x"},
		{"unterminated shape bails", " @param array{", " @param array{"},
		{"not a type tag", " @author Jane <jane@example.com>", " @author Jane <jane@example.com>"},
		{"no tag is prose", " Just a sentence about @param.", " Just a sentence about @param."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(normalizeDocContent([]byte(tc.in)))
			if got != tc.want {
				t.Fatalf("normalizeDocContent(%q) = %q, want %q", tc.in, got, tc.want)
			}

			again := string(normalizeDocContent([]byte(got)))
			if again != got {
				t.Fatalf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

func TestDocCommentTypeNormalization(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "multiline docblock normalizes param and return types",
			input: "<?php\n\nfunction foo()\n{\n/**\n* @param array{foo:int,bar:string} $x\n* @return Foo<int,Bar>\n*/\n    return;\n}\n",
			want:  "<?php\n\nfunction foo()\n{\n    /**\n     * @param array{foo: int, bar: string} $x\n     * @return Foo<int, Bar>\n     */\n    return;\n}\n",
		},
		{
			name:  "single-line docblock normalizes the var type",
			input: "<?php\n\n/** @var array<int,string> $x */\nfunction foo() {}\n",
			want:  "<?php\n\n/** @var array<int, string> $x */\nfunction foo() {}\n",
		},
		{
			name:  "description spacing is preserved while the type is normalized",
			input: "<?php\n\nfunction foo()\n{\n/**\n* @param  array<int,Foo>  $items   The list.\n*/\n    return;\n}\n",
			want:  "<?php\n\nfunction foo()\n{\n    /**\n     * @param  array<int, Foo>  $items   The list.\n     */\n    return;\n}\n",
		},
		{
			name:  "callable signature type is normalized",
			input: "<?php\n\nfunction foo()\n{\n/**\n* @param callable(int,string):void $cb\n*/\n    return;\n}\n",
			want:  "<?php\n\nfunction foo()\n{\n    /**\n     * @param callable(int, string): void $cb\n     */\n    return;\n}\n",
		},
		{
			name:  "method return type is normalized, signature kept verbatim",
			input: "<?php\n\nfunction foo()\n{\n/**\n* @method static Collection<int,User> users(int $id = 0)\n*/\n    return;\n}\n",
			want:  "<?php\n\nfunction foo()\n{\n    /**\n     * @method static Collection<int, User> users(int $id = 0)\n     */\n    return;\n}\n",
		},
		{
			name:  "phpstan-type alias body is normalized",
			input: "<?php\n\nfunction foo()\n{\n/**\n* @phpstan-type UserData array{id:int,name:string}\n*/\n    return;\n}\n",
			want:  "<?php\n\nfunction foo()\n{\n    /**\n     * @phpstan-type UserData array{id: int, name: string}\n     */\n    return;\n}\n",
		},
		{
			name:  "template bound and default are normalized",
			input: "<?php\n\nfunction foo()\n{\n/**\n* @template T of array<int,Foo> = array<int,Bar>\n*/\n    return;\n}\n",
			want:  "<?php\n\nfunction foo()\n{\n    /**\n     * @template T of array<int, Foo> = array<int, Bar>\n     */\n    return;\n}\n",
		},
		{
			name:  "assert type is normalized, negation and variable kept",
			input: "<?php\n\nfunction foo()\n{\n/**\n* @phpstan-assert-if-true !array<int,Foo> $x\n*/\n    return;\n}\n",
			want:  "<?php\n\nfunction foo()\n{\n    /**\n     * @phpstan-assert-if-true !array<int, Foo> $x\n     */\n    return;\n}\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tc.input), config.Default())

			if string(got) != tc.want {
				t.Fatalf("output mismatch.\n=== got ===\n%s\n=== want ===\n%s", got, tc.want)
			}

			twice := mustFormat(t, got, config.Default())

			if !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}
