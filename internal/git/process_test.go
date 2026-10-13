package git

import (
	"bufio"
	"strings"
	"testing"
)

func TestReadBatchBlobParsesCatFileReplies(t *testing.T) {
	const sha = "2222222222222222222222222222222222222222"

	tests := []struct {
		name    string
		reply   string
		want    string
		wantErr bool
	}{
		{name: "blob", reply: sha + " blob 5\n<?php\n", want: "<?php"},
		{name: "empty blob", reply: sha + " blob 0\n\n", want: ""},
		{name: "content holding a newline", reply: sha + " blob 3\na\nb\n", want: "a\nb"},
		{name: "missing object", reply: sha + " missing\n", wantErr: true},
		{name: "not a blob", reply: sha + " tree 0\n\n", wantErr: true},
		{name: "cut short", reply: sha + " blob 9\n<?php\n", wantErr: true},
		{name: "size out of step", reply: sha + " blob 2\n<?php\n", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := bufio.NewReader(strings.NewReader(tt.reply))

			got, err := readBatchBlob(r)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("readBatchBlob = %q, want an error", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("readBatchBlob: %v", err)
			}

			if string(got) != tt.want {
				t.Errorf("readBatchBlob = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQuotePathEscapesWhatBreaksALine(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "app/User.php", want: `"app/User.php"`},
		{path: `C:\Temp\blob`, want: `"C:\\Temp\\blob"`},
		{path: "a\nb.php", want: `"a\012b.php"`},
		{path: "a.php\r", want: `"a.php\015"`},
		{path: `say "hi".php`, want: `"say \"hi\".php"`},
		{path: "æøå.php", want: `"æøå.php"`},
	}

	for _, tt := range tests {
		if got := quotePath(tt.path); got != tt.want {
			t.Errorf("quotePath(%q) = %s, want %s", tt.path, got, tt.want)
		}
	}
}
