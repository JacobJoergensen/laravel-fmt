package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// conn frames JSON-RPC 2.0 messages over a reader/writer pair using the LSP
// `Content-Length` header convention.
type conn struct {
	r *bufio.Reader
	w io.Writer
}

func newConn(r io.Reader, w io.Writer) *conn {
	return &conn{r: bufio.NewReader(r), w: w}
}

// request is one incoming JSON-RPC message. A missing id marks a notification,
// for which no response is written.
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (r *request) isNotification() bool {
	return len(r.ID) == 0
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *responseError  `json:"error,omitempty"`
}

type notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	jsonrpcVersion = "2.0"

	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602

	maxPreallocatedBody = 1 << 20
)

// A message whose body fails to decode was still framed, so the stream stays
// in sync and the server answers it instead of exiting.
var (
	errParse          = errors.New("message is not valid JSON")
	errInvalidRequest = errors.New("message is not a request")
)

// read returns the next framed message; at end of input the error wraps io.EOF.
func (c *conn) read() (*request, error) {
	length := -1

	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read header: %w", err)
		}

		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			break
		}

		name, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}

		if strings.EqualFold(strings.TrimSpace(name), "content-length") {
			length, err = strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, fmt.Errorf("parse content-length: %w", err)
			}
		}
	}

	if length < 0 {
		return nil, errors.New("message missing content-length header")
	}

	body, err := c.readBody(length)
	if err != nil {
		return nil, fmt.Errorf("read message body: %w", err)
	}

	if !json.Valid(body) {
		return nil, errParse
	}

	// JSON-RPC answers an invalid request with its id when the id can be read,
	// so the request returned alongside the error keeps it.
	var req request
	if err := json.Unmarshal(body, &req); err != nil {
		var idOnly struct {
			ID json.RawMessage `json:"id"`
		}

		_ = json.Unmarshal(body, &idOnly)

		return &request{ID: idOnly.ID}, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	return &req, nil
}

// A corrupted header can declare any length, so the body is read in chunks
// and memory grows only with the bytes that arrive.
func (c *conn) readBody(length int) ([]byte, error) {
	body := make([]byte, 0, min(length, maxPreallocatedBody))

	for len(body) < length {
		chunk := min(length-len(body), maxPreallocatedBody)
		body = slices.Grow(body, chunk)

		n, err := io.ReadFull(c.r, body[len(body):len(body)+chunk])
		body = body[:len(body)+n]

		if err != nil {
			return nil, fmt.Errorf("got %d of %d bytes: %w", len(body), length, err)
		}
	}

	return body, nil
}

func (c *conn) reply(id json.RawMessage, result any) error {
	return c.write(response{JSONRPC: jsonrpcVersion, ID: id, Result: result})
}

func (c *conn) replyError(id json.RawMessage, code int, message string) error {
	return c.write(response{JSONRPC: jsonrpcVersion, ID: id, Error: &responseError{Code: code, Message: message}})
}

func (c *conn) notify(method string, params any) error {
	return c.write(notification{JSONRPC: jsonrpcVersion, Method: method, Params: params})
}

func (c *conn) write(msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("encode message: %w", err)
	}

	if _, err := fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	if _, err := c.w.Write(body); err != nil {
		return fmt.Errorf("write body: %w", err)
	}

	return nil
}
