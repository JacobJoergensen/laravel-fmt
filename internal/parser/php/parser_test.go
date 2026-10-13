package php

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestExpressionPrecedence(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "concat binds looser than shift",
			src:  "1 << 2 . 3",
			want: "(. (<< 1 2) 3)",
		},
		{
			name: "null coalesce binds looser than logical or",
			src:  "$a || $b ?? $c",
			want: "(?? (|| $a $b) $c)",
		},
		{
			name: "null coalesce is right associative",
			src:  "$a ?? $b ?? $c",
			want: "(?? $a (?? $b $c))",
		},
		{
			name: "pow is right associative",
			src:  "2 ** 3 ** 2",
			want: "(** 2 (** 3 2))",
		},
		{
			name: "pipeline is left associative",
			src:  "$a |> $b |> $c",
			want: "(|> (|> $a $b) $c)",
		},
		{
			name: "pipeline sits between coalesce and ternary",
			src:  "$a ?? $b |> $c ? $d : $e",
			want: "(? (|> (?? $a $b) $c) $d $e)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expr, _, errs := ParseFullExpression([]byte(tc.src))

			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			got := sexpr(expr)

			if got != tc.want {
				t.Fatalf("\nsrc:  %s\ngot:  %s\nwant: %s", tc.src, got, tc.want)
			}
		})
	}
}

func TestHeredocParsesAsSingleStringExpr(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"heredoc unquoted label", "<?php echo <<<EOT\nhello\nEOT;\n"},
		{"heredoc double-quoted label", "<?php echo <<<\"EOT\"\nhello\nEOT;\n"},
		{"nowdoc single-quoted label", "<?php echo <<<'EOT'\nhello\nEOT;\n"},
		// PHP's heredoc newline includes a lone CR; php -l accepts both.
		{"lone CR line breaks", "<?php echo <<<EOT\rhello\rEOT;\n"},
		{"lone CR before a CRLF", "<?php echo <<<EOT\r\r\nEOT;\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src))

			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			echo := findFirst[*EchoStmt](file.Stmts)

			if echo == nil {
				t.Fatalf("no EchoStmt produced")
				return
			}

			if len(echo.Exprs) != 1 {
				t.Fatalf("EchoStmt has %d exprs, want 1", len(echo.Exprs))
			}

			if _, ok := echo.Exprs[0].(*StringExpr); !ok {
				t.Fatalf("expression is %T, want *StringExpr", echo.Exprs[0])
			}
		})
	}
}

func TestBacktickParsesAsShellExecExpr(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"bare command", "<?php echo `ls`;"},
		{"command with whitespace", "<?php echo `ls -la /tmp`;"},
		{"interpolation marker", "<?php echo `echo $foo`;"},
		{"escaped backtick in body", "<?php echo `echo \\`hi\\``;"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src))

			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			echo := findFirst[*EchoStmt](file.Stmts)

			if echo == nil {
				t.Fatal("no EchoStmt produced")
				return
			}

			if len(echo.Exprs) != 1 {
				t.Fatalf("EchoStmt has %d exprs, want 1", len(echo.Exprs))
			}

			if _, ok := echo.Exprs[0].(*ShellExecExpr); !ok {
				t.Fatalf("expression is %T, want *ShellExecExpr", echo.Exprs[0])
			}
		})
	}
}

func TestUnterminatedBacktickIsRejected(t *testing.T) {
	_, errs := Parse([]byte("<?php echo `unclosed"))

	if len(errs) == 0 {
		t.Fatal("unterminated backtick should produce a parse error")
	}
}

func TestAssignToNonLvalueIsRejected(t *testing.T) {
	// PHP rejects each of these ("Can't use function return value in write
	// context" and kin).
	rejected := []string{
		"<?php foo() = 1;",
		"<?php $o->m() = 1;",
		"<?php C::m() = 1;",
		"<?php 5 = 1;",
		"<?php 'x' = 1;",
		"<?php new X() = 1;",
		"<?php CONST_NAME = 1;",
		"<?php Foo\\Bar = 1;",
		"<?php ! CONST_NAME = 1;",
		"<?php ++$a = 1;",
		"<?php $a++ = 1;",
	}

	for _, src := range rejected {
		t.Run("reject "+src, func(t *testing.T) {
			if _, errs := Parse([]byte(src)); len(errs) == 0 {
				t.Fatalf("expected a parse error for %q", src)
			}
		})
	}

	accepted := []string{
		"<?php $x = 1;",
		"<?php $o->p = 1;",
		"<?php C::$p = 1;",
		"<?php $a[0] = 1;",
		"<?php $a[] = 1;",
		"<?php [$a, $b] = $x;",
		"<?php list($a, $b) = $x;",
		"<?php $obj->make()->prop = 1;",
		"<?php $x += 1;",
		"<?php $c ? $a[] = 1 : $b[] = 2;",
		"<?php !$a = 1;",
		"<?php @$a = 1;",
		"<?php -$a = 1;",
	}

	for _, src := range accepted {
		t.Run("accept "+src, func(t *testing.T) {
			if _, errs := Parse([]byte(src)); len(errs) != 0 {
				t.Fatalf("unexpected errors for %q: %v", src, errs)
			}
		})
	}
}

// PHP rejects a property with no modifier and an empty hook list.
func TestPropertyDeclarationErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"untyped property with no modifier", "<?php class A{$a;}", "1:15: expected a modifier or function, got Variable"},
		{"typed property with no modifier", "<?php class A{int $a;}", "1:15: expected a modifier or function, got Ident"},
		{"empty hook list", "<?php class A{public $a {}}", "1:25: property hook list must not be empty"},
		{"empty hook list on a promoted property", "<?php class A{function __construct(public $a {}){}}", "1:46: property hook list must not be empty"},
		{"repeated final on a hook", "<?php class A{public $a {final final get => 1;}}", "1:32: expected Ident, got final"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errs := Parse([]byte(tt.src))
			if len(errs) == 0 {
				t.Fatalf("expected a parse error for %q", tt.src)
			}

			if got := errs[0].Error(); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}

	if _, errs := Parse([]byte("<?php class A{var $a; public $b { get => 1; }}")); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

// PHP rejects `0->p`, `0::C` and `0[1]` in any context. Parenthesised they
// parse and fail only at runtime, so `(0)->p` is accepted.
func TestPostfixOnNumericLiteralIsRejected(t *testing.T) {
	rejected := []string{
		"<?php $x = 0->p;",
		"<?php $x = 0?->p;",
		"<?php $x = 0.5->p;",
		"<?php $x = 0::C;",
		"<?php $x = 0[1];",
		"<?php 0->p = 1;",
	}

	for _, src := range rejected {
		t.Run("reject "+src, func(t *testing.T) {
			if _, errs := Parse([]byte(src)); len(errs) == 0 {
				t.Fatalf("expected a parse error for %q", src)
			}
		})
	}

	accepted := []string{
		"<?php $x = (0)->p;",
		"<?php $x = 's'[1];",
		"<?php $x = $a[0]->p;",
		"<?php $x = [1, 2][0];",
	}

	for _, src := range accepted {
		t.Run("accept "+src, func(t *testing.T) {
			if _, errs := Parse([]byte(src)); len(errs) != 0 {
				t.Fatalf("unexpected errors for %q: %v", src, errs)
			}
		})
	}
}

func TestReservedWordClassConstName(t *testing.T) {
	valid := []string{
		"<?php class C { const DEFAULT = 1; }",
		"<?php class C { const LIST = 2; }",
		"<?php class C { const ARRAY = []; }",
		"<?php enum E: int { case A = 1; const DEFAULT = self::A; }",
	}

	for _, src := range valid {
		t.Run("class const: "+src, func(t *testing.T) {
			if _, errs := Parse([]byte(src)); len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
		})
	}

	if _, errs := Parse([]byte("<?php const DEFAULT = 1;")); len(errs) == 0 {
		t.Fatal("top-level const with a reserved-word name should be a parse error")
	}
}

// A `?>` close tag ends a statement wherever a `;` would. Expected values are
// php -l's verdicts.
func TestStatementTerminatedByCloseTagOrEof(t *testing.T) {
	valid := []string{
		"<?php echo $x ?>",
		"<?php echo $a, $b ?>",
		// Alternative-syntax closers, the shape legacy templates are full of.
		"<?php if ($a): ?>x<?php endif ?>",
		"<?php foreach ($a as $b): ?>x<?php endforeach ?>",
		"<?php while ($a): ?>x<?php endwhile ?>",
		"<?php for (;;): ?>x<?php endfor ?>",
		"<?php switch ($a): case 1: ?>x<?php endswitch ?>",
		"<?php return ?>",
		"<?php return 1 ?>",
		"<?php while (1): break ?><?php endwhile; ?>",
		"<?php while (1): continue ?><?php endwhile; ?>",
		"<?php unset($a) ?>",
		"<?php do { } while (1) ?>",
		"<?php goto a ?><?php a: ?>",
		"<?php namespace A ?>",
		"<?php use A\\B ?>",
		"<?php const A = 1 ?>",
		"<?php declare(strict_types=1) ?>",
		"<?php function f() { global $a ?><?php }",
		"<?php function f() { static $a = 1 ?><?php }",
	}

	for _, src := range valid {
		t.Run("valid: "+src, func(t *testing.T) {
			if _, errs := Parse([]byte(src)); len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
		})
	}

	invalid := []string{
		"<?php echo $x $y;",
		// EOF ends a file, not a statement: php -l rejects a last statement
		// without its `;`.
		"<?php echo $x",
		// The semicolons in a `for` header separate rather than terminate, so a
		// close tag cannot stand in for one. php -l rejects this too.
		"<?php for ($i = 0 ?>",
		// An alternative-syntax block is copied byte for byte, so a synthesized
		// `;` would never reach the output; php -l agrees EOF does not end one
		// there.
		"<?php if ($x): foo(); endif",
		"<?php while (1): endwhile",
	}

	for _, src := range invalid {
		t.Run("invalid: "+src, func(t *testing.T) {
			if _, errs := Parse([]byte(src)); len(errs) == 0 {
				t.Fatal("expected a parse error")
			}
		})
	}
}

func TestLeadingDotFloat(t *testing.T) {
	valid := []string{
		"<?php $x = .5;",
		"<?php $y = .5e3;",
		"<?php $z = [.5, .25, 1.5];",
	}

	for _, src := range valid {
		t.Run("valid: "+src, func(t *testing.T) {
			if _, errs := Parse([]byte(src)); len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
		})
	}

	if _, errs := Parse([]byte("<?php $a.5;")); len(errs) == 0 {
		t.Fatal("`$a.5` (float after operand, no operator) should be a parse error")
	}
}

func TestArrayEmptyDestructureSlots(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"middle slot", "<?php [$a, , $c] = $arr;"},
		{"leading slot", "<?php [, $b] = $arr;"},
		{"foreach pattern", "<?php foreach ($rows as [$id, , $name]) {}"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, errs := Parse([]byte(tc.src)); len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
		})
	}

	file, _ := Parse([]byte("<?php [$a, , $c] = $arr;"))
	stmt := findFirst[*ExpressionStmt](file.Stmts)
	if stmt == nil {
		t.Fatal("no ExpressionStmt produced")
		return
	}

	assign, ok := stmt.Expr.(*AssignExpr)
	if !ok {
		t.Fatalf("expression is %T, want *AssignExpr", stmt.Expr)
	}

	arr, ok := assign.Left.(*ArrayExpr)
	if !ok {
		t.Fatalf("assignment LHS is %T, want *ArrayExpr", assign.Left)
	}

	if len(arr.Elements) != 3 {
		t.Fatalf("array has %d elements, want 3", len(arr.Elements))
	}

	if arr.Elements[1].Value != nil {
		t.Fatalf("middle element value is %v, want nil (empty slot)", arr.Elements[1].Value)
	}
}

func TestRelativeNamespaceName(t *testing.T) {
	valid := []string{
		"<?php $x = namespace\\Foo::BAR;",
		"<?php namespace\\Foo::bar();",
		"<?php $x = new namespace\\Foo();",
		"<?php function f(namespace\\Foo $x): namespace\\Bar {}",
		"<?php class C extends namespace\\Base {}",
		"<?php namespace App\\Services;",
	}

	for _, src := range valid {
		t.Run(src, func(t *testing.T) {
			if _, errs := Parse([]byte(src)); len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
		})
	}

	file, _ := Parse([]byte("<?php namespace App;"))
	if findFirst[*NamespaceStmt](file.Stmts) == nil {
		t.Fatal("`namespace App;` should parse as a NamespaceStmt")
	}
}

func TestForeachKeyByRefRejected(t *testing.T) {
	if _, errs := Parse([]byte("<?php foreach ($a as &$k => $v) {}")); len(errs) == 0 {
		t.Fatal("`foreach ($a as &$k => $v)` should be a parse error")
	}

	valid := []string{
		"<?php foreach ($a as $v) {}",
		"<?php foreach ($a as &$v) {}",
		"<?php foreach ($a as $k => $v) {}",
		"<?php foreach ($a as $k => &$v) {}",
	}

	for _, src := range valid {
		t.Run(src, func(t *testing.T) {
			if _, errs := Parse([]byte(src)); len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
		})
	}
}

func TestReferenceAssignmentMarksByRef(t *testing.T) {
	valid := []string{
		"<?php $a = &$b;",
		"<?php $a =& $b;",
		"<?php $a = &$b[$k];",
		"<?php $this->a = &$b->c;",
	}

	for _, src := range valid {
		t.Run(src, func(t *testing.T) {
			file, errs := Parse([]byte(src))
			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			stmt := findFirst[*ExpressionStmt](file.Stmts)
			if stmt == nil {
				t.Fatal("no ExpressionStmt produced")
				return
			}

			assign, ok := stmt.Expr.(*AssignExpr)
			if !ok {
				t.Fatalf("expression is %T, want *AssignExpr", stmt.Expr)
			}

			if !assign.ByRef {
				t.Fatal("assignment is not marked by-reference")
			}
		})
	}

	if _, errs := Parse([]byte("<?php $a += &$b;")); len(errs) == 0 {
		t.Fatal("`$a += &$b` should be a parse error")
	}
}

func TestParsesDynamicMemberName(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"instance member access", "$obj->{$prop}"},
		{"nullsafe member access", "$obj?->{$prop}"},
		{"static member access", "SomeClass::{$const}"},
		{"complex inner expression", "$obj->{$prefix . 'Name'}"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expr, _, errs := ParseFullExpression([]byte(tc.src))

			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			var prop Expr

			switch e := expr.(type) {
			case *MemberExpr:
				prop = e.Property
			case *StaticMemberExpr:
				prop = e.Name
			default:
				t.Fatalf("expression is %T, want *MemberExpr or *StaticMemberExpr", expr)
			}

			if _, ok := prop.(*DynamicName); !ok {
				t.Fatalf("member name is %T, want *DynamicName", prop)
			}
		})
	}
}

func TestFirstClassCallableSyntax(t *testing.T) {
	expr, _, errs := ParseFullExpression([]byte("strlen(...)"))

	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	if _, ok := expr.(*FirstClassCallableExpr); !ok {
		t.Fatalf("expression is %T, want *FirstClassCallableExpr", expr)
	}
}

func TestParseRejectsFirstClassNewSyntax(t *testing.T) {
	expr, _, errs := ParseFullExpression([]byte("new Foo(...)"))

	if len(errs) != 1 {
		t.Fatalf("got %d errors, want exactly 1: %v", len(errs), errs)
	}

	if !strings.Contains(errs[0].Msg, "new Class(...)") {
		t.Fatalf("error message = %q, want mention of new Class(...)", errs[0].Msg)
	}

	newExpr, ok := expr.(*NewExpr)

	if !ok {
		t.Fatalf("expression is %T, want *NewExpr", expr)
	}

	if len(newExpr.Args) != 0 {
		t.Fatalf("got %d args, want 0 (rejected form reduces to empty args)", len(newExpr.Args))
	}

	if newExpr.LParen.Kind == KindInvalid || newExpr.RParen.Kind == KindInvalid {
		t.Fatalf("LParen=%v RParen=%v, want both non-invalid (tokens consumed cleanly)", newExpr.LParen.Kind, newExpr.RParen.Kind)
	}
}

func TestParseCloneShapes(t *testing.T) {
	cases := []struct {
		name        string
		src         string
		wantCall    bool
		wantArgs    int
		wantHasExpr bool
	}{
		{"unary variable", "clone $a", false, 0, true},
		{"unary member access", "clone $obj->inner", false, 0, true},
		{"call single arg", "clone($a)", true, 1, false},
		{"call with modifications", "clone($a, ['x' => 1])", true, 2, false},
		{"call paren-wrapped operand normalises", "clone ($a)", true, 1, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expr, _, errs := ParseFullExpression([]byte(tc.src))

			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			cloneExpr, ok := expr.(*CloneExpr)

			if !ok {
				t.Fatalf("expression is %T, want *CloneExpr", expr)
			}

			gotCall := cloneExpr.LParen.Kind != KindInvalid

			if gotCall != tc.wantCall {
				t.Fatalf("call-form=%v, want %v", gotCall, tc.wantCall)
			}

			if len(cloneExpr.Args) != tc.wantArgs {
				t.Fatalf("got %d args, want %d", len(cloneExpr.Args), tc.wantArgs)
			}

			if (cloneExpr.Expr != nil) != tc.wantHasExpr {
				t.Fatalf("Expr set=%v, want %v", cloneExpr.Expr != nil, tc.wantHasExpr)
			}
		})
	}
}

func TestParseRejectsFirstClassCloneSyntax(t *testing.T) {
	expr, _, errs := ParseFullExpression([]byte("clone(...)"))

	if len(errs) != 1 {
		t.Fatalf("got %d errors, want exactly 1: %v", len(errs), errs)
	}

	if !strings.Contains(errs[0].Msg, "clone(...)") {
		t.Fatalf("error message = %q, want mention of clone(...)", errs[0].Msg)
	}

	cloneExpr, ok := expr.(*CloneExpr)

	if !ok {
		t.Fatalf("expression is %T, want *CloneExpr", expr)
	}

	if len(cloneExpr.Args) != 0 {
		t.Fatalf("got %d args, want 0 (rejected form reduces to empty args)", len(cloneExpr.Args))
	}

	if cloneExpr.LParen.Kind == KindInvalid || cloneExpr.RParen.Kind == KindInvalid {
		t.Fatalf("LParen=%v RParen=%v, want both non-invalid (tokens consumed cleanly)", cloneExpr.LParen.Kind, cloneExpr.RParen.Kind)
	}
}

func TestAsymmetricVisibilitySuffixIsNotADNFType(t *testing.T) {
	src := "<?php class C { public private(set) string $x; }"

	file, errs := Parse([]byte(src))

	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	class := findFirst[*ClassStmt](file.Stmts)

	if class == nil {
		t.Fatalf("no ClassStmt produced")
		return
	}

	var prop *PropertyDecl

	for _, m := range class.Members {
		if p, ok := m.(*PropertyDecl); ok {
			prop = p

			break
		}
	}

	if prop == nil {
		t.Fatalf("no PropertyDecl produced")
		return
	}

	if len(prop.Modifiers) != 2 {
		t.Fatalf("got %d modifiers, want 2 (public + private(set))", len(prop.Modifiers))
	}

	second := prop.Modifiers[1]

	if second.Keyword.Kind != KindPrivate {
		t.Fatalf("second modifier keyword kind=%v, want KindPrivate", second.Keyword.Kind)
	}

	if string(second.Set.Text) != "set" {
		t.Fatalf("asymmetric suffix Set.Text=%q, want %q", second.Set.Text, "set")
	}
}

func TestParsesAltControlSyntax(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"if endif", "<?php if ($x): foo(); endif;"},
		{"if elseif else endif", "<?php if ($x): foo(); elseif ($y): bar(); else: baz(); endif;"},
		{"empty if body", "<?php if ($x): endif;"},
		{"while endwhile", "<?php while ($x): foo(); endwhile;"},
		{"for endfor", "<?php for ($i = 0; $i < 10; $i = $i + 1): work(); endfor;"},
		{"foreach endforeach", "<?php foreach ($a as $b): foo(); endforeach;"},
		{"foreach key endforeach", "<?php foreach ($a as $k => $v): foo(); endforeach;"},
		{"switch endswitch", "<?php switch ($x): case 1: foo(); break; default: baz(); endswitch;"},
		{"declare enddeclare", "<?php declare(ticks=1): work(); enddeclare;"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src))

			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			if len(file.Stmts) == 0 {
				t.Fatal("no statements parsed")
			}

			var colon, endSemi Token

			switch s := file.Stmts[1].(type) {
			case *IfStmt:
				colon, endSemi = s.Colon, s.EndSemi
			case *WhileStmt:
				colon, endSemi = s.Colon, s.EndSemi
			case *ForStmt:
				colon, endSemi = s.Colon, s.EndSemi
			case *ForEachStmt:
				colon, endSemi = s.Colon, s.EndSemi
			case *SwitchStmt:
				colon, endSemi = s.Colon, s.EndSemi
			case *DeclareStmt:
				colon, endSemi = s.Colon, s.EndSemi
			default:
				t.Fatalf("statement is %T, want one of the alt-aware control nodes", s)
			}

			if colon.Kind != KindColon {
				t.Fatalf("Colon.Kind=%v, want KindColon", colon.Kind)
			}

			if endSemi.Kind != KindSemicolon {
				t.Fatalf("EndSemi.Kind=%v, want KindSemicolon", endSemi.Kind)
			}
		})
	}
}

func TestAltControlSyntaxRequiresTrailingSemicolon(t *testing.T) {
	_, errs := Parse([]byte("<?php if ($x): foo(); endif"))

	if len(errs) == 0 {
		t.Fatal("missing semicolon should produce a parse error")
	}
}

func TestParseFullExpressionAcceptsBareSource(t *testing.T) {
	expr, _, errs := ParseFullExpression([]byte("$x + 1"))

	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	bin, ok := expr.(*BinaryExpr)

	if !ok {
		t.Fatalf("expression is %T, want *BinaryExpr", expr)
	}

	if string(bin.Op.Text) != "+" {
		t.Fatalf("Op=%q, want %q", bin.Op.Text, "+")
	}
}

func sexpr(e Expr) string {
	switch v := e.(type) {
	case *IntExpr:
		return string(v.Token.Text)
	case *FloatExpr:
		return string(v.Token.Text)
	case *VariableExpr:
		return string(v.Token.Text)
	case *IdentExpr:
		return string(v.Token.Text)
	case *BinaryExpr:
		return "(" + string(v.Op.Text) + " " + sexpr(v.Left) + " " + sexpr(v.Right) + ")"
	case *UnaryExpr:
		return "(" + string(v.Op.Text) + " " + sexpr(v.Expr) + ")"
	case *TernaryExpr:
		then := "_"
		if v.Then != nil {
			then = sexpr(v.Then)
		}

		return "(? " + sexpr(v.Cond) + " " + then + " " + sexpr(v.Else) + ")"
	case *ParenExpr:
		return sexpr(v.Expr)
	}

	return fmt.Sprintf("<%T>", e)
}

func findFirst[T Stmt](stmts []Stmt) T {
	var zero T

	for _, s := range stmts {
		if v, ok := s.(T); ok {
			return v
		}
	}

	return zero
}

func TestParseShortEcho(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		wantExprs    int
		wantCommas   int
		wantTrailing int
		wantClose    bool
	}{
		{
			name:         "single expression closed",
			src:          "<?= $name ?>",
			wantExprs:    1,
			wantCommas:   0,
			wantTrailing: 0,
			wantClose:    true,
		},
		{
			name:         "comma-separated list closed",
			src:          "<?= $a, $b, $c ?>",
			wantExprs:    3,
			wantCommas:   2,
			wantTrailing: 0,
			wantClose:    true,
		},
		{
			name:         "comma list with trailing semi statement",
			src:          "<?= $a, $b; $c ?>",
			wantExprs:    2,
			wantCommas:   1,
			wantTrailing: 1,
			wantClose:    true,
		},
		{
			name:         "unclosed single expression ended by a semicolon",
			src:          "<?= $name;",
			wantExprs:    1,
			wantCommas:   0,
			wantTrailing: 0,
			wantClose:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src))

			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			head := findFirst[*ShortEchoStmt](file.Stmts)

			if head == nil {
				t.Fatalf("no ShortEchoStmt produced")
				return
			}

			if got := len(head.Exprs); got != tc.wantExprs {
				t.Fatalf("Exprs len = %d, want %d", got, tc.wantExprs)
			}

			if got := len(head.Commas); got != tc.wantCommas {
				t.Fatalf("Commas len = %d, want %d", got, tc.wantCommas)
			}

			trailing := 0
			hasClose := false

			for _, s := range file.Stmts {
				switch v := s.(type) {
				case *ShortEchoStmt:
					continue
				case *ExpressionStmt:
					_ = v
					trailing++
				case *TagStmt:
					if v.Token.Kind == KindCloseTag {
						hasClose = true
					}
				}
			}

			if trailing != tc.wantTrailing {
				t.Fatalf("trailing ExpressionStmts = %d, want %d", trailing, tc.wantTrailing)
			}

			if hasClose != tc.wantClose {
				t.Fatalf("hasClose = %v, want %v", hasClose, tc.wantClose)
			}
		})
	}
}

func TestTrailingCommaLists(t *testing.T) {
	accepted := []struct {
		name string
		src  string
	}{
		{"parameters", "<?php function f($a, $b,) {}"},
		{"arguments", "<?php f($a, $b,);"},
		{"short array", "<?php $x = [1, 2,];"},
		{"long array", "<?php $x = array(1, 2,);"},
		{"match arms", "<?php $x = match ($v) { 1 => 'a', 2 => 'b', };"},
		{"closure use", "<?php $f = function () use ($a, $b,) {};"},
		{"attribute group", "<?php #[A, B,] class X {}"},
		{"grouped use", "<?php use A\\{B, C,};"},
	}

	for _, tc := range accepted {
		t.Run("accept "+tc.name, func(t *testing.T) {
			if _, errs := Parse([]byte(tc.src)); len(errs) != 0 {
				t.Fatalf("%q should parse, got %v", tc.src, errs)
			}
		})
	}

	rejected := []struct {
		name string
		src  string
	}{
		{"global", "<?php global $a, $b,;"},
		{"const", "<?php const A = 1, B = 2,;"},
		{"static", "<?php function f() { static $a = 1, $b = 2,; }"},
		{"echo", "<?php echo 1, 2,;"},
		{"declare", "<?php declare(strict_types=1,);"},
		{"implements list", "<?php class X implements A, B, {}"},
		{"empty unset", "<?php unset();"},
	}

	for _, tc := range rejected {
		t.Run("reject "+tc.name, func(t *testing.T) {
			if _, errs := Parse([]byte(tc.src)); len(errs) == 0 {
				t.Fatalf("%q should not parse", tc.src)
			}
		})
	}
}

// Expected values are php -l's verdicts.
func TestClassBodyTagPairTerminatesAMember(t *testing.T) {
	valid := []string{
		"<?php class A { public $x = 1 ?><?php }",
		// The lexer eats the one newline after `?>`, so this is the same pair.
		"<?php class A { public $x = 1 ?>\n<?php }",
		"<?php class A { public $x = 1 ?><?php public $y = 2; }",
		"<?php class A { const X = 1 ?><?php }",
		"<?php class A { use T ?><?php }",
		"<?php abstract class A { abstract function f() ?><?php }",
		"<?php interface I { function f() ?><?php }",
		"<?php enum E { case A ?><?php }",
	}

	for _, src := range valid {
		t.Run("valid: "+src, func(t *testing.T) {
			if _, errs := Parse([]byte(src)); len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
		})
	}

	invalid := []string{
		// Nothing may sit between the two tags, not even a space.
		"<?php class A { public $x = 1 ?>   <?php }",
		"<?php class A { public $x = 1 ?>TEXT<?php }",
		"<?php class A { public $x = 1 ?><p>x</p><?php }",
		// A member that needs no `;` leaves the tag standing for nothing.
		"<?php class A { function f() {} ?><?php function g() {} }",
		"<?php class A { ?><?php public $x = 1; }",
		"<?php class A { public $x = 1; ?><?php }",
	}

	for _, src := range invalid {
		t.Run("invalid: "+src, func(t *testing.T) {
			if _, errs := Parse([]byte(src)); len(errs) == 0 {
				t.Fatal("expected a parse error")
			}
		})
	}
}

func TestParseFrontmatterMatchesLexingTheBlockAlone(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"newline after the tag", "<?php\n$x = 1;\n?>\n<div>{{ $x }}</div>\n"},
		{"template right after the tag", "<?php $x = 1; ?><div></div>"},
		{"CRLF", "<?php\r\n$x = 1;\r\n?>\r\n<div></div>\r\n"},
		{"tag inside strings", "<?php $s = '?>'; $t = \"?>\"; ?>\n<p></p>"},
		{"tag inside a block comment", "<?php /* ?> */ $x = 1; ?>\n<p></p>"},
		{"tag ending a line comment", "<?php $x = 1; // note ?>\n<p></p>"},
		{"tag inside a heredoc", "<?php $h = <<<EOT\n?>\nEOT;\n?>\n<p></p>"},
		{"nullsafe operator", "<?php $a?->b(); ?>\n<p></p>"},
		{"PHP again in the template", "<?php $x = 1; ?>\n<p><?php echo 2; ?></p>"},
		{"no tag", "<?php\nclass A {}\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := []byte(tt.src)

			wantEnd, wantClosed := Position{}, false
			all, _ := Lex(src)

			for _, tok := range all {
				if tok.Kind == KindCloseTag {
					wantEnd, wantClosed = tok.Span.End, true

					break
				}
			}

			block := src
			if wantClosed {
				block = src[:wantEnd.Offset]
			}

			want, wantErrs := Parse(block)
			got, end, closed, errs := ParseFrontmatter(src)

			if closed != wantClosed || (closed && end != wantEnd) {
				t.Fatalf("ParseFrontmatter end = %+v closed=%v, want %+v closed=%v", end, closed, wantEnd, wantClosed)
			}

			if !reflect.DeepEqual(got.Tokens, want.Tokens) {
				t.Errorf("tokens differ from lexing the block alone:\n got %+v\nwant %+v", got.Tokens, want.Tokens)
			}

			if !bytes.Equal(got.Src, want.Src) || !reflect.DeepEqual(errs, wantErrs) {
				t.Errorf("Src/errors differ: got %q %v, want %q %v", got.Src, errs, want.Src, wantErrs)
			}
		})
	}
}

func TestParseAcceptsValidAndRejectsInvalidSyntax(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr bool
	}{
		{"by-reference array value", `<?php $x = [&$a, 'k' => &$v];`, false},
		{"by-reference array key", `<?php $x = [&$a => 1];`, true},
		{"void cast", `<?php (void) f();`, false},
		{"real cast removed in PHP 8", `<?php $x = (real) $y;`, true},
		{"DNF intersection in a union", `<?php function f((A&B)|C $x) {}`, false},
		{"DNF group second in a union", `<?php function f(A|(B&C) $x) {}`, false},
		{"parenthesized union", `<?php function f((A|B)|C $x) {}`, true},
		{"parenthesized type alone", `<?php function f((A&B) $x) {}`, true},
		{"nullable parenthesized type", `<?php function f(?(A&B) $x) {}`, true},
		{"parenthesized type in an intersection", `<?php function f((A&B)&C $x) {}`, true},
		{"unqualified trait method", `<?php class C { use A, B { A::foo insteadof B; } }`, false},
		{"qualified trait method", `<?php class C { use \App\A, B { \App\A::foo insteadof B; } }`, false},
		{"namespaced trait alias", `<?php class C { use App\A { App\A::foo as bar; } }`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := Parse([]byte(tc.src))

			if gotErr := len(errs) > 0; gotErr != tc.wantErr {
				t.Fatalf("errors = %v, want error %v", errs, tc.wantErr)
			}
		})
	}
}
