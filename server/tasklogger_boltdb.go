package server

import (
	"errors"
	"fmt"
	"github.com/boltdb/bolt"
	proto "github.com/golang/protobuf/proto"
	tl "github.com/ohsu-comp-bio/funnel/proto/tasklogger"
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

// UpdateTaskState updates a task's state in the database.
func (taskBolt *TaskBolt) UpdateTaskState(ctx context.Context, req *tl.UpdateTaskStateRequest) (*tl.UpdateTaskStateResponse, error) {
	err := taskBolt.db.Update(func(tx *bolt.Tx) error {
		return transitionTaskState(tx, req.Id, req.State)
	})
	return &tl.UpdateTaskStateResponse{}, err
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
		log.Error("Unexpected transition", "current", current, "target", target)
		return errors.New("Unexpected transition to Initializing")

	case target == Queued:
		log.Error("Can't transition to Queued state")
		return errors.New("Can't transition to Queued state")
	}

	switch target {
	case Unknown, Paused:
		log.Error("Unimplemented task state", "state", target)
		return errors.New("Unimplemented task state")

	case Canceled, Complete, Error, SystemError:
		// Remove from queue
		tx.Bucket(TasksQueued).Delete(idBytes)

	case Running, Initializing:
		if current != Unknown && current != Queued && current != Initializing {
			log.Error("Unexpected transition", "current", current, "target", target)
			return errors.New("Unexpected transition to Initializing")
		}
		tx.Bucket(TasksQueued).Delete(idBytes)

	default:
		log.Error("Unknown target state", "target", target)
		return errors.New("Unknown task state")
	}

	tx.Bucket(TaskState).Put(idBytes, []byte(target.String()))
	log.Info("Set task state", "taskID", id, "state", target.String())
	return nil
}

// UpdateTaskLogs is an internal API endpoint that allows the worker to update
// task logs (start time, end time, output files, etc).
func (taskBolt *TaskBolt) UpdateTaskLogs(ctx context.Context, req *tl.UpdateTaskLogsRequest) (*tl.UpdateTaskLogsResponse, error) {
	log.Debug("Update task logs", req)

	err := taskBolt.db.Update(func(tx *bolt.Tx) error {
		tasklog := &tes.TaskLog{}

		// Try to load existing task log
		b := tx.Bucket(TasksLog).Get([]byte(req.Id))
		if b != nil {
			proto.Unmarshal(b, tasklog)
		}

		if req.TaskLog.StartTime != "" {
			tasklog.StartTime = req.TaskLog.StartTime
		}

		if req.TaskLog.EndTime != "" {
			tasklog.EndTime = req.TaskLog.EndTime
		}

		if req.TaskLog.Outputs != nil {
			tasklog.Outputs = req.TaskLog.Outputs
		}

		if req.TaskLog.Metadata != nil {
			if tasklog.Metadata == nil {
				tasklog.Metadata = map[string]string{}
			}
			for k, v := range req.TaskLog.Metadata {
				tasklog.Metadata[k] = v
			}
		}

		logbytes, _ := proto.Marshal(tasklog)
		tx.Bucket(TasksLog).Put([]byte(req.Id), logbytes)
		return nil
	})
	return &tl.UpdateTaskLogsResponse{}, err
}

// UpdateExecutorLogs is an API endpoint that updates the logs of a task.
// This is used by workers to communicate task updates to the server.
func (taskBolt *TaskBolt) UpdateExecutorLogs(ctx context.Context, req *tl.UpdateExecutorLogsRequest) (*tl.UpdateExecutorLogsResponse, error) {
	log.Debug("Update task executor logs", req)

	taskBolt.db.Update(func(tx *bolt.Tx) error {
		bL := tx.Bucket(ExecutorLogs)

		// max size (bytes) for stderr and stdout streams to keep in db
		max := taskBolt.conf.Server.MaxExecutorLogSize
		key := []byte(fmt.Sprint(req.Id, req.Step))

		if req.Log != nil {
			// Check if there is an existing task log
			o := bL.Get(key)
			if o != nil {
				// There is an existing log in the DB, load it
				existing := &tes.ExecutorLog{}
				// max bytes to be stored in the db
				proto.Unmarshal(o, existing)

				stdout := []byte(existing.Stdout + req.Log.Stdout)
				stderr := []byte(existing.Stderr + req.Log.Stderr)

				// Trim the stdout/err logs to the max size if needed
				if len(stdout) > max {
					stdout = stdout[:max]
				}
				if len(stderr) > max {
					stderr = stderr[:max]
				}

				req.Log.Stdout = string(stdout)
				req.Log.Stderr = string(stderr)

				// Merge the updates into the existing.
				proto.Merge(existing, req.Log)
				// existing is updated, so set that to req.Log which will get saved below.
				req.Log = existing
			}

			// Save the updated log
			logbytes, _ := proto.Marshal(req.Log)
			tx.Bucket(ExecutorLogs).Put(key, logbytes)
		}

		return nil
	})
	return &tl.UpdateExecutorLogsResponse{}, nil
}
