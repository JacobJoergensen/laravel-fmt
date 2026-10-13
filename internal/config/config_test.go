package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestStylesheetAcceptsStringOrList(t *testing.T) {
	cases := map[string]struct {
		json string
		want []string
	}{
		"single string": {`{"tailwind": {"stylesheet": "a.css"}}`, []string{"a.css"}},
		"list":          {`{"tailwind": {"stylesheet": ["a.css", "b.css"]}}`, []string{"a.css", "b.css"}},
		"empty list":    {`{"tailwind": {"stylesheet": []}}`, nil},
		"empty string":  {`{"tailwind": {"stylesheet": ""}}`, nil},
		"null":          {`{"tailwind": {"stylesheet": null}}`, nil},
		"absent":        {`{}`, nil},
		"blank entry":   {`{"tailwind": {"stylesheet": ["", "b.css"]}}`, []string{"b.css"}},
		"all blank":     {`{"tailwind": {"stylesheet": ["", ""]}}`, nil},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, FileName)

			if err := os.WriteFile(path, []byte(tc.json), 0o600); err != nil {
				t.Fatal(err)
			}

			loaded, err := LoadFile(Default(), path)
			if err != nil {
				t.Fatalf("LoadFile: %v", err)
			}

			if !slices.Equal(loaded.File.TailwindCS.Stylesheet, tc.want) {
				t.Fatalf("Stylesheet = %q, want %q", loaded.File.TailwindCS.Stylesheet, tc.want)
			}
		})
	}
}

func TestStylesheetMarshalsCanonically(t *testing.T) {
	cases := map[string]struct {
		in   StringOrSlice
		want string
	}{
		"unset":  {nil, `""`},
		"single": {StringOrSlice{"a.css"}, `"a.css"`},
		"many":   {StringOrSlice{"a.css", "b.css"}, `["a.css","b.css"]`},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := json.Marshal(tc.in)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			if string(got) != tc.want {
				t.Fatalf("Marshal = %s, want %s", got, tc.want)
			}
		})
	}
}
