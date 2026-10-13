// Package lsp serves laravel-fmt's formatter over the Language Server
// Protocol on stdio, resolving config per file exactly as the CLI does.
package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"slices"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/detector"
	"github.com/JacobJoergensen/laravel-fmt/internal/diff"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/dispatch"
	"github.com/JacobJoergensen/laravel-fmt/internal/walk"
)

// ErrExitWithoutShutdown reports that the client sent `exit` before
// `shutdown`; the LSP spec requires the server to terminate with a non-zero
// status in that case.
var ErrExitWithoutShutdown = errors.New("lsp: exit without prior shutdown")

// CONTRACT: handle runs one request at a time on Serve's goroutine, so docs
// and shutdownRequested are unsynchronized on purpose. Handling requests
// concurrently needs a lock.
type server struct {
	conn *conn
	log  io.Writer

	encoding string
	version  string

	docs map[string]string

	shutdownRequested bool
}

// Serve answers JSON-RPC from in on out, logging to logw. It returns nil on
// shutdown or EOF, and ErrExitWithoutShutdown when the client exits first.
func Serve(in io.Reader, out, logw io.Writer, version string) error {
	s := &server{
		conn:     newConn(in, out),
		log:      logw,
		encoding: encodingUTF16,
		version:  version,
		docs:     map[string]string{},
	}

	for {
		req, err := s.conn.read()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return fmt.Errorf("read message: %w", err)
		}

		stop, err := s.handle(req)
		if err != nil {
			return err
		}

		if stop {
			return nil
		}
	}
}

func (s *server) handle(req *request) (bool, error) {
	switch req.Method {
	case "initialize":
		return false, s.handleInitialize(req)
	case "shutdown":
		s.shutdownRequested = true

		// JSON-RPC requires result on success; shutdown's is null, so send it
		// explicitly rather than letting omitempty drop the field.
		return false, s.conn.reply(req.ID, json.RawMessage("null"))
	case "exit":
		if s.shutdownRequested {
			return true, nil
		}

		return false, ErrExitWithoutShutdown
	case "textDocument/didOpen":
		s.handleDidOpen(req)

		return false, nil
	case "textDocument/didChange":
		s.handleDidChange(req)

		return false, nil
	case "textDocument/didClose":
		s.handleDidClose(req)

		return false, nil
	case "textDocument/formatting":
		return false, s.handleFormatting(req)
	case "textDocument/rangeFormatting":
		return false, s.handleRangeFormatting(req)
	default:
		if req.isNotification() {
			return false, nil
		}

		return false, s.conn.replyError(req.ID, codeMethodNotFound, "method not supported: "+req.Method)
	}
}

func (s *server) handleInitialize(req *request) error {
	var p initializeParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		s.logf("initialize: decode params: %v", err)
	}

	if slices.Contains(p.Capabilities.General.PositionEncodings, encodingUTF8) {
		s.encoding = encodingUTF8
	}

	result := initializeResult{
		Capabilities: serverCapabilities{
			PositionEncoding:                s.encoding,
			TextDocumentSync:                textDocumentSyncOptions{OpenClose: true, Change: syncFull},
			DocumentFormattingProvider:      true,
			DocumentRangeFormattingProvider: true,
		},
		ServerInfo: serverInfo{Name: "laravel-fmt", Version: s.version},
	}

	return s.conn.reply(req.ID, result)
}

func (s *server) handleDidOpen(req *request) {
	var p didOpenParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		s.logf("didOpen: decode params: %v", err)

		return
	}

	s.docs[p.TextDocument.URI] = p.TextDocument.Text
}

// Full sync sends the whole buffer in the last content change, so only that
// entry is kept. A change carrying a range is a fragment from an incremental
// client: the buffer cannot be rebuilt from it, so the document is forgotten
// rather than formatted from text the server does not have.
func (s *server) handleDidChange(req *request) {
	var p didChangeParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		s.logf("didChange: decode params: %v", err)

		return
	}

	if len(p.ContentChanges) == 0 {
		return
	}

	last := p.ContentChanges[len(p.ContentChanges)-1]

	if last.Range != nil {
		s.logf("didChange %s: ignoring an incremental change; the server negotiated full sync", p.TextDocument.URI)
		delete(s.docs, p.TextDocument.URI)

		return
	}

	s.docs[p.TextDocument.URI] = last.Text
}

func (s *server) handleDidClose(req *request) {
	var p didCloseParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		s.logf("didClose: decode params: %v", err)

		return
	}

	delete(s.docs, p.TextDocument.URI)
}

// scopeRange leaves out the passes that move code; see
// config.DisableRelocatingPasses.
type formatScope int

const (
	scopeDocument formatScope = iota
	scopeRange
)

func (s *server) handleFormatting(req *request) error {
	var p formattingParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return s.conn.reply(req.ID, []TextEdit{})
	}

	edits, err := s.documentEdits(p.TextDocument.URI)
	if err != nil {
		return s.conn.replyError(req.ID, codeRequestFailed, err.Error())
	}

	return s.conn.reply(req.ID, edits)
}

func (s *server) handleRangeFormatting(req *request) error {
	var p rangeFormattingParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return s.conn.reply(req.ID, []TextEdit{})
	}

	edits, err := s.rangeEdits(p.TextDocument.URI, p.Range)
	if err != nil {
		return s.conn.replyError(req.ID, codeRequestFailed, err.Error())
	}

	return s.conn.reply(req.ID, edits)
}

func (s *server) documentEdits(uri string) ([]TextEdit, error) {
	res, err := s.format(uri, scopeDocument)
	if err != nil {
		return refusal(err)
	}

	return buildEdits(res.src, res.out, s.encoding), nil
}

// refusal is the reply for a document format would not format: no edits when
// it is left as it is on purpose, the error when the user has something to fix.
func refusal(err error) ([]TextEdit, error) {
	if errors.Is(err, errUntouched) {
		return []TextEdit{}, nil
	}

	return nil, err
}

// The selection is resolved against the formatted document, not filtered out
// of its edits: half a change can leave source that does not parse. See
// diff.ApplyInRange.
func (s *server) rangeEdits(uri string, r Range) ([]TextEdit, error) {
	res, err := s.format(uri, scopeRange)
	if err != nil {
		return refusal(err)
	}

	lineStart := lspLineStarts(res.src)
	start := positionToByte(res.src, lineStart, r.Start, s.encoding)
	end := positionToByte(res.src, lineStart, r.End, s.encoding)

	parses := func(b []byte) bool {
		_, err := dispatch.File(res.path, b, &res.cfg)

		return err == nil
	}

	ranged := diff.ApplyInRange(res.src, res.out, start, end, parses)

	return buildEdits(res.src, ranged, s.encoding), nil
}

// The range handler re-formats candidate splices, so the result keeps the
// path and config, not just the output.
type formatResult struct {
	path string
	cfg  config.File
	src  []byte
	out  []byte
}

// errUntouched marks a document format leaves as it is on purpose.
var errUntouched = errors.New("document left untouched")

// A buffer the server does not hold, cannot map to a file or cannot parse is
// errUntouched: a formatter never emits recovery output. An unreadable config
// is returned as the error, since an empty reply would pass for a formatted
// file.
func (s *server) format(uri string, scope formatScope) (*formatResult, error) {
	text, ok := s.docs[uri]
	if !ok {
		return nil, errUntouched
	}

	path, err := uriToPath(uri)
	if err != nil {
		s.logf("uri %s: %v", uri, err)

		return nil, errUntouched
	}

	// Resolved as --stdin-filepath resolves it, from the file's own directory,
	// and afresh per request: a cache would have to track every file detection
	// reads, and a stale one formats differently from the CLI.
	project, err := detector.NewResolver(detector.LoadConfig).For(path)
	if err != nil {
		return nil, err
	}

	// Format on save reaches every file an editor opens, vendor code and
	// lockfiles included, so the project's ignore rules apply as in a run.
	ignored, err := walk.IgnoresFile(project.IgnoreAnchor(), &project.Cfg, path)
	if err != nil {
		return nil, err
	}

	if ignored {
		return nil, errUntouched
	}

	cfg, err := detector.ResolveFileConfig(&project.Cfg, path, project.Ctx, project.UserKeys)
	if err != nil {
		return nil, err
	}

	if scope == scopeRange {
		config.DisableRelocatingPasses(&cfg)
	}

	src := []byte(text)
	config.ResolveEndOfLine(src, &cfg)

	out, err := dispatch.File(path, src, &cfg)
	if err != nil {
		s.logf("format %s: %v", path, err)

		return nil, errUntouched
	}

	return &formatResult{path: path, cfg: cfg, src: src, out: out}, nil
}

func (s *server) logf(format string, args ...any) {
	if s.log == nil {
		return
	}

	_, _ = fmt.Fprintf(s.log, "laravel-fmt lsp: "+format+"\n", args...)
}

// file:///C:/x decodes to /C:/x, so a slash before a drive letter is dropped.
func uriToPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("parse uri: %w", err)
	}

	if u.Scheme != "file" {
		return "", fmt.Errorf("unsupported uri scheme %q", u.Scheme)
	}

	path := u.Path

	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}

	return filepath.FromSlash(path), nil
}
