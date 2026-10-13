package php

import "testing"

func TestBuildKeepsAnEmptyStatementBodyOnTheHeader(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"while", "<?php while ($a);", "while ($a);\n"},
		{"for", "<?php for (;$i<3;$i++);", "for (; $i < 3; $i++);\n"},
		{"foreach", "<?php foreach ($a as $b);", "foreach ($a as $b);\n"},
		{"if", "<?php if ($a);", "if ($a);\n"},
		{"if chain", "<?php if ($a); elseif ($b); else;", "if ($a);\nelseif ($b);\nelse;\n"},
		{"else after a block", "<?php if ($a) {} else;", "if ($a) {\n} else;\n"},
		{"do while", "<?php do; while ($a);", "do;\nwhile ($a);\n"},
		{"trailing comment", "<?php while ($a) ; // t", "while ($a); // t\n"},
		{"comment before the semicolon", "<?php while ($a) /* c */ ;", "while ($a) /* c */ ;\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPHP(t, tc.src)
			if want := "<?php\n\n" + tc.want; got != want {
				t.Fatalf("got %q, want %q", got, want)
			}

			if again := buildPHP(t, got); again != got {
				t.Fatalf("not idempotent: %q then %q", got, again)
			}
		})
	}
}
