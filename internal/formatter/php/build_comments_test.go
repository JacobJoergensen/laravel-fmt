package php

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestAuthorAlignedCommentsStayAlignedWhateverTheLineEnding(t *testing.T) {
	want := "<?php\n\n$first = 1;   // one\n$second = 22; // two\n"

	for _, eol := range []string{"\n", "\r\n", "\r"} {
		src := "<?php" + eol + eol + "$first = 1;     // one" + eol + "$second = 22;   // two" + eol

		if got := string(mustFormat(t, []byte(src), config.Default())); got != want {
			t.Errorf("eol %q:\ngot  %q\nwant %q", eol, got, want)
		}
	}
}

func TestTrailingCommentGroupsUseDisplayColumns(t *testing.T) {
	src := "<?php\n\n$a = [\n    'æ' => 1, // a\n    'ab' => 1, // b\n];\n"

	once := mustFormat(t, []byte(src), config.Default())
	if twice := mustFormat(t, once, config.Default()); !bytes.Equal(twice, once) {
		t.Fatalf("not idempotent:\n%s\n---\n%s", once, twice)
	}

	if string(once) != src {
		t.Errorf("got\n%s\nwant the source unchanged", once)
	}
}

func TestCarriageReturnInsideATokenOrCommentDoesNotStartAnAlignedLine(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "block comment",
			src:  "<?php\n\n$a = [0/*\r*//*\n*//**/];\n",
			want: "<?php\n\n$a = [\n    0, /*\r*/ /*\n    */ /**/\n];\n",
		},
		{
			name: "string",
			src:  "<?php\n\n$a = ['\r00000000000',#0\n'000000'#0\n];\n",
			want: "<?php\n\n$a = [\n    '\r00000000000', #0\n    '000000', #0\n];\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			once := mustFormat(t, []byte(tt.src), config.Default())
			if string(once) != tt.want {
				t.Errorf("got  %q\nwant %q", once, tt.want)
			}

			if twice := mustFormat(t, once, config.Default()); !bytes.Equal(twice, once) {
				t.Errorf("not idempotent:\n%q\n---\n%q", once, twice)
			}
		})
	}
}
