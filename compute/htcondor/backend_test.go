package htcondor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/logger"
	"github.com/ohsu-comp-bio/funnel/tes"
)

func fakeCommands(t *testing.T, scripts map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// The example from the condor_submit documentation. Multi-digit cluster IDs
// are not covered: extractID keeps only their last digit (reported).
func TestExtractID(t *testing.T) {
	if got := extractID("Submitting job(s).\n1 job(s) submitted to cluster 1.\n"); got != "1" {
		t.Errorf("got %q", got)
	}
}

func TestMapStates(t *testing.T) {
	// JobStatus: 1 Idle, 2 Running, 3 Removed, 4 Completed, 5 Held, 6 Transferring, 7 Suspended
	fakeCommands(t, map[string]string{
		"condor_q": `if [ "$1" = "-analyze" ]; then
  if [ "$2" = "2" ]; then echo "No machines matched the jobs's constraints"; fi
  exit 0
fi
echo '[{"ClusterId":1,"JobStatus":1},{"ClusterId":2,"JobStatus":1},{"ClusterId":3,"JobStatus":2},{"ClusterId":4,"JobStatus":5}]'`,
		"condor_history": `echo '[{"ClusterId":5,"JobStatus":3},{"ClusterId":6,"JobStatus":4,"ExitCode":0},{"ClusterId":7,"JobStatus":4,"ExitCode":1}]'`,
	})
	states, err := mapStates([]string{"1", "2", "3", "4", "5", "6", "7"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]tes.State{"1": tes.Queued, "2": tes.SystemError, "3": tes.Running, "4": tes.Running,
		"5": tes.SystemError, "6": tes.Complete, "7": tes.SystemError}
	got := map[string]tes.State{}
	for _, s := range states {
		got[s.ID] = s.TESState
		if s.ID == "2" && !s.Remove {
			t.Error("a job no machine can run must be removed")
		}
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("job %s: %v, want %v", id, got[id], w)
		}
	}
}

func TestMapStatesErrors(t *testing.T) {
	for name, scripts := range map[string]map[string]string{
		"condor_q fails":    {"condor_q": "exit 1", "condor_history": "echo []"},
		"condor_q not JSON": {"condor_q": "echo nope", "condor_history": "echo []"},
		"history fails":     {"condor_q": "echo []", "condor_history": "exit 1"},
		"history not JSON":  {"condor_q": "echo []", "condor_history": "echo nope"},
	} {
		t.Run(name, func(t *testing.T) {
			fakeCommands(t, scripts)
			if _, err := mapStates([]string{"1"}); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestNewBackend(t *testing.T) {
	conf := config.DefaultConfig()
	conf.HTCondor.DisableReconciler = true
	b, err := NewBackend(context.Background(), conf, nil, nil, logger.NewLogger("test", logger.DefaultConfig()))
	if err != nil || b.SubmitCmd != "condor_submit" {
		t.Fatalf("%+v %v", b, err)
	}
	conf.HTCondor.TemplateFile = filepath.Join(t.TempDir(), "missing")
	if _, err := NewBackend(context.Background(), conf, nil, nil, nil); err == nil {
		t.Error("expected an error for a missing template file")
	}
}
