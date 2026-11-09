package detector

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/JacobJoergensen/laravel-fmt/internal/upward"
)

// probeKind names one of the upward-walking probes, so two of them asking
// about the same directory do not read each other's answer.
type probeKind uint8

const (
	probePint probeKind = iota
	probePhpCsFixer
	probeMago
	probePrettier
	probeTailwind
)

type probeKey struct {
	dir  string
	kind probeKind
}

type probeEntry struct {
	value any
	err   error
	found bool
}

type statEntry struct {
	err    error
	exists bool
}

// probeMemo remembers each probe's answer per directory. Outside a project
// every file walks up from its own directory and siblings share almost the
// whole chain; without the memo a markerless tree probed each ancestor once
// per directory, doubling the run time.
//
// WARNING: it lives as long as one Resolver, never the process. The LSP
// builds a fresh Resolver per request so nothing outlives an edit.
type probeMemo struct {
	probes        sync.Map
	stats         sync.Map
	editorConfigs sync.Map
}

func probeDir[T any](memo *probeMemo, kind probeKind, dir string, detect func(string) (T, bool, error)) (T, bool, error) {
	key := probeKey{dir: dir, kind: kind}

	if cached, ok := memo.probes.Load(key); ok {
		entry, _ := cached.(probeEntry)
		value, _ := entry.value.(T)

		return value, entry.found, entry.err
	}

	value, found, err := detect(dir)
	stored, _ := memo.probes.LoadOrStore(key, probeEntry{value: value, err: err, found: found})
	entry, _ := stored.(probeEntry)
	shared, _ := entry.value.(T)

	return shared, entry.found, entry.err
}

// statExists is shared by the walks that look for the same marker files. An
// error means unknown, not absent, and the caller adds the context.
func (memo *probeMemo) statExists(path string) (bool, error) {
	if cached, ok := memo.stats.Load(path); ok {
		entry, _ := cached.(statEntry)

		return entry.exists, entry.err
	}

	var entry statEntry

	switch _, err := os.Stat(path); {
	case err == nil:
		entry.exists = true
	case !upward.Absent(err):
		entry.err = err
	}

	stored, _ := memo.stats.LoadOrStore(path, entry)
	shared, _ := stored.(statEntry)

	return shared.exists, shared.err
}

// One listing per directory is cheaper than a stat per candidate name, which
// added 7% to a monorepo run. A directory that can be entered but not listed,
// like a 0711 home folder, falls back to the stat per candidate.
func dirNames(dir string, candidates []string) (map[string]bool, error) {
	f, err := os.Open(dir)
	if err != nil {
		switch {
		case upward.Absent(err):
			return map[string]bool{}, nil
		case errors.Is(err, os.ErrPermission):
			return statNames(dir, candidates)
		}

		return nil, fmt.Errorf("open %s: %w", dir, err)
	}

	names, err := f.Readdirnames(-1)
	if closeErr := f.Close(); err == nil && closeErr != nil {
		err = closeErr
	}

	// A Windows ACL can allow opening the folder and deny reading it.
	if errors.Is(err, os.ErrPermission) {
		return statNames(dir, candidates)
	}

	if err != nil {
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}

	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[nameKey(name)] = true
	}

	return set, nil
}

// Lstat, not Stat, so a dangling symlink counts as present, as a listing
// reports it.
func statNames(dir string, candidates []string) (map[string]bool, error) {
	set := make(map[string]bool, len(candidates))

	for _, name := range candidates {
		path := filepath.Join(dir, name)

		_, err := os.Lstat(path)
		if err == nil {
			set[nameKey(name)] = true

			continue
		}

		if !upward.Absent(err) {
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}
	}

	return set, nil
}

// A listed name has to match exactly the files an os.Stat of the candidate finds.
func nameKey(name string) string {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.ToLower(name)
	}

	return name
}
