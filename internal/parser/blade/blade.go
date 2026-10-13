// Package blade parses Laravel Blade templates into a tree of Blade and html.*
// nodes, delegating embedded PHP to internal/parser/php.
package blade

// Parse parses src with the block directives in dirs, nil meaning Blade's
// built-in ones; File is non-nil even when errors are returned.
func Parse(src []byte, dirs *Directives) (*File, []Error) {
	return ParseAt(src, Position{Line: 1, Column: 1}, dirs)
}

// ParseAt parses src from start instead of offset 0, which Livewire uses
// to skip the PHP frontmatter.
func ParseAt(src []byte, start Position, dirs *Directives) (*File, []Error) {
	if dirs == nil {
		dirs = builtinDirectives
	}

	p := &bladeParser{Src: src, Pos: start, dirs: dirs}
	nodes := p.parseFile()

	return &File{Src: src, Nodes: nodes, Directives: dirs}, p.errors
}
