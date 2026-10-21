package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ohsu-comp-bio/funnel/tes"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// Inputs (files and directory trees) are copied into the scratch directory
// under their full path.
func TestCopyInputsToScratch(t *testing.T) {
	work := t.TempDir()
	scratch := t.TempDir()
	file := filepath.Join(work, "inputs", "a.txt")
	dir := filepath.Join(work, "inputs", "dir")
	writeFile(t, file, "A")
	writeFile(t, filepath.Join(dir, "b.txt"), "B")
	writeFile(t, filepath.Join(dir, "nested", "c.txt"), "C")

	mapper := &FileMapper{Inputs: []*tes.Input{{Path: file}, {Path: dir, Type: tes.Directory}}}
	if err := mapper.CopyInputsToScratch(scratch); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(scratch, file)); got != "A" {
		t.Errorf("file input: %q", got)
	}
	if got := readFile(t, filepath.Join(scratch, dir, "nested", "c.txt")); got != "C" {
		t.Errorf("directory input: %q", got)
	}
}

// Outputs written in the scratch directory are copied back to the output
// paths; wildcards select several files.
func TestCopyOutputsToWorkDir(t *testing.T) {
	work := t.TempDir()
	scratch := t.TempDir()
	outFile := filepath.Join(work, "out", "result.txt")
	outDir := filepath.Join(work, "out", "dir")
	writeFile(t, filepath.Join(scratch, outFile), "R")
	writeFile(t, filepath.Join(scratch, outDir, "x.txt"), "X")
	writeFile(t, filepath.Join(scratch, work, "out", "log1.log"), "L1")
	writeFile(t, filepath.Join(scratch, work, "out", "log2.log"), "L2")
	os.MkdirAll(filepath.Join(work, "out"), 0o755)

	mapper := &FileMapper{Outputs: []*tes.Output{
		{Path: outFile},
		{Path: outDir, Type: tes.Directory},
		{Path: filepath.Join(work, "out", "*.log")},
	}}
	if err := mapper.CopyOutputsToWorkDir(scratch); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		outFile:                                "R",
		filepath.Join(outDir, "x.txt"):         "X",
		filepath.Join(work, "out", "log1.log"): "L1",
		filepath.Join(work, "out", "log2.log"): "L2",
	} {
		if got := readFile(t, path); got != want {
			t.Errorf("%s: %q, want %q", path, got, want)
		}
	}

	missing := &FileMapper{Outputs: []*tes.Output{{Path: filepath.Join(work, "never-written.txt")}}}
	if err := missing.CopyOutputsToWorkDir(scratch); err == nil {
		t.Error("expected an error for an output that was not written")
	}
	bad := &FileMapper{Outputs: []*tes.Output{{Path: filepath.Join(work, "[")}}}
	if err := bad.CopyOutputsToWorkDir(scratch); err == nil {
		t.Error("expected an error for an invalid pattern")
	}
}

func TestCopyFileErrors(t *testing.T) {
	dir := t.TempDir()
	if err := copyFile(filepath.Join(dir, "missing"), filepath.Join(dir, "dst")); err == nil {
		t.Error("expected an error for a missing source")
	}
	writeFile(t, filepath.Join(dir, "src"), "x")
	if err := copyFile(filepath.Join(dir, "src"), filepath.Join(dir, "no", "such", "dst")); err == nil {
		t.Error("expected an error for a destination in a missing directory")
	}
	if err := copyDir(filepath.Join(dir, "missing"), dir); err == nil {
		t.Error("expected an error for a missing source directory")
	}
}
