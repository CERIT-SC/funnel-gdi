package server

import (
	"testing"

	"github.com/ohsu-comp-bio/funnel/tes"
)

// A failed CreateTask returns a typed-nil *tes.CreateTaskResponse alongside
// a non-nil error. Once boxed into the `resp interface{}` that the audit
// interceptor receives, that's a non-nil interface wrapping a nil pointer,
// so auditTaskID must not dereference it.
func TestAuditTaskIDNilCreateTaskResponse(t *testing.T) {
	var resp *tes.CreateTaskResponse

	id := auditTaskID(&tes.Task{}, resp)
	if id != "" {
		t.Errorf("expected empty taskID for nil CreateTaskResponse, got %q", id)
	}
}

func TestAuditTaskIDCreateTaskResponse(t *testing.T) {
	resp := &tes.CreateTaskResponse{Id: "task-123"}

	id := auditTaskID(&tes.Task{}, resp)
	if id != "task-123" {
		t.Errorf("expected task-123, got %q", id)
	}
}

func TestAuditTaskIDGetTaskRequest(t *testing.T) {
	req := &tes.GetTaskRequest{Id: "task-456"}

	id := auditTaskID(req, nil)
	if id != "task-456" {
		t.Errorf("expected task-456, got %q", id)
	}
}
