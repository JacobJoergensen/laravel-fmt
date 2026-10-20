package detector

import (
	"reflect"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

// setFinder wraps setRules here, so the fixture also covers a setRules that
// is not the outermost call.
func TestDetectPhpCsFixerParsesExplicitRules(t *testing.T) {
	dir := t.TempDir()
	src := `<?php

$finder = (new PhpCsFixer\Finder())->in(__DIR__);

return (new PhpCsFixer\Config())
    ->setRules([
        '@PSR12' => true,
        'concat_space' => ['spacing' => 'one'],
        'binary_operator_spaces' => ['operators' => ['=>' => 'align']],
        'function_declaration' => ['closure_fn_spacing' => 'none'],
        'not_operator_with_successor_space' => true,
        'blank_line_before_statement' => ['statements' => ['return']],
        'heredoc_indentation' => false,
    ])
    ->setFinder($finder);
`
	seedFileAt(t, dir, ".php-cs-fixer.dist.php", src)

	pint, found, err := detectPhpCsFixer(dir)
	if err != nil {
		t.Fatalf("detectPhpCsFixer: %v", err)
	}

	if !found {
		t.Fatal("detectPhpCsFixer reported not found, want found")
	}

	if pint.ConcatSpace == nil || pint.ConcatSpace.Spacing != pintSpacingOne {
		t.Errorf("ConcatSpace = %+v, want spacing=one", pint.ConcatSpace)
	}

	if pint.BinaryOperatorSpaces == nil || pint.BinaryOperatorSpaces.Operators["=>"] != pintAlign {
		t.Errorf("BinaryOperatorSpaces = %+v, want operators[=>]=align", pint.BinaryOperatorSpaces)
	}

	if pint.FunctionDeclaration == nil || pint.FunctionDeclaration.ClosureFnSpacing != pintSpacingNone {
		t.Errorf("FunctionDeclaration = %+v, want closure_fn_spacing=none", pint.FunctionDeclaration)
	}

	if pint.NotOperatorWithSuccessorSpace == nil || !*pint.NotOperatorWithSuccessorSpace {
		t.Errorf("NotOperatorWithSuccessorSpace = %+v, want true", pint.NotOperatorWithSuccessorSpace)
	}

	if pint.BlankLineBeforeStatement == nil || len(pint.BlankLineBeforeStatement.Statements) != 1 ||
		pint.BlankLineBeforeStatement.Statements[0] != "return" {
		t.Errorf("BlankLineBeforeStatement = %+v, want statements=[return]", pint.BlankLineBeforeStatement)
	}

	if pint.HeredocIndentation == nil || *pint.HeredocIndentation != config.HeredocIndentNone {
		t.Errorf("HeredocIndentation = %v, want none", pint.HeredocIndentation)
	}
}

func TestDetectPhpCsFixerRuleSetsOnlyNotDetected(t *testing.T) {
	dir := t.TempDir()
	src := `<?php

return (new PhpCsFixer\Config())->setRules(['@PSR12' => true, '@Symfony' => true]);
`
	seedFileAt(t, dir, ".php-cs-fixer.dist.php", src)

	_, found, err := detectPhpCsFixer(dir)
	if err != nil {
		t.Fatalf("detectPhpCsFixer: %v", err)
	}

	if found {
		t.Fatal("detectPhpCsFixer reported found for an @-rule-set-only config, want not found")
	}
}

func TestDetectPhpCsFixerKeepsAConfigWithANumericOption(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".php-cs-fixer.dist.php", `<?php
return (new PhpCsFixer\Config())->setRules([
    'concat_space' => ['spacing' => 'one'],
    'blank_lines_before_namespace' => ['min_line_breaks' => 2, 'max_line_breaks' => 2.0],
]);`)

	pint, found, err := detectPhpCsFixer(dir)
	if err != nil {
		t.Fatalf("detectPhpCsFixer: %v", err)
	}

	if !found || pint.ConcatSpace == nil || pint.ConcatSpace.Spacing != pintSpacingOne {
		t.Fatalf("found = %v, ConcatSpace = %+v, want the config adopted with spacing=one", found, pint.ConcatSpace)
	}
}

func TestDetectPhpCsFixerBailsToDefaults(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			"rules from a variable",
			`<?php
$rules = ['concat_space' => ['spacing' => 'one']];
return (new PhpCsFixer\Config())->setRules($rules);`,
		},
		{
			"array_merge",
			`<?php
return (new PhpCsFixer\Config())->setRules(array_merge(['concat_space' => true], []));`,
		},
		{
			"spread element",
			`<?php
return (new PhpCsFixer\Config())->setRules([...$base, 'concat_space' => ['spacing' => 'one']]);`,
		},
		{
			"non-literal value",
			`<?php
return (new PhpCsFixer\Config())->setRules(['concat_space' => Options::SPACING]);`,
		},
		{
			"shared-config class",
			`<?php
return (new Acme\PhpCsFixerConfig())->setRules(['concat_space' => true]);`,
		},
		{
			"returns a variable",
			`<?php
$config = (new PhpCsFixer\Config())->setRules(['concat_space' => true]);
return $config;`,
		},
		{
			"no setRules call",
			`<?php
return (new PhpCsFixer\Config())->setFinder($finder);`,
		},
		{
			"setRules called twice",
			`<?php
return (new PhpCsFixer\Config())->setRules(['concat_space' => true])->setRules(['strict_param' => true]);`,
		},
		{
			"value string with backslash",
			`<?php
return (new PhpCsFixer\Config())->setRules(['php_unit_namespace' => ['target' => 'PHPUnit\\Framework']]);`,
		},
		{
			// 010 is 8 in PHP and invalid JSON.
			"octal number",
			`<?php
return (new PhpCsFixer\Config())->setRules(['blank_lines_before_namespace' => ['min_line_breaks' => 010]]);`,
		},
		{
			"parse error",
			`<?php
return (new PhpCsFixer\Config())->setRules([`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seedFileAt(t, dir, ".php-cs-fixer.dist.php", tc.src)

			pint, found, err := detectPhpCsFixer(dir)
			if err != nil {
				t.Fatalf("detectPhpCsFixer: %v", err)
			}

			if found {
				t.Fatalf("detectPhpCsFixer reported found for %q, want not found (bail to defaults)", tc.name)
			}

			if !reflect.DeepEqual(pint, Pint{}) {
				t.Fatalf("Pint = %+v, want zero value on bail", pint)
			}
		})
	}
}

func TestDetectPhpCsFixerOverrideBeatsDist(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".php-cs-fixer.dist.php",
		`<?php
return (new PhpCsFixer\Config())->setRules(['concat_space' => ['spacing' => 'none']]);`)
	seedFileAt(t, dir, ".php-cs-fixer.php",
		`<?php
return (new PhpCsFixer\Config())->setRules(['concat_space' => ['spacing' => 'one']]);`)

	pint, found, err := detectPhpCsFixer(dir)
	if err != nil {
		t.Fatalf("detectPhpCsFixer: %v", err)
	}

	if !found {
		t.Fatal("detectPhpCsFixer reported not found, want found")
	}

	if pint.ConcatSpace == nil || pint.ConcatSpace.Spacing != pintSpacingOne {
		t.Errorf("ConcatSpace = %+v, want the override's spacing=one", pint.ConcatSpace)
	}
}

func TestDetectPhpCsFixerMissing(t *testing.T) {
	dir := t.TempDir()

	pint, found, err := detectPhpCsFixer(dir)
	if err != nil {
		t.Fatalf("detectPhpCsFixer: %v", err)
	}

	if found {
		t.Fatal("detectPhpCsFixer reported found with no config present, want not found")
	}

	if !reflect.DeepEqual(pint, Pint{}) {
		t.Fatalf("Pint = %+v, want zero value", pint)
	}
}

func TestDetectPhpCsFixerDisabledRuleIsAdopted(t *testing.T) {
	dir := t.TempDir()
	seedFileAt(t, dir, ".php-cs-fixer.php", `<?php

return (new PhpCsFixer\Config())->setRules([
    '@PSR12' => true,
    'ordered_imports' => false,
]);
`)

	pint, found, err := detectPhpCsFixer(dir)
	if err != nil || !found {
		t.Fatalf("detectPhpCsFixer: found=%v err=%v", found, err)
	}

	if pint.SortImports == nil || *pint.SortImports {
		t.Fatalf("SortImports = %v, want false", pint.SortImports)
	}
}

func TestApplyPhpCsFixerArraySyntax(t *testing.T) {
	dir := t.TempDir()
	src := `<?php

return (new PhpCsFixer\Config())->setRules(['array_syntax' => ['syntax' => 'long']]);
`

	seedFileAt(t, dir, ".php-cs-fixer.dist.php", src)

	pint, found, err := detectPhpCsFixer(dir)
	if err != nil {
		t.Fatalf("detectPhpCsFixer: %v", err)
	}

	if !found {
		t.Fatal("detectPhpCsFixer reported not found, want found")
	}

	cfg := config.Default()
	applyPint(cfg, &pint)

	if cfg.PHP.ShortArray {
		t.Error("ShortArray = true, want false from php-cs-fixer array_syntax: long")
	}
}
