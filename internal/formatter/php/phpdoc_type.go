package php

import (
	"bytes"

	"github.com/JacobJoergensen/laravel-fmt/internal/parser/phpdoc"
)

// printPHPDocType renders t with canonical PHPDoc spacing. An *Invalid is
// written back unchanged, so a type the parser could not read survives
// byte for byte.
func printPHPDocType(t phpdoc.Type, spaced bool) []byte {
	var b bytes.Buffer

	writePHPDocType(&b, t, spaced)

	return b.Bytes()
}

func writePHPDocType(b *bytes.Buffer, t phpdoc.Type, spaced bool) {
	switch n := t.(type) {
	case *phpdoc.IdentType:
		b.Write(n.Name)
	case *phpdoc.LiteralType:
		b.Write(n.Raw)
	case *phpdoc.NullableType:
		b.WriteByte('?')
		writePHPDocType(b, n.Inner, spaced)
	case *phpdoc.UnionType:
		for i, m := range n.Members {
			if i > 0 {
				b.WriteString(typeSeparator("|", spaced))
			}

			writePHPDocType(b, m, spaced)
		}
	case *phpdoc.IntersectionType:
		for i, m := range n.Members {
			if i > 0 {
				b.WriteString(typeSeparator("&", spaced))
			}

			writePHPDocType(b, m, spaced)
		}
	case *phpdoc.ParenType:
		b.WriteByte('(')
		writePHPDocType(b, n.Inner, spaced)
		b.WriteByte(')')
	case *phpdoc.GenericType:
		writePHPDocType(b, n.Base, spaced)
		b.WriteByte('<')

		for i, a := range n.Args {
			if i > 0 {
				b.WriteString(", ")
			}

			if a.Variance != nil {
				b.Write(a.Variance)
				b.WriteByte(' ')
			}

			writePHPDocType(b, a.Type, spaced)
		}

		b.WriteByte('>')
	case *phpdoc.ShapeType:
		writePHPDocShape(b, n, spaced)
	case *phpdoc.ArraySuffixType:
		writePHPDocType(b, n.Inner, spaced)
		b.WriteString("[]")
	case *phpdoc.OffsetAccessType:
		writePHPDocType(b, n.Inner, spaced)
		b.WriteByte('[')
		writePHPDocType(b, n.Offset, spaced)
		b.WriteByte(']')
	case *phpdoc.ConstFetchType:
		b.Write(n.Class)
		b.WriteString("::")
		b.Write(n.Member)
	case *phpdoc.CallableType:
		writePHPDocCallable(b, n, spaced)
	case *phpdoc.ConditionalType:
		writePHPDocType(b, n.Subject, spaced)
		b.WriteString(" is ")

		if n.Negated {
			b.WriteString("not ")
		}

		writePHPDocType(b, n.Target, spaced)
		b.WriteString(" ? ")
		writePHPDocType(b, n.If, spaced)
		b.WriteString(" : ")
		writePHPDocType(b, n.Else, spaced)
	case *phpdoc.Invalid:
		b.Write(n.Raw)
	}
}

func writePHPDocCallable(b *bytes.Buffer, c *phpdoc.CallableType, spaced bool) {
	b.Write(c.Keyword)
	b.WriteByte('(')

	for i, pm := range c.Params {
		if i > 0 {
			b.WriteString(", ")
		}

		writePHPDocType(b, pm.Type, spaced)

		if pm.ByRef || pm.Variadic || pm.Name != nil {
			b.WriteByte(' ')

			if pm.ByRef {
				b.WriteByte('&')
			}

			if pm.Variadic {
				b.WriteString("...")
			}

			b.Write(pm.Name)
		}

		if pm.Optional {
			b.WriteByte('=')
		}
	}

	b.WriteByte(')')

	if c.Return != nil {
		b.WriteString(": ")
		writePHPDocType(b, c.Return, spaced)
	}
}

func writePHPDocShape(b *bytes.Buffer, s *phpdoc.ShapeType, spaced bool) {
	b.Write(s.Keyword)
	b.WriteByte('{')

	for i, it := range s.Items {
		if i > 0 {
			b.WriteString(", ")
		}

		if it.Key != nil {
			b.Write(it.Key)

			if it.Optional {
				b.WriteByte('?')
			}

			b.WriteString(": ")
		}

		writePHPDocType(b, it.Value, spaced)
	}

	if s.HasRest {
		if len(s.Items) > 0 {
			b.WriteString(", ")
		}

		b.WriteString("...")
	}

	b.WriteByte('}')
}
