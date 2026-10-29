package badger

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/events"
	"github.com/ohsu-comp-bio/funnel/server"
	"github.com/ohsu-comp-bio/funnel/tes"
)

func newTestDB(t *testing.T) *Badger {
	t.Helper()
	db, err := NewBadger(&config.Badger{Path: filepath.Join(t.TempDir(), "badger")})
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

func create(t *testing.T, db *Badger, ctx context.Context, task *tes.Task) {
	t.Helper()
	if err := db.WriteEvent(ctx, events.NewTaskCreated(task)); err != nil {
		t.Fatal(err)
	}
}

func TestTaskEvents(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	create(t, db, ctx, &tes.Task{Id: "t1", State: tes.Queued, Inputs: []*tes.Input{{Content: "secret", Path: "/in"}}})

	now := time.Now()
	for _, ev := range []*events.Event{
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
		len(log.Outputs) != 1 || len(log.SystemLogs) != 1 || log.StartTime == "" || log.EndTime == "" || log.Logs[0].EndTime == "" {
		t.Errorf("full view %v", full)
	}
	basic, _ := db.GetTask(ctx, &tes.GetTaskRequest{Id: "t1", View: tes.View_BASIC.String()})
	if basic.Inputs[0].Content != "" || basic.Logs[0].Logs[0].Stdout != "" {
		t.Errorf("basic view %v", basic)
	}
	minimal, _ := db.GetTask(ctx, &tes.GetTaskRequest{Id: "t1", View: tes.View_MINIMAL.String()})
	if minimal.State != tes.Complete || minimal.Inputs != nil {
		t.Errorf("minimal view %v", minimal)
	}

	// A finished task cannot change state, and the error is not retried.
	start := time.Now()
	var te *tes.TransitionError
	if err := db.WriteEvent(ctx, events.NewState("t1", tes.Running)); !errors.As(err, &te) {
		t.Errorf("got %v, want a TransitionError", err)
	}
	if time.Since(start) > time.Second {
		t.Error("a transition error must not be retried")
	}
	if err := db.WriteEvent(ctx, events.NewState("missing", tes.Running)); err != tes.ErrNotFound {
		t.Errorf("unknown task: %v", err)
	}
	if _, err := db.GetTask(ctx, &tes.GetTaskRequest{Id: "missing"}); err != tes.ErrNotFound {
		t.Errorf("unknown task: %v", err)
	}
}

func TestListTasks(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	for _, task := range []*tes.Task{
		{Id: "a", State: tes.Queued, Tags: map[string]string{"project": "x"}},
		{Id: "b", State: tes.Running, Tags: map[string]string{"project": "y"}},
		{Id: "c", State: tes.Queued, Tags: map[string]string{"project": "x"}},
	} {
		create(t, db, ctx, task)
	}

	all, err := db.ListTasks(ctx, &tes.ListTasksRequest{View: tes.View_MINIMAL.String()})
	if err != nil || len(all.Tasks) != 3 || all.Tasks[0].Id != "c" || all.NextPageToken != "" {
		t.Fatalf("all (newest first): %v %v", all, err)
	}

	// Pages of two: c, b then a.
	page1, _ := db.ListTasks(ctx, &tes.ListTasksRequest{PageSize: 2})
	page2, _ := db.ListTasks(ctx, &tes.ListTasksRequest{PageSize: 2, PageToken: page1.NextPageToken})
	if len(page1.Tasks) != 2 || page1.NextPageToken != "b" || len(page2.Tasks) != 1 || page2.Tasks[0].Id != "a" {
		t.Errorf("pages %v / %v", page1, page2)
	}

	queued, _ := db.ListTasks(ctx, &tes.ListTasksRequest{State: tes.Queued, View: tes.View_BASIC.String()})
	tagged, _ := db.ListTasks(ctx, &tes.ListTasksRequest{TagKey: []string{"project"}, TagValue: []string{"y"}})
	if len(queued.Tasks) != 2 || len(tagged.Tasks) != 1 || tagged.Tasks[0].Id != "b" {
		t.Errorf("filters: queued %v tagged %v", queued, tagged)
	}
}

// With Server.TaskAccess "Owner", users see only their own tasks.
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
