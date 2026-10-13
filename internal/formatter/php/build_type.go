package php

import (
	"bytes"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/document"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

// typeHint collapses a top-level `T|null` or `null|T` union to `?T`.
func (b *builder) typeHint(t parser.TypeHint) *document.Doc {
	if t == nil {
		return document.Text("")
	}

	if nonNull, ok := extractSimpleNullable(t); ok {
		return document.Concat(document.Text("?"), b.typeHintRaw(nonNull))
	}

	return b.typeHintRaw(t)
}

func (b *builder) typeHintRaw(t parser.TypeHint) *document.Doc {
	switch n := t.(type) {
	case nil:
		return document.Text("")
	case *parser.NamedType:
		if lower, ok := reservedTypeLower(n.Name); ok {
			return document.Text(string(lower))
		}

		return b.name(n.Name)
	case *parser.KeywordType:
		return document.Text(strings.ToLower(string(n.Token.Text)))
	case *parser.NullableType:
		return document.Concat(document.Text("?"), b.typeHintRaw(n.Type))
	case *parser.UnionType:
		return b.joinTypes(n.Types, "|")
	case *parser.IntersectionType:
		return b.joinTypes(n.Types, "&")
	case *parser.ParenType:
		return document.Concat(document.Text("("), b.typeHintRaw(n.Type), document.Text(")"))
	}

	return b.verbatim(t.Span())
}

func (b *builder) joinTypes(types []parser.TypeHint, sep string) *document.Doc {
	docs := make([]*document.Doc, len(types))
	for i, t := range types {
		docs[i] = b.typeHintRaw(t)
	}

	return document.Join(document.Text(sep), docs)
}

func extractSimpleNullable(t parser.TypeHint) (parser.TypeHint, bool) {
	var nonNull parser.TypeHint

	switch typ := t.(type) {
	case *parser.NullableType:
		nonNull = typ.Type
	case *parser.UnionType:
		if len(typ.Types) != 2 {
			return nil, false
		}

		switch {
		case isNullType(typ.Types[0]):
			nonNull = typ.Types[1]
		case isNullType(typ.Types[1]):
			nonNull = typ.Types[0]
		default:
			return nil, false
		}
	default:
		return nil, false
	}

	switch nonNull.(type) {
	case *parser.NamedType, *parser.KeywordType:
		return nonNull, true
	}

	return nil, false
}

func isNullType(t parser.TypeHint) bool {
	kt, ok := t.(*parser.KeywordType)

	if !ok {
		return false
	}

	return kt.Token.Kind == parser.KindNull
}

// reservedTypeLower returns the lowercased bytes of a bare, unqualified reserved
// scalar/special type name (PSR-12: types MUST be lower case), or ok=false for a
// class name, which keeps its source casing.
func reservedTypeLower(n *parser.Name) ([]byte, bool) {
	if n.LeadingBackslash.Kind != parser.KindInvalid || len(n.Parts) != 1 {
		return nil, false
	}

	lower := bytes.ToLower(n.Parts[0].Text)

	if reservedTypeNames[string(lower)] {
		return lower, true
	}

	return nil, false
}

var reservedTypeNames = map[string]bool{
	"int": true, "float": true, "string": true, "bool": true, "void": true,
	"never": true, "iterable": true, "object": true, "mixed": true,
	"null": true, "false": true, "true": true, "parent": true,
}
