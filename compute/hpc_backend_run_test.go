package compute

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/events"
	"github.com/ohsu-comp-bio/funnel/logger"
	"github.com/ohsu-comp-bio/funnel/tes"
)

// recorder records the events written to it.
type recorder struct {
	mu     sync.Mutex
	events []*events.Event
}

func (r *recorder) WriteEvent(ctx context.Context, e *events.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	return nil
}
func (r *recorder) Close() {}

func (r *recorder) snapshot() []*events.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*events.Event(nil), r.events...)
}

// script writes an executable shell script and returns its path.
func script(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// tasksDB is a task database with fixed tasks.
type tasksDB struct{ tasks []*tes.Task }

func (d tasksDB) GetTask(ctx context.Context, r *tes.GetTaskRequest) (*tes.Task, error) {
	for _, t := range d.tasks {
		if t.Id == r.Id {
			return t, nil
		}
	}
	return nil, tes.ErrNotFound
}
func (d tasksDB) ListTasks(ctx context.Context, r *tes.ListTasksRequest) (*tes.ListTasksResponse, error) {
	var out []*tes.Task
	for _, t := range d.tasks {
		if t.State == r.State {
			out = append(out, t)
		}
	}
	return &tes.ListTasksResponse{Tasks: out}, nil
}
func (d tasksDB) Close() {}

func withBackendID(id, state string, backendID string) *tes.Task {
	s := tes.State(tes.State_value[state])
	return &tes.Task{Id: id, State: s, Logs: []*tes.TaskLog{{Metadata: map[string]string{"slurm_id": backendID}}}}
}

func newHPC(t *testing.T, submit, cancel string, db tes.ReadOnlyServer) (*HPCBackend, *recorder) {
	t.Helper()
	conf := config.DefaultConfig()
	conf.Worker.WorkDir = t.TempDir()
	rec := &recorder{}
	return &HPCBackend{
		Name:      "slurm",
		SubmitCmd: submit,
		CancelCmd: cancel,
		Template:  "#SBATCH --job-name {{.TaskId}}\n#SBATCH --cpus-per-task {{.Cpus}}\n#SBATCH --mem {{.RamGb}}G\n# zone {{.Zone}}\nfunnel worker run --taskID {{.TaskId}} {{.Args}}\n",
		Conf:      conf,
		Event:     rec,
		Database:  db,
		ExtractID: func(s string) string { return strings.TrimSpace(strings.TrimPrefix(s, "Submitted batch job ")) },
		Log:       logger.NewLogger("test", logger.DefaultConfig()),
	}, rec
}

func TestHPCSubmit(t *testing.T) {
	dir := t.TempDir()
	submitted := filepath.Join(dir, "submitted")
	submit := script(t, dir, "sbatch", `cp "$1" `+submitted+`; echo "Submitted batch job 4242"`)
	b, rec := newHPC(t, submit, "true", nil)

	task := &tes.Task{Id: "task1", Resources: &tes.Resources{CpuCores: 2, RamGb: 4, Zones: []string{"zone-a"}}}
	ctx := context.WithValue(context.Background(), "Config", b.Conf)
	if err := b.WriteEvent(ctx, events.NewTaskCreated(task)); err != nil {
		t.Fatal(err)
	}
	script, _ := os.ReadFile(submitted)
	for _, want := range []string{"--job-name task1", "--cpus-per-task 2", "--mem 4G", "zone zone-a", "--config "} {
		if !strings.Contains(string(script), want) {
			t.Errorf("submit script lacks %q:\n%s", want, script)
		}
	}
	evs := rec.snapshot()
	if len(evs) != 1 || evs[0].GetMetadata().GetValue()["slurm_id"] != "4242" {
		t.Errorf("events %v", evs)
	}
}

func TestHPCSubmitFails(t *testing.T) {
	dir := t.TempDir()
	submit := script(t, dir, "sbatch", `echo "invalid partition" >&2; exit 1`)
	b, rec := newHPC(t, submit, "true", nil)
	if err := b.Submit(context.Background(), &tes.Task{Id: "task1"}); err == nil {
		t.Fatal("expected an error")
	}
	var state tes.State
	var logged bool
	for _, e := range rec.snapshot() {
		if e.Type == events.Type_TASK_STATE {
			state = e.GetState()
		}
		if e.Type == events.Type_SYSTEM_LOG && strings.Contains(e.GetSystemLog().GetFields()["stderr"], "invalid partition") {
			logged = true
		}
	}
	if state != tes.SystemError || !logged {
		t.Errorf("state %v, stderr logged %v", state, logged)
	}

	b.Template = "{{ .Nope"
	if err := b.Submit(context.Background(), &tes.Task{Id: "task2"}); err == nil {
		t.Error("expected a template error")
	}
}

func TestHPCCancel(t *testing.T) {
	dir := t.TempDir()
	canceled := filepath.Join(dir, "canceled")
	cancel := script(t, dir, "scancel", `echo "$1" > `+canceled)
	db := tasksDB{tasks: []*tes.Task{withBackendID("task1", "RUNNING", "77"), {Id: "no-id"}}}
	b, _ := newHPC(t, "true", cancel, db)

	if err := b.WriteEvent(context.Background(), events.NewState("task1", tes.Canceled)); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(canceled); strings.TrimSpace(string(got)) != "77" {
		t.Errorf("canceled %q", got)
	}
	if err := b.Cancel(context.Background(), "no-id"); err == nil {
		t.Error("a task without a backend ID cannot be canceled")
	}
	if err := b.Cancel(context.Background(), "missing"); err == nil {
		t.Error("an unknown task cannot be canceled")
	}
	// Other state changes are ignored.
	if err := b.WriteEvent(context.Background(), events.NewState("task1", tes.Running)); err != nil {
		t.Error(err)
	}
}

func TestHPCReconcile(t *testing.T) {
	dir := t.TempDir()
	removed := filepath.Join(dir, "removed")
	cancel := script(t, dir, "scancel", `echo "$1" >> `+removed)
	db := tasksDB{tasks: []*tes.Task{
		withBackendID("queued-stuck", "QUEUED", "1"),
		withBackendID("running-ok", "RUNNING", "2"),
		{Id: "no-backend-id", State: tes.Running},
	}}
	b, rec := newHPC(t, "true", cancel, db)
	b.ReconcileRate = 5 * time.Millisecond
	b.MapStates = func(ids []string) ([]*HPCTaskState, error) {
		var out []*HPCTaskState
		for _, id := range ids {
			if id == "1" {
				out = append(out, &HPCTaskState{ID: "1", TESState: tes.SystemError, State: "PENDING", Reason: "resources can never be satisfied", Remove: true})
			} else {
				out = append(out, &HPCTaskState{ID: id, TESState: tes.Running, State: "RUNNING"})
			}
		}
		return out, nil
	}

	ctx, cancelCtx := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelCtx()
	b.Reconcile(ctx)

	failed := map[string]bool{}
	for _, e := range rec.snapshot() {
		if e.Type == events.Type_TASK_STATE && e.GetState() == tes.SystemError {
			failed[e.Id] = true
		}
	}
	if !failed["queued-stuck"] || failed["running-ok"] {
		t.Errorf("failed tasks %v", failed)
	}
	if got, _ := os.ReadFile(removed); !strings.Contains(string(got), "1") {
		t.Errorf("the stuck job should be removed from the backend: %q", got)
	}
}
