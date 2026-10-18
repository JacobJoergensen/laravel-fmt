package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	codeMethodNotFound = -32601
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

	body := make([]byte, length)
	if _, err := io.ReadFull(c.r, body); err != nil {
		return nil, fmt.Errorf("read message body: %w", err)
	}

	var req request
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("decode message: %w", err)
	}

	return &req, nil
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
