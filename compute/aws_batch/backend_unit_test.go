package aws_batch

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/batch"
	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/events"
	"github.com/ohsu-comp-bio/funnel/logger"
	"github.com/ohsu-comp-bio/funnel/tes"
	"google.golang.org/protobuf/types/known/durationpb"
)

// batchFake records submit and cancel requests and answers DescribeJobs.
type batchFake struct {
	mu        sync.Mutex
	submitted *batch.SubmitJobInput
	canceled  []string
	submitErr error
	statuses  map[string]string
}

func (f *batchFake) SubmitJob(in *batch.SubmitJobInput) (*batch.SubmitJobOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submitted = in
	if f.submitErr != nil {
		return nil, f.submitErr
	}
	return &batch.SubmitJobOutput{JobId: aws.String("aws-1")}, nil
}

func (f *batchFake) CancelJob(in *batch.CancelJobInput) (*batch.CancelJobOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.canceled = append(f.canceled, *in.JobId)
	return &batch.CancelJobOutput{}, nil
}

func (f *batchFake) DescribeJobsWithContext(ctx context.Context, in *batch.DescribeJobsInput, opts ...request.Option) (*batch.DescribeJobsOutput, error) {
	var out batch.DescribeJobsOutput
	for _, id := range in.Jobs {
		out.Jobs = append(out.Jobs, &batch.JobDetail{JobId: id, Status: aws.String(f.statuses[*id]), StatusReason: aws.String("essential container exited")})
	}
	return &out, nil
}

type eventLog struct {
	mu     sync.Mutex
	events []*events.Event
}

func (e *eventLog) WriteEvent(ctx context.Context, ev *events.Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, ev)
	return nil
}
func (e *eventLog) Close() {}

func (e *eventLog) systemError(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, ev := range e.events {
		if ev.Id == id && ev.Type == events.Type_TASK_STATE && ev.GetState() == tes.SystemError {
			return true
		}
	}
	return false
}

type taskList []*tes.Task

func (l taskList) GetTask(ctx context.Context, r *tes.GetTaskRequest) (*tes.Task, error) {
	for _, t := range l {
		if t.Id == r.Id {
			return t, nil
		}
	}
	return nil, tes.ErrNotFound
}
func (l taskList) ListTasks(ctx context.Context, r *tes.ListTasksRequest) (*tes.ListTasksResponse, error) {
	var out []*tes.Task
	for _, t := range l {
		if t.State == r.State {
			out = append(out, t)
		}
	}
	return &tes.ListTasksResponse{Tasks: out}, nil
}
func (l taskList) Close() {}

func withAWSID(id string, state tes.State, awsID string) *tes.Task {
	return &tes.Task{Id: id, State: state, Logs: []*tes.TaskLog{{Metadata: map[string]string{"awsbatch_id": awsID}}}}
}

func newAWSBackend(fake *batchFake, tasks taskList) (*Backend, *eventLog) {
	ev := &eventLog{}
	return &Backend{
		client:   fake,
		conf:     &config.AWSBatch{JobDefinition: "def", JobQueue: "queue", ReconcileRate: durationpb.New(5 * time.Millisecond)},
		event:    ev,
		database: tasks,
		log:      logger.NewLogger("test", logger.DefaultConfig()),
	}, ev
}

func TestAWSSubmit(t *testing.T) {
	fake := &batchFake{}
	b, ev := newAWSBackend(fake, nil)
	task := &tes.Task{Id: "t1", Name: "my task/1", Resources: &tes.Resources{RamGb: 2, CpuCores: 4}}
	if err := b.WriteEvent(context.Background(), events.NewTaskCreated(task)); err != nil {
		t.Fatal(err)
	}
	in := fake.submitted
	if *in.JobName != "my_task_1" || *in.Parameters["taskID"] != "t1" || *in.ContainerOverrides.Vcpus != 4 || *in.ContainerOverrides.Memory != 1907 {
		t.Errorf("submit request %v", in)
	}
	if len(ev.events) != 1 || ev.events[0].GetMetadata().GetValue()["awsbatch_id"] != "aws-1" {
		t.Errorf("events %v", ev.events)
	}

	fake.submitErr = errors.New("queue disabled")
	if err := b.Submit(&tes.Task{Id: "t2"}); err == nil || !ev.systemError("t2") {
		t.Errorf("a failed submit must fail the task: %v", err)
	}
}

func TestAWSCancel(t *testing.T) {
	fake := &batchFake{}
	b, _ := newAWSBackend(fake, taskList{
		withAWSID("queued", tes.Queued, "aws-q"),
		withAWSID("running", tes.Running, "aws-r"),
		{Id: "no-id", State: tes.Queued},
	})
	ctx := context.Background()
	if err := b.WriteEvent(ctx, events.NewState("queued", tes.Canceled)); err != nil {
		t.Fatal(err)
	}
	if err := b.Cancel(ctx, "running"); err != nil {
		t.Fatal(err)
	}
	if len(fake.canceled) != 1 || fake.canceled[0] != "aws-q" {
		t.Errorf("canceled %v: only queued jobs are canceled", fake.canceled)
	}
	if err := b.Cancel(ctx, "no-id"); err == nil {
		t.Error("a task without an AWS ID cannot be canceled")
	}
	if err := b.Cancel(ctx, "missing"); err == nil {
		t.Error("an unknown task cannot be canceled")
	}
}

func TestAWSReconcile(t *testing.T) {
	fake := &batchFake{statuses: map[string]string{"aws-1": "FAILED", "aws-2": "RUNNING"}}
	b, ev := newAWSBackend(fake, taskList{withAWSID("failed", tes.Running, "aws-1"), withAWSID("ok", tes.Running, "aws-2")})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	b.reconcile(ctx)
	if !ev.systemError("failed") || ev.systemError("ok") {
		t.Errorf("events %v", ev.events)
	}
}
