package walk

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

type ignorePattern struct {
	negate   bool
	anchored bool
	dirOnly  bool
	segments []string
}

type ignoreMatcher []ignorePattern

// Input order is preserved; match relies on it for last-match-wins.
func compileMatcher(lines []string) ignoreMatcher {
	out := make(ignoreMatcher, 0, len(lines))

	for _, raw := range lines {
		if p, ok := compilePattern(raw); ok {
			out = append(out, p)
		}
	}

	return out
}

func compilePattern(raw string) (ignorePattern, bool) {
	line := trimTrailingSpaces(raw)

	if line == "" || line[0] == '#' {
		return ignorePattern{}, false
	}

	var p ignorePattern

	if line[0] == '!' {
		p.negate = true
		line = line[1:]
	}

	if strings.HasSuffix(line, "/") {
		p.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}

	if strings.HasPrefix(line, "/") {
		p.anchored = true
		line = strings.TrimPrefix(line, "/")
	} else if strings.Contains(line, "/") {
		// An internal / anchors a gitignore pattern: foo/bar does not match
		// baz/foo/bar. Unanchored ones get **/ prepended below.
		p.anchored = true
	}

	if line == "" {
		return ignorePattern{}, false
	}

	if !p.anchored {
		line = "**/" + line
	}

	p.segments = strings.Split(line, "/")

	return p, true
}

// trimTrailingSpaces drops the trailing spaces git ignores, keeping one that is
// escaped as `\ `. Leading spaces are part of the pattern, as they are to git.
func trimTrailingSpaces(line string) string {
	trimmed := strings.TrimRight(line, " ")
	if len(trimmed) < len(line) && strings.HasSuffix(trimmed, `\`) {
		return trimmed + " "
	}

	return trimmed
}

// compileScopedPattern compiles raw as it appears in a .gitignore living in
// dir, a slash path relative to the ignore root and empty for the root itself.
//
// CONTRACT: the directory is prepended to the pattern's segments, so a pattern
// cannot match outside its own subtree however broad it reads: a bare `*` in
// `storage/` compiles to `storage/**/*`. Scoping a nested file by anything
// weaker is how a lone `*` ends up ignoring an entire project.
func compileScopedPattern(dir, raw string) (ignorePattern, bool) {
	p, ok := compilePattern(raw)
	if !ok || dir == "" {
		return p, ok
	}

	p.segments = append(strings.Split(dir, "/"), p.segments...)

	return p, true
}

func (m ignoreMatcher) match(rel string, isDir bool) bool {
	return m.matchFrom(false, rel, isDir)
}

// matchFrom continues last-match-wins from an earlier layer's verdict, which is
// how .gitignore patterns and the configured ones compose into one answer.
func (m ignoreMatcher) matchFrom(ignored bool, rel string, isDir bool) bool {
	if len(m) == 0 {
		return ignored
	}

	return m.matchSegmentsFrom(ignored, strings.Split(rel, "/"), isDir)
}

func (m ignoreMatcher) matchSegmentsFrom(ignored bool, segments []string, isDir bool) bool {
	for _, p := range m {
		if !p.matches(segments, isDir) {
			continue
		}

		ignored = !p.negate
	}

	return ignored
}

// ignoreRules are the configured patterns. Each adopted list is a layer that
// ignores by its own last match, so its negations re-include only what it
// excluded; the built-ins and laravel-fmt.json's list then continue from that
// verdict, so a "!" there still wins over everything.
type ignoreRules struct {
	adopted []ignoreMatcher
	own     ignoreMatcher
}

func compileRules(cfg *config.File) ignoreRules {
	if cfg == nil {
		return ignoreRules{}
	}

	rules := ignoreRules{own: compileMatcher(cfg.Ignore)}

	for _, list := range cfg.AdoptedIgnore {
		rules.adopted = append(rules.adopted, compileMatcher(list))
	}

	return rules
}

func (r ignoreRules) match(rel string, isDir bool) bool {
	return r.matchFrom(false, rel, isDir)
}

func (r ignoreRules) matchFrom(ignored bool, rel string, isDir bool) bool {
	if len(r.adopted) == 0 && len(r.own) == 0 {
		return ignored
	}

	segments := strings.Split(rel, "/")

	for _, layer := range r.adopted {
		if !ignored {
			ignored = layer.matchSegmentsFrom(false, segments, isDir)
		}
	}

	return r.own.matchSegmentsFrom(ignored, segments, isDir)
}

// ignoreSet answers with the .gitignore layer first and the configured one
// second, so an entry in laravel-fmt.json's ignore list overrides git. Within
// the git layer a deeper .gitignore is appended later and so wins, as git
// specifies.
type ignoreSet struct {
	git ignoreMatcher
	cfg ignoreRules

	above     ignoreMatcher
	aboveRoot string

	// excluded is set when a .gitignore above the ignore root excludes the root
	// or a directory above it; git then ignores everything inside.
	excluded bool

	// repos are the git work trees nested below the ignore root, keyed by their
	// slash path from it with a trailing slash. A path inside one answers to
	// that repo's .gitignore files alone, as git reads it; the repo's own
	// directory is still an entry of the tree around it.
	repos map[string]ignoreMatcher

	// projects are the ignore patterns of the other projects a run reaches,
	// each anchored at its own root; a path answers to its nearest one.
	projects []projectPatterns
}

type projectPatterns struct {
	anchor   string
	patterns ignoreRules
}

func (s *ignoreSet) addProject(anchor string, cfg *config.File) {
	for _, p := range s.projects {
		if p.anchor == anchor {
			return
		}
	}

	s.projects = append(s.projects, projectPatterns{anchor: anchor, patterns: compileRules(cfg)})
}

// enterProject records the project dir belongs to. With anchoredHere, only a
// project anchored at dir itself counts: a walk entering directories meets each
// project at its anchor, and asking about the others there would only repeat
// the one already recorded.
func (s *ignoreSet) enterProject(projectOf ProjectIgnore, dir string, anchoredHere bool) error {
	if projectOf == nil {
		return nil
	}

	anchor, cfg, err := projectOf(dir)
	if err != nil {
		return fmt.Errorf("resolve the project of %s: %w", dir, err)
	}

	if anchor == "" || (anchoredHere && filepath.Clean(anchor) != filepath.Clean(dir)) {
		return nil
	}

	s.addProject(filepath.Clean(anchor), cfg)

	return nil
}

func (s *ignoreSet) matchProject(abs string, isDir bool) bool {
	p := s.nearestProject(abs)

	return p != nil && p.ignores(abs, isDir)
}

// matchProjectFile is matchProject for a file tested on its own, outside a walk
// that prunes: an ignored directory of its project excludes it too, as matchFile
// does for the other layers. The scan stops at the project's anchor, since the
// directories above it belong to another project.
func (s *ignoreSet) matchProjectFile(abs string) bool {
	p := s.nearestProject(abs)
	if p == nil {
		return false
	}

	if p.ignores(abs, false) {
		return true
	}

	for dir := filepath.Dir(abs); dir != p.anchor && underAnchor(p.anchor, dir); dir = filepath.Dir(dir) {
		if p.ignores(dir, true) {
			return true
		}
	}

	return false
}

func (s *ignoreSet) nearestProject(abs string) *projectPatterns {
	var nearest *projectPatterns

	for i := range s.projects {
		p := &s.projects[i]

		if underAnchor(p.anchor, abs) && (nearest == nil || len(p.anchor) > len(nearest.anchor)) {
			nearest = p
		}
	}

	return nearest
}

// abs must lie at or below p's anchor.
func (p *projectPatterns) ignores(abs string, isDir bool) bool {
	rel := strings.TrimPrefix(abs[len(p.anchor):], string(filepath.Separator))
	if rel == "" {
		return false
	}

	return p.patterns.match(filepath.ToSlash(rel), isDir)
}

func (s *ignoreSet) match(rel string, isDir bool) bool {
	return s.cfg.matchFrom(s.matchGit(rel, isDir), rel, isDir)
}

// The files above the ignore root are shallower, so they match first.
func (s *ignoreSet) matchGit(rel string, isDir bool) bool {
	if repo, ok := s.repoOf(rel); ok {
		return s.repos[repo].match(rel, isDir)
	}

	if s.excluded {
		return true
	}

	ignored := s.above.matchFrom(false, s.aboveRoot+rel, isDir)

	return s.git.matchFrom(ignored, rel, isDir)
}

func (s *ignoreSet) repoOf(rel string) (string, bool) {
	var deepest string

	for repo := range s.repos {
		if len(repo) > len(deepest) && strings.HasPrefix(rel, repo) {
			deepest = repo
		}
	}

	return deepest, deepest != ""
}

// git cannot re-include a file under an excluded directory, so every ancestor
// is checked; a walk gets this by pruning, but IgnoreFilter tests files alone.
func (s *ignoreSet) matchFile(filePath string) bool {
	if s.match(filePath, false) {
		return true
	}

	for dir := filePath; ; {
		i := strings.LastIndexByte(dir, '/')
		if i < 0 {
			return false
		}

		dir = dir[:i]

		if s.match(dir, true) {
			return true
		}
	}
}

func (p ignorePattern) matches(segments []string, isDir bool) bool {
	if p.dirOnly && !isDir {
		return false
	}

	return matchSegments(p.segments, segments)
}

// path.Match, not filepath.Match: on Windows filepath reads \ as a separator,
// so a gitignore escape such as \#name never matched.
func matchSegments(pattern, segments []string) bool {
	if len(pattern) == 0 {
		return len(segments) == 0
	}

	head := pattern[0]

	// A trailing `**` matches what is inside a directory, not the directory:
	// `foo/**` leaves `foo` itself unignored, so `!foo/keep.php` can still
	// re-include a file below it, which pruning `foo` would make impossible.
	if head == "**" && len(pattern) == 1 {
		return len(segments) > 0
	}

	if head == "**" {
		if matchSegments(pattern[1:], segments) {
			return true
		}

		if len(segments) == 0 {
			return false
		}

		return matchSegments(pattern, segments[1:])
	}

	if len(segments) == 0 {
		return false
	}

	ok, err := path.Match(head, segments[0])

	if err != nil || !ok {
		return false
	}

	return matchSegments(pattern[1:], segments[1:])
}

// Both paths are clean and absolute, so a separator-bounded prefix answers
// without Abs or Rel; this runs for every file against every project.
func underAnchor(anchor, abs string) bool {
	if !strings.HasPrefix(abs, anchor) {
		return false
	}

	return len(abs) == len(anchor) || abs[len(anchor)] == filepath.Separator || strings.HasSuffix(anchor, string(filepath.Separator))
}
