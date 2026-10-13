package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGenerateRoundTrips(t *testing.T) {
	data, err := Generate(Default())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if !strings.HasPrefix(string(data), "{\n  \"$schema\"") {
		t.Fatalf("generated config does not lead with $schema:\n%s", data)
	}

	path := filepath.Join(t.TempDir(), FileName)

	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	loaded, err := LoadFile(Default(), path)
	if err != nil {
		t.Fatalf("LoadFile of generated config: %v", err)
	}

	if !reflect.DeepEqual(loaded.File, *Default()) {
		t.Fatalf("generated config did not round-trip:\n got %+v\nwant %+v", loaded.File, *Default())
	}
}
