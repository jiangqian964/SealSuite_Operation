package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteFileWritesContent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")

	if err := AtomicWriteFile(p, []byte("hello"), 0o600); err != nil {
		t.Fatalf("AtomicWriteFile err=%v", err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile err=%v", err)
	}
	if string(b) != "hello" {
		t.Fatalf("unexpected content: %q", string(b))
	}
}

