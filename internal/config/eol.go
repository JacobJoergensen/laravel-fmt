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
