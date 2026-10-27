package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/events"
	"github.com/ohsu-comp-bio/funnel/logger"
	"github.com/ohsu-comp-bio/funnel/tes"
	"github.com/ohsu-comp-bio/funnel/version"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// The HTTP gateway maps gRPC codes to the HTTP status codes of the TES API.
func TestCustomErrorHandler(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
	}{
		{status.Error(codes.InvalidArgument, "m"), http.StatusBadRequest},
		{status.Error(codes.FailedPrecondition, "m"), http.StatusBadRequest},
		{status.Error(codes.OutOfRange, "m"), http.StatusBadRequest},
		{status.Error(codes.Unauthenticated, "m"), http.StatusUnauthorized},
		{status.Error(codes.PermissionDenied, "m"), http.StatusForbidden},
		{status.Error(codes.NotFound, "m"), http.StatusNotFound},
		{status.Error(codes.AlreadyExists, "m"), http.StatusConflict},
		{status.Error(codes.Aborted, "m"), http.StatusConflict},
		{status.Error(codes.Canceled, "m"), 499},
		{status.Error(codes.DeadlineExceeded, "m"), http.StatusGatewayTimeout},
		{status.Error(codes.Internal, "m"), http.StatusInternalServerError},
		{status.Error(codes.Unknown, "m"), http.StatusInternalServerError},
	} {
		t.Run(status.Code(tc.err).String(), func(t *testing.T) {
			w := httptest.NewRecorder()
			customErrorHandler(context.Background(), nil, &runtime.JSONPb{}, w, httptest.NewRequest(http.MethodGet, "/", nil), tc.err)
			if w.Code != tc.code {
				t.Errorf("HTTP %d, want %d", w.Code, tc.code)
			}
			var body JSONError
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error != "m" {
				t.Errorf("body %q", w.Body.String())
			}
		})
	}

	t.Run("not a gRPC status", func(t *testing.T) {
		w := httptest.NewRecorder()
		customErrorHandler(context.Background(), nil, &runtime.JSONPb{}, w, httptest.NewRequest(http.MethodGet, "/", nil), errors.New("plain"))
		if w.Code != http.StatusInternalServerError || w.Body.String() != `{"error": "failed to process the request"}` {
			t.Errorf("got %d %q", w.Code, w.Body.String())
		}
	})
}

func TestHealthHandler(t *testing.T) {
	w := httptest.NewRecorder()
	healthHandler(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	var body struct {
		Status     string           `json:"status"`
		LogMetrics map[string]int64 `json:"log_metrics"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || body.Status != "OK" || w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("got %d %q", w.Code, w.Body.String())
	}
	for _, k := range []string{"stdout_events_dropped", "stderr_events_dropped", "total_events"} {
		if _, ok := body.LogMetrics[k]; !ok {
			t.Errorf("log_metrics.%s missing", k)
		}
	}
}

func TestNegotiate(t *testing.T) {
	for _, tc := range []struct {
		url, accept, want string
	}{
		{"/v1/tasks?html", "application/json", "html"},
		{"/v1/tasks?json", "text/html", "json"},
		{"/v1/tasks", "text/html", "html"},
		{"/v1/tasks", "application/json", "json"},
		{"/v1/tasks", "", "json"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.url, nil)
		if tc.accept != "" {
			req.Header.Set("Accept", tc.accept)
		}
		if got := negotiate(req); got != tc.want {
			t.Errorf("%s (Accept %q): %s, want %s", tc.url, tc.accept, got, tc.want)
		}
	}
}

// messageHandler turns the X-Funnel-Message gRPC header (set e.g. when a
// task to cancel has already finished) into a JSON body.
func TestMessageHandler(t *testing.T) {
	withMessage := messageHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Grpc-Metadata-X-Funnel-Message", "Task is already in COMPLETE state, no action needed")
		w.Header().Set("X-Other", "kept")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	w := httptest.NewRecorder()
	withMessage.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/tasks/x:cancel", nil))
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["message"] != "Task is already in COMPLETE state, no action needed" {
		t.Errorf("body %q", w.Body.String())
	}
	if w.Header().Get("X-Other") != "kept" || w.Header().Get("Grpc-Metadata-X-Funnel-Message") != "" {
		t.Errorf("headers %v", w.Header())
	}

	plain := messageHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Other", "kept")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"1"}`))
	}))
	w = httptest.NewRecorder()
	plain.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/tasks", nil))
	if w.Code != http.StatusCreated || w.Body.String() != `{"id":"1"}` || w.Header().Get("X-Other") != "kept" {
		t.Errorf("got %d %q %v", w.Code, w.Body.String(), w.Header())
	}
}

func TestAddHeaderToCtx(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer x")
	req = (&Server{}).addHeadertoCtx(req)
	md, ok := metadata.FromOutgoingContext(req.Context())
	if !ok || len(md.Get("authorization")) != 1 || md.Get("authorization")[0] != "Bearer x" {
		t.Errorf("metadata %v", md)
	}
}

func TestGetServiceInfo(t *testing.T) {
	ts := &TaskService{Name: "Test Funnel", Config: config.DefaultConfig()}
	info, err := ts.GetServiceInfo(context.Background(), &tes.GetServiceInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "Test Funnel" || info.Version != version.Version || info.Type.Artifact != "tes" {
		t.Errorf("got %+v", info)
	}
}

// completeReader is a task database with one finished task.
type completeReader struct{ notFoundReader }

func (completeReader) GetTask(context.Context, *tes.GetTaskRequest) (*tes.Task, error) {
	return &tes.Task{Id: "done", State: tes.Complete}, nil
}

func TestCancelFinishedTask(t *testing.T) {
	ts := &TaskService{Read: completeReader{}, Config: config.DefaultConfig(), Log: logger.NewLogger("test", logger.DefaultConfig())}
	// Without an event writer: a finished task must not be changed.
	if _, err := ts.CancelTask(context.Background(), &tes.CancelTaskRequest{Id: "done"}); err != nil {
		t.Fatal(err)
	}
}

func TestCreateTaskResourceLimits(t *testing.T) {
	conf := config.DefaultConfig()
	conf.Compute = "kubernetes"
	conf.Kubernetes.Resources = &config.KubernetesResources{
		Limits: &config.ResourceLimits{Cpus: "2", RamGb: "4G", DiskGb: "10G"},
	}
	ts := &TaskService{Config: conf, Compute: noopComputer{}, Log: logger.NewLogger("test", logger.DefaultConfig())}
	task := &tes.Task{
		Executors: []*tes.Executor{{Image: "alpine", Command: []string{"true"}}},
		Resources: &tes.Resources{CpuCores: 64},
	}
	_, err := ts.CreateTask(context.Background(), task)
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument || st.Message()[:16] != "FUNNEL-TASK-003:" {
		t.Fatalf("got %v %q", st.Code(), st.Message())
	}
}

// noopComputer is a compute backend that accepts every task.
type noopComputer struct{}

func (noopComputer) WriteEvent(context.Context, *events.Event) error { return nil }
func (noopComputer) Close()                                          {}
func (noopComputer) CheckBackendParameterSupport(*tes.Task) error    { return nil }

// finishingReader is a task database with a task that finishes between the
// two reads of CancelTask.
type finishingReader struct {
	notFoundReader
	reads *int
}

func (r finishingReader) GetTask(context.Context, *tes.GetTaskRequest) (*tes.Task, error) {
	*r.reads++
	if *r.reads <= 1 {
		return &tes.Task{Id: "racing", State: tes.Running}, nil
	}
	return &tes.Task{Id: "racing", State: tes.Complete}, nil
}

// transitionErrorWriter rejects the state change, as a database does when the
// task has already finished.
type transitionErrorWriter struct{}

func (transitionErrorWriter) WriteEvent(context.Context, *events.Event) error {
	return &tes.TransitionError{From: tes.Complete, To: tes.Canceled}
}

func (transitionErrorWriter) Close() {}

func TestCancelTaskFinishingMeanwhile(t *testing.T) {
	reads := 0
	ts := &TaskService{
		Read:   finishingReader{reads: &reads},
		Event:  transitionErrorWriter{},
		Config: config.DefaultConfig(),
		Log:    logger.NewLogger("test", logger.DefaultConfig()),
	}
	if _, err := ts.CancelTask(context.Background(), &tes.CancelTaskRequest{Id: "racing"}); err != nil {
		t.Fatalf("a task that finished meanwhile: %v", err)
	}

	// A task that is still running when the update fails is an error.
	reads = -10
	if _, err := ts.CancelTask(context.Background(), &tes.CancelTaskRequest{Id: "racing"}); err == nil {
		t.Fatal("expected the update error")
	}
}
