package yaml

import "testing"

// Expected values follow YAML 1.2 rule c-printable (section 5.1).
func TestParseRejectsNonPrintableCharacters(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr bool
	}{
		{"tab, LF and CR", "a: b\tc\r\nd: e\n", false},
		{"multi-byte text", "a: \xc3\xa9\xe4\xb8\xad\xf0\x9f\x98\x80\n", false},
		{"next line", "a: b\xc2\x85c\n", false},
		{"byte order mark", "\xef\xbb\xbfa: b\n", false},
		{"replacement character", "a: \xef\xbf\xbd\n", false},
		{"C0 control", "a: b\x01\n", true},
		{"delete", "a: b\x7f\n", true},
		{"C1 control", "a: b\xc2\x80\n", true},
		{"noncharacter", "a: b\xef\xbf\xbf\n", true},
		{"encoded surrogate", "a: b\xed\xa0\x80\n", true},
		{"invalid UTF-8", "a: b\xff\n", true},
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

func TestParseReportsTheForbiddenCharacterPosition(t *testing.T) {
	_, errs := Parse([]byte("a: b\nc: \xc3\xa9\xc2\x80\n"))

	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1", len(errs))
	}

	if pos := errs[0].Pos; pos.Offset != 10 || pos.Line != 2 || pos.Column != 6 {
		t.Fatalf("position = %+v, want offset 10, line 2, column 6", pos)
	}
}
