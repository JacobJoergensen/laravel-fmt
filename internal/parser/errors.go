package parser

import "fmt"

// Error is a position-aware lex or parse error.
type Error struct {
	Pos Position
	Msg string
}

// Error formats the error as line:column: message.
func (e Error) Error() string {
	return fmt.Sprintf("%d:%d: %s", e.Pos.Line, e.Pos.Column, e.Msg)
}

// CollectErrors returns every Error carried by err, in report order.
func CollectErrors(err error) []Error {
	var out []Error

	var visit func(error)
	visit = func(e error) {
		// errors.As is avoided on purpose: it searches the whole tree and stops
		// at the first Error, which would drop the sibling diagnostics an
		// errors.Join carries.
		switch e := e.(type) { //nolint:errorlint // intentional per-node assertion, not a chain search
		case nil:
			return
		case Error:
			out = append(out, e)
		case interface{ Unwrap() []error }:
			for _, sub := range e.Unwrap() {
				visit(sub)
			}
		case interface{ Unwrap() error }:
			visit(e.Unwrap())
		}
	}

	visit(err)

	return out
}
