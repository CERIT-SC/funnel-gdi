package boltdb

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ohsu-comp-bio/funnel/compute/scheduler"
	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/events"
	"github.com/ohsu-comp-bio/funnel/server"
	"github.com/ohsu-comp-bio/funnel/tes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestDB(t *testing.T) *BoltDB {
	t.Helper()
	db, err := NewBoltDB(&config.BoltDB{Path: filepath.Join(t.TempDir(), "funnel.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return db
}

func asUser(name string) context.Context {
	return context.WithValue(context.Background(), server.UserInfoKey, &server.UserInfo{Username: name})
}

func create(t *testing.T, db *BoltDB, ctx context.Context, task *tes.Task) {
	t.Helper()
	if err := db.WriteEvent(ctx, events.NewTaskCreated(task)); err != nil {
		t.Fatal(err)
	}
}

func TestTaskEvents(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	// Executor logs are stored per executor of the task.
	create(t, db, ctx, &tes.Task{Id: "t1", State: tes.Queued, Inputs: []*tes.Input{{Content: "secret", Path: "/in"}},
		Executors: []*tes.Executor{{Image: "alpine", Command: []string{"true"}}}})

	now := time.Now()
	for _, ev := range []*events.Event{
		events.NewState("t1", tes.Initializing),
		events.NewState("t1", tes.Running),
		events.NewStartTime("t1", 0, now),
		events.NewMetadata("t1", 0, map[string]string{"host": "node-1"}),
		events.NewExecutorStartTime("t1", 0, 0, now),
		events.NewStdout("t1", 0, 0, "out"),
		events.NewStderr("t1", 0, 0, "err"),
		events.NewExitCode("t1", 0, 0, 0),
		events.NewExecutorEndTime("t1", 0, 0, now),
		events.NewOutputs("t1", 0, []*tes.OutputFileLog{{Path: "/out", Url: "file:///out", SizeBytes: "3"}}),
		events.NewSystemLog("t1", 0, 0, "info", "hello", map[string]string{"k": "v"}),
		events.NewEndTime("t1", 0, now),
		events.NewState("t1", tes.Complete),
	} {
		if err := db.WriteEvent(ctx, ev); err != nil {
			t.Fatalf("%s: %v", ev.Type, err)
		}
	}

	full, err := db.GetTask(ctx, &tes.GetTaskRequest{Id: "t1", View: tes.View_FULL.String()})
	if err != nil {
		t.Fatal(err)
	}
	log := full.Logs[0]
	if full.State != tes.Complete || log.Metadata["host"] != "node-1" || log.Logs[0].Stdout != "out" || log.Logs[0].Stderr != "err" ||
		len(log.Outputs) != 1 || len(log.SystemLogs) != 1 || log.StartTime == "" || log.EndTime == "" {
		t.Errorf("full view %v", full)
	}
	basic, err := db.GetTask(ctx, &tes.GetTaskRequest{Id: "t1", View: tes.View_BASIC.String()})
	if err != nil || len(basic.Inputs) != 1 || basic.Inputs[0].Content != "" || basic.Inputs[0].Path != "/in" {
		t.Errorf("basic view %v %v", basic, err)
	}
	for _, tl := range basic.Logs {
		for _, el := range tl.Logs {
			if el.Stdout != "" || el.Stderr != "" {
				t.Errorf("basic view has executor output: %v", el)
			}
		}
	}

	// A finished task cannot change state.
	if err := db.WriteEvent(ctx, events.NewState("t1", tes.Running)); err == nil {
		t.Error("expected an error for COMPLETE -> RUNNING")
	}
	if got, _ := db.GetTask(ctx, &tes.GetTaskRequest{Id: "t1", View: tes.View_MINIMAL.String()}); got.State != tes.Complete {
		t.Errorf("state changed to %s", got.State)
	}
	if err := db.WriteEvent(ctx, events.NewState("missing", tes.Running)); err != tes.ErrNotFound {
		t.Errorf("unknown task: %v", err)
	}
	if _, err := db.GetTask(ctx, &tes.GetTaskRequest{Id: "missing"}); err != tes.ErrNotFound {
		t.Errorf("unknown task: %v", err)
	}
}

func TestQueue(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	for _, id := range []string{"q1", "q2", "q3"} {
		create(t, db, ctx, &tes.Task{Id: id, State: tes.Queued})
	}
	if got := db.ReadQueue(2); len(got) != 2 {
		t.Errorf("ReadQueue(2): %d tasks", len(got))
	}
	// A task that leaves QUEUED leaves the queue.
	if err := db.WriteEvent(ctx, events.NewState("q1", tes.Initializing)); err != nil {
		t.Fatal(err)
	}
	for _, task := range db.ReadQueue(10) {
		if task.Id == "q1" {
			t.Error("an initializing task is still queued")
		}
	}
}

func TestNodes(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	create(t, db, ctx, &tes.Task{Id: "t1", State: tes.Running, Resources: &tes.Resources{CpuCores: 1}})

	node := &scheduler.Node{Id: "n1", State: scheduler.NodeState_ALIVE, TaskIds: []string{"t1"}, Resources: &scheduler.Resources{Cpus: 4}}
	if _, err := db.PutNode(ctx, node); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetNode(ctx, &scheduler.GetNodeRequest{Id: "n1"})
	if err != nil || got.Available.GetCpus() != 3 || got.LastPing == 0 {
		t.Fatalf("node %v %v", got, err)
	}

	// Optimistic locking: an update must carry the stored version.
	got.Version = 1
	if _, err := db.PutNode(ctx, got); err != nil {
		t.Fatal(err)
	}
	stale := &scheduler.Node{Id: "n1", Version: 99}
	if _, err := db.PutNode(ctx, stale); err == nil {
		t.Error("a stale version must be rejected")
	}

	list, err := db.ListNodes(ctx, &scheduler.ListNodesRequest{})
	if err != nil || len(list.Nodes) != 1 {
		t.Errorf("list %v %v", list, err)
	}
	if _, err := db.DeleteNode(ctx, got); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetNode(ctx, &scheduler.GetNodeRequest{Id: "n1"}); status.Code(err) != codes.NotFound {
		t.Errorf("deleted node: %v", err)
	}
}

func TestTaskOwners(t *testing.T) {
	server.NewAuthentication(nil, nil, server.AccessOwner, nil)
	defer server.NewAuthentication(nil, nil, server.AccessAll, nil)

	db := newTestDB(t)
	create(t, db, asUser("alice"), &tes.Task{Id: "alices", State: tes.Queued})
	create(t, db, asUser("bob"), &tes.Task{Id: "bobs", State: tes.Queued})

	if _, err := db.GetTask(asUser("bob"), &tes.GetTaskRequest{Id: "alices"}); err != tes.ErrNotPermitted {
		t.Errorf("bob reads alice's task: %v", err)
	}
	if err := db.WriteEvent(asUser("bob"), events.NewState("alices", tes.Canceled)); err != tes.ErrNotPermitted {
		t.Errorf("bob cancels alice's task: %v", err)
	}
	list, _ := db.ListTasks(asUser("bob"), &tes.ListTasksRequest{})
	if len(list.Tasks) != 1 || list.Tasks[0].Id != "bobs" {
		t.Errorf("bob lists %v", list.Tasks)
	}
}
