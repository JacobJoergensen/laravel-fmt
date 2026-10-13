// Package terminal styles laravel-fmt's CLI output.
package terminal

import (
	"io"
	"os"
)

// Color escapes for stderr output, set by Enable and blanked by Disable.
var (
	Bold   string
	Dim    string
	Reset  string
	Red    string
	Green  string
	Yellow string
	Gray   string
	Cyan   string
	// WhitespaceMark highlights trailing whitespace in a diff, which renders as
	// nothing without it.
	WhitespaceMark string
)

const (
	codeBold           = "\033[1m"
	codeDim            = "\033[2m"
	codeReset          = "\033[0m"
	codeRed            = "\033[38;2;248;113;113m"
	codeGreen          = "\033[38;2;52;211;153m"
	codeYellow         = "\033[38;2;246;193;119m"
	codeGray           = "\033[38;2;100;116;139m"
	codeCyan           = "\033[38;2;103;232;249m"
	codeWhitespaceMark = "\033[41m"
)

var palette = []struct {
	ptr  *string
	code string
}{
	{&Bold, codeBold},
	{&Dim, codeDim},
	{&Reset, codeReset},
	{&Red, codeRed},
	{&Green, codeGreen},
	{&Yellow, codeYellow},
	{&Gray, codeGray},
	{&Cyan, codeCyan},
	{&WhitespaceMark, codeWhitespaceMark},
}

// Palette holds the escapes for stdout, which decides color on its own; the
// package variables follow stderr. The zero value writes no color.
type Palette struct {
	Bold           string
	Reset          string
	Red            string
	Green          string
	Cyan           string
	WhitespaceMark string
}

// Colors returns the Palette for a stream that renders color.
func Colors() Palette {
	return Palette{
		Bold:           codeBold,
		Reset:          codeReset,
		Red:            codeRed,
		Green:          codeGreen,
		Cyan:           codeCyan,
		WhitespaceMark: codeWhitespaceMark,
	}
}

// WantsColor reports whether w should get color when the user left --color on
// auto: it is a terminal, and NO_COLOR is not set.
func WantsColor(w io.Writer) bool {
	return os.Getenv("NO_COLOR") == "" && IsTTY(w)
}

const (
	CheckMark   = "✓"
	CrossMark   = "✗"
	Tilde       = "~"
	WarningSign = "⚠"
)

// Disable blanks every color escape.
func Disable() {
	for _, c := range palette {
		*c.ptr = ""
	}
}

// Enable restores the color escapes, so --color=always can override the
// init-time auto-disable.
func Enable() {
	for _, c := range palette {
		*c.ptr = c.code
	}
}

// IsTTY reports whether w is a terminal that can render ANSI escapes.
func IsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)

	if !ok {
		return false
	}

	stat, err := f.Stat()
	if err != nil {
		return false
	}

	return (stat.Mode() & os.ModeCharDevice) != 0
}

func init() {
	Enable()
	enableVirtualTerminal()

	// The colored status lines and diagnostics go to stderr, so stderr decides.
	if !WantsColor(os.Stderr) {
		Disable()
	}
}
