package slurm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/logger"
	"github.com/ohsu-comp-bio/funnel/tes"
)

// fakeCommands puts shell scripts with the given names first on PATH.
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

func TestExtractID(t *testing.T) {
	if got := extractID("Submitted batch job 2\n"); got != "2" {
		t.Errorf("got %q", got)
	}
}

func TestMapStates(t *testing.T) {
	fakeCommands(t, map[string]string{
		"squeue": `printf '1 PENDING Priority\n2 PENDING PartitionConfig\n3 RUNNING None\n'`,
		"sacct":  `printf '4 COMPLETED\n5 FAILED\n'`,
	})
	states, err := mapStates([]string{"1", "2", "3", "4", "5"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]tes.State{"1": tes.Queued, "2": tes.SystemError, "3": tes.Running, "4": tes.Complete, "5": tes.SystemError}
	for _, s := range states {
		if s.TESState != want[s.ID] {
			t.Errorf("job %s: %v, want %v", s.ID, s.TESState, want[s.ID])
		}
		if s.ID == "2" && (!s.Remove || s.Reason == "") {
			t.Errorf("a job without a suitable partition must be removed with a reason: %+v", s)
		}
	}
	if len(states) != 5 {
		t.Errorf("%d states", len(states))
	}
}

func TestMapStatesErrors(t *testing.T) {
	for name, scripts := range map[string]map[string]string{
		"squeue fails":      {"squeue": "exit 1", "sacct": "true"},
		"squeue unparsable": {"squeue": "echo 'one two'", "sacct": "true"},
		"sacct fails":       {"squeue": "true", "sacct": "exit 1"},
		"sacct unparsable":  {"squeue": "true", "sacct": "echo 'a b c'"},
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
	tpl := filepath.Join(t.TempDir(), "slurm.tpl")
	os.WriteFile(tpl, []byte("#SBATCH {{.TaskId}}"), 0o644)
	conf := config.DefaultConfig()
	conf.Slurm.TemplateFile = tpl
	conf.Slurm.DisableReconciler = true
	b, err := NewBackend(context.Background(), conf, nil, nil, logger.NewLogger("test", logger.DefaultConfig()))
	if err != nil || b.Template != "#SBATCH {{.TaskId}}" || b.SubmitCmd != "sbatch" {
		t.Fatalf("%+v %v", b, err)
	}
	conf.Slurm.TemplateFile = filepath.Join(t.TempDir(), "missing")
	if _, err := NewBackend(context.Background(), conf, nil, nil, nil); err == nil {
		t.Error("expected an error for a missing template file")
	}
}
