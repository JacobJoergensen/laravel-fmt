package json

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
)

func FuzzFormat(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".json")
	cfg := config.Default()

	f.Fuzz(func(t *testing.T, src []byte) {
		format := func(b []byte) ([]byte, error) { return Format("fuzz.json", b, cfg) }
		out, err := formattertest.FormatIdempotent(t, format, src)
		valid := isStrictJSON(src)

		if valid != (err == nil) {
			t.Fatalf("jsontext valid = %v, Format error = %v, for %q", valid, err, src)
		}

		if valid {
			checkSameTokens(t, src, out)
		}
	})
}

// JSONC is not JSON, so only input jsontext reads is held to it: such input
// must format, to the same tokens.
func FuzzFormatJSONC(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".json")
	formattertest.AddFuzzSeeds(f, ".jsonc")
	cfg := config.Default()

	f.Fuzz(func(t *testing.T, src []byte) {
		format := func(b []byte) ([]byte, error) { return Format("fuzz.jsonc", b, cfg) }
		out, err := formattertest.FormatIdempotent(t, format, src)
		if err == nil {
			checkSameComments(t, src, out)
		}

		if !isStrictJSON(src) {
			return
		}

		if err != nil {
			t.Fatalf("Format rejected JSON that jsontext reads: %v, for %q", err, src)
		}

		checkSameTokens(t, src, out)
	})
}

// The formatter may re-indent a block comment and trims the end of a line
// comment, so each comment is compared line by line with its edges trimmed.
func checkSameComments(t *testing.T, src, out []byte) {
	t.Helper()

	want := jsoncComments(src)
	got := jsoncComments(out)

	if strings.Join(want, "\x00") != strings.Join(got, "\x00") {
		t.Fatalf("comments differ:\ninput  %q\noutput %q\nwant %q\ngot  %q", src, out, want, got)
	}
}

func jsoncComments(src []byte) []string {
	var comments []string

	for i := 0; i < len(src); i++ {
		var end int

		switch {
		case src[i] == '"':
			for i++; i < len(src) && src[i] != '"'; i++ {
				if src[i] == '\\' {
					i++
				}
			}

			continue
		case bytes.HasPrefix(src[i:], []byte("//")):
			end = len(src)
			if n := bytes.IndexAny(src[i:], "\r\n"); n >= 0 {
				end = i + n
			}
		case bytes.HasPrefix(src[i:], []byte("/*")):
			end = len(src)
			if n := bytes.Index(src[i+2:], []byte("*/")); n >= 0 {
				end = i + 2 + n + 2
			}
		default:
			continue
		}

		comments = append(comments, trimmedLines(src[i:end]))
		i = end - 1
	}

	return comments
}

func trimmedLines(comment []byte) string {
	text := strings.ReplaceAll(string(comment), "\r\n", "\n")
	unified := strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(unified, "\n")

	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}

	return strings.Join(lines, "\n")
}

// RFC 8259 allows what jsontext refuses by default: a key written twice, and an
// escape naming a lone surrogate like "\ud800". The text itself must still be
// UTF-8. A blank file is not JSON, but it has nothing to format.
var rfc8259 = []jsontext.Options{jsontext.AllowDuplicateNames(true), jsontext.AllowInvalidUTF8(true)}

// parserNestingCap is the parser's maxNestingDepth. It refuses deeper input
// on purpose, where jsontext reads up to 10000 levels.
const parserNestingCap = 500

func isStrictJSON(src []byte) bool {
	trimmed := bytes.Trim(src, " \t\r\n")
	if len(trimmed) == 0 {
		return true
	}

	return utf8.Valid(src) && jsontext.Value(src).IsValid(rfc8259...) && valueDepth(src) <= parserNestingCap
}

// valueDepth counts nesting as the parser does: one level per value, the
// innermost scalar included, and none for an object key.
func valueDepth(src []byte) int {
	dec := jsontext.NewDecoder(bytes.NewReader(src), rfc8259...)
	deepest := 0

	for {
		depth := dec.StackDepth()
		parent, length := dec.StackIndex(depth)
		kind := dec.PeekKind()

		var err error

		switch kind {
		case jsontext.KindInvalid:
			return deepest
		case jsontext.KindEndObject, jsontext.KindEndArray:
			_, err = dec.ReadToken()
		case jsontext.KindBeginObject, jsontext.KindBeginArray:
			deepest = max(deepest, depth+1)
			_, err = dec.ReadToken()
		default:
			if parent != jsontext.KindBeginObject || length%2 == 1 {
				deepest = max(deepest, depth+1)
			}

			_, err = dec.ReadValue()
		}

		if err != nil {
			return deepest
		}
	}
}

type jsonToken struct {
	kind jsontext.Kind
	raw  string
}

func checkSameTokens(t *testing.T, src, out []byte) {
	t.Helper()

	want := readTokens(t, src)
	got := readTokens(t, out)

	for i := range max(len(want), len(got)) {
		if i >= len(want) || i >= len(got) || want[i] != got[i] {
			t.Fatalf("token %d differs:\ninput  %q\noutput %q", i, src, out)
		}
	}
}

func readTokens(t *testing.T, src []byte) []jsonToken {
	t.Helper()

	dec := jsontext.NewDecoder(bytes.NewReader(src), rfc8259...)

	var tokens []jsonToken

	for {
		kind := dec.PeekKind()

		switch kind {
		case jsontext.KindInvalid:
			if _, err := dec.ReadToken(); !errors.Is(err, io.EOF) {
				t.Fatalf("read %q: %v", src, err)
			}

			return tokens
		case jsontext.KindBeginObject, jsontext.KindEndObject, jsontext.KindBeginArray, jsontext.KindEndArray:
			if _, err := dec.ReadToken(); err != nil {
				t.Fatalf("read %q: %v", src, err)
			}

			tokens = append(tokens, jsonToken{kind: kind})
		default:
			value, err := dec.ReadValue()
			if err != nil {
				t.Fatalf("read %q: %v", src, err)
			}

			tokens = append(tokens, jsonToken{kind: kind, raw: string(value)})
		}
	}
}
