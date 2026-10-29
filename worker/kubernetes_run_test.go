package worker

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ohsu-comp-bio/funnel/tes"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func executorTemplate(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile("../config/kubernetes/executor-job.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func newRunCommand(clientset *fake.Clientset, template string) KubernetesCommand {
	return KubernetesCommand{
		TaskId:         "task1",
		Namespace:      testNamespace,
		JobsNamespace:  testNamespace,
		TaskTemplate:   template,
		PVCMode:        "shared",
		Resources:      &tes.Resources{},
		ResourceLimits: &tes.Resources{},
		Clientset:      clientset,
		Command: Command{
			Image:        "alpine",
			ShellCommand: []string{"echo", "hello world"},
			Stdout:       io.Discard,
			Stderr:       io.Discard,
		},
	}
}

func runBriefly(kcmd KubernetesCommand) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	return kcmd.Run(ctx)
}

func TestKubernetesRunTemplateErrors(t *testing.T) {
	for _, tc := range []struct {
		name, template, reason string
	}{
		{"invalid template", "{{ .TaskId ", "TemplateParsingFailed"},
		{"template fails", "{{ index .Command 99 }}", "TemplateExecutionFailed"},
		{"not a Kubernetes object", "just text", "JobCreationFailed"},
		{"not a Job", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: x\n", "JobCreationFailed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := runBriefly(newRunCommand(fake.NewSimpleClientset(), tc.template))
			var sysErr *K8sSystemErr
			if !errors.As(err, &sysErr) || sysErr.Reason != tc.reason {
				t.Fatalf("got %v, want reason %s", err, tc.reason)
			}
			if !strings.Contains(sysErr.Error(), tc.reason) {
				t.Errorf("message %q", sysErr.Error())
			}
		})
	}
}

func TestKubernetesRunCreateFails(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("create", "jobs", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("forbidden: quota exceeded")
	})
	err := runBriefly(newRunCommand(clientset, executorTemplate(t)))
	var sysErr *K8sSystemErr
	if !errors.As(err, &sysErr) || sysErr.Reason != "JobCreationFailed" || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("got %v", err)
	}
}

// Stdio redirects are turned into one shell command with quoted arguments.
func TestKubernetesRunRedirects(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	kcmd := newRunCommand(clientset, executorTemplate(t))
	kcmd.StdinFile = "/in.txt"
	kcmd.StdoutFile = "/out.txt"
	kcmd.StderrFile = "/err.txt"
	kcmd.ShellCommand = []string{"grep", "it's"}
	_ = runBriefly(kcmd)

	job, err := clientset.BatchV1().Jobs(testNamespace).Get(context.Background(), "task1-0", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cmd := strings.Join(append(job.Spec.Template.Spec.Containers[0].Command, job.Spec.Template.Spec.Containers[0].Args...), " ")
	for _, want := range []string{`'grep' 'it'\''s' < /in.txt > /out.txt 2> /err.txt`} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %q does not contain %q", cmd, want)
		}
	}
}

// A container that cannot start (e.g. the image does not exist) fails the
// executor at once with the waiting reason.
func TestKubernetesRunImagePullFailure(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	clientset.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		fw := watch.NewFake()
		go fw.Modify(&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "task1-0-abc", Namespace: testNamespace, ResourceVersion: "1",
				Labels: map[string]string{"job-name": "task1-0"}},
			Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "image not found"}},
			}}},
		})
		return true, fw, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := newRunCommand(clientset, executorTemplate(t)).Run(ctx)
	var sysErr *K8sSystemErr
	if !errors.As(err, &sysErr) || sysErr.Reason != "ImagePullBackOff" || !strings.Contains(sysErr.Message, "image not found") {
		t.Fatalf("got %v", err)
	}
}

// An executor Job left over from an earlier attempt is replaced.
func TestKubernetesRunReplacesExistingJob(t *testing.T) {
	existing := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "task1-0", Namespace: testNamespace, Labels: map[string]string{"old": "yes"}}}
	clientset := fake.NewSimpleClientset(existing)
	deleted := make(chan struct{}, 1)
	clientset.PrependWatchReactor("jobs", func(k8stesting.Action) (bool, watch.Interface, error) {
		fw := watch.NewFake()
		go func() {
			fw.Delete(existing)
			deleted <- struct{}{}
		}()
		return true, fw, nil
	})
	_ = runBriefly(newRunCommand(clientset, executorTemplate(t)))

	select {
	case <-deleted:
	default:
		t.Fatal("the existing Job was not deleted")
	}
	job, err := clientset.BatchV1().Jobs(testNamespace).Get(context.Background(), "task1-0", metav1.GetOptions{})
	if err != nil || job.Labels["old"] == "yes" {
		t.Fatalf("the Job was not recreated: %v %v", job, err)
	}
}

func TestNormalizeShellCommand(t *testing.T) {
	for in, want := range map[string]string{
		"echo hello && ls":   "echo hello && ls",
		"echo Hello O'hare!": `'echo' 'Hello' 'O'\''hare!'`,
	} {
		if got := normalizeShellCommand(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	e := &K8sSystemErr{Reason: "R", Message: "M"}
	if e.Error() != "kubernetes system error (R): M" || e.Unwrap() != nil {
		t.Errorf("%q", e.Error())
	}
}

// Outside a cluster there is no in-cluster configuration: stopping the
// executor and streaming its logs fail with an error instead of panicking.
func TestKubernetesOutsideCluster(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	kcmd := newRunCommand(fake.NewSimpleClientset(), "")
	if err := kcmd.Stop(); err == nil {
		t.Error("Stop: expected an error outside a cluster")
	}
	if err := streamPodLogs(context.Background(), testNamespace, "pod", io.Discard, io.Discard); err == nil {
		t.Error("streamPodLogs: expected an error outside a cluster")
	}
	if kcmd.GetStdout() != io.Discard || kcmd.GetStderr() != io.Discard {
		t.Error("GetStdout/GetStderr must return the command's writers")
	}
}

func TestKubernetesErrors(t *testing.T) {
	inner := errors.New("boom")
	sys := &K8sSystemErr{Reason: "R", Message: "M", Err: inner}
	if sys.Error() != "kubernetes system error (R): M: boom" || !errors.Is(sys, inner) {
		t.Errorf("%q", sys.Error())
	}
	exec := &K8sExecutorErr{ExitCode: 2, JobName: "t-0", Message: "failed"}
	if exec.Error() != "executor job t-0 failed with exit code 2 (ExitError): failed" {
		t.Errorf("%q", exec.Error())
	}
	exec.Reason = "OOMKilled"
	if !strings.Contains(exec.Error(), "(OOMKilled)") {
		t.Errorf("%q", exec.Error())
	}
}
