package config

import "bytes"

// ResolveEndOfLine settles an "auto" end of line from src.
func ResolveEndOfLine(src []byte, cfg *File) {
	if cfg.EndOfLine != EndOfLineAuto {
		return
	}

	if bytes.Contains(src, []byte("\r\n")) {
		cfg.EndOfLine = EndOfLineCRLF
	} else {
		cfg.EndOfLine = EndOfLineLF
	}
}

// EOL returns the bytes a line ends with. EndOfLineAuto reads as LF, since
// ResolveEndOfLine settles it before formatting.
func (f *File) EOL() string {
	if f.EndOfLine == EndOfLineCRLF {
		return "\r\n"
	}

	return "\n"
}
