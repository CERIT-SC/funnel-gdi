package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/events"
	"github.com/ohsu-comp-bio/funnel/logger"
	"github.com/ohsu-comp-bio/funnel/tes"
	"google.golang.org/protobuf/types/known/durationpb"
)

// memNodes is an in-memory node database.
type memNodes struct {
	UnimplementedSchedulerServiceServer
	mu      sync.Mutex
	nodes   map[string]*Node
	putErr  error
	listErr error
	deleted []string
}

func (m *memNodes) PutNode(ctx context.Context, n *Node) (*PutNodeResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.putErr != nil {
		return nil, m.putErr
	}
	m.nodes[n.Id] = n
	return &PutNodeResponse{}, nil
}

func (m *memNodes) DeleteNode(ctx context.Context, n *Node) (*DeleteNodeResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.nodes, n.Id)
	m.deleted = append(m.deleted, n.Id)
	return &DeleteNodeResponse{}, nil
}

func (m *memNodes) ListNodes(ctx context.Context, r *ListNodesRequest) (*ListNodesResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listErr != nil {
		return nil, m.listErr
	}
	var out []*Node
	for _, n := range m.nodes {
		out = append(out, n)
	}
	return &ListNodesResponse{Nodes: out}, nil
}

// recorder records the events written to it.
type recorder struct {
	mu     sync.Mutex
	events []*events.Event
}

func (r *recorder) WriteEvent(ctx context.Context, e *events.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	return nil
}
func (r *recorder) Close() {}

func (r *recorder) states(taskID string) []tes.State {
	var out []tes.State
	for _, e := range r.events {
		if e.Id == taskID && e.Type == events.Type_TASK_STATE {
			out = append(out, e.GetState())
		}
	}
	return out
}

type sliceQueue []*tes.Task

func (q sliceQueue) ReadQueue(n int) []*tes.Task { return q }

func schedConf() *config.Scheduler {
	return &config.Scheduler{
		ScheduleChunk:   10,
		NodePingTimeout: &config.TimeoutConfig{TimeoutOption: &config.TimeoutConfig_Duration{Duration: durationpb.New(time.Minute)}},
		NodeInitTimeout: &config.TimeoutConfig{TimeoutOption: &config.TimeoutConfig_Duration{Duration: durationpb.New(5 * time.Minute)}},
		NodeDeadTimeout: &config.TimeoutConfig{TimeoutOption: &config.TimeoutConfig_Duration{Duration: durationpb.New(10 * time.Minute)}},
	}
}

func ago(d time.Duration) int64 { return time.Now().Add(-d).UnixNano() }

func TestUpdateNodeState(t *testing.T) {
	nodes := []*Node{
		{Id: "fresh", State: NodeState_ALIVE, LastPing: ago(time.Second)},
		{Id: "silent", State: NodeState_ALIVE, LastPing: ago(2 * time.Minute)},
		{Id: "init-ok", State: NodeState_INITIALIZING, LastPing: ago(2 * time.Minute)},
		{Id: "init-failed", State: NodeState_UNINITIALIZED, LastPing: ago(6 * time.Minute)},
		{Id: "dead-recent", State: NodeState_DEAD, LastPing: ago(2 * time.Minute)},
		{Id: "dead-long", State: NodeState_DEAD, LastPing: ago(11 * time.Minute)},
		{Id: "gone", State: NodeState_GONE, LastPing: ago(time.Hour)},
		{Id: "never-pinged", State: NodeState_ALIVE},
	}
	updated := map[string]NodeState{}
	for _, n := range UpdateNodeState(nodes, schedConf()) {
		updated[n.Id] = n.State
	}
	want := map[string]NodeState{
		"silent":      NodeState_DEAD,
		"init-failed": NodeState_DEAD,
		"dead-long":   NodeState_GONE,
		"gone":        NodeState_GONE,
	}
	for id, state := range want {
		if updated[id] != state {
			t.Errorf("%s: %v, want %v", id, updated[id], state)
		}
	}
	// dead-recent stays dead (not yet gone): unchanged.
	for _, id := range []string{"fresh", "init-ok", "never-pinged", "dead-recent"} {
		if _, ok := updated[id]; ok {
			t.Errorf("%s should not change", id)
		}
	}
}

func TestResourceAccounting(t *testing.T) {
	base := &Resources{Cpus: 4, RamGb: 8, DiskGb: 100}
	tasks := []*tes.Task{
		{Resources: &tes.Resources{CpuCores: 1, RamGb: 2, DiskGb: 10}},
		{Resources: &tes.Resources{CpuCores: 2, RamGb: 2}},
		{},
	}
	got := AvailableResources(tasks, base)
	if got.Cpus != 1 || got.RamGb != 4 || got.DiskGb != 90 {
		t.Errorf("available %v", got)
	}
	// Asking for more than there is never goes below zero.
	over := SubtractResources(&tes.Task{Resources: &tes.Resources{CpuCores: 10, RamGb: 100, DiskGb: 1000}}, base)
	if over.Cpus != 0 || over.RamGb != 0 || over.DiskGb != 0 {
		t.Errorf("over-subscribed %v", over)
	}
	if base.Cpus != 4 {
		t.Error("SubtractResources changed its input")
	}
}

// fixedReader answers GetTask with fixed states.
type fixedReader map[string]tes.State

func (r fixedReader) GetTask(ctx context.Context, req *tes.GetTaskRequest) (*tes.Task, error) {
	s, ok := r[req.Id]
	if !ok {
		return nil, tes.ErrNotFound
	}
	return &tes.Task{Id: req.Id, State: s, Resources: &tes.Resources{CpuCores: 1, RamGb: 1}}, nil
}
func (r fixedReader) ListTasks(context.Context, *tes.ListTasksRequest) (*tes.ListTasksResponse, error) {
	return nil, nil
}
func (r fixedReader) Close() {}

func TestUpdateNode(t *testing.T) {
	node := &Node{
		Id:        "n1",
		TaskIds:   []string{"running", "done", "missing"},
		Resources: &Resources{Cpus: 4, RamGb: 8},
		Metadata:  map[string]string{"new": "1", "both": "node"},
	}
	existing := &Node{Metadata: map[string]string{"old": "1", "both": "existing"}}
	reader := fixedReader{"running": tes.Running, "done": tes.Complete}
	if err := UpdateNode(context.Background(), reader, node, existing); err != nil {
		t.Fatal(err)
	}
	// Only the running task still uses resources.
	if node.Available.Cpus != 3 || node.Available.RamGb != 7 {
		t.Errorf("available %v", node.Available)
	}
	if node.LastPing == 0 {
		t.Error("LastPing not set")
	}
	if node.Metadata["old"] != "1" || node.Metadata["new"] != "1" || node.Metadata["both"] != "node" {
		t.Errorf("metadata %v", node.Metadata)
	}
}

func newTestScheduler(nodes *memNodes, queue []*tes.Task) (*Scheduler, *recorder) {
	rec := &recorder{}
	return &Scheduler{
		Conf:  schedConf(),
		Log:   logger.NewLogger("test", logger.DefaultConfig()),
		Nodes: nodes,
		Queue: sliceQueue(queue),
		Event: rec,
	}, rec
}

func TestScheduleAssignsTask(t *testing.T) {
	nodes := &memNodes{nodes: map[string]*Node{
		"n1": {Id: "n1", State: NodeState_ALIVE, LastPing: ago(time.Second),
			Resources: &Resources{Cpus: 4, RamGb: 8, DiskGb: 100}, Available: &Resources{Cpus: 4, RamGb: 8, DiskGb: 100}},
	}}
	task := &tes.Task{Id: "t1", Resources: &tes.Resources{CpuCores: 1}}
	tooBig := &tes.Task{Id: "t2", Resources: &tes.Resources{CpuCores: 64}}
	s, rec := newTestScheduler(nodes, []*tes.Task{task, tooBig})

	if err := s.Schedule(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := nodes.nodes["n1"].TaskIds; len(got) != 1 || got[0] != "t1" {
		t.Errorf("node tasks %v", got)
	}
	if st := rec.states("t1"); len(st) != 1 || st[0] != tes.Initializing {
		t.Errorf("t1 states %v", st)
	}
	if st := rec.states("t2"); len(st) != 0 {
		t.Errorf("t2 does not fit and must stay queued: %v", st)
	}
}

func TestScheduleAssignFails(t *testing.T) {
	nodes := &memNodes{nodes: map[string]*Node{
		"n1": {Id: "n1", State: NodeState_ALIVE, LastPing: ago(time.Second),
			Resources: &Resources{Cpus: 4, RamGb: 8, DiskGb: 100}, Available: &Resources{Cpus: 4, RamGb: 8, DiskGb: 100}},
	}}
	s, rec := newTestScheduler(nodes, []*tes.Task{{Id: "t1"}})
	nodes.putErr = errors.New("database down")
	if err := s.Schedule(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st := rec.states("t1"); len(st) != 0 {
		t.Errorf("a task that could not be assigned must stay queued: %v", st)
	}
}

func TestCheckNodesCleansUpGoneNodes(t *testing.T) {
	nodes := &memNodes{nodes: map[string]*Node{
		"gone": {Id: "gone", State: NodeState_DEAD, LastPing: ago(time.Hour), TaskIds: []string{"t1"}},
		"dead": {Id: "dead", State: NodeState_ALIVE, LastPing: ago(2 * time.Minute)},
	}}
	s, rec := newTestScheduler(nodes, nil)
	if err := s.CheckNodes(); err != nil {
		t.Fatal(err)
	}
	if len(nodes.deleted) != 1 || nodes.deleted[0] != "gone" {
		t.Errorf("deleted %v", nodes.deleted)
	}
	if st := rec.states("t1"); len(st) != 1 || st[0] != tes.SystemError {
		t.Errorf("task of a gone node: %v", st)
	}
	if nodes.nodes["dead"].State != NodeState_DEAD {
		t.Errorf("silent node: %v", nodes.nodes["dead"].State)
	}

	nodes.listErr = errors.New("database down")
	if err := s.CheckNodes(); err == nil {
		t.Error("expected the list error")
	}
}

func TestSchedulerRunStops(t *testing.T) {
	s, _ := newTestScheduler(&memNodes{nodes: map[string]*Node{}}, nil)
	s.Conf.ScheduleRate = durationpb.New(time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.Run(ctx); err != nil {
		t.Fatal(err)
	}
}
