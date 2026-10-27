package detector

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

const (
	pintBraceSameLine = "same_line"
	pintBraceNextLine = "next_line_unless_newline_at_signature_end"
	pintSpacingOne    = "one"
	pintSpacingNone   = "none"
	pintAlign         = "align"
	pintDefaultPreset = "laravel"

	// pintIndentWidth is the indent Pint always formats PHP with.
	pintIndentWidth = 4
)

// Pint is the part of pint.json laravel-fmt applies.
type Pint struct {
	Path                          string
	Preset                        string
	CurlyBracesPosition           *CurlyBracesPosition
	ConcatSpace                   *ConcatSpace
	NotOperatorWithSuccessorSpace *bool
	FunctionDeclaration           *FunctionDeclaration
	BinaryOperatorSpaces          *BinaryOperatorSpaces
	BlankLineBeforeStatement      *BlankLineBeforeStatement
	BladeBlankLineBeforeStatement *BlankLineBeforeStatement
	OrderedImports                *OrderedImports
	ArraySyntax                   *ArraySyntax

	HeredocIndentation *string

	// nil when pint.json does not mention the rule.
	SortImports                   *bool
	SingleImportPerStatement      *bool
	SingleTraitInsertPerStatement *bool
	NoClosingTag                  *bool
	NoMultilineDoubleArrow        *bool

	GroupToSingleImports *bool

	// The Symfony Finder options pint.json may set to keep files from Pint.
	Exclude []string
	NotName []string
	NotPath []string
}

// CurlyBracesPosition is Pint's curly_braces_position rule, also written
// braces_position; an empty field leaves the setting alone.
type CurlyBracesPosition struct {
	FunctionsOpeningBrace         string `json:"functions_opening_brace"`
	ClassesOpeningBrace           string `json:"classes_opening_brace"`
	AnonymousClassesOpeningBrace  string `json:"anonymous_classes_opening_brace"`
	ControlStructuresOpeningBrace string `json:"control_structures_opening_brace"`
}

// ConcatSpace is Pint's concat_space rule.
type ConcatSpace struct {
	Spacing string `json:"spacing"`
}

// FunctionDeclaration is Pint's function_declaration rule; only
// ClosureFnSpacing is used.
type FunctionDeclaration struct {
	ClosureFnSpacing string `json:"closure_fn_spacing"`
}

// BinaryOperatorSpaces is Pint's binary_operator_spaces rule; only its =>
// entry is used.
type BinaryOperatorSpaces struct {
	Operators map[string]string `json:"operators"`
}

// BlankLineBeforeStatement is Pint's blank_line_before_statement rule.
type BlankLineBeforeStatement struct {
	Statements []string `json:"statements"`
}

// OrderedImports is Pint's ordered_imports rule. Its sort_algorithm is
// ignored: laravel-fmt always sorts alphabetically.
type OrderedImports struct {
	ImportsOrder []string `json:"imports_order"`
}

// SingleImportOptions is the options object of Pint's
// single_import_per_statement rule.
type SingleImportOptions struct {
	GroupToSingleImports *bool `json:"group_to_single_imports"`
}

// LaravelBladeOptions is the options object of Pint's Pint/laravel_blade rule.
type LaravelBladeOptions struct {
	BlankLineBeforeStatement *BlankLineBeforeStatement `json:"blank_line_before_statement"`
}

// A snapshot of Pint's BlankLineBeforeControlStructures::STATEMENTS.
var pintBladeStatements = []string{"if", "foreach", "forelse", "for", "while", "switch", config.BladeDirectivePHP}

// ArraySyntax is Pint's array_syntax rule.
type ArraySyntax struct {
	Syntax string `json:"syntax"`
}

type pintFile struct {
	Extend  *string                    `json:"extend"`
	Preset  string                     `json:"preset"`
	Rules   map[string]json.RawMessage `json:"rules"`
	Exclude json.RawMessage            `json:"exclude"`
	NotName json.RawMessage            `json:"notName"`
	NotPath json.RawMessage            `json:"notPath"`
}

func detectPint(root string) (Pint, bool, error) {
	path := filepath.Join(root, "pint.json")

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return detectComposerPint(root)
		}

		return Pint{}, false, fmt.Errorf("read %s: %w", path, err)
	}

	parsed, err := parsePintFile(path, raw)
	if err != nil {
		return Pint{}, false, err
	}

	pint := rulesToPint(parsed.Rules)
	pint.Path = path
	pint.Preset = cmp.Or(parsed.Preset, pintDefaultPreset)
	pint.Exclude = finderList(parsed.Exclude)
	pint.NotName = finderList(parsed.NotName)
	pint.NotPath = finderList(parsed.NotPath)

	return pint, true, nil
}

func parsePintFile(path string, raw []byte) (pintFile, error) {
	var parsed pintFile

	if err := json.Unmarshal(raw, &parsed); err != nil {
		return pintFile{}, config.ParseJSONError(path, raw, err)
	}

	if parsed.Extend == nil {
		return parsed, nil
	}

	merged, err := extendPint(path, *parsed.Extend, raw)
	if err != nil {
		return pintFile{}, err
	}

	var extended pintFile

	if err := json.Unmarshal(merged, &extended); err != nil {
		return pintFile{}, fmt.Errorf("parse %s: %w", path, err)
	}

	return extended, nil
}

func extendPint(path, extend string, raw []byte) ([]byte, error) {
	// Joined without cleaning, as Pint does, so `..` resolves through symlinks.
	basePath := filepath.Dir(path) + string(filepath.Separator) + extend

	baseRaw, err := os.ReadFile(basePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", basePath, err)
	}

	var base, own map[string]any

	if err := json.Unmarshal(baseRaw, &base); err != nil {
		return nil, config.ParseJSONError(basePath, baseRaw, err)
	}

	if base["extend"] != nil {
		return nil, fmt.Errorf("read %s: Pint cannot extend from more than 1 file", basePath)
	}

	if err := json.Unmarshal(raw, &own); err != nil {
		return nil, config.ParseJSONError(path, raw, err)
	}

	merged, err := json.Marshal(replaceRecursive(base, own))
	if err != nil {
		return nil, fmt.Errorf("merge %s into %s: %w", path, basePath, err)
	}

	return merged, nil
}

// replaceRecursive is PHP's array_replace_recursive over decoded JSON: objects
// merge by key and lists by index, at any depth; anything else is replaced.
func replaceRecursive(base, overlay any) any {
	switch overlay := overlay.(type) {
	case map[string]any:
		if base, ok := base.(map[string]any); ok {
			for key, value := range overlay {
				base[key] = replaceRecursive(base[key], value)
			}

			return base
		}
	case []any:
		if base, ok := base.([]any); ok {
			for i, value := range overlay {
				if i < len(base) {
					base[i] = replaceRecursive(base[i], value)
				} else {
					base = append(base, value)
				}
			}

			return base
		}
	}

	return overlay
}

// Without a pint.json, Pint runs its laravel preset, and a project that
// requires it is formatted by it: that is how `laravel new` ships. A
// composer.json that does not parse says nothing about Pint, so it is no error.
func detectComposerPint(root string) (Pint, bool, error) {
	path := filepath.Join(root, "composer.json")

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Pint{}, false, nil
		}

		return Pint{}, false, fmt.Errorf("read %s: %w", path, err)
	}

	var manifest struct {
		Require    map[string]json.RawMessage `json:"require"`
		RequireDev map[string]json.RawMessage `json:"require-dev"`
	}

	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Pint{}, false, nil //nolint:nilerr // formatting composer.json reports it; detection just finds no Pint
	}

	_, dev := manifest.RequireDev["laravel/pint"]
	_, prod := manifest.Require["laravel/pint"]

	if !dev && !prod {
		return Pint{}, false, nil
	}

	return Pint{Path: path, Preset: pintDefaultPreset}, true, nil
}

// Pint casts each Finder option to an array, so a single string is a list.
func finderList(raw json.RawMessage) []string {
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return list
	}

	var single string
	if json.Unmarshal(raw, &single) == nil {
		return []string{single}
	}

	return nil
}

// Of the two brace rule names, braces_position wins, as in Pint. A rule set
// to false switches its behavior off where laravel-fmt has an off state; a
// spacing rule has none, so disabling it keeps laravel-fmt's own, which Pint
// with the rule off accepts too.
func rulesToPint(rules map[string]json.RawMessage) Pint {
	var pint Pint

	for _, key := range []string{"curly_braces_position", "braces_position"} {
		// The nil-guard keeps a malformed braces_position from clobbering a
		// good curly_braces_position decoded on the previous iteration.
		if cbp := rule[CurlyBracesPosition](rules, key); cbp != nil {
			pint.CurlyBracesPosition = cbp
		}
	}

	pint.ConcatSpace = rule[ConcatSpace](rules, "concat_space")
	pint.NotOperatorWithSuccessorSpace = rule[bool](rules, "not_operator_with_successor_space")
	pint.FunctionDeclaration = rule[FunctionDeclaration](rules, "function_declaration")
	pint.BinaryOperatorSpaces = rule[BinaryOperatorSpaces](rules, "binary_operator_spaces")
	pint.BlankLineBeforeStatement = rule[BlankLineBeforeStatement](rules, "blank_line_before_statement")
	pint.OrderedImports = rule[OrderedImports](rules, "ordered_imports")
	pint.ArraySyntax = rule[ArraySyntax](rules, "array_syntax")

	if on := ruleEnabled(rules, "blank_line_before_statement"); on != nil && !*on {
		pint.BlankLineBeforeStatement = &BlankLineBeforeStatement{}
	}

	if blade := rule[LaravelBladeOptions](rules, "Pint/laravel_blade"); blade != nil {
		pint.BladeBlankLineBeforeStatement = blade.BlankLineBeforeStatement
	}

	pint.SortImports = ruleEnabled(rules, "ordered_imports")
	pint.SingleImportPerStatement = ruleEnabled(rules, "single_import_per_statement")

	if opts := rule[SingleImportOptions](rules, "single_import_per_statement"); opts != nil {
		pint.GroupToSingleImports = opts.GroupToSingleImports
	}
	pint.SingleTraitInsertPerStatement = ruleEnabled(rules, "single_trait_insert_per_statement")
	pint.NoClosingTag = ruleEnabled(rules, "no_closing_tag")
	pint.NoMultilineDoubleArrow = ruleEnabled(rules, "no_multiline_whitespace_around_double_arrow")

	if body, ok := rules["heredoc_indentation"]; ok {
		if mode := parseHeredocIndentation(body); mode != "" {
			pint.HeredocIndentation = &mode
		}
	}

	return pint
}

// rule returns nil for an absent rule or one not shaped like T, such as true,
// so one odd rule does not abort the file.
func rule[T any](rules map[string]json.RawMessage, key string) *T {
	body, ok := rules[key]
	if !ok {
		return nil
	}

	var v T

	if err := json.Unmarshal(body, &v); err != nil {
		return nil
	}

	return &v
}

func ruleEnabled(rules map[string]json.RawMessage, key string) *bool {
	body, ok := rules[key]
	if !ok {
		return nil
	}

	var enabled bool
	if err := json.Unmarshal(body, &enabled); err == nil {
		return &enabled
	}

	var options map[string]json.RawMessage
	if err := json.Unmarshal(body, &options); err == nil {
		return new(true)
	}

	return nil
}

func parseHeredocIndentation(body json.RawMessage) string {
	var enabled bool

	if err := json.Unmarshal(body, &enabled); err == nil {
		if enabled {
			return config.HeredocIndentStartPlusOne
		}

		return config.HeredocIndentNone
	}

	var obj struct {
		Indentation string `json:"indentation"`
	}

	if err := json.Unmarshal(body, &obj); err == nil {
		switch obj.Indentation {
		case config.HeredocIndentSameAsStart:
			return config.HeredocIndentSameAsStart
		case config.HeredocIndentStartPlusOne, "":
			return config.HeredocIndentStartPlusOne
		}
	}

	return ""
}

// An unknown preset returns the zero Pint. Each preset is a snapshot of Pint's
// presets/Laravel.php, Psr12.php, Symfony.php and Per.php.
func presetPint(name string) Pint {
	switch name {
	case "laravel":
		notOperatorSpace := true
		heredoc := config.HeredocIndentNone
		doubleArrow := true

		return Pint{
			CurlyBracesPosition: &CurlyBracesPosition{
				FunctionsOpeningBrace:         pintBraceNextLine,
				ClassesOpeningBrace:           pintBraceNextLine,
				AnonymousClassesOpeningBrace:  pintBraceNextLine,
				ControlStructuresOpeningBrace: pintBraceSameLine,
			},
			ConcatSpace:                   &ConcatSpace{Spacing: pintSpacingNone},
			NotOperatorWithSuccessorSpace: &notOperatorSpace,
			FunctionDeclaration:           &FunctionDeclaration{ClosureFnSpacing: pintSpacingOne},
			BlankLineBeforeStatement: &BlankLineBeforeStatement{
				Statements: []string{"continue", "return"},
			},
			NoMultilineDoubleArrow: &doubleArrow,
			HeredocIndentation:     &heredoc,
			// Only Laravel's preset sets an import order; the others keep the
			// default.
			OrderedImports: &OrderedImports{
				ImportsOrder: []string{config.ImportKindConst, config.ImportKindClass, config.ImportKindFunction},
			},
		}
	case "psr12":
		notOperatorSpace := false
		heredoc := config.HeredocIndentNone

		return Pint{
			CurlyBracesPosition: &CurlyBracesPosition{
				FunctionsOpeningBrace:         pintBraceNextLine,
				ClassesOpeningBrace:           pintBraceNextLine,
				AnonymousClassesOpeningBrace:  pintBraceNextLine,
				ControlStructuresOpeningBrace: pintBraceSameLine,
			},
			ConcatSpace:                   &ConcatSpace{Spacing: pintSpacingNone},
			NotOperatorWithSuccessorSpace: &notOperatorSpace,
			FunctionDeclaration:           &FunctionDeclaration{ClosureFnSpacing: pintSpacingOne},
			HeredocIndentation:            &heredoc,
			SingleTraitInsertPerStatement: new(true),
		}
	case "symfony":
		notOperatorSpace := false
		heredoc := config.HeredocIndentNone
		doubleArrow := true

		return Pint{
			CurlyBracesPosition: &CurlyBracesPosition{
				FunctionsOpeningBrace:         pintBraceNextLine,
				ClassesOpeningBrace:           pintBraceNextLine,
				AnonymousClassesOpeningBrace:  pintBraceNextLine,
				ControlStructuresOpeningBrace: pintBraceSameLine,
			},
			ConcatSpace:                   &ConcatSpace{Spacing: pintSpacingNone},
			NotOperatorWithSuccessorSpace: &notOperatorSpace,
			FunctionDeclaration:           &FunctionDeclaration{ClosureFnSpacing: pintSpacingOne},
			BlankLineBeforeStatement: &BlankLineBeforeStatement{
				Statements: []string{"return"},
			},
			HeredocIndentation:            &heredoc,
			NoMultilineDoubleArrow:        &doubleArrow,
			SingleTraitInsertPerStatement: new(true),
		}
	case "per":
		notOperatorSpace := false
		heredoc := config.HeredocIndentNone

		return Pint{
			CurlyBracesPosition: &CurlyBracesPosition{
				FunctionsOpeningBrace:         pintBraceNextLine,
				ClassesOpeningBrace:           pintBraceNextLine,
				AnonymousClassesOpeningBrace:  pintBraceNextLine,
				ControlStructuresOpeningBrace: pintBraceSameLine,
			},
			ConcatSpace:                   &ConcatSpace{Spacing: pintSpacingNone},
			NotOperatorWithSuccessorSpace: &notOperatorSpace,
			FunctionDeclaration:           &FunctionDeclaration{ClosureFnSpacing: pintSpacingOne},
			HeredocIndentation:            &heredoc,
			SingleTraitInsertPerStatement: new(true),
		}
	}

	return Pint{}
}

func applyPint(cfg *config.File, p *Pint) {
	effective := presetPint(p.Preset)
	mergePint(&effective, p)

	applyCurlyBracesPosition(cfg, effective.CurlyBracesPosition)
	applyConcatSpace(cfg, effective.ConcatSpace)
	applyNotOperatorSpace(cfg, effective.NotOperatorWithSuccessorSpace)
	applyFunctionDeclaration(cfg, effective.FunctionDeclaration)
	applyBinaryOperatorSpaces(cfg, effective.BinaryOperatorSpaces)
	applyBlankLineBeforeStatement(cfg, effective.BlankLineBeforeStatement)
	applyBladeBlankLineBeforeStatement(cfg, effective.BladeBlankLineBeforeStatement)
	applyHeredocIndentation(cfg, effective.HeredocIndentation)
	applyOrderedImports(cfg, effective.OrderedImports)
	setIfPresent(&cfg.PHP.SortUses, effective.SortImports)
	setIfPresent(&cfg.PHP.ExpandUseGroups, effective.SingleImportPerStatement)

	if keep := effective.GroupToSingleImports; keep != nil && !*keep {
		cfg.PHP.KeepBracedUseGroups = true
	}

	setIfPresent(&cfg.PHP.SplitTraitUses, effective.SingleTraitInsertPerStatement)
	setIfPresent(&cfg.PHP.RemoveTrailingCloseTag, effective.NoClosingTag)
	setIfPresent(&cfg.PHP.ArrowValueOnKeyLine, effective.NoMultilineDoubleArrow)
	applyArraySyntax(cfg, effective.ArraySyntax)
}

// Slices replace rather than merge, as Pint replaces a whole rule body.
func mergePint(base *Pint, overlay *Pint) {
	if overlay == nil {
		return
	}

	base.Path = overlay.Path
	base.Preset = overlay.Preset

	if overlay.CurlyBracesPosition != nil {
		base.CurlyBracesPosition = overlay.CurlyBracesPosition
	}

	if overlay.ConcatSpace != nil {
		base.ConcatSpace = overlay.ConcatSpace
	}

	if overlay.NotOperatorWithSuccessorSpace != nil {
		base.NotOperatorWithSuccessorSpace = overlay.NotOperatorWithSuccessorSpace
	}

	if overlay.FunctionDeclaration != nil {
		base.FunctionDeclaration = overlay.FunctionDeclaration
	}

	if overlay.BinaryOperatorSpaces != nil {
		base.BinaryOperatorSpaces = overlay.BinaryOperatorSpaces
	}

	if overlay.BlankLineBeforeStatement != nil {
		base.BlankLineBeforeStatement = overlay.BlankLineBeforeStatement
	}

	if overlay.BladeBlankLineBeforeStatement != nil {
		base.BladeBlankLineBeforeStatement = overlay.BladeBlankLineBeforeStatement
	}

	if overlay.HeredocIndentation != nil {
		base.HeredocIndentation = overlay.HeredocIndentation
	}

	if overlay.OrderedImports != nil {
		base.OrderedImports = overlay.OrderedImports
	}

	if overlay.ArraySyntax != nil {
		base.ArraySyntax = overlay.ArraySyntax
	}

	overlayPtr(&base.SortImports, overlay.SortImports)
	overlayPtr(&base.SingleImportPerStatement, overlay.SingleImportPerStatement)
	overlayPtr(&base.GroupToSingleImports, overlay.GroupToSingleImports)
	overlayPtr(&base.SingleTraitInsertPerStatement, overlay.SingleTraitInsertPerStatement)
	overlayPtr(&base.NoClosingTag, overlay.NoClosingTag)
	overlayPtr(&base.NoMultilineDoubleArrow, overlay.NoMultilineDoubleArrow)
}

func applyCurlyBracesPosition(cfg *config.File, cbp *CurlyBracesPosition) {
	if cbp == nil {
		return
	}

	apply := func(value string, targets ...*string) {
		var mapped string
		switch value {
		case pintBraceSameLine:
			mapped = config.BraceStyleSameLine
		case pintBraceNextLine:
			mapped = config.BraceStyleNextLine
		default:
			return
		}

		for _, target := range targets {
			*target = mapped
		}
	}

	// PHP-CS-Fixer's functions_opening_brace covers methods too, and anonymous
	// classes have their own rule apart from classes_opening_brace.
	apply(cbp.FunctionsOpeningBrace, &cfg.PHP.FunctionBraceStyle, &cfg.PHP.MethodBraceStyle)
	apply(cbp.ClassesOpeningBrace, &cfg.PHP.ClasslikeBraceStyle)
	apply(cbp.AnonymousClassesOpeningBrace, &cfg.PHP.AnonClassBraceStyle)
	apply(cbp.ControlStructuresOpeningBrace, &cfg.PHP.ControlBraceStyle)
}

func applyConcatSpace(cfg *config.File, cs *ConcatSpace) {
	if cs == nil {
		return
	}

	switch cs.Spacing {
	case pintSpacingOne:
		cfg.PHP.ConcatSpace = true
	case pintSpacingNone:
		cfg.PHP.ConcatSpace = false
	}
}

func applyNotOperatorSpace(cfg *config.File, enabled *bool) {
	if enabled == nil {
		return
	}

	cfg.PHP.NotOperatorSpace = *enabled
}

func applyFunctionDeclaration(cfg *config.File, fd *FunctionDeclaration) {
	if fd == nil {
		return
	}

	switch fd.ClosureFnSpacing {
	case pintSpacingOne:
		cfg.PHP.ClosureFnSpace = true
	case pintSpacingNone:
		cfg.PHP.ClosureFnSpace = false
	}
}

func applyBinaryOperatorSpaces(cfg *config.File, bos *BinaryOperatorSpaces) {
	if bos == nil {
		return
	}

	value, ok := bos.Operators["=>"]

	if !ok {
		return
	}

	switch value {
	case pintAlign, "align_single_space", "align_single_space_minimal", "align_by_scope":
		cfg.PHP.AlignFatArrow = true
		cfg.PHP.PintAlignment = true
	case "single_space", "no_space", "at_least_single_space":
		cfg.PHP.AlignFatArrow = false
		cfg.PHP.PintAlignment = false
	}
}

func applyBlankLineBeforeStatement(cfg *config.File, blbs *BlankLineBeforeStatement) {
	if blbs == nil {
		return
	}

	var kept []string

	for _, stmt := range blbs.Statements {
		if slices.Contains(config.BlankLineStatements, stmt) && !slices.Contains(kept, stmt) {
			kept = append(kept, stmt)
		}
	}

	cfg.PHP.BlankLineBeforeStatements = kept
}

func applyBladeBlankLineBeforeStatement(cfg *config.File, blbs *BlankLineBeforeStatement) {
	if blbs == nil {
		return
	}

	var kept []string

	for _, stmt := range blbs.Statements {
		if slices.Contains(pintBladeStatements, stmt) && !slices.Contains(kept, stmt) {
			kept = append(kept, stmt)
		}
	}

	cfg.Blade.BlankLineBeforeDirectives = kept
}

func applyHeredocIndentation(cfg *config.File, mode *string) {
	if mode == nil {
		return
	}

	cfg.PHP.HeredocIndentation = *mode
}

func applyOrderedImports(cfg *config.File, oi *OrderedImports) {
	if oi == nil || len(oi.ImportsOrder) == 0 {
		return
	}

	cfg.PHP.ImportOrder = oi.ImportsOrder
}

func applyArraySyntax(cfg *config.File, as *ArraySyntax) {
	if as == nil {
		return
	}

	switch as.Syntax {
	case "short":
		cfg.PHP.ShortArray = true
	case "long":
		cfg.PHP.ShortArray = false
	}
}
