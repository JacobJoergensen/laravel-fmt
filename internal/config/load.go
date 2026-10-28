package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
)

// Loaded is a parsed config plus the keys the user wrote, which no detected
// setting may override.
type Loaded struct {
	File     File
	UserKeys map[string]bool

	// IgnoreAnchor is the directory the ignore patterns were read from, so the
	// directory they match paths against.
	IgnoreAnchor string
}

// LoadFrom reads workDir's laravel-fmt.json onto base; a missing file leaves
// base as is.
func LoadFrom(base *File, workDir string) (Loaded, error) {
	loaded, err := LoadFile(base, filepath.Join(workDir, FileName))

	if err != nil && errors.Is(err, os.ErrNotExist) {
		if legacyExists(workDir) {
			return Loaded{}, fmt.Errorf("%s is no longer supported; run `laravel-fmt init` to generate %s, then delete %s", LegacyFileName, FileName, LegacyFileName)
		}

		return Loaded{File: *base, UserKeys: map[string]bool{}}, nil
	}

	return loaded, err
}

func legacyExists(workDir string) bool {
	_, err := os.Stat(filepath.Join(workDir, LegacyFileName))

	return err == nil
}

// LoadFile reads the JSON config at path onto base; a missing file is a wrapped
// os.ErrNotExist.
func LoadFile(base *File, path string) (Loaded, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Loaded{}, fmt.Errorf("read %s: %w", path, err)
	}

	// Runs before the typed decode: json.Unmarshal reports every malformed file
	// (truncated, empty, trailing data) as a positioned SyntaxError, where the
	// Decoder gives a bare "unexpected EOF" and accepts trailing data.
	keys, err := extractTopLevelKeys(raw)
	if err != nil {
		return Loaded{}, decodeError(path, raw, err)
	}

	if err := checkRenamedKeys(raw, path); err != nil {
		return Loaded{}, err
	}

	cfg := *base

	// The copy shares base's backing array, which decoding a JSON array
	// overwrites, so the built-ins must be cloned before the decode.
	builtInIgnore := slices.Clone(base.Ignore)

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&cfg); err != nil {
		return Loaded{}, decodeError(path, raw, err)
	}

	if err := cfg.validate(); err != nil {
		return Loaded{}, fmt.Errorf("%s: %w", path, err)
	}

	if keys["ignore"] {
		cfg.Ignore = mergeIgnore(builtInIgnore, cfg.Ignore)
	}

	return Loaded{File: cfg, UserKeys: keys}, nil
}

const jsonTruncatedMessage = "unexpected end of JSON input"

// ParseJSONError reports a failed decode of the JSON at path, locating a syntax
// error as path:line:column in bytes.
func ParseJSONError(path string, raw []byte, err error) error {
	syntaxErr, ok := errors.AsType[*json.SyntaxError](err)
	if !ok {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	// Offset counts the offending byte itself, except for a truncated file,
	// where it is len(raw) too and the error sits past the last byte. Only the
	// message tells the two apart (`{"a":1}}` ends on an offending byte).
	before := raw[:max(syntaxErr.Offset-1, 0)]
	if syntaxErr.Error() == jsonTruncatedMessage {
		before = raw
	}

	line := bytes.Count(before, []byte("\n")) + 1
	column := len(before) - (bytes.LastIndexByte(before, '\n') + 1) + 1

	return fmt.Errorf("%s:%d:%d: %w", path, line, column, syntaxErr)
}

func decodeError(path string, raw []byte, err error) error {
	typeErr, ok := errors.AsType[*json.UnmarshalTypeError](err)
	if !ok {
		return ParseJSONError(path, raw, err)
	}

	key := typeErr.Field
	if key == "" {
		key = "the config"
	}

	return fmt.Errorf("%s: %s must be %s, not %s", path, key, describeGoType(typeErr.Type), describeJSONValue(typeErr.Value))
}

// Decoding a list onto the slice would replace it, so one entry of the user's
// own would drop every built-in. There is no opt-out: a path named on the
// command line already overrides the ignore list.
func mergeIgnore(builtIn, user []string) []string {
	merged := make([]string, 0, len(builtIn)+len(user))
	merged = append(merged, builtIn...)

	for _, pattern := range user {
		if !slices.Contains(merged, pattern) {
			merged = append(merged, pattern)
		}
	}

	return merged
}

func describeGoType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.String {
			return "a list of strings"
		}

		return "a list"
	case reflect.Bool:
		return "true or false"
	case reflect.String:
		return "a string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "a whole number"
	case reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.Struct, reflect.Map:
		return "an object"
	default:
		return t.String()
	}
}

// encoding/json reports a fraction in an integer field as "number 1.5".
func describeJSONValue(value string) string {
	switch value {
	case "bool":
		return "a boolean"
	case "string":
		return "a string"
	case "array":
		return "a list"
	case "object":
		return "an object"
	case "number":
		return "a number"
	}

	if number, ok := strings.CutPrefix(value, "number "); ok {
		return number
	}

	return value
}

// checkRenamedKeys turns a removed-but-recognized key into a migration hint
// instead of the generic unknown-field error DisallowUnknownFields would give.
func checkRenamedKeys(raw []byte, path string) error {
	var probe struct {
		PHP map[string]json.RawMessage `json:"php"`
	}

	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil //nolint:nilerr // a malformed file is reported by the main decode, not here
	}

	if _, ok := probe.PHP["blankLineBeforeReturn"]; ok {
		return fmt.Errorf("%s: php.blankLineBeforeReturn was replaced by php.blankLineBeforeStatements, a list (e.g. [%q, %q])", path, "continue", "return")
	}

	return nil
}

// A second, generic decode is needed because the typed struct can't tell an
// absent key from an explicit zero value.
func extractTopLevelKeys(raw []byte) (map[string]bool, error) {
	var generic map[string]json.RawMessage

	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}

	keys := make(map[string]bool, len(generic))

	for k := range generic {
		keys[k] = true
	}

	// The per-file Prettier overlay adopts proseWrap, which lives under
	// "markdown"; record it so an override cannot clobber a value the user set.
	if rawMarkdown, ok := generic["markdown"]; ok {
		var markdown map[string]json.RawMessage

		if err := json.Unmarshal(rawMarkdown, &markdown); err == nil {
			if _, ok := markdown["proseWrap"]; ok {
				keys["markdown.proseWrap"] = true
			}
		}
	}

	return keys, nil
}
