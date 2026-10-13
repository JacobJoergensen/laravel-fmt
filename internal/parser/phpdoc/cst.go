package phpdoc

// Type is a parsed PHPDoc type expression.
type Type interface {
	isType()
}

// IdentType is a type name such as int, Foo, \A\B, non-empty-string, or $this.
type IdentType struct {
	Name []byte
}

// LiteralType is an integer, float or quoted string used as a type.
type LiteralType struct {
	Raw []byte
}

// NullableType is ?T.
type NullableType struct {
	Inner Type
}

// UnionType is A|B|C.
type UnionType struct {
	Members []Type
}

// IntersectionType is A&B.
type IntersectionType struct {
	Members []Type
}

// ParenType is a parenthesized group such as (A&B) in a DNF type.
type ParenType struct {
	Inner Type
}

// GenericType is Foo<A, B>.
type GenericType struct {
	Base Type
	Args []GenericArg
}

// GenericArg is one argument of a generic type, optionally prefixed by a
// variance keyword (covariant or contravariant).
type GenericArg struct {
	Variance []byte
	Type     Type
}

// ShapeType is a shape such as array{foo: int}; HasRest marks a trailing ...
// that leaves it unsealed. Broken records a line break right after the `{`:
// the author laid the shape out one item per line.
type ShapeType struct {
	Keyword []byte
	Items   []ShapeItem
	HasRest bool
	Broken  bool
}

// ShapeItem is one entry of a shape. Key is nil for a positional item such as
// the int in array{int, string}; Optional records a key? marker.
type ShapeItem struct {
	Key      []byte
	Optional bool
	Value    Type
}

// ArraySuffixType is the legacy T[] array notation.
type ArraySuffixType struct {
	Inner Type
}

// OffsetAccessType is T[K], an offset into another type.
type OffsetAccessType struct {
	Inner  Type
	Offset Type
}

// ConstFetchType is Foo::BAR used as a type; Member may end in a * wildcard.
type ConstFetchType struct {
	Class  []byte
	Member []byte
}

// CallableType is a callable or Closure signature such as
// callable(int, string): bool. Return is nil when no return type is given.
type CallableType struct {
	Keyword []byte
	Params  []CallableParam
	Return  Type
}

// CallableParam is one parameter of a callable signature; Name is nil when
// the parameter is unnamed.
type CallableParam struct {
	Type     Type
	ByRef    bool
	Variadic bool
	Name     []byte
	Optional bool
}

// ConditionalType is T is X ? Y : Z, optionally negated as T is not X ? Y : Z.
type ConditionalType struct {
	Subject Type
	Negated bool
	Target  Type
	If      Type
	Else    Type
}

// Invalid is a type the parser could not read in full; Raw is the whole source.
type Invalid struct {
	Raw []byte
}

func (*IdentType) isType()        {}
func (*LiteralType) isType()      {}
func (*NullableType) isType()     {}
func (*UnionType) isType()        {}
func (*IntersectionType) isType() {}
func (*ParenType) isType()        {}
func (*GenericType) isType()      {}
func (*ShapeType) isType()        {}
func (*ArraySuffixType) isType()  {}
func (*OffsetAccessType) isType() {}
func (*ConstFetchType) isType()   {}
func (*CallableType) isType()     {}
func (*ConditionalType) isType()  {}
func (*Invalid) isType()          {}
