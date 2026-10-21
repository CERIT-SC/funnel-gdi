package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/ohsu-comp-bio/funnel/tes"
)

func TestGenericTaskReader(t *testing.T) {
	var views []string
	closed := false
	r := NewGenericTaskReader(func(ctx context.Context, in *tes.GetTaskRequest) (*tes.Task, error) {
		views = append(views, in.View)
		if in.Id != "t1" {
			return nil, errors.New("unknown task")
		}
		return &tes.Task{Id: "t1", State: tes.Running}, nil
	}, "t1", func() { closed = true })

	task, err := r.Task(context.Background())
	if err != nil || task.Id != "t1" {
		t.Fatalf("Task: %v %v", task, err)
	}
	state, err := r.State(context.Background())
	if err != nil || state != tes.Running {
		t.Fatalf("State: %v %v", state, err)
	}
	if views[0] != "FULL" || views[1] != "MINIMAL" {
		t.Errorf("views %v", views)
	}

	r.Close()
	if !closed {
		t.Error("Close must call the close function")
	}
	// A closed reader reports SYSTEM_ERROR without asking the server.
	if state, err := r.State(context.Background()); state != tes.SystemError || err != nil || len(views) != 2 {
		t.Errorf("after Close: %v %v (%d calls)", state, err, len(views))
	}
}

func TestResolveWildcards(t *testing.T) {
	work := t.TempDir()
	for _, f := range []string{"out/a.log", "out/b.log", "out/c.txt"} {
		writeFile(t, work+"/"+f, f)
	}
	mapper := &FileMapper{WorkDir: work, Outputs: []*tes.Output{
		{Path: work + "/out/*.log", Url: "s3://bucket/logs", PathPrefix: "/out"},
		{Path: work + "/out/c.txt", Url: "s3://bucket/c.txt"},
	}}
	if err := resolveWildcards(mapper); err != nil {
		t.Fatal(err)
	}
	var urls []string
	for _, o := range mapper.Outputs {
		urls = append(urls, o.Url)
	}
	if len(urls) != 3 || urls[0] != "s3://bucket/logs/a.log" || urls[1] != "s3://bucket/logs/b.log" || urls[2] != "s3://bucket/c.txt" {
		t.Errorf("urls %v", urls)
	}

	bad := &FileMapper{Outputs: []*tes.Output{{Path: work + "/[*"}}}
	if err := resolveWildcards(bad); err == nil {
		t.Error("expected an error for an invalid pattern")
	}
}
