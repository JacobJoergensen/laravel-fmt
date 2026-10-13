package php

import (
	"strings"
	"testing"
)

func TestKeywordLenBounds(t *testing.T) {
	shortest, longest := len("include_once"), 0

	for kw := range keywords {
		shortest = min(shortest, len(kw))
		longest = max(longest, len(kw))

		if kw != strings.ToLower(kw) {
			t.Errorf("keyword %q is not lowercase; byte-wise lowering would never match it", kw)
		}

		for i := range len(kw) {
			if kw[i] >= 0x80 {
				t.Errorf("keyword %q is not ASCII; byte-wise lowering is only valid for ASCII", kw)

				break
			}
		}
	}

	if shortest != keywordLenMin {
		t.Errorf("shortest keyword is %d bytes, keywordLenMin is %d", shortest, keywordLenMin)
	}

	if longest != keywordLenMax {
		t.Errorf("longest keyword is %d bytes, keywordLenMax is %d", longest, keywordLenMax)
	}
}

// Agreement with strings.ToLower is asserted for ASCII only: past 0x80 the
// two differ and byte-wise is right, as TestKeywordKindIgnoresUnicodeCase pins.
func TestKeywordKindMatchesToLowerPath(t *testing.T) {
	viaToLower := func(s string) Kind {
		if k, ok := keywords[strings.ToLower(s)]; ok {
			return k
		}

		return KindIdent
	}

	casings := func(kw string) []string {
		mixed := []byte(kw)
		for i := range mixed {
			if i%2 == 0 {
				mixed[i] = strings.ToUpper(string(mixed[i]))[0]
			}
		}

		return []string{kw, strings.ToUpper(kw), string(mixed)}
	}

	for kw, want := range keywords {
		for _, form := range casings(kw) {
			if got := KeywordKind([]byte(form)); got != want {
				t.Errorf("KeywordKind(%q) = %v, want %v", form, got, want)
			}

			if got, viaLower := KeywordKind([]byte(form)), viaToLower(form); got != viaLower {
				t.Errorf("KeywordKind(%q) = %v, ToLower path = %v", form, got, viaLower)
			}
		}
	}

	// `yield from` is here because the lexer never presents it as one word (see
	// TestYieldFromLexesAsOneToken).
	misses := []string{
		"", "a", "x", "if_", "_if", "iff", "yield from", "YIELD FROM",
		"getUserName", "Collection", "InvalidArgumentException",
		strings.Repeat("a", keywordLenMax+1), "include_once_",
	}

	for _, s := range misses {
		if got := KeywordKind([]byte(s)); got != KindIdent {
			t.Errorf("KeywordKind(%q) = %v, want KindIdent", s, got)
		}

		if got, viaLower := KeywordKind([]byte(s)), viaToLower(s); got != viaLower {
			t.Errorf("KeywordKind(%q) = %v, ToLower path = %v", s, got, viaLower)
		}
	}
}

// strings.ToLower maps U+0130 to `i` and U+212A to `k`, but PHP matches
// keywords with ASCII character classes only, so each of these stays an
// identifier.
func TestKeywordKindIgnoresUnicodeCase(t *testing.T) {
	cases := []string{
		"İF",    // LATIN CAPITAL LETTER I WITH DOT ABOVE + F; ToLower gives "if"
		"breaK", // BREA + KELVIN SIGN; ToLower gives "break"
		"ıf",    // dotless i + f
		"ﬁnal",  // fi ligature + nal
		"ＩＦ",    // fullwidth I F
		"ünicode", "ÅSA",
	}

	for _, s := range cases {
		if got := KeywordKind([]byte(s)); got != KindIdent {
			t.Errorf("KeywordKind(%q) = %v, want KindIdent (PHP matches keywords in ASCII case only)", s, got)
		}
	}
}

func TestKeywordKindDoesNotAllocate(t *testing.T) {
	cases := [][]byte{
		[]byte("FOREACH"),   // uppercase keyword: takes the lowering loop
		[]byte("foreach"),   // already lowercase: the case ToLower also gets right
		[]byte("userIdent"), // miss, but inside the length bounds
	}

	for _, text := range cases {
		got := testing.AllocsPerRun(100, func() {
			keywordSink = KeywordKind(text)
		})

		if got != 0 {
			t.Errorf("KeywordKind(%q) allocates %.1f times per call, want 0", text, got)
		}
	}
}

// keywordSink keeps the call above from being optimized away as dead code.
var keywordSink Kind
