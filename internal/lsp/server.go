// Package lsp serves laravel-fmt's formatter over the Language Server
// Protocol on stdio, resolving config per file exactly as the CLI does.
package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"runtime"
	"slices"
	"strings"

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

	// Format on save asks on every save, so each failure is shown once, and
	// again only after a document formats.
	shownFailures map[string]bool

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

		shownFailures: map[string]bool{},
	}

	for {
		req, err := s.conn.read()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if errors.Is(err, errParse) || errors.Is(err, errInvalidRequest) {
			if err := s.replyMalformed(req, err); err != nil {
				return err
			}

			continue
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

func (s *server) replyMalformed(req *request, readErr error) error {
	s.logf("read message: %v", readErr)

	code := codeInvalidRequest
	if errors.Is(readErr, errParse) {
		code = codeParseError
	}

	id := json.RawMessage("null")
	if req != nil && !req.isNotification() {
		id = req.ID
	}

	return s.conn.replyError(id, code, readErr.Error())
}

// exit is a notification, but one sent with an id still ends the server.
func (s *server) handle(req *request) (bool, error) {
	if req.Method == "exit" {
		if s.shutdownRequested {
			return true, nil
		}

		return false, ErrExitWithoutShutdown
	}

	if req.isNotification() {
		s.handleNotification(req)

		return false, nil
	}

	return false, s.handleRequest(req)
}

func (s *server) handleNotification(req *request) {
	if s.shutdownRequested {
		return
	}

	switch req.Method {
	case "textDocument/didOpen":
		s.handleDidOpen(req)
	case "textDocument/didChange":
		s.handleDidChange(req)
	case "textDocument/didClose":
		s.handleDidClose(req)
	}
}

func (s *server) handleRequest(req *request) error {
	if s.shutdownRequested {
		return s.conn.replyError(req.ID, codeInvalidRequest, "server is shutting down")
	}

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "shutdown":
		s.shutdownRequested = true

		// JSON-RPC requires result on success; shutdown's is null, so send it
		// explicitly rather than letting omitempty drop the field.
		return s.conn.reply(req.ID, json.RawMessage("null"))
	case "textDocument/formatting":
		return s.handleFormatting(req)
	case "textDocument/rangeFormatting":
		return s.handleRangeFormatting(req)
	case "laravel-fmt/endOfLine":
		return s.handleEndOfLine(req)
	case "laravel-fmt/format":
		return s.handleFormat(req)
	default:
		return s.conn.replyError(req.ID, codeMethodNotFound, "method not supported: "+req.Method)
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
			Experimental:                    experimentalCapabilities{LaravelFmtFormat: true},
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

	return s.replyEdits(req.ID, edits, err)
}

func (s *server) handleRangeFormatting(req *request) error {
	var p rangeFormattingParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return s.conn.reply(req.ID, []TextEdit{})
	}

	edits, err := s.rangeEdits(p.TextDocument.URI, p.Range)

	return s.replyEdits(req.ID, edits, err)
}

// The answer is the configured value for a document the server formats.
// Anything else is auto, which means the client keeps the line ending it has:
// an empty edit list cannot tell a refused or unheld document from one that
// was already formatted.
func (s *server) handleEndOfLine(req *request) error {
	var p endOfLineParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return s.conn.reply(req.ID, config.EndOfLineAuto)
	}

	res, err := s.format(p.TextDocument.URI, scopeDocument)
	if err != nil {
		if !errors.Is(err, errUntouched) && !errors.Is(err, errUnparseable) {
			s.logf("endOfLine %s: %v", p.TextDocument.URI, err)
		}

		return s.conn.reply(req.ID, config.EndOfLineAuto)
	}

	return s.conn.reply(req.ID, res.endOfLine)
}

func (s *server) handleFormat(req *request) error {
	// The request is laravel-fmt's own, so a malformed one is an error rather
	// than an empty edit list that reads as an already formatted document.
	var p formatParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return s.conn.replyError(req.ID, codeInvalidParams, "laravel-fmt/format: "+err.Error())
	}

	if p.Text == nil {
		return s.conn.replyError(req.ID, codeInvalidParams, "laravel-fmt/format: missing text")
	}

	scope := scopeDocument
	if p.Range != nil {
		scope = scopeRange
	}

	res, err := s.formatText(p.TextDocument.URI, *p.Text, scope)
	if err != nil {
		untouched := formatReply{Edits: []TextEdit{}, EndOfLine: config.EndOfLineAuto}

		return s.replyFailure(req.ID, untouched, err)
	}

	var edits []TextEdit
	if p.Range != nil {
		edits = s.selectRange(res, *p.Range)
	} else {
		edits = buildEdits(res.src, res.out, s.encoding)
	}

	clear(s.shownFailures)

	return s.conn.reply(req.ID, formatReply{Edits: edits, EndOfLine: res.endOfLine})
}

func (s *server) replyEdits(id json.RawMessage, edits []TextEdit, formatErr error) error {
	if formatErr != nil {
		return s.replyFailure(id, []TextEdit{}, formatErr)
	}

	clear(s.shownFailures)

	return s.conn.reply(id, edits)
}

// An editor discards or merely logs an error reply to a formatting request,
// so a failure the user has to fix is shown as a message instead. A document
// that does not parse is usually mid-edit and already marked by the editor,
// so it only goes to the server log.
func (s *server) replyFailure(id json.RawMessage, untouched any, formatErr error) error {
	switch {
	case errors.Is(formatErr, errUntouched):
	case errors.Is(formatErr, errUnparseable):
		if err := s.conn.notify("window/logMessage", messageParams{Type: messageWarning, Message: formatErr.Error()}); err != nil {
			return err
		}
	case !s.shownFailures[formatErr.Error()]:
		s.shownFailures[formatErr.Error()] = true

		if err := s.conn.notify("window/showMessage", messageParams{Type: messageError, Message: formatErr.Error()}); err != nil {
			return err
		}
	}

	return s.conn.reply(id, untouched)
}

func (s *server) documentEdits(uri string) ([]TextEdit, error) {
	res, err := s.format(uri, scopeDocument)
	if err != nil {
		return nil, err
	}

	return buildEdits(res.src, res.out, s.encoding), nil
}

// The selection is resolved against the formatted document, not filtered out
// of its edits: half a change can leave source that does not parse. See
// diff.ApplyInRange.
func (s *server) rangeEdits(uri string, r Range) ([]TextEdit, error) {
	res, err := s.format(uri, scopeRange)
	if err != nil {
		return nil, err
	}

	return s.selectRange(res, r), nil
}

func (s *server) selectRange(res *formatResult, r Range) []TextEdit {
	lineStart := lspLineStarts(res.src)
	start := positionToByte(res.src, lineStart, r.Start, s.encoding)
	end := positionToByte(res.src, lineStart, r.End, s.encoding)

	parses := func(b []byte) bool {
		_, err := dispatch.File(res.path, b, &res.cfg)

		return err == nil
	}

	ranged := diff.ApplyInRange(res.src, res.out, start, end, parses)

	return buildEdits(res.src, ranged, s.encoding)
}

// The range handler re-formats candidate splices, so the result keeps the
// path and config, not just the output. endOfLine is the configured value,
// before formatting resolves cfg's to the buffer's own.
type formatResult struct {
	path      string
	cfg       config.File
	endOfLine string
	src       []byte
	out       []byte
}

var (
	errUntouched   = errors.New("document left untouched")
	errUnparseable = errors.New("cannot format")
)

func (s *server) format(uri string, scope formatScope) (*formatResult, error) {
	text, ok := s.docs[uri]
	if !ok {
		return nil, errUntouched
	}

	return s.formatText(uri, text, scope)
}

// A document the server cannot map to a file or ignores is errUntouched. One
// that does not parse is errUnparseable: a formatter never emits recovery
// output.
func (s *server) formatText(uri, text string, scope formatScope) (*formatResult, error) {
	path, cfg, err := s.fileConfig(uri)
	if err != nil {
		return nil, err
	}

	if scope == scopeRange {
		config.DisableRelocatingPasses(&cfg)
	}

	// No client changes a line ending through a TextEdit: VS Code, JetBrains
	// and Neovim all rewrite the new text to the document's own. Converting
	// here only replaces every line on every save; handleEndOfLine tells the
	// client instead.
	src := []byte(text)
	configured := cfg.EndOfLine
	cfg.EndOfLine = config.EndOfLineAuto
	config.ResolveEndOfLine(src, &cfg)

	out, err := dispatch.File(path, src, &cfg)
	if err != nil {
		return nil, fmt.Errorf("%w %s: %w", errUnparseable, path, err)
	}

	return &formatResult{path: path, cfg: cfg, endOfLine: configured, src: src, out: out}, nil
}

func (s *server) fileConfig(uri string) (string, config.File, error) {
	path, err := uriToPath(uri)
	if err != nil {
		s.logf("uri %s: %v", uri, err)

		return "", config.File{}, errUntouched
	}

	// Resolved as --stdin-filepath resolves it, from the file's own directory,
	// and afresh per request: a cache would have to track every file detection
	// reads, and a stale one formats differently from the CLI.
	project, err := detector.NewResolver(detector.LoadConfig).For(path)
	if err != nil {
		return "", config.File{}, err
	}

	// Format on save reaches every file an editor opens, vendor code and
	// lockfiles included, so the project's ignore rules apply as in a run.
	ignored, err := walk.IgnoresFile(project.IgnoreAnchor(), &project.Cfg, path)
	if err != nil {
		return "", config.File{}, err
	}

	if ignored {
		return "", config.File{}, errUntouched
	}

	cfg, err := detector.ResolveFileConfig(&project.Cfg, path, project.Ctx, project.UserKeys)
	if err != nil {
		return "", config.File{}, err
	}

	return path, cfg, nil
}

func (s *server) logf(format string, args ...any) {
	if s.log == nil {
		return
	}

	_, _ = fmt.Fprintf(s.log, "laravel-fmt lsp: "+format+"\n", args...)
}

func uriToPath(uri string) (string, error) {
	return uriToPathFor(uri, runtime.GOOS)
}

// RFC 8089: localhost is the same as no host. Go rejects a percent-escaped
// host like wsl%24, the form VS Code sends for \\wsl$, so the host is cut off
// and decoded before parsing.
// file:///C:/x decodes to /C:/x, so a slash before a drive letter is dropped.
func uriToPathFor(uri, goos string) (string, error) {
	var host string

	if rest, ok := strings.CutPrefix(uri, "file://"); ok {
		rawHost, hostless, _ := strings.Cut(rest, "/")

		decoded, err := url.PathUnescape(rawHost)
		if err != nil {
			return "", fmt.Errorf("parse uri host: %w", err)
		}

		host = decoded
		uri = "file:///" + hostless
	}

	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("parse uri: %w", err)
	}

	if u.Scheme != "file" {
		return "", fmt.Errorf("unsupported uri scheme %q", u.Scheme)
	}

	path := u.Path
	windows := goos == "windows"

	if host != "" && !strings.EqualFold(host, "localhost") {
		if !windows {
			return "", fmt.Errorf("uri host %q names a network share, which only Windows can open", host)
		}

		path = "//" + host + path
	} else if windows && isDrivePath(path) {
		path = path[1:]
	}

	if windows {
		path = strings.ReplaceAll(path, "/", `\`)
	}

	return path, nil
}

func isDrivePath(path string) bool {
	if len(path) < 3 || path[0] != '/' || path[2] != ':' {
		return false
	}

	letter := path[1]

	return ('a' <= letter && letter <= 'z') || ('A' <= letter && letter <= 'Z')
}
