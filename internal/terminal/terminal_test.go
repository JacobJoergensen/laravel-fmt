package terminal

import (
	"bytes"
	"testing"
)

func TestEnableDisableTogglePalette(t *testing.T) {
	Enable()

	if Red != "\033[38;2;248;113;113m" {
		t.Errorf("Enable: Red = %q, want the red escape", Red)
	}

	if Reset != "\033[0m" {
		t.Errorf("Enable: Reset = %q, want the reset escape", Reset)
	}

	Disable()

	for _, c := range palette {
		if *c.ptr != "" {
			t.Errorf("Disable left an escape set: %q", *c.ptr)
		}
	}

	Enable()
}

func TestIsTTYNonFileIsFalse(t *testing.T) {
	if IsTTY(&bytes.Buffer{}) {
		t.Error("IsTTY(*bytes.Buffer) = true, want false for a non-file writer")
	}
}
