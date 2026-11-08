package lsp

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
)

func frame(body string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
}

func TestConnReadParsesFramedMessage(t *testing.T) {
	in := frame(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":"file:///x"}}`)
	c := newConn(strings.NewReader(in), &bytes.Buffer{})

	req, err := c.read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if req.Method != "initialize" {
		t.Fatalf("method = %q, want initialize", req.Method)
	}

	if req.isNotification() {
		t.Fatal("request with id must not be a notification")
	}
}

func TestConnReadNotificationHasNoID(t *testing.T) {
	in := frame(`{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	c := newConn(strings.NewReader(in), &bytes.Buffer{})

	req, err := c.read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if !req.isNotification() {
		t.Fatal("message without id must be a notification")
	}
}

func TestConnReadDoesNotAllocateABodyThatNeverArrives(t *testing.T) {
	const declared = 256 << 20
	const allowed = 16 << 20

	in := fmt.Sprintf("Content-Length: %d\r\n\r\n{}", declared)
	c := newConn(strings.NewReader(in), &bytes.Buffer{})

	var before, after runtime.MemStats

	runtime.ReadMemStats(&before)
	_, err := c.read()
	runtime.ReadMemStats(&after)

	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("read = %v, want io.ErrUnexpectedEOF", err)
	}

	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > allowed {
		t.Fatalf("read allocated %d bytes for a 2-byte body, want at most %d", allocated, allowed)
	}
}

func TestConnReplyIsFramedAndReadable(t *testing.T) {
	var out bytes.Buffer
	c := newConn(strings.NewReader(""), &out)

	if err := c.reply(jsontext.Value("7"), []TextEdit{{NewText: "x"}}); err != nil {
		t.Fatalf("reply: %v", err)
	}

	back := newConn(bytes.NewReader(out.Bytes()), &bytes.Buffer{})

	req, err := back.read()
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	if string(req.ID) != "7" {
		t.Fatalf("id = %s, want 7", req.ID)
	}
}
