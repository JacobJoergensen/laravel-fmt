package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLoadFile(t *testing.T) {
	cases := []struct {
		name      string
		json      string
		wantErr   bool
		wantWidth int
	}{
		{
			name:      "clean json overrides base",
			json:      `{"printWidth": 80}`,
			wantWidth: 80,
		},
		{
			name:    "unknown key errors",
			json:    `{"printWith": 80}`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), FileName)

			if err := os.WriteFile(path, []byte(tc.json), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			base := Default()
			got, err := LoadFile(base, path)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("LoadFile = nil error, want error for unknown key")
				}

				if !strings.Contains(err.Error(), path) {
					t.Fatalf("LoadFile error %q does not contain file path %q", err, path)
				}

				return
			}

			if err != nil {
				t.Fatalf("LoadFile returned error: %v", err)
			}

			if got.File.PrintWidth != tc.wantWidth {
				t.Fatalf("PrintWidth = %d, want %d", got.File.PrintWidth, tc.wantWidth)
			}
		})
	}
}

func TestLoadFilePHPStyleOverridesBase(t *testing.T) {
	src := `{"php": {` +
		`"concatSpace": false,` +
		`"classBraceStyle": "same-line",` +
		`"controlBraceStyle": "next-line",` +
		`"importOrder": ["class", "function", "const"]` +
		`}}`

	path := filepath.Join(t.TempDir(), FileName)

	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := LoadFile(Default(), path)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}

	php := got.File.PHP

	if php.ConcatSpace {
		t.Errorf("ConcatSpace = true, want false (json overrides base)")
	}

	if php.ClasslikeBraceStyle != BraceStyleSameLine {
		t.Errorf("ClasslikeBraceStyle = %q, want %q", php.ClasslikeBraceStyle, BraceStyleSameLine)
	}

	if php.ControlBraceStyle != BraceStyleNextLine {
		t.Errorf("ControlBraceStyle = %q, want %q", php.ControlBraceStyle, BraceStyleNextLine)
	}

	if !slices.Equal(php.ImportOrder, []string{ImportKindClass, ImportKindFunction, ImportKindConst}) {
		t.Errorf("ImportOrder = %v, want [class function const]", php.ImportOrder)
	}
}

func TestLoadFilePHPValidation(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"invalid quote style", `{"quoteStyle": "backtick"}`},
		{"invalid brace style", `{"php": {"functionBraceStyle": "curly"}}`},
		{"invalid heredoc mode", `{"php": {"heredocIndentation": "deep"}}`},
		{"invalid import kind", `{"php": {"importOrder": ["class", "traits"]}}`},
		{"invalid blank line statement", `{"php": {"blankLineBeforeStatements": ["return", "phpdoc"]}}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), FileName)

			if err := os.WriteFile(path, []byte(tc.json), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			if _, err := LoadFile(Default(), path); err == nil {
				t.Fatalf("LoadFile = nil error, want validation error")
			}
		})
	}
}

func TestLoadFileBladeBlockDirectives(t *testing.T) {
	invalid := []struct {
		name string
		json string
	}{
		{"not a directive name", `{"blade": {"blockDirectives": [{"open": "my-block"}]}}`},
		{"redeclares a built-in opener", `{"blade": {"blockDirectives": [{"open": "if"}]}}`},
		{"closes with a built-in closer", `{"blade": {"blockDirectives": [{"open": "feature", "close": "endif"}]}}`},
		{"declared twice", `{"blade": {"blockDirectives": [{"open": "feature"}, {"open": "feature"}]}}`},
		{"closes itself", `{"blade": {"blockDirectives": [{"open": "feature", "close": "feature"}]}}`},
	}

	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), FileName)

			if err := os.WriteFile(path, []byte(tc.json), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			if _, err := LoadFile(Default(), path); err == nil {
				t.Fatalf("LoadFile = nil error, want validation error")
			}
		})
	}

	t.Run("close defaults to end plus the opener", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), FileName)

		if err := os.WriteFile(path, []byte(`{"blade": {"blockDirectives": [{"open": "feature", "branches": ["else"]}]}}`), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}

		loaded, err := LoadFile(Default(), path)
		if err != nil {
			t.Fatalf("LoadFile: %v", err)
		}

		if got := loaded.File.Blade.BlockDirectives[0].Closer(); got != "endfeature" {
			t.Fatalf("Closer() = %q, want endfeature", got)
		}
	})
}

func TestLoadFileValidationErrorNamesPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my-config.json")

	if err := os.WriteFile(path, []byte(`{"printWidth": 0}`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFile(Default(), path)

	if err == nil {
		t.Fatalf("LoadFile = nil error, want validation error")
	}

	if !strings.Contains(err.Error(), path) {
		t.Fatalf("LoadFile error %q does not name the config path %q", err, path)
	}
}

func TestLoadFileAcceptsAlwaysNextLineBrace(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)

	if err := os.WriteFile(path, []byte(`{"php": {"functionBraceStyle": "always-next-line"}}`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := LoadFile(Default(), path)
	if err != nil {
		t.Fatalf("LoadFile = %v, want nil (always-next-line is valid)", err)
	}

	if got.File.PHP.FunctionBraceStyle != BraceStyleAlwaysNextLine {
		t.Errorf("FunctionBraceStyle = %q, want %q", got.File.PHP.FunctionBraceStyle, BraceStyleAlwaysNextLine)
	}
}

func TestLoadFileRejectsRenamedBlankLineKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)

	if err := os.WriteFile(path, []byte(`{"php": {"blankLineBeforeReturn": true}}`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadFile(Default(), path)

	if err == nil || !strings.Contains(err.Error(), "blankLineBeforeStatements") {
		t.Fatalf("LoadFile err = %v, want a migration hint naming blankLineBeforeStatements", err)
	}
}

func TestLoadFileWrongValueTypeNamesKeyAndExpectedShape(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{"boolean for a statement list", `{"php": {"blankLineBeforeStatements": true}}`, "php.blankLineBeforeStatements must be a list of strings, not a boolean"},
		{"string for a width", `{"printWidth": "120"}`, "printWidth must be a whole number, not a string"},
		{"fraction for a width", `{"printWidth": 1.5}`, "printWidth must be a whole number, not 1.5"},
		{"boolean for a string-or-list", `{"tailwind": {"stylesheet": true}}`, "expected a string or a list of strings, not a boolean"},
		{"list for the whole file", `[]`, "the config must be an object, not a list"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), FileName)

			if err := os.WriteFile(path, []byte(tc.json), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			_, err := LoadFile(Default(), path)

			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "Go struct") {
				t.Fatalf("LoadFile err = %v, want it to contain %q and no Go type names", err, tc.want)
			}
		})
	}
}

func TestLoadFileSyntaxErrorNamesLineAndColumn(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{"trailing comma on a later line", "{\n  \"printWidth\": 120,\n}\n", ":3:1: invalid character '}'"},
		{"offending last byte", `{"printWidth": 120}}`, ":1:20: invalid character '}' after top-level value"},
		{"truncated after a key", "{\n  \"printWidth\": ", ":2:17: unexpected end of JSON input"},
		{"empty file", "", ":1:1: unexpected end of JSON input"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), FileName)

			if err := os.WriteFile(path, []byte(tc.json), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			_, err := LoadFile(Default(), path)

			if err == nil || !strings.Contains(err.Error(), FileName+tc.want) {
				t.Fatalf("LoadFile err = %v, want it to contain %q", err, FileName+tc.want)
			}
		})
	}
}

func TestUserIgnoreExtendsTheBuiltInList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	if err := os.WriteFile(path, []byte(`{"ignore": ["public/build/**"]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadFile(Default(), path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	for _, want := range Default().Ignore {
		if !slices.Contains(loaded.File.Ignore, want) {
			t.Errorf("built-in ignore %q was discarded; got %v", want, loaded.File.Ignore)
		}
	}

	if !slices.Contains(loaded.File.Ignore, "public/build/**") {
		t.Errorf("the user's own entry is missing; got %v", loaded.File.Ignore)
	}
}

func TestUserIgnoreDoesNotDuplicateABuiltIn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	if err := os.WriteFile(path, []byte(`{"ignore": ["vendor/", "public/build/**"]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadFile(Default(), path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	count := 0

	for _, p := range loaded.File.Ignore {
		if p == "vendor/" {
			count++
		}
	}

	if count != 1 {
		t.Errorf("vendor/ appears %d times, want 1; got %v", count, loaded.File.Ignore)
	}
}
