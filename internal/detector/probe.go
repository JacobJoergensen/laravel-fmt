package detector

import (
	"os"
	"sync"
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
	case !absent(err):
		entry.err = err
	}

	stored, _ := memo.stats.LoadOrStore(path, entry)
	shared, _ := stored.(statEntry)

	return shared.exists, shared.err
}
