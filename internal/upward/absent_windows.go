//go:build windows

package upward

import (
	"errors"
	"os"
	"syscall"
)

// ERROR_INVALID_NAME, which syscall does not export.
const errInvalidName = syscall.Errno(0x7B)

// Absent reports whether err means no file is there. Windows answers a name it
// cannot represent with ERROR_INVALID_NAME rather than not-exist, and such a
// path cannot hold a file. Any other error is a real failure: the answer is
// unknown, not no.
func Absent(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, errInvalidName)
}
