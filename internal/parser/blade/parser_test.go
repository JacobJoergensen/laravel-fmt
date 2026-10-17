package blade

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/parser/html"
)

func TestDirectiveArgWithParenInString(t *testing.T) {
	file, errs := Parse([]byte(`@if($x === ')')`), nil)

	if len(errs) > 0 {
		t.Fatalf("Parse returned errors: %v", errs)
	}

	if len(file.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1 (the directive)", len(file.Nodes))
	}

	d, ok := file.Nodes[0].(*DirectiveNode)
	if !ok {
		t.Fatalf("node is %T, want *DirectiveNode", file.Nodes[0])
	}

	if string(d.Name) != "if" {
		t.Fatalf("Name=%q, want %q", d.Name, "if")
	}

	if len(d.Args) != 1 {
		t.Fatalf("got %d args, want 1 (the full condition)", len(d.Args))
	}
}

func TestPhpDirectiveParsesSingleExpression(t *testing.T) {
	file, errs := Parse([]byte("@php($count = 1)"), nil)

	if len(errs) > 0 {
		t.Fatalf("Parse returned errors: %v", errs)
	}

	d, ok := file.Nodes[0].(*DirectiveNode)
	if !ok {
		t.Fatalf("node is %T, want *DirectiveNode", file.Nodes[0])
	}

	if string(d.Name) != phpDirective {
		t.Fatalf("Name=%q, want %q", d.Name, phpDirective)
	}

	if len(d.Args) != 1 {
		t.Fatalf("got %d args, want 1 (the single expression)", len(d.Args))
	}
}

func TestPhpDirectiveRejectsMultipleStatements(t *testing.T) {
	_, errs := Parse([]byte("@php($a = 1; $b = 2)"), nil)

	if len(errs) == 0 {
		t.Fatal("Parse accepted a multi-statement @php(...); want a diagnostic")
	}
}

func TestEchoRejectsContentBladeCannotCompile(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"statement terminator", "{{ $a; }}", "1:6: unexpected token after arguments"},
		{"two expressions", "{{ $a $b }}", "1:7: unexpected token after arguments"},
		{"raw echo trailing comma", "{!! $a, !!}", "1:7: a raw echo is an echo list; a trailing comma does not parse"},
		{"raw echo spread", "{!! ...$a !!}", "1:5: a raw echo is an echo list; named arguments and spread do not parse"},
		{"raw echo named argument", "{!! a: $b !!}", "1:5: a raw echo is an echo list; named arguments and spread do not parse"},
		{"line comment runs to the closer", "{{ $a // c }}", "1:7: a line comment runs to the echo's closer, which Blade compiles onto the same line"},
		{"hash comment runs to the raw closer", "{!! $a #c!!}", "1:8: a line comment runs to the echo's closer, which Blade compiles onto the same line"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := Parse([]byte(tc.src), nil)

			if len(errs) == 0 || errs[0].Error() != tc.want {
				t.Fatalf("Parse(%q) errors = %v, want first %q", tc.src, errs, tc.want)
			}
		})
	}
}

func TestDirectiveArgumentErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"line comment swallows the paren", "@if($a // c)\nx\n@endif", "1:8: a line comment swallows the directive's ), which Blade reads the same way"},
		{"line comment swallows the paren in an unquoted value", "<div class=a@x(0 # c\">0)</div>", "1:13: a line comment swallows the directive's ), which Blade reads the same way"},
		{"line comment swallows the paren in a quoted value", "<div class=\"a @x (0 # c\">0)</div>", "1:15: a line comment swallows the directive's ), which Blade reads the same way"},
		{"paren never closed in a quoted value", "<div class=\"a @x(\"b=c></div>", "1:15: the directive's ( is never closed"},
		{"paren never closed in an unquoted value", "<div class=@x( b=c></div>", "1:12: the directive's ( is never closed"},
		{"laravel directive with non-PHP argument", "@hasSection(x y)", "1:15: unexpected token after arguments"},
		{"js argument that is not PHP", "@js(x y)", "1:7: unexpected token after arguments"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := Parse([]byte(tc.src), nil)

			if len(errs) == 0 || errs[0].Error() != tc.want {
				t.Fatalf("Parse(%q) errors = %v, want first %q", tc.src, errs, tc.want)
			}
		})
	}
}

func TestLoopDirectiveArguments(t *testing.T) {
	for _, src := range []string{"@foreach($users as $user)", "@forelse($a as $k => $v)", "@for($i = 0; $i < 3; $i++)"} {
		file, errs := Parse([]byte(src), nil)
		if len(errs) > 0 {
			t.Fatalf("Parse(%q) errors = %v", src, errs)
		}

		if d, ok := file.Nodes[0].(*DirectiveNode); !ok || d.Loop == nil {
			t.Fatalf("Parse(%q) left the loop clause unparsed", src)
		}
	}

	tests := []struct {
		src  string
		want string
	}{
		{"@foreach($users as)", "1:19: unexpected )"},
		{"@for($i = 0; $i < ; $i++)", "1:19: unexpected ;"},
	}

	for _, tc := range tests {
		if _, errs := Parse([]byte(tc.src), nil); len(errs) == 0 || errs[0].Error() != tc.want {
			t.Errorf("Parse(%q) errors = %v, want first %q", tc.src, errs, tc.want)
		}
	}
}

func TestCustomDirectiveKeepsNonPHPArgumentRaw(t *testing.T) {
	file, errs := Parse([]byte("@markdown(Hello world)"), nil)
	if len(errs) > 0 {
		t.Fatalf("Parse errors = %v, want none", errs)
	}

	d, ok := file.Nodes[0].(*DirectiveNode)
	if !ok || string(d.Raw) != "Hello world" || d.Args != nil {
		t.Fatalf("node = %#v, want a directive with Raw %q and no Args", file.Nodes[0], "Hello world")
	}
}

func TestParseDispatchesEachBladeNode(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want any
	}{
		{"escaped echo {{ expr }}", "{{ $x }}", (*EchoNode)(nil)},
		{"raw echo {!! expr !!}", "{!! $x !!}", (*RawEchoNode)(nil)},
		{"blade comment {{-- text --}}", "{{-- hi --}}", (*CommentNode)(nil)},
		{"directive without args", "@csrf", (*DirectiveNode)(nil)},
		{"directive with args", "@if($x)", (*DirectiveNode)(nil)},
		{"@@ escapes to literal @", "@@", (*EscapedAtNode)(nil)},
		{"@{{ ... }} passes through as literal", "@{{ x }}", (*EscapedEchoNode)(nil)},
		{"@php ... @endphp block", "@php $x = 1; @endphp", (*PhpBlockNode)(nil)},
		{"@verbatim ... @endverbatim", "@verbatim{{ x }}@endverbatim", (*VerbatimNode)(nil)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src), nil)

			if len(file.Nodes) == 0 {
				t.Fatalf("no nodes produced")
			}

			var found bool
			for _, n := range file.Nodes {
				switch tc.want.(type) {
				case *EchoNode:
					_, found = n.(*EchoNode)
				case *RawEchoNode:
					_, found = n.(*RawEchoNode)
				case *CommentNode:
					_, found = n.(*CommentNode)
				case *DirectiveNode:
					_, found = n.(*DirectiveNode)
				case *EscapedAtNode:
					_, found = n.(*EscapedAtNode)
				case *EscapedEchoNode:
					_, found = n.(*EscapedEchoNode)
				case *PhpBlockNode:
					_, found = n.(*PhpBlockNode)
				case *VerbatimNode:
					_, found = n.(*VerbatimNode)
				}

				if found {
					break
				}
			}

			if !found {
				t.Fatalf("did not find a node of the expected kind in %v", file.Nodes)
			}
		})
	}
}

func TestDirectiveClosingParenThroughPhpLex(t *testing.T) {
	cases := []string{
		`@if("(")`,
		`@if(")")`,
		`@if('(')`,
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			file, errs := Parse([]byte(src), nil)

			if len(file.Nodes) != 1 {
				t.Fatalf("got %d nodes, want 1, directive did not consume its paren region", len(file.Nodes))
			}

			d, ok := file.Nodes[0].(*DirectiveNode)

			if !ok {
				t.Fatalf("node is %T, want *DirectiveNode", file.Nodes[0])
			}

			if d.RParen.End.Offset != len(src) {
				t.Fatalf("RParen.End=%d, want %d (directive did not consume full input)", d.RParen.End.Offset, len(src))
			}

			for _, e := range errs {
				if e.Msg == "expected ) to close directive arguments" {
					t.Fatalf("unexpected mismatched-paren diagnostic: %v", e)
				}
			}
		})
	}
}

func TestDirectiveBlockParses(t *testing.T) {
	src := "@if($x)\n<p>hi</p>\n@endif"
	file, errs := Parse([]byte(src), nil)

	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	if len(file.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(file.Nodes))
	}

	block, ok := file.Nodes[0].(*DirectiveBlockNode)

	if !ok {
		t.Fatalf("node is %T, want *DirectiveBlockNode", file.Nodes[0])
	}

	if string(block.Opener.Name) != "if" {
		t.Fatalf("opener name = %q, want \"if\"", block.Opener.Name)
	}

	if block.End == nil || string(block.End.Name) != "endif" {
		t.Fatalf("end = %v, want @endif", block.End)
	}
}

func TestDirectiveBlockBranchesAttachToIf(t *testing.T) {
	src := "@if($a)\na\n@elseif($b)\nb\n@else\nc\n@endif"
	file, errs := Parse([]byte(src), nil)

	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	block, ok := file.Nodes[0].(*DirectiveBlockNode)

	if !ok {
		t.Fatalf("node is %T, want *DirectiveBlockNode", file.Nodes[0])
	}

	if len(block.Branches) != 2 {
		t.Fatalf("got %d branches, want 2", len(block.Branches))
	}

	if string(block.Branches[0].Directive.Name) != "elseif" {
		t.Fatalf("branch 0 = %q, want \"elseif\"", block.Branches[0].Directive.Name)
	}

	if string(block.Branches[1].Directive.Name) != "else" {
		t.Fatalf("branch 1 = %q, want \"else\"", block.Branches[1].Directive.Name)
	}
}

func TestAttributeValueWithEmbeddedEchoExposesParts(t *testing.T) {
	src := `<div class="btn {{ $variant }} x"></div>`
	file, _ := Parse([]byte(src), nil)

	el, ok := file.Nodes[0].(*html.Element)

	if !ok {
		t.Fatalf("node is %T, want *html.Element", file.Nodes[0])
	}

	if len(el.Attributes) != 1 {
		t.Fatalf("got %d attributes, want 1", len(el.Attributes))
	}

	a := el.Attributes[0]

	if a.Parts == nil {
		t.Fatalf("Attr.Parts is nil; echo inside attribute value was not decomposed")
	}

	var sawEcho bool

	for _, part := range a.Parts {
		if _, ok := part.(*EchoNode); ok {
			sawEcho = true
		}
	}

	if !sawEcho {
		t.Fatalf("Attr.Parts contained no EchoNode: %v", a.Parts)
	}
}

func TestWireAttrValueStaysVerbatim(t *testing.T) {
	cases := []string{
		`<button wire:click="increment">+</button>`,
		`<input wire:model="form.name">`,
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			file, _ := Parse([]byte(src), nil)

			el, ok := file.Nodes[0].(*html.Element)
			if !ok {
				t.Fatalf("node is %T, want *html.Element", file.Nodes[0])
			}

			a := el.Attributes[0]

			if a.Bound {
				t.Fatalf("wire attr should not be marked Bound")
			}

			if a.Expr != nil {
				t.Fatalf("wire attr was reformatted as a PHP expression; want verbatim")
			}
		})
	}
}

func TestAttrEchoWithQuoteNotTruncated(t *testing.T) {
	src := `<button wire:click="f({{ $x ? "'on'" : 'off' }})" class="btn">x</button>`
	file, _ := Parse([]byte(src), nil)

	el, ok := file.Nodes[0].(*html.Element)
	if !ok {
		t.Fatalf("node is %T, want *html.Element", file.Nodes[0])
	}

	if len(el.Attributes) != 2 {
		t.Fatalf("got %d attributes, want 2: a quote inside the {{ }} echo truncated the attribute", len(el.Attributes))
	}

	want := `f({{ $x ? "'on'" : 'off' }})`
	if string(el.Attributes[0].Value) != want {
		t.Fatalf("wire:click value = %q, want %q", el.Attributes[0].Value, want)
	}

	if string(el.Attributes[1].Name) != "class" {
		t.Fatalf("second attribute name = %q, want class", el.Attributes[1].Name)
	}
}

func TestAlpineAttrValueStaysPlain(t *testing.T) {
	src := `<div x-data="{ count: 0 }" x-on:click="showMenu = !showMenu" @click="open = !open"></div>`
	file, _ := Parse([]byte(src), nil)

	el, ok := file.Nodes[0].(*html.Element)

	if !ok {
		t.Fatalf("node is %T, want *html.Element", file.Nodes[0])
	}

	for _, a := range el.Attributes {
		if a.Expr != nil {
			t.Fatalf("Alpine attr %q has Attr.Expr populated; JS should not route through the PHP parser", a.Name)
		}
	}
}

// Blade compiles a bound value only on a component tag; elsewhere `:name` is
// Alpine's x-bind shorthand, so JS that also parses as PHP must stay plain.
func TestBoundAttrIsPHPOnlyOnComponentTags(t *testing.T) {
	cases := []struct {
		src     string
		wantPHP bool
	}{
		{`<div :hidden="TRUE"></div>`, false},
		{`<div :hidden=TRUE></div>`, false},
		{`<x-card :hidden="TRUE"></x-card>`, true},
		{`<flux:badge :hidden=TRUE />`, true},
	}

	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			file, _ := Parse([]byte(tc.src), nil)

			el, ok := file.Nodes[0].(*html.Element)
			if !ok {
				t.Fatalf("node is %T, want *html.Element", file.Nodes[0])
			}

			if got := el.Attributes[0].Expr != nil; got != tc.wantPHP {
				t.Fatalf("Attr.Expr populated = %v, want %v", got, tc.wantPHP)
			}
		})
	}
}

func TestAlpineEventAttrParsesAsSingleAttr(t *testing.T) {
	cases := []struct {
		name          string
		attr          string
		wantName      string
		wantDirective bool
	}{
		{"event with modifier chain", `@keydown.window.cmd.1.prevent="setStatus('one')"`, "@keydown.window.cmd.1.prevent", false},
		{"event with value", `@click="open = !open"`, "@click", false},
		{"blade conditional class directive", `@class(['p-4' => $on])`, "", true},
		{"blade checked directive", `@checked($on)`, "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "<div " + tc.attr + "></div>"
			file, errs := Parse([]byte(src), nil)

			if len(errs) != 0 {
				t.Fatalf("unexpected parse errors: %v", errs)
			}

			el, ok := file.Nodes[0].(*html.Element)

			if !ok {
				t.Fatalf("node is %T, want *html.Element", file.Nodes[0])
			}

			if len(el.Attributes) != 1 {
				t.Fatalf("got %d attributes, want 1, the directive was split", len(el.Attributes))
			}

			a := el.Attributes[0]
			gotDirective := a.Directive != nil

			if gotDirective != tc.wantDirective {
				t.Fatalf("attr Directive set = %v, want %v", gotDirective, tc.wantDirective)
			}

			if !tc.wantDirective && string(a.Name) != tc.wantName {
				t.Fatalf("attr Name = %q, want %q", a.Name, tc.wantName)
			}
		})
	}
}

// Blade closes an echo at the first }} with no string awareness, so
// {{ "}}" }} does not compile in Blade either.
func TestEchoStopsAtFirstCloseMarker(t *testing.T) {
	const src = `{{ "}}" }}`

	file, errs := Parse([]byte(src), nil)

	if len(file.Nodes) == 0 {
		t.Fatalf("no nodes produced")
	}

	echo, ok := file.Nodes[0].(*EchoNode)

	if !ok {
		t.Fatalf("node is %T, want *EchoNode", file.Nodes[0])
	}

	if echo.Close.Start.Offset != 4 {
		t.Fatalf("echo close starts at offset %d, want 4 (the first `}}`)", echo.Close.Start.Offset)
	}

	if len(errs) == 0 {
		t.Fatalf("expected a parse error for the truncated expression; got none")
	}
}

func TestBlockDirectivesParse(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		opener string
		closer string
	}{
		{"pushOnce", "@pushOnce('scripts')\nx\n@endPushOnce", "pushOnce", "endPushOnce"},
		{"prependOnce", "@prependOnce('head')\nx\n@endPrependOnce", "prependOnce", "endPrependOnce"},
		{"hasSection", "@hasSection('nav')\nx\n@endif", "hasSection", "endif"},
		{"sectionMissing", "@sectionMissing('nav')\nx\n@endif", "sectionMissing", "endif"},
		{"fragment", "@fragment('list')\nx\n@endfragment", "fragment", "endfragment"},
		{"session", "@session('status')\nx\n@endsession", "session", "endsession"},
		{"hasstack", "@hasstack('scripts')\nx\n@endif", "hasstack", "endif"},
		{"hasStack", "@hasStack('scripts')\nx\n@endif", "hasStack", "endif"},
		{"context", "@context('canonical')\nx\n@endcontext", "context", "endcontext"},
		{"componentFirst", "@componentFirst(['a', 'b'])\nx\n@endComponentFirst", "componentFirst", "endComponentFirst"},
		{"section append", "@section('side')\nx\n@append", "section", "append"},
		{"section overwrite", "@section('side')\nx\n@overwrite", "section", "overwrite"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src), nil)

			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			if len(file.Nodes) != 1 {
				t.Fatalf("got %d nodes, want 1", len(file.Nodes))
			}

			block, ok := file.Nodes[0].(*DirectiveBlockNode)

			if !ok {
				t.Fatalf("node is %T, want *DirectiveBlockNode", file.Nodes[0])
			}

			if string(block.Opener.Name) != tc.opener {
				t.Fatalf("opener name = %q, want %q", block.Opener.Name, tc.opener)
			}

			if block.End == nil || string(block.End.Name) != tc.closer {
				t.Fatalf("end = %v, want @%s", block.End, tc.closer)
			}
		})
	}
}

func TestHasSectionElseBranchAttaches(t *testing.T) {
	src := "@hasSection('nav')\na\n@else\nb\n@endif"
	file, errs := Parse([]byte(src), nil)

	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	block, ok := file.Nodes[0].(*DirectiveBlockNode)

	if !ok {
		t.Fatalf("node is %T, want *DirectiveBlockNode", file.Nodes[0])
	}

	if len(block.Branches) != 1 {
		t.Fatalf("got %d branches, want 1", len(block.Branches))
	}

	if string(block.Branches[0].Directive.Name) != "else" {
		t.Fatalf("branch 0 = %q, want \"else\"", block.Branches[0].Directive.Name)
	}
}

func TestRawPhpRoleClassifiesAlternativeSyntax(t *testing.T) {
	tests := []struct {
		island                  string
		opens, closes, isBranch bool
	}{
		{island: "<?php if ($x): ?>", opens: true},
		{island: "<?php foreach ($a as $b): ?>", opens: true},
		{island: "<?php while ($x): ?>", opens: true},
		{island: "<?php elseif ($y): ?>", isBranch: true},
		{island: "<?php else: ?>", isBranch: true},
		{island: "<?php endif; ?>", closes: true},
		{island: "<?php endforeach; ?>", closes: true},

		// Brace syntax carries its own structure.
		{island: "<?php if ($x) { ?>"},
		{island: "<?php } ?>"},
		{island: "<?= $x ?>"},
		{island: "<?php $x = ['a' => 1]; ?>"},
		{island: "<?php $label = $x ? 'a' : 'b'; ?>"},
	}

	for _, tt := range tests {
		t.Run(tt.island, func(t *testing.T) {
			opens, closes, branch := rawPhpRole([]byte(tt.island))

			if opens != tt.opens || closes != tt.closes || branch != tt.isBranch {
				t.Errorf("rawPhpRole(%q) = (%v, %v, %v), want (%v, %v, %v)",
					tt.island, opens, closes, branch, tt.opens, tt.closes, tt.isBranch)
			}
		})
	}
}

// Each case puts the fault on the third line, so a sub-parse coordinate
// that leaks through cannot coincide with the file one.
func TestSubParseErrorsUseFileCoordinates(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		line, col int
	}{
		{"echo", "aaa\nbbb\n<p>{{ $a ? }}</p>\n", 3, 12},
		{"raw echo", "aaa\nbbb\n<p>{!! $a ? !!}</p>\n", 3, 13},
		{"directive args", "aaa\nbbb\n@if($a ? )\nx\n@endif\n", 3, 10},
		{"inline @php", "aaa\nbbb\n@php($a ? )\n", 3, 11},
		{"@php block", "aaa\nbbb\n@php\n  $x = ;\n@endphp\n", 4, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errs := Parse([]byte(tt.src), nil)

			if len(errs) == 0 {
				t.Fatalf("Parse reported no error; the case must produce one to pin its position")
			}

			if errs[0].Pos.Line != tt.line || errs[0].Pos.Column != tt.col {
				t.Errorf("first error at %d:%d, want %d:%d (%q)",
					errs[0].Pos.Line, errs[0].Pos.Column, tt.line, tt.col, errs[0].Msg)
			}

			lines := bytes.Split([]byte(tt.src), []byte("\n"))
			faulty := lines[tt.line-1]

			if errs[0].Pos.Column > len(faulty)+1 {
				t.Errorf("column %d is past the end of line %d (%q)", errs[0].Pos.Column, tt.line, faulty)
			}
		})
	}
}

func TestParseRawPhpIslandStatements(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		hasStmt bool
	}{
		{"complete statement", `<?php $x = 1; ?>`, true},
		{"several statements", `<?php $x = 1; $y = 2; ?>`, true},
		{"import", `<?php use App\Support\Helper; ?>`, true},
		{"uppercase open tag", `<?PHP $x = 1; ?>`, true},
		{"alt syntax opener", `<?php if ($a): ?>`, false},
		{"alt syntax branch", `<?php elseif ($b): ?>`, false},
		{"alt syntax closer", `<?php endif; ?>`, false},
		{"brace opener", `<?php if ($a) { ?>`, false},
		{"brace closer", `<?php } ?>`, false},
		{"short echo tag", `<?= $x ?>`, false},
		{"short open tag", `<? $x = 1; ?>`, false},
		{"comment only", `<?php // note ?>`, false},
		{"empty", `<?php ?>`, false},
		{"syntax error", `<?php $x = ; ?>`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, errs := Parse([]byte(tt.src), nil)

			if len(errs) > 0 {
				t.Fatalf("Parse reported %v; an island must never raise its body's errors", errs)
			}

			if len(file.Nodes) != 1 {
				t.Fatalf("parsed %d nodes, want 1", len(file.Nodes))
			}

			node, ok := file.Nodes[0].(*RawPhpNode)
			if !ok {
				t.Fatalf("node is %T, want *RawPhpNode", file.Nodes[0])
			}

			if got := len(node.Stmts) > 0; got != tt.hasStmt {
				t.Fatalf("has statements = %v, want %v", got, tt.hasStmt)
			}

			if body := tt.src[node.Body.Start.Offset:node.Body.End.Offset]; strings.Contains(body, "?>") {
				t.Fatalf("body %q reaches past the close tag", body)
			}
		})
	}
}

func TestBareAtStaysInTheTextAroundIt(t *testing.T) {
	for _, src := range []string{"<p>a @ b</p>", "<p>a @\n\nb</p>", "<p>a @1 b</p>"} {
		file, errs := Parse([]byte(src), nil)
		if len(errs) != 0 {
			t.Fatalf("%q: unexpected errors: %v", src, errs)
		}

		el, ok := file.Nodes[0].(*html.Element)
		if !ok {
			t.Fatalf("%q: node is %T, want *html.Element", src, file.Nodes[0])
		}

		if len(el.Children) != 1 {
			t.Fatalf("%q: %d children, want the text as one node", src, len(el.Children))
		}
	}
}

func TestUnquotedAttrValueEndsAtWhitespaceOrTagEnd(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		attrName  string
		value     string
		selfClose bool
	}{
		{"slash inside the value", "<a href=/docs>d</a>", "href", "/docs", false},
		{"slash-gt closes the tag", "<x-icon name=check/>", "name", "check", true},
		{"form feed separates attributes", "<a\fhref=\"x\">d</a>", "href", "x", false},
		// PHP runs the tag first, so neither the `>` of `?>` nor one it prints ends
		// the start tag.
		{"php tag", "<div a=<?=$v?>>t</div>", "a", "<?=$v?>", false},
		{"php tag printing a tag end", `<a b=<?= "/>" ?>>t</a>`, "b", `<?= "/>" ?>`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src), nil)
			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			el, ok := file.Nodes[0].(*html.Element)
			if !ok {
				t.Fatalf("node is %T, want *html.Element", file.Nodes[0])
			}

			a := el.Attributes[0]

			if string(a.Name) != tc.attrName || string(a.Value) != tc.value {
				t.Fatalf("attr = %q=%q, want %q=%q", a.Name, a.Value, tc.attrName, tc.value)
			}

			if el.SelfClose != tc.selfClose {
				t.Fatalf("SelfClose = %v, want %v", el.SelfClose, tc.selfClose)
			}
		})
	}
}

// A failed body search is remembered per opener, so doubling the nesting
// roughly quadruples the work (each opener scans to EOF once). Remembering it
// per enclosing depth as well made the growth cubic, about 8x per doubling.
func TestNestedUnclosedOpenersSearchOncePerOpener(t *testing.T) {
	allocs := func(levels int) float64 {
		src := []byte(strings.Repeat("@if(1)", levels) + "\n")

		return testing.AllocsPerRun(3, func() { Parse(src, nil) })
	}

	if ratio := allocs(32) / allocs(16); ratio > 5 {
		t.Fatalf("doubling the nesting multiplied allocations by %.1f, want at most 5", ratio)
	}
}

func TestRawBodyRecordsThePHPLiteralsWhoseLinesAreData(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"multi-line string in an echo", "{!! f([\n'a\nb',\n$c,\n]) !!}", []string{"'a\nb'"}},
		{"multi-line heredoc in a php block", "@php\n$x = <<<TXT\n  a\n  TXT;\n@endphp", []string{"<<<TXT\n  a\n  TXT"}},
		{"multi-line code without a literal", "{!! f([\n'a',\n$c,\n]) !!}", nil},
		{"single-line island", "{!! f('a') !!}", nil},
		{"code the lexer cannot read", "{!! f('a\n !!}", []string{"{!! f('a\n !!}"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := []byte("<script>\n" + tt.body + "\n</script>")

			file, errs := Parse(src, nil)
			if len(errs) > 0 {
				t.Fatalf("Parse returned errors: %v", errs)
			}

			script, ok := file.Nodes[0].(*html.Element)
			if !ok || len(script.Children) != 1 {
				t.Fatalf("parsed %#v, want a script element with one text child", file.Nodes[0])
			}

			text, ok := script.Children[0].(*html.Text)
			if !ok {
				t.Fatalf("script child is %T, want *html.Text", script.Children[0])
			}

			var got []string
			for _, s := range text.PHPLiterals {
				got = append(got, string(src[s.Start.Offset:s.End.Offset]))

				for _, pos := range []html.Position{s.Start, s.End} {
					before := src[:pos.Offset]
					line := bytes.Count(before, []byte("\n")) + 1
					column := pos.Offset - bytes.LastIndexByte(before, '\n')

					if pos.Line != line || pos.Column != column {
						t.Fatalf("position %+v, want line %d column %d", pos, line, column)
					}
				}
			}

			if !slices.Equal(got, tt.want) {
				t.Fatalf("PHPLiterals = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQuotedValueHoldingOutputKeepsItsQuote(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{"json directive", `'@json($v)'`, true},
		{"directive with a space before its arguments", `'@json ($v)'`, true},
		{"directive without arguments", `'@csrf'`, true},
		{"raw echo", `'{!! $raw !!}'`, true},
		{"php tag", `'<?= $x ?>'`, true},
		{"bare short tag", `'<?x'`, false},
		{"plain text", `'a b'`, false},
		{"escaped echo", `'{{ $x }}'`, false},
		{"at sign inside a word", `'mail@x(y)'`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, _ := Parse([]byte(`<div data-x=`+tc.value+`></div>`), nil)

			el, ok := file.Nodes[0].(*html.Element)
			if !ok || len(el.Attributes) != 1 {
				t.Fatalf("node = %T with attributes %v, want one *html.Element attribute", file.Nodes[0], el)
			}

			if got := el.Attributes[0].KeepQuote; got != tc.want {
				t.Fatalf("KeepQuote = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestQuoteInsideADirectiveOrTagDoesNotEndTheValue(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		value string
	}{
		{"directive arguments", `<v-x :channels='@json($c->map(fn ($c) => ['id' => $c->id]))' a="b"></v-x>`, `@json($c->map(fn ($c) => ['id' => $c->id]))`},
		{"space before the arguments", `<div title="@lang ("x")" a="b"></div>`, `@lang ("x")`},
		{"php tag", `<div data-p="<?= "q" ?>" a="b"></div>`, `<?= "q" ?>`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src), nil)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}

			el, ok := file.Nodes[0].(*html.Element)
			if !ok || len(el.Attributes) != 2 {
				t.Fatalf("node = %T with attributes %v, want two attributes", file.Nodes[0], el)
			}

			if got := string(el.Attributes[0].Value); got != tc.value {
				t.Fatalf("value = %q, want %q", got, tc.value)
			}
		})
	}
}

func TestAComponentClosesWithAnyCloseTagOfItsFamily(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantError string
	}{
		{"shorter flux close", `<flux:icon.check class="a"></flux:icon>`, ""},
		{"sibling component close", "<x-table.body.row>\n<x-table.cell>a</x-table.cell>\n</x-table.head.row>", ""},
		{"short slot close", "<x-card><x-slot:title>T</x-slot></x-card>", ""},
		{"livewire close", "<livewire:panel.item></livewire:panel>", ""},
		{"close from another family", "<x-card>t</flux:card>", "1:10: unexpected closing tag"},
		{"mismatched html close", "<div>t</span></div>", "1:7: unexpected closing tag"},
		{"outer close ends the inner component", "<x-outer><x-inner>t</x-outer>", "1:1: unclosed element"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := Parse([]byte(tc.src), nil)

			switch {
			case tc.wantError == "" && len(errs) > 0:
				t.Fatalf("Parse(%q) errors = %v, want none", tc.src, errs)
			case tc.wantError != "" && (len(errs) == 0 || errs[0].Error() != tc.wantError):
				t.Fatalf("Parse(%q) errors = %v, want first %q", tc.src, errs, tc.wantError)
			}
		})
	}
}

func TestARawBodyComponentEndsOnlyAtItsOwnClose(t *testing.T) {
	src := "<x-mail::message>\n# Hi\n<x-mail::button :url=\"$u\">Go</x-mail::button>\nThanks\n</x-mail::message>"

	file, errs := Parse([]byte(src), nil)
	if len(errs) > 0 {
		t.Fatalf("Parse errors = %v", errs)
	}

	el, ok := file.Nodes[0].(*html.Element)
	if !ok {
		t.Fatalf("node is %T, want *html.Element", file.Nodes[0])
	}

	if got := string(el.CloseName); got != "x-mail::message" {
		t.Fatalf("message closed at </%s>, want its own close", got)
	}
}

func TestAPhpDirectiveIsABlockExactlyWhenAnEndphpFollows(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantBlock bool
		wantError bool
	}{
		{"spaced args before an endphp", "@php ($a = 1) @endphp", true, false},
		{"glued args before an endphp", "@php($a = 1) @endphp", true, false},
		{"spaced args with no endphp", "@php ($b = 2)\n<p>{{ $b }}</p>", false, false},
		{"block then an unpaired statement", "@php ($a = 1) @endphp\n@php ($b = 2)\n<p>{{ $b }}</p>", true, false},
		{"a statement a later endphp swallows", "@php($a = 1)\n<p>x</p>\n@php\n$b = 2;\n@endphp", true, true},
		{"bare php with no endphp", "@php\n<p>x</p>", true, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src), nil)

			if got := len(errs) > 0; got != tc.wantError {
				t.Fatalf("Parse(%q) errors = %v, want error %v", tc.src, errs, tc.wantError)
			}

			_, block := file.Nodes[0].(*PhpBlockNode)
			if block != tc.wantBlock {
				t.Fatalf("first node is %T, want block %v", file.Nodes[0], tc.wantBlock)
			}
		})
	}
}

func TestParseRejectsAByteGluedToATagName(t *testing.T) {
	const noError = -1

	cases := []struct {
		src  string
		want int
	}{
		{`<span">@lang('a')</span>`, 5},
		{`<div{{ $a }}="v">x</div>`, 4},
		{`<x-alert{{ $attributes }}>x</x-alert>`, 8},
		{`<x-slot:title>x</x-slot:title>`, noError},
		{`<flux:icon.check />`, noError},
	}

	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			_, errs := Parse([]byte(tc.src), nil)

			got := noError

			for _, e := range errs {
				if e.Msg == "unexpected character after tag name" {
					got = e.Pos.Offset
				}
			}

			if got != tc.want {
				t.Fatalf("glued-name error at %d, want %d (errors: %v)", got, tc.want, errs)
			}
		})
	}
}

func TestParseReadsATagNamedByAnEchoOnlyWithItsClose(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		element bool
	}{
		{"paired", `<{{ $a }} class="b">x</{{ $a }}>`, true},
		{"self-closed", `<{{ $a }} class="b" />`, true},
		{"empty close does not end it", `<{{ $a }}>{{ $b }}</></{{ $a }}>`, true},
		{"close ends an open child", `<{{ $a }}><p>x</{{ $a }}>`, true},
		{"no close in prose", `a <{{ $b }}> c`, false},
		{"close of another echo", `<{{ $a }}>x</{{ $b }}>`, false},
		{"attributes that do not parse", `<{{ $a }}<?php if ($b) : ?>="c"<?php endif ?>>x</{{ $a }}>`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src), nil)
			if len(errs) > 0 {
				t.Fatalf("errors: %v", errs)
			}

			el, ok := file.Nodes[0].(*html.Element)
			if ok != tc.element {
				t.Fatalf("first node is %T, want element %v", file.Nodes[0], tc.element)
			}

			if ok && !el.SelfClose && el.CloseNameEcho == nil {
				t.Fatalf("closed by %q, want the echo close", tc.src[el.CloseOpen.Start.Offset:])
			}
		})
	}
}

func TestParsePairsACloseTagByTheEchoGluedToItsName(t *testing.T) {
	const unpaired = 2

	cases := []struct {
		name string
		src  string
		want int
	}{
		{"spacing inside the echo differs", `<h{{$a}}>x</h{{ $a }}>`, 0},
		{"echo was an attribute bag", `<div{!! $a !!}>x</div>`, 0},
		{"close ends an open child", `<h{{ $a }}><p>x</h{{ $a }}>`, 0},
		{"other expression", `<h{{ $a }}>x</h{{ $b }}>`, unpaired},
		{"raw echo against escaped", `<h{{ $a }}>x</h{!! $a !!}>`, unpaired},
		{"unparsable close echo reports no PHP error", `<h{{ $a }}><p>x</h{{ $a + }}>`, unpaired},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := Parse([]byte(tc.src), nil)

			if len(errs) != tc.want {
				t.Fatalf("got %d errors, want %d: %v", len(errs), tc.want, errs)
			}
		})
	}
}
