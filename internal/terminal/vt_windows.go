//go:build windows

package terminal

import (
	"os"

	"golang.org/x/sys/windows"
)

// conhost leaves ENABLE_VIRTUAL_TERMINAL_PROCESSING off, so escapes print as
// ←[31m litter there. Both streams carry color, and either may be the console
// while the other is redirected. A handle that is not a console fails
// GetConsoleMode and is skipped; it gets no color anyway.
func enableVirtualTerminal() {
	for _, f := range []*os.File{os.Stdout, os.Stderr} {
		handle := windows.Handle(f.Fd())

		var mode uint32
		if err := windows.GetConsoleMode(handle, &mode); err != nil {
			continue
		}

		_ = windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
}
