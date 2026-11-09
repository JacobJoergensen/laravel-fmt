package detector

import (
	"bytes"
	"cmp"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/upward"
)

const (
	pintBraceSameLine = "same_line"
	pintBraceNextLine = "next_line_unless_newline_at_signature_end"
	pintSpacingOne    = "one"
	pintSpacingNone   = "none"
	pintAlign         = "align"
	pintSingleSpace   = "single_space"
	pintSortAlpha     = "alpha"
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
	SingleLineEmptyBody           *bool

	ClassDefinition *ClassDefinition

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

	AllowSingleLineEmptyAnonClasses bool `json:"allow_single_line_empty_anonymous_classes"`
	AllowSingleLineAnonFunctions    bool `json:"allow_single_line_anonymous_functions"`
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

// BinaryOperatorSpaces is Pint's binary_operator_spaces rule; only what it
// does to => is used.
type BinaryOperatorSpaces struct {
	Default   string            `json:"default"`
	Operators map[string]string `json:"operators"`
}

// BlankLineBeforeStatement is Pint's blank_line_before_statement rule.
type BlankLineBeforeStatement struct {
	Statements []string `json:"statements"`
}

// OrderedImports is Pint's ordered_imports rule. A nil ImportsOrder is the
// fixer's null: no grouping by kind.
type OrderedImports struct {
	ImportsOrder  []string `json:"imports_order"`
	SortAlgorithm string   `json:"sort_algorithm"`
}

type unaryOperatorSpaces struct {
	OnlyDecInc bool `json:"only_dec_inc"`
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

// ClassDefinition is Pint's class_definition rule; only its anonymous class
// option is used.
type ClassDefinition struct {
	SpaceBeforeParenthesis bool `json:"space_before_parenthesis"`
}

// ArraySyntax is Pint's array_syntax rule.
type ArraySyntax struct {
	Syntax string `json:"syntax"`
}

type pintFile struct {
	Extend  *string        `json:"extend"`
	Preset  *string        `json:"preset"`
	Rules   jsontext.Value `json:"rules"`
	Exclude jsontext.Value `json:"exclude"`
	NotName jsontext.Value `json:"notName"`
	NotPath jsontext.Value `json:"notPath"`
}

func detectPint(root string) (Pint, bool, error) {
	path := filepath.Join(root, "pint.json")

	raw, err := os.ReadFile(path)
	if err != nil {
		if upward.Absent(err) {
			return detectComposerPint(root)
		}

		return Pint{}, false, fmt.Errorf("read %s: %w", path, err)
	}

	parsed, err := parsePintFile(path, raw)
	if err != nil {
		return Pint{}, false, err
	}

	preset := pintDefaultPreset
	if parsed.Preset != nil {
		preset = *parsed.Preset
	}

	presetRules, ok := pintPresets[preset]
	if !ok {
		return Pint{}, false, fmt.Errorf("%s: preset %q is not one of laravel, per, psr12, symfony or empty", path, preset)
	}

	rules, err := orderedPintRules(path, parsed.Rules)
	if err != nil {
		return Pint{}, false, err
	}

	pint := resolvePintRules(mergePintRules(presetRules, rules))
	pint.Path = path
	pint.Preset = preset
	pint.Exclude = finderList(parsed.Exclude)
	pint.NotName = finderList(parsed.NotName)
	pint.NotPath = finderList(parsed.NotPath)

	return pint, true, nil
}

func parsePintFile(path string, raw []byte) (pintFile, error) {
	own, err := lastKeyWins(raw)
	if err != nil {
		return pintFile{}, config.ParseJSONError(path, raw, err)
	}

	// Pint reads any JSON array as its config, and a list holds none of its
	// keys; anything that is not an array it refuses.
	switch jsontext.Value(own).Kind() {
	case '[':
		return pintFile{}, nil
	case '{':
	default:
		return pintFile{}, fmt.Errorf("%s: the config must be an object, not %s", path, own)
	}

	var parsed pintFile

	if err := json.Unmarshal(own, &parsed); err != nil {
		return pintFile{}, config.ParseJSONError(path, own, err)
	}

	if parsed.Extend == nil {
		return parsed, nil
	}

	// Joined without cleaning, as Pint does, so `..` resolves through symlinks.
	basePath := filepath.Dir(path) + string(filepath.Separator) + *parsed.Extend

	merged, err := extendPint(path, basePath, own)
	if err != nil {
		return pintFile{}, err
	}

	var extended pintFile

	// pint.json's own values passed the decode above, so a bad value here came
	// from the extended file.
	if err := json.Unmarshal(merged, &extended); err != nil {
		return pintFile{}, config.ParseJSONError(basePath, merged, err)
	}

	return extended, nil
}

func extendPint(path, basePath string, own []byte) ([]byte, error) {
	baseRaw, err := os.ReadFile(basePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", basePath, err)
	}

	baseKept, err := lastKeyWins(baseRaw)
	if err != nil {
		return nil, config.ParseJSONError(basePath, baseRaw, err)
	}

	base := jsontext.Value(baseKept)
	if base.Kind() != '{' {
		return nil, fmt.Errorf("%s: the config must be an object, not %s", basePath, base)
	}

	baseMembers, err := decodeMembers(base)
	if err != nil {
		return nil, config.ParseJSONError(basePath, baseKept, err)
	}

	for _, member := range baseMembers {
		if member.name == "extend" && member.value.Kind() != 'n' {
			return nil, fmt.Errorf("read %s: Pint cannot extend from more than 1 file", basePath)
		}
	}

	merged, err := replaceRecursive(base, own)
	if err != nil {
		return nil, fmt.Errorf("merge %s into %s: %w", path, basePath, err)
	}

	return merged, nil
}

// replaceRecursive is PHP's array_replace_recursive over JSON: objects merge by
// key, keeping base's order with new keys after it, and lists by index, at any
// depth; anything else is replaced. Both values hold no repeated key.
func replaceRecursive(base, overlay jsontext.Value) (jsontext.Value, error) {
	switch {
	case base.Kind() == '{' && overlay.Kind() == '{':
		return replaceMembers(base, overlay)
	case overlay.Kind() == '[':
		// [] is PHP's empty array, which an object is too, so it replaces nothing.
		if base.Kind() == '{' && len(bytes.TrimSpace(overlay[1:len(overlay)-1])) == 0 {
			return base, nil
		}

		if base.Kind() == '[' {
			return replaceElements(base, overlay)
		}
	}

	return overlay, nil
}

func replaceMembers(base, overlay jsontext.Value) (jsontext.Value, error) {
	baseMembers, err := decodeMembers(base)
	if err != nil {
		return nil, err
	}

	overlayMembers, err := decodeMembers(overlay)
	if err != nil {
		return nil, err
	}

	for _, member := range overlayMembers {
		at := slices.IndexFunc(baseMembers, func(m jsonMember) bool { return m.name == member.name })
		if at < 0 {
			baseMembers = append(baseMembers, member)

			continue
		}

		if baseMembers[at].value, err = replaceRecursive(baseMembers[at].value, member.value); err != nil {
			return nil, err
		}
	}

	return encodeMembers(baseMembers)
}

func replaceElements(base, overlay jsontext.Value) (jsontext.Value, error) {
	var baseElements, overlayElements []jsontext.Value

	if err := errors.Join(json.Unmarshal(base, &baseElements), json.Unmarshal(overlay, &overlayElements)); err != nil {
		return nil, fmt.Errorf("decode list: %w", err)
	}

	for i, element := range overlayElements {
		if i >= len(baseElements) {
			baseElements = append(baseElements, element)

			continue
		}

		merged, err := replaceRecursive(baseElements[i], element)
		if err != nil {
			return nil, err
		}

		baseElements[i] = merged
	}

	encoded, err := json.Marshal(baseElements)
	if err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}

	return encoded, nil
}

// Without a pint.json, Pint runs its laravel preset, and a project that
// requires it is formatted by it: that is how `laravel new` ships. A
// composer.json that does not parse says nothing about Pint, so it is no error.
func detectComposerPint(root string) (Pint, bool, error) {
	path := filepath.Join(root, "composer.json")

	raw, err := os.ReadFile(path)
	if err != nil {
		if upward.Absent(err) {
			return Pint{}, false, nil
		}

		return Pint{}, false, fmt.Errorf("read %s: %w", path, err)
	}

	kept, err := lastKeyWins(raw)
	if err != nil {
		return Pint{}, false, nil //nolint:nilerr // formatting composer.json reports it; detection just finds no Pint
	}

	var manifest struct {
		Require    map[string]jsontext.Value `json:"require"`
		RequireDev map[string]jsontext.Value `json:"require-dev"`
	}

	if err := json.Unmarshal(kept, &manifest); err != nil {
		return Pint{}, false, nil //nolint:nilerr // formatting composer.json reports it; detection just finds no Pint
	}

	_, dev := manifest.RequireDev["laravel/pint"]
	_, prod := manifest.Require["laravel/pint"]

	if !dev && !prod {
		return Pint{}, false, nil
	}

	pint := resolvePintRules(pintPresets[pintDefaultPreset])
	pint.Path = path
	pint.Preset = pintDefaultPreset

	return pint, true, nil
}

// @ sets need the rules in file order. Pint reads rules with `?? []`, so null
// and [] are no rules; a list of rules is not.
func orderedPintRules(path string, raw jsontext.Value) ([]pintRule, error) {
	switch raw.Kind() {
	case 0, 'n':
		return nil, nil
	case '[':
		if len(bytes.TrimSpace(raw[1:len(raw)-1])) == 0 {
			return nil, nil
		}
	case '{':
		return decodeOrderedRules(path, raw)
	default:
	}

	return nil, fmt.Errorf("%s: rules must be an object, not %s", path, raw)
}

func decodeOrderedRules(path string, raw jsontext.Value) ([]pintRule, error) {
	dec := jsontext.NewDecoder(bytes.NewReader(raw))

	if _, err := dec.ReadToken(); err != nil {
		return nil, fmt.Errorf("%s: rules: %w", path, err)
	}

	var rules []pintRule

	for dec.PeekKind() == '"' {
		token, err := dec.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("%s: rules: %w", path, err)
		}

		name := token.String()

		body, err := dec.ReadValue()
		if err != nil {
			return nil, fmt.Errorf("%s: rules.%s: %w", path, name, err)
		}

		rules = append(rules, pintRule{name: name, body: bytes.Clone(body)})
	}

	return rules, nil
}

// Pint casts each Finder option to an array, so a single string is a list.
func finderList(raw jsontext.Value) []string {
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

// rules is what resolvePintRules expanded. A rule set to false switches its
// behavior off where laravel-fmt has an off state; a
// spacing rule has none, so disabling it keeps laravel-fmt's own, which Pint
// with the rule off accepts too.
func rulesToPint(rules map[string]jsontext.Value) Pint {
	var pint Pint

	pint.CurlyBracesPosition = ruleOptions(rules, ruleBracesPosition, CurlyBracesPosition{
		FunctionsOpeningBrace:         pintBraceNextLine,
		ClassesOpeningBrace:           pintBraceNextLine,
		AnonymousClassesOpeningBrace:  pintBraceSameLine,
		ControlStructuresOpeningBrace: pintBraceSameLine,

		AllowSingleLineEmptyAnonClasses: true,
		AllowSingleLineAnonFunctions:    true,
	})
	pint.SingleLineEmptyBody = ruleEnabled(rules, ruleSingleLineEmptyBody)
	pint.ClassDefinition = ruleOptions(rules, ruleClassDefinition, ClassDefinition{})

	pint.ConcatSpace = ruleOptions(rules, ruleConcatSpace, ConcatSpace{Spacing: pintSpacingNone})
	pint.NotOperatorWithSuccessorSpace = notOperatorSpace(rules)
	pint.FunctionDeclaration = ruleOptions(rules, ruleFunctionDeclaration, FunctionDeclaration{ClosureFnSpacing: pintSpacingOne})
	pint.BinaryOperatorSpaces = ruleOptions(rules, ruleBinaryOperatorSpaces, BinaryOperatorSpaces{Default: pintSingleSpace})
	pint.BlankLineBeforeStatement = ruleOptions(rules, ruleBlankLineBeforeStatement, BlankLineBeforeStatement{
		Statements: []string{"break", "continue", "declare", "return", "throw", "try"},
	})
	pint.OrderedImports = ruleOptions(rules, ruleOrderedImports, OrderedImports{SortAlgorithm: pintSortAlpha})
	pint.ArraySyntax = ruleOptions(rules, ruleArraySyntax, ArraySyntax{Syntax: "short"})

	if on := ruleEnabled(rules, ruleBlankLineBeforeStatement); on != nil && !*on {
		pint.BlankLineBeforeStatement = &BlankLineBeforeStatement{}
	}

	if blade := rule[LaravelBladeOptions](rules, "Pint/laravel_blade"); blade != nil {
		pint.BladeBlankLineBeforeStatement = blade.BlankLineBeforeStatement
	}

	// laravel-fmt sorts only alphabetically. Under none or length it leaves
	// the order alone, which Pint then keeps.
	pint.SortImports = ruleEnabled(rules, ruleOrderedImports)
	if pint.OrderedImports != nil && pint.OrderedImports.SortAlgorithm != pintSortAlpha {
		pint.SortImports = new(false)
	}

	pint.SingleImportPerStatement = ruleEnabled(rules, ruleSingleImportPerStatement)

	if opts := ruleOptions(rules, ruleSingleImportPerStatement, SingleImportOptions{GroupToSingleImports: new(true)}); opts != nil {
		pint.GroupToSingleImports = opts.GroupToSingleImports
	}
	pint.SingleTraitInsertPerStatement = ruleEnabled(rules, ruleSingleTraitInsert)
	pint.NoClosingTag = ruleEnabled(rules, ruleNoClosingTag)
	pint.NoMultilineDoubleArrow = ruleEnabled(rules, ruleNoMultilineDoubleArrow)

	if body, ok := rules[ruleHeredocIndentation]; ok {
		if mode := parseHeredocIndentation(body); mode != "" {
			pint.HeredocIndentation = &mode
		}
	}

	return pint
}

// rule returns nil for an absent rule or one not shaped like T, such as true,
// so one odd rule does not abort the file.
func rule[T any](rules map[string]jsontext.Value, key string) *T {
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

// ruleOptions decodes a rule's body over the fixer's own defaults, since Pint
// replaces a whole body and an option left out takes the fixer default, not
// the preset's value.
func ruleOptions[T any](rules map[string]jsontext.Value, key string, defaults T) *T {
	body, ok := rules[key]
	if !ok {
		return nil
	}

	if body.Kind() == 't' {
		return &defaults
	}

	if body.Kind() != '{' {
		return nil
	}

	if err := json.Unmarshal(body, &defaults); err != nil {
		return nil
	}

	return &defaults
}

// unary_operator_spaces strips the space after !, and
// not_operator_with_successor_space, which runs after it, puts one back.
func notOperatorSpace(rules map[string]jsontext.Value) *bool {
	notOperator := ruleEnabled(rules, ruleNotOperatorSpace)
	if notOperator != nil && *notOperator {
		return notOperator
	}

	if unary := ruleOptions(rules, ruleUnaryOperatorSpaces, unaryOperatorSpaces{}); unary != nil && !unary.OnlyDecInc {
		return new(false)
	}

	return notOperator
}

func ruleEnabled(rules map[string]jsontext.Value, key string) *bool {
	body, ok := rules[key]
	if !ok {
		return nil
	}

	var enabled bool
	if err := json.Unmarshal(body, &enabled); err == nil {
		return &enabled
	}

	var options map[string]jsontext.Value
	if err := json.Unmarshal(body, &options); err == nil {
		return new(true)
	}

	return nil
}

func parseHeredocIndentation(body jsontext.Value) string {
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

func applyPint(cfg *config.File, p *Pint) {
	applyCurlyBracesPosition(cfg, p.CurlyBracesPosition)
	applyConcatSpace(cfg, p.ConcatSpace)
	applyNotOperatorSpace(cfg, p.NotOperatorWithSuccessorSpace)
	applyFunctionDeclaration(cfg, p.FunctionDeclaration)
	applyBinaryOperatorSpaces(cfg, p.BinaryOperatorSpaces)
	applyBlankLineBeforeStatement(cfg, p.BlankLineBeforeStatement)
	applyBladeBlankLineBeforeStatement(cfg, p.BladeBlankLineBeforeStatement)
	applyHeredocIndentation(cfg, p.HeredocIndentation)
	applyOrderedImports(cfg, p.OrderedImports)
	setIfPresent(&cfg.PHP.SortUses, p.SortImports)
	setIfPresent(&cfg.PHP.ExpandUseGroups, p.SingleImportPerStatement)

	if keep := p.GroupToSingleImports; keep != nil && !*keep {
		cfg.PHP.KeepBracedUseGroups = true
	}

	setIfPresent(&cfg.PHP.SplitTraitUses, p.SingleTraitInsertPerStatement)
	setIfPresent(&cfg.PHP.RemoveTrailingCloseTag, p.NoClosingTag)
	setIfPresent(&cfg.PHP.ArrowValueOnKeyLine, p.NoMultilineDoubleArrow)
	applyArraySyntax(cfg, p.ArraySyntax)
	applyEmptyBodies(cfg, p)

	if p.ClassDefinition != nil {
		cfg.PHP.AnonClassParenSpace = p.ClassDefinition.SpaceBeforeParenthesis
	}
}

// single_line_empty_body runs after braces_position and keeps every empty body
// on one line. Without it, braces_position puts an empty body's braces on lines
// of their own, unless an allow_single_line option keeps one; with neither
// rule, Pint leaves empty bodies as written.
func applyEmptyBodies(cfg *config.File, p *Pint) {
	if on := p.SingleLineEmptyBody; on != nil && *on {
		cfg.PHP.InlineEmptyFunctionBodies = true
		cfg.PHP.InlineEmptyMethodBodies = true
		cfg.PHP.InlineEmptyClassBodies = true
		cfg.PHP.InlineEmptyAnonClassBodies = true
		cfg.PHP.InlineEmptyClosureBodies = true

		return
	}

	cbp := p.CurlyBracesPosition
	if cbp == nil {
		return
	}

	cfg.PHP.InlineEmptyFunctionBodies = false
	cfg.PHP.InlineEmptyMethodBodies = false
	cfg.PHP.InlineEmptyClassBodies = false
	cfg.PHP.InlineEmptyAnonClassBodies = cbp.AllowSingleLineEmptyAnonClasses
	cfg.PHP.InlineEmptyClosureBodies = cbp.AllowSingleLineAnonFunctions
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
		value = bos.Default
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
	if oi == nil {
		return
	}

	order := oi.ImportsOrder
	if order == nil {
		order = []string{}
	}

	cfg.PHP.ImportOrder = order
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

// Order matters: a later entry replaces an earlier one's whole body, as PHP's
// array_merge does, and a set expands where it is listed.
type pintRule struct {
	name string
	body jsontext.Value
}

var (
	ruleOn  = jsontext.Value("true")
	ruleOff = jsontext.Value("false")
)

const (
	ruleArraySyntax              = "array_syntax"
	ruleBinaryOperatorSpaces     = "binary_operator_spaces"
	ruleBlankLineBeforeStatement = "blank_line_before_statement"
	ruleBracesPosition           = "braces_position"
	ruleClassDefinition          = "class_definition"
	ruleConcatSpace              = "concat_space"
	ruleFunctionDeclaration      = "function_declaration"
	ruleHeredocIndentation       = "heredoc_indentation"
	ruleNoClosingTag             = "no_closing_tag"
	ruleNoMultilineDoubleArrow   = "no_multiline_whitespace_around_double_arrow"
	ruleNotOperatorSpace         = "not_operator_with_successor_space"
	ruleOrderedImports           = "ordered_imports"
	ruleSingleImportPerStatement = "single_import_per_statement"
	ruleSingleLineEmptyBody      = "single_line_empty_body"
	ruleSingleTraitInsert        = "single_trait_insert_per_statement"
	ruleUnaryOperatorSpaces      = "unary_operator_spaces"

	setPSR2       = "@PSR2"
	setPSR12      = "@PSR12"
	setPERCS1x0   = "@PER-CS1x0"
	setPERCS2x0   = "@PER-CS2x0"
	setPERCS3x0   = "@PER-CS3x0"
	setPERCS      = "@PER-CS"
	setSymfony    = "@Symfony"
	setPhpCsFixer = "@PhpCsFixer"
)

// Pint's presets and the PHP-CS-Fixer v3.95.25 sets they build on, keeping
// only the rules and options laravel-fmt models; a body whose modeled options
// are all the fixer's defaults is written true. No preset runs
// heredoc_indentation, so each one leaves heredocs as written.
var pintPresets = map[string][]pintRule{
	"laravel": {
		{ruleHeredocIndentation, ruleOff},
		{ruleArraySyntax, jsontext.Value(`{"syntax":"short"}`)},
		{ruleBinaryOperatorSpaces, jsontext.Value(`{"default":"single_space"}`)},
		{ruleBlankLineBeforeStatement, jsontext.Value(`{"statements":["continue","return"]}`)},
		{ruleBracesPosition, jsontext.Value(`{"control_structures_opening_brace":"same_line",` +
			`"functions_opening_brace":"next_line_unless_newline_at_signature_end",` +
			`"classes_opening_brace":"next_line_unless_newline_at_signature_end",` +
			`"anonymous_classes_opening_brace":"next_line_unless_newline_at_signature_end",` +
			`"allow_single_line_empty_anonymous_classes":false,"allow_single_line_anonymous_functions":false}`)},
		{ruleClassDefinition, ruleOn},
		{ruleConcatSpace, jsontext.Value(`{"spacing":"none"}`)},
		{ruleFunctionDeclaration, ruleOn},
		{ruleNoClosingTag, ruleOn},
		{ruleNoMultilineDoubleArrow, ruleOn},
		{ruleNotOperatorSpace, ruleOn},
		{ruleOrderedImports, jsontext.Value(`{"sort_algorithm":"alpha","imports_order":["const","class","function"]}`)},
		{ruleSingleImportPerStatement, ruleOn},
		{ruleSingleLineEmptyBody, ruleOn},
		{ruleUnaryOperatorSpaces, ruleOn},
	},
	"per":     {{ruleHeredocIndentation, ruleOff}, {setPERCS, ruleOn}},
	"psr12":   {{ruleHeredocIndentation, ruleOff}, {setPSR12, ruleOn}},
	"symfony": {{ruleHeredocIndentation, ruleOff}, {setSymfony, ruleOn}},
	"empty":   {{ruleHeredocIndentation, ruleOff}},
}

var pintRuleSets = map[string][]pintRule{
	setPSR2: {
		{ruleBracesPosition, jsontext.Value(`{"allow_single_line_anonymous_functions":false}`)},
		{ruleClassDefinition, ruleOn},
		{ruleFunctionDeclaration, jsontext.Value(`{"closure_fn_spacing":"one"}`)},
		{ruleNoClosingTag, ruleOn},
		{ruleSingleImportPerStatement, ruleOn},
	},
	setPSR12: {
		{setPSR2, ruleOn},
		{ruleBinaryOperatorSpaces, jsontext.Value(`{"default":"at_least_single_space"}`)},
		{ruleBracesPosition, jsontext.Value(`{"allow_single_line_anonymous_functions":false,` +
			`"allow_single_line_empty_anonymous_classes":true}`)},
		{ruleClassDefinition, jsontext.Value(`{"space_before_parenthesis":true}`)},
		{ruleOrderedImports, jsontext.Value(`{"imports_order":["class","function","const"],"sort_algorithm":"none"}`)},
		{ruleSingleImportPerStatement, jsontext.Value(`{"group_to_single_imports":false}`)},
		{ruleSingleTraitInsert, ruleOn},
		{ruleUnaryOperatorSpaces, jsontext.Value(`{"only_dec_inc":true}`)},
	},
	setPERCS1x0: {{setPSR12, ruleOn}},
	setPERCS2x0: {
		{setPERCS1x0, ruleOn},
		{ruleArraySyntax, ruleOn},
		{ruleConcatSpace, jsontext.Value(`{"spacing":"one"}`)},
		{ruleFunctionDeclaration, jsontext.Value(`{"closure_fn_spacing":"none"}`)},
		{ruleSingleLineEmptyBody, ruleOn},
	},
	setPERCS3x0: {{setPERCS2x0, ruleOn}},
	setPERCS:    {{setPERCS3x0, ruleOn}},
	setSymfony: {
		{setPERCS3x0, ruleOn},
		{ruleBinaryOperatorSpaces, ruleOn},
		{ruleBlankLineBeforeStatement, jsontext.Value(`{"statements":["return"]}`)},
		{ruleBracesPosition, jsontext.Value(`{"allow_single_line_anonymous_functions":true,` +
			`"allow_single_line_empty_anonymous_classes":true}`)},
		{ruleClassDefinition, ruleOn},
		{ruleConcatSpace, ruleOn},
		{ruleFunctionDeclaration, jsontext.Value(`{"closure_fn_spacing":"one"}`)},
		{ruleNoMultilineDoubleArrow, ruleOn},
		{ruleOrderedImports, jsontext.Value(`{"imports_order":["class","function","const"],"sort_algorithm":"alpha"}`)},
		{ruleSingleImportPerStatement, ruleOn},
		{ruleSingleLineEmptyBody, ruleOff},
		{ruleUnaryOperatorSpaces, ruleOn},
	},
	setPhpCsFixer: {
		{setPERCS, ruleOn},
		{setSymfony, ruleOn},
		{ruleBlankLineBeforeStatement, jsontext.Value(`{"statements":["break","case","continue","declare",` +
			`"default","exit","goto","include","include_once","phpdoc","require","require_once","return",` +
			`"switch","throw","try","yield","yield_from"]}`)},
		{ruleSingleLineEmptyBody, ruleOn},
	},
}

var pintRuleAliases = map[string]string{
	"curly_braces_position": ruleBracesPosition,
}

var pintRuleSetAliases = map[string]string{
	"@PER":       setPERCS,
	"@PER-CS1.0": setPERCS1x0,
	"@PER-CS2.0": setPERCS2x0,
	"@PER-CS3.0": setPERCS3x0,
}

func resolvePintRules(rules []pintRule) Pint {
	effective := map[string]jsontext.Value{}
	expandPintRules(effective, rules, nil)

	return rulesToPint(effective)
}

// A set written false switches off every rule it holds, so its rules take the
// set's body in place of their own when override is set.
func expandPintRules(into map[string]jsontext.Value, rules []pintRule, override jsontext.Value) {
	for _, r := range rules {
		body := r.body
		if override != nil {
			body = override
		}

		if !strings.HasPrefix(r.name, "@") {
			into[cmp.Or(pintRuleAliases[r.name], r.name)] = body

			continue
		}

		name := r.name
		if alias, ok := pintRuleSetAliases[name]; ok {
			name = alias
		}

		var setOverride jsontext.Value
		if body.Kind() == 'f' {
			setOverride = ruleOff
		}

		expandPintRules(into, pintRuleSets[name], setOverride)
	}
}

// PHP's array_merge: a rule the preset names keeps its place with pint.json's
// body, and only new rules go to the end. The place decides which body a later
// @ set overrides.
func mergePintRules(preset, own []pintRule) []pintRule {
	merged := slices.Clone(preset)

	for _, r := range own {
		at := slices.IndexFunc(merged, func(existing pintRule) bool { return existing.name == r.name })
		if at < 0 {
			merged = append(merged, r)

			continue
		}

		merged[at].body = r.body
	}

	return merged
}
