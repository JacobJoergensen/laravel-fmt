package livewire

import (
	parserpkg "github.com/JacobJoergensen/laravel-fmt/internal/parser"
	"github.com/JacobJoergensen/laravel-fmt/internal/parser/blade"
	"github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

// Position is parser.Position.
type Position = parserpkg.Position

// Span is a half-open byte range [Start, End) in source.
type Span = parserpkg.Span

// Error is a parse diagnostic.
type Error = parserpkg.Error

// File is a parsed Livewire SFC. PHP holds the frontmatter <?php ... ?>;
// Template is nil when there's no close tag, and the whole source is then PHP.
type File struct {
	Src      []byte
	PHP      *php.File
	Template *blade.File
}
