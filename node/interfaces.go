package node

import (
	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/worker"
)

// WorkerFactory is a function which creates a new task runner instance.
type WorkerFactory func(c config.Worker, taskID string) worker.Worker
