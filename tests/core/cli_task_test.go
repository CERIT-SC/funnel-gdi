package core

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cmdtask "github.com/ohsu-comp-bio/funnel/cmd/task"
	"github.com/ohsu-comp-bio/funnel/tes"
	"github.com/ohsu-comp-bio/funnel/tests"
)

// The "funnel task" CLI commands against a running server.
func TestTaskCLI(t *testing.T) {
	tests.SetLogOutput(log, t)
	server := fun.Conf.Server.HTTPAddress()

	// create: from a file and from stdin
	file := filepath.Join(t.TempDir(), "task.json")
	os.WriteFile(file, []byte(`{"name": "cli-1", "executors": [{"image": "alpine", "command": ["echo", "one"]}], "tags": {"cli": "yes"}}`), 0o644)
	stdin := strings.NewReader(`{"name": "cli-2", "executors": [{"image": "alpine", "command": ["echo", "two"]}]}`)
	var out bytes.Buffer
	if err := cmdtask.Create(server, []string{file}, stdin, &out); err != nil {
		t.Fatal(err)
	}
	// Tasks from stdin come first, then those from the files.
	ids := strings.Fields(out.String())
	if len(ids) != 2 {
		t.Fatalf("created %q", out.String())
	}
	fromStdin, fromFile := ids[0], ids[1]

	// wait
	if err := cmdtask.Wait(server, ids); err != nil {
		t.Fatal(err)
	}

	// get
	out.Reset()
	if err := cmdtask.Get(server, []string{fromFile}, "full", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"COMPLETE"`) || !strings.Contains(out.String(), `"one\n"`) {
		t.Errorf("get: %s", out.String())
	}

	// list: filters by state, tag and name prefix
	out.Reset()
	if err := cmdtask.List(server, "minimal", "", "complete", []string{"cli=yes"}, "cli-", 10, true, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), fromFile) || strings.Contains(out.String(), fromStdin) {
		t.Errorf("list with tag filter: %s", out.String())
	}

	// cancel of a finished task reports that nothing was done
	out.Reset()
	if err := cmdtask.Cancel(server, ids[:1], &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "already in COMPLETE state") {
		t.Errorf("cancel: %s", out.String())
	}

	// errors
	for name, err := range map[string]error{
		"get unknown view":    cmdtask.Get(server, ids[:1], "everything", &out),
		"get unknown task":    cmdtask.Get(server, []string{"no-such-task"}, "minimal", &out),
		"list unknown state":  cmdtask.List(server, "minimal", "", "sleeping", nil, "", 10, false, &out),
		"list bad tag":        cmdtask.List(server, "minimal", "", "", []string{"no-equals-sign"}, "", 10, false, &out),
		"create invalid JSON": cmdtask.Create(server, nil, strings.NewReader("{"), &out),
		"create invalid task": cmdtask.Create(server, nil, strings.NewReader(`{"name": 1}`), &out),
		"create missing file": cmdtask.Create(server, []string{"/no/such/file.json"}, strings.NewReader(""), &out),
		"cancel unknown task": cmdtask.Cancel(server, []string{"no-such-task"}, &out),
		"wait unknown task":   cmdtask.Wait(server, []string{"no-such-task"}),
	} {
		if err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}

	// A running task can be canceled.
	id := fun.Run(`--sh 'sleep 100'`)
	fun.WaitForRunning(id)
	out.Reset()
	if err := cmdtask.Cancel(server, []string{id}, &out); err != nil {
		t.Fatal(err)
	}
	if task := fun.Wait(id); task.State != tes.Canceled {
		t.Errorf("state %s", task.State)
	}
}
