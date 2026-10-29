package fsutil

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContextReaderWriter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var buf bytes.Buffer
	w := Writer(ctx, &buf)
	if n, err := w.Write([]byte("hello")); n != 5 || err != nil {
		t.Fatalf("write: %d %v", n, err)
	}
	r := Reader(ctx, strings.NewReader("data"))
	p := make([]byte, 4)
	if n, err := r.Read(p); n != 4 || err != nil || string(p) != "data" {
		t.Fatalf("read: %d %v %q", n, err, p)
	}

	cancel()
	if _, err := w.Write([]byte("more")); !errors.Is(err, context.Canceled) {
		t.Errorf("write after cancel: %v", err)
	}
	if _, err := r.Read(p); !errors.Is(err, context.Canceled) {
		t.Errorf("read after cancel: %v", err)
	}
	if buf.String() != "hello" {
		t.Errorf("written %q", buf.String())
	}
}

func TestFileSizeAndRepoRoot(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	os.WriteFile(f, []byte("12345"), 0o600)
	if FileSize(f) != 5 {
		t.Errorf("size %d", FileSize(f))
	}
	if FileSize(filepath.Join(dir, "missing")) != 0 {
		t.Error("missing file should have size 0")
	}

	root := FindRepoRoot()
	if _, err := os.Stat(filepath.Join(root, "go.mod")); root == "" || err != nil {
		t.Errorf("FindRepoRoot() = %q", root)
	}

	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	os.Chdir(dir)
	if got := FindRepoRoot(); got != "" {
		t.Errorf("outside a repository: %q", got)
	}
}
