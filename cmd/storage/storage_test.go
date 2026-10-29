package storage

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// run executes "funnel storage <args>" with a config that allows dir and
// returns what the command printed.
func run(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	conf := filepath.Join(dir, "funnel.yaml")
	os.WriteFile(conf, []byte("LocalStorage:\n  AllowedDirs:\n    - "+dir+"\n"), 0o644)

	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w
	out := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		out <- buf.String()
	}()

	cmd := NewCommand()
	cmd.SetArgs(append(args, "--config", conf))
	cmd.SetOut(io.Discard)
	err := cmd.Execute()

	w.Close()
	os.Stdout = stdout
	return <-out, err
}

func TestStorageCommands(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	os.WriteFile(src, []byte("hello"), 0o644)
	url := "file://" + filepath.Join(dir, "stored.txt")

	if _, err := run(t, dir, "put", src, url); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "copy.txt")
	if _, err := run(t, dir, "get", url, dest); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(dest); string(data) != "hello" {
		t.Errorf("get: %q", data)
	}

	out, err := run(t, dir, "stat", url)
	if err != nil || !strings.Contains(out, "stored.txt") || !strings.Contains(out, `"Size":5`) {
		t.Errorf("stat: %q %v", out, err)
	}
	out, err = run(t, dir, "list", "file://"+dir)
	if err != nil || !strings.Contains(out, "src.txt") || !strings.Contains(out, "copy.txt") {
		t.Errorf("list: %q %v", out, err)
	}

	taskFile := filepath.Join(dir, "task.json")
	os.WriteFile(taskFile, []byte(`{"inputs": [{"url": "`+url+`", "path": "/in"}], "outputs": [{"url": "file:///not/allowed", "path": "/out"}]}`), 0o644)
	out, err = run(t, dir, "stat-task", taskFile)
	if err != nil || !strings.Contains(out, "stored.txt") {
		t.Errorf("stat-task: %q %v", out, err)
	}
}

func TestStorageCommandErrors(t *testing.T) {
	dir := t.TempDir()
	for name, args := range map[string][]string{
		"stat outside allowed dirs": {"stat", "file:///etc/hosts"},
		"get unknown protocol":      {"get", "nope://x", filepath.Join(dir, "x")},
		"put missing file":          {"put", filepath.Join(dir, "missing"), "file://" + filepath.Join(dir, "y")},
		"list unknown protocol":     {"list", "nope://x"},
		"stat-task missing file":    {"stat-task", filepath.Join(dir, "missing.json")},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := run(t, dir, args...); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	// A wrong number of arguments prints the usage.
	for _, sub := range []string{"stat", "list", "get", "put", "stat-task"} {
		if _, err := run(t, dir, sub); err != nil {
			t.Errorf("%s without arguments: %v", sub, err)
		}
	}
	if _, err := run(t, dir, "stat", "--config", filepath.Join(dir, "missing.yaml"), "file://x"); err == nil {
		t.Error("expected an error for a missing config file")
	}
}
