package livewire

import (
	"github.com/JacobJoergensen/laravel-fmt/internal/parser/blade"
	"github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

// Position is php.Position, shared so all three layers use one coordinate type.
type Position = php.Position

// Span is a half-open byte range [Start, End) in source.
type Span = php.Span

// Error is a parse diagnostic.
type Error = php.Error

// File is a parsed Livewire SFC. PHP holds the frontmatter <?php ... ?>;
// Template is nil when there's no close tag, and the whole source is then PHP.
type File struct {
	Src      []byte
	PHP      *php.File
	Template *blade.File
}
