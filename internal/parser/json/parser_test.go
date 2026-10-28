package json

import (
	"testing"
	"time"
)

func TestParseRecognizesEachValueKind(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want any
	}{
		{"object", `{"a": 1}`, (*Object)(nil)},
		{"array", `[1, 2]`, (*Array)(nil)},
		{"string", `"hello"`, (*String)(nil)},
		{"number", `42`, (*Number)(nil)},
		{"bool true", `true`, (*Bool)(nil)},
		{"bool false", `false`, (*Bool)(nil)},
		{"null", `null`, (*Null)(nil)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, errs := Parse([]byte(tc.src))

			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			if file.Root == nil {
				t.Fatalf("root is nil; want %T", tc.want)
			}

			switch tc.want.(type) {
			case *Object:
				if _, ok := file.Root.(*Object); !ok {
					t.Fatalf("root is %T, want *Object", file.Root)
				}
			case *Array:
				if _, ok := file.Root.(*Array); !ok {
					t.Fatalf("root is %T, want *Array", file.Root)
				}
			case *String:
				if _, ok := file.Root.(*String); !ok {
					t.Fatalf("root is %T, want *String", file.Root)
				}
			case *Number:
				if _, ok := file.Root.(*Number); !ok {
					t.Fatalf("root is %T, want *Number", file.Root)
				}
			case *Bool:
				if _, ok := file.Root.(*Bool); !ok {
					t.Fatalf("root is %T, want *Bool", file.Root)
				}
			case *Null:
				if _, ok := file.Root.(*Null); !ok {
					t.Fatalf("root is %T, want *Null", file.Root)
				}
			}
		})
	}
}

func TestParsePreservesStringAndNumberRawBytes(t *testing.T) {
	src := `{"esc": "A\\n", "num": 1.0, "exp": 1e5}`

	file, errs := Parse([]byte(src))

	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	obj, ok := file.Root.(*Object)

	if !ok {
		t.Fatalf("root is %T, want *Object", file.Root)
	}

	if len(obj.Members) != 3 {
		t.Fatalf("got %d members, want 3", len(obj.Members))
	}

	esc, ok := obj.Members[0].Value.(*String)

	if !ok {
		t.Fatalf("members[0] is %T, want *String", obj.Members[0].Value)
	}

	if string(esc.Raw) != `A\\n` {
		t.Fatalf("string raw = %q, want %q", esc.Raw, `A\\n`)
	}

	num, ok := obj.Members[1].Value.(*Number)

	if !ok {
		t.Fatalf("members[1] is %T, want *Number", obj.Members[1].Value)
	}

	if string(num.Raw) != "1.0" {
		t.Fatalf("number raw = %q, want %q", num.Raw, "1.0")
	}

	exp, ok := obj.Members[2].Value.(*Number)

	if !ok {
		t.Fatalf("members[2] is %T, want *Number", obj.Members[2].Value)
	}

	if string(exp.Raw) != "1e5" {
		t.Fatalf("number raw = %q, want %q", exp.Raw, "1e5")
	}
}

func TestParseNestedStructures(t *testing.T) {
	src := `{"items": [{"id": 1}, {"id": 2}], "tags": ["a", "b"]}`

	file, errs := Parse([]byte(src))

	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	obj, ok := file.Root.(*Object)

	if !ok {
		t.Fatalf("root is %T, want *Object", file.Root)
	}

	items, ok := obj.Members[0].Value.(*Array)

	if !ok {
		t.Fatalf("items is %T, want *Array", obj.Members[0].Value)
	}

	if len(items.Elements) != 2 {
		t.Fatalf("got %d items, want 2", len(items.Elements))
	}

	first, ok := items.Elements[0].Value.(*Object)

	if !ok {
		t.Fatalf("items[0] is %T, want *Object", items.Elements[0].Value)
	} else if len(first.Members) != 1 {
		t.Fatalf("items[0] has %d members, want 1", len(first.Members))
	}
}

func TestParseEmptyDocument(t *testing.T) {
	for _, src := range []string{"", "   \n\t  "} {
		t.Run(src, func(t *testing.T) {
			file, errs := Parse([]byte(src))

			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}

			if file == nil {
				t.Fatal("file is nil; want non-nil with Root=nil")
			} else if file.Root != nil {
				t.Fatalf("root = %T, want nil", file.Root)
			}
		})
	}
}

func TestParseRejectsInvalidSyntax(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		jsonc bool
	}{
		{"trailing comma in object", `{"a": 1,}`, false},
		{"trailing comma in array", `[1, 2,]`, false},
		{"leading zero", `01`, false},
		{"unterminated string", `"hello`, false},
		{"control char in string", "\"a\x01b\"", false},
		{"line comment", `{"a": 1} // tail`, false},
		{"block comment", `/* head */ {"a": 1}`, false},
		{"trailing content", `{"a": 1} extra`, false},
		{"bare value after root", `42 13`, false},
		{"invalid string escape", `"\x"`, false},
		{"short unicode escape", `"\u12"`, false},
		{"non-hex unicode escape", `"\uZZZZ"`, false},
		{"value missing at the end", `{"a":`, false},
		{"unclosed empty object", `{`, false},
		{"unclosed object after a member", `{"a": 1`, false},
		{"unquoted key", `{a: 1}`, false},
		{"missing colon", `{"a" 1}`, false},
		{"missing comma in object", `{"a": 1 "b": 2}`, false},
		{"unclosed array", `[1, 2`, false},
		{"missing comma in array", `[1 2]`, false},
		{"escape at the end", `"ab\`, false},
		{"raw newline in string", "\"a\nb\"", false},
		{"lone minus", `-`, false},
		{"dot without a digit", `1.`, false},
		{"exponent without a digit", `1e+`, false},
		{"bad true", `tru`, false},
		{"bad null", `nul`, false},
		{"unterminated block comment", `/* open`, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parse := Parse
			if tc.jsonc {
				parse = ParseJSONC
			}

			_, errs := parse([]byte(tc.src))

			if len(errs) == 0 {
				t.Fatalf("expected at least one error, got none")
			}
		})
	}
}

func TestParseAcceptsValidStringEscapes(t *testing.T) {
	// Append the \uXXXX bytes explicitly so an editor can't rewrite the escape
	// into a literal rune.
	src := []byte(`"\" \\ \/ \b \f \n \r \t `)
	src = append(src, '\\', 'u', '0', '0', '4', '1', '"')

	_, errs := Parse(src)

	if len(errs) != 0 {
		t.Fatalf("valid string escapes were rejected: %v", errs)
	}
}

func TestParseRejectsDeepNestingWithoutCrashing(t *testing.T) {
	n := maxNestingDepth + 50

	src := make([]byte, 0, 2*n)

	for range n {
		src = append(src, '[')
	}

	for range n {
		src = append(src, ']')
	}

	result := make(chan []Error, 1)

	go func() {
		_, errs := Parse(src)
		result <- errs
	}()

	select {
	case errs := <-result:
		if len(errs) != 1 || errs[0].Msg != "maximum nesting depth exceeded" {
			t.Fatalf("errors = %v, want one depth error on input nested %d deep", errs, n)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Parse did not return on deeply nested input")
	}
}

func TestParseJSONCCapturesComments(t *testing.T) {
	src := "// header\n{\n  // lead\n  \"a\": 1, // trail\n}\n"

	file, errs := ParseJSONC([]byte(src))
	if len(errs) > 0 {
		t.Fatalf("ParseJSONC: %v", errs)
	}

	if len(file.Leading) != 1 || string(file.Leading[0].Text) != "// header" {
		t.Fatalf("file leading = %+v, want [// header]", file.Leading)
	}

	obj, ok := file.Root.(*Object)
	if !ok || len(obj.Members) != 1 {
		t.Fatalf("root = %T, want *Object with 1 member", file.Root)
	}

	m := obj.Members[0]
	if len(m.Leading) != 1 || string(m.Leading[0].Text) != "// lead" {
		t.Fatalf("member leading = %+v, want [// lead]", m.Leading)
	}

	if m.Trailing == nil || string(m.Trailing.Text) != "// trail" {
		t.Fatalf("member trailing = %+v, want // trail", m.Trailing)
	}
}

func TestParseJSONCAcceptsTrailingCommasStrictRejects(t *testing.T) {
	for _, src := range []string{`{"a": 1,}`, `[1, 2,]`} {
		if _, errs := ParseJSONC([]byte(src)); len(errs) > 0 {
			t.Fatalf("ParseJSONC(%q) errored: %v", src, errs)
		}

		if _, errs := Parse([]byte(src)); len(errs) == 0 {
			t.Fatalf("strict Parse(%q) accepted a trailing comma", src)
		}
	}
}

func TestParseStrictRejectsComments(t *testing.T) {
	if _, errs := Parse([]byte("{\n// c\n\"a\": 1}")); len(errs) == 0 {
		t.Fatal("strict Parse accepted a comment")
	}
}

func TestParseRecordsABlankLineBeforeAnItem(t *testing.T) {
	file, errs := ParseJSONC([]byte("{\n\n\"a\": [1,\n\n2, 3],\n\"b\": 4,\r\n\r\n// note\n\n\"c\": 5\n}"))
	if len(errs) > 0 {
		t.Fatalf("ParseJSONC returned errors: %v", errs)
	}

	obj, ok := file.Root.(*Object)
	if !ok {
		t.Fatalf("root is %T, want *Object", file.Root)
	}

	arr, ok := obj.Members[0].Value.(*Array)
	if !ok {
		t.Fatalf("first value is %T, want *Array", obj.Members[0].Value)
	}

	c := obj.Members[2]

	got := []bool{
		obj.Members[0].BlankBefore, obj.Members[1].BlankBefore, c.BlankBefore, c.Leading[0].BlankBefore,
		arr.Elements[1].BlankBefore, arr.Elements[2].BlankBefore,
	}
	want := []bool{true, false, true, true, true, false}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("BlankBefore flags = %v, want %v", got, want)
		}
	}
}

func TestUnterminatedConstructIsReportedAtItsOpener(t *testing.T) {
	cases := []struct {
		name, src, wantMsg string
		wantLine, wantCol  int
	}{
		{"object", "{\n  \"a\": 1,\n\n", "unclosed object", 1, 1},
		{"array", "[\n  1\n\n", "unclosed array", 1, 1},
		{"string", "[\"abc", "unterminated string", 1, 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := Parse([]byte(tc.src))

			for _, err := range errs {
				if err.Msg == tc.wantMsg {
					if err.Pos.Line != tc.wantLine || err.Pos.Column != tc.wantCol {
						t.Fatalf("%q at %d:%d, want %d:%d", err.Msg, err.Pos.Line, err.Pos.Column, tc.wantLine, tc.wantCol)
					}

					return
				}
			}

			t.Fatalf("errors = %v, want %q", errs, tc.wantMsg)
		})
	}
}
