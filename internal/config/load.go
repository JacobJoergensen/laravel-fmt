package config

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
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

	keys, err := extractTopLevelKeys(raw)
	if err != nil {
		return Loaded{}, ParseJSONError(path, raw, err)
	}

	if err := checkRenamedKeys(raw, path); err != nil {
		return Loaded{}, err
	}

	cfg := *base

	// The copy shares base's backing array, which decoding a JSON array
	// overwrites, so the built-ins must be cloned before the decode.
	builtInIgnore := slices.Clone(base.Ignore)

	if err := json.Unmarshal(raw, &cfg, json.RejectUnknownMembers(true), rejectNull); err != nil {
		return Loaded{}, ParseJSONError(path, raw, err)
	}

	if err := cfg.validate(); err != nil {
		return Loaded{}, fmt.Errorf("%s: %w", path, err)
	}

	if keys["ignore"] {
		cfg.Ignore = mergeIgnore(builtInIgnore, cfg.Ignore)
	}

	return Loaded{File: cfg, UserKeys: keys}, nil
}

// The schema never allows null, and encoding/json would decode it as the
// zero value: `"sortUses": null` would switch sorting off without a word.
var rejectNull = json.WithUnmarshalers(json.UnmarshalFromFunc(func(dec *jsontext.Decoder, target any) error {
	if dec.PeekKind() != jsontext.KindNull {
		return errors.ErrUnsupported
	}

	return &json.SemanticError{JSONKind: jsontext.KindNull, GoType: reflect.TypeOf(target).Elem()}
}))

// ParseJSONError reports a failed decode of the JSON at path: a value of the
// wrong type by its key, and a syntax error as path:line:column in bytes.
func ParseJSONError(path string, raw []byte, err error) error {
	semanticErr, ok := errors.AsType[*json.SemanticError](err)
	if !ok || semanticErr.GoType == nil {
		return syntaxError(path, raw, err)
	}

	tokens := slices.Collect(semanticErr.JSONPointer.Tokens())

	key := strings.Join(tokens, ".")
	if key == "" {
		key = "the config"
	}

	if errors.Is(semanticErr.Err, json.ErrUnknownName) {
		return fmt.Errorf("%s: unknown key %q", path, key)
	}

	if errors.Is(semanticErr.Err, strconv.ErrRange) {
		return fmt.Errorf("%s: %s is out of range: %s", path, key, semanticErr.JSONValue)
	}

	return fmt.Errorf("%s: %s must be %s, not %s", path, key, describeGoType(semanticErr.GoType), describeJSONValue(semanticErr.JSONKind, semanticErr.JSONValue))
}

func syntaxError(path string, raw []byte, err error) error {
	syntaxErr, ok := errors.AsType[*jsontext.SyntacticError](err)
	if !ok {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	before := raw[:min(syntaxErr.ByteOffset, int64(len(raw)))]
	line := bytes.Count(before, []byte("\n")) + 1
	column := len(before) - (bytes.LastIndexByte(before, '\n') + 1) + 1

	// The cause alone, since the full message repeats the offset as a byte count.
	if errors.Is(syntaxErr.Err, jsontext.ErrDuplicateName) {
		return fmt.Errorf("%s:%d:%d: %w %q", path, line, column, syntaxErr.Err, syntaxErr.JSONPointer.LastToken())
	}

	return fmt.Errorf("%s:%d:%d: %w", path, line, column, syntaxErr.Err)
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
	if t == reflect.TypeFor[StringOrSlice]() {
		return "a string or a list of strings"
	}

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

// encoding/json keeps the number itself only when the number is the fault, as
// for a fraction in a whole-number field.
func describeJSONValue(kind jsontext.Kind, value jsontext.Value) string {
	switch kind {
	case jsontext.KindTrue, jsontext.KindFalse:
		return "a boolean"
	case jsontext.KindString:
		return "a string"
	case jsontext.KindBeginArray:
		return "a list"
	case jsontext.KindBeginObject:
		return "an object"
	case jsontext.KindNumber:
		if len(value) > 0 {
			return string(value)
		}

		return "a number"
	default:
		return kind.String()
	}
}

// checkRenamedKeys turns a removed-but-recognized key into a migration hint
// instead of the generic unknown-key error.
func checkRenamedKeys(raw []byte, path string) error {
	var probe struct {
		PHP map[string]jsontext.Value `json:"php"`
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
	var generic map[string]jsontext.Value

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
		var markdown map[string]jsontext.Value

		if err := json.Unmarshal(rawMarkdown, &markdown); err == nil {
			if _, ok := markdown["proseWrap"]; ok {
				keys["markdown.proseWrap"] = true
			}
		}
	}

	return keys, nil
}
