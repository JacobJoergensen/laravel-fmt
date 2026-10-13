package lsp

// Position encodings laravel-fmt negotiates. UTF-16 is the LSP default; UTF-8
// is offered by newer clients and lets edit columns equal byte offsets.
const (
	encodingUTF16 = "utf-16"
	encodingUTF8  = "utf-8"
)

// Full document sync: each change carries the whole new buffer. A formatter
// only needs the buffer at format time, so incremental sync would add state
// for no gain.
const syncFull = 1

// Position is a zero-based line and character offset; the character unit is the
// negotiated position encoding.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a half-open span between two positions.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// TextEdit replaces the text in Range with NewText.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

type initializeParams struct {
	Capabilities struct {
		General struct {
			PositionEncodings []string `json:"positionEncodings"`
		} `json:"general"`
	} `json:"capabilities"`
}

type initializeResult struct {
	Capabilities serverCapabilities `json:"capabilities"`
	ServerInfo   serverInfo         `json:"serverInfo"`
}

type serverCapabilities struct {
	PositionEncoding                string                  `json:"positionEncoding"`
	TextDocumentSync                textDocumentSyncOptions `json:"textDocumentSync"`
	DocumentFormattingProvider      bool                    `json:"documentFormattingProvider"`
	DocumentRangeFormattingProvider bool                    `json:"documentRangeFormattingProvider"`
}

type textDocumentSyncOptions struct {
	OpenClose bool `json:"openClose"`
	Change    int  `json:"change"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

type didOpenParams struct {
	TextDocument struct {
		URI  string `json:"uri"`
		Text string `json:"text"`
	} `json:"textDocument"`
}

type didChangeParams struct {
	TextDocument   textDocumentIdentifier `json:"textDocument"`
	ContentChanges []struct {
		// Set only by an incremental client: a range means Text is a fragment.
		Range *Range `json:"range"`
		Text  string `json:"text"`
	} `json:"contentChanges"`
}

type didCloseParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type formattingParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type rangeFormattingParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
}
