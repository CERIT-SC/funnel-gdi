// Package local contains code for accessing compute resources via the local computer, for Funnel development and debugging.
package local

import (
	"context"
	"fmt"

	workerCmd "github.com/ohsu-comp-bio/funnel/cmd/worker"
	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/events"
	"github.com/ohsu-comp-bio/funnel/logger"
	"github.com/ohsu-comp-bio/funnel/tes"
)

// NewBackend returns a new local Backend instance.
func NewBackend(ctx context.Context, conf config.Config, log *logger.Logger) (*Backend, error) {
	return &Backend{conf, log}, nil
}

// Backend represents the local backend.
type Backend struct {
	conf config.Config
	log  *logger.Logger
}

// WriteEvent writes an event to the compute backend.
// Currently, only TASK_CREATED is handled, which calls Submit.
func (b *Backend) WriteEvent(ctx context.Context, ev *events.Event) error {
	switch ev.Type {
	case events.Type_TASK_CREATED:
		return b.Submit(ev.GetTask())
	}
	return nil
}

func (b *Backend) Close() {}

// Submit submits a task. For the Local backend this results in the task
// running immediately.
func (b *Backend) Submit(task *tes.Task) error {
	ctx := context.Background()

	w, err := workerCmd.NewWorker(ctx, b.conf, b.log, &workerCmd.Options{
		TaskID: task.Id,
	})
	if err != nil {
		return err
	}

	go func() {
		if err := w.Run(ctx); err != nil {
			fmt.Printf("Detected error while submitting a task: %s\n", err)
		}
		w.Close()
	}()
	return nil
}
