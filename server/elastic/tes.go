package elastic

import (
	"fmt"
	"github.com/ohsu-comp-bio/funnel/compute"
	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/events"
	"github.com/ohsu-comp-bio/funnel/proto/tes"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"gopkg.in/olivere/elastic.v5"
)

// TES provides the TES API endpoints, backed by elasticsearch.
type TES struct {
	*Elastic
	Backend compute.Backend
}

// NewTES creates a new TES API with the given config.
func NewTES(conf config.Elastic) (*TES, error) {
	es, err := NewElastic(conf)
	return &TES{Elastic: es}, err
}

// WithComputeBackend sets the compute backend.
func (et *TES) WithComputeBackend(b compute.Backend) {
	et.Backend = b
}

// CreateTask creates a new task.
func (et *TES) CreateTask(ctx context.Context, task *tes.Task) (*tes.CreateTaskResponse, error) {

	if err := tes.InitTask(task); err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}

	if task.Tags == nil {
		task.Tags = map[string]string{}
	}

	if err := et.Elastic.CreateTask(ctx, task); err != nil {
		return nil, err
	}

	if et.Backend != nil {
		if err := et.Backend.Submit(task); err != nil {
			return nil, err
		}
	}
	return &tes.CreateTaskResponse{Id: task.Id}, nil
}

// GetTask gets a task by ID.
func (et *TES) GetTask(ctx context.Context, req *tes.GetTaskRequest) (*tes.Task, error) {
	resp, err := et.Elastic.GetTask(ctx, req)
	if elastic.IsNotFound(err) {
		return nil, grpc.Errorf(codes.NotFound, fmt.Sprintf("%v: task ID: %s", err.Error(), req.Id))
	}
	return resp, err
}

// ListTasks lists tasks.
// TODO list is maybe where the having the TES api separated from the core
//      database breaks down.
func (et *TES) ListTasks(ctx context.Context, req *tes.ListTasksRequest) (*tes.ListTasksResponse, error) {
	return et.Elastic.ListTasks(ctx, req)
}

// CancelTask cancels a task by ID.
func (et *TES) CancelTask(ctx context.Context, req *tes.CancelTaskRequest) (*tes.CancelTaskResponse, error) {
	err := et.Elastic.WriteContext(ctx, events.NewState(req.Id, 0, tes.State_CANCELED))
	if elastic.IsNotFound(err) {
		return nil, grpc.Errorf(codes.NotFound, fmt.Sprintf("%v: task ID: %s", err.Error(), req.Id))
	}
	return &tes.CancelTaskResponse{}, err
}

// GetServiceInfo returns service metadata.
func (et *TES) GetServiceInfo(ctx context.Context, info *tes.ServiceInfoRequest) (*tes.ServiceInfo, error) {
	return &tes.ServiceInfo{Name: "elastic"}, nil
}

// CreateEvent writes a task event to the database.
func (et *TES) CreateEvent(ctx context.Context, req *events.Event) (*events.CreateEventResponse, error) {
	err := et.Elastic.WriteContext(ctx, req)
	if err != nil {
		return nil, err
	}
	return &events.CreateEventResponse{}, err
}
