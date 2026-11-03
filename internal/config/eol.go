package config

import "bytes"

// ResolveEndOfLine settles "auto" from the first LF in src, or LF without one.
func ResolveEndOfLine(src []byte, cfg *File) {
	if cfg.EndOfLine != EndOfLineAuto {
		return
	}

	firstLF := bytes.IndexByte(src, '\n')

	if firstLF > 0 && src[firstLF-1] == '\r' {
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
