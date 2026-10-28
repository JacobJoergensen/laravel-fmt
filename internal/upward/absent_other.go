//go:build !windows

package upward

import (
	"errors"
	"os"
)

// Absent reports whether err means no file is there.
func Absent(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
