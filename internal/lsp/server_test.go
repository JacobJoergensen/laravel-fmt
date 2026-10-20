package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/dispatch"
	"github.com/JacobJoergensen/laravel-fmt/internal/upward"
)

// A fixture must not pick up the host's files above the temp directory.
func init() {
	upward.SetCeiling(os.TempDir())
}

func pathToURI(path string) string {
	slashed := filepath.ToSlash(path)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}

	return "file://" + slashed
}

type wireResponse struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *responseError  `json:"error"`
}

func readFrame(r *bufio.Reader) ([]byte, error) {
	length := -1

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read header: %w", err)
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}

		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			length, err = strconv.Atoi(strings.TrimSpace(line[len("content-length:"):]))
			if err != nil {
				return nil, fmt.Errorf("parse content-length: %w", err)
			}
		}
	}

	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return body, nil
}

const formatRequestID = "2"

func editsFromResponses(t *testing.T, out []byte) []TextEdit {
	t.Helper()

	resp := responseTo(t, out, formatRequestID)

	var edits []TextEdit

	if err := json.Unmarshal(resp.Result, &edits); err != nil {
		t.Fatalf("decode edits: %v", err)
	}

	return edits
}

func messagesSent(t *testing.T, out []byte, method string) []messageParams {
	t.Helper()

	r := bufio.NewReader(bytes.NewReader(out))

	var sent []messageParams

	for {
		body, err := readFrame(r)
		if errors.Is(err, io.EOF) {
			return sent
		}

		if err != nil {
			t.Fatalf("read frame: %v", err)
		}

		var msg struct {
			Method string        `json:"method"`
			Params messageParams `json:"params"`
		}

		if err := json.Unmarshal(body, &msg); err != nil {
			t.Fatalf("decode message: %v", err)
		}

		if msg.Method == method {
			sent = append(sent, msg.Params)
		}
	}
}

func responseTo(t *testing.T, out []byte, id string) wireResponse {
	t.Helper()

	r := bufio.NewReader(bytes.NewReader(out))

	for {
		body, err := readFrame(r)
		if err != nil {
			t.Fatalf("no response to request %s: %v", id, err)
		}

		var resp wireResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		if string(resp.ID) == id {
			return resp
		}
	}
}

func TestServerFormatsDocumentOverProtocol(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "sample.php")
	docURI := pathToURI(docPath)

	src := "<?php\n\necho   'hello'  ;\n"
	text, err := json.Marshal(src)
	if err != nil {
		t.Fatal(err)
	}

	input := frame(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":%q}}`, pathToURI(dir))) +
		frame(fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"text":%s}}}`, docURI, text)) +
		frame(fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"textDocument/formatting","params":{"textDocument":{"uri":%q}}}`, docURI)) +
		frame(`{"jsonrpc":"2.0","id":3,"method":"shutdown"}`) +
		frame(`{"jsonrpc":"2.0","method":"exit"}`)

	var out bytes.Buffer
	if err := Serve(strings.NewReader(input), &out, io.Discard, ""); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	edits := editsFromResponses(t, out.Bytes())

	cfg := config.Default()
	config.ResolveEndOfLine([]byte(src), cfg)

	want, err := dispatch.File(docPath, []byte(src), cfg)
	if err != nil {
		t.Fatalf("dispatch baseline: %v", err)
	}

	if got := applyEditsUTF8(src, edits); got != string(want) {
		t.Fatalf("applying formatting edits gave %q, want formatter output %q", got, want)
	}
}

func TestDidChangeIgnoresIncrementalSync(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "sample.php")
	docURI := pathToURI(docPath)

	opened := "<?php\n\nuse App\\Models\\Order;\n\necho Order::class;\n"
	buffer := "<?php\n\nuse App\\Models\\Order;\n\necho Order::class; \n"

	text, err := json.Marshal(opened)
	if err != nil {
		t.Fatal(err)
	}

	change := fmt.Sprintf(
		`{"jsonrpc":"2.0","method":"textDocument/didChange","params":{"textDocument":{"uri":%q,"version":2},`+
			`"contentChanges":[{"range":{"start":{"line":4,"character":18},"end":{"line":4,"character":18}},"rangeLength":0,"text":" "}]}}`,
		docURI,
	)

	input := frame(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":%q}}`, pathToURI(dir))) +
		frame(fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"text":%s}}}`, docURI, text)) +
		frame(change) +
		frame(fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"textDocument/formatting","params":{"textDocument":{"uri":%q}}}`, docURI)) +
		frame(`{"jsonrpc":"2.0","id":3,"method":"shutdown"}`) +
		frame(`{"jsonrpc":"2.0","method":"exit"}`)

	var out bytes.Buffer
	if err := Serve(strings.NewReader(input), &out, io.Discard, ""); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	edits := editsFromResponses(t, out.Bytes())

	if got := applyEditsUTF8(buffer, edits); got != buffer {
		t.Fatalf("edits computed from a change the server cannot interpret rewrote the client buffer to:\n%q\nwant it left alone:\n%q", got, buffer)
	}
}

func TestDidChangeReplacesBufferOnFullSync(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "sample.php")
	docURI := pathToURI(docPath)

	opened, err := json.Marshal("<?php\n\necho 1;\n")
	if err != nil {
		t.Fatal(err)
	}

	changed := "<?php\n\necho   2  ;\n"

	changedJSON, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}

	input := frame(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":%q}}`, pathToURI(dir))) +
		frame(fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"text":%s}}}`, docURI, opened)) +
		frame(fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didChange","params":{"textDocument":{"uri":%q,"version":2},"contentChanges":[{"text":%s}]}}`, docURI, changedJSON)) +
		frame(fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"textDocument/formatting","params":{"textDocument":{"uri":%q}}}`, docURI)) +
		frame(`{"jsonrpc":"2.0","id":3,"method":"shutdown"}`) +
		frame(`{"jsonrpc":"2.0","method":"exit"}`)

	var out bytes.Buffer
	if err := Serve(strings.NewReader(input), &out, io.Discard, ""); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	edits := editsFromResponses(t, out.Bytes())

	const want = "<?php\n\necho 2;\n"

	if got := applyEditsUTF8(changed, edits); got != want {
		t.Fatalf("formatting after a full change gave %q, want %q", got, want)
	}
}

func TestServeExitWithoutShutdownErrors(t *testing.T) {
	input := frame(`{"jsonrpc":"2.0","method":"exit"}`)

	err := Serve(strings.NewReader(input), &bytes.Buffer{}, io.Discard, "")
	if !errors.Is(err, ErrExitWithoutShutdown) {
		t.Fatalf("err = %v, want ErrExitWithoutShutdown", err)
	}
}

func TestServeShutdownThenExitIsClean(t *testing.T) {
	input := frame(`{"jsonrpc":"2.0","id":1,"method":"shutdown"}`) +
		frame(`{"jsonrpc":"2.0","method":"exit"}`)

	var out bytes.Buffer
	if err := Serve(strings.NewReader(input), &out, io.Discard, ""); err != nil {
		t.Fatalf("Serve = %v, want nil", err)
	}

	body, err := readFrame(bufio.NewReader(bytes.NewReader(out.Bytes())))
	if err != nil {
		t.Fatalf("read shutdown response: %v", err)
	}

	var resp wireResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode shutdown response: %v", err)
	}

	if string(resp.Result) != "null" {
		t.Fatalf("shutdown result = %q, want null", resp.Result)
	}
}

func TestInitializeReportsTheServerVersion(t *testing.T) {
	input := frame(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`) +
		frame(`{"jsonrpc":"2.0","id":2,"method":"shutdown"}`) +
		frame(`{"jsonrpc":"2.0","method":"exit"}`)

	var out bytes.Buffer
	if err := Serve(strings.NewReader(input), &out, io.Discard, "1.2.3"); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	var result initializeResult
	if err := json.Unmarshal(responseTo(t, out.Bytes(), "1").Result, &result); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}

	if result.ServerInfo.Version != "1.2.3" {
		t.Fatalf("serverInfo.version = %q, want 1.2.3", result.ServerInfo.Version)
	}
}

// Each server gets a directory of its own, so the resolver answers from an
// empty tree rather than from wherever the tests run.
func newDirectServer(t *testing.T, name, text string) (*server, string) {
	t.Helper()

	dir := t.TempDir()
	uri := pathToURI(filepath.Join(dir, name))

	return &server{
		encoding: encodingUTF8,
		docs:     map[string]string{uri: text},
	}, uri
}

func TestFormatDocumentUnknownExtensionNoEdits(t *testing.T) {
	s, uri := newDirectServer(t, "note.txt", "hello world")
	if edits := mustDocumentEdits(t, s, uri); len(edits) != 0 {
		t.Fatalf("unknown extension should yield no edits, got %+v", edits)
	}
}

func TestFormattingLogsAnUnparseableDocumentWithoutShowingIt(t *testing.T) {
	s, uri := newDirectServer(t, "broken.php", "<?php if (")

	var out bytes.Buffer
	s.conn = newConn(strings.NewReader(""), &out)

	if err := s.handleFormatting(formattingRequest(t, uri)); err != nil {
		t.Fatalf("handleFormatting: %v", err)
	}

	if edits := editsFromResponses(t, out.Bytes()); len(edits) != 0 {
		t.Fatalf("unparseable input must be a no-op, got %+v", edits)
	}

	if shown := messagesSent(t, out.Bytes(), "window/showMessage"); len(shown) != 0 {
		t.Fatalf("a document that does not parse was shown to the user: %+v", shown)
	}

	logged := messagesSent(t, out.Bytes(), "window/logMessage")

	if len(logged) != 1 || logged[0].Type != messageWarning || !strings.Contains(logged[0].Message, "broken.php") {
		t.Fatalf("logMessage = %+v, want one warning naming broken.php", logged)
	}
}

func TestFormattingShowsABrokenConfigOnceUntilADocumentFormats(t *testing.T) {
	const brokenConfig = `{"printWidht":40}`

	dir := t.TempDir()
	seedFiles(t, dir, map[string]string{
		"composer.json":    "{}",
		"laravel-fmt.json": brokenConfig,
	})

	uri := pathToURI(filepath.Join(dir, "app", "x.php"))
	s, out := newRecordingServer(map[string]string{uri: longCallSrc})

	formatEach(t, s, uri, uri)

	seedFiles(t, dir, map[string]string{"laravel-fmt.json": `{"printWidth":40}`})
	formatEach(t, s, uri)

	seedFiles(t, dir, map[string]string{"laravel-fmt.json": brokenConfig})
	formatEach(t, s, uri)

	if shown := messagesSent(t, out.Bytes(), "window/showMessage"); len(shown) != 2 {
		t.Fatalf("showMessage sent %d times, want once per break: %+v", len(shown), shown)
	}
}

func TestFormattingKeepsAShownFailureAcrossAnUntouchedDocument(t *testing.T) {
	dir := t.TempDir()
	seedFiles(t, dir, map[string]string{
		"composer.json":    "{}",
		"laravel-fmt.json": `{"printWidht":40}`,
	})

	uri := pathToURI(filepath.Join(dir, "app", "x.php"))
	const untitledURI = "untitled:Untitled-1"

	s, out := newRecordingServer(map[string]string{uri: longCallSrc, untitledURI: "<?php\n"})

	formatEach(t, s, uri, untitledURI, uri)

	if shown := messagesSent(t, out.Bytes(), "window/showMessage"); len(shown) != 1 {
		t.Fatalf("showMessage sent %d times, want 1: %+v", len(shown), shown)
	}
}

func TestFormattingShowsEachOfTwoBrokenConfigsOnce(t *testing.T) {
	base := t.TempDir()
	seedFiles(t, base, map[string]string{
		"a/composer.json":    "{}",
		"a/laravel-fmt.json": `{"printWidht":40}`,
		"b/composer.json":    "{}",
		"b/laravel-fmt.json": `{"printWidht":40}`,
	})

	a := pathToURI(filepath.Join(base, "a", "x.php"))
	b := pathToURI(filepath.Join(base, "b", "y.php"))

	s, out := newRecordingServer(map[string]string{a: longCallSrc, b: longCallSrc})

	formatEach(t, s, a, b, a, b)

	if shown := messagesSent(t, out.Bytes(), "window/showMessage"); len(shown) != 2 {
		t.Fatalf("showMessage sent %d times, want 2: %+v", len(shown), shown)
	}
}

func newRecordingServer(docs map[string]string) (*server, *bytes.Buffer) {
	var out bytes.Buffer

	return &server{
		conn:          newConn(strings.NewReader(""), &out),
		encoding:      encodingUTF8,
		docs:          docs,
		shownFailures: map[string]bool{},
	}, &out
}

func formatEach(t *testing.T, s *server, uris ...string) {
	t.Helper()

	for _, uri := range uris {
		if err := s.handleFormatting(formattingRequest(t, uri)); err != nil {
			t.Fatalf("handleFormatting %s: %v", uri, err)
		}
	}
}

func formattingRequest(t *testing.T, uri string) *request {
	t.Helper()

	params, err := json.Marshal(formattingParams{TextDocument: textDocumentIdentifier{URI: uri}})
	if err != nil {
		t.Fatal(err)
	}

	return &request{JSONRPC: "2.0", ID: json.RawMessage(formatRequestID), Method: "textDocument/formatting", Params: params}
}

func TestFormatDocumentUnknownURIIsUntouched(t *testing.T) {
	s, _ := newDirectServer(t, "open.php", "<?php\n")
	if _, err := s.documentEdits("file:///never-opened.php"); !errors.Is(err, errUntouched) {
		t.Fatalf("documentEdits = %v, want errUntouched", err)
	}
}

func TestFileURIBecomesAPathForTheTargetOS(t *testing.T) {
	cases := []struct {
		uri     string
		goos    string
		want    string
		wantErr bool
	}{
		{uri: "file:///C:/app/x.php", goos: "windows", want: `C:\app\x.php`},
		{uri: "file:///home/app/x.php", goos: "linux", want: "/home/app/x.php"},
		{uri: "file://localhost/C:/app/x.php", goos: "windows", want: `C:\app\x.php`},
		{uri: "file://localhost/home/app/x.php", goos: "linux", want: "/home/app/x.php"},
		{uri: "file://wsl$/Ubuntu/home/app/x.php", goos: "windows", want: `\\wsl$\Ubuntu\home\app\x.php`},
		{uri: "file://wsl.localhost/Ubuntu/home/app/x.php", goos: "windows", want: `\\wsl.localhost\Ubuntu\home\app\x.php`},
		{uri: "file://server/share/app/x.php", goos: "windows", want: `\\server\share\app\x.php`},
		{uri: "file://wsl%24/Ubuntu/x.php", goos: "windows", want: `\\wsl$\Ubuntu\x.php`},
		{uri: "file:////wsl$/Ubuntu/x.php", goos: "windows", want: `\\wsl$\Ubuntu\x.php`},
		{uri: "file://wsl$/Ubuntu/home/app/x.php", goos: "linux", wantErr: true},
		{uri: "file://wsl.localhost/Ubuntu/home/app/x.php", goos: "darwin", wantErr: true},
		{uri: "file://server/share/app/x.php", goos: "linux", wantErr: true},
		{uri: "file://wsl%24/Ubuntu/x.php", goos: "linux", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.goos+" "+tc.uri, func(t *testing.T) {
			got, err := uriToPathFor(tc.uri, tc.goos)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("uriToPathFor = %q, want an error", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("uriToPathFor: %v", err)
			}

			if got != tc.want {
				t.Fatalf("uriToPathFor = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormattingUsesTheProjectBehindALocalhostURI(t *testing.T) {
	dir := t.TempDir()
	seedFiles(t, dir, map[string]string{
		"composer.json":    "{}",
		"laravel-fmt.json": `{"printWidth":40}`,
	})

	hostlessURI := pathToURI(filepath.Join(dir, "app", "x.php"))
	uri := "file://localhost" + strings.TrimPrefix(hostlessURI, "file://")

	s := &server{
		encoding: encodingUTF8,
		docs:     map[string]string{uri: longCallSrc},
	}

	got := applyEditsUTF8(longCallSrc, mustDocumentEdits(t, s, uri))

	if !strings.Contains(got, "doSomething(\n") {
		t.Fatalf("the project's printWidth was not applied; the call stayed on one line:\n%s", got)
	}
}

func formatWholeDocument(t *testing.T, name, text string) string {
	t.Helper()

	s, uri := newDirectServer(t, name, text)

	return applyEditsUTF8(text, mustDocumentEdits(t, s, uri))
}

func TestRangeFormattingLeavesBufferOnPathToFullFormat(t *testing.T) {
	const name = "order.php"

	src := `<?php

namespace App;

use App\Models\Zebra;
use App\Models\Apple;
use App\Models\Mango;

class OrderController
{
    public function index()
    {
        return   Zebra::query( )->get();
    }
}
`

	want := formatWholeDocument(t, name, src)

	sorted := "use App\\Models\\Apple;\nuse App\\Models\\Mango;\nuse App\\Models\\Zebra;\n"
	if !strings.Contains(want, sorted) {
		t.Fatalf("whole-document formatting must sort the imports, got:\n%s", want)
	}

	s, uri := newDirectServer(t, name, src)
	lines := strings.Count(src, "\n") + 1

	for start := range lines {
		for end := start; end < lines; end++ {
			r := Range{Start: Position{start, 0}, End: Position{end, 0}}
			applied := applyEditsUTF8(src, mustRangeEdits(t, s, uri, r))

			if got := formatWholeDocument(t, name, applied); got != want {
				t.Fatalf("selection of lines %d-%d left the buffer at:\n%s\nwhich formats to:\n%s\nwant:\n%s", start, end, applied, got, want)
			}
		}
	}
}

func TestRangeFormattingNeverLeavesUnparseableSource(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"brace-less if", "<?php\n\nif ($x)\n    doThing();\n"},
		{"long array syntax", "<?php\n\n$a = array(\n    1,\n    2,\n);\n"},
	}

	const name = "r.php"

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, uri := newDirectServer(t, name, tc.src)
			lines := strings.Count(tc.src, "\n") + 1

			for start := range lines {
				for end := start; end < lines; end++ {
					r := Range{Start: Position{start, 0}, End: Position{end, 0}}
					applied := applyEditsUTF8(tc.src, mustRangeEdits(t, s, uri, r))

					if _, err := dispatch.File("/r.php", []byte(applied), config.Default()); err != nil {
						t.Fatalf("selecting lines %d-%d left source that does not parse:\n%s\n%v", start, end, applied, err)
					}
				}
			}
		})
	}
}

func TestFormattingUsesTheDocumentsOwnProject(t *testing.T) {
	workspace := t.TempDir()
	app := filepath.Join(workspace, "packages", "app")

	seedFiles(t, workspace, map[string]string{
		".prettierrc":                `{"printWidth":120,"plugins":["@prettier/plugin-php"]}`,
		"packages/app/composer.json": "{}",
		"packages/app/artisan":       "",
		"packages/app/.prettierrc":   `{"printWidth":40,"plugins":["@prettier/plugin-php"]}`,
	})

	uri := pathToURI(filepath.Join(app, "src", "x.php"))

	s := &server{
		encoding: encodingUTF8,
		docs:     map[string]string{uri: longCallSrc},
	}

	got := applyEditsUTF8(longCallSrc, mustDocumentEdits(t, s, uri))

	if !strings.Contains(got, "doSomething(\n") {
		t.Fatalf("the application's printWidth was not applied; the call stayed on one line:\n%s", got)
	}
}

func TestFormattingLeavesAnIgnoredDocumentUntouched(t *testing.T) {
	const unformattedSrc = "<?php\n$a=[1,2];\n"

	workspace := t.TempDir()

	seedFiles(t, workspace, map[string]string{
		"laravel-fmt.json": `{"ignore": ["generated/**"]}`,
	})

	ignoredURI := pathToURI(filepath.Join(workspace, "generated", "a.php"))
	keptURI := pathToURI(filepath.Join(workspace, "app", "a.php"))

	s := &server{
		encoding: encodingUTF8,
		docs:     map[string]string{ignoredURI: unformattedSrc, keptURI: unformattedSrc},
	}

	if _, err := s.documentEdits(ignoredURI); !errors.Is(err, errUntouched) {
		t.Fatalf("documentEdits on an ignored document = %v, want errUntouched", err)
	}

	if edits := mustDocumentEdits(t, s, keptURI); len(edits) == 0 {
		t.Fatal("a document outside the ignore got no edits")
	}
}

func TestFormattingFollowsProjectConfigChangedOnDisk(t *testing.T) {
	cases := []struct {
		name   string
		before map[string]string
		after  map[string]string
	}{
		{
			name:   "config edited",
			before: map[string]string{"laravel-fmt.json": `{"printWidth":120}`},
			after:  map[string]string{"laravel-fmt.json": `{"printWidth":40}`},
		},
		{
			name:  "config created",
			after: map[string]string{".editorconfig": "root = true\n[*]\nmax_line_length = 40\n"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seedFiles(t, dir, map[string]string{"composer.json": "{}"})
			seedFiles(t, dir, tc.before)

			uri := pathToURI(filepath.Join(dir, "app", "x.php"))

			s := &server{
				encoding: encodingUTF8,
				docs:     map[string]string{uri: longCallSrc},
			}

			if got := applyEditsUTF8(longCallSrc, mustDocumentEdits(t, s, uri)); strings.Contains(got, "doSomething(\n") {
				t.Fatalf("the call broke before the width changed:\n%s", got)
			}

			seedFiles(t, dir, tc.after)

			if got := applyEditsUTF8(longCallSrc, mustDocumentEdits(t, s, uri)); !strings.Contains(got, "doSomething(\n") {
				t.Fatalf("the new printWidth was not applied; the call stayed on one line:\n%s", got)
			}
		})
	}
}

func TestFormattingFollowsATailwindStylesheetChangedOnDisk(t *testing.T) {
	const src = `<div class="flex brand-x"></div>` + "\n"

	dir := t.TempDir()
	seedFiles(t, dir, map[string]string{
		"composer.json":         "{}",
		"resources/css/app.css": `@import "tailwindcss";` + "\n",
	})

	uri := pathToURI(filepath.Join(dir, "resources", "views", "x.blade.php"))

	s := &server{
		encoding: encodingUTF8,
		docs:     map[string]string{uri: src},
	}

	if got := applyEditsUTF8(src, mustDocumentEdits(t, s, uri)); !strings.Contains(got, `class="brand-x flex"`) {
		t.Fatalf("an undeclared class did not sort first:\n%s", got)
	}

	seedFiles(t, dir, map[string]string{
		"resources/css/app.css": "@import \"tailwindcss\";\n\n@utility brand-x {\n  color: red;\n}\n",
	})

	if got := applyEditsUTF8(src, mustDocumentEdits(t, s, uri)); !strings.Contains(got, `class="flex brand-x"`) {
		t.Fatalf("the utility the stylesheet now declares still sorted as unknown:\n%s", got)
	}
}

func TestFormattingReportsAnUnreadableConfig(t *testing.T) {
	cases := []struct {
		name    string
		request string
	}{
		{
			name:    "document",
			request: `{"jsonrpc":"2.0","id":2,"method":"textDocument/formatting","params":{"textDocument":{"uri":%q}}}`,
		},
		{
			name: "range",
			request: `{"jsonrpc":"2.0","id":2,"method":"textDocument/rangeFormatting","params":{"textDocument":{"uri":%q},` +
				`"range":{"start":{"line":0,"character":0},"end":{"line":3,"character":0}}}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seedFiles(t, dir, map[string]string{
				"composer.json":    "{}",
				"laravel-fmt.json": `{"printWidht":40}`,
			})

			docURI := pathToURI(filepath.Join(dir, "app", "x.php"))

			text, err := json.Marshal(longCallSrc)
			if err != nil {
				t.Fatal(err)
			}

			input := frame(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":%q}}`, pathToURI(dir))) +
				frame(fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"text":%s}}}`, docURI, text)) +
				frame(fmt.Sprintf(tc.request, docURI)) +
				frame(`{"jsonrpc":"2.0","id":3,"method":"shutdown"}`) +
				frame(`{"jsonrpc":"2.0","method":"exit"}`)

			var out bytes.Buffer
			if err := Serve(strings.NewReader(input), &out, io.Discard, ""); err != nil {
				t.Fatalf("Serve: %v", err)
			}

			if edits := editsFromResponses(t, out.Bytes()); len(edits) != 0 {
				t.Fatalf("a broken config produced edits: %+v", edits)
			}

			shown := messagesSent(t, out.Bytes(), "window/showMessage")

			if len(shown) != 1 || shown[0].Type != messageError {
				t.Fatalf("showMessage = %+v, want one error", shown)
			}

			if !strings.Contains(shown[0].Message, "laravel-fmt.json") {
				t.Fatalf("message %q does not name the file to fix", shown[0].Message)
			}
		})
	}
}

func TestFormattingUsesTheFilesOwnFolderInAMultiRootWorkspace(t *testing.T) {
	base := t.TempDir()
	backend := filepath.Join(base, "backend")
	scripts := filepath.Join(base, "scripts")

	seedFiles(t, base, map[string]string{
		"backend/composer.json": "{}",
		"backend/artisan":       "",
		"backend/.prettierrc":   `{"printWidth":200,"plugins":["@prettier/plugin-php"]}`,
		"scripts/.prettierrc":   `{"printWidth":40,"plugins":["@prettier/plugin-php"]}`,
	})

	docURI := pathToURI(filepath.Join(scripts, "x.php"))

	text, err := json.Marshal(longCallSrc)
	if err != nil {
		t.Fatal(err)
	}

	folders := fmt.Sprintf(`[{"uri":%q,"name":"backend"},{"uri":%q,"name":"scripts"}]`, pathToURI(backend), pathToURI(scripts))

	input := frame(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":%q,"workspaceFolders":%s}}`, pathToURI(backend), folders)) +
		frame(fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"text":%s}}}`, docURI, text)) +
		frame(fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"textDocument/formatting","params":{"textDocument":{"uri":%q}}}`, docURI)) +
		frame(`{"jsonrpc":"2.0","id":3,"method":"shutdown"}`) +
		frame(`{"jsonrpc":"2.0","method":"exit"}`)

	var out bytes.Buffer
	if err := Serve(strings.NewReader(input), &out, io.Discard, ""); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	edits := editsFromResponses(t, out.Bytes())

	if got := applyEditsUTF8(longCallSrc, edits); !strings.Contains(got, "doSomething(\n") {
		t.Fatalf("scripts/x.php was formatted with the first folder's printWidth, not its own:\n%s", got)
	}
}

func mustDocumentEdits(t *testing.T, s *server, uri string) []TextEdit {
	t.Helper()

	edits, err := s.documentEdits(uri)
	if err != nil {
		t.Fatalf("documentEdits: %v", err)
	}

	return edits
}

func mustRangeEdits(t *testing.T, s *server, uri string, r Range) []TextEdit {
	t.Helper()

	edits, err := s.rangeEdits(uri, r)
	if err != nil {
		t.Fatalf("rangeEdits: %v", err)
	}

	return edits
}

// longCallSrc fits the default width and breaks at a width of 40.
const longCallSrc = "<?php\n\n$result = doSomething($alpha, $beta, $gamma, $delta);\n"

func seedFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for rel, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))

		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

func TestFormattingKeepsTheBuffersLineEnding(t *testing.T) {
	cases := []struct {
		name      string
		endOfLine string
		src       string
	}{
		{name: "crlf buffer under lf", endOfLine: "lf", src: "<?php\r\n\r\n$a = 1;\r\n"},
		{name: "lf buffer under crlf", endOfLine: "crlf", src: "<?php\n\n$a = 1;\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workspace := t.TempDir()

			seedFiles(t, workspace, map[string]string{
				"laravel-fmt.json": `{"endOfLine": "` + tc.endOfLine + `"}`,
			})

			uri := pathToURI(filepath.Join(workspace, "a.php"))

			s := &server{encoding: encodingUTF8, docs: map[string]string{uri: tc.src}}

			if edits := mustDocumentEdits(t, s, uri); len(edits) != 0 {
				t.Fatalf("a formatted buffer got edits for its line ending: %+v", edits)
			}
		})
	}
}

func TestEndOfLineReportsTheProjectsLineEnding(t *testing.T) {
	workspace := t.TempDir()

	seedFiles(t, workspace, map[string]string{
		".editorconfig":    "root = true\n[*.php]\nend_of_line = crlf\n",
		"laravel-fmt.json": `{"ignore": ["generated/**"]}`,
	})

	cases := []struct {
		name string
		file string
		want string
	}{
		{name: "editorconfig section", file: "app/a.php", want: "crlf"},
		{name: "ignored file", file: "generated/a.php", want: "auto"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uri := pathToURI(filepath.Join(workspace, filepath.FromSlash(tc.file)))
			s, out := newRecordingServer(map[string]string{})

			params, err := json.Marshal(endOfLineParams{TextDocument: textDocumentIdentifier{URI: uri}})
			if err != nil {
				t.Fatal(err)
			}

			req := &request{JSONRPC: "2.0", ID: json.RawMessage(formatRequestID), Method: "laravel-fmt/endOfLine", Params: params}

			if _, err := s.handle(req); err != nil {
				t.Fatalf("handle: %v", err)
			}

			var got string
			if err := json.Unmarshal(responseTo(t, out.Bytes(), formatRequestID).Result, &got); err != nil {
				t.Fatalf("decode result: %v", err)
			}

			if got != tc.want {
				t.Fatalf("endOfLine = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDidCloseForgetsTheDocument(t *testing.T) {
	dir := t.TempDir()
	docURI := pathToURI(filepath.Join(dir, "sample.php"))

	text, err := json.Marshal("<?php\n\necho   'hello'  ;\n")
	if err != nil {
		t.Fatal(err)
	}

	input := frame(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":%q}}`, pathToURI(dir))) +
		frame(fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"text":%s}}}`, docURI, text)) +
		frame(fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didClose","params":{"textDocument":{"uri":%q}}}`, docURI)) +
		frame(fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"textDocument/formatting","params":{"textDocument":{"uri":%q}}}`, docURI)) +
		frame(`{"jsonrpc":"2.0","id":3,"method":"shutdown"}`) +
		frame(`{"jsonrpc":"2.0","method":"exit"}`)

	var out bytes.Buffer
	if err := Serve(strings.NewReader(input), &out, io.Discard, ""); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	if edits := editsFromResponses(t, out.Bytes()); len(edits) != 0 {
		t.Fatalf("a closed document was formatted from its stale buffer: %+v", edits)
	}
}

func TestUnknownMethodRepliesOnlyToARequest(t *testing.T) {
	input := frame(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`) +
		frame(`{"jsonrpc":"2.0","method":"$/unknownNotification"}`) +
		frame(`{"jsonrpc":"2.0","id":2,"method":"workspace/unknownRequest"}`) +
		frame(`{"jsonrpc":"2.0","id":3,"method":"shutdown"}`) +
		frame(`{"jsonrpc":"2.0","method":"exit"}`)

	var out bytes.Buffer
	if err := Serve(strings.NewReader(input), &out, io.Discard, ""); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	if resp := responseTo(t, out.Bytes(), "2"); resp.Error == nil || resp.Error.Code != codeMethodNotFound {
		t.Fatalf("unknown request got %+v, want error code %d", resp.Error, codeMethodNotFound)
	}

	r := bufio.NewReader(bytes.NewReader(out.Bytes()))

	for {
		body, err := readFrame(r)
		if errors.Is(err, io.EOF) {
			return
		}

		if err != nil {
			t.Fatalf("read frame: %v", err)
		}

		var resp wireResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		if resp.Error != nil && string(resp.ID) != "2" {
			t.Fatalf("a notification was answered: %s", body)
		}
	}
}
