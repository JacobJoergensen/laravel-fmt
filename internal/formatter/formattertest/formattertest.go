// Package formattertest holds the fixture, fuzz, and round-trip helpers shared
// by the per-language formatter tests. It lives apart from package formatter so
// the testing package never links into the laravel-fmt binary.
package formattertest

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ReadFixture reads testdata/<name> with CRLF normalized to LF.
func ReadFixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}

	return NormalizeEOL(data)
}

// GoldenFixtureNames returns the base names of the ext input fixtures in
// testdata, skipping ".formatted" outputs and, when excludeInput is set, the
// "_input" fixtures that need a non-default config.
func GoldenFixtureNames(t *testing.T, ext string, excludeInput bool) []string {
	t.Helper()

	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}

	formatted := ".formatted" + ext

	var names []string

	for _, e := range entries {
		name := e.Name()

		if e.IsDir() {
			continue
		}

		if !strings.HasSuffix(name, ext) {
			continue
		}

		if strings.HasSuffix(name, formatted) {
			continue
		}

		base := strings.TrimSuffix(name, ext)

		if excludeInput && strings.HasSuffix(base, "_input") {
			continue
		}

		names = append(names, base)
	}

	return names
}

// AddFuzzSeeds seeds a fuzz target with every testdata fixture ending in ext,
// including the ".formatted" siblings: already-formatted input is a valid seed.
func AddFuzzSeeds(f *testing.F, ext string) {
	f.Helper()

	entries, err := os.ReadDir("testdata")
	if err != nil {
		f.Fatalf("read testdata: %v", err)
	}

	for _, e := range entries {
		name := e.Name()

		if e.IsDir() || !strings.HasSuffix(name, ext) {
			continue
		}

		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			f.Fatalf("read %s: %v", name, err)
		}

		f.Add(NormalizeEOL(data))
	}
}

// CheckIdempotent asserts the formatter contract on one fuzz input: formatting
// may reject invalid input, but any output it produces must itself format to a
// byte-identical result.
func CheckIdempotent(t *testing.T, format func([]byte) ([]byte, error), src []byte) {
	t.Helper()

	out, err := format(src)
	if err != nil {
		return
	}

	out2, err := format(out)
	if err != nil {
		t.Fatalf("formatted output failed to re-format: %v", err)
	}

	if diff := FirstDiff(out, out2); diff >= 0 {
		t.Fatalf("not idempotent at offset %d.\n=== first ===\n%s\n=== second ===\n%s", diff, out, out2)
	}
}

// SpanCoverage records which source bytes are claimed by parse tree spans.
type SpanCoverage struct {
	t       *testing.T
	src     []byte
	covered []bool
}

// NewSpanCoverage returns a coverage map over src.
func NewSpanCoverage(t *testing.T, src []byte) *SpanCoverage {
	t.Helper()

	return &SpanCoverage{t: t, src: src, covered: make([]bool, len(src))}
}

// Cover marks [start, end) as covered. Empty and inverted ranges are ignored,
// so an optional span such as an omitted semicolon needs no guard.
func (c *SpanCoverage) Cover(start, end int) {
	c.t.Helper()

	if end <= start {
		return
	}

	if start < 0 || end > len(c.src) {
		c.t.Fatalf("span [%d, %d) outside source of %d bytes", start, end, len(c.src))
	}

	for i := start; i < end; i++ {
		c.covered[i] = true
	}
}

// AssertOnlyAllowedUncovered fails at the first uncovered byte not in allowed.
func (c *SpanCoverage) AssertOnlyAllowedUncovered(allowed string) {
	c.t.Helper()

	for i, covered := range c.covered {
		if covered || strings.IndexByte(allowed, c.src[i]) >= 0 {
			continue
		}

		line := 1 + bytes.Count(c.src[:i], []byte("\n"))
		context := c.src[max(i-20, 0):min(i+20, len(c.src))]

		c.t.Fatalf("byte %q at offset %d (line %d) is not covered by any node span\ncontext: %q", c.src[i], i, line, context)
	}
}

// FirstDiff returns the first byte offset where a and b differ, or -1 when
// they are equal.
func FirstDiff(a, b []byte) int {
	n := min(len(a), len(b))

	for i := range n {
		if a[i] != b[i] {
			return i
		}
	}

	if len(a) != len(b) {
		return n
	}

	return -1
}

// NormalizeEOL rewrites CRLF to LF.
func NormalizeEOL(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}
