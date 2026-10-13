//go:build windows

package detector

import (
	"errors"
	"os"
	"syscall"
)

// ERROR_INVALID_NAME, which syscall does not export.
const errInvalidName = syscall.Errno(0x7B)

// Windows answers a name it cannot represent with ERROR_INVALID_NAME rather
// than not-exist, and such a path cannot hold the marker file. Any other error
// still stops the walk: the answer is unknown, not no.
func absent(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, errInvalidName)
}
