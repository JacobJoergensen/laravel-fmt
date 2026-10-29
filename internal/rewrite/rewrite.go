// Package rewrite replaces a file's contents in place.
package rewrite

import (
	"errors"
	"fmt"
	"os"
)

// file is what a rewrite needs from an open file. *os.File satisfies it, and a
// test substitutes one whose write fails partway.
type file interface {
	WriteAt(b []byte, off int64) (int, error)
	Truncate(size int64) error
	Close() error
}

// Rewrite replaces the contents of the file at path, currently orig, with
// formatted.
//
// It writes over the file and then trims it, as gofmt does: truncating first
// would leave the file empty if a full disk failed the write. If anything fails
// once bytes may have been written, orig goes back. The file stays the same
// file: a temporary file renamed over it would break hard links, drop its
// owner, and fail on Windows while an editor holds it open.
func Rewrite(path string, orig, formatted []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}

	return rewrite(f, orig, formatted)
}

func rewrite(f file, orig, formatted []byte) error {
	if err := replace(f, formatted); err != nil {
		if restoreErr := replace(f, orig); restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("restore original: %w", restoreErr))
		}

		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close: %w", closeErr))
		}

		return err
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	return nil
}

func replace(f file, data []byte) error {
	if _, err := f.WriteAt(data, 0); err != nil {
		return fmt.Errorf("write contents: %w", err)
	}

	if err := f.Truncate(int64(len(data))); err != nil {
		return fmt.Errorf("trim to the new length: %w", err)
	}

	return nil
}
