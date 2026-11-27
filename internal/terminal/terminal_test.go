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

func TestAutoColorFollowsEnvironmentAndStream(t *testing.T) {
	tests := []struct {
		name   string
		env    map[string]string
		tty    bool
		stderr bool
		want   bool
	}{
		{"NO_COLOR beats FORCE_COLOR", map[string]string{"NO_COLOR": "1", "FORCE_COLOR": "1"}, true, true, false},
		{"FORCE_COLOR colors a stderr pipe", map[string]string{"FORCE_COLOR": "1"}, false, true, true},
		{"FORCE_COLOR leaves a stdout pipe plain", map[string]string{"FORCE_COLOR": "1"}, false, false, false},
		{"a terminal gets color", nil, true, false, true},
		{"a pipe stays plain", nil, false, true, false},
		{"a dumb terminal stays plain", map[string]string{"TERM": "dumb"}, true, false, false},
		{"Composer under Git Bash colors a stderr pipe", map[string]string{"MSYSTEM": "MINGW64", "COMPOSER_BINARY": "composer"}, false, true, true},
		{"Composer under Git Bash leaves a stdout pipe plain", map[string]string{"MSYSTEM": "MINGW64", "COMPOSER_BINARY": "composer"}, false, false, false},
		{"Git Bash outside Composer leaves a stderr pipe plain", map[string]string{"MSYSTEM": "MINGW64"}, false, true, false},
		{"Composer outside Git Bash leaves a stderr pipe plain", map[string]string{"COMPOSER_BINARY": "composer"}, false, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string { return tt.env[key] }

			if got := autoColor(getenv, tt.tty, tt.stderr); got != tt.want {
				t.Errorf("autoColor(tty=%v, stderr=%v) = %v, want %v", tt.tty, tt.stderr, got, tt.want)
			}
		})
	}
}

func TestIsTTYNonFileIsFalse(t *testing.T) {
	if IsTTY(&bytes.Buffer{}) {
		t.Error("IsTTY(*bytes.Buffer) = true, want false for a non-file writer")
	}
}
