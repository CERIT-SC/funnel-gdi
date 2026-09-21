package kubernetes

import (
	"context"
	"fmt"
	"os"
	"testing"

	v1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/logger"
	"github.com/ohsu-comp-bio/funnel/tes"
)

func TestCreateJobc(t *testing.T) {
	conf := config.DefaultConfig().Kubernetes
	content, err := os.ReadFile("../../config/kubernetes-template.yaml")
	if err != nil {
		t.Fatal(fmt.Errorf("reading template: %v", err))
	}
	conf.Template = string(content)
	log := logger.NewLogger("test", logger.DefaultConfig())
	b := &Backend{
		client:    nil,
		namespace: conf.Namespace,
		template:  conf.Template,
		event:     nil,
		database:  nil,
		log:       log,
	}

	task := &tes.Task{
		Id: "task1",
		Executors: []*tes.Executor{
			{
				Image:   "alpine",
				Command: []string{"echo", "hello world"},
			},
		},
	}

	job, err := b.createJob(task)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", job)
}

// jobPVCClaimName returns the PersistentVolumeClaim.ClaimName mounted by the
// job's "funnel-storage-<taskId>" volume, or "" if not found.
func jobPVCClaimName(t *testing.T, job *v1.Job) string {
	t.Helper()
	for _, v := range job.Spec.Template.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			return v.PersistentVolumeClaim.ClaimName
		}
	}
	return ""
}

// TestPVCName verifies Backend.pvcName switches between the shared PVC name
// and a per-task PVC name based on DisablePV.
func TestPVCName(t *testing.T) {
	cases := []struct {
		name      string
		disablePV bool
		taskID    string
		want      string
	}{
		{"disabled: shared PVC", true, "task1", "funnel-pvc"},
		{"enabled: per-task PVC", false, "task1", "funnel-pvc-task1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &Backend{disablePV: c.disablePV}
			got := b.pvcName(c.taskID)
			if got != c.want {
				t.Errorf("pvcName(%q) = %q, want %q", c.taskID, got, c.want)
			}
		})
	}
}

// TestCreateJob_PVCName verifies the rendered Worker Job mounts the correct
// PVC depending on DisablePV: the shared "funnel-pvc" when disabled, or a
// dedicated per-task PVC when enabled (upstream default behavior).
func TestCreateJob_PVCName(t *testing.T) {
	content, err := os.ReadFile("../../config/kubernetes-template.yaml")
	if err != nil {
		t.Fatal(fmt.Errorf("reading template: %v", err))
	}
	log := logger.NewLogger("test", logger.DefaultConfig())
	task := &tes.Task{
		Id: "task1",
		Executors: []*tes.Executor{
			{Image: "alpine", Command: []string{"echo", "hello world"}},
		},
	}

	cases := []struct {
		name      string
		disablePV bool
		want      string
	}{
		{"disabled: mounts shared PVC", true, "funnel-pvc"},
		{"enabled: mounts per-task PVC", false, "funnel-pvc-task1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &Backend{
				namespace: "default",
				template:  string(content),
				disablePV: c.disablePV,
				log:       log,
			}
			job, err := b.createJob(task)
			if err != nil {
				t.Fatal(err)
			}
			got := jobPVCClaimName(t, job)
			if got != c.want {
				t.Errorf("worker Job PVC claimName = %q, want %q", got, c.want)
			}
		})
	}
}

// TestCreatePVC_CreatePV verifies the per-task PVC/PV pair (used when
// DisablePV is false, i.e. upstream's default behavior) still renders
// correctly: a PVC statically bound (via volumeName) to a PV backed by the
// S3 CSI driver, both named after the task ID.
func TestCreatePVC_CreatePV(t *testing.T) {
	pvcContent, err := os.ReadFile("../../config/kubernetes-pvc.yaml")
	if err != nil {
		t.Fatal(fmt.Errorf("reading PVC template: %v", err))
	}
	pvContent, err := os.ReadFile("../../config/kubernetes-pv.yaml")
	if err != nil {
		t.Fatal(fmt.Errorf("reading PV template: %v", err))
	}

	b := &Backend{
		namespace:   "default",
		bucket:      "test-bucket",
		region:      "us-west-2",
		pvcTemplate: string(pvcContent),
		pvTemplate:  string(pvContent),
	}

	task := &tes.Task{Id: "task1"}

	pvc, err := b.createPVC(task)
	if err != nil {
		t.Fatal(err)
	}
	if pvc.Name != "funnel-pvc-task1" {
		t.Errorf("PVC name = %q, want %q", pvc.Name, "funnel-pvc-task1")
	}
	if pvc.Spec.VolumeName != "funnel-pv-task1" {
		t.Errorf("PVC volumeName = %q, want %q", pvc.Spec.VolumeName, "funnel-pv-task1")
	}

	pv, err := b.createPV(task)
	if err != nil {
		t.Fatal(err)
	}
	if pv.Name != "funnel-pv-task1" {
		t.Errorf("PV name = %q, want %q", pv.Name, "funnel-pv-task1")
	}
	if pv.Spec.ClaimRef == nil || pv.Spec.ClaimRef.Name != "funnel-pvc-task1" {
		t.Errorf("PV claimRef = %+v, want name %q", pv.Spec.ClaimRef, "funnel-pvc-task1")
	}
	if pv.Spec.CSI == nil || pv.Spec.CSI.VolumeAttributes["bucketName"] != "test-bucket" {
		t.Errorf("PV CSI bucketName = %+v, want %q", pv.Spec.CSI, "test-bucket")
	}
}

// TestSubmit_DisablePV verifies that when DisablePV is true, Submit creates
// only the worker Job (no PVC/PV API calls) and the Job mounts the shared
// "funnel-pvc". This exercises the real Submit() branch end-to-end against a
// fake Kubernetes API, proving it never needs b.config (a working
// *rest.Config) when DisablePV is set.
func TestSubmit_DisablePV(t *testing.T) {
	content, err := os.ReadFile("../../config/kubernetes-template.yaml")
	if err != nil {
		t.Fatal(fmt.Errorf("reading template: %v", err))
	}

	clientset := fake.NewSimpleClientset()
	log := logger.NewLogger("test", logger.DefaultConfig())

	b := &Backend{
		namespace: "default",
		template:  string(content),
		disablePV: true,
		client:    clientset.BatchV1().Jobs("default"),
		log:       log,
		// config intentionally left nil: disablePV=true must never call
		// kubernetes.NewForConfig(b.config).
	}

	task := &tes.Task{
		Id: "task1",
		Executors: []*tes.Executor{
			{Image: "alpine", Command: []string{"echo", "hello world"}},
		},
	}

	if err := b.Submit(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	job, err := clientset.BatchV1().Jobs("default").Get(context.Background(), "task1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected Job to be created: %v", err)
	}
	got := jobPVCClaimName(t, job)
	if got != "funnel-pvc" {
		t.Errorf("worker Job PVC claimName = %q, want %q", got, "funnel-pvc")
	}

	// No PVC should have been created for the task.
	pvcs, err := clientset.CoreV1().PersistentVolumeClaims("default").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pvcs.Items) != 0 {
		t.Errorf("expected no PVCs to be created, got %d", len(pvcs.Items))
	}
}
