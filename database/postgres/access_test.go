package postgres

import (
	"context"
	"testing"

	"github.com/ohsu-comp-bio/funnel/events"
	"github.com/ohsu-comp-bio/funnel/logger"
	"github.com/ohsu-comp-bio/funnel/server"
	"github.com/ohsu-comp-bio/funnel/tes"
	"github.com/testcontainers/testcontainers-go"
)

func userContext(name string) context.Context {
	return context.WithValue(context.Background(), server.UserInfoKey, &server.UserInfo{Username: name})
}

// The owner is taken from the request context, and reads are limited to the
// owner with Server.TaskAccess: Owner.
func TestPostgresOwnerViewsAndMetadata(t *testing.T) {
	db, container := getTestPostgres(t)
	t.Cleanup(func() {
		db.Close()
		_ = testcontainers.TerminateContainer(container)
	})

	server.NewAuthentication(nil, nil, server.AccessOwner, logger.NewLogger("test", logger.DefaultConfig()))
	t.Cleanup(func() {
		server.NewAuthentication(nil, nil, server.AccessAll, logger.NewLogger("test", logger.DefaultConfig()))
	})

	alice, bob := userContext("alice"), userContext("bob")
	if err := db.WriteEvent(alice, events.NewTaskCreated(newTestTask("task-a", "Alice's task", "alice"))); err != nil {
		t.Fatal(err)
	}

	if _, err := db.GetTask(bob, &tes.GetTaskRequest{Id: "task-a"}); err != tes.ErrNotPermitted {
		t.Errorf("GetTask by another user: got %v, want ErrNotPermitted", err)
	}
	if list, err := db.ListTasks(bob, &tes.ListTasksRequest{}); err != nil || len(list.Tasks) != 0 {
		t.Errorf("ListTasks by another user: %v %v", list, err)
	}
	if list, err := db.ListTasks(alice, &tes.ListTasksRequest{}); err != nil || len(list.Tasks) != 1 {
		t.Errorf("ListTasks by the owner: %v %v", list, err)
	}

	// No view is the MINIMAL view: only the ID and the state.
	task, err := db.GetTask(alice, &tes.GetTaskRequest{Id: "task-a"})
	if err != nil {
		t.Fatal(err)
	}
	if task.Id != "task-a" || task.State != tes.State_QUEUED || task.Name != "" || task.Executors != nil {
		t.Errorf("minimal view %v", task)
	}
	if _, err := db.GetTask(alice, &tes.GetTaskRequest{Id: "task-a", View: "BOGUS"}); err == nil {
		t.Error("an unknown view must fail")
	}

	// Metadata events are merged; values are JSON-escaped.
	for _, md := range []map[string]string{{"one": "two"}, {"quote": `say "hi"`}} {
		if err := db.WriteEvent(alice, events.NewMetadata("task-a", 0, md)); err != nil {
			t.Fatal(err)
		}
	}
	task, err = db.GetTask(alice, &tes.GetTaskRequest{Id: "task-a", View: tes.View_FULL.String()})
	if err != nil {
		t.Fatal(err)
	}
	if md := task.Logs[0].Metadata; len(md) != 2 || md["one"] != "two" || md["quote"] != `say "hi"` {
		t.Errorf("metadata %v", md)
	}
}
