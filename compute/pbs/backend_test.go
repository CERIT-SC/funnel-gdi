package pbs

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

func TestMapStates(t *testing.T) {
	fakeCommands(t, map[string]string{"qstat": `cat <<'XML'
<Data>
<Job><Job_Id>1.server</Job_Id><job_state>Q</job_state></Job>
<Job><Job_Id>2.server</Job_Id><job_state>R</job_state></Job>
<Job><Job_Id>3.server</Job_Id><job_state>C</job_state><exit_status>0</exit_status></Job>
<Job><Job_Id>4.server</Job_Id><job_state>C</job_state><exit_status>2</exit_status></Job>
<Job><Job_Id>other.server</Job_Id><job_state>R</job_state></Job>
</Data>
XML`})
	states, err := mapStates([]string{"1.server", "2.server", "3.server", "4.server"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]tes.State{"1.server": tes.Queued, "2.server": tes.Running, "3.server": tes.Complete, "4.server": tes.SystemError}
	if len(states) != 4 {
		t.Fatalf("%d states (jobs of other tasks must be ignored)", len(states))
	}
	for _, s := range states {
		if s.TESState != want[s.ID] {
			t.Errorf("job %s: %v, want %v", s.ID, s.TESState, want[s.ID])
		}
	}
	if extractID("5.server") != "5.server" {
		t.Error("extractID")
	}
}

func TestMapStatesErrors(t *testing.T) {
	for name, body := range map[string]string{"qstat fails": "exit 1", "not XML": "echo nope"} {
		t.Run(name, func(t *testing.T) {
			fakeCommands(t, map[string]string{"qstat": body})
			if _, err := mapStates([]string{"1"}); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestNewBackend(t *testing.T) {
	conf := config.DefaultConfig()
	conf.PBS.DisableReconciler = true
	conf.PBS.TemplateFile = filepath.Join(t.TempDir(), "missing")
	if _, err := NewBackend(context.Background(), conf, nil, nil, logger.NewLogger("test", logger.DefaultConfig())); err == nil {
		t.Error("expected an error for a missing template file")
	}
	conf.PBS.TemplateFile = ""
	b, err := NewBackend(context.Background(), conf, nil, nil, logger.NewLogger("test", logger.DefaultConfig()))
	if err != nil || b.SubmitCmd != "qsub" {
		t.Fatalf("%+v %v", b, err)
	}
}
