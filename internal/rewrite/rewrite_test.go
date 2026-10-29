package rewrite

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// memFile is a file whose first write stores only failAfter bytes and then
// fails, the way a write that runs out of disk space does.
type memFile struct {
	data      []byte
	failAfter int
	failed    bool
	truncErr  error
	closeErr  error
}

var (
	errDiskFull = errors.New("no space left on device")
	errIO       = errors.New("input/output error")
)

func (m *memFile) WriteAt(b []byte, off int64) (int, error) {
	n := len(b)
	if !m.failed && m.failAfter >= 0 {
		m.failed = true
		n = m.failAfter
	}

	end := int(off) + n
	if end > len(m.data) {
		m.data = append(m.data, make([]byte, end-len(m.data))...)
	}

	copy(m.data[off:], b[:n])

	if n < len(b) {
		return n, errDiskFull
	}

	return n, nil
}

func (m *memFile) Truncate(size int64) error {
	if m.truncErr != nil {
		err := m.truncErr
		m.truncErr = nil

		return err
	}

	m.data = m.data[:size]

	return nil
}

func (m *memFile) Close() error { return m.closeErr }

func TestRewriteReplacesTheContent(t *testing.T) {
	for name, tc := range map[string]struct{ orig, formatted string }{
		"a longer result":  {"<?php\n$a=1;\n", "<?php\n\n$a = 1;\n"},
		"a shorter result": {"<?php   $a   =   1;   \n\n\n", "<?php $a = 1;\n"},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "x.php")
			if err := os.WriteFile(path, []byte(tc.orig), 0o600); err != nil {
				t.Fatal(err)
			}

			if err := Rewrite(path, []byte(tc.orig), []byte(tc.formatted)); err != nil {
				t.Fatalf("Rewrite: %v", err)
			}

			if got, _ := os.ReadFile(path); string(got) != tc.formatted {
				t.Fatalf("file holds %q, want %q", got, tc.formatted)
			}
		})
	}
}

func TestRewriteRestoresTheOriginalWhenTheWriteFails(t *testing.T) {
	orig := []byte("<?php\n\nreturn ['a' => 1, 'b' => 2];\n")
	formatted := []byte("<?php\n\nreturn [\n    'a' => 1,\n    'b' => 2,\n];\n")

	for name, f := range map[string]*memFile{
		// Past byte 15, where the two first differ, so the partial write
		// changes the file.
		"out of space partway": {data: bytes.Clone(orig), failAfter: 24},
		"truncate fails":       {data: bytes.Clone(orig), failAfter: -1, truncErr: errDiskFull},
	} {
		t.Run(name, func(t *testing.T) {
			err := rewrite(f, orig, formatted)
			if !errors.Is(err, errDiskFull) {
				t.Fatalf("err = %v, want the write failure", err)
			}

			if !bytes.Equal(f.data, orig) {
				t.Fatalf("file holds %q after the failure, want the original %q", f.data, orig)
			}
		})
	}
}

// The restored original may not have reached the disk, so a failed close is
// reported alongside the failed write.
func TestRewriteReportsACloseFailureAfterAFailedWrite(t *testing.T) {
	orig := []byte("<?php $a=1;")
	f := &memFile{data: bytes.Clone(orig), failAfter: 3, closeErr: errIO}

	err := rewrite(f, orig, []byte("<?php $a = 1;\n"))
	if !errors.Is(err, errDiskFull) || !errors.Is(err, errIO) {
		t.Fatalf("err = %v, want both the write and the close failure", err)
	}
}

func TestRewriteKeepsTheFileItself(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.php")
	link := filepath.Join(dir, "link.php")

	if err := os.WriteFile(path, []byte("<?php $a=1;"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Link(path, link); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}

	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := Rewrite(path, []byte("<?php $a=1;"), []byte("<?php $a = 1;\n")); err != nil {
		t.Fatalf("Rewrite: %v", err)
	}

	if got, _ := os.ReadFile(link); string(got) != "<?php $a = 1;\n" {
		t.Fatalf("the hard link holds %q: the file was replaced, not rewritten", got)
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if after.Mode() != before.Mode() {
		t.Fatalf("mode changed from %v to %v", before.Mode(), after.Mode())
	}
}
