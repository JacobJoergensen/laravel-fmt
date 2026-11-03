//go:build windows

package walk

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

// VOLUME_NAME_DOS, which x/sys/windows does not export.
const volumeNameDOS = 0x0

// filepath.EvalSymlinks leaves junctions alone since Go 1.23, so Windows is
// asked for the path it opens. FILE_FLAG_BACKUP_SEMANTICS lets a folder open.
func resolveLinks(path string) (string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}

	const share = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE

	handle, err := windows.CreateFile(name, 0, share, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}

	// The path is already read by then; a failed close changes nothing.
	defer func() { _ = windows.CloseHandle(handle) }()

	var buf [windows.MAX_LONG_PATH]uint16

	n, err := windows.GetFinalPathNameByHandle(handle, &buf[0], windows.MAX_LONG_PATH, volumeNameDOS)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}

	final := windows.UTF16ToString(buf[:n])

	if share, ok := strings.CutPrefix(final, `\?\UNC\`); ok {
		return `\` + share, nil
	}

	return strings.TrimPrefix(final, `\?\`), nil
}
