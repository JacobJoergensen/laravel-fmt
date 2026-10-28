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

// Skeleton returns the laravel-fmt.json `init` writes: the schema reference
// alone. A setting left out keeps coming from the project's own tools, per
// file; one written here would override them for every file.
func Skeleton() []byte {
	return []byte("{\n  \"$schema\": \"" + SchemaURL + "\"\n}\n")
}
