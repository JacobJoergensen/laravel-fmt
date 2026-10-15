package blade

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/formattertest"
	htmlfmt "github.com/JacobJoergensen/laravel-fmt/internal/formatter/html"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/blade"
	htmlparser "github.com/JacobJoergensen/laravel-fmt/internal/parser/html"
)

func FuzzFormat(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".blade.php")
	cfg := config.Default()

	f.Fuzz(func(t *testing.T, src []byte) {
		formattertest.CheckIdempotent(t, func(b []byte) ([]byte, error) {
			return Format(b, cfg)
		}, src)
	})
}

// FuzzFormatTailwind exercises the Tailwind-enabled path (class="" sorting and
// `@class([...])` class-list sorting) for idempotence, which the default-config
// FuzzFormat never reaches.
func FuzzFormatTailwind(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".blade.php")
	cfg := config.Default()
	cfg.Tailwind = true

	f.Fuzz(func(t *testing.T, src []byte) {
		formattertest.CheckIdempotent(t, func(b []byte) ([]byte, error) {
			return Format(b, cfg)
		}, src)
	})
}

func FuzzFormatBlankLineBeforeDirectives(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".blade.php")
	cfg := config.Default()
	cfg.Blade.BlankLineBeforeDirectives = append(parser.BuiltinOpeners(), config.BladeDirectivePHP)

	f.Fuzz(func(t *testing.T, src []byte) {
		formattertest.CheckIdempotent(t, func(b []byte) ([]byte, error) {
			return Format(b, cfg)
		}, src)
	})
}

// A close edge at the end of the file matches either way.
func sameGlue(a, b string) bool {
	if a == b {
		return true
	}

	ai, bi := strings.LastIndexByte(a, ':'), strings.LastIndexByte(b, ':')

	return (a[ai+1:] == "eof" || b[bi+1:] == "eof") && a[:ai] == b[:bi]
}

// Only inline containers holding inline HTML count. A block element strips its
// edge whitespace, as Laravel does a component slot's, and a raw-body mail component (markdown) or raw-text element
// (JS/CSS) has no inline box to pad, so reflowing those is layout, not content.
func glueProfile(src []byte) (profile []string, ok bool) {
	file, errs := parser.Parse(src, nil)
	if formatter.JoinErrors(errs) != nil {
		return nil, false
	}

	var walk func(n parser.Node)

	walk = func(n parser.Node) {
		switch v := n.(type) {
		case *htmlparser.Element:
			body := make([]parser.Node, len(v.Children))
			for i, c := range v.Children {
				body[i] = c
			}

			if htmlfmt.IsInlineContainer(v.Name) && !parser.TrimsSlot(v.Name) && !parser.IsRawBodyComponent(v.Name) &&
				!htmlparser.IsRawTextElement(v.Name) {
				// An element left open to the end of the file closes at its end,
				// where the final newline every file gains renders nothing.
				closer := strconv.FormatBool(bodyGluedToCloseMarker(body))
				if v.CloseFinal.End.Offset == 0 && len(bytes.TrimRight(src[v.CloseOpen.Start.Offset:], " \t\r\n")) == 0 {
					closer = "eof"
				}

				profile = append(profile, fmt.Sprintf("%s:%t:%s", v.Name, bodyGluedToOpenMarker(body), closer))
			}

			for _, c := range body {
				walk(c)
			}
		case *parser.DirectiveBlockNode:
			for _, c := range v.Body {
				walk(c)
			}

			for _, b := range v.Branches {
				for _, c := range b.Body {
					walk(c)
				}
			}
		}
	}

	for _, n := range file.Nodes {
		walk(n)
	}

	return profile, true
}

// The whitespace between an inline element's markers and its body renders as a
// space inside the inline box, so formatting must never add or remove it. Only
// inline containers are checked, so a misclassified element escapes this.
func FuzzFormatGlue(f *testing.F) {
	formattertest.AddFuzzSeeds(f, ".blade.php")

	cfg := config.Default()

	f.Fuzz(func(t *testing.T, src []byte) {
		before, ok := glueProfile(src)
		if !ok {
			return
		}

		out, err := Format(src, cfg)
		if err != nil {
			return
		}

		after, ok := glueProfile(out)
		if !ok {
			t.Fatalf("formatted output does not parse.\n=== src ===\n%s\n=== out ===\n%s", src, out)
		}

		if !slices.EqualFunc(before, after, sameGlue) {
			t.Fatalf("element glue changed.\n=== before ===\n%v\n=== after ===\n%v\n=== src ===\n%s\n=== out ===\n%s",
				before, after, src, out)
		}
	})
}
