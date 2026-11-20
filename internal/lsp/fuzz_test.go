package lsp

import (
	"bufio"
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/dispatch"
)

func FuzzBuildEdits(f *testing.F) {
	f.Add("<?php\n$a=1;\n", "<?php\n\n$a = 1;\n")
	f.Add("a\r\nb\r\nc", "a\r\nB\r\nc\r\n")
	f.Add("a\rb\rc", "a\rc")
	f.Add("x \U0001D54F y\n", "x\n\U0001D54F\ny")
	f.Add("", "a")
	f.Add("a\r", "a\r\n")
	f.Add("\r\n\r", "\n\r\r\n")

	f.Fuzz(func(t *testing.T, src, out string) {
		if !utf8.ValidString(src) || !utf8.ValidString(out) {
			t.Skip("an LSP client only sends UTF-8")
		}

		if got := applyEditsUTF8(src, buildEdits([]byte(src), []byte(out), encodingUTF8)); got != out {
			t.Fatalf("utf-8 edits applied to %q gave %q, want %q", src, got, out)
		}

		if got := applyEditsUTF16(src, buildEdits([]byte(src), []byte(out), encodingUTF16)); got != out {
			t.Fatalf("utf-16 edits applied to %q gave %q, want %q", src, got, out)
		}
	})
}

var fuzzRangeFiles = []string{"t.php", "t.blade.php", "t.md", "t.yaml", "t.css", "t.html", "t.json", "t.xml"}

func FuzzRangeFormatting(f *testing.F) {
	f.Add(uint8(0), "<?php\nuse B;\nuse A;\n$a=1;\nif($x){\n$y=2;}\n", 1, 0, 3, 0)
	f.Add(uint8(1), "<div>\n@if($a)\n<p>{{$b}}</p>\n@endif\n</div>\n", 2, 0, 2, 5)
	f.Add(uint8(2), "-   item\n\n    para\n\n````\ncode\n````\n", 0, 3, 5, 1)
	f.Add(uint8(3), "key: |\n    a\n\n    b\nother:\n    - x\n", 4, 0, 1, 0)
	f.Add(uint8(4), "a{color:red;b{c:d}}\n", 0, 2, 0, 9)
	f.Add(uint8(5), "<div>\n\n<p>a</p>\n\n\n<p>b</p>\n\n</div>\n", -1, -5, 99, 99)
	f.Add(uint8(6), "{\"a\":1,\n\"b\":[1,2]}", 1, 0, 1, 3)
	f.Add(uint8(7), "<a><b>x</b>\n<c/></a>\n", 0, 0, 1, 0)

	f.Fuzz(func(t *testing.T, kind uint8, src string, startLine, startChar, endLine, endChar int) {
		if !utf8.ValidString(src) {
			t.Skip("an LSP client only sends UTF-8")
		}

		name := fuzzRangeFiles[int(kind)%len(fuzzRangeFiles)]

		cfg := config.Default()
		config.ResolveEndOfLine([]byte(src), cfg)

		want, err := dispatch.File("/"+name, []byte(src), cfg)
		if err != nil {
			t.Skip("unparseable input is refused, not formatted")
		}

		s, uri := newDirectServer(t, name, src)
		s.encoding = encodingUTF16

		r := Range{Start: Position{startLine, startChar}, End: Position{endLine, endChar}}

		edits, err := s.rangeEdits(uri, r)
		if err != nil {
			t.Fatalf("range %+v of a parseable %s: %v", r, name, err)
		}

		applied := applyEditsUTF16(src, edits)

		got, err := dispatch.File("/"+name, []byte(applied), cfg)
		if err != nil {
			t.Fatalf("range %+v left %s that does not parse: %v\n%q", r, name, err, applied)
		}

		if !bytes.Equal(got, want) {
			t.Fatalf("range %+v changed what %s formats to.\nsrc:     %q\napplied: %q\ngot:     %q\nwant:    %q", r, name, src, applied, got, want)
		}
	})
}

func FuzzServe(f *testing.F) {
	uri := pathToURI(filepath.Join(f.TempDir(), "t.php"))

	session := func(bodies ...string) []byte {
		var b bytes.Buffer
		for _, body := range bodies {
			b.WriteString(frame(body))
		}

		return b.Bytes()
	}

	f.Add(session(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{"general":{"positionEncodings":["utf-8"]}}}}`,
		`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"`+uri+`","text":"<?php\n$a=1;\n"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"textDocument/formatting","params":{"textDocument":{"uri":"`+uri+`"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"textDocument/rangeFormatting","params":{"textDocument":{"uri":"`+uri+`"},"range":{"start":{"line":1,"character":0},"end":{"line":1,"character":4}}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"shutdown"}`,
		`{"jsonrpc":"2.0","method":"exit"}`,
	))
	f.Add(session(
		`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":"a","method":"laravel-fmt/format","params":{"textDocument":{"uri":"`+uri+`"},"text":"<?php\r\nif($a){}\r\n","range":{"start":{"line":-3,"character":-1},"end":{"line":0,"character":99}}}}`,
		`{"jsonrpc":"2.0","id":"b","method":"laravel-fmt/endOfLine","params":{"textDocument":{"uri":"`+uri+`"}}}`,
		`{"jsonrpc":"2.0","method":"textDocument/didChange","params":{"textDocument":{"uri":"`+uri+`"},"contentChanges":[{"text":"<?php\n"}]}}`,
		`{"id":null,"method":"x"}`,
		`[1,2]`,
		`{"jsonrpc":"2.0","id":5,"method":"textDocument/formatting","params":null}`,
	))
	f.Add([]byte("Content-Length: 3\r\n\r\n{]}Content-Length: 2\r\n\r\n{}"))

	f.Fuzz(func(t *testing.T, in []byte) {
		var out bytes.Buffer

		// Garbage may end the session with an error; only a panic or a broken
		// reply fails.
		_ = Serve(bytes.NewReader(in), &out, io.Discard, "fuzz")

		r := bufio.NewReader(&out)

		for {
			body, err := readFrame(r)
			if errors.Is(err, io.EOF) {
				return
			}

			if err != nil {
				t.Fatalf("server wrote a broken frame: %v\n%q", err, out.String())
			}

			if !jsontext.Value(body).IsValid(jsontext.AllowInvalidUTF8(true)) {
				t.Fatalf("server wrote a frame that is not JSON: %q", body)
			}
		}
	})
}
