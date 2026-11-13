package php

import (
	"bytes"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/formatter/document"
	parser "github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
)

func TestKeepCommentsTrustsALayoutOnlyIfEveryVariantPrintsTheComment(t *testing.T) {
	file, errs := parser.Parse([]byte("<?php\n\nf(/* note */ $a);\n"))
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}

	cases := []struct {
		name  string
		doc   func(note *document.Doc) *document.Doc
		trust bool
	}{
		{"in every variant", func(note *document.Doc) *document.Doc {
			return document.ConditionalGroup(document.Concat(note, document.Text("a")), document.Group(note))
		}, true},
		{"in one variant only", func(note *document.Doc) *document.Doc {
			return document.ConditionalGroup(document.Concat(note, document.Text("a")), document.Text("b"))
		}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := &builder{src: file.Src, cfg: config.Default(), tokens: file.Tokens, comments: commentOffsets(file.Tokens)}
			stmt := file.Stmts[len(file.Stmts)-1]

			b.place(b.comments[0])
			note := b.tagComment(document.Text("/* note */ "), b.comments[0])
			doc := tc.doc(note)

			if trusted := b.keepComments(stmt.Span(), doc, stmt) == doc; trusted != tc.trust {
				t.Fatalf("layout kept = %v, want %v", trusted, tc.trust)
			}
		})
	}
}

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

func TestCopiedStatementKeepsTheSemicolonACloseTagStoodIn(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{
			"alternative syntax inside added braces",
			"<?php while(0)while(0):0;endwhile?>0",
			"<?php while (0) {\n    while(0):0;endwhile;\n}?>0\n",
		},
		{
			"alternative switch inside added braces",
			"<?php while(0)switch(0):endswitch?>0",
			"<?php while (0) {\n    switch(0):endswitch;\n}?>0\n",
		},
		{
			"template region at the end of the file",
			"<?php if ($x): ?>\n<p>hi</p>\n<?php endif ?>\n",
			"<?php\n\nif ($x): ?>\n<p>hi</p>\n<?php endif;\n?>\n",
		},
		{
			"alternative syntax ending a file of pure PHP",
			"<?php while(0)while(0):0;endwhile?>",
			"<?php\n\nwhile (0) {\n    while(0):0;endwhile;\n}\n",
		},
		{
			"commented declare inside added braces",
			"<?php while(0)declare(ticks=1)#\n0?>0",
			"<?php while (0) {\n    declare(ticks=1)#\n0;\n}?>0\n",
		},
		{
			"commented loop inside added braces",
			"<?php while(0)while(0)#\n0?>0",
			"<?php while (0) {\n    while(0)#\n0;\n}?>0\n",
		},
		{
			"commented if and else inside added braces",
			"<?php while(0)if(0)#\n0; else #\n1?>0",
			"<?php while (0) {\n    if(0)#\n0; else #\n1;\n}?>0\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustFormat(t, []byte(tt.in), config.Default())
			if string(got) != tt.want {
				t.Fatalf("got\n%s\nwant\n%s", got, tt.want)
			}

			if twice := mustFormat(t, got, config.Default()); !bytes.Equal(twice, got) {
				t.Fatalf("not idempotent:\n%s\n---\n%s", got, twice)
			}
		})
	}
}
