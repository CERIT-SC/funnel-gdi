package core

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ohsu-comp-bio/funnel/tes"
	"github.com/ohsu-comp-bio/funnel/tests"
)

// stdioTask reads its stdin from an input and writes stdout and stderr to
// files that are uploaded as outputs.
func stdioTask(storageDir string) *tes.Task {
	storageDir, _ = filepath.Abs(storageDir)
	return &tes.Task{
		Inputs: []*tes.Input{{Content: "hello stdin\n", Path: "/inputs/in.txt"}},
		Outputs: []*tes.Output{
			{Url: "file://" + storageDir + "/stdout.txt", Path: "/outputs/stdout.txt"},
			{Url: "file://" + storageDir + "/stderr.txt", Path: "/outputs/stderr.txt"},
		},
		Executors: []*tes.Executor{{
			Image:   "alpine",
			Command: []string{"sh", "-c", "cat; echo to-stderr >&2"},
			Stdin:   "/inputs/in.txt",
			Stdout:  "/outputs/stdout.txt",
			Stderr:  "/outputs/stderr.txt",
		}},
	}
}

func TestExecutorStdioFiles(t *testing.T) {
	tests.SetLogOutput(log, t)
	id, err := fun.RunTask(stdioTask(fun.StorageDir))
	if err != nil {
		t.Fatal(err)
	}
	task := fun.Wait(id)
	if task.State != tes.State_COMPLETE {
		t.Fatalf("state %s: %v", task.State, task.Logs)
	}
	if got := fun.ReadFile("stdout.txt"); got != "hello stdin\n" {
		t.Errorf("stdout file %q", got)
	}
	if got := fun.ReadFile("stderr.txt"); got != "to-stderr\n" {
		t.Errorf("stderr file %q", got)
	}
}

// An output to a storage the server does not support fails the task before
// it runs.
func TestUnsupportedOutputStorage(t *testing.T) {
	tests.SetLogOutput(log, t)
	id, err := fun.RunTask(&tes.Task{
		Outputs:   []*tes.Output{{Url: "gs://bucket/out.txt", Path: "/outputs/out.txt"}},
		Executors: []*tes.Executor{{Image: "alpine", Command: []string{"touch", "/outputs/out.txt"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	task := fun.Wait(id)
	if task.State != tes.State_SYSTEM_ERROR {
		t.Fatalf("state %s, want SYSTEM_ERROR", task.State)
	}
	if logs := strings.Join(task.Logs[0].SystemLogs, "\n"); !strings.Contains(logs, "not supported") {
		t.Errorf("system logs do not explain the error:\n%s", logs)
	}
}
