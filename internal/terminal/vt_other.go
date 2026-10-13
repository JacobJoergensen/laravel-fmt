//go:build !windows

package terminal

// Terminals here render ANSI escapes without opting in.
func enableVirtualTerminal() {}
