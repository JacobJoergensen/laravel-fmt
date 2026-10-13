//go:build !windows

package detector

import (
	"errors"
	"os"
)

func absent(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
