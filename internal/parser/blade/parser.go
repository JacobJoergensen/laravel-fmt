package blade

import (
	"bytes"
	"slices"

	parserpkg "github.com/JacobJoergensen/laravel-fmt/internal/parser"
	"github.com/JacobJoergensen/laravel-fmt/internal/parser/html"
	"github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

type bladeParser struct {
	parserpkg.Cursor

	errors []Error

	dirs *Directives

	// closeIndex maps each lowercased tag name to the ascending offsets of its
	// </name> close tags. Built lazily and reused so void/paired disambiguation
	// is a binary search, not a per-element rescan. nil until built.
	closeIndex map[string][]int

	// blockStack holds the openers of the Blade blocks currently being parsed. An
	// HTML element opened inside a block body must not consume that block's closer
	// or branch directives: Blade block structure dominates HTML nesting, so the
	// element stops at the boundary as a deferred-open tag.
	blockStack []string

	// failedBlocks remembers where tryParseBlockBody already searched for a
	// closer and found none. Without it the search is exponential: the opener
	// parses its body to EOF, rolls back, and the caller parses the same text
	// again as flat directives, so each nesting level doubles the work.
	//
	// The key leaves out blockStack. The body loop stops only at its own
	// closer, and an element that stops early at an enclosing block's closer
	// changes the body's shape, not whether that closer is reached. Keying on
	// the stack made every opener search once per depth it was reached at.
	failedBlocks map[blockMemoKey]bool

	// depthErr lives apart from errors because a block or element that rolls
	// back truncates errors, and the cap must survive that. Once set, every
	// node parse jumps to end, the position EOF was first reached at.
	depth    int
	depthErr *Error
	end      Position
}

// maxNestingDepth caps elements and blocks together, since both recurse
// through parseNext: deeper, a pathological file exhausts the Go stack, a
// fatal error no recover can catch.
const maxNestingDepth = 500

type blockMemoKey struct {
	offset int
	name   string
}

// blockDirectiveClosers maps a block opener to its closers. @hasSection,
// @sectionMissing and @hasstack close with @endif because they compile to a
// plain if. Keys
// are case-sensitive in Blade's documented casing, so @pushOnce and
// @prependOnce are camelCase.
//
// Livewire's @island, @placeholder and @teleport wrap markup, so their bodies
// indent. So do @script and @assets: their body is a `<script>` or `<link>`
// element whose tag indents like any markup while a raw-text body stays
// verbatim, as it does inside @push.
var blockDirectiveClosers = map[string][]string{
	"if":             {"endif"},
	"unless":         {"endunless"},
	"isset":          {"endisset"},
	"empty":          {"endempty"},
	"for":            {"endfor"},
	"foreach":        {"endforeach"},
	"while":          {"endwhile"},
	"once":           {"endonce"},
	"section":        {"endsection", "show", "stop", "append", "overwrite"},
	"hasSection":     {"endif"},
	"sectionMissing": {"endif"},
	"hasstack":       {"endif"},
	"hasStack":       {"endif"},
	"push":           {"endpush"},
	"pushOnce":       {"endPushOnce"},
	"prepend":        {"endprepend"},
	"prependOnce":    {"endPrependOnce"},
	"fragment":       {"endfragment"},
	"session":        {"endsession"},
	"context":        {"endcontext"},
	"auth":           {"endauth"},
	"guest":          {"endguest"},
	"error":          {"enderror"},
	"production":     {"endproduction"},
	"env":            {"endenv"},
	"can":            {"endcan"},
	"cannot":         {"endcannot"},
	"canany":         {"endcanany"},
	"switch":         {"endswitch"},
	"forelse":        {"endforelse"},
	"island":         {"endisland"},
	"placeholder":    {"endplaceholder"},
	"teleport":       {"endteleport"},
	"script":         {"endscript"},
	"assets":         {"endassets"},
	"persist":        {"endpersist"},
	"volt":           {"endvolt"},
	"pushIf":         {"endPushIf"},
	"component":      {"endcomponent"},
	"componentFirst": {"endComponentFirst"},
	"slot":           {"endslot"},
}

// @break is not a branch: it ends a case body rather than opening a peer
// clause, so it stays a flat DirectiveNode, which is also right in loops.
var blockDirectiveBranches = map[string][]string{
	"if":             {"elseif", "else"},
	"unless":         {"else"},
	"isset":          {"else"},
	"empty":          {"else"},
	"auth":           {"elseauth", "else"},
	"guest":          {"elseguest", "else"},
	"env":            {"else"},
	"production":     {"else"},
	"can":            {"elsecan", "else"},
	"cannot":         {"elsecannot", "else"},
	"canany":         {"elsecanany", "else"},
	"hasSection":     {"elseif", "else"},
	"sectionMissing": {"elseif", "else"},
	"hasstack":       {"elseif", "else"},
	"hasStack":       {"elseif", "else"},
	"pushIf":         {"elsePushIf", "elsePush"},
	"switch":         {"case", "default"},
	"forelse":        {"empty"},
}

// blockDirectiveBareBranches holds branches that count only when bare: in
// @forelse, a bare @empty is the "collection empty" clause, while
// @empty(expr) opens its own @empty/@endempty block.
var blockDirectiveBareBranches = map[string]map[string]bool{
	"forelse": {"empty": true},
}

func (p *bladeParser) addError(pos Position, msg string) {
	if p.depthErr != nil {
		return
	}

	p.errors = append(p.errors, Error{Pos: pos, Msg: msg})
}

func (p *bladeParser) exceedDepth() {
	if p.depthErr == nil {
		p.depthErr = &Error{Pos: p.Pos, Msg: "maximum nesting depth exceeded"}
		p.AdvanceN(len(p.Src) - p.Pos.Offset)
		p.end = p.Pos
	}

	p.Pos = p.end
}

// rehomeErrors moves PHP sub-parse errors onto file coordinates. The sub-parsers
// receive a bounded slice of the template, so they number it from 1:1 and their
// positions would otherwise name an unrelated line of the file. Only errors are
// re-homed: the spans in the returned tree stay relative to the slice, which is
// what the formatter reads them against.
func rehomeErrors(errs []Error, base Position) []Error {
	for i, err := range errs {
		abs := Position{
			Offset: base.Offset + err.Pos.Offset,
			Line:   base.Line + err.Pos.Line - 1,
			Column: err.Pos.Column,
		}

		if err.Pos.Line == 1 {
			abs.Column = base.Column + err.Pos.Column - 1
		}

		errs[i].Pos = abs
	}

	return errs
}

func (p *bladeParser) parseFile() []Node {
	var nodes []Node

	for !p.Eof() {
		if n := p.parseNext(nil); n != nil {
			nodes = append(nodes, n)
		}
	}

	return nodes
}

// parseNext dispatches on the leading bytes; Blade markers are checked before
// HTML so template constructs win over similar-looking HTML. stack is the
// currently-open HTML element names, empty at the top level and inside
// directive bodies, and bounds void-element relaxation and the omitted-end-tag
// boundary.
func (p *bladeParser) parseNext(stack [][]byte) Node {
	p.depth++
	defer func() { p.depth-- }()

	if p.depth > maxNestingDepth || p.depthErr != nil {
		p.exceedDepth()

		return nil
	}

	switch {
	case p.StartsWith("{{--"):
		return p.parseComment()
	case p.StartsWith("{!!"):
		return p.parseRawEcho()
	case p.StartsWith("{{"):
		return p.parseEcho()
	case p.atBladeAt():
		return p.parseAt()
	case p.StartsWith("<?"):
		return p.parseRawPhp()
	case p.StartsWith("<!--"):
		return p.parseHTMLComment()
	case p.StartsWithFold("<!doctype"):
		return p.parseDoctype()
	case p.isTagOpen():
		return p.parseElement(stack)
	case html.IsTagClose(&p.Cursor):
		return p.parseStrayClose(stack)
	case p.atDynamicTagClose():
		return p.parseUnclaimedDynamicClose()
	default:
		return p.parseText()
	}
}

func (p *bladeParser) isTagOpen() bool {
	return p.Peek() == '<' && (html.IsNameStart(p.PeekAt(1)) || p.atDynamicTagOpen())
}

func (p *bladeParser) atDynamicTagOpen() bool {
	return p.StartsWith("<{{") || p.StartsWith("<{!!")
}

func (p *bladeParser) atDynamicTagClose() bool {
	return p.StartsWith("</{{") || p.StartsWith("</{!!")
}

// Takes only the `</`: the echo may hold a `>`, so it parses as an echo next.
func (p *bladeParser) parseUnclaimedDynamicClose() Node {
	start := p.Pos
	p.AdvanceN(len("</"))

	return &html.Text{Start: start, End: p.Pos, Text: p.Src[start.Offset:p.Pos.Offset]}
}

func (p *bladeParser) parseText() Node {
	start := p.Pos

	for !p.Eof() && !p.isBladeMarker() {
		p.Advance()
	}

	if p.Pos.Offset == start.Offset {
		return nil
	}

	return &html.Text{
		Start: start, End: p.Pos,
		Text: p.Src[start.Offset:p.Pos.Offset],
	}
}

// parseRawPhp captures a `<?php … ?>` or `<?= … ?>` island whole, so the
// emitter can lay it out as a structural boundary instead of reflowing it as
// text. A `<?` with no closing `?>` becomes a verbatim island that stops at
// the next real marker, so it cannot swallow the markup that follows.
func (p *bladeParser) parseRawPhp() Node {
	start := p.Pos
	p.AdvanceN(2)

	// The island ends where PHP says it does: in `"?> <div class='x'>"` the
	// `?>` is inside a string.
	closeAt := php.CloseTagOffset(p.Src[p.Pos.Offset:])
	if closeAt < 0 {
		bodyStart := p.Pos

		// A `?>` hidden in a string or comment can still close the island on
		// the next pass, once reflowing the text before it ends that string or
		// comment early. So the island runs through the last `?>` ahead; the
		// formatter writes no `?>` of its own, so nothing past it can close it.
		if hidden := bytes.LastIndex(p.Src[p.Pos.Offset:], []byte("?>")); hidden >= 0 {
			p.AdvanceN(hidden + len("?>"))
		}

		// PHP needs no closing tag: after an unclosed `<?php` or `<?=` the rest
		// of the file is PHP, markup-looking strings included. A bare `<?` is
		// only PHP with short tags on, so it stops at the next marker instead.
		toEOF := p.StartsWithFold("php") || p.StartsWith("=")

		for !p.Eof() && (toEOF || !p.isBladeMarker() || p.StartsWith("<?")) {
			p.Advance()
		}

		// Trailing whitespace belongs to the next node, as after a closed
		// island. Kept inside, it would gain the file's final newline on every
		// pass.
		end := start.Offset + len(bytes.TrimRight(p.Src[start.Offset:p.Pos.Offset], " \t\r\n"))
		p.Pos = bodyStart

		for p.Pos.Offset < end {
			p.Advance()
		}

		// Unclosed, so no layout of it is known to be safe: reflowing it can
		// pull a `?>` onto a `#` comment's line and close the island there. It
		// goes out as written.
		return &RawPhpNode{Start: start, End: p.Pos, Body: Span{Start: bodyStart, End: p.Pos}}
	}

	closeAt += p.Pos.Offset

	longTag := p.StartsWithFold("php")

	if longTag {
		p.AdvanceN(len("php"))
	}

	bodyStart := p.Pos

	for p.Pos.Offset < closeAt {
		p.Advance()
	}

	bodyEnd := p.Pos

	p.AdvanceN(2)

	node := &RawPhpNode{Start: start, End: p.Pos, Body: Span{Start: bodyStart, End: bodyEnd}}
	node.Opens, node.Closes, node.Branch = rawPhpRole(p.Src[start.Offset:p.Pos.Offset])

	if longTag {
		node.Stmts, node.Tokens = rawPhpStmts(p.Src[bodyStart.Offset:bodyEnd.Offset])
	}

	// A complete statement list is no half of an alt-syntax structure, though
	// its text can end in `:` like one: `if(0)A:` is an if whose body is the
	// label `A:`, and an indent opened after it would not survive the next
	// pass.
	if node.Stmts != nil {
		node.Opens, node.Closes, node.Branch = false, false, false
	}

	return node
}

// rawPhpStmts returns the island body's statements when it parses as a
// complete statement list, and nil otherwise so the island stays verbatim.
// Errors are dropped, not reported: half of an alt-syntax or brace
// structure (`if ($a):`, `}`, `} else {`) cannot parse alone but is valid
// Blade, so an error here must not reject the file.
//
// Only a `<?php` island may come here. A `<?=` body is an expression, which
// also parses as a statement, so `<?= $x ?>` would print as `<?= $x; ?>`.
func rawPhpStmts(body []byte) ([]php.Stmt, []php.Token) {
	stmts, tokens, errs := php.ParseStatements(body)
	if len(errs) > 0 {
		return nil, nil
	}

	return stmts, tokens
}

// switch has an alt-syntax form, but its case and default indent deeper than
// a branch, which the emitter's flat counter cannot express, so they are not
// branches.
var altSyntaxOpeners = []string{"if", "foreach", "for", "while", "switch"}

var altSyntaxClosers = []string{"endif", "endforeach", "endfor", "endwhile", "endswitch"}

// rawPhpRole classifies an island's part in an alt-syntax structure. Brace syntax
// stays unclassified, so an island the emitter cannot pair never moves an indent.
func rawPhpRole(island []byte) (opens, closes, branch bool) {
	body := bytes.TrimSuffix(island, []byte("?>"))
	for _, open := range []string{"<?php", "<?=", "<?"} {
		if trimmed, ok := bytes.CutPrefix(body, []byte(open)); ok {
			body = trimmed

			break
		}
	}

	body = bytes.TrimSpace(body)

	for _, kw := range altSyntaxClosers {
		if startsWithKeyword(body, kw) {
			return false, true, false
		}
	}

	if !bytes.HasSuffix(body, []byte(":")) {
		return false, false, false
	}

	if startsWithKeyword(body, "else") || startsWithKeyword(body, "elseif") {
		return false, false, true
	}

	for _, kw := range altSyntaxOpeners {
		if startsWithKeyword(body, kw) {
			return true, false, false
		}
	}

	return false, false, false
}

func startsWithKeyword(body []byte, kw string) bool {
	if len(body) < len(kw) || !bytes.EqualFold(body[:len(kw)], []byte(kw)) {
		return false
	}

	return len(body) == len(kw) || !isKeywordChar(body[len(kw)])
}

func isKeywordChar(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// atAtMarker reports whether the `@` at the cursor can open a Blade construct.
// Laravel's statement regex starts `\B@`, so a `@` glued to the end of a word is
// literal text: `support@gmail.com` is an address, not a `@gmail` directive.
//
// The raw-block (`@php`, `@verbatim`) and escaped-echo (`@{{`, `@{!!`) passes run
// before the statement pass and match without `\B`, so those four still open
// after a word character.
func (p *bladeParser) atAtMarker() bool {
	off := p.Pos.Offset

	if off == 0 || !isKeywordChar(p.Src[off-1]) {
		return true
	}

	return p.atRawBlockOpener() || p.StartsWith("@{{") || p.StartsWith("@{!!")
}

func (p *bladeParser) atRawBlockOpener() bool {
	return p.namesDirective("verbatim") || p.atPhpBlock()
}

// atPhpBlock reports whether the cursor is at a `@php` that Blade pairs with a
// later `@endphp`: its storePhpBlocks matches `@php(.*?)@endphp` whatever follows
// the name, so `@php ($x)` is a statement only when no `@endphp` comes after it.
func (p *bladeParser) atPhpBlock() bool {
	return p.namesDirective(phpDirective) && p.hasCloseAhead("@endphp")
}

// GluedLiteralAt reports whether the byte at off is a `@` that Blade reads as
// literal text only because a word character precedes it. A line break emitted in
// front of such a `@` would replace that word character and promote the text to a
// directive, so the formatter has to keep the boundary glued.
func GluedLiteralAt(src []byte, off int) bool {
	return off > 0 && off < len(src) && src[off] == '@' && isKeywordChar(src[off-1])
}

// Any other `@` is text and stays inside the text around it.
func (p *bladeParser) atBladeAt() bool {
	if p.Peek() != '@' || !p.atAtMarker() {
		return false
	}

	return p.PeekAt(1) == '@' || p.StartsWith("@{{") || p.StartsWith("@{!!") || html.IsNameStart(p.PeekAt(1))
}

func (p *bladeParser) isBladeMarker() bool {
	switch p.Peek() {
	case '@':
		return p.atBladeAt()
	case '{':
		return p.PeekAt(1) == '{' || (p.PeekAt(1) == '!' && p.PeekAt(2) == '!')
	case '<':
		return p.isTagOpen() || html.IsTagClose(&p.Cursor) || p.atDynamicTagClose() || p.StartsWith("<?") ||
			p.StartsWith("<!--") || p.StartsWithFold("<!doctype")
	}

	return false
}

// A `{{{ … }}}` echo is still live Blade: illuminate/view compiles it in
// compileEscapedEchos to the same `echo e(…)` as `{{ }}`. The delimiter
// width is kept in the Open and Close spans, which is all the emitter needs
// to reproduce the form the author wrote.
func (p *bladeParser) parseEcho() Node {
	openStart := p.Pos

	closer := "}}"
	if p.StartsWith("{{{") {
		closer = "}}}"
	}

	p.AdvanceN(len(closer))

	open := Span{Start: openStart, End: p.Pos}

	contentOffset := p.Pos.Offset

	for !p.Eof() && !p.StartsWith(closer) {
		p.Advance()
	}

	closeStart := p.Pos

	// Without a closer the rest of the file is no echo's arguments; parsing it
	// as PHP would report its markup first.
	if !p.StartsWith(closer) {
		p.addError(openStart, "unterminated Blade echo")

		return &EchoNode{Open: open, Close: Span{Start: closeStart, End: p.Pos}}
	}

	content := p.Src[contentOffset:p.Pos.Offset]
	args, tokens := p.parseEchoArgs(content, open.End, false)

	p.AdvanceN(len(closer))
	p.takeSwallowedCR()

	return &EchoNode{
		Open: open, Args: args, Tokens: tokens,
		Close: Span{Start: closeStart, End: p.Pos},
	}
}

func (p *bladeParser) parseRawEcho() Node {
	openStart := p.Pos
	p.AdvanceN(3)
	open := Span{Start: openStart, End: p.Pos}

	contentOffset := p.Pos.Offset

	for !p.Eof() && !p.StartsWith("!!}") {
		p.Advance()
	}

	closeStart := p.Pos

	if !p.StartsWith("!!}") {
		p.addError(openStart, "unterminated Blade raw echo")

		return &RawEchoNode{Open: open, Close: Span{Start: closeStart, End: p.Pos}}
	}

	content := p.Src[contentOffset:p.Pos.Offset]
	args, tokens := p.parseEchoArgs(content, open.End, true)

	p.AdvanceN(3)
	p.takeSwallowedCR()

	return &RawEchoNode{
		Open: open, Args: args, Tokens: tokens,
		Close: Span{Start: closeStart, End: p.Pos},
	}
}

// parseEchoArgs parses echo content as the list Blade compiles it into:
// e()'s call arguments for `{{ }}`, echo's expression list for `{!! !!}`,
// where a named argument, spread or trailing comma does not parse.
func (p *bladeParser) parseEchoArgs(content []byte, base Position, raw bool) ([]php.Expr, []php.Token) {
	args, tokens, errs := php.ParseCallArgs(content)
	eof := tokens[len(tokens)-1]

	if len(args) == 0 && len(errs) == 0 {
		errs = append(errs, Error{Pos: eof.Span.Start, Msg: "unexpected EOF"})
	}

	// Blade compiles the echo onto one line, `<?php echo e(x // note); ?>`, so a
	// line comment that runs to the closer comments out the call's `);` and PHP
	// does not parse the template.
	if n := len(eof.Leading); n > 0 && eof.Leading[n-1].Kind == php.TriviaLineComment {
		errs = append(errs, Error{Pos: eof.Leading[n-1].Span.Start, Msg: "a line comment runs to the echo's closer, which Blade compiles onto the same line"})
	}

	if raw {
		for _, arg := range args {
			switch arg.(type) {
			case *php.NamedArgExpr, *php.SpreadExpr:
				errs = append(errs, Error{Pos: arg.Span().Start, Msg: "a raw echo is an echo list; named arguments and spread do not parse"})
			}
		}

		if len(tokens) >= 2 && tokens[len(tokens)-2].Kind == php.KindComma {
			errs = append(errs, Error{Pos: tokens[len(tokens)-2].Span.Start, Msg: "a raw echo is an echo list; a trailing comma does not parse"})
		}
	}

	p.errors = append(p.errors, rehomeErrors(errs, base)...)

	return args, tokens
}

// takeSwallowedCR makes a lone CR after an echo's closer part of the echo,
// the way it renders. Blade doubles a following `\n` or `\r\n` so one
// survives PHP eating the newline after `?>`, but has no lone-CR case, so
// PHP eats the CR and `{{ $a }}<CR>b` renders `Ab`.
func (p *bladeParser) takeSwallowedCR() {
	if p.Peek() == '\r' && p.PeekAt(1) != '\n' {
		p.Advance()
	}
}

func (p *bladeParser) parseComment() Node {
	openStart := p.Pos
	p.AdvanceN(4)
	open := Span{Start: openStart, End: p.Pos}

	contentOffset := p.Pos.Offset

	for !p.Eof() && !p.StartsWith("--}}") {
		p.Advance()
	}

	text := p.Src[contentOffset:p.Pos.Offset]
	closeStart := p.Pos

	if p.StartsWith("--}}") {
		p.AdvanceN(4)
	} else {
		p.addError(openStart, "unterminated Blade comment")
	}

	return &CommentNode{
		Open: open, Text: text,
		Close: Span{Start: closeStart, End: p.Pos},
	}
}

func (p *bladeParser) parseAt() Node {
	start := p.Pos
	p.Advance()

	if p.Peek() == '@' {
		p.Advance()

		return &EscapedAtNode{Start: start, End: p.Pos}
	}

	if p.StartsWith("{{") {
		return p.parseEscapedEcho(start, "{{", "}}")
	}

	if p.StartsWith("{!!") {
		return p.parseEscapedEcho(start, "{!!", "!!}")
	}

	name := p.readDirectiveName()

	switch string(name) {
	case phpDirective:
		// The name is consumed, so the block test looks past it; a bare @php
		// without an @endphp still takes the block path for its error.
		if p.hasCloseAhead("@endphp") || !p.directiveArgsFollow() {
			return p.parsePhpBlock(start)
		}
	case "verbatim":
		return p.parseVerbatimBlock(start)
	}

	dir := p.parseDirective(start, name)

	if p.dirs.IsOpener(string(name)) {
		if block := p.tryParseBlockBody(dir); block != nil {
			return block
		}
	}

	return dir
}

// tryParseBlockBody consumes nodes until opener's closer. Returns nil when the
// closer is never found, having rewound to just after the opener so the caller
// treats it as a flat DirectiveNode and reparses the body as siblings. The
// recovery is silent: a diagnostic here would make the formatter bail on the
// next pass, since auto-closing a parent element (`<li>…@if</li>`) turns an
// opener that was at EOF on this pass into one with a trailing close tag.
func (p *bladeParser) tryParseBlockBody(opener *DirectiveNode) *DirectiveBlockNode {
	name := string(opener.Name)
	closers := p.dirs.closers[name]

	// A block opener with only trailing whitespace before EOF is a bare
	// directive fragment, not an unclosed block. Trailing whitespace counts
	// as EOF so the emitter's final newline cannot turn a formatted fragment
	// into a block on the next pass.
	if p.Eof() || len(bytes.TrimSpace(p.Src[p.Pos.Offset:])) == 0 {
		return nil
	}

	key := blockMemoKey{offset: p.Pos.Offset, name: name}
	if p.failedBlocks[key] {
		return nil
	}

	savedPos := p.Pos
	savedErrCount := len(p.errors)

	p.blockStack = append(p.blockStack, name)
	defer func() { p.blockStack = p.blockStack[:len(p.blockStack)-1] }()

	block := &DirectiveBlockNode{Opener: opener}
	currentBody := &block.Body

	for !p.Eof() {
		if p.atOneOfDirectives(closers) {
			closeStart := p.Pos
			p.Advance()
			closeName := p.readDirectiveName()
			block.End = p.parseDirective(closeStart, closeName)

			return block
		}

		if p.matchBranchForOpener(name) {
			branchStart := p.Pos
			p.Advance()
			branchName := p.readDirectiveName()
			br := &DirectiveBranch{Directive: p.parseDirective(branchStart, branchName)}
			block.Branches = append(block.Branches, br)
			currentBody = &br.Body

			continue
		}

		child := p.parseNext(nil)

		if child != nil {
			*currentBody = append(*currentBody, child)
		}
	}

	p.Pos = savedPos
	p.errors = p.errors[:savedErrCount]

	if p.failedBlocks == nil {
		p.failedBlocks = make(map[blockMemoKey]bool)
	}

	p.failedBlocks[key] = true

	return nil
}

func (p *bladeParser) atDirective(name string) bool {
	return p.namesDirective(name) && p.atAtMarker()
}

// namesDirective reports whether the cursor is at `@name` with a name boundary
// after it. It does not apply the `\B` rule, so it is the right check for a
// caller that has already established the position opens a construct.
func (p *bladeParser) namesDirective(name string) bool {
	full := "@" + name
	if !p.StartsWith(full) {
		return false
	}

	next := p.PeekAt(len(full))

	return !html.IsNameStart(next) && !html.IsDigit(next)
}

func (p *bladeParser) atOneOfDirectives(names []string) bool {
	return slices.ContainsFunc(names, p.atDirective)
}

func (p *bladeParser) matchBranchForOpener(opener string) bool {
	branches := p.dirs.branches[opener]
	bareMap := blockDirectiveBareBranches[opener]

	for _, b := range branches {
		if !p.atDirective(b) {
			continue
		}

		if bareMap[b] && p.PeekAt(1+len(b)) == '(' {
			continue
		}

		return true
	}

	return false
}

func (p *bladeParser) parseEscapedEcho(at Position, open, closer string) Node {
	openStart := p.Pos
	p.AdvanceN(len(open))
	openSpan := Span{Start: openStart, End: p.Pos}

	contentOffset := p.Pos.Offset

	for !p.Eof() && !p.StartsWith(closer) {
		p.Advance()
	}

	text := p.Src[contentOffset:p.Pos.Offset]
	closeStart := p.Pos

	if p.StartsWith(closer) {
		p.AdvanceN(len(closer))
	} else {
		p.addError(p.Pos, "expected "+closer+" to close escaped echo")
	}

	return &EscapedEchoNode{
		At: at, Open: openSpan, Text: text,
		Close: Span{Start: closeStart, End: p.Pos},
	}
}

func (p *bladeParser) readDirectiveName() []byte {
	startOff := p.Pos.Offset

	for !p.Eof() {
		c := p.Peek()

		if html.IsNameStart(c) || (html.IsDigit(c) && p.Pos.Offset > startOff) {
			p.Advance()
			continue
		}

		break
	}

	return p.Src[startOff:p.Pos.Offset]
}

const phpDirective = "php"

// statementShapedDirectives take a statement-shaped argument, not a call
// argument list: @foreach and @forelse use `expr as value`, @for three
// semicolon clauses. php.ParseCallArgs would reject that, so parseLoopArg
// parses it as the loop statement instead.
var statementShapedDirectives = map[string]bool{
	"foreach": true,
	"forelse": true,
	"for":     true,
}

// parseLoopArg parses a loop directive's argument inside the statement Blade
// compiles it to, `foreach (<arg>) {}` for @foreach and @forelse and
// `for (<arg>) {}` for @for, so the formatter can lay out its clause as PHP.
// That statement is complete, so an argument that does not parse as exactly it
// cannot compile either (Laravel rejects `@foreach($a as)` as malformed), and
// it is reported where the argument starts, at base, like a call argument
// that does not parse.
func (p *bladeParser) parseLoopArg(name string, raw []byte, base Position) (php.Stmt, []php.Token, []byte) {
	keyword := "foreach"
	if name == "for" {
		keyword = "for"
	}

	prefix := keyword + " ("

	src := make([]byte, 0, len(prefix)+len(raw)+len(") {}"))
	src = append(src, prefix...)
	src = append(src, raw...)
	src = append(src, ") {}"...)

	stmts, tokens, errs := php.ParseStatements(src)

	if len(errs) == 0 && len(stmts) == 1 {
		switch stmts[0].(type) {
		case *php.ForEachStmt, *php.ForStmt:
			return stmts[0], tokens, src
		}
	}

	if len(errs) == 0 {
		errs = []Error{{Pos: php.Position{Offset: len(prefix), Line: 1, Column: len(prefix) + 1}, Msg: "expected a single " + keyword + " clause"}}
	}

	// The positions count the wrapper's prefix, which is not in the file.
	for i := range errs {
		errs[i].Pos.Offset = max(errs[i].Pos.Offset-len(prefix), 0)

		if errs[i].Pos.Line == 1 {
			errs[i].Pos.Column = max(errs[i].Pos.Column-len(prefix), 1)
		}
	}

	p.errors = append(p.errors, rehomeErrors(errs[:1], base)...)

	return nil, nil, nil
}

func (p *bladeParser) parseDirective(at Position, name []byte) *DirectiveNode {
	d := &DirectiveNode{At: at, Name: name, NameEnd: p.Pos}

	if p.directiveArgsFollow() {
		for p.Peek() == ' ' || p.Peek() == '\t' {
			p.Advance()
		}

		switch {
		case statementShapedDirectives[string(name)]:
			d.LParen, d.Raw, d.RParen = p.parseDirectiveRaw()
			if d.RParen.End.Offset > d.RParen.Start.Offset {
				d.Loop, d.LoopTokens, d.LoopSrc = p.parseLoopArg(string(name), d.Raw, d.LParen.End)
			}

			if d.Loop == nil {
				tokens, errs := php.LexBare(d.Raw)
				d.RawTokens, d.RawLexed = tokens, len(errs) == 0
			}
		case string(name) == phpDirective:
			d.LParen, d.Args, d.Tokens, d.RParen = p.parseDirectivePhp()
		default:
			d.LParen, d.Args, d.Tokens, d.Raw, d.RParen = p.parseDirectiveArgs(string(name))
			d.Opaque = d.Raw != nil
		}
	}

	return d
}

// directiveArgsFollow reports whether a ( opens after the name, skipping only
// horizontal whitespace: @if(...) and @if (...) are the same directive, but a (
// on a later line is unrelated content. The spaces are consumed, not stored;
// the emitter re-derives spacing.
func (p *bladeParser) directiveArgsFollow() bool {
	off := p.Pos.Offset

	for off < len(p.Src) && (p.Src[off] == ' ' || p.Src[off] == '\t') {
		off++
	}

	return off < len(p.Src) && p.Src[off] == '('
}

// parseDirectiveArgs parses a directive's argument as a PHP call argument list.
// A package or Blade::directive handler receives its argument as a string and
// may not read it as PHP (`@markdown(Hello world)`), so for a directive that is
// not Blade's own an argument that does not parse comes back as raw, kept as
// written, instead of failing the file.
func (p *bladeParser) parseDirectiveArgs(name string) (Span, []php.Expr, []php.Token, []byte, Span) {
	lp, content, rp := p.consumeDirectiveParenContent()

	args, tokens, errs := php.ParseCallArgs(content)
	if len(errs) > 0 && rp.End.Offset > rp.Start.Offset && !isLaravelArgDirective(name) {
		return lp, nil, nil, content, rp
	}

	p.errors = append(p.errors, rehomeErrors(errs, lp.End)...)

	return lp, args, tokens, nil, rp
}

func (p *bladeParser) parseDirectiveRaw() (Span, []byte, Span) {
	lp, content, rp := p.consumeDirectiveParenContent()

	return lp, content, rp
}

// parseDirectivePhp parses inline @php(...) as one PHP expression, not a
// call argument list: Blade compiles it to `<?php (…); ?>`, where a comma
// list does not parse.
func (p *bladeParser) parseDirectivePhp() (Span, []php.Expr, []php.Token, Span) {
	lp, content, rp := p.consumeDirectiveParenContent()

	expr, tokens, errs := php.ParseFullExpression(content)
	p.errors = append(p.errors, rehomeErrors(errs, lp.End)...)

	if expr == nil {
		return lp, nil, tokens, rp
	}

	return lp, []php.Expr{expr}, tokens, rp
}

func (p *bladeParser) consumeDirectiveParenContent() (Span, []byte, Span) {
	lpStart := p.Pos
	p.Advance()
	lp := Span{Start: lpStart, End: p.Pos}

	rpStartAbs, rpEndAbs := p.findDirectiveClose(lpStart.Offset)
	contentEnd := rpStartAbs

	if contentEnd < 0 {
		contentEnd = len(p.Src)
	}

	contentOffset := p.Pos.Offset

	for p.Pos.Offset < contentEnd && !p.Eof() {
		p.Advance()
	}

	content := p.Src[contentOffset:p.Pos.Offset]
	rpStart := p.Pos

	if rpEndAbs < 0 {
		p.addError(p.unclosedDirectiveAt(content, lp.End))
	} else {
		for p.Pos.Offset < rpEndAbs && !p.Eof() {
			p.Advance()
		}
	}

	return lp, content, Span{Start: rpStart, End: p.Pos}
}

// unclosedDirectiveAt places and words the error for a directive whose `)` was never
// found. Usually that is the end of input, but when a line comment swallowed
// the `)` (`@if($a // note)`, which Blade cannot compile either: it reads
// PHP comments the same way) the comment is what the author has to fix, so the
// error points there instead of at the last line of the file.
func (p *bladeParser) unclosedDirectiveAt(content []byte, base Position) (Position, string) {
	tokens, _ := php.LexBare(content)

	for _, tok := range tokens {
		for _, tr := range tok.Leading {
			if tr.Kind == php.TriviaLineComment && bytes.IndexByte(tr.Text, ')') >= 0 {
				errs := rehomeErrors([]Error{{Pos: tr.Span.Start}}, base)

				return errs[0].Pos, errArgsHiddenByComment
			}
		}
	}

	return p.Pos, "expected ) to close directive arguments"
}

// findDirectiveClose scans PHP tokens from the opening ( to the matching ),
// returning absolute offsets (-1, -1 if none). Token-based counting stays
// correct across PHP strings, heredocs, and comments, and stopping at the match
// avoids lexing the whole document tail.
func (p *bladeParser) findDirectiveClose(lpOffset int) (rpStart, rpEnd int) {
	rpStart, rpEnd = -1, -1
	depth := 0

	php.LexBareFunc(p.Src[lpOffset:], func(tok php.Token) bool {
		switch tok.Kind {
		case php.KindLParen:
			depth++
		case php.KindRParen:
			depth--
			if depth == 0 {
				rpStart = lpOffset + tok.Span.Start.Offset
				rpEnd = lpOffset + tok.Span.End.Offset

				return false
			}
		}

		return true
	})

	return rpStart, rpEnd
}

func (p *bladeParser) parsePhpBlock(at Position) Node {
	open := Span{Start: at, End: p.Pos}
	contentOffset := p.Pos.Offset

	for !p.Eof() && !p.StartsWith("@endphp") {
		p.Advance()
	}

	body := p.Src[contentOffset:p.Pos.Offset]
	stmts, tokens, errs := php.ParseStatements(body)
	p.errors = append(p.errors, rehomeErrors(errs, open.End)...)

	closeStart := p.Pos

	if p.StartsWith("@endphp") {
		p.AdvanceN(len("@endphp"))
	} else {
		p.addError(at, "expected @endphp to close @php block")
	}

	return &PhpBlockNode{
		Open: open, Stmts: stmts, Tokens: tokens,
		Close: Span{Start: closeStart, End: p.Pos},
	}
}

func (p *bladeParser) parseVerbatimBlock(at Position) Node {
	open := Span{Start: at, End: p.Pos}
	contentOffset := p.Pos.Offset

	for !p.Eof() && !p.StartsWith("@endverbatim") {
		p.Advance()
	}

	text := p.Src[contentOffset:p.Pos.Offset]
	closeStart := p.Pos

	if p.StartsWith("@endverbatim") {
		p.AdvanceN(len("@endverbatim"))
	} else {
		p.addError(at, "expected @endverbatim to close @verbatim block")
	}

	return &VerbatimNode{
		Open: open, Text: text,
		Close: Span{Start: closeStart, End: p.Pos},
	}
}

func (p *bladeParser) parseHTMLComment() Node {
	openStart := p.Pos
	p.AdvanceN(4)
	open := Span{Start: openStart, End: p.Pos}

	contentOffset := p.Pos.Offset

	for !p.Eof() && !p.StartsWith("-->") {
		p.Advance()
	}

	text := p.Src[contentOffset:p.Pos.Offset]
	closeStart := p.Pos

	if p.StartsWith("-->") {
		p.AdvanceN(3)
	} else {
		p.addError(openStart, "unterminated HTML comment")
	}

	return &html.Comment{
		Open: open, Text: text,
		Close: Span{Start: closeStart, End: p.Pos},
	}
}

func (p *bladeParser) parseDoctype() Node {
	openStart := p.Pos
	p.AdvanceN(len("<!doctype"))
	open := Span{Start: openStart, End: p.Pos}

	contentOffset := p.Pos.Offset

	for !p.Eof() && p.Peek() != '>' {
		p.Advance()
	}

	text := p.Src[contentOffset:p.Pos.Offset]
	closeStart := p.Pos

	if p.Peek() == '>' {
		p.Advance()
	} else {
		p.addError(p.Pos, "expected > to close DOCTYPE")
	}

	return &html.Doctype{
		Open: open, Text: text,
		Close: Span{Start: closeStart, End: p.Pos},
	}
}

// parseElement parses any <tag ...>; Blade components and HTML elements share
// it. A void element is marked SelfClose even without /, unless the author wrote
// a matching </name> in scope; that parses as paired so RSS/sitemap templates
// using void names with content round-trip cleanly.
func (p *bladeParser) parseElement(stack [][]byte) Node {
	openStart := p.Pos
	openErrCount := len(p.errors)
	p.Advance()
	open := Span{Start: openStart, End: p.Pos}

	name := html.ReadTagName(&p.Cursor)

	// Laravel's component compiler wants whitespace before every attribute, so
	// a glued echo leaves a component tag uncompiled.
	var nameEcho Node
	if !IsComponentTag(name) && p.atAttrEcho() {
		nameEcho = p.parseAttrEcho()
	} else if !p.Eof() && html.GluedToTagName(p.Peek()) {
		p.addError(p.Pos, "unexpected character after tag name")
	}

	attrs := p.parseAttrs(IsComponentTag(name))

	openCloseStart := p.Pos
	selfClose := false

	switch {
	case p.Peek() == '/' && p.PeekAt(1) == '>':
		p.AdvanceN(2)
		selfClose = true
	case p.Peek() == '>':
		p.Advance()
	default:
		p.addError(p.Pos, "expected > or /> to close element tag")
	}

	entry := p.stackEntry(name, nameEcho)

	// A `<` before an echo is a tag only when its attributes parse and a close
	// with the same echo follows (stillat/blade-parser pairs these names by
	// equality too); otherwise it stays the text it always formatted as.
	if len(name) == 0 && (len(p.errors) > openErrCount || !selfClose && !p.hasDynamicCloseAhead(entry)) {
		p.Pos = open.End
		p.errors = p.errors[:openErrCount]

		return &html.Text{Start: open.Start, End: open.End, Text: p.Src[open.Start.Offset:open.End.Offset]}
	}

	el := &html.Element{
		Open: open, Name: name,
		NameEcho:    nameEcho,
		Attributes:  attrs,
		OpenClose:   Span{Start: openCloseStart, End: p.Pos},
		SelfClose:   selfClose,
		SlashClosed: selfClose,
	}

	var parentClose []byte
	if len(stack) > 0 {
		parentClose = entryName(stack[len(stack)-1])
	}

	if selfClose || (html.IsVoidElement(name) && !p.hasPairedClose(name, parentClose)) {
		el.SelfClose = true
		return el
	}

	savedPos := p.Pos
	savedErrCount := len(p.errors)

	if html.IsRawTextElement(name) || html.IsRcdataElement(name) || IsRawBodyComponent(name) {
		el.Children = p.parseRawContent(name)
	} else {
		el.Children = p.parseElementChildren(append(stack, entry))
	}

	if p.closesElement(entry) {
		closeOpenStart := p.Pos
		p.AdvanceN(2)
		el.CloseOpen = Span{Start: closeOpenStart, End: p.Pos}
		el.CloseName = html.ReadTagName(&p.Cursor)

		if p.StartsWith("{{") || p.StartsWith("{!!") {
			el.CloseNameEcho = p.parseAttrEcho()
		}

		html.SkipTagWhitespace(&p.Cursor)

		closeFinalStart := p.Pos

		if p.Peek() == '>' {
			p.Advance()
		} else {
			p.addError(p.Pos, "expected > to close element end tag")
		}

		el.CloseFinal = Span{Start: closeFinalStart, End: p.Pos}

		return el
	}

	// A tag left open at an enclosing block boundary is a deferred-open: its
	// close lives in a sibling branch or later iteration. Only valid at Blade
	// body level (empty HTML stack); nested in another element it is genuinely
	// unclosed and must fail loudly, since a close-less tag trapped inside an
	// ancestor's close tag no longer reparses. It owns no children: rewind past
	// them so the body reparses them as siblings, since nesting content under a
	// close-less tag flips its inline/block layout on reparse (non-idempotent).
	if len(stack) == 0 && p.atEnclosingBlockBoundary() {
		el.Loose = true
		el.Children = nil
		p.Pos = savedPos
		p.errors = p.errors[:savedErrCount]

		return el
	}

	// No close tag: an element with an optional end tag is implicitly closed
	// here (the formatter inserts it); anything else is genuinely unclosed and
	// fails loudly. CloseOpen still records where the body ends, so the
	// element lays out like its explicitly closed twin.
	el.CloseOpen = Span{Start: p.Pos, End: p.Pos}

	// After a PHP island running to the end of the file every byte is PHP, so
	// an inserted close tag would land in the code.
	if !html.HasOptionalEndTag(name) || endsInOpenPhp(p.Src, el.Children) {
		p.addError(openStart, "unclosed element")
	}

	return el
}

func endsInOpenPhp(src []byte, children []html.Node) bool {
	last := len(children) - 1
	for last >= 0 {
		t, ok := children[last].(*html.Text)
		if !ok || len(bytes.TrimSpace(t.Text)) > 0 {
			break
		}

		last--
	}

	if last < 0 {
		return false
	}

	island, ok := children[last].(*RawPhpNode)
	if !ok {
		return false
	}

	text := src[island.Start.Offset:island.End.Offset]
	opens := len(text) >= len("<?php") && bytes.EqualFold(text[:len("<?php")], []byte("<?php")) ||
		bytes.HasPrefix(text, []byte("<?="))

	return opens && !bytes.HasSuffix(text, []byte("?>"))
}

func (p *bladeParser) parseElementChildren(stack [][]byte) []html.Node {
	var children []html.Node

	for !p.Eof() {
		if p.atContainerBoundary(stack) {
			break
		}

		n := p.parseNext(stack)

		if n == nil {
			continue
		}

		children = append(children, n)
	}

	return children
}

// atContainerBoundary reports whether the cursor sits at something that ends
// the current element: a closer or branch of an enclosing Blade block, its
// own close tag, an open ancestor's close tag (current is implicitly
// closed), or a start tag that implicitly closes it (a peer <li>, a block
// after <p>).
func (p *bladeParser) atContainerBoundary(stack [][]byte) bool {
	if p.atEnclosingBlockBoundary() {
		return true
	}

	if len(stack) == 0 {
		return false
	}

	current := stack[len(stack)-1]

	if p.closesElement(current) {
		return true
	}

	if html.IsTagClose(&p.Cursor) || p.atDynamicTagClose() {
		if slices.ContainsFunc(stack[:len(stack)-1], p.closesElement) {
			return true
		}
	}

	if p.isTagOpen() && html.StartTagClosesCurrent(entryName(current), p.peekTagName()) {
		return true
	}

	return false
}

func (p *bladeParser) atEnclosingBlockBoundary() bool {
	for _, opener := range p.blockStack {
		if p.atOneOfDirectives(p.dirs.closers[opener]) {
			return true
		}

		if p.matchBranchForOpener(opener) {
			return true
		}
	}

	return false
}

func (p *bladeParser) peekTagName() []byte {
	return p.tagNameAt(p.Pos.Offset + 1)
}

func (p *bladeParser) tagNameAt(start int) []byte {
	off := start

	for off < len(p.Src) {
		c := p.Src[off]

		if html.IsNameStart(c) || html.IsDigit(c) || c == '-' || c == '_' || c == '.' || c == ':' {
			off++
			continue
		}

		break
	}

	return p.Src[start:off]
}

func (p *bladeParser) parseRawContent(name []byte) []html.Node {
	start := p.Pos

	var islands []Span

	for !p.Eof() && !p.matchesCloseTag(name) {
		if island, ok := p.skipRawBladeConstruct(); ok {
			if island.End.Offset > island.Start.Offset {
				islands = append(islands, island)
			}

			continue
		}

		p.Advance()
	}

	if p.Pos.Offset == start.Offset {
		return nil
	}

	return []html.Node{&html.Text{
		Start: start, End: p.Pos,
		Text:        p.Src[start.Offset:p.Pos.Offset],
		PHP:         islands,
		PHPLiterals: p.phpLiterals(islands),
	}}
}

func (p *bladeParser) phpLiterals(islands []Span) []Span {
	var literals []Span

	for _, island := range islands {
		code := p.Src[island.Start.Offset:island.End.Offset]
		if !bytes.ContainsAny(code, "\n\r") {
			continue
		}

		tokens, errs := php.LexBare(code)
		if len(errs) > 0 {
			literals = append(literals, island)

			continue
		}

		for _, tok := range tokens {
			if isDataToken(tok.Kind) && bytes.ContainsAny(tok.Text, "\n\r") {
				start := rebasePosition(island.Start, tok.Span.Start)
				end := rebasePosition(island.Start, tok.Span.End)
				literals = append(literals, Span{Start: start, End: end})
			}
		}
	}

	return literals
}

func isDataToken(kind php.Kind) bool {
	return kind == php.KindString || kind == php.KindShellExec || kind == php.KindInlineHTML
}

func rebasePosition(origin, rel Position) Position {
	pos := Position{Offset: origin.Offset + rel.Offset, Line: origin.Line + rel.Line - 1, Column: rel.Column}

	if rel.Line == 1 {
		pos.Column = origin.Column + rel.Column - 1
	}

	return pos
}

// skipRawBladeConstruct consumes the Blade construct at the cursor inside a raw
// body and returns the span Blade compiles into PHP, empty for one that compiles
// to none (a comment, an escaped echo, `@verbatim`). It reports false, consuming
// nothing, anywhere else. The checks follow BladeCompiler's order: raw blocks,
// then comments, then echoes, then statements.
func (p *bladeParser) skipRawBladeConstruct() (Span, bool) {
	start := p.Pos

	switch {
	case p.Peek() == '@' && p.atAtMarker() && p.namesDirective("verbatim") && p.hasCloseAhead("@endverbatim"):
		p.skipRegion("@verbatim", "@endverbatim")

		return Span{Start: start, End: start}, true
	case p.Peek() == '@' && p.atAtMarker() && p.atPhpBlock():
		p.skipRegion("@php", "@endphp")
	case p.StartsWith("{{--") && p.hasCloseAhead("--}}"):
		p.skipRegion("{{--", "--}}")

		return Span{Start: start, End: start}, true
	case p.StartsWith("@{{") && p.hasCloseAhead("}}"):
		p.skipRegion("@{{", "}}")

		return Span{Start: start, End: start}, true
	case p.StartsWith("@{!!") && p.hasCloseAhead("!!}"):
		p.skipRegion("@{!!", "!!}")

		return Span{Start: start, End: start}, true
	case p.StartsWith("{{{") && p.hasCloseAhead("}}}"):
		p.skipRegion("{{{", "}}}")
		p.takeSwallowedCR()
	case p.StartsWith("{{") && p.hasCloseAhead("}}"):
		p.skipRegion("{{", "}}")
		p.takeSwallowedCR()
	case p.StartsWith("{!!") && p.hasCloseAhead("!!}"):
		p.skipRegion("{!!", "!!}")
		p.takeSwallowedCR()
	case p.StartsWith("<?") && p.skipRawPhp():
	case p.StartsWith("@@"):
		p.AdvanceN(2)

		return Span{Start: start, End: start}, true
	case p.Peek() == '@' && p.atAtMarker() && html.IsNameStart(p.PeekAt(1)):
		if !p.skipRawDirective() {
			return Span{Start: start, End: start}, true
		}
	default:
		return Span{}, false
	}

	return Span{Start: start, End: p.Pos}, true
}

// skipRawDirective consumes a `@name` and, the way parseDirective reads one, the
// argument list that follows it on the same line. It reports whether there was
// an argument list, which is the part Blade compiles into PHP.
func (p *bladeParser) skipRawDirective() bool {
	p.Advance()
	p.readDirectiveName()

	if !p.directiveArgsFollow() {
		return false
	}

	for p.Peek() == ' ' || p.Peek() == '\t' {
		p.Advance()
	}

	_, rpEnd := p.findDirectiveClose(p.Pos.Offset)
	if rpEnd < 0 {
		return false
	}

	for p.Pos.Offset < rpEnd && !p.Eof() {
		p.Advance()
	}

	return true
}

func (p *bladeParser) parseStrayClose(stack [][]byte) Node {
	start := p.Pos

	// A close tag with no open element in scope is a deferred close: its open
	// lives in a sibling branch, an earlier iteration, or the including
	// template. That is valid Blade, so it goes out verbatim; inside an open
	// element it is a genuine mismatch and fails loudly.
	if len(stack) > 0 {
		p.addError(start, "unexpected closing tag")
	}

	for !p.Eof() && p.Peek() != '>' {
		p.Advance()
	}

	if p.Peek() == '>' {
		p.Advance()
	}

	return &html.Text{
		Start: start, End: p.Pos,
		Text: p.Src[start.Offset:p.Pos.Offset],
	}
}

// matchesCloseTag reports whether the cursor begins </name> (case-insensitive,
// optional whitespace before >). A Blade slot <x-slot:title> also accepts the
// short close </x-slot>, per Laravel's slot syntax.
func (p *bladeParser) matchesCloseTag(name []byte) bool {
	return p.matchesCloseTagAt(p.Pos.Offset, name)
}

func (p *bladeParser) closesElement(entry []byte) bool {
	name := entryName(entry)
	if len(name) > 0 && p.matchesCloseTag(name) || p.closesDynamicName(name, entry) {
		return true
	}

	family := componentFamily(name)
	if family == "" || !html.IsTagClose(&p.Cursor) {
		return false
	}

	closeName := p.tagNameAt(p.Pos.Offset + 2)

	return componentFamily(closeName) == family && p.matchesCloseTagName(p.Pos.Offset, closeName)
}

// stackEntry keys an open element for its closers. A glued echo may be an
// attribute bag (`<div{!! $a !!}>…</div>`) or part of the name
// (`<h{{ $l }}>…</h{{ $l }}>`), and only the close tells which, so the entry
// keeps the echo's opener and PHP tokens; spacing inside it does not count.
func (p *bladeParser) stackEntry(name []byte, nameEcho Node) []byte {
	var open Span

	var tokens []php.Token

	switch echo := nameEcho.(type) {
	case *EchoNode:
		open, tokens = echo.Open, echo.Tokens
	case *RawEchoNode:
		open, tokens = echo.Open, echo.Tokens
	default:
		return name
	}

	entry := append(bytes.ToLower(name), p.Src[open.Start.Offset:open.End.Offset]...)

	for _, tok := range tokens {
		entry = append(entry, ' ')
		entry = append(entry, tok.Text...)
	}

	return entry
}

// A tag name never holds a `{`, so the first one ends an entry's name.
func entryName(entry []byte) []byte {
	name, _, _ := bytes.Cut(entry, []byte("{"))

	return name
}

func (p *bladeParser) hasDynamicCloseAhead(entry []byte) bool {
	save := p.Pos
	defer func() { p.Pos = save }()

	name := entryName(entry)

	for {
		i := bytes.Index(p.Src[p.Pos.Offset:], []byte("</{"))
		if i < 0 {
			return false
		}

		p.AdvanceN(i)

		if p.closesDynamicName(name, entry) {
			return true
		}

		p.AdvanceN(len("</{"))
	}
}

// Neither the cursor nor errors from parsing the close's echo survive the call.
func (p *bladeParser) closesDynamicName(name, entry []byte) bool {
	if len(name) == len(entry) || !p.StartsWith("</") {
		return false
	}

	save, errCount := p.Pos, len(p.errors)

	defer func() {
		p.Pos = save
		p.errors = p.errors[:errCount]
	}()

	p.AdvanceN(len("</"))
	closeName := html.ReadTagName(&p.Cursor)

	if !p.StartsWith("{{") && !p.StartsWith("{!!") {
		return false
	}

	closeEntry := p.stackEntry(closeName, p.parseAttrEcho())
	html.SkipTagWhitespace(&p.Cursor)

	return p.Peek() == '>' && bytes.Equal(closeEntry, entry)
}

func (p *bladeParser) matchesCloseTagAt(offset int, name []byte) bool {
	if p.matchesCloseTagName(offset, name) {
		return true
	}

	if before, _, ok := bytes.Cut(name, []byte{':'}); ok {
		return p.matchesCloseTagName(offset, before)
	}

	return false
}

func (p *bladeParser) matchesCloseTagName(offset int, name []byte) bool {
	if offset+1 >= len(p.Src) || p.Src[offset] != '<' || p.Src[offset+1] != '/' {
		return false
	}

	off := offset + 2
	end := off + len(name)

	if end > len(p.Src) {
		return false
	}

	if !bytes.EqualFold(p.Src[off:end], name) {
		return false
	}

	for end < len(p.Src) && html.IsSpaceByte(p.Src[end]) {
		end++
	}

	return end < len(p.Src) && p.Src[end] == '>'
}

// hasPairedClose reports whether a </name> close tag exists in scope ahead of
// the cursor (scope ends at </parentClose> or EOF). The check is shallow and
// ignores nested <name> opens, because a document never mixes void and
// paired uses of one HTML5 void name. It searches the lazy closeIndex, so a
// void-heavy document is not rescanned per element.
func (p *bladeParser) hasPairedClose(name, parentClose []byte) bool {
	if p.closeIndex == nil {
		p.closeIndex = html.CloseTagIndex(p.Src)
	}

	off := p.Pos.Offset

	nameOff, ok := p.firstCloseInScope(name, off)
	if !ok {
		return false
	}

	if parentClose == nil {
		return true
	}

	parentOff, ok := p.firstCloseInScope(parentClose, off)
	if !ok {
		return true
	}

	return nameOff <= parentOff
}

func (p *bladeParser) firstCloseInScope(name []byte, off int) (int, bool) {
	best, ok := html.FirstCloseAtOrAfter(p.closeIndex[string(bytes.ToLower(name))], off)

	if before, _, cut := bytes.Cut(name, []byte{':'}); cut {
		if o, found := html.FirstCloseAtOrAfter(p.closeIndex[string(bytes.ToLower(before))], off); found && (!ok || o < best) {
			best, ok = o, true
		}
	}

	return best, ok
}

func (p *bladeParser) parseAttrs(component bool) []*html.Attr {
	var attrs []*html.Attr

	for !p.Eof() {
		html.SkipTagWhitespace(&p.Cursor)

		c := p.Peek()

		if c == '/' || c == '>' || c == 0 {
			break
		}

		a := p.parseAttr(component)

		// Nothing here reads as an attribute name: after skipTagWhitespace and
		// the checks above, the only byte AttrNameByte rejects is a stray `=`.
		// Skipping it would discard source bytes.
		if a == nil {
			p.addError(p.Pos, "expected an attribute name")

			return attrs
		}

		attrs = append(attrs, a)
	}

	return attrs
}

func (p *bladeParser) parseAttr(component bool) *html.Attr {
	start := p.Pos

	// A Blade echo or comment in attribute position (e.g. the attribute-bag
	// idiom <div {{ $attributes->merge([...]) }}>) is parsed whole and carried
	// on Attr.Directive, the same channel as @class([...]), so the attribute
	// scanner can't shred it into bogus names.
	//
	// An echo the author followed with an `=` is the other construct: the name
	// itself is composed with an echo (`{{ $name }}="v"`, which Blade compiles to
	// `<?php echo e($name); ?>="v"`), so it takes the ordinary name path below.
	if p.atAttrEcho() {
		node := p.parseAttrEcho()

		return &html.Attr{Start: start, End: p.Pos, Directive: node}
	}

	// A raw `<?php … ?>` (or `<?= … ?>`) tag in attribute position is a Blade
	// conditional-attribute construct. Capture it whole to its `?>`, so the attribute
	// scanner does not read the PHP keywords as bogus names and `?>`'s `>` does not
	// close the tag early.
	if p.StartsWith("<?") {
		p.skipRawPhpOrRest()

		return &html.Attr{Start: start, End: p.Pos, RawPhp: p.Src[start.Offset:p.Pos.Offset]}
	}

	if p.Peek() == '@' && html.IsNameStart(p.PeekAt(1)) {
		if p.atAlpineEventAttr() {
			return p.parseAlpineEventAttr(start)
		}

		return p.parseDirectiveAttr(start)
	}

	bound := false
	shorthand := false

	if p.Peek() == ':' {
		p.Advance()
		bound = true

		if p.Peek() == '$' {
			p.Advance()
			shorthand = true
		}
	}

	name := p.readAttrName()

	if len(name) == 0 {
		p.Pos = start
		return nil
	}

	attr := &html.Attr{
		Start: start,
		Bound: bound, Shorthand: shorthand,
		Name: name,
	}

	if shorthand {
		attr.End = p.Pos
		return attr
	}

	if html.SkipToAttrEquals(&p.Cursor) {
		eqStart := p.Pos
		p.Advance()
		attr.Eq = Span{Start: eqStart, End: p.Pos}
		p.parseAttrValue(attr, component)
	}

	attr.End = p.Pos

	return attr
}

// skipToAttrValue consumes whitespace between an `=` and its value, per HTML5's
// before-attribute-value state, so `href = "x"` is one attribute rather than a
// valueless `href` plus a second attribute made from the value.
//
// It stops short when the run that follows carries a quote character, because
// that run is the head of the next attribute's quoted value (`@if= data-x="a
// b"`): consuming it would swallow that attribute and orphan its closing quote.
// skipToAttrEquals consumes whitespace between an attribute name and an `=`,
// per HTML5's after-attribute-name state, and reports whether one follows.
// Nothing is consumed when it does not, so `<a href class=y>` still reads as two
// attributes rather than swallowing the second as the first one's value.
func (p *bladeParser) parseDirectiveAttr(start Position) *html.Attr {
	p.Advance()
	name := p.readDirectiveName()
	dir := p.parseDirective(start, name)

	return &html.Attr{
		Start:     start,
		End:       p.Pos,
		Directive: dir,
	}
}

// atAlpineEventAttr reports whether the @-token is an Alpine event binding (the
// @event shorthand for x-on:event) rather than a Blade attribute directive:
// Alpine bindings have a .modifier chain or = value right after the name, while
// Blade directives take parens or stand bare. The cursor is not moved.
func (p *bladeParser) atAlpineEventAttr() bool {
	off := p.Pos.Offset + 1
	dashed := false

	for off < len(p.Src) {
		c := p.Src[off]

		if html.IsNameStart(c) || html.IsDigit(c) || c == '-' || c == '_' {
			dashed = dashed || c == '-'
			off++

			continue
		}

		break
	}

	// A dash rules the directive reading out: no Blade directive name carries
	// one, while a custom Alpine event may, so a valueless `@my-event` is not
	// the directive `@my` plus an attribute `-event`.
	if dashed {
		return true
	}

	if off >= len(p.Src) {
		return false
	}

	// So does any other byte an attribute name may carry but a directive name
	// may not, `(` excepted since it opens the directive's arguments: `@a0"`
	// is one attribute, not the directive `@a0` plus an attribute `"`.
	if html.AttrNameByte(p.Src[off]) && p.Src[off] != '(' {
		return true
	}

	return p.Src[off] == '.' || p.Src[off] == '='
}

// parseAlpineEventAttr consumes an Alpine @event binding (@click,
// @keydown.enter, @submit.prevent="save()"). The @, name, and .modifier chain
// become the attribute name and the value is emitted verbatim, never through
// the PHP grammar (embedded Blade echoes still decompose via Attr.Parts).
func (p *bladeParser) parseAlpineEventAttr(start Position) *html.Attr {
	p.Advance()
	p.readAttrName()

	attr := &html.Attr{
		Start: start,
		Name:  p.Src[start.Offset:p.Pos.Offset],
	}

	if html.SkipToAttrEquals(&p.Cursor) {
		eqStart := p.Pos
		p.Advance()
		attr.Eq = Span{Start: eqStart, End: p.Pos}
		p.parseAttrValue(attr, false)
	}

	attr.End = p.Pos

	return attr
}

func (p *bladeParser) readAttrName() []byte {
	startOff := p.Pos.Offset

	for !p.Eof() {
		if p.skipBladeRegion() {
			continue
		}

		c := p.Peek()

		if !html.AttrNameByte(c) {
			break
		}

		p.Advance()
	}

	return p.Src[startOff:p.Pos.Offset]
}

func (p *bladeParser) atAttrEcho() bool {
	return (p.StartsWith("{{") || p.StartsWith("{!!")) && !p.echoOpensAttrName()
}

func (p *bladeParser) parseAttrEcho() Node {
	switch {
	case p.StartsWith("{{--"):
		return p.parseComment()
	case p.StartsWith("{!!"):
		return p.parseRawEcho()
	default:
		return p.parseEcho()
	}
}

// echoOpensAttrName reports whether the echo at the cursor opens an attribute
// name rather than standing alone as the attribute-bag idiom: the author wrote an
// `=` after it. `{{ $attributes->merge([…]) }}` holds a `=>` of its own, which is
// why the region is skipped whole before looking. The cursor is not moved.
func (p *bladeParser) echoOpensAttrName() bool {
	save := p.Pos
	defer func() { p.Pos = save }()

	if p.StartsWith("{{--") {
		return false
	}

	// An opener with no close ahead is malformed, not a name; the echo branch
	// in parseAttr then rejects it loudly.
	if !p.skipBladeRegion() {
		return false
	}

	p.readAttrName()

	return html.SkipToAttrEquals(&p.Cursor)
}

// skipBladeRegion consumes a Blade echo, raw echo, comment or escaped echo when
// one starts at the cursor, and reports whether it did. An unterminated opener is
// not a region: it stays literal, as it does in an attribute value.
//
// The name scanner needs this because a region can hold whitespace, which
// ends an attribute name for an HTML tokenizer: without it,
// `wire:model{{ $live ? '.live' : null }}="name"` would split into several
// attributes and lose the binding once the tag wrapped.
func (p *bladeParser) skipBladeRegion() bool {
	switch {
	case p.StartsWith("@{{--") && p.hasCloseAhead("--}}"):
		p.skipRegion("@{{--", "--}}")
	case p.StartsWith("@{{") && p.hasCloseAhead("}}"):
		p.skipRegion("@{{", "}}")
	case p.StartsWith("@{!!") && p.hasCloseAhead("!!}"):
		p.skipRegion("@{!!", "!!}")
	case p.StartsWith("{{--") && p.hasCloseAhead("--}}"):
		p.skipRegion("{{--", "--}}")
	case p.StartsWith("{{") && p.hasCloseAhead("}}"):
		p.skipRegion("{{", "}}")
	case p.StartsWith("{!!") && p.hasCloseAhead("!!}"):
		p.skipRegion("{!!", "!!}")
	default:
		return false
	}

	return true
}

func (p *bladeParser) parseAttrValue(attr *html.Attr, component bool) {
	html.SkipToAttrValue(&p.Cursor)

	c := p.Peek()

	if c != '"' && c != '\'' {
		contentOffset := p.Pos.Offset

		// HTML5 ends an unquoted value at whitespace, but a Blade echo, escape
		// or directive argument is one unit that may hold whitespace and `>`,
		// so a byte scan would cut `src={{ $url }}` at the space in the echo.
		attr.Parts, attr.KeepUnquoted = p.parseAttrParts(0)
		attr.Value = p.Src[contentOffset:p.Pos.Offset]
		attr.ValueStart = contentOffset

		if attr.Bound && component && attr.Parts == nil {
			if expr, tokens, errs := php.ParseFullExpression(attr.Value); len(errs) == 0 {
				attr.Expr, attr.ExprTokens = expr, tokens
			}
		}

		return
	}

	attr.Quote = c
	p.Advance()

	contentOffset := p.Pos.Offset

	if attr.Bound && component {
		// Blade compiles a bound (:name) value only on a component tag; on any
		// other element it is Alpine's x-bind shorthand, JavaScript that may
		// happen to parse as PHP (`:hidden="TRUE"`), so it stays verbatim.
		//
		// A component's value is PHP only when it has no embedded echoes and
		// parses cleanly. Echoes present → keep the Attr.Parts decomposition;
		// echo-free PHP → route through the grammar; echo-free non-PHP → verbatim.
		//
		// PHP has to consume the whole value, or Alpine JS whose first token
		// happens to parse as PHP would be cut down to that token.
		attr.Parts, attr.KeepQuote = p.parseAttrParts(c)
		attr.Value = p.Src[contentOffset:p.Pos.Offset]
		attr.ValueStart = contentOffset

		if attr.Parts == nil {
			if expr, tokens, errs := php.ParseFullExpression(attr.Value); len(errs) == 0 {
				attr.Expr, attr.ExprTokens = expr, tokens
			}
		}
	} else {
		// Plain and wire:* values emit verbatim (embedded echoes still
		// decompose). wire:* is a Livewire DSL, not PHP: reflowing it would
		// rewrite a wire:model dot-path like form.name into a `.` concatenation
		// and break the bind.
		attr.Parts, attr.KeepQuote = p.parseAttrParts(c)
		attr.Value = p.Src[contentOffset:p.Pos.Offset]
		attr.ValueStart = contentOffset
	}

	if p.Peek() == c {
		p.Advance()
	} else {
		p.addError(p.Pos, "expected matching quote to close attribute value")
	}
}

// skipRegion consumes open, the text up to closer, and closer as one unit,
// so nothing inside the region, such as a quote in an attribute value, is
// read as structure.
func (p *bladeParser) skipRegion(open, closer string) {
	p.AdvanceN(len(open))

	for !p.Eof() && !p.StartsWith(closer) {
		p.Advance()
	}

	p.AdvanceN(len(closer))
}

// parseAttrParts returns a mixed slice of html.Text and echo nodes when the
// value has embedded {{ }}/{!! !!} echoes, or nil when it's plain text (the
// caller falls back to the opaque Value). An echo region is opaque: a quote
// inside it (`wire:click="f({{ $x ? "'a'" : 'b' }})"`) does not end the
// attribute, so the real closing quote is only one outside an echo. A {{ with
// no close marker ahead stays literal.
//
// endQuote 0 means the value was left unquoted, where whitespace and `>` end it.
// A Blade echo, an escaped echo, a comment and a `@name(…)` argument list are each
// one unit that may hold both, so they are consumed whole. keep then reports
// whether the value has to stay unquoted, per html.Attr.KeepUnquoted; for a
// quoted value it reports html.Attr.KeepQuote.
func (p *bladeParser) parseAttrParts(endQuote byte) (parts []html.Node, keep bool) {
	literalStart := p.Pos
	literalOffset := literalStart.Offset
	hasEcho := false

	flush := func() {
		if p.Pos.Offset > literalOffset {
			parts = append(parts, &html.Text{
				Start: literalStart, End: p.Pos,
				Text: p.Src[literalOffset:p.Pos.Offset],
			})
		}
	}

	// Literal bytes reach the browser as they are, so whitespace or a `>` among
	// them has already ended an unquoted value for an HTML tokenizer.
	literal := func(from int) {
		if endQuote == 0 && !keep && bytes.ContainsAny(p.Src[from:p.Pos.Offset], attrValueBreakers) {
			keep = true
		}
	}

	for !p.atAttrValueEnd(endQuote) {
		from := p.Pos.Offset

		switch {
		case p.StartsWith("@{{") && !p.StartsWith("@{{--") && p.hasCloseAhead("}}"):
			p.skipRegion("@{{", "}}")
			literal(from)
		case p.StartsWith("@{!!") && p.hasCloseAhead("!!}"):
			p.skipRegion("@{!!", "!!}")
			literal(from)
		case endQuote == 0 && p.StartsWith("{{--") && p.hasCloseAhead("--}}"):
			p.skipRegion("{{--", "--}}")
			keep = true
		case endQuote == 0 && p.atUnquotedDirectiveArgs():
			p.skipDirectiveArgs()

			// A directive expands to unescaped markup (@class([…]) becomes a whole
			// `class="…"` attribute), so quoting the value around it would break the
			// expansion out of its own delimiters.
			keep = true
		case endQuote == 0 && p.directiveParenAt(false) >= 0:
			p.addError(p.Pos, p.unclosedArgsError(p.directiveParenAt(false)))
			p.Advance()
			literal(from)
		case p.StartsWith("{{") && !p.StartsWith("{{--") && p.hasCloseAhead("}}"):
			flush()
			parts = append(parts, p.parseEcho())
			hasEcho = true
			literalStart = p.Pos
			literalOffset = p.Pos.Offset
		case p.StartsWith("{!!") && p.hasCloseAhead("!!}"):
			flush()
			parts = append(parts, p.parseRawEcho())
			hasEcho = true
			literalStart = p.Pos
			literalOffset = p.Pos.Offset

			// A raw echo prints unescaped, so its runtime value can hold a quote.
			keep = true
		case endQuote != 0 && p.atQuotedValueOutput():
			// Blade compiles a directive and PHP runs a tag before any HTML is read,
			// so a quote inside either does not end the value: each is one unit.
			switch {
			case p.atQuotedDirectiveArgs():
				p.skipDirectiveArgs()
			case p.directiveParenAt(true) >= 0:
				p.addError(p.Pos, p.unclosedArgsError(p.directiveParenAt(true)))
				p.Advance()
			case p.atPHPOpenTag() && p.hasCloseAhead("?>"):
				p.skipRegion("<?", "?>")
			default:
				p.Advance()
			}

			keep = true
		case endQuote == 0 && p.atPHPOpenTag():
			// PHP runs the tag before any HTML is read, so the `>` of its `?>` does
			// not end the value, and quotes put around it would land inside the PHP.
			p.skipRawPhpOrRest()

			keep = true
		default:
			p.Advance()
			literal(from)
		}
	}

	if !hasEcho {
		return nil, keep
	}

	flush()

	return parts, keep
}

// attrValueBreakers are the bytes that end an unquoted attribute value, or the
// tag, in HTML5's attribute-value (unquoted) state. `"`, `'`, `<`, `=` and a
// backtick are parse errors there but are appended to the value, so they do not
// belong here.
const attrValueBreakers = " \t\n\r\f>"

const (
	errArgsHiddenByComment = "a line comment swallows the directive's ), which Blade reads the same way"
	errArgsNeverClose      = "the directive's ( is never closed"
)

// rawPhpCloseAt returns the offset of the `?>` that ends the raw PHP opened by
// the `<?` at the cursor, or -1 when PHP runs to the end of the source.
func (p *bladeParser) rawPhpCloseAt() int {
	body := p.Pos.Offset + len("<?")

	at := php.CloseTagOffset(p.Src[body:])
	if at < 0 {
		return -1
	}

	return body + at
}

// skipRawPhp moves the cursor past the raw PHP opened by the `<?` at it and
// reports true, or leaves the cursor and reports false when PHP runs to the end
// of the source.
func (p *bladeParser) skipRawPhp() bool {
	closeAt := p.rawPhpCloseAt()
	if closeAt < 0 {
		return false
	}

	for p.Pos.Offset < closeAt {
		p.Advance()
	}

	p.AdvanceN(len("?>"))

	return true
}

func (p *bladeParser) skipRawPhpOrRest() {
	if p.skipRawPhp() {
		return
	}

	for !p.Eof() {
		p.Advance()
	}
}

func (p *bladeParser) hasCloseAhead(closer string) bool {
	return bytes.Contains(p.Src[p.Pos.Offset:], []byte(closer))
}

// atAttrValueEnd reports whether the cursor sits at the end of an attribute
// value. A quoted value ends at its quote; endQuote 0 means the value was
// left unquoted, and html.EndsUnquotedValue decides where that ends.
func (p *bladeParser) atAttrValueEnd(endQuote byte) bool {
	if p.Eof() {
		return true
	}

	if endQuote != 0 {
		return p.Peek() == endQuote
	}

	return html.EndsUnquotedValue(p.Src, p.Pos.Offset)
}

// atQuotedValueOutput reports whether the cursor starts output inside a quoted
// value that can print either quote: a `<?` tag, or a `@name` Blade compiles as
// a directive, which it does only when no word character comes before the `@`.
func (p *bladeParser) atQuotedValueOutput() bool {
	if p.atPHPOpenTag() {
		return true
	}

	if p.Peek() != '@' || !html.IsNameStart(p.PeekAt(1)) {
		return false
	}

	off := p.Pos.Offset

	return off == 0 || !html.IsNameStart(p.Src[off-1]) && !html.IsDigit(p.Src[off-1])
}

// atPHPOpenTag reports whether the cursor is at a tag PHP opens: `<?=`, or
// `<?php` followed by whitespace. A bare `<?` is a short tag, off by default.
func (p *bladeParser) atPHPOpenTag() bool {
	if p.StartsWith("<?=") {
		return true
	}

	return p.StartsWith("<?php") && (p.Pos.Offset+5 >= len(p.Src) || html.IsSpaceByte(p.Src[p.Pos.Offset+5]))
}

func (p *bladeParser) atQuotedDirectiveArgs() bool {
	return p.hasDirectiveClose(p.directiveParenAt(true))
}

// atUnquotedDirectiveArgs reports whether the cursor is at a `@name(` whose
// argument list has a matching `)`. Only the argument list needs skipping: a bare
// `@name` ends on the same bytes an unquoted value does.
func (p *bladeParser) atUnquotedDirectiveArgs() bool {
	return p.hasDirectiveClose(p.directiveParenAt(false))
}

// spaced allows the blanks Blade's pattern takes before the `(`, which an
// unquoted value cannot hold; -1 means no `(`.
func (p *bladeParser) directiveParenAt(spaced bool) int {
	if p.Peek() != '@' || !html.IsNameStart(p.PeekAt(1)) {
		return -1
	}

	off := p.Pos.Offset + 1
	for off < len(p.Src) && (html.IsNameStart(p.Src[off]) || html.IsDigit(p.Src[off])) {
		off++
	}

	for spaced && off < len(p.Src) && (p.Src[off] == ' ' || p.Src[off] == '\t') {
		off++
	}

	if off >= len(p.Src) || p.Src[off] != '(' {
		return -1
	}

	return off
}

func (p *bladeParser) hasDirectiveClose(lp int) bool {
	if lp < 0 {
		return false
	}

	_, rpEnd := p.findDirectiveClose(lp)

	return rpEnd >= 0
}

// Blade's compiler checks the list with PHP's tokenizer too, so where an
// unclosed list would end moves with the bytes after it, and the formatter
// rewrites those.
func (p *bladeParser) unclosedArgsError(lp int) string {
	if p.argsHiddenByComment(lp) {
		return errArgsHiddenByComment
	}

	return errArgsNeverClose
}

func (p *bladeParser) argsHiddenByComment(lp int) bool {
	depth := 0
	hidden := false

	php.LexBareFunc(p.Src[lp:], func(tok php.Token) bool {
		for _, t := range tok.Leading {
			if t.Kind == php.TriviaLineComment {
				hidden = true

				return false
			}
		}

		switch tok.Kind {
		case php.KindLParen:
			depth++
		case php.KindRParen:
			depth--
		}

		return depth > 0
	})

	return hidden
}

// skipDirectiveArgs consumes a `@name(…)` whole, so a `=>` or a space inside the
// argument list cannot end the attribute value or the start tag.
func (p *bladeParser) skipDirectiveArgs() {
	p.Advance()
	p.readDirectiveName()

	_, rpEnd := p.findDirectiveClose(p.Pos.Offset)

	for p.Pos.Offset < rpEnd && !p.Eof() {
		p.Advance()
	}
}
