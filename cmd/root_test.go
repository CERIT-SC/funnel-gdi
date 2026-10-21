package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// execute runs the funnel CLI with args and returns what it printed.
func execute(t *testing.T, args ...string) (string, error) {
	t.Helper()
	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w
	out := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		out <- buf.String()
	}()
	RootCmd.SetArgs(args)
	err := RootCmd.Execute()
	w.Close()
	os.Stdout = stdout
	return <-out, err
}

func TestExamplesCommand(t *testing.T) {
	list, err := execute(t, "examples")
	if err != nil || !strings.Contains(list, "hello-world") || !strings.Contains(list, "default-config") {
		t.Fatalf("list: %q %v", list, err)
	}
	example, err := execute(t, "examples", "hello-world")
	if err != nil || !strings.Contains(example, `"executors"`) {
		t.Errorf("hello-world: %q %v", example, err)
	}
	if _, err := execute(t, "examples", "no-such-example"); err == nil {
		t.Error("expected an error for an unknown example")
	}
}

func TestCompletionAndDocs(t *testing.T) {
	out, err := execute(t, "completion", "bash")
	if err != nil || !strings.Contains(out, "funnel") {
		t.Errorf("completion: %v", err)
	}

	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	dir := t.TempDir()
	os.Chdir(dir)
	os.Mkdir("funnel-cmd-docs", 0o755)
	if _, err := execute(t, "genmarkdown"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "funnel-cmd-docs", "funnel_task_create.md")); err != nil {
		t.Errorf("markdown docs not generated: %v", err)
	}
}
