// Package config defines the laravel-fmt.json schema and loads and generates it.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/tailwind"
	bladeparser "github.com/JacobJoergensen/laravel-fmt/internal/parser/blade"
)

const (
	FileName       = "laravel-fmt.json"
	LegacyFileName = "laravel-fmt.yml"
	SchemaURL      = "https://raw.githubusercontent.com/JacobJoergensen/laravel-fmt/main/schema/v1.json"
)

const (
	EndOfLineLF   = "lf"
	EndOfLineCRLF = "crlf"
	EndOfLineAuto = "auto"
)

const (
	IndentStyleSpace = "space"
	IndentStyleTab   = "tab"
)

// "auto" means each language's convention: single quotes in PHP, double in CSS,
// HTML and XML.
const (
	QuoteStyleAuto   = "auto"
	QuoteStyleSingle = "single"
	QuoteStyleDouble = "double"
)

const (
	BraceStyleSameLine       = "same-line"
	BraceStyleNextLine       = "next-line"
	BraceStyleAlwaysNextLine = "always-next-line"
)

const (
	ProseWrapAlways   = "always"
	ProseWrapNever    = "never"
	ProseWrapPreserve = "preserve"
)

// Values of Pint's heredoc_indentation rule.
const (
	HeredocIndentStartPlusOne = "start_plus_one"
	HeredocIndentSameAsStart  = "same_as_start"
	HeredocIndentNone         = "none"
)

// Values of Pint's ordered_imports.imports_order.
const (
	ImportKindClass    = "class"
	ImportKindFunction = "function"
	ImportKindConst    = "const"
)

// Keywords of Pint's blank_line_before_statement, less case, default and
// phpdoc, which are not statements in a sequence, and exit/die, which has no
// node of its own.
const (
	StmtBreak       = "break"
	StmtContinue    = "continue"
	StmtDeclare     = "declare"
	StmtDo          = "do"
	StmtFor         = "for"
	StmtForeach     = "foreach"
	StmtGoto        = "goto"
	StmtIf          = "if"
	StmtInclude     = "include"
	StmtIncludeOnce = "include_once"
	StmtRequire     = "require"
	StmtRequireOnce = "require_once"
	StmtReturn      = "return"
	StmtSwitch      = "switch"
	StmtThrow       = "throw"
	StmtTry         = "try"
	StmtWhile       = "while"
	StmtYield       = "yield"
	StmtYieldFrom   = "yield_from"
)

var BlankLineStatements = []string{
	StmtBreak, StmtContinue, StmtDeclare, StmtDo, StmtFor, StmtForeach,
	StmtGoto, StmtIf, StmtInclude, StmtIncludeOnce, StmtRequire, StmtRequireOnce,
	StmtReturn, StmtSwitch, StmtThrow, StmtTry, StmtWhile, StmtYield, StmtYieldFrom,
}

// File is the laravel-fmt.json configuration.
type File struct {
	Schema string `json:"$schema,omitempty"`

	Ignore []string `json:"ignore,omitempty"`

	// AdoptedIgnore is one layer per tool: a negation re-includes only what
	// its own list excluded.
	AdoptedIgnore [][]string `json:"-"`

	PrintWidth  int    `json:"printWidth"`
	IndentStyle string `json:"indentStyle"`
	IndentWidth int    `json:"indentWidth"`
	EndOfLine   string `json:"endOfLine"`
	QuoteStyle  string `json:"quoteStyle"`

	PHP        PHP        `json:"php"`
	Blade      Blade      `json:"blade,omitzero"`
	Markdown   Markdown   `json:"markdown"`
	TailwindCS TailwindCS `json:"tailwind"`

	// Resolved per project, never read from JSON: Tailwind is set once a v4
	// stylesheet is configured or discovered, and TailwindTheme is built from
	// that project's stylesheets.
	Tailwind      bool            `json:"-"`
	TailwindTheme *tailwind.Theme `json:"-"`

	// IndentExplicit is set when EditorConfig or laravel-fmt.json sets the
	// indent; JSON and YAML then use IndentWidth instead of their own default.
	IndentExplicit bool `json:"-"`
}

// UseTabs reports whether indentation uses tabs.
func (f *File) UseTabs() bool { return f.IndentStyle == IndentStyleTab }

// SingleQuote reports whether PHP strings prefer single quotes.
func (f *File) SingleQuote() bool { return f.QuoteStyle != QuoteStyleDouble }

// PHP holds the PHP options: Pint's laravel preset, overridden by pint.json,
// .php-cs-fixer.php or a Mago config, and those by laravel-fmt.json.
type PHP struct {
	RemoveTrailingCloseTag    bool     `json:"removeTrailingCloseTag"`
	SortUses                  bool     `json:"sortUses"`
	ExpandUseGroups           bool     `json:"expandUseGroups"`
	FunctionBraceStyle        string   `json:"functionBraceStyle"`
	MethodBraceStyle          string   `json:"methodBraceStyle"`
	ClasslikeBraceStyle       string   `json:"classBraceStyle"`
	AnonClassBraceStyle       string   `json:"anonymousClassBraceStyle"`
	ControlBraceStyle         string   `json:"controlBraceStyle"`
	ConcatSpace               bool     `json:"concatSpace"`
	NotOperatorSpace          bool     `json:"notOperatorSpace"`
	ClosureFnSpace            bool     `json:"closureFnSpace"`
	AlignFatArrow             bool     `json:"alignFatArrow"`
	ShortArray                bool     `json:"shortArray"`
	BlankLineBeforeStatements []string `json:"blankLineBeforeStatements"`
	HeredocIndentation        string   `json:"heredocIndentation"`
	ImportOrder               []string `json:"importOrder"`

	// ArrowValueOnKeyLine is set when the project's Pint or PHP-CS-Fixer rules
	// include no_multiline_whitespace_around_double_arrow, which joins a value
	// moved below its `=>` back onto the key line.
	ArrowValueOnKeyLine bool `json:"-"`
	// PintAlignment is set when AlignFatArrow comes from Pint or PHP-CS-Fixer.
	PintAlignment bool `json:"-"`
	// Off by default: Laravel's preset keeps a trait list as written.
	SplitTraitUses bool `json:"-"`
	// KeepBracedUseGroups is set when the project's single_import_per_statement
	// rule has group_to_single_imports off: a `use A\{B, C};` group stays one
	// statement while ExpandUseGroups still splits a comma list.
	KeepBracedUseGroups bool `json:"-"`
}

// Blade holds Blade formatter options.
type Blade struct {
	// BlockDirectives are declared, never guessed from @x … @endx: Laravel
	// Boost's @boostsnippet and Envoy's @task share that shape, and their
	// bodies are code and shell an indent would change.
	BlockDirectives []BladeBlockDirective `json:"blockDirectives,omitempty"`
	// Empty by default: whether a block belongs with the markup above it is
	// the author's call.
	BlankLineBeforeDirectives []string `json:"blankLineBeforeDirectives,omitempty"`
}

// BladeDirectivePHP is the name `@php` takes in BlankLineBeforeDirectives.
const BladeDirectivePHP = "php"

// BladeBlockDirective is one declared block directive.
type BladeBlockDirective struct {
	Open     string   `json:"open"`
	Close    string   `json:"close,omitempty"`
	Branches []string `json:"branches,omitempty"`
}

// Closer returns the directive that closes this block.
func (d BladeBlockDirective) Closer() string {
	if d.Close != "" {
		return d.Close
	}

	return "end" + d.Open
}

// Markdown holds Markdown formatter options.
type Markdown struct {
	ProseWrap string `json:"proseWrap"`
}

// TailwindCS configures Tailwind class sorting.
type TailwindCS struct {
	// Stylesheet names the Tailwind v4 entry points, relative to this
	// laravel-fmt.json. Empty means discovery under resources/css, a missing
	// path is an error, and several merge in order.
	Stylesheet StringOrSlice `json:"stylesheet"`
}

// StringOrSlice is a JSON field written as one string or a list of them.
type StringOrSlice []string

// UnmarshalJSON accepts a bare string as a one-element list.
func (s *StringOrSlice) UnmarshalJSON(data []byte) error {
	var single string

	if err := json.Unmarshal(data, &single); err == nil {
		*s = withoutBlanks([]string{single})

		return nil
	}

	var many []string

	if err := json.Unmarshal(data, &many); err != nil {
		// encoding/json adds no key path to an Unmarshaler's error, so this is
		// the whole message the user sees; keep Go's type names out of it.
		if typeErr, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
			return fmt.Errorf("expected a string or a list of strings, not %s", describeJSONValue(typeErr.Value))
		}

		return fmt.Errorf("expected a string or a list of strings: %w", err)
	}

	*s = withoutBlanks(many)

	return nil
}

// A blank entry must stay unset: an empty path resolves to the directory of the
// file that set it, and a directory passes the exists check.
func withoutBlanks(values []string) StringOrSlice {
	kept := slices.DeleteFunc(values, func(v string) bool { return v == "" })

	if len(kept) == 0 {
		return nil
	}

	return kept
}

// MarshalJSON writes one entry as a string and none as "", since the schema
// does not allow null.
func (s StringOrSlice) MarshalJSON() ([]byte, error) {
	var value any = []string(s)

	switch len(s) {
	case 0:
		value = ""
	case 1:
		value = s[0]
	}

	out, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal stylesheet: %w", err)
	}

	return out, nil
}

// IndentStyleFor maps a useTabs boolean to its indentStyle value.
func IndentStyleFor(useTabs bool) string {
	if useTabs {
		return IndentStyleTab
	}

	return IndentStyleSpace
}

// Default returns the baseline configuration.
func Default() *File {
	return &File{
		Schema: SchemaURL,
		Ignore: []string{
			// Dependency trees nest, so these match at any depth. Laravel's own
			// directories stay anchored: a `storage` further down is the
			// project's code, not the framework's.
			"vendor/",
			"node_modules/",
			"storage/**",
			"bootstrap/cache/**",
			"composer.lock",
			"package-lock.json",
			"pnpm-lock.yaml",
			"yarn.lock",
			// barryvdh/laravel-ide-helper regenerates these whole.
			"_ide_helper*.php",
		},
		PrintWidth:  120,
		IndentStyle: IndentStyleSpace,
		IndentWidth: 4,
		EndOfLine:   EndOfLineLF,
		QuoteStyle:  QuoteStyleAuto,
		PHP: PHP{
			RemoveTrailingCloseTag:    true,
			SortUses:                  true,
			ExpandUseGroups:           true,
			FunctionBraceStyle:        BraceStyleNextLine,
			MethodBraceStyle:          BraceStyleNextLine,
			ClasslikeBraceStyle:       BraceStyleNextLine,
			AnonClassBraceStyle:       BraceStyleNextLine,
			ControlBraceStyle:         BraceStyleSameLine,
			ConcatSpace:               false,
			NotOperatorSpace:          true,
			ClosureFnSpace:            true,
			AlignFatArrow:             false,
			ShortArray:                true,
			BlankLineBeforeStatements: []string{StmtContinue, StmtReturn},
			HeredocIndentation:        HeredocIndentNone,
			ImportOrder:               []string{ImportKindConst, ImportKindClass, ImportKindFunction},
		},
		Markdown: Markdown{
			ProseWrap: ProseWrapPreserve,
		},
	}
}

// validate's errors name only the field; the loader adds the file path.
func (f *File) validate() error {
	if f.PrintWidth < 1 {
		return errors.New("printWidth must be >= 1")
	}

	if f.IndentWidth < 1 {
		return errors.New("indentWidth must be >= 1")
	}

	if err := validateEnum("indentStyle", f.IndentStyle, IndentStyleSpace, IndentStyleTab); err != nil {
		return err
	}

	if err := validateEnum("endOfLine", f.EndOfLine, EndOfLineLF, EndOfLineCRLF, EndOfLineAuto); err != nil {
		return err
	}

	if err := validateEnum("quoteStyle", f.QuoteStyle, QuoteStyleAuto, QuoteStyleSingle, QuoteStyleDouble); err != nil {
		return err
	}

	if err := f.PHP.validate(); err != nil {
		return fmt.Errorf("php: %w", err)
	}

	if err := f.Blade.validate(); err != nil {
		return fmt.Errorf("blade: %w", err)
	}

	if err := f.Markdown.validate(); err != nil {
		return fmt.Errorf("markdown: %w", err)
	}

	return nil
}

func (b *Blade) validate() error {
	opens := make(map[string]bool, len(b.BlockDirectives))

	for _, d := range b.BlockDirectives {
		names := append([]string{d.Open, d.Closer()}, d.Branches...)

		for _, name := range names {
			if !isDirectiveName(name) {
				return fmt.Errorf("blockDirectives: %q is not a directive name (letters, digits, underscore)", name)
			}
		}

		for _, name := range []string{d.Open, d.Closer()} {
			if bladeparser.IsBuiltinOpenerOrCloser(name) {
				return fmt.Errorf("blockDirectives: @%s is already one of Blade's own block directives", name)
			}
		}

		if d.Open == d.Closer() {
			return fmt.Errorf("blockDirectives: @%s cannot close itself", d.Open)
		}

		if opens[d.Open] {
			return fmt.Errorf("blockDirectives: @%s is declared twice", d.Open)
		}

		opens[d.Open] = true
	}

	builtins := bladeparser.BuiltinOpeners()

	for _, name := range b.BlankLineBeforeDirectives {
		if name != BladeDirectivePHP && !opens[name] && !slices.Contains(builtins, name) {
			return fmt.Errorf("blankLineBeforeDirectives: @%s is not a block directive (Blade's own, or one declared in blockDirectives)", name)
		}
	}

	return nil
}

// isDirectiveName matches the \w+ Blade's compiler accepts as a directive name.
func isDirectiveName(name string) bool {
	return name != "" && !strings.ContainsFunc(name, func(c rune) bool {
		return c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9')
	})
}

func (p *PHP) validate() error {
	braceStyles := []struct {
		field string
		value string
	}{
		{"functionBraceStyle", p.FunctionBraceStyle},
		{"methodBraceStyle", p.MethodBraceStyle},
		{"classBraceStyle", p.ClasslikeBraceStyle},
		{"anonymousClassBraceStyle", p.AnonClassBraceStyle},
		{"controlBraceStyle", p.ControlBraceStyle},
	}

	for _, b := range braceStyles {
		if err := validateEnum(b.field, b.value, BraceStyleSameLine, BraceStyleNextLine, BraceStyleAlwaysNextLine); err != nil {
			return err
		}
	}

	if err := validateEnum("heredocIndentation", p.HeredocIndentation, HeredocIndentStartPlusOne, HeredocIndentSameAsStart, HeredocIndentNone); err != nil {
		return err
	}

	for _, kind := range p.ImportOrder {
		if err := validateEnum("importOrder", kind, ImportKindConst, ImportKindClass, ImportKindFunction); err != nil {
			return err
		}
	}

	for _, stmt := range p.BlankLineBeforeStatements {
		if err := validateEnum("blankLineBeforeStatements", stmt, BlankLineStatements...); err != nil {
			return err
		}
	}

	return nil
}

func (m Markdown) validate() error {
	return validateEnum("proseWrap", m.ProseWrap, ProseWrapAlways, ProseWrapNever, ProseWrapPreserve)
}

func validateEnum(field, value string, allowed ...string) error {
	if slices.Contains(allowed, value) {
		return nil
	}

	return fmt.Errorf("invalid %s %q (use: %s)", field, value, strings.Join(allowed, ", "))
}
