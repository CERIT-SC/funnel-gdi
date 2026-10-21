package gcp_batch

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	batch "cloud.google.com/go/batch/apiv1"
	"cloud.google.com/go/batch/apiv1/batchpb"
	logadmin "cloud.google.com/go/logging/apiv2"
	"cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/events"
	"github.com/ohsu-comp-bio/funnel/logger"
	"github.com/ohsu-comp-bio/funnel/tes"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fakeGCP is an in-memory GCP Batch and Cloud Logging service.
type fakeGCP struct {
	batchpb.UnimplementedBatchServiceServer
	loggingpb.UnimplementedLoggingServiceV2Server
	jobs []*batchpb.Job
	logs map[string][]*loggingpb.LogEntry // by job UID
}

func (f *fakeGCP) ListJobs(ctx context.Context, req *batchpb.ListJobsRequest) (*batchpb.ListJobsResponse, error) {
	return &batchpb.ListJobsResponse{Jobs: f.jobs}, nil
}

func (f *fakeGCP) ListLogEntries(ctx context.Context, req *loggingpb.ListLogEntriesRequest) (*loggingpb.ListLogEntriesResponse, error) {
	for uid, entries := range f.logs {
		if req.Filter == `labels.job_uid="`+uid+`"` {
			return &loggingpb.ListLogEntriesResponse{Entries: entries}, nil
		}
	}
	return &loggingpb.ListLogEntriesResponse{}, nil
}

func startFakeGCP(t *testing.T, f *fakeGCP) (*batch.Client, *logadmin.Client) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	batchpb.RegisterBatchServiceServer(srv, f)
	loggingpb.RegisterLoggingServiceV2Server(srv, f)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	ctx := context.Background()
	bc, err := batch.NewClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	lc, err := logadmin.NewClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	return bc, lc
}

type gcpEvents struct {
	mu     sync.Mutex
	events []*events.Event
}

func (e *gcpEvents) WriteEvent(ctx context.Context, ev *events.Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, ev)
	return nil
}
func (e *gcpEvents) Close() {}

func (e *gcpEvents) states(id string) []tes.State {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []tes.State
	for _, ev := range e.events {
		if ev.Id == id && ev.Type == events.Type_TASK_STATE {
			out = append(out, ev.GetState())
		}
	}
	return out
}

func (e *gcpEvents) logMessages(id string) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for _, ev := range e.events {
		if ev.Id == id && ev.Type == events.Type_SYSTEM_LOG {
			out = append(out, ev.GetSystemLog().Msg)
		}
	}
	return out
}

type gcpTasks []*tes.Task

func (l gcpTasks) GetTask(ctx context.Context, r *tes.GetTaskRequest) (*tes.Task, error) {
	return nil, tes.ErrNotFound
}
func (l gcpTasks) ListTasks(ctx context.Context, r *tes.ListTasksRequest) (*tes.ListTasksResponse, error) {
	var out []*tes.Task
	for _, t := range l {
		if t.State == r.State {
			out = append(out, t)
		}
	}
	return &tes.ListTasksResponse{Tasks: out}, nil
}
func (l gcpTasks) Close() {}

func job(task string, state batchpb.JobStatus_State, events ...string) *batchpb.Job {
	status := &batchpb.JobStatus{State: state}
	for _, d := range events {
		status.StatusEvents = append(status.StatusEvents, &batchpb.StatusEvent{Description: d})
	}
	return &batchpb.Job{Name: "projects/p/locations/l/jobs/" + task, Uid: "uid-" + task, Status: status}
}

func TestGCPReconcile(t *testing.T) {
	fake := &fakeGCP{
		jobs: []*batchpb.Job{
			job("failed", batchpb.JobStatus_FAILED, "", "Task failed: exit code 1"),
			job("failed-no-reason", batchpb.JobStatus_FAILED),
			job("done", batchpb.JobStatus_SUCCEEDED),
			job("started", batchpb.JobStatus_RUNNING),
			job("not-ours", batchpb.JobStatus_FAILED),
		},
		logs: map[string][]*loggingpb.LogEntry{
			"uid-done": {
				{Payload: &loggingpb.LogEntry_TextPayload{TextPayload: "hello from the task"}, Timestamp: timestamppb.Now(),
					Resource: &monitoredres.MonitoredResource{Type: "batch.googleapis.com/Job", Labels: map[string]string{"job_id": "1"}},
					Labels:   map[string]string{"job_uid": "uid-done"}},
			},
		},
	}
	bc, lc := startFakeGCP(t, fake)
	ev := &gcpEvents{}
	b := &Backend{
		client:             bc,
		loggingAdminClient: lc,
		conf:               &config.GCPBatch{Project: "p", Location: "l", ReconcileRate: durationpb.New(10 * time.Millisecond)},
		event:              ev,
		database:           gcpTasks{{Id: "failed", State: tes.Running}, {Id: "failed-no-reason", State: tes.Queued}, {Id: "done", State: tes.Running}, {Id: "started", State: tes.Queued}},
		log:                logger.NewLogger("test", logger.DefaultConfig()),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	b.reconcile(ctx)

	for id, want := range map[string]tes.State{"failed": tes.ExecutorError, "failed-no-reason": tes.ExecutorError, "done": tes.Complete, "started": tes.Running} {
		if st := ev.states(id); len(st) == 0 || st[0] != want {
			t.Errorf("%s: states %v, want %v", id, st, want)
		}
	}
	if st := ev.states("not-ours"); len(st) != 0 {
		t.Errorf("a job without a Funnel task must be ignored: %v", st)
	}
	if msgs := ev.logMessages("failed"); !contains(msgs, "Task failed: exit code 1") {
		t.Errorf("failure reason not logged: %v", msgs)
	}
	if msgs := ev.logMessages("failed-no-reason"); !contains(msgs, "GCP Batch job in FAILED state") {
		t.Errorf("default failure reason not logged: %v", msgs)
	}
	if msgs := ev.logMessages("done"); !contains(msgs, "hello from the task") {
		t.Errorf("Cloud Logging entries not copied to the task: %v", msgs)
	}
}

func TestGCPFetchLogs(t *testing.T) {
	fake := &fakeGCP{logs: map[string][]*loggingpb.LogEntry{"uid-1": {
		{Payload: &loggingpb.LogEntry_TextPayload{TextPayload: "line"}, Timestamp: timestamppb.Now(), Severity: 500},
	}}}
	_, lc := startFakeGCP(t, fake)
	b := &Backend{loggingAdminClient: lc, conf: &config.GCPBatch{Project: "p"}, log: logger.NewLogger("test", logger.DefaultConfig())}

	logs, err := b.fetchLogs(context.Background(), "t1", "uid-1")
	if err != nil || len(logs) != 1 || logs[0].Msg != "line" || logs[0].Level != "ERROR" || logs[0].Fields["gcpbatch_job_id"] != "uid-1" {
		t.Fatalf("%v %v", logs, err)
	}
	if _, err := b.fetchLogs(context.Background(), "t2", "uid-none"); err == nil {
		t.Error("expected an error when there are no logs")
	}
	if _, err := (&Backend{}).fetchLogs(context.Background(), "t", "u"); err == nil {
		t.Error("expected an error without a logging client")
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
