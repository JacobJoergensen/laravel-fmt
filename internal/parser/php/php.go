package php

// Parse lexes and parses src into a File. File is always non-nil; a non-empty
// error slice means malformed input whose partial tree must not be re-emitted.
func Parse(src []byte) (*File, []Error) {
	tokens, errs := Lex(src)

	p := &parser{
		tokens:  tokens,
		errors:  errs,
		fileEOF: true,
	}

	stmts := p.parseFile()

	return &File{
		Src:    src,
		Tokens: tokens,
		Stmts:  stmts,
	}, p.errors
}

// ParseFrontmatter parses the PHP block that opens src, through its first `?>`,
// and returns where that tag ends. closed is false when src never leaves PHP,
// and then the File covers all of src. Lexing stops at the tag, so the rest of
// src is never scanned.
func ParseFrontmatter(src []byte) (file *File, end Position, closed bool, errs []Error) {
	tokens, errs, closed := lexFrontmatter(src)
	end = tokens[len(tokens)-1].Span.End

	p := &parser{tokens: tokens, errors: errs, fileEOF: true}
	stmts := p.parseFile()

	return &File{Src: src[:end.Offset], Tokens: tokens, Stmts: stmts}, end, closed, p.errors
}

// ParseFullExpression parses src as a single bare PHP expression, without the
// `<?php` tag, and reports anything after it as an error. The tokens are
// returned too, so a host that embeds the fragment can hand them to the
// formatter instead of lexing again.
func ParseFullExpression(src []byte) (Expr, []Token, []Error) {
	tokens, errs := LexBare(src)

	p := &parser{tokens: tokens, errors: errs}
	expr := p.parseExpression(precNone)

	if tok := p.peek(); tok.Kind != KindEOF {
		p.addError(tok.Span.Start, "unexpected token after expression; expected a single expression")
	}

	return expr, tokens, p.errors
}

// ParseStatements parses src as a sequence of bare PHP statements, without the
// `<?php` tag.
func ParseStatements(src []byte) ([]Stmt, []Token, []Error) {
	tokens, errs := LexBare(src)
	p := &parser{tokens: tokens, errors: errs}

	var stmts []Stmt

	for p.peekKind() != KindEOF {
		stmts = append(stmts, p.parseStmt())
	}

	return stmts, tokens, p.errors
}

// ParseCallArgs parses src as a comma-separated PHP call argument list, without
// surrounding parentheses. It requires the list to consume all of src: trailing
// content after the arguments is reported as an error rather than dropped, so a
// caller (a Blade directive condition) fails loudly on malformed input instead of
// formatting a silently truncated parse.
func ParseCallArgs(src []byte) ([]Expr, []Token, []Error) {
	tokens, errs := LexBare(src)

	p := &parser{tokens: tokens, errors: errs}
	args := p.parseCallArgs(KindEOF)

	if tok := p.peek(); tok.Kind != KindEOF {
		p.addError(tok.Span.Start, "unexpected token after arguments")
	}

	return args, tokens, p.errors
}
