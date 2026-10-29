package server

import (
	"context"
	"strings"
	"testing"

	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/logger"
	"github.com/ohsu-comp-bio/funnel/tes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// notFoundReader is a task database without tasks.
type notFoundReader struct{}

func (notFoundReader) GetTask(context.Context, *tes.GetTaskRequest) (*tes.Task, error) {
	return nil, tes.ErrNotFound
}

func (notFoundReader) ListTasks(context.Context, *tes.ListTasksRequest) (*tes.ListTasksResponse, error) {
	return &tes.ListTasksResponse{}, nil
}

func (notFoundReader) Close() {}

func TestTaskNotFoundCode(t *testing.T) {
	ts := &TaskService{Read: notFoundReader{}, Config: config.DefaultConfig(), Log: logger.NewLogger("test", logger.DefaultConfig())}
	long := strings.Repeat("a", 64*1024)

	for _, tc := range []struct {
		name, id, wantID string
	}{
		{"unknown ID", "no-such-task", "no-such-task"},
		{"empty ID", "", ""},
		{"very long ID", long, long[:256]},
	} {
		for method, call := range map[string]func() error{
			"GetTask": func() error {
				_, err := ts.GetTask(context.Background(), &tes.GetTaskRequest{Id: tc.id})
				return err
			},
			"CancelTask": func() error {
				_, err := ts.CancelTask(context.Background(), &tes.CancelTaskRequest{Id: tc.id})
				return err
			},
		} {
			t.Run(tc.name+"/"+method, func(t *testing.T) {
				st, _ := status.FromError(call())
				if st.Code() != codes.NotFound {
					t.Errorf("code %v, want NotFound", st.Code())
				}
				want := "FUNNEL-TASK-001: task not found: taskID: " + tc.wantID
				if !strings.HasPrefix(st.Message(), want) {
					t.Errorf("message %.120q should start with %.120q", st.Message(), want)
				}
				if len(st.Message()) > 400 {
					t.Errorf("message is %d bytes long: the task ID is not shortened", len(st.Message()))
				}
			})
		}
	}
}

func TestSDAInputWithoutTokenCode(t *testing.T) {
	ts := &TaskService{Config: config.DefaultConfig(), Log: logger.NewLogger("test", logger.DefaultConfig())}
	task := &tes.Task{
		Executors: []*tes.Executor{{Image: "alpine", Command: []string{"true"}}},
		Inputs:    []*tes.Input{{Url: "sda://dataset/file", Path: "/data/file"}},
	}
	_, err := ts.CreateTask(context.Background(), task)
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument || !strings.HasPrefix(st.Message(), "FUNNEL-TASK-002: ") {
		t.Fatalf("got %v %q", st.Code(), st.Message())
	}
}
