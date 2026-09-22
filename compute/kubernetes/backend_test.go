package kubernetes

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	v1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/ohsu-comp-bio/funnel/config"
	"github.com/ohsu-comp-bio/funnel/logger"
	"github.com/ohsu-comp-bio/funnel/tes"
)

// TestNewBackend_PVCModeValidation verifies NewBackend rejects invalid
// PVCMode values and a "pvc" mode missing StorageClassName, before it ever
// tries to load a kubeconfig (so this is testable without a real cluster).
func TestNewBackend_PVCModeValidation(t *testing.T) {
	content, err := os.ReadFile("../../config/kubernetes-template.yaml")
	if err != nil {
		t.Fatal(fmt.Errorf("reading template: %v", err))
	}
	log := logger.NewLogger("test", logger.DefaultConfig())
	base := config.Kubernetes{
		Template:  string(content),
		Namespace: "default",
	}

	t.Run("rejects unknown PVCMode", func(t *testing.T) {
		conf := base
		conf.PVCMode = "bogus"
		_, err := NewBackend(context.Background(), conf, nil, nil, log)
		if err == nil {
			t.Fatal("expected an error for an invalid PVCMode, got nil")
		}
		t.Logf("got expected error: %v", err)
	})

	t.Run("rejects pvc mode without StorageClassName", func(t *testing.T) {
		conf := base
		conf.PVCMode = PVCModePVC
		conf.StorageClassName = ""
		_, err := NewBackend(context.Background(), conf, nil, nil, log)
		if err == nil {
			t.Fatal("expected an error for PVCMode=pvc without StorageClassName, got nil")
		}
		t.Logf("got expected error: %v", err)
	})

	t.Run("accepts pvc mode with StorageClassName (fails later, on kubeconfig, not validation)", func(t *testing.T) {
		conf := base
		conf.PVCMode = PVCModePVC
		conf.StorageClassName = "nfs-csi"
		_, err := NewBackend(context.Background(), conf, nil, nil, log)
		if err == nil {
			t.Fatal("expected an error (no in-cluster kubeconfig available in test env), got nil")
		}
		if strings.Contains(err.Error(), "PVCMode") || strings.Contains(err.Error(), "StorageClassName") {
			t.Errorf("expected the error to come from kubeconfig setup, not validation: %v", err)
		}
		t.Logf("got expected (non-validation) error: %v", err)
	})

	t.Run("empty PVCMode defaults to full (fails later, on kubeconfig, not validation)", func(t *testing.T) {
		conf := base
		conf.PVCMode = ""
		_, err := NewBackend(context.Background(), conf, nil, nil, log)
		if err == nil {
			t.Fatal("expected an error (no in-cluster kubeconfig available in test env), got nil")
		}
		if strings.Contains(err.Error(), "PVCMode") || strings.Contains(err.Error(), "StorageClassName") {
			t.Errorf("expected the error to come from kubeconfig setup, not validation: %v", err)
		}
		t.Logf("got expected (non-validation) error: %v", err)
	})
}

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
// and a per-task PVC name based on PVCMode.
func TestPVCName(t *testing.T) {
	cases := []struct {
		name    string
		pvcMode string
		taskID  string
		want    string
	}{
		{"full: per-task PVC", PVCModeFull, "task1", "funnel-pvc-task1"},
		{"pvc: per-task PVC", PVCModePVC, "task1", "funnel-pvc-task1"},
		{"shared: shared PVC", PVCModeShared, "task1", "funnel-pvc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &Backend{pvcMode: c.pvcMode}
			got := b.pvcName(c.taskID)
			if got != c.want {
				t.Errorf("pvcName(%q) = %q, want %q", c.taskID, got, c.want)
			}
		})
	}
}

// TestCreateJob_PVCName verifies the rendered Worker Job mounts the correct
// PVC depending on PVCMode: a dedicated per-task PVC for "full"/"pvc", or
// the shared "funnel-pvc" for "shared".
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
		name    string
		pvcMode string
		want    string
	}{
		{"full: mounts per-task PVC", PVCModeFull, "funnel-pvc-task1"},
		{"pvc: mounts per-task PVC", PVCModePVC, "funnel-pvc-task1"},
		{"shared: mounts shared PVC", PVCModeShared, "funnel-pvc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &Backend{
				namespace: "default",
				template:  string(content),
				pvcMode:   c.pvcMode,
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

// TestCreatePVC_Full verifies that in PVCModeFull, createPVC renders a PVC
// statically bound (via volumeName) to a PV, and createPV renders that PV,
// backed by the S3 CSI driver - both named after the task ID.
func TestCreatePVC_Full(t *testing.T) {
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
		pvcMode:     PVCModeFull,
		// storageClassName intentionally left empty: PVCModeFull must fall
		// back to the static volumeName binding, not dynamic provisioning.
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
	if pvc.Spec.StorageClassName != nil {
		t.Errorf("PVC storageClassName = %v, want nil (static binding)", *pvc.Spec.StorageClassName)
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

// TestCreatePVC_Dynamic verifies that in PVCModePVC, createPVC renders a PVC
// with storageClassName set (dynamic provisioning) instead of a static
// volumeName binding - no PV is involved.
func TestCreatePVC_Dynamic(t *testing.T) {
	pvcContent, err := os.ReadFile("../../config/kubernetes-pvc.yaml")
	if err != nil {
		t.Fatal(fmt.Errorf("reading PVC template: %v", err))
	}

	b := &Backend{
		namespace:        "default",
		pvcTemplate:      string(pvcContent),
		pvcMode:          PVCModePVC,
		storageClassName: "nfs-csi",
	}

	task := &tes.Task{Id: "task1"}

	pvc, err := b.createPVC(task)
	if err != nil {
		t.Fatal(err)
	}
	if pvc.Name != "funnel-pvc-task1" {
		t.Errorf("PVC name = %q, want %q", pvc.Name, "funnel-pvc-task1")
	}
	if pvc.Spec.VolumeName != "" {
		t.Errorf("PVC volumeName = %q, want empty (dynamic provisioning)", pvc.Spec.VolumeName)
	}
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "nfs-csi" {
		t.Errorf("PVC storageClassName = %v, want %q", pvc.Spec.StorageClassName, "nfs-csi")
	}
}

// TestSubmit_PVCModes verifies Submit()'s PVCModeShared branch end-to-end
// against a fake Kubernetes API: it creates only the worker Job (no PVC),
// and the Job mounts the shared "funnel-pvc". It also proves PVCModeShared
// never touches b.config (kubernetes.NewForConfig), since it's left nil
// here. PVCModeFull/PVCModePVC's object creation isn't exercised through
// Submit() here because they call kubernetes.NewForConfig(b.config) - a real
// *rest.Config - separately from the fake b.client injected below (a
// pre-existing architectural quirk, not introduced by PVCMode); their
// rendering logic is covered directly by TestCreatePVC_Full/Dynamic instead.
func TestSubmit_PVCModes(t *testing.T) {
	jobContent, err := os.ReadFile("../../config/kubernetes-template.yaml")
	if err != nil {
		t.Fatal(fmt.Errorf("reading job template: %v", err))
	}
	log := logger.NewLogger("test", logger.DefaultConfig())

	task := &tes.Task{
		Id: "task1",
		Executors: []*tes.Executor{
			{Image: "alpine", Command: []string{"echo", "hello world"}},
		},
	}

	t.Run("shared: creates neither PVC nor PV", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		b := &Backend{
			namespace: "default",
			template:  string(jobContent),
			pvcMode:   PVCModeShared,
			client:    clientset.BatchV1().Jobs("default"),
			log:       log,
			// config intentionally left nil: PVCModeShared must never call
			// kubernetes.NewForConfig(b.config).
		}

		if err := b.Submit(context.Background(), task); err != nil {
			t.Fatal(err)
		}

		job, err := clientset.BatchV1().Jobs("default").Get(context.Background(), "task1", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("expected Job to be created: %v", err)
		}
		if got := jobPVCClaimName(t, job); got != "funnel-pvc" {
			t.Errorf("worker Job PVC claimName = %q, want %q", got, "funnel-pvc")
		}

		pvcs, err := clientset.CoreV1().PersistentVolumeClaims("default").List(context.Background(), metav1.ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(pvcs.Items) != 0 {
			t.Errorf("expected no PVCs to be created, got %d", len(pvcs.Items))
		}
	})
}
