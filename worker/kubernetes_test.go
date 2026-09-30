package worker

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"text/template"

	"github.com/ohsu-comp-bio/funnel/tes"
	v1 "k8s.io/api/batch/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

// TestKubernetesCommand_pvcName verifies pvcName switches between the shared
// PVC name and a per-task PVC name based on PVCMode, matching the
// server-side Backend.pvcName convention (compute/kubernetes/backend.go).
func TestKubernetesCommand_pvcName(t *testing.T) {
	cases := []struct {
		name    string
		pvcMode string
		want    string
	}{
		{"full: per-task PVC", "full", "funnel-pvc-task1"},
		{"pvc: per-task PVC", "pvc", "funnel-pvc-task1"},
		{"shared: shared PVC", "shared", "funnel-pvc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kcmd := KubernetesCommand{TaskId: "task1", PVCMode: c.pvcMode}
			got := kcmd.pvcName()
			if got != c.want {
				t.Errorf("pvcName() = %q, want %q", got, c.want)
			}
		})
	}
}

// renderExecutorJob mirrors the template rendering step in
// KubernetesCommand.Run (kubernetes.go), stopping short of talking to a
// Kubernetes API, so the PVC wiring can be checked directly.
func renderExecutorJob(t *testing.T, kcmd KubernetesCommand) *v1.Job {
	t.Helper()

	tpl, err := template.New(kcmd.TaskId).Parse(kcmd.TaskTemplate)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err = tpl.Execute(&buf, map[string]interface{}{
		"TaskId":    kcmd.TaskId,
		"JobId":     kcmd.JobId,
		"Namespace": kcmd.Namespace,
		// NOTE: left empty deliberately. "args: {{.Command}}" in
		// kubernetes-executor-template.yaml renders .Command as a bare YAML
		// scalar, which the Kubernetes API's typed decoder cannot unmarshal
		// into Container.Args ([]string) unless it's empty/null. See the bug
		// noted in the test report - unrelated to PVCName, out of scope here.
		"Command":        "",
		"Env":            kcmd.Env,
		"Workdir":        kcmd.Workdir,
		"Volumes":        kcmd.Volumes,
		"Cpus":           kcmd.Resources.CpuCores,
		"RamGb":          kcmd.Resources.RamGb,
		"DiskGb":         kcmd.Resources.DiskGb,
		"ServiceAccount": kcmd.ServiceAccount,
		"Image":          kcmd.Image,
		"PVCName":        kcmd.pvcName(),
	})
	if err != nil {
		t.Fatal(err)
	}

	decode := scheme.Codecs.UniversalDeserializer().Decode
	obj, _, err := decode(buf.Bytes(), nil, nil)
	if err != nil {
		t.Fatalf("decoding job spec: %v\n%s", err, buf.String())
	}
	job, ok := obj.(*v1.Job)
	if !ok {
		t.Fatalf("decoded object is not a Job: %T", obj)
	}
	return job
}

func executorJobPVCClaimName(job *v1.Job) string {
	for _, v := range job.Spec.Template.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			return v.PersistentVolumeClaim.ClaimName
		}
	}
	return ""
}

// TestExecutorJob_PVCName verifies the rendered Executor Job mounts the
// correct PVC depending on PVCMode, using the real
// kubernetes-executor-template.yaml shipped with Funnel.
func TestExecutorJob_PVCName(t *testing.T) {
	content, err := os.ReadFile("../config/kubernetes-executor-template.yaml")
	if err != nil {
		t.Fatal(fmt.Errorf("reading executor template: %v", err))
	}

	cases := []struct {
		name    string
		pvcMode string
		want    string
	}{
		{"full: mounts per-task PVC", "full", "funnel-pvc-task1"},
		{"pvc: mounts per-task PVC", "pvc", "funnel-pvc-task1"},
		{"shared: mounts shared PVC", "shared", "funnel-pvc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kcmd := KubernetesCommand{
				TaskId:       "task1",
				JobId:        0,
				Namespace:    "default",
				TaskTemplate: string(content),
				PVCMode:      c.pvcMode,
				Resources:    &tes.Resources{},
				Command:      Command{Image: "alpine", ShellCommand: []string{"echo", "hello"}},
			}
			job := renderExecutorJob(t, kcmd)
			got := executorJobPVCClaimName(job)
			if got != c.want {
				t.Errorf("executor Job PVC claimName = %q, want %q", got, c.want)
			}
		})
	}
}
