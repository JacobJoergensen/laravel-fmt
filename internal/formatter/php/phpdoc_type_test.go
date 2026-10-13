package php

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/parser/phpdoc"
)

func TestPrintPHPDocTypeIsCanonical(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"identifier", "int", "int"},
		{"fqn", `\App\Models\User`, `\App\Models\User`},
		{"hyphenated", "non-empty-string", "non-empty-string"},
		{"class string", "class-string", "class-string"},
		{"this", "$this", "$this"},
		{"nullable", "?int", "?int"},
		{"nullable spaced", "? int", "?int"},
		{"union tighten", "int | string | null", "int|string|null"},
		{"intersection tighten", "Countable & Traversable", "Countable&Traversable"},
		{"generic spacing", "array<int,string>", "array<int, string>"},
		{"generic already", "array<int, string>", "array<int, string>"},
		{"generic nested", "array<int, array<string, Foo>>", "array<int, array<string, Foo>>"},
		{"int range", "int<0,100>", "int<0, 100>"},
		{"int range min max", "int<min, max>", "int<min, max>"},
		{"list generic", "list<Foo>", "list<Foo>"},
		{"variance", "Collection<covariant User>", "Collection<covariant User>"},
		{"shape spacing", "array{foo:int,bar:string}", "array{foo: int, bar: string}"},
		{"shape optional", "array{foo?:int}", "array{foo?: int}"},
		{"shape rest", "array{foo: int, ...}", "array{foo: int, ...}"},
		{"shape positional", "array{int,string}", "array{int, string}"},
		{"shape nested", "array{user: array{id: int, name: string}}", "array{user: array{id: int, name: string}}"},
		{"object shape", "object{id:int}", "object{id: int}"},
		{"empty shape", "array{}", "array{}"},
		{"int key shape", "array{0:int,1?:string}", "array{0: int, 1?: string}"},
		{"array suffix", "Foo[]", "Foo[]"},
		{"nested suffix", "int[][]", "int[][]"},
		{"offset access", "Foo[K]", "Foo[K]"},
		{"dnf", "(A&B)|C", "(A&B)|C"},
		{"literal union", "1|2|3", "1|2|3"},
		{"string literal union", "'a'|'b'", "'a'|'b'"},
		{"deep generic of shape", "non-empty-list<array{id:int,name:string}>", "non-empty-list<array{id: int, name: string}>"},
		{"const fetch", "Foo::BAR", "Foo::BAR"},
		{"const fetch self", "self::VALUES", "self::VALUES"},
		{"const fetch wildcard", "Foo::PREFIX_*", "Foo::PREFIX_*"},
		{"const fetch class", `\App\Foo::class`, `\App\Foo::class`},
		{"callable spacing", "callable(int,string):bool", "callable(int, string): bool"},
		{"callable no return", "callable(int)", "callable(int)"},
		{"callable named params", "Closure(string $name,int $age):bool", "Closure(string $name, int $age): bool"},
		{"callable optional", "callable(int=):string", "callable(int=): string"},
		{"callable variadic", "callable(float ...$f):int", "callable(float ...$f): int"},
		{"callable by ref", "callable(int &$x):void", "callable(int &$x): void"},
		{"callable paren return", "callable():(int|null)", "callable(): (int|null)"},
		{"conditional", "T is int?string:bool", "T is int ? string : bool"},
		{"conditional negated", "T is not int ? A : B", "T is not int ? A : B"},
		{"conditional paren param", "($value is int?A:B)", "($value is int ? A : B)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(printPHPDocType(phpdoc.ParseType([]byte(tc.in))))
			if got != tc.want {
				t.Fatalf("%q printed %q, want %q", tc.in, got, tc.want)
			}

			again := string(printPHPDocType(phpdoc.ParseType([]byte(tc.want))))
			if again != tc.want {
				t.Fatalf("not idempotent: %q printed %q", tc.want, again)
			}
		})
	}
}

func TestPrintPHPDocTypeIsStable(t *testing.T) {
	inputs := []string{
		"", " ", "@", "<>", "{}", "((((", "array<<<", "Foo::::", "?|&",
		"callable(", "callable(int,,)", "array{,,}", "int<>", "T is ? :",
		"|||", "...", "[]", "Foo<covariant>", "$", `\\\`, "a-b-c-",
		"array{0:,1:}", "string&", "?", "Foo[][", "()", "T is",
	}

	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			once := printPHPDocType(phpdoc.ParseType([]byte(in)))
			twice := printPHPDocType(phpdoc.ParseType(once))

			if !bytes.Equal(once, twice) {
				t.Fatalf("print-after-parse not stable for %q: %q then %q", in, once, twice)
			}
		})
	}
}
