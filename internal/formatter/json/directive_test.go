package json

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestFormatKeepsAnIgnoredItemAsWritten(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"member",
			"{\n  // laravel-fmt-ignore\n  \"grid\": [1,2,   3],\n  \"other\": [1,2,   3]\n}\n",
			"{\n  // laravel-fmt-ignore\n  \"grid\": [1,2,   3],\n  \"other\": [1, 2, 3]\n}\n",
		},
		{
			"element",
			"[\n  /* laravel-fmt-ignore */\n  {\"a\":1,   \"b\":2},\n  {\"a\":1,   \"b\":2}\n]\n",
			"[\n  /* laravel-fmt-ignore */\n  {\"a\":1,   \"b\":2},\n  { \"a\": 1, \"b\": 2 }\n]\n",
		},
		{
			"root",
			"// laravel-fmt-ignore\n{\"a\":1,   \"b\":[1,2]}\n",
			"// laravel-fmt-ignore\n{\"a\":1,   \"b\":[1,2]}\n",
		},
		{
			"member followed by a same-line comment",
			"{\n  // laravel-fmt-ignore\n  \"grid\": [1,2,   3], // note\n  \"other\": 1\n}\n",
			"{\n  // laravel-fmt-ignore\n  \"grid\": [1,2,   3], // note\n  \"other\": 1\n}\n",
		},
		{
			"directive lookalike",
			"{\n  // laravel-fmt-ignore-not-really\n  \"grid\": [1,2,   3]\n}\n",
			"{\n  // laravel-fmt-ignore-not-really\n  \"grid\": [1, 2, 3]\n}\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustFormat(t, "tsconfig.json", []byte(tc.src), config.Default())
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}

			if twice := mustFormat(t, "tsconfig.json", got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent:\n=== once ===\n%q\n=== twice ===\n%q", got, twice)
			}
		})
	}
}
