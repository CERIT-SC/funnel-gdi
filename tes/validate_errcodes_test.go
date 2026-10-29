package tes

import (
	"strings"
	"testing"
)

func validTask() *Task {
	return &Task{Executors: []*Executor{{Image: "alpine", Command: []string{"true"}}}}
}

// Every validation rule reports its own error code.
func TestValidationErrorCodes(t *testing.T) {
	forbidden := []string{"/forbidden"}
	for _, tc := range []struct {
		code   string
		modify func(*Task)
	}{
		{"FUNNEL-VAL-001", func(t *Task) { t.Executors = nil }},
		{"FUNNEL-VAL-002", func(t *Task) { t.Executors[0].Image = "" }},
		{"FUNNEL-VAL-003", func(t *Task) { t.Executors[0].Command = nil }},
		{"FUNNEL-VAL-004", func(t *Task) { t.Executors[0].Workdir = "relative" }},
		{"FUNNEL-VAL-005", func(t *Task) { t.Executors[0].Workdir = "/forbidden/x" }},
		{"FUNNEL-VAL-006", func(t *Task) { t.Executors[0].Stdin = "in.txt" }},
		{"FUNNEL-VAL-007", func(t *Task) { t.Executors[0].Stdout = "out.txt" }},
		{"FUNNEL-VAL-008", func(t *Task) { t.Executors[0].Stderr = "err.txt" }},
		{"FUNNEL-VAL-009", func(t *Task) { t.Inputs = []*Input{{Content: "x", Url: "s3://b/k", Path: "/in"}} }},
		{"FUNNEL-VAL-010", func(t *Task) { t.Inputs = []*Input{{Path: "/in"}} }},
		{"FUNNEL-VAL-011", func(t *Task) { t.Inputs = []*Input{{Url: "s3://b/k"}} }},
		{"FUNNEL-VAL-012", func(t *Task) { t.Inputs = []*Input{{Url: "s3://b/k", Path: "in"}} }},
		{"FUNNEL-VAL-013", func(t *Task) { t.Inputs = []*Input{{Url: "s3://b/k", Path: "/forbidden/in"}} }},
		{"FUNNEL-VAL-014", func(t *Task) { t.Outputs = []*Output{{Path: "/out"}} }},
		{"FUNNEL-VAL-015", func(t *Task) { t.Outputs = []*Output{{Url: "s3://b/k"}} }},
		{"FUNNEL-VAL-016", func(t *Task) { t.Outputs = []*Output{{Url: "s3://b/k", Path: "out"}} }},
		{"FUNNEL-VAL-017", func(t *Task) { t.Outputs = []*Output{{Url: "s3://b/k", Path: "/forbidden/out"}} }},
		{"FUNNEL-VAL-018", func(t *Task) { t.Volumes = []string{"vol"} }},
		{"FUNNEL-VAL-019", func(t *Task) { t.Volumes = []string{"/forbidden/vol"} }},
		{"FUNNEL-VAL-020", func(t *Task) { t.Tags = map[string]string{"": "v"} }},
	} {
		t.Run(tc.code, func(t *testing.T) {
			task := validTask()
			tc.modify(task)
			errs := ValidateWithForbiddenPathPrefixes(task, forbidden)
			if len(errs) != 1 {
				t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
			}
			if !strings.HasPrefix(errs[0].Error(), tc.code+": ") {
				t.Errorf("error %q should start with %s", errs[0], tc.code)
			}
		})
	}
}

func TestValidationReportsEveryProblem(t *testing.T) {
	task := validTask()
	task.Executors[0].Image = ""
	task.Inputs = []*Input{{Url: "s3://b/k", Path: "in"}}
	task.Volumes = []string{"vol"}

	errs := Validate(task)
	var got []string
	for _, e := range errs {
		got = append(got, strings.SplitN(e.Error(), ":", 2)[0])
	}
	if strings.Join(got, " ") != "FUNNEL-VAL-002 FUNNEL-VAL-012 FUNNEL-VAL-018" {
		t.Errorf("got codes %v", got)
	}
}

func TestValidTaskHasNoErrors(t *testing.T) {
	if errs := Validate(validTask()); errs != nil {
		t.Fatalf("unexpected errors: %v", errs)
	}
}
