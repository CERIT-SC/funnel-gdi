package storage

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/ohsu-comp-bio/funnel/config"
)

// Get of a directory copies the whole tree.
func TestLocalGetDirectory(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copy")
	os.MkdirAll(filepath.Join(src, "a", "b"), 0o755)
	os.WriteFile(filepath.Join(src, "top.txt"), []byte("top"), 0o644)
	os.WriteFile(filepath.Join(src, "a", "b", "deep.txt"), []byte("deep"), 0o644)

	local, err := NewLocal(&config.LocalStorage{AllowedDirs: []string{src}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := local.Get(context.Background(), "file://"+src, dst); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"top.txt": "top", "a/b/deep.txt": "deep"} {
		if data, err := os.ReadFile(filepath.Join(dst, path)); err != nil || string(data) != want {
			t.Errorf("%s: %q %v", path, data, err)
		}
	}

	files, err := FilePathWalkDir(src)
	sort.Strings(files)
	if err != nil || len(files) != 2 || filepath.Base(files[0]) != "deep.txt" {
		t.Errorf("FilePathWalkDir: %v %v", files, err)
	}

	// Paths outside the allowed directories are not supported.
	other := t.TempDir()
	if ops := local.UnsupportedOperations("file://" + other); ops.Get == nil {
		t.Error("a path outside AllowedDirs must not be readable")
	}
}
