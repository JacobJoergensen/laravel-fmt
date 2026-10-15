package config

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	bladeparser "github.com/JacobJoergensen/laravel-fmt/internal/parser/blade"
)

type schemaNode struct {
	Properties map[string]schemaNode `json:"properties"`
}

// schema/v1.json is hand-written, so nothing else keeps it in step with the
// structs.
func TestSchemaMatchesStructs(t *testing.T) {
	root := readSchema(t)

	cases := []struct {
		label  string
		typ    reflect.Type
		schema map[string]schemaNode
	}{
		{"root", reflect.TypeFor[File](), root.Properties},
		{"php", reflect.TypeFor[PHP](), root.Properties["php"].Properties},
		{"blade", reflect.TypeFor[Blade](), root.Properties["blade"].Properties},
		{"markdown", reflect.TypeFor[Markdown](), root.Properties["markdown"].Properties},
		{"tailwind", reflect.TypeFor[TailwindCS](), root.Properties["tailwind"].Properties},
	}

	for _, tc := range cases {
		want := structJSONFields(tc.typ)
		got := slices.Sorted(maps.Keys(tc.schema))

		want = slices.DeleteFunc(want, func(s string) bool { return s == "$schema" })
		got = slices.DeleteFunc(got, func(s string) bool { return s == "$schema" })

		if !slices.Equal(want, got) {
			t.Errorf("%s properties drift:\n  structs: %v\n  schema:  %v", tc.label, want, got)
		}
	}
}

func TestSchemaBlankLineDirectivesMatchParser(t *testing.T) {
	var schema struct {
		Properties struct {
			Blade struct {
				Properties struct {
					BlankLineBeforeDirectives struct {
						Items struct {
							AnyOf []struct {
								Enum []string `json:"enum"`
							} `json:"anyOf"`
						} `json:"items"`
					} `json:"blankLineBeforeDirectives"`
				} `json:"properties"`
			} `json:"blade"`
		} `json:"properties"`
	}

	if err := json.Unmarshal(schemaBytes(t), &schema); err != nil {
		t.Fatalf("parse schema: %v", err)
	}

	var got []string

	for _, alt := range schema.Properties.Blade.Properties.BlankLineBeforeDirectives.Items.AnyOf {
		got = append(got, alt.Enum...)
	}

	want := append(bladeparser.BuiltinOpeners(), BladeDirectivePHP)

	if !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))) {
		t.Errorf("blankLineBeforeDirectives enum drift:\n  parser: %v\n  schema: %v", want, got)
	}
}

func structJSONFields(t reflect.Type) []string {
	var names []string

	for field := range t.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")

		if name == "" || name == "-" {
			continue
		}

		names = append(names, name)
	}

	slices.Sort(names)

	return names
}

func readSchema(t *testing.T) schemaNode {
	t.Helper()

	var root schemaNode

	if err := json.Unmarshal(schemaBytes(t), &root); err != nil {
		t.Fatalf("parse schema: %v", err)
	}

	return root
}

func schemaBytes(t *testing.T) []byte {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above test directory")
		}

		dir = parent
	}

	raw, err := os.ReadFile(filepath.Join(dir, "schema", "v1.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}

	return raw
}
