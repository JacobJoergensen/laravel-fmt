// Package php is a lossless lexer and parser for PHP 8.4 and 8.5 source.
// The token stream preserves every byte of input: whitespace and comments are
// recorded as Trivia attached to the token that follows them, so a formatter
// can choose between preserving and rewriting any span of the original source.
package php

import (
	"fmt"

	parserpkg "github.com/JacobJoergensen/laravel-fmt/internal/parser"
)

// Position is a byte offset into source with its 1-based line and column,
// aliased to parser.Position so the parser tree shares one coordinate type.
// Column is measured in bytes, not runes.
type Position = parserpkg.Position

// Span is a half-open byte range [Start, End) in source. Aliased to
// parser.Span for the same reason as Position.
type Span = parserpkg.Span

// Kind identifies a PHP token.
type Kind uint16

const (
	KindInvalid Kind = iota
	KindEOF

	KindInlineHTML
	KindOpenTag
	KindOpenTagEcho
	KindCloseTag
	KindHaltCompiler
	KindHaltPayload

	KindInt
	KindFloat
	// KindString also represents heredoc and nowdoc literals.
	KindString
	KindShellExec

	KindIdent
	KindVariable

	// KindAbstract begins the keyword tokens. The block must stay contiguous and
	// end at KindYieldFrom; isMemberNameToken tests that ordinal range.
	KindAbstract
	KindAnd
	KindArray
	KindAs
	KindBreak
	KindCallable
	KindCase
	KindCatch
	KindClass
	KindClone
	KindConst
	KindContinue
	KindDeclare
	KindDefault
	KindDo
	KindEcho
	KindElse
	KindElseIf
	KindEmpty
	KindEndDeclare
	KindEndFor
	KindEndForEach
	KindEndIf
	KindEndSwitch
	KindEndWhile
	KindEnum
	KindExtends
	KindFalse
	KindFinal
	KindFinally
	KindFn
	KindFor
	KindForEach
	KindFunction
	KindGlobal
	KindGoto
	KindIf
	KindImplements
	KindInclude
	KindIncludeOnce
	KindInstanceOf
	KindInsteadOf
	KindInterface
	KindList
	KindMatch
	KindNamespace
	KindNew
	KindNull
	KindOr
	KindPrint
	KindPrivate
	KindProtected
	KindPublic
	KindReadonly
	KindRequire
	KindRequireOnce
	KindReturn
	KindSelf
	KindStatic
	KindSwitch
	KindThrow
	KindTrait
	KindTrue
	KindTry
	KindUnset
	KindUse
	KindVar
	KindWhile
	KindXor
	KindYield
	KindYieldFrom

	KindPlus
	KindMinus
	KindPlusPlus
	KindMinusMinus
	KindStar
	KindSlash
	KindPercent
	KindStarStar
	KindDot
	KindAmp
	KindPipe
	KindCaret
	KindTilde
	KindBang
	KindAt
	KindEq
	KindLt
	KindGt
	KindQuestion
	KindColon
	KindEqEq
	KindEqEqEq
	KindBangEq
	KindBangEqEq
	KindLtEq
	KindGtEq
	KindSpaceship
	KindAmpAmp
	KindPipePipe
	KindPipeline
	KindNullCoalesce
	KindShl
	KindShr
	KindArrow
	KindNullSafeArrow
	KindDoubleArrow
	KindScopeRes
	KindEllipsis
	KindAttributeOpen

	KindPlusAssign
	KindMinusAssign
	KindStarAssign
	KindSlashAssign
	KindPercentAssign
	KindStarStarAssign
	KindDotAssign
	KindAmpAssign
	KindPipeAssign
	KindCaretAssign
	KindShlAssign
	KindShrAssign
	KindNullCoalesceAssign

	KindLParen
	KindRParen
	KindLBrace
	KindRBrace
	KindLBracket
	KindRBracket
	KindComma
	KindSemicolon
	KindBackslash
	KindDollar

	kindLast
)

var kindNames = [...]string{
	KindInvalid: "Invalid",
	KindEOF:     "EOF",

	KindInlineHTML:   "InlineHTML",
	KindHaltCompiler: "__halt_compiler",
	KindHaltPayload:  "HaltPayload",
	KindOpenTag:      "OpenTag",
	KindOpenTagEcho:  "OpenTagEcho",
	KindCloseTag:     "CloseTag",

	KindInt:       "Int",
	KindFloat:     "Float",
	KindString:    "String",
	KindShellExec: "ShellExec",

	KindIdent:    "Ident",
	KindVariable: "Variable",

	KindAbstract:    "abstract",
	KindAnd:         "and",
	KindArray:       "array",
	KindAs:          "as",
	KindBreak:       "break",
	KindCallable:    "callable",
	KindCase:        "case",
	KindCatch:       "catch",
	KindClass:       "class",
	KindClone:       "clone",
	KindConst:       "const",
	KindContinue:    "continue",
	KindDeclare:     "declare",
	KindDefault:     "default",
	KindDo:          "do",
	KindEcho:        "echo",
	KindElse:        "else",
	KindElseIf:      "elseif",
	KindEmpty:       "empty",
	KindEndDeclare:  "enddeclare",
	KindEndFor:      "endfor",
	KindEndForEach:  "endforeach",
	KindEndIf:       "endif",
	KindEndSwitch:   "endswitch",
	KindEndWhile:    "endwhile",
	KindEnum:        "enum",
	KindExtends:     "extends",
	KindFalse:       "false",
	KindFinal:       "final",
	KindFinally:     "finally",
	KindFn:          "fn",
	KindFor:         "for",
	KindForEach:     "foreach",
	KindFunction:    "function",
	KindGlobal:      "global",
	KindGoto:        "goto",
	KindIf:          "if",
	KindImplements:  "implements",
	KindInclude:     "include",
	KindIncludeOnce: "include_once",
	KindInstanceOf:  "instanceof",
	KindInsteadOf:   "insteadof",
	KindInterface:   "interface",
	KindList:        "list",
	KindMatch:       "match",
	KindNamespace:   "namespace",
	KindNew:         "new",
	KindNull:        "null",
	KindOr:          "or",
	KindPrint:       "print",
	KindPrivate:     "private",
	KindProtected:   "protected",
	KindPublic:      "public",
	KindReadonly:    "readonly",
	KindRequire:     "require",
	KindRequireOnce: "require_once",
	KindReturn:      "return",
	KindSelf:        "self",
	KindStatic:      "static",
	KindSwitch:      "switch",
	KindThrow:       "throw",
	KindTrait:       "trait",
	KindTrue:        "true",
	KindTry:         "try",
	KindUnset:       "unset",
	KindUse:         "use",
	KindVar:         "var",
	KindWhile:       "while",
	KindXor:         "xor",
	KindYield:       "yield",
	KindYieldFrom:   "yield from",

	KindPlus:          "+",
	KindMinus:         "-",
	KindPlusPlus:      "++",
	KindMinusMinus:    "--",
	KindStar:          "*",
	KindSlash:         "/",
	KindPercent:       "%",
	KindStarStar:      "**",
	KindDot:           ".",
	KindAmp:           "&",
	KindPipe:          "|",
	KindCaret:         "^",
	KindTilde:         "~",
	KindBang:          "!",
	KindAt:            "@",
	KindEq:            "=",
	KindLt:            "<",
	KindGt:            ">",
	KindQuestion:      "?",
	KindColon:         ":",
	KindEqEq:          "==",
	KindEqEqEq:        "===",
	KindBangEq:        "!=",
	KindBangEqEq:      "!==",
	KindLtEq:          "<=",
	KindGtEq:          ">=",
	KindSpaceship:     "<=>",
	KindAmpAmp:        "&&",
	KindPipePipe:      "||",
	KindPipeline:      "|>",
	KindNullCoalesce:  "??",
	KindShl:           "<<",
	KindShr:           ">>",
	KindArrow:         "->",
	KindNullSafeArrow: "?->",
	KindDoubleArrow:   "=>",
	KindScopeRes:      "::",
	KindEllipsis:      "...",
	KindAttributeOpen: "#[",

	KindPlusAssign:         "+=",
	KindMinusAssign:        "-=",
	KindStarAssign:         "*=",
	KindSlashAssign:        "/=",
	KindPercentAssign:      "%=",
	KindStarStarAssign:     "**=",
	KindDotAssign:          ".=",
	KindAmpAssign:          "&=",
	KindPipeAssign:         "|=",
	KindCaretAssign:        "^=",
	KindShlAssign:          "<<=",
	KindShrAssign:          ">>=",
	KindNullCoalesceAssign: "??=",

	KindLParen:    "(",
	KindRParen:    ")",
	KindLBrace:    "{",
	KindRBrace:    "}",
	KindLBracket:  "[",
	KindRBracket:  "]",
	KindComma:     ",",
	KindSemicolon: ";",
	KindBackslash: "\\",
	KindDollar:    "$",
}

// String returns the kind's source spelling (`;`) or its name (`EOF`), which
// parse errors print.
func (k Kind) String() string {
	if int(k) < len(kindNames) && kindNames[k] != "" {
		return kindNames[k]
	}

	return fmt.Sprintf("Kind(%d)", k)
}

var keywords = map[string]Kind{
	"abstract":     KindAbstract,
	"and":          KindAnd,
	"array":        KindArray,
	"as":           KindAs,
	"break":        KindBreak,
	"callable":     KindCallable,
	"case":         KindCase,
	"catch":        KindCatch,
	"class":        KindClass,
	"clone":        KindClone,
	"const":        KindConst,
	"continue":     KindContinue,
	"declare":      KindDeclare,
	"default":      KindDefault,
	"do":           KindDo,
	"echo":         KindEcho,
	"else":         KindElse,
	"elseif":       KindElseIf,
	"empty":        KindEmpty,
	"enddeclare":   KindEndDeclare,
	"endfor":       KindEndFor,
	"endforeach":   KindEndForEach,
	"endif":        KindEndIf,
	"endswitch":    KindEndSwitch,
	"endwhile":     KindEndWhile,
	"enum":         KindEnum,
	"extends":      KindExtends,
	"false":        KindFalse,
	"final":        KindFinal,
	"finally":      KindFinally,
	"fn":           KindFn,
	"for":          KindFor,
	"foreach":      KindForEach,
	"function":     KindFunction,
	"global":       KindGlobal,
	"goto":         KindGoto,
	"if":           KindIf,
	"implements":   KindImplements,
	"include":      KindInclude,
	"include_once": KindIncludeOnce,
	"instanceof":   KindInstanceOf,
	"insteadof":    KindInsteadOf,
	"interface":    KindInterface,
	"list":         KindList,
	"match":        KindMatch,
	"namespace":    KindNamespace,
	"new":          KindNew,
	"null":         KindNull,
	"or":           KindOr,
	"print":        KindPrint,
	"private":      KindPrivate,
	"protected":    KindProtected,
	"public":       KindPublic,
	"readonly":     KindReadonly,
	"require":      KindRequire,
	"require_once": KindRequireOnce,
	"return":       KindReturn,
	"self":         KindSelf,
	"static":       KindStatic,
	"switch":       KindSwitch,
	"throw":        KindThrow,
	"trait":        KindTrait,
	"true":         KindTrue,
	"try":          KindTry,
	"unset":        KindUnset,
	"use":          KindUse,
	"var":          KindVar,
	"while":        KindWhile,
	"xor":          KindXor,
	"yield":        KindYield,
}

// Keyword lengths bound the scan in KeywordKind. TestKeywordLenBounds pins both
// to the keywords table: a keyword added outside this range would be silently
// lexed as an identifier, a correctness bug rather than a slow path.
const (
	keywordLenMin = 2  // as, do, fn, if, or
	keywordLenMax = 12 // include_once, require_once
)

// KeywordKind returns the keyword Kind for an identifier's raw source bytes, or
// KindIdent when they are not a reserved word. PHP keywords are case-insensitive,
// so the text is lowered here rather than by the caller.
//
// The lowering is byte-wise ASCII, which is equivalent to strings.ToLower for
// anything that could match: every keyword is lowercase ASCII, so text the two
// would lower differently cannot be a keyword under either rule and returns
// KindIdent from both.
func KeywordKind(text []byte) Kind {
	if len(text) < keywordLenMin || len(text) > keywordLenMax {
		return KindIdent
	}

	var lower [keywordLenMax]byte

	for i, c := range text {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}

		lower[i] = c
	}

	if k, ok := keywords[string(lower[:len(text)])]; ok {
		return k
	}

	return KindIdent
}

// IsKeyword reports whether k is a reserved-word kind. It is derived from the
// keywords table, so magic constants (absent from that table) are excluded and
// keep their source casing.
func IsKeyword(k Kind) bool {
	return keywordKinds[k]
}

var keywordKinds = func() map[Kind]bool {
	set := make(map[Kind]bool, len(keywords))

	for _, k := range keywords {
		set[k] = true
	}

	return set
}()

// TriviaKind categorizes a run of non-token source.
type TriviaKind uint8

const (
	TriviaWhitespace TriviaKind = iota
	TriviaLineComment
	TriviaBlockComment
	TriviaDocComment
)

// Trivia is a whitespace run or comment attached to the token that follows it.
type Trivia struct {
	Kind TriviaKind
	Span Span
	Text []byte
}

// Token is a single lexical unit with its preceding trivia.
// Text slices into the original source; callers must keep the source alive
// for as long as they hold Tokens.
type Token struct {
	Kind    Kind
	Span    Span
	Text    []byte
	Leading []Trivia
}
