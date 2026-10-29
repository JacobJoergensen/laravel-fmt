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

type schemaNode struct {
	Properties map[string]schemaNode `json:"properties"`
	Items      *schemaNode           `json:"items"`
	Enum       []string              `json:"enum"`
}

// Append each release's new keys and enum values; never remove one.
var releasedSchemaEntries = []string{
	"$schema", "ignore", "printWidth", "indentWidth",
	"indentStyle", "indentStyle=space", "indentStyle=tab",
	"endOfLine", "endOfLine=lf", "endOfLine=crlf", "endOfLine=auto",
	"quoteStyle", "quoteStyle=auto", "quoteStyle=single", "quoteStyle=double",

	"php", "php.removeTrailingCloseTag", "php.sortUses", "php.expandUseGroups",
	"php.concatSpace", "php.notOperatorSpace", "php.closureFnSpace",
	"php.alignFatArrow", "php.shortArray",
	"php.functionBraceStyle", "php.functionBraceStyle=same-line",
	"php.functionBraceStyle=next-line", "php.functionBraceStyle=always-next-line",
	"php.methodBraceStyle", "php.methodBraceStyle=same-line",
	"php.methodBraceStyle=next-line", "php.methodBraceStyle=always-next-line",
	"php.classBraceStyle", "php.classBraceStyle=same-line",
	"php.classBraceStyle=next-line", "php.classBraceStyle=always-next-line",
	"php.anonymousClassBraceStyle", "php.anonymousClassBraceStyle=same-line",
	"php.anonymousClassBraceStyle=next-line", "php.anonymousClassBraceStyle=always-next-line",
	"php.controlBraceStyle", "php.controlBraceStyle=same-line",
	"php.controlBraceStyle=next-line", "php.controlBraceStyle=always-next-line",
	"php.blankLineBeforeStatements",
	"php.blankLineBeforeStatements[]=break", "php.blankLineBeforeStatements[]=continue",
	"php.blankLineBeforeStatements[]=declare", "php.blankLineBeforeStatements[]=do",
	"php.blankLineBeforeStatements[]=for", "php.blankLineBeforeStatements[]=foreach",
	"php.blankLineBeforeStatements[]=goto", "php.blankLineBeforeStatements[]=if",
	"php.blankLineBeforeStatements[]=include", "php.blankLineBeforeStatements[]=include_once",
	"php.blankLineBeforeStatements[]=require", "php.blankLineBeforeStatements[]=require_once",
	"php.blankLineBeforeStatements[]=return", "php.blankLineBeforeStatements[]=switch",
	"php.blankLineBeforeStatements[]=throw", "php.blankLineBeforeStatements[]=try",
	"php.blankLineBeforeStatements[]=while", "php.blankLineBeforeStatements[]=yield",
	"php.blankLineBeforeStatements[]=yield_from",
	"php.heredocIndentation", "php.heredocIndentation=start_plus_one",
	"php.heredocIndentation=same_as_start", "php.heredocIndentation=none",
	"php.importOrder", "php.importOrder[]=class", "php.importOrder[]=function",
	"php.importOrder[]=const",

	"blade", "blade.blockDirectives", "blade.blockDirectives[].open",
	"blade.blockDirectives[].close", "blade.blockDirectives[].branches",
	"blade.blankLineBeforeDirectives",

	"markdown", "markdown.proseWrap", "markdown.proseWrap=always",
	"markdown.proseWrap=never", "markdown.proseWrap=preserve",

	"tailwind", "tailwind.stylesheet",
}

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

// The generated $schema URL tracks v1.json on main, so a config written for
// any release must keep validating.
func TestSchemaKeepsReleasedEntries(t *testing.T) {
	current := schemaEntries(readSchema(t), "")

	var missing []string

	for _, entry := range releasedSchemaEntries {
		if !slices.Contains(current, entry) {
			missing = append(missing, entry)
		}
	}

	if len(missing) > 0 {
		t.Errorf("schema/v1.json dropped released entries %v; add schema/v2.json instead", missing)
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

// anyOf and oneOf are skipped: the one enum under them, in
// blankLineBeforeDirectives, only lists names, since any name validates.
func schemaEntries(node schemaNode, path string) []string {
	var entries []string

	for _, value := range node.Enum {
		entries = append(entries, path+"="+value)
	}

	if node.Items != nil {
		entries = append(entries, schemaEntries(*node.Items, path+"[]")...)
	}

	for name, child := range node.Properties {
		childPath := name
		if path != "" {
			childPath = path + "." + name
		}

		entries = append(entries, childPath)
		entries = append(entries, schemaEntries(child, childPath)...)
	}

	return entries
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

	raw, err := os.ReadFile(filepath.Join("..", "..", "schema", "v1.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}

	return raw
}
