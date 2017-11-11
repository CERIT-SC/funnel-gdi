package boltdb

import (
	"fmt"
	"github.com/boltdb/bolt"
	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/ohsu-comp-bio/funnel/events"
	"github.com/ohsu-comp-bio/funnel/proto/tes"
	"golang.org/x/net/context"
)

// State variables for convenience
const (
	Unknown      = tes.State_UNKNOWN
	Queued       = tes.State_QUEUED
	Running      = tes.State_RUNNING
	Paused       = tes.State_PAUSED
	Complete     = tes.State_COMPLETE
	Error        = tes.State_ERROR
	SystemError  = tes.State_SYSTEM_ERROR
	Canceled     = tes.State_CANCELED
	Initializing = tes.State_INITIALIZING
)

// CreateEvent creates an event for the server to handle.
func (taskBolt *BoltDB) CreateEvent(ctx context.Context, req *events.Event) (*events.CreateEventResponse, error) {
	return &events.CreateEventResponse{}, taskBolt.WriteContext(ctx, req)
}

// Write writes task events to the database, updating the task record they
// are related to. System log events are ignored.
func (taskBolt *BoltDB) Write(req *events.Event) error {
	return taskBolt.WriteContext(context.Background(), req)
}

// WriteContext is Write, but with context.
func (taskBolt *BoltDB) WriteContext(ctx context.Context, req *events.Event) error {
	var err error

	tl := &tes.TaskLog{}
	el := &tes.ExecutorLog{}

	switch req.Type {
	case events.Type_TASK_STATE:
		err = taskBolt.db.Update(func(tx *bolt.Tx) error {
			return transitionTaskState(tx, req.Id, req.GetState())
		})

	case events.Type_TASK_START_TIME:
		tl.StartTime = ptypes.TimestampString(req.GetStartTime())
		err = taskBolt.db.Update(func(tx *bolt.Tx) error {
			return updateTaskLogs(tx, req.Id, tl)
		})

	case events.Type_TASK_END_TIME:
		tl.EndTime = ptypes.TimestampString(req.GetEndTime())
		err = taskBolt.db.Update(func(tx *bolt.Tx) error {
			return updateTaskLogs(tx, req.Id, tl)
		})

	case events.Type_TASK_OUTPUTS:
		tl.Outputs = req.GetOutputs().Value
		err = taskBolt.db.Update(func(tx *bolt.Tx) error {
			return updateTaskLogs(tx, req.Id, tl)
		})

	case events.Type_TASK_METADATA:
		tl.Metadata = req.GetMetadata().Value
		err = taskBolt.db.Update(func(tx *bolt.Tx) error {
			return updateTaskLogs(tx, req.Id, tl)
		})

	case events.Type_EXECUTOR_START_TIME:
		el.StartTime = ptypes.TimestampString(req.GetStartTime())
		err = taskBolt.db.Update(func(tx *bolt.Tx) error {
			return updateExecutorLogs(tx, fmt.Sprint(req.Id, req.Index), el)
		})

	case events.Type_EXECUTOR_END_TIME:
		el.EndTime = ptypes.TimestampString(req.GetEndTime())
		err = taskBolt.db.Update(func(tx *bolt.Tx) error {
			return updateExecutorLogs(tx, fmt.Sprint(req.Id, req.Index), el)
		})

	case events.Type_EXECUTOR_EXIT_CODE:
		el.ExitCode = req.GetExitCode()
		err = taskBolt.db.Update(func(tx *bolt.Tx) error {
			return updateExecutorLogs(tx, fmt.Sprint(req.Id, req.Index), el)
		})

	case events.Type_EXECUTOR_STDOUT:
		err = taskBolt.db.Update(func(tx *bolt.Tx) error {
			return updateExecutorStdout(tx, fmt.Sprint(req.Id, req.Index), req.GetStdout())
		})

	case events.Type_EXECUTOR_STDERR:
		err = taskBolt.db.Update(func(tx *bolt.Tx) error {
			return updateExecutorStderr(tx, fmt.Sprint(req.Id, req.Index), req.GetStderr())
		})
	}

	return err
}

func transitionTaskState(tx *bolt.Tx, id string, target tes.State) error {
	idBytes := []byte(id)
	current := getTaskState(tx, id)

	switch {
	case target == current:
		// Current state matches target state. Do nothing.
		return nil

	case tes.TerminalState(target) && tes.TerminalState(current):
		// Avoid switching between two terminal states.
		return fmt.Errorf("Won't switch between two terminal states: %s -> %s",
			current, target)

	case tes.TerminalState(current) && !tes.TerminalState(target):
		// Error when trying to switch out of a terminal state to a non-terminal one.
		return fmt.Errorf("Unexpected transition from %s to %s", current.String(), target.String())

	case target == Queued:
		return fmt.Errorf("Can't transition to Queued state")
	}

	switch target {
	case Unknown, Paused:
		return fmt.Errorf("Unimplemented task state %s", target.String())

	case Canceled, Complete, Error, SystemError:
		// Remove from queue
		tx.Bucket(TasksQueued).Delete(idBytes)

	case Running, Initializing:
		if current != Unknown && current != Queued && current != Initializing {
			return fmt.Errorf("Unexpected transition from %s to %s", current.String(), target.String())
		}
		tx.Bucket(TasksQueued).Delete(idBytes)

	default:
		return fmt.Errorf("Unknown target state: %s", target.String())
	}

	tx.Bucket(TaskState).Put(idBytes, []byte(target.String()))
	return nil
}

func updateTaskLogs(tx *bolt.Tx, id string, tl *tes.TaskLog) error {
	tasklog := &tes.TaskLog{}

	// Try to load existing task log
	b := tx.Bucket(TasksLog).Get([]byte(id))
	if b != nil {
		err := proto.Unmarshal(b, tasklog)
		if err != nil {
			return err
		}
	}

	if tl.StartTime != "" {
		tasklog.StartTime = tl.StartTime
	}

	if tl.EndTime != "" {
		tasklog.EndTime = tl.EndTime
	}

	if tl.Outputs != nil {
		tasklog.Outputs = tl.Outputs
	}

	if tl.Metadata != nil {
		if tasklog.Metadata == nil {
			tasklog.Metadata = map[string]string{}
		}
		for k, v := range tl.Metadata {
			tasklog.Metadata[k] = v
		}
	}

	logbytes, err := proto.Marshal(tasklog)
	if err != nil {
		return err
	}
	return tx.Bucket(TasksLog).Put([]byte(id), logbytes)
}

func updateExecutorLogs(tx *bolt.Tx, id string, el *tes.ExecutorLog) error {
	// Check if there is an existing task log
	o := tx.Bucket(ExecutorLogs).Get([]byte(id))
	if o != nil {
		// There is an existing log in the DB, load it
		existing := &tes.ExecutorLog{}
		err := proto.Unmarshal(o, existing)
		if err != nil {
			return err
		}

		el.Stdout = ""
		el.Stderr = ""

		// Merge the updates into the existing.
		proto.Merge(existing, el)
		// existing is updated, so set that to el which will get saved below.
		el = existing
	}

	// Save the updated log
	logbytes, err := proto.Marshal(el)
	if err != nil {
		return err
	}
	return tx.Bucket(ExecutorLogs).Put([]byte(id), logbytes)
}

func updateExecutorStdout(tx *bolt.Tx, id, stdout string) error {
	return tx.Bucket(ExecutorStdout).Put([]byte(id), []byte(stdout))
}

func updateExecutorStderr(tx *bolt.Tx, id, stderr string) error {
	return tx.Bucket(ExecutorStderr).Put([]byte(id), []byte(stderr))
}
