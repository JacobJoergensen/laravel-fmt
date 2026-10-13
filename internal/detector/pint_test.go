package detector

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestDetectPintParsesCurlyBracesPositionObject(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`{
  "preset": "laravel",
  "rules": {
    "curly_braces_position": {
      "functions_opening_brace": "same_line",
      "classes_opening_brace": "next_line_unless_newline_at_signature_end",
      "anonymous_classes_opening_brace": "same_line",
      "control_structures_opening_brace": "same_line"
    }
  }
}`)

	seedFileAt(t, dir, "pint.json", string(raw))

	pint, found, err := detectPint(dir)
	if err != nil {
		t.Fatalf("detectPint: %v", err)
	}

	if !found {
		t.Fatal("detectPint reported not found, want found")
	}

	if pint.CurlyBracesPosition == nil {
		t.Fatal("CurlyBracesPosition is nil, want populated")
	}

	cbp := pint.CurlyBracesPosition

	if cbp.FunctionsOpeningBrace != pintBraceSameLine {
		t.Errorf("FunctionsOpeningBrace = %q, want %q", cbp.FunctionsOpeningBrace, pintBraceSameLine)
	}

	if cbp.ClassesOpeningBrace != pintBraceNextLine {
		t.Errorf("ClassesOpeningBrace = %q, want %q", cbp.ClassesOpeningBrace, pintBraceNextLine)
	}

	if cbp.AnonymousClassesOpeningBrace != pintBraceSameLine {
		t.Errorf("AnonymousClassesOpeningBrace = %q, want %q", cbp.AnonymousClassesOpeningBrace, pintBraceSameLine)
	}

	if cbp.ControlStructuresOpeningBrace != pintBraceSameLine {
		t.Errorf("ControlStructuresOpeningBrace = %q, want %q", cbp.ControlStructuresOpeningBrace, pintBraceSameLine)
	}
}

func TestDetectPintParsesOrderedImports(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`{
  "preset": "laravel",
  "rules": {
    "ordered_imports": {
      "sort_algorithm": "alpha",
      "imports_order": ["class", "function", "const"]
    }
  }
}`)

	seedFileAt(t, dir, "pint.json", string(raw))

	pint, found, err := detectPint(dir)
	if err != nil {
		t.Fatalf("detectPint: %v", err)
	}

	if !found {
		t.Fatal("detectPint reported not found, want found")
	}

	if pint.OrderedImports == nil {
		t.Fatal("OrderedImports is nil, want populated")
	}

	order := pint.OrderedImports.ImportsOrder

	if len(order) != 3 || order[0] != config.ImportKindClass || order[1] != config.ImportKindFunction || order[2] != config.ImportKindConst {
		t.Errorf("ImportsOrder = %v, want [class function const]", order)
	}
}

func TestDetectPintToleratesNonObjectRuleBody(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"rule set to true", `{"rules":{"curly_braces_position":true}}`},
		{"rule set to false", `{"rules":{"curly_braces_position":false}}`},
		{"rule set to string", `{"rules":{"curly_braces_position":"default"}}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			seedFileAt(t, dir, "pint.json", tc.raw)

			pint, found, err := detectPint(dir)
			if err != nil {
				t.Fatalf("detectPint returned error for non-object rule body: %v", err)
			}

			if !found {
				t.Fatal("detectPint reported not found, want found")
			}

			if pint.CurlyBracesPosition != nil {
				t.Fatalf("CurlyBracesPosition = %+v, want nil when rule body is non-object", pint.CurlyBracesPosition)
			}
		})
	}
}

func TestDetectPintNilWhenRuleAbsent(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"rules present without curly_braces_position", `{"rules":{"binary_operator_spaces":true}}`},
		{"no rules key", `{"preset":"laravel"}`},
		{"empty object", `{}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			seedFileAt(t, dir, "pint.json", tc.raw)

			pint, found, err := detectPint(dir)
			if err != nil {
				t.Fatalf("detectPint: %v", err)
			}

			if !found {
				t.Fatal("detectPint reported not found, want found")
			}

			if pint.CurlyBracesPosition != nil {
				t.Fatalf("CurlyBracesPosition = %+v, want nil", pint.CurlyBracesPosition)
			}
		})
	}
}

func TestDetectPintMissingFile(t *testing.T) {
	dir := t.TempDir()
	pint, found, err := detectPint(dir)
	if err != nil {
		t.Fatalf("detectPint: %v", err)
	}

	if found {
		t.Fatal("detectPint reported found, want not found")
	}

	if !reflect.DeepEqual(pint, Pint{}) {
		t.Fatalf("Pint = %+v, want zero value", pint)
	}
}

func TestDetectPintParsesNewRules(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`{
  "rules": {
    "concat_space": {"spacing": "none"},
    "not_operator_with_successor_space": true,
    "function_declaration": {"closure_fn_spacing": "one"},
    "binary_operator_spaces": {"operators": {"=>": "align"}}
  }
}`)

	seedFileAt(t, dir, "pint.json", string(raw))

	pint, found, err := detectPint(dir)
	if err != nil {
		t.Fatalf("detectPint: %v", err)
	}

	if !found {
		t.Fatal("detectPint reported not found, want found")
	}

	if pint.ConcatSpace == nil || pint.ConcatSpace.Spacing != pintSpacingNone {
		t.Errorf("ConcatSpace = %+v, want spacing=none", pint.ConcatSpace)
	}

	if pint.NotOperatorWithSuccessorSpace == nil || !*pint.NotOperatorWithSuccessorSpace {
		t.Errorf("NotOperatorWithSuccessorSpace = %+v, want true", pint.NotOperatorWithSuccessorSpace)
	}

	if pint.FunctionDeclaration == nil || pint.FunctionDeclaration.ClosureFnSpacing != pintSpacingOne {
		t.Errorf("FunctionDeclaration = %+v, want closure_fn_spacing=one", pint.FunctionDeclaration)
	}

	if pint.BinaryOperatorSpaces == nil || pint.BinaryOperatorSpaces.Operators["=>"] != pintAlign {
		t.Errorf("BinaryOperatorSpaces = %+v, want operators[=>]=align", pint.BinaryOperatorSpaces)
	}
}

func TestDetectPintParsesPreset(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"laravel preset", `{"preset":"laravel"}`, "laravel"},
		{"psr12 preset", `{"preset":"psr12"}`, "psr12"},
		{"symfony preset", `{"preset":"symfony"}`, "symfony"},
		{"per preset", `{"preset":"per"}`, "per"},
		{"no preset", `{}`, "laravel"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			seedFileAt(t, dir, "pint.json", tc.raw)

			pint, found, err := detectPint(dir)
			if err != nil {
				t.Fatalf("detectPint: %v", err)
			}

			if !found {
				t.Fatal("detectPint reported not found, want found")
			}

			if pint.Preset != tc.want {
				t.Fatalf("Preset = %q, want %q", pint.Preset, tc.want)
			}
		})
	}
}

func TestDetectPintBracesPositionAlias(t *testing.T) {
	cases := []struct {
		name string
		key  string
	}{
		{"legacy curly_braces_position", "curly_braces_position"},
		{"new braces_position", "braces_position"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			raw := []byte(`{
  "rules": {
    "` + tc.key + `": {
      "functions_opening_brace": "same_line",
      "classes_opening_brace": "same_line",
      "anonymous_classes_opening_brace": "same_line",
      "control_structures_opening_brace": "same_line"
    }
  }
}`)

			seedFileAt(t, dir, "pint.json", string(raw))

			pint, _, err := detectPint(dir)
			if err != nil {
				t.Fatalf("detectPint: %v", err)
			}

			if pint.CurlyBracesPosition == nil {
				t.Fatalf("CurlyBracesPosition nil for %q, want populated", tc.key)
			}

			cbp := pint.CurlyBracesPosition

			if cbp.FunctionsOpeningBrace != pintBraceSameLine || cbp.ClassesOpeningBrace != pintBraceSameLine ||
				cbp.AnonymousClassesOpeningBrace != pintBraceSameLine || cbp.ControlStructuresOpeningBrace != pintBraceSameLine {
				t.Fatalf("CurlyBracesPosition fields not populated from %q: %+v", tc.key, cbp)
			}
		})
	}
}

func TestDetectPintParsesBlankLineBeforeStatement(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`{"rules":{"blank_line_before_statement":{"statements":["continue","return"]}}}`)

	seedFileAt(t, dir, "pint.json", string(raw))

	pint, _, err := detectPint(dir)
	if err != nil {
		t.Fatalf("detectPint: %v", err)
	}

	if pint.BlankLineBeforeStatement == nil {
		t.Fatal("BlankLineBeforeStatement nil, want populated")
	}

	want := []string{"continue", "return"}
	got := pint.BlankLineBeforeStatement.Statements

	if len(got) != len(want) {
		t.Fatalf("Statements length = %d, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Statements[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPresetPintLaravelDefaults(t *testing.T) {
	preset := presetPint("laravel")

	if preset.ConcatSpace == nil || preset.ConcatSpace.Spacing != pintSpacingNone {
		t.Errorf("ConcatSpace = %+v, want spacing=none", preset.ConcatSpace)
	}

	if preset.NotOperatorWithSuccessorSpace == nil || !*preset.NotOperatorWithSuccessorSpace {
		t.Errorf("NotOperatorWithSuccessorSpace = %+v, want true", preset.NotOperatorWithSuccessorSpace)
	}

	if preset.FunctionDeclaration == nil || preset.FunctionDeclaration.ClosureFnSpacing != pintSpacingOne {
		t.Errorf("FunctionDeclaration = %+v, want closure_fn_spacing=one", preset.FunctionDeclaration)
	}

	if preset.BlankLineBeforeStatement == nil {
		t.Fatal("BlankLineBeforeStatement nil, want populated for laravel preset")
	}

	stmts := preset.BlankLineBeforeStatement.Statements

	if len(stmts) != 2 || stmts[0] != "continue" || stmts[1] != "return" {
		t.Errorf("BlankLineBeforeStatement.Statements = %v, want [continue return]", stmts)
	}

	if preset.OrderedImports == nil {
		t.Fatal("OrderedImports nil, want populated for laravel preset")
	}

	order := preset.OrderedImports.ImportsOrder

	if len(order) != 3 || order[0] != config.ImportKindConst || order[1] != config.ImportKindClass || order[2] != config.ImportKindFunction {
		t.Errorf("OrderedImports.ImportsOrder = %v, want [const class function]", order)
	}
}

func TestPresetPintUnknownReturnsZero(t *testing.T) {
	got := presetPint("not-a-real-preset")

	if got.ConcatSpace != nil || got.NotOperatorWithSuccessorSpace != nil ||
		got.FunctionDeclaration != nil || got.BinaryOperatorSpaces != nil ||
		got.BlankLineBeforeStatement != nil || got.CurlyBracesPosition != nil ||
		got.OrderedImports != nil {
		t.Fatalf("unknown preset returned non-zero Pint: %+v", got)
	}
}

func TestDetectPintMalformedJSONErrors(t *testing.T) {
	dir := t.TempDir()

	seedFileAt(t, dir, "pint.json", "{not json")

	_, _, err := detectPint(dir)

	if err == nil || !strings.Contains(err.Error(), "pint.json:1:2: invalid character 'n'") {
		t.Fatalf("detectPint err = %v, want the malformed byte located as pint.json:1:2", err)
	}
}

func TestDetectPintHeredocIndentation(t *testing.T) {
	tests := []struct {
		name string
		rule string
		want string
	}{
		{"true enables start_plus_one", `true`, config.HeredocIndentStartPlusOne},
		{"false disables", `false`, config.HeredocIndentNone},
		{"object same_as_start", `{"indentation": "same_as_start"}`, config.HeredocIndentSameAsStart},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			body := `{"rules": {"heredoc_indentation": ` + tc.rule + `}}`

			seedFileAt(t, dir, "pint.json", body)

			pint, _, err := detectPint(dir)
			if err != nil {
				t.Fatalf("detectPint: %v", err)
			}

			if pint.HeredocIndentation == nil {
				t.Fatalf("HeredocIndentation = nil, want %q", tc.want)
			}

			if *pint.HeredocIndentation != tc.want {
				t.Errorf("HeredocIndentation = %q, want %q", *pint.HeredocIndentation, tc.want)
			}
		})
	}
}

func TestApplyPintBinaryOperatorSpaces(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{"align flips on", pintAlign, true},
		{"align_single_space flips on", "align_single_space", true},
		{"align_single_space_minimal flips on", "align_single_space_minimal", true},
		{"align_by_scope flips on", "align_by_scope", true},
		{"single_space flips off", "single_space", false},
		{"no_space flips off", "no_space", false},
		{"at_least_single_space flips off", "at_least_single_space", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.PHP.AlignFatArrow = !tc.want

			applyPint(cfg, &Pint{BinaryOperatorSpaces: &BinaryOperatorSpaces{
				Operators: map[string]string{"=>": tc.value},
			}})

			if cfg.PHP.AlignFatArrow != tc.want {
				t.Fatalf("AlignFatArrow = %v, want %v", cfg.PHP.AlignFatArrow, tc.want)
			}
		})
	}

	t.Run("missing => key leaves baseline", func(t *testing.T) {
		cfg := config.Default()
		cfg.PHP.AlignFatArrow = true

		applyPint(cfg, &Pint{BinaryOperatorSpaces: &BinaryOperatorSpaces{
			Operators: map[string]string{"=": "single_space"},
		}})

		if !cfg.PHP.AlignFatArrow {
			t.Fatalf("AlignFatArrow cleared when => entry was absent")
		}
	})

	t.Run("unknown => value leaves baseline", func(t *testing.T) {
		cfg := config.Default()
		cfg.PHP.AlignFatArrow = false

		applyPint(cfg, &Pint{BinaryOperatorSpaces: &BinaryOperatorSpaces{
			Operators: map[string]string{"=>": "definitely_not_a_pint_value"},
		}})

		if cfg.PHP.AlignFatArrow {
			t.Fatalf("AlignFatArrow flipped on unknown value")
		}
	})
}

func TestApplyPintBlankLineBeforeStatement(t *testing.T) {
	cases := []struct {
		name       string
		statements []string
		want       []string
	}{
		{"single return", []string{"return"}, []string{"return"}},
		{"supported kinds keep Pint order", []string{"continue", "return", "throw"}, []string{"continue", "return", "throw"}},
		{"unsupported kinds dropped", []string{"continue", "case", "phpdoc", "throw"}, []string{"continue", "throw"}},
		{"duplicates removed", []string{"return", "return"}, []string{"return"}},
		{"empty list disables", []string{}, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()

			applyPint(cfg, &Pint{BlankLineBeforeStatement: &BlankLineBeforeStatement{
				Statements: tc.statements,
			}})

			if !reflect.DeepEqual(cfg.PHP.BlankLineBeforeStatements, tc.want) {
				t.Fatalf("BlankLineBeforeStatements = %v, want %v", cfg.PHP.BlankLineBeforeStatements, tc.want)
			}
		})
	}

	t.Run("nil leaves baseline", func(t *testing.T) {
		cfg := config.Default()

		applyPint(cfg, &Pint{})

		if want := []string{config.StmtContinue, config.StmtReturn}; !reflect.DeepEqual(cfg.PHP.BlankLineBeforeStatements, want) {
			t.Fatalf("BlankLineBeforeStatements = %v, want baseline %v on nil rule body", cfg.PHP.BlankLineBeforeStatements, want)
		}
	})
}

// Values differ from config.Default(), so a no-op cannot pass.
func TestApplyPintBracesPositionAliasFlowsThrough(t *testing.T) {
	cfg := config.Default()

	applyPint(cfg, &Pint{CurlyBracesPosition: &CurlyBracesPosition{
		FunctionsOpeningBrace:         pintBraceSameLine,
		ClassesOpeningBrace:           pintBraceSameLine,
		ControlStructuresOpeningBrace: pintBraceSameLine,
	}})

	if cfg.PHP.FunctionBraceStyle != config.BraceStyleSameLine ||
		cfg.PHP.MethodBraceStyle != config.BraceStyleSameLine ||
		cfg.PHP.ClasslikeBraceStyle != config.BraceStyleSameLine ||
		cfg.PHP.ControlBraceStyle != config.BraceStyleSameLine {
		t.Fatalf("brace styles not propagated: %+v", cfg.PHP)
	}
}

func TestApplyPintClosureFnSpacing(t *testing.T) {
	cases := []struct {
		name     string
		spacing  string
		baseline bool
		want     bool
	}{
		{"one enables space", pintSpacingOne, false, true},
		{"none disables space", pintSpacingNone, true, false},
		{"unknown leaves baseline", "default", true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.PHP.ClosureFnSpace = tc.baseline

			applyPint(cfg, &Pint{FunctionDeclaration: &FunctionDeclaration{ClosureFnSpacing: tc.spacing}})

			if cfg.PHP.ClosureFnSpace != tc.want {
				t.Fatalf("ClosureFnSpace = %v, want %v", cfg.PHP.ClosureFnSpace, tc.want)
			}
		})
	}
}

func TestApplyPintConcatSpace(t *testing.T) {
	cases := []struct {
		name     string
		spacing  string
		baseline bool
		want     bool
	}{
		{"one enables space", pintSpacingOne, false, true},
		{"none disables space", pintSpacingNone, true, false},
		{"unknown leaves baseline", "weird", true, true},
		{"unknown leaves baseline (false)", "weird", false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.PHP.ConcatSpace = tc.baseline

			applyPint(cfg, &Pint{ConcatSpace: &ConcatSpace{Spacing: tc.spacing}})

			if cfg.PHP.ConcatSpace != tc.want {
				t.Fatalf("ConcatSpace = %v, want %v", cfg.PHP.ConcatSpace, tc.want)
			}
		})
	}

	t.Run("nil leaves baseline", func(t *testing.T) {
		cfg := config.Default()
		cfg.PHP.ConcatSpace = false

		applyPint(cfg, &Pint{})

		if cfg.PHP.ConcatSpace != false {
			t.Fatalf("ConcatSpace flipped on nil rule body: got %v", cfg.PHP.ConcatSpace)
		}
	})
}

func TestApplyPintCurlyBracesPosition(t *testing.T) {
	t.Run("nil CurlyBracesPosition leaves baseline", func(t *testing.T) {
		cfg := config.Default()
		original := cfg.PHP
		pint := &Pint{}

		applyPint(cfg, pint)

		if cfg.PHP.FunctionBraceStyle != original.FunctionBraceStyle ||
			cfg.PHP.MethodBraceStyle != original.MethodBraceStyle ||
			cfg.PHP.ClasslikeBraceStyle != original.ClasslikeBraceStyle ||
			cfg.PHP.ControlBraceStyle != original.ControlBraceStyle {
			t.Fatalf("brace styles changed when CurlyBracesPosition is nil: %+v", cfg.PHP)
		}
	})

	t.Run("same_line maps to BraceStyleSameLine", func(t *testing.T) {
		cfg := config.Default()

		pint := &Pint{CurlyBracesPosition: &CurlyBracesPosition{
			FunctionsOpeningBrace:         pintBraceSameLine,
			ClassesOpeningBrace:           pintBraceSameLine,
			ControlStructuresOpeningBrace: pintBraceSameLine,
		}}

		applyPint(cfg, pint)

		if cfg.PHP.FunctionBraceStyle != config.BraceStyleSameLine {
			t.Errorf("FunctionBraceStyle = %q, want %q", cfg.PHP.FunctionBraceStyle, config.BraceStyleSameLine)
		}

		if cfg.PHP.MethodBraceStyle != config.BraceStyleSameLine {
			t.Errorf("MethodBraceStyle = %q, want %q", cfg.PHP.MethodBraceStyle, config.BraceStyleSameLine)
		}

		if cfg.PHP.ClasslikeBraceStyle != config.BraceStyleSameLine {
			t.Errorf("ClasslikeBraceStyle = %q, want %q", cfg.PHP.ClasslikeBraceStyle, config.BraceStyleSameLine)
		}

		if cfg.PHP.ControlBraceStyle != config.BraceStyleSameLine {
			t.Errorf("ControlBraceStyle = %q, want %q", cfg.PHP.ControlBraceStyle, config.BraceStyleSameLine)
		}
	})

	t.Run("next_line_unless_newline_at_signature_end maps to BraceStyleNextLine", func(t *testing.T) {
		cfg := config.Default()
		cfg.PHP.FunctionBraceStyle = config.BraceStyleSameLine
		cfg.PHP.ClasslikeBraceStyle = config.BraceStyleSameLine

		pint := &Pint{CurlyBracesPosition: &CurlyBracesPosition{
			FunctionsOpeningBrace: pintBraceNextLine,
			ClassesOpeningBrace:   pintBraceNextLine,
		}}

		applyPint(cfg, pint)

		if cfg.PHP.FunctionBraceStyle != config.BraceStyleNextLine {
			t.Errorf("FunctionBraceStyle = %q, want %q", cfg.PHP.FunctionBraceStyle, config.BraceStyleNextLine)
		}

		if cfg.PHP.ClasslikeBraceStyle != config.BraceStyleNextLine {
			t.Errorf("ClasslikeBraceStyle = %q, want %q", cfg.PHP.ClasslikeBraceStyle, config.BraceStyleNextLine)
		}
	})

	t.Run("AnonymousClassesOpeningBrace is ignored; classlike follows ClassesOpeningBrace", func(t *testing.T) {
		cfg := config.Default()
		pint := &Pint{CurlyBracesPosition: &CurlyBracesPosition{
			ClassesOpeningBrace:          pintBraceSameLine,
			AnonymousClassesOpeningBrace: pintBraceNextLine,
		}}

		applyPint(cfg, pint)

		if cfg.PHP.ClasslikeBraceStyle != config.BraceStyleSameLine {
			t.Errorf("ClasslikeBraceStyle = %q, want %q (follows ClassesOpeningBrace, not anonymous)", cfg.PHP.ClasslikeBraceStyle, config.BraceStyleSameLine)
		}
	})

	t.Run("methods follow FunctionsOpeningBrace, not ClassesOpeningBrace", func(t *testing.T) {
		cfg := config.Default()
		pint := &Pint{CurlyBracesPosition: &CurlyBracesPosition{
			FunctionsOpeningBrace: pintBraceSameLine,
			ClassesOpeningBrace:   pintBraceNextLine,
		}}

		applyPint(cfg, pint)

		if cfg.PHP.MethodBraceStyle != config.BraceStyleSameLine {
			t.Errorf("MethodBraceStyle = %q, want %q (methods are functions in PHP-CS-Fixer)", cfg.PHP.MethodBraceStyle, config.BraceStyleSameLine)
		}

		if cfg.PHP.ClasslikeBraceStyle != config.BraceStyleNextLine {
			t.Errorf("ClasslikeBraceStyle = %q, want %q", cfg.PHP.ClasslikeBraceStyle, config.BraceStyleNextLine)
		}
	})

	t.Run("unknown value leaves baseline", func(t *testing.T) {
		cfg := config.Default()
		original := cfg.PHP.FunctionBraceStyle
		pint := &Pint{CurlyBracesPosition: &CurlyBracesPosition{
			FunctionsOpeningBrace: "next_line_at_signature_end",
		}}

		applyPint(cfg, pint)

		if cfg.PHP.FunctionBraceStyle != original {
			t.Fatalf("FunctionBraceStyle changed on unknown Pint value: got %q, want %q", cfg.PHP.FunctionBraceStyle, original)
		}
	})

	t.Run("empty CurlyBracesPosition leaves baseline", func(t *testing.T) {
		cfg := config.Default()
		original := cfg.PHP
		pint := &Pint{CurlyBracesPosition: &CurlyBracesPosition{}}

		applyPint(cfg, pint)

		if !reflect.DeepEqual(cfg.PHP, original) {
			t.Fatalf("PHP config changed with empty CurlyBracesPosition: got %+v, want %+v", cfg.PHP, original)
		}
	})
}

func TestApplyPintDisabledRuleSwitchesItsBehaviorOff(t *testing.T) {
	cases := []struct {
		name  string
		rules string
		check func(p config.PHP) bool
	}{
		{"ordered_imports false stops sorting", `{"ordered_imports": false}`, func(p config.PHP) bool { return !p.SortUses }},
		{"ordered_imports object keeps sorting", `{"ordered_imports": {"sort_algorithm": "alpha"}}`, func(p config.PHP) bool { return p.SortUses }},
		{"single_import_per_statement false keeps groups", `{"single_import_per_statement": false}`, func(p config.PHP) bool { return !p.ExpandUseGroups }},
		{"single_import_per_statement true splits groups", `{"single_import_per_statement": true}`, func(p config.PHP) bool { return p.ExpandUseGroups }},
		{"no_closing_tag false keeps the tag", `{"no_closing_tag": false}`, func(p config.PHP) bool { return !p.RemoveTrailingCloseTag }},
		{"blank_line_before_statement false adds none", `{"blank_line_before_statement": false}`, func(p config.PHP) bool { return len(p.BlankLineBeforeStatements) == 0 }},
		{"concat_space false keeps the preset", `{"concat_space": false}`, func(p config.PHP) bool { return !p.ConcatSpace }},
		{"absent rules keep the preset", `{}`, func(p config.PHP) bool {
			return p.SortUses && p.ExpandUseGroups && p.RemoveTrailingCloseTag && len(p.BlankLineBeforeStatements) == 2
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rules map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.rules), &rules); err != nil {
				t.Fatalf("rules: %v", err)
			}

			pint := rulesToPint(rules)
			pint.Preset = "laravel"

			cfg := config.Default()
			applyPint(cfg, &pint)

			if !tc.check(cfg.PHP) {
				t.Fatalf("rules %s gave %+v", tc.rules, cfg.PHP)
			}
		})
	}
}

func TestApplyPintExplicitOverridesPreset(t *testing.T) {
	disabled := false
	cfg := config.Default()

	applyPint(cfg, &Pint{
		Preset:                        "laravel",
		ConcatSpace:                   &ConcatSpace{Spacing: pintSpacingOne},
		NotOperatorWithSuccessorSpace: &disabled,
		FunctionDeclaration:           &FunctionDeclaration{ClosureFnSpacing: pintSpacingNone},
		BlankLineBeforeStatement:      &BlankLineBeforeStatement{Statements: []string{}},
	})

	if !cfg.PHP.ConcatSpace {
		t.Errorf("ConcatSpace = false, want true (explicit concat_space=one overrides preset none)")
	}

	if cfg.PHP.NotOperatorSpace {
		t.Errorf("NotOperatorSpace = true, want false (explicit false overrides preset true)")
	}

	if cfg.PHP.ClosureFnSpace {
		t.Errorf("ClosureFnSpace = true, want false (explicit none overrides preset one)")
	}

	if len(cfg.PHP.BlankLineBeforeStatements) != 0 {
		t.Errorf("BlankLineBeforeStatements = %v, want empty (explicit empty list overrides preset)", cfg.PHP.BlankLineBeforeStatements)
	}
}

func TestApplyPintHeredocIndentation(t *testing.T) {
	modes := []string{
		config.HeredocIndentStartPlusOne,
		config.HeredocIndentSameAsStart,
		config.HeredocIndentNone,
	}

	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			cfg := config.Default()
			m := mode

			applyPint(cfg, &Pint{HeredocIndentation: &m})

			if cfg.PHP.HeredocIndentation != mode {
				t.Errorf("HeredocIndentation = %q, want %q", cfg.PHP.HeredocIndentation, mode)
			}
		})
	}

	t.Run("absent rule keeps the baseline", func(t *testing.T) {
		cfg := config.Default()

		applyPint(cfg, &Pint{})

		if want := config.Default().PHP.HeredocIndentation; cfg.PHP.HeredocIndentation != want {
			t.Errorf("HeredocIndentation = %q, want baseline %q", cfg.PHP.HeredocIndentation, want)
		}
	})

	// No Pint preset includes heredoc_indentation; PHP-CS-Fixer has it only in
	// @PHP7x3Migration.
	for _, preset := range []string{"laravel", "psr12", "symfony", "per"} {
		t.Run(preset+" preset leaves heredocs alone", func(t *testing.T) {
			cfg := config.Default()
			cfg.PHP.HeredocIndentation = config.HeredocIndentStartPlusOne

			applyPint(cfg, &Pint{Preset: preset})

			if cfg.PHP.HeredocIndentation != config.HeredocIndentNone {
				t.Errorf("%s HeredocIndentation = %q, want none", preset, cfg.PHP.HeredocIndentation)
			}
		})
	}
}

func TestApplyPintLaravelPresetWithoutOverrides(t *testing.T) {
	cfg := config.Default()
	applyPint(cfg, &Pint{Preset: "laravel"})

	if cfg.PHP.ConcatSpace {
		t.Errorf("ConcatSpace = true, want false (laravel preset uses concat_space none)")
	}

	if !cfg.PHP.NotOperatorSpace {
		t.Errorf("NotOperatorSpace = false, want true (laravel preset uses not_operator_with_successor_space true)")
	}

	if !cfg.PHP.ClosureFnSpace {
		t.Errorf("ClosureFnSpace = false, want true (laravel preset uses closure_fn_spacing one)")
	}

	if want := []string{config.StmtContinue, config.StmtReturn}; !reflect.DeepEqual(cfg.PHP.BlankLineBeforeStatements, want) {
		t.Errorf("BlankLineBeforeStatements = %v, want %v (laravel preset blank_line_before_statement)", cfg.PHP.BlankLineBeforeStatements, want)
	}
}

func TestApplyPintNotOperatorSpace(t *testing.T) {
	enabled := true
	disabled := false

	t.Run("enabled flips to true", func(t *testing.T) {
		cfg := config.Default()
		applyPint(cfg, &Pint{NotOperatorWithSuccessorSpace: &enabled})

		if !cfg.PHP.NotOperatorSpace {
			t.Fatalf("NotOperatorSpace = false, want true")
		}
	})

	t.Run("disabled flips to false", func(t *testing.T) {
		cfg := config.Default()
		cfg.PHP.NotOperatorSpace = true

		applyPint(cfg, &Pint{NotOperatorWithSuccessorSpace: &disabled})

		if cfg.PHP.NotOperatorSpace {
			t.Fatalf("NotOperatorSpace = true, want false")
		}
	})

	t.Run("nil leaves baseline", func(t *testing.T) {
		cfg := config.Default()
		cfg.PHP.NotOperatorSpace = true

		applyPint(cfg, &Pint{})

		if !cfg.PHP.NotOperatorSpace {
			t.Fatalf("NotOperatorSpace cleared on nil rule body")
		}
	})
}

func TestApplyPintOrderedImports(t *testing.T) {
	baseline := []string{config.ImportKindConst, config.ImportKindClass, config.ImportKindFunction}

	t.Run("explicit order wins", func(t *testing.T) {
		cfg := config.Default()
		order := []string{config.ImportKindClass, config.ImportKindFunction, config.ImportKindConst}

		applyPint(cfg, &Pint{OrderedImports: &OrderedImports{ImportsOrder: order}})

		if !reflect.DeepEqual(cfg.PHP.ImportOrder, order) {
			t.Errorf("ImportOrder = %v, want %v", cfg.PHP.ImportOrder, order)
		}
	})

	t.Run("absent rule keeps the baseline", func(t *testing.T) {
		cfg := config.Default()

		applyPint(cfg, &Pint{})

		if !reflect.DeepEqual(cfg.PHP.ImportOrder, baseline) {
			t.Errorf("ImportOrder = %v, want baseline %v", cfg.PHP.ImportOrder, baseline)
		}
	})

	t.Run("empty order keeps the baseline", func(t *testing.T) {
		cfg := config.Default()

		applyPint(cfg, &Pint{OrderedImports: &OrderedImports{ImportsOrder: []string{}}})

		if !reflect.DeepEqual(cfg.PHP.ImportOrder, baseline) {
			t.Errorf("ImportOrder = %v, want baseline %v", cfg.PHP.ImportOrder, baseline)
		}
	})

	t.Run("laravel preset reproduces the baseline", func(t *testing.T) {
		cfg := config.Default()

		applyPint(cfg, &Pint{Preset: "laravel"})

		if !reflect.DeepEqual(cfg.PHP.ImportOrder, baseline) {
			t.Errorf("laravel ImportOrder = %v, want %v", cfg.PHP.ImportOrder, baseline)
		}
	})

	t.Run("explicit order overrides preset", func(t *testing.T) {
		cfg := config.Default()
		order := []string{config.ImportKindClass, config.ImportKindFunction, config.ImportKindConst}

		applyPint(cfg, &Pint{Preset: "laravel", OrderedImports: &OrderedImports{ImportsOrder: order}})

		if !reflect.DeepEqual(cfg.PHP.ImportOrder, order) {
			t.Errorf("ImportOrder = %v, want %v (explicit overrides laravel preset)", cfg.PHP.ImportOrder, order)
		}
	})
}

func TestDefaultMatchesLaravelPreset(t *testing.T) {
	cfg := config.Default()

	applyPint(cfg, &Pint{Preset: "laravel"})

	// What Pint itself will rewrite is a fact about a project running Pint, not
	// a default laravel-fmt shares with the preset.
	cfg.PHP.ArrowValueOnKeyLine = false

	if !reflect.DeepEqual(cfg.PHP, config.Default().PHP) {
		t.Errorf("config.Default().PHP drifted from the Pint laravel preset:\napplied = %+v\ndefault = %+v", cfg.PHP, config.Default().PHP)
	}
}

func TestDetectPintParsesArraySyntax(t *testing.T) {
	dir := t.TempDir()

	seedFileAt(t, dir, "pint.json", `{"rules":{"array_syntax":{"syntax":"long"}}}`)

	pint, _, err := detectPint(dir)
	if err != nil {
		t.Fatalf("detectPint: %v", err)
	}

	if pint.ArraySyntax == nil || pint.ArraySyntax.Syntax != "long" {
		t.Fatalf("ArraySyntax = %+v, want syntax=long", pint.ArraySyntax)
	}
}

func TestApplyPintArraySyntax(t *testing.T) {
	cases := []struct {
		name   string
		syntax string
		want   bool
	}{
		{"short keeps short", "short", true},
		{"long disables short", "long", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			applyPint(cfg, &Pint{ArraySyntax: &ArraySyntax{Syntax: tc.syntax}})

			if cfg.PHP.ShortArray != tc.want {
				t.Errorf("ShortArray = %v, want %v", cfg.PHP.ShortArray, tc.want)
			}
		})
	}
}

func TestApplyPintArraySyntaxAbsentKeepsDefault(t *testing.T) {
	cfg := config.Default()
	applyPint(cfg, &Pint{})

	if !cfg.PHP.ShortArray {
		t.Error("ShortArray = false, want true when array_syntax is absent")
	}
}

func TestApplyPintRecordsWhatPintRewritesAroundTheDoubleArrow(t *testing.T) {
	cases := []struct {
		name      string
		pintJSON  string
		onKeyLine bool
		pintAlign bool
	}{
		{"laravel preset joins the value", `{"preset":"laravel"}`, true, false},
		{"symfony preset joins the value", `{"preset":"symfony"}`, true, false},
		{"psr12 preset leaves it", `{"preset":"psr12"}`, false, false},
		{"rule switched off", `{"preset":"laravel","rules":{"no_multiline_whitespace_around_double_arrow":false}}`, false, false},
		{
			"alignment from binary_operator_spaces",
			`{"preset":"psr12","rules":{"binary_operator_spaces":{"operators":{"=>":"align_single_space_minimal"}}}}`,
			false,
			true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seedFileAt(t, dir, "pint.json", tc.pintJSON)

			pint, _, err := detectPint(dir)
			if err != nil {
				t.Fatalf("detectPint: %v", err)
			}

			cfg := config.Default()
			applyPint(cfg, &pint)

			if cfg.PHP.ArrowValueOnKeyLine != tc.onKeyLine || cfg.PHP.PintAlignment != tc.pintAlign {
				t.Errorf("ArrowValueOnKeyLine = %v, PintAlignment = %v; want %v, %v",
					cfg.PHP.ArrowValueOnKeyLine, cfg.PHP.PintAlignment, tc.onKeyLine, tc.pintAlign)
			}
		})
	}
}

func TestPintJSONWithoutAPresetFormatsAsTheLaravelPreset(t *testing.T) {
	configFor := func(t *testing.T, pintJSON string) config.PHP {
		t.Helper()

		dir := t.TempDir()
		seedFileAt(t, dir, "pint.json", pintJSON)

		pint, _, err := detectPint(dir)
		if err != nil {
			t.Fatalf("detectPint: %v", err)
		}

		cfg := config.Default()
		applyPint(cfg, &pint)

		return cfg.PHP
	}

	cases := []struct {
		name     string
		bare     string
		explicit string
	}{
		{"empty object", `{}`, `{"preset":"laravel"}`},
		{
			"rules only",
			`{"rules":{"concat_space":{"spacing":"one"}}}`,
			`{"preset":"laravel","rules":{"concat_space":{"spacing":"one"}}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := configFor(t, tc.bare)
			want := configFor(t, tc.explicit)

			if !reflect.DeepEqual(got, want) {
				t.Errorf("without a preset:\ngot  %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestApplyPintSplitsTraitUsesOnlyWhereThePresetOrRuleDoes(t *testing.T) {
	if config.Default().PHP.SplitTraitUses {
		t.Fatal("without Pint, trait lists must stay as written")
	}

	cases := []struct {
		name     string
		pintJSON string
		split    bool
	}{
		{"laravel preset", `{"preset":"laravel"}`, false},
		{"per preset", `{"preset":"per"}`, true},
		{"psr12 preset", `{"preset":"psr12"}`, true},
		{"symfony preset", `{"preset":"symfony"}`, true},
		{"rule switched on", `{"preset":"laravel","rules":{"single_trait_insert_per_statement":true}}`, true},
		{"rule switched off", `{"preset":"per","rules":{"single_trait_insert_per_statement":false}}`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seedFileAt(t, dir, "pint.json", tc.pintJSON)

			pint, _, err := detectPint(dir)
			if err != nil {
				t.Fatalf("detectPint: %v", err)
			}

			cfg := config.Default()
			applyPint(cfg, &pint)

			if cfg.PHP.SplitTraitUses != tc.split {
				t.Errorf("SplitTraitUses = %v, want %v", cfg.PHP.SplitTraitUses, tc.split)
			}
		})
	}
}

func TestDetectPintFromAComposerRequirementWithoutPintJSON(t *testing.T) {
	cases := []struct {
		name       string
		composer   string
		pintJSON   string
		found      bool
		wantPreset string
	}{
		{"required for development", `{"require-dev":{"laravel/pint":"^1.24"}}`, "", true, "laravel"},
		{"required", `{"require":{"laravel/pint":"^1.24"}}`, "", true, "laravel"},
		{"pint.json wins", `{"require-dev":{"laravel/pint":"^1.24"}}`, `{"preset":"psr12"}`, true, "psr12"},
		{"not required", `{"require":{"laravel/framework":"^13.0"}}`, "", false, ""},
		{"unparseable composer.json", `{"require-dev":`, "", false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seedFileAt(t, dir, "composer.json", tc.composer)

			if tc.pintJSON != "" {
				seedFileAt(t, dir, "pint.json", tc.pintJSON)
			}

			pint, found, err := detectPint(dir)
			if err != nil {
				t.Fatalf("detectPint: %v", err)
			}

			if found != tc.found || pint.Preset != tc.wantPreset {
				t.Errorf("found = %v, preset = %q; want %v, %q", found, pint.Preset, tc.found, tc.wantPreset)
			}
		})
	}
}

func TestApplyPintKeepsBracedUseGroupsWhenTheOptionIsOff(t *testing.T) {
	cases := []struct {
		name       string
		rule       string
		splitLists bool
		keepGroups bool
	}{
		{"rule on", `true`, true, false},
		{"option off", `{"group_to_single_imports": false}`, true, true},
		{"option on", `{"group_to_single_imports": true}`, true, false},
		{"rule off", `false`, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seedFileAt(t, dir, "pint.json", `{"preset":"laravel","rules":{"single_import_per_statement":`+tc.rule+`}}`)

			pint, _, err := detectPint(dir)
			if err != nil {
				t.Fatalf("detectPint: %v", err)
			}

			cfg := config.Default()
			applyPint(cfg, &pint)

			if cfg.PHP.ExpandUseGroups != tc.splitLists || cfg.PHP.KeepBracedUseGroups != tc.keepGroups {
				t.Errorf("ExpandUseGroups = %v, KeepBracedUseGroups = %v; want %v, %v",
					cfg.PHP.ExpandUseGroups, cfg.PHP.KeepBracedUseGroups, tc.splitLists, tc.keepGroups)
			}
		})
	}
}
