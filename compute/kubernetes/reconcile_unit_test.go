package kubernetes

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/events"
	"github.com/ohsu-comp-bio/funnel/logger"
	"github.com/ohsu-comp-bio/funnel/tes"
	"google.golang.org/protobuf/types/known/durationpb"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

const recNS = "jobs-ns"

func workerJob(name string, status batchv1.JobStatus) *batchv1.Job {
	backoff := int32(0)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: recNS, Labels: map[string]string{"app": "funnel-worker"}},
		Spec:       batchv1.JobSpec{BackoffLimit: &backoff},
		Status:     status,
	}
}

func jobPod(job string, status corev1.PodStatus) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: job + "-pod", Namespace: recNS, Labels: map[string]string{"job-name": job}},
		Status:     status,
	}
}

func newReconcileBackend(t *testing.T, tasks []*tes.Task, objects ...runtime.Object) (*Backend, *fake.Clientset, *capturingEventWriter) {
	t.Helper()
	client := fake.NewSimpleClientset(objects...)
	conf := config.DefaultConfig()
	conf.Kubernetes.Namespace = recNS
	conf.Kubernetes.JobsNamespace = recNS
	conf.Kubernetes.PVCMode = "shared"
	w := &capturingEventWriter{}
	return &Backend{
		client:   client,
		event:    w,
		database: &mockReadOnlyServer{tasks: tasks},
		log:      logger.NewLogger("test", logger.DefaultConfig()),
		conf:     conf,
	}, client, w
}

func jobExists(client *fake.Clientset, name string) bool {
	_, err := client.BatchV1().Jobs(recNS).Get(context.Background(), name, metav1.GetOptions{})
	return err == nil
}

func systemLogField(w *capturingEventWriter, taskID, field string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, ev := range w.events {
		if ev.Id == taskID && ev.Type == events.Type_SYSTEM_LOG {
			if v := ev.GetSystemLog().GetFields()[field]; v != "" {
				return v
			}
		}
	}
	return ""
}

func TestReconcileJob(t *testing.T) {
	ctx := context.Background()
	done := []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}
	waiting := corev1.PodStatus{Phase: corev1.PodPending, ContainerStatuses: []corev1.ContainerStatus{{
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CreateContainerConfigError", Message: "secret not found"}},
	}}}
	unschedulable := corev1.PodStatus{Phase: corev1.PodPending, Conditions: []corev1.PodCondition{{
		Type: corev1.PodScheduled, Status: corev1.ConditionFalse, LastTransitionTime: metav1.NewTime(time.Now().Add(-time.Hour)),
	}}}
	crashed := corev1.PodStatus{Phase: corev1.PodFailed, ContainerStatuses: []corev1.ContainerStatus{{
		State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 3, Reason: "Error", Message: "worker crashed", FinishedAt: metav1.Now()}},
	}}}

	for _, tc := range []struct {
		name        string
		status      batchv1.JobStatus
		pod         *corev1.PodStatus
		timeout     bool
		systemError bool
		deleted     bool
		field       string // a system log field that must be set
	}{
		{"terminal waiting error", batchv1.JobStatus{Active: 1}, &waiting, false, true, true, "error"},
		{"scheduling timed out", batchv1.JobStatus{Active: 1}, &unschedulable, true, true, true, "error"},
		{"active and fine", batchv1.JobStatus{Active: 1}, nil, false, false, false, ""},
		{"succeeded", batchv1.JobStatus{Succeeded: 1}, nil, false, false, true, ""},
		{"failed, still retrying", batchv1.JobStatus{Failed: 1}, nil, false, false, false, ""},
		{"failed and active", batchv1.JobStatus{Failed: 1, Active: 1}, nil, false, false, false, ""},
		{"failed for good", batchv1.JobStatus{Failed: 1, Conditions: done}, &crashed, false, true, true, "executor_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			job := workerJob("task1", tc.status)
			objects := []runtime.Object{job}
			if tc.pod != nil {
				objects = append(objects, jobPod("task1", *tc.pod))
			}
			b, client, w := newReconcileBackend(t, []*tes.Task{{Id: "task1", State: tes.Running}}, objects...)
			if tc.timeout {
				b.conf.Kubernetes.Timeout = &config.TimeoutConfig{TimeoutOption: &config.TimeoutConfig_Duration{Duration: durationpb.New(time.Minute)}}
			}

			b.reconcileJob(ctx, job, false)

			if got := w.hasSystemError("task1"); got != tc.systemError {
				t.Errorf("SYSTEM_ERROR written: %v, want %v (%v)", got, tc.systemError, w.events)
			}
			if got := !jobExists(client, "task1"); got != tc.deleted {
				t.Errorf("job deleted: %v, want %v", got, tc.deleted)
			}
			if tc.field != "" && systemLogField(w, "task1", tc.field) == "" {
				t.Errorf("system log lacks %q", tc.field)
			}
		})
	}

	t.Run("failed for good, cleanup disabled", func(t *testing.T) {
		job := workerJob("task1", batchv1.JobStatus{Failed: 1, Conditions: done})
		b, client, w := newReconcileBackend(t, nil, job)
		b.reconcileJob(ctx, job, true)
		if !w.hasSystemError("task1") || !jobExists(client, "task1") {
			t.Error("the error is reported but the job is kept")
		}
		if !strings.Contains(systemLogField(w, "task1", "error"), "Failed") {
			t.Errorf("the job conditions should be logged: %q", systemLogField(w, "task1", "error"))
		}
	})
}

func TestReconcileOnce(t *testing.T) {
	done := []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
	b, client, w := newReconcileBackend(t,
		[]*tes.Task{{Id: "no-job", State: tes.Queued}, {Id: "has-job", State: tes.Running}},
		workerJob("has-job", batchv1.JobStatus{Active: 1}),
		workerJob("orphan-done", batchv1.JobStatus{Succeeded: 1, Conditions: done}),
		workerJob("orphan-running", batchv1.JobStatus{Active: 1}),
	)
	b.reconcileOnce(context.Background(), false)

	if !w.hasSystemError("no-job") {
		t.Error("a task without a job must fail")
	}
	if w.hasSystemError("has-job") || !jobExists(client, "has-job") {
		t.Error("a task with a running job must be left alone")
	}
	if jobExists(client, "orphan-done") {
		t.Error("a finished job without a task must be cleaned up")
	}
	if !jobExists(client, "orphan-running") {
		t.Error("a job that is still running must not be cleaned up")
	}
}

func TestCleanBacklog(t *testing.T) {
	b, client, _ := newReconcileBackend(t,
		[]*tes.Task{{Id: "known", State: tes.Running}},
		workerJob("completed", batchv1.JobStatus{Succeeded: 1}),
		workerJob("orphan", batchv1.JobStatus{Active: 1}),
		workerJob("known", batchv1.JobStatus{Active: 1}),
	)
	// Without DisableJobCleanup the reconciler cleans up; nothing to do here.
	b.cleanBacklog(context.Background(), false)
	if !jobExists(client, "completed") {
		t.Fatal("cleanBacklog must do nothing when job cleanup is enabled")
	}

	b.cleanBacklog(context.Background(), true)
	if jobExists(client, "completed") || jobExists(client, "orphan") {
		t.Error("completed and orphaned jobs must be deleted")
	}
	if !jobExists(client, "known") {
		t.Error("the job of a known task must be kept")
	}
}

func TestWriteEventCancel(t *testing.T) {
	b, client, _ := newReconcileBackend(t, []*tes.Task{{Id: "task1", State: tes.Running}}, workerJob("task1", batchv1.JobStatus{Active: 1}))
	if err := b.WriteEvent(context.Background(), events.NewState("task1", tes.Canceled)); err != nil {
		t.Fatal(err)
	}
	if jobExists(client, "task1") {
		t.Error("cancel must delete the worker job")
	}
	// Other events are ignored.
	if err := b.WriteEvent(context.Background(), events.NewState("task1", tes.Running)); err != nil {
		t.Error(err)
	}
}

func TestCheckBackendParameterSupport(t *testing.T) {
	b := Backend{backendParameters: map[string]string{"queue": ""}}
	strict := &tes.Task{Resources: &tes.Resources{BackendParametersStrict: true, BackendParameters: map[string]string{"queue": "a"}}}
	if err := b.CheckBackendParameterSupport(strict); err != nil {
		t.Errorf("supported parameter: %v", err)
	}
	strict.Resources.BackendParameters["gpu"] = "1"
	if err := b.CheckBackendParameterSupport(strict); err == nil {
		t.Error("an unsupported parameter must be rejected in strict mode")
	}
	strict.Resources.BackendParametersStrict = false
	if err := b.CheckBackendParameterSupport(strict); err != nil {
		t.Errorf("non-strict: %v", err)
	}
}

func TestNewBackendConfigErrors(t *testing.T) {
	for name, modify := range map[string]func(*config.Config){
		"no worker template": func(c *config.Config) { c.Kubernetes.WorkerTemplate = "" },
		"no namespace":       func(c *config.Config) { c.Kubernetes.Namespace = "" },
		"invalid PVC mode":   func(c *config.Config) { c.Kubernetes.PVCMode = "everything" },
	} {
		t.Run(name, func(t *testing.T) {
			conf := config.DefaultConfig()
			conf.Kubernetes.WorkerTemplate = "apiVersion: batch/v1\nkind: Job\n"
			conf.Kubernetes.Namespace = "ns"
			modify(conf)
			if _, err := NewBackend(context.Background(), conf, nil, nil, logger.NewLogger("test", logger.DefaultConfig())); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestGetFailedPodInfo(t *testing.T) {
	older := metav1.NewTime(time.Now().Add(-time.Minute))
	newer := metav1.Now()
	pods := &corev1.PodList{Items: []corev1.Pod{
		*jobPod("a", corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1, Message: "old", FinishedAt: older}}}}}),
		*jobPod("b", corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 137, Reason: "OOMKilled", Message: "new", FinishedAt: newer}}}}}),
		*jobPod("c", corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0, FinishedAt: newer}}}}}),
	}}
	b := &Backend{}
	if got := b.getFailedPodInfo(context.Background(), pods); got != "exit code 137 (OOMKilled): new" {
		t.Errorf("got %q", got)
	}
	if got := b.getFailedPodInfo(context.Background(), &corev1.PodList{}); got != "" {
		t.Errorf("no pods: %q", got)
	}
}
