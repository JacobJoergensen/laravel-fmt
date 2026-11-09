package xml

import (
	"bytes"
	stdxml "encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
)

// encoding/xml is the oracle: XML it reads must format to the same tokens, and
// XML it refuses must not format. It reads more than XML 1.0 allows, so
// wellFormed adds the rules it skips.
func FuzzFormat(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".xml")
	cfg := config.Default()

	f.Fuzz(func(t *testing.T, src []byte) {
		format := func(b []byte) ([]byte, error) { return Format(b, cfg) }
		out, err := formattertest.FormatIdempotent(t, format, src)

		want, verr := wellFormed(src)
		if errors.Is(verr, errOutsideOracle) {
			return
		}

		if (verr == nil) != (err == nil) {
			t.Fatalf("encoding/xml error = %v, Format error = %v, for %q", verr, err, src)
		}

		if verr != nil {
			return
		}

		got, oerr := wellFormed(out)
		if oerr != nil {
			t.Fatalf("output is not well-formed: %v\ninput  %q\noutput %q", oerr, src, out)
		}

		if strings.Join(want, "\x00") != strings.Join(got, "\x00") {
			t.Fatalf("tokens differ\ninput  %q\noutput %q\nwant %q\ngot  %q", src, out, want, got)
		}
	})
}

// A fragment drops only the document rules, so a document formats the same
// either way.
func FuzzFormatFragment(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".xml")
	cfg := config.Default()

	f.Fuzz(func(t *testing.T, src []byte) {
		fragment := func(b []byte) ([]byte, error) { return FormatFragment(b, cfg) }
		out, err := formattertest.FormatIdempotent(t, fragment, src)

		document, derr := Format(src, cfg)
		if derr == nil && (err != nil || !bytes.Equal(out, document)) {
			t.Fatalf("a document formats differently as a fragment: %v\ninput    %q\ndocument %q\nfragment %q", err, src, document, out)
		}
	})
}

// errOutsideOracle marks input encoding/xml cannot judge: a version other
// than 1.0, an encoding other than UTF-8, or a name its fourth-edition tables
// refuse but the fifth edition allows.
var errOutsideOracle = errors.New("outside what encoding/xml reads")

var (
	undeclaredEntity = regexp.MustCompile(`invalid character entity &([^#;\s]+);$`)
	refusedName      = regexp.MustCompile(`invalid XML name: (.*)$`)

	declarationGrammar = regexp.MustCompile(`^<\?xml[ \t\r\n]+version[ \t\r\n]*=[ \t\r\n]*("1\.[0-9]+"|'1\.[0-9]+')` +
		`([ \t\r\n]+encoding[ \t\r\n]*=[ \t\r\n]*("[A-Za-z][A-Za-z0-9._-]*"|'[A-Za-z][A-Za-z0-9._-]*'))?` +
		`([ \t\r\n]+standalone[ \t\r\n]*=[ \t\r\n]*("(yes|no)"|'(yes|no)'))?[ \t\r\n]*\?>$`)
)

// encoding/xml refuses a named entity it does not know; one a DOCTYPE could
// declare is accepted on a second read.
//
// A colon is read as an underscore. XML 1.0 gives both the same place in
// every production, but encoding/xml splits a name at its colon for
// namespaces and refuses a name like `::`, which XML 1.0 allows.
func wellFormed(src []byte) ([]string, error) {
	if blank := len(bytes.Trim(src, " \t\r\n")) == 0; blank {
		return nil, nil
	}

	src = bytes.ReplaceAll(src, []byte(":"), []byte("_"))

	entities := map[string]string{}

	for {
		tokens, declares, err := readTokens(src, entities)
		if errors.Is(err, errOutsideOracle) {
			return nil, err
		}

		m := undeclaredEntity.FindStringSubmatch(fmt.Sprint(err))
		if m != nil && declares && isXMLName(m[1]) && entities[m[1]] == "" {
			entities[m[1]] = "&"

			continue
		}

		// encoding/xml checks characters in text only.
		if cerr := checkXMLChars(src); cerr != nil {
			return nil, cerr
		}

		return tokens, err
	}
}

func checkXMLChars(src []byte) error {
	if !utf8.Valid(src) {
		return errors.New("invalid UTF-8")
	}

	for _, r := range string(src) {
		if r != '\t' && r != '\n' && r != '\r' && (r < 0x20 || r > 0xD7FF && r < 0xE000 || r > 0xFFFD && r < 0x10000) {
			return fmt.Errorf("character %U", r)
		}
	}

	return nil
}

func readTokens(src []byte, entities map[string]string) ([]string, bool, error) {
	dec := stdxml.NewDecoder(bytes.NewReader(src))
	dec.Strict = true
	dec.Entity = entities
	dec.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		return nil, fmt.Errorf("encoding %q: %w", label, errOutsideOracle)
	}

	var (
		tokens     []string
		open       []string
		roots      int
		hasDoctype bool
		declares   bool
	)

	for {
		before := dec.InputOffset()

		tok, err := dec.RawToken()
		if errors.Is(err, io.EOF) {
			if roots != 1 {
				return nil, declares, fmt.Errorf("%d root elements", roots)
			}

			if len(open) > 0 {
				return nil, declares, errors.New("unclosed element")
			}

			return tokens, declares, nil
		}

		if err != nil {
			name := refusedName.FindStringSubmatch(err.Error())
			if strings.Contains(err.Error(), "unsupported version") || len(name) == 2 && isXMLName(name[1]) {
				err = errOutsideOracle
			}

			return nil, declares, err
		}

		raw := src[before:dec.InputOffset()]

		switch v := tok.(type) {
		case stdxml.StartElement:
			if err := checkStartTag(v, raw); err != nil {
				return nil, declares, err
			}

			if len(open) == 0 {
				roots++
			}

			open = append(open, v.Name.Local)
			tokens = append(tokens, fmt.Sprintf("<%s %v", v.Name.Local, v.Attr))
		case stdxml.EndElement:
			if len(open) == 0 || open[len(open)-1] != v.Name.Local {
				return nil, declares, fmt.Errorf("unexpected </%s>", v.Name.Local)
			}

			open = open[:len(open)-1]
			tokens = append(tokens, "</"+v.Name.Local)
		case stdxml.CharData:
			if len(open) == 0 && len(bytes.Trim(raw, " \t\r\n")) > 0 {
				return nil, declares, errors.New("content outside the root element")
			}

			if len(bytes.Trim(v, " \t\r\n")) > 0 {
				tokens = append(tokens, fmt.Sprintf("T%q", v))
			}
		case stdxml.Comment:
			tokens = append(tokens, "C"+trimmedLines(v))
		case stdxml.ProcInst:
			reserved := strings.EqualFold(v.Target, "xml")
			afterTarget := raw[len("<?")+len(v.Target):]
			spaced := bytes.HasPrefix(afterTarget, []byte("?>")) || isSpace(afterTarget[0])

			if !isXMLName(v.Target) || !spaced || reserved && (before > 0 || !declarationGrammar.Match(raw)) {
				return nil, declares, fmt.Errorf("processing instruction <?%s", v.Target)
			}

			tokens = append(tokens, "P"+v.Target+" "+trimmedLines(v.Inst))
		case stdxml.Directive:
			doctype := bytes.HasPrefix(v, []byte("DOCTYPE")) && len(v) > len("DOCTYPE") && isSpace(v[len("DOCTYPE")])
			if !doctype || hasDoctype || roots > 0 {
				return nil, declares, errors.New("misplaced <!")
			}

			fields := strings.Fields(string(v))
			declares = bytes.ContainsRune(v, '[') || len(fields) > 2 && (fields[2] == "SYSTEM" || fields[2] == "PUBLIC")
			hasDoctype = true
			tokens = append(tokens, "D"+trimmedLines(v))
		}
	}
}

func checkStartTag(el stdxml.StartElement, raw []byte) error {
	if !isXMLName(el.Name.Local) {
		return fmt.Errorf("element name %q", el.Name.Local)
	}

	seen := map[string]bool{}

	for _, a := range el.Attr {
		name := a.Name.Local
		if !isXMLName(name) || seen[name] {
			return fmt.Errorf("attribute %q", name)
		}

		seen[name] = true
	}

	var quote byte

	for i, c := range raw {
		switch {
		case quote == 0 && (c == '"' || c == '\''):
			quote = c
		case c == quote:
			quote = 0

			if next := raw[i+1]; !isSpace(next) && next != '/' && next != '>' {
				return errors.New("no space between attributes")
			}
		}
	}

	return nil
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

// nameStartRanges is XML 1.0 production [4] NameStartChar, and nameRanges
// the characters [4a] NameChar adds to it.
var (
	nameStartRanges = [][2]rune{
		{':', ':'},
		{'A', 'Z'},
		{'_', '_'},
		{'a', 'z'},
		{0xC0, 0xD6},
		{0xD8, 0xF6},
		{0xF8, 0x2FF},
		{0x370, 0x37D},
		{0x37F, 0x1FFF},
		{0x200C, 0x200D},
		{0x2070, 0x218F},
		{0x2C00, 0x2FEF},
		{0x3001, 0xD7FF},
		{0xF900, 0xFDCF},
		{0xFDF0, 0xFFFD},
		{0x10000, 0xEFFFF},
	}
	nameRanges = [][2]rune{{'-', '.'}, {'0', '9'}, {0xB7, 0xB7}, {0x300, 0x36F}, {0x203F, 0x2040}}
)

func isXMLName(s string) bool {
	if s == "" {
		return false
	}

	for i, r := range s {
		if !inRanges(r, nameStartRanges) && (i == 0 || !inRanges(r, nameRanges)) {
			return false
		}
	}

	return true
}

func inRanges(r rune, ranges [][2]rune) bool {
	for _, rg := range ranges {
		if r >= rg[0] && r <= rg[1] {
			return true
		}
	}

	return false
}

// The formatter re-homes the lines of a comment, processing instruction or
// DOCTYPE, so they compare with each line's edges trimmed. A lone CR is a
// line break (XML 1.0 section 2.11).
func trimmedLines(b []byte) string {
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	unified := strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(unified, "\n")

	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}

	return strings.Join(lines, "\n")
}
