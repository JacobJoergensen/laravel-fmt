package config

import (
	"encoding/json"
	"fmt"
)

// Generate encodes cfg as laravel-fmt.json, adding $schema so editors offer
// completion and validation.
func Generate(cfg *File) ([]byte, error) {
	out := *cfg

	if out.Schema == "" {
		out.Schema = SchemaURL
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", FileName, err)
	}

	return append(data, '\n'), nil
}
