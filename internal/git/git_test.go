package git

import "testing"

func TestParseRawKeepsRegularStagedBlobs(t *testing.T) {
	out := []byte(
		":100644 100644 1111111111111111111111111111111111111111 2222222222222222222222222222222222222222 M\x00app/User.php\x00" +
			":000000 100755 0000000000000000000000000000000000000000 3333333333333333333333333333333333333333 A\x00bin/run\x00" +
			":100644 120000 4444444444444444444444444444444444444444 5555555555555555555555555555555555555555 M\x00link\x00",
	)

	got := parseRaw(out)

	want := []StagedFile{
		{Path: "app/User.php", Mode: "100644", SHA: "2222222222222222222222222222222222222222"},
		{Path: "bin/run", Mode: "100755", SHA: "3333333333333333333333333333333333333333"},
	}

	if len(got) != len(want) {
		t.Fatalf("parseRaw returned %d entries, want %d: %+v", len(got), len(want), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSplitNULDropsEmptyTrailingRecord(t *testing.T) {
	got := splitNUL([]byte("a\x00b\x00"))

	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("splitNUL = %q, want [a b]", got)
	}

	if splitNUL(nil) != nil {
		t.Error("splitNUL(nil) should be nil")
	}
}
