package tes

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateTransition(t *testing.T) {
	all := []State{Unknown, Queued, Initializing, Running, Paused, Complete, ExecutorError, SystemError, Canceled}
	allowed := map[State][]State{
		Unknown:       {Unknown, Queued, Initializing, Running, Complete, ExecutorError, SystemError, Canceled},
		Queued:        {Queued, Initializing, Running, Complete, ExecutorError, SystemError, Canceled},
		Initializing:  {Initializing, Running, Complete, ExecutorError, SystemError, Canceled},
		Running:       {Running, Complete, ExecutorError, SystemError, Canceled},
		Paused:        {Paused},
		Complete:      {Complete},
		ExecutorError: {ExecutorError, Queued, Initializing},
		SystemError:   {SystemError, Queued, Initializing},
		Canceled:      {Canceled},
	}
	for _, from := range all {
		ok := map[State]bool{}
		for _, to := range allowed[from] {
			ok[to] = true
		}
		for _, to := range all {
			err := ValidateTransition(from, to)
			if ok[to] && err != nil {
				t.Errorf("%s -> %s: unexpected error %v", from, to, err)
			}
			if !ok[to] && err == nil {
				t.Errorf("%s -> %s: expected an error", from, to)
			}
			var te *TransitionError
			if err != nil && from != Paused && to != Paused && !errors.As(err, &te) {
				t.Errorf("%s -> %s: error %v is not a TransitionError", from, to, err)
			}
		}
	}
	if got := (&TransitionError{Running, Queued}).Error(); got != "invalid state transition from RUNNING to QUEUED" {
		t.Errorf("message %q", got)
	}
}

func TestTaskViews(t *testing.T) {
	task := &Task{
		Id:     "t1",
		State:  Running,
		Name:   "name",
		Inputs: []*Input{{Content: "secret content", Path: "/in"}},
		Logs: []*TaskLog{{
			SystemLogs: []string{"system"},
			Logs:       []*ExecutorLog{{Stdout: "out", Stderr: "err", ExitCode: 1}},
		}},
	}

	basic := task.GetBasicView()
	if basic.Inputs[0].Content != "" || basic.Logs[0].SystemLogs != nil || basic.Logs[0].Logs[0].Stdout != "" || basic.Logs[0].Logs[0].Stderr != "" {
		t.Errorf("basic view keeps FULL-only fields: %v", basic)
	}
	if basic.Name != "name" || basic.Logs[0].Logs[0].ExitCode != 1 || basic.Inputs[0].Path != "/in" {
		t.Errorf("basic view lost fields: %v", basic)
	}
	if task.Inputs[0].Content != "secret content" || task.Logs[0].Logs[0].Stdout != "out" {
		t.Error("GetBasicView changed the original task")
	}

	minimal := task.GetMinimalView()
	if minimal.Id != "t1" || minimal.State != Running || minimal.Name != "" || minimal.Inputs != nil {
		t.Errorf("minimal view: %v", minimal)
	}

	s, err := MarshalToString(&Task{Id: "t1", State: Queued})
	if err != nil || s == "" || !strings.Contains(s, `"id"`) || !strings.Contains(s, `"QUEUED"`) {
		t.Errorf("MarshalToString: %q %v", s, err)
	}
}
