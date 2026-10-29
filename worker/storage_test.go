package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/go-test/deep"
	"github.com/ohsu-comp-bio/funnel/events"
	"github.com/ohsu-comp-bio/funnel/storage"
	"github.com/ohsu-comp-bio/funnel/tes"
)

type fakelist struct {
	list []*storage.Object
	storage.Fake
}

func (f fakelist) List(context.Context, string) ([]*storage.Object, error) {
	return f.list, nil
}

func TestFlattenInputs(t *testing.T) {
	ev := events.NewTaskWriter("task-1", 0, events.Noop{})
	fake := fakelist{
		list: []*storage.Object{
			{URL: "s3://bkt/path/to/one.txt", Name: "path/to/one.txt"},
			{URL: "s3://bkt/path/to/two.txt", Name: "path/to/two.txt"},
		},
	}

	in := []*tes.Input{
		{Url: "s3://bkt/path", Path: "/inputs/foo", Type: tes.Directory},
		{Url: "s3://bkt/path", Path: "/inputs/foo/", Type: tes.Directory},
		{Url: "s3://bkt/path/", Path: "/inputs/foo", Type: tes.Directory},
		{Url: "s3://bkt/path/", Path: "/inputs/foo/", Type: tes.Directory},
	}

	bg := context.Background()
	flat, err := FlattenInputs(bg, in, fake, ev)
	if err != nil {
		t.Fatal(err)
	}

	expected := []*tes.Input{
		{Url: "s3://bkt/path/to/one.txt", Path: "/inputs/foo/to/one.txt"},
		{Url: "s3://bkt/path/to/two.txt", Path: "/inputs/foo/to/two.txt"},
		{Url: "s3://bkt/path/to/one.txt", Path: "/inputs/foo/to/one.txt"},
		{Url: "s3://bkt/path/to/two.txt", Path: "/inputs/foo/to/two.txt"},
		{Url: "s3://bkt/path/to/one.txt", Path: "/inputs/foo/to/one.txt"},
		{Url: "s3://bkt/path/to/two.txt", Path: "/inputs/foo/to/two.txt"},
		{Url: "s3://bkt/path/to/one.txt", Path: "/inputs/foo/to/one.txt"},
		{Url: "s3://bkt/path/to/two.txt", Path: "/inputs/foo/to/two.txt"},
	}

	for _, diff := range deep.Equal(flat, expected) {
		t.Error(diff)
	}
}

// eventRecorder keeps the events written to it.
type eventRecorder struct{ events []*events.Event }

func (r *eventRecorder) WriteEvent(_ context.Context, ev *events.Event) error {
	r.events = append(r.events, ev)
	return nil
}
func (r *eventRecorder) Close() {}

// A failed transfer writes a system log with an error code.
func TestTransferFailedSystemLog(t *testing.T) {
	rec := &eventRecorder{}
	ev := events.NewTaskWriter("task", 0, rec)
	d := &download{ev: ev, in: &tes.Input{Url: "s3://bucket/in"}, cancel: func() {}}
	d.Failed(errors.New("access denied"))
	u := &upload{ev: ev, out: &tes.Output{Url: "s3://bucket/out"}}
	u.Failed(errors.New("access denied"))

	want := []string{"FUNNEL-RUN-012: download failed", "FUNNEL-RUN-013: upload failed"}
	if len(rec.events) != len(want) {
		t.Fatalf("got %d events, want %d", len(rec.events), len(want))
	}
	for i, e := range rec.events {
		log := e.GetSystemLog()
		if log.GetMsg() != want[i] || log.GetLevel() != "error" || log.GetFields()["error"] != "access denied" {
			t.Errorf("system log %d: %v", i, log)
		}
	}
}
