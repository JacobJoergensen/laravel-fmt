package dispatch

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter"
)

func TestStripBOMBeforeDispatch(t *testing.T) {
	cases := []struct {
		name string
		path string
		src  string
	}{
		{"php", "x.php", "<?php\n$x = 1;\n"},
		{"blade template", "x.blade.php", "<div>{{ $name }}</div>\n"},
		{"livewire sfc", "x.blade.php", "<?php\n$x = 1;\n?>\n<div></div>\n"},
		{"html", "x.html", "<div>x</div>\n"},
		{"css", "x.css", ".x {\n    color: red;\n}\n"},
		{"markdown", "x.md", "# Heading\n"},
	}

	cfg := config.Default()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cleanOut, err := File(tc.path, []byte(tc.src), cfg)
			if err != nil {
				t.Fatalf("File on clean source returned error: %v", err)
			}

			withBOM := slices.Concat(utf8BOM, []byte(tc.src))

			bomOut, err := File(tc.path, withBOM, cfg)
			if err != nil {
				t.Fatalf("File on BOM-prefixed source returned error: %v", err)
			}

			if !bytes.Equal(cleanOut, bomOut) {
				t.Fatalf("BOM stripping changed output:\n=== clean ===\n%s\n=== bom ===\n%s", cleanOut, bomOut)
			}

			if bytes.HasPrefix(bomOut, utf8BOM) {
				t.Fatalf("output retained leading BOM: %q", bomOut)
			}
		})
	}
}

func TestFileReturnsAnUnknownExtensionUnchanged(t *testing.T) {
	src := slices.Concat(utf8BOM, []byte("leave me alone\n"))

	out, err := File("x.txt", src, config.Default())
	if err != nil {
		t.Fatalf("File returned error: %v", err)
	}

	if !bytes.Equal(out, src) {
		t.Fatalf("got %q, want %q", out, src)
	}
}

func TestFileRefusesPathologicalNesting(t *testing.T) {
	// Nesting past the parser's depth cap is refused with a located error; run
	// into, it would be a stack overflow, which no recover can catch.
	deep := []byte("<?php\n$a = " + strings.Repeat("[", 5000) + strings.Repeat("]", 5000) + ";\n")

	out, err := File("deep.php", deep, config.Default())

	if err == nil {
		t.Fatal("pathological nesting was accepted; want a located error")
	}

	if out != nil {
		t.Fatalf("bytes returned alongside an error: %q", out)
	}
}

func TestFilePanicBecomesAFileError(t *testing.T) {
	realFormatLanguage := formatLanguage

	formatLanguage = func(formatter.Language, string, []byte, *config.File) ([]byte, error) {
		panic("emitter bug")
	}

	out, err := File("bad.php", []byte("<?php\n"), config.Default())

	formatLanguage = realFormatLanguage

	if err == nil || !strings.Contains(err.Error(), "bad.php") || !strings.Contains(err.Error(), "emitter bug") {
		t.Fatalf("got error %v, want one naming bad.php and the panic", err)
	}

	if out != nil {
		t.Fatalf("bytes returned alongside an error: %q", out)
	}

	got, err := File("ok.php", []byte("<?php\n$a   =   1;\n"), config.Default())
	if err != nil {
		t.Fatalf("a later file failed after a bad one: %v", err)
	}

	if !bytes.Contains(got, []byte("$a = 1;")) {
		t.Fatalf("later file not formatted: %q", got)
	}
}
