package xml

import "testing"

func TestDocumentIsWellFormed(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr bool
	}{
		{"tab and newline in text", "<a>\tx\r\n</a>", false},
		{"control character", "<a>\x01</a>", true},
		{"noncharacter U+FFFE", "<a>￾</a>", true},
		{"invalid UTF-8", "<a>\xff</a>", true},
		{"Latin-1 bytes under a Latin-1 declaration", "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><a>caf\xe9</a>", false},
		{"Latin-1 bytes without a declaration", "<a>caf\xe9</a>", true},
		{"Latin-1 bytes under a PI named like a declaration", "<?xmlencoding=\"ISO-8859-1\"?><a>caf\xe9</a>", true},
		{"predefined entity", "<a b=\"&quot;\">&amp;</a>", false},
		{"character references", "<a>&#38;&#x26;</a>", false},
		{"reference to character zero", "<a>&#0;</a>", true},
		{"hexadecimal reference without digits", "<a>&#x;</a>", true},
		{"bare ampersand", "<a>x & y</a>", true},
		{"bare ampersand in a value", "<a b=\"x & y\"/>", true},
		{"undeclared entity", "<a>&nbsp;</a>", true},
		{"entity an external DTD may declare", "<!DOCTYPE a SYSTEM \"a.dtd\"><a>&nbsp;</a>", false},
		{"entity an internal subset may declare", "<!DOCTYPE a [<!ENTITY nbsp \"&#160;\">]><a>&nbsp;</a>", false},
		{"entity under a DOCTYPE that cannot declare one", "<!DOCTYPE a><a>&nbsp;</a>", true},
		{"text outside the root", "junk<a/>", true},
		{"CDATA outside the root", "<![CDATA[x]]><a/>", true},
		{"two roots", "<a/><b/>", true},
		{"no root", "<!-- only a comment -->", true},
		{"comment before the DOCTYPE", "<!-- c --><!DOCTYPE a><a/>", false},
		{"DOCTYPE after the root", "<a/><!DOCTYPE a>", true},
		{"two DOCTYPEs", "<!DOCTYPE a><!DOCTYPE a><a/>", true},
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
