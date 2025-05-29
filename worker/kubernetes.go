package worker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/ohsu-comp-bio/funnel/tes"
	v1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// KubernetesCommand is responsible for configuring and running a task in a Kubernetes cluster.
type KubernetesCommand struct {
	TaskId         string
	JobId          int
	StdinFile      string
	TaskTemplate   string
	Namespace      string
	Resources      *tes.Resources
	ServiceAccount string
	Clientset      kubernetes.Interface
	Command
}

// Create the Executor K8s job from kubernetes-executor-template.yaml
// Funnel Worker job is created in compute/kubernetes/backend.go#createJob
func (kcmd KubernetesCommand) Run(ctx context.Context) error {
	var taskId = kcmd.TaskId
	tpl, err := template.New(taskId).Parse(kcmd.TaskTemplate)

	if err != nil {
		return err
	}

	var command = kcmd.ShellCommand
	if kcmd.StdinFile != "" {
		command = append(command, "<", kcmd.StdinFile)
	}
	for i, v := range command {
		if strings.Contains(v, " ") {
			command[i] = fmt.Sprintf("'%s'", v)
		}
	}

	var buf bytes.Buffer
	err = tpl.Execute(&buf, map[string]interface{}{
		"TaskId":         taskId,
		"JobId":          kcmd.JobId,
		"Namespace":      kcmd.Namespace,
		"Command":        strings.Join(command, " "),
		"Env":            kcmd.Env,
		"Workdir":        kcmd.Workdir,
		"Volumes":        kcmd.Volumes,
		"Cpus":           kcmd.Resources.CpuCores,
		"RamGb":          kcmd.Resources.RamGb,
		"DiskGb":         kcmd.Resources.DiskGb,
		"ServiceAccount": kcmd.ServiceAccount,
		"Image":          kcmd.Image,
	})

	if err != nil {
		return err
	}

	decode := scheme.Codecs.UniversalDeserializer().Decode
	obj, _, err := decode(buf.Bytes(), nil, nil)
	if err != nil {
		return err
	}

	job, ok := obj.(*v1.Job)
	if !ok {
		return err
	}

	clientset := kcmd.Clientset
	if clientset == nil {
		var err error
		clientset, err = getKubernetesClientset()
		if err != nil {
			return err
		}
	}

	var client = clientset.BatchV1().Jobs(kcmd.Namespace)

	job, err = client.Create(ctx, job, metav1.CreateOptions{})
	var maxRetries = 5

	if err != nil {
		// Retry creating the Executor Pod on failure
		var retryCount int
		for retryCount < maxRetries {
			_, err = client.Create(ctx, job, metav1.CreateOptions{})
			if err == nil {
				break
			}
			retryCount++
			time.Sleep(2 * time.Second)
		}
		if retryCount == maxRetries {
			return fmt.Errorf("Funnel Worker: Failed to create Executor Job after %v attempts: %v", maxRetries, err)
		}
	}
	var jobName = fmt.Sprintf("%s-%d", taskId, kcmd.JobId)

	waitForJobPodStart(ctx, kcmd.Namespace, jobName)

	// Wait until the job finishes
	watcher, err := client.Watch(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("job-name=%s", jobName)})
	defer watcher.Stop()

	pods, err := clientset.CoreV1().Pods(kcmd.Namespace).List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("job-name=%s", jobName)})
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(pods.Items))

	for _, v := range pods.Items {
		wg.Add(1)
		go func(podName string) {
			defer wg.Done()
			// Wait for the pod to reach Running state
			pod, err := waitForPodRunning(ctx, kcmd.Namespace, podName, 5*time.Minute)
			if err != nil {
				errChan <- fmt.Errorf("error waiting for pod %s: %v", podName, err)
				return
			}

			// Stream logs from the running pod
			err = streamPodLogs(ctx, kcmd.Namespace, pod.Name, kcmd.Stdout)
			if err != nil {
				errChan <- fmt.Errorf("error streaming logs from pod %s: %v", pod.Name, err)
			}
		}(v.Name)
	}

	// Wait for all goroutines to finish
	wg.Wait()
	close(errChan)

	// Check for errors from goroutines
	for err := range errChan {
		if err != nil {
			log.Printf("Error: %v", err)
		}
	}
	waitForJobFinish(ctx, watcher)

	return nil
}

func waitForPodRunning(ctx context.Context, namespace string, podName string, timeout time.Duration) (*corev1.Pod, error) {
	clientset, err := getKubernetesClientset()
	if err != nil {
		return nil, fmt.Errorf("failed getting kubernetes clientset: %v", err)
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeoutCh := time.After(timeout)

	for {
		select {
		case <-timeoutCh:
			return nil, fmt.Errorf("timed out waiting for pod %s to be in running state", podName)
		case <-ticker.C:
			pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("getting pod %s: %v", podName, err)
			}

			if pod.Status.Phase == corev1.PodRunning {
				return pod, nil
			} else if pod.Status.Phase == corev1.PodSucceeded {
				return pod, nil
			} else if pod.Status.Phase == corev1.PodFailed {
				return nil, fmt.Errorf("pod %s failed", podName)
			}
		}
	}
}

func streamPodLogs(ctx context.Context, namespace string, podName string, stdout io.Writer) error {
	clientset, err := getKubernetesClientset()
	if err != nil {
		return fmt.Errorf("getting kubernetes clientset: %v", err)
	}

	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{Follow: true})
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf("streaming logs: %v", err)
	}
	defer podLogs.Close()

	_, err = io.Copy(stdout, podLogs)
	return err
}

// Deletes the job running the task.
func (kcmd KubernetesCommand) Stop() error {
	clientset, err := getKubernetesClientset()
	if err != nil {
		return err
	}

	jobName := fmt.Sprintf("%s-%d", kcmd.TaskId, kcmd.JobId)

	backgroundDeletion := metav1.DeletePropagationBackground
	err = clientset.BatchV1().Jobs(kcmd.Namespace).Delete(context.TODO(), jobName, metav1.DeleteOptions{
		PropagationPolicy: &backgroundDeletion,
	})

	if err != nil {
		return fmt.Errorf("deleting job: %v", err)
	}

	return nil
}

func (kcmd KubernetesCommand) GetStdout() io.Writer {
	return kcmd.Stdout
}

func (kcmd KubernetesCommand) GetStderr() io.Writer {
	return kcmd.Stderr
}

func waitForJobPodStart(ctx context.Context, namespace string, jobName string) error {
	clientset, err := getKubernetesClientset()
	if err != nil {
		return err
	}

	ticker := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("job-name=%s", jobName)})

			if err != nil {
				return err
			}
			// Currently there should be only one pod per job
			if len(pods.Items) > 0 {
				return nil
			}
		}
	}
}

// Waits until the job finishes
func waitForJobFinish(ctx context.Context, watcher watch.Interface) {
	for {
		select {
		case event := <-watcher.ResultChan():
			job := event.Object.(*v1.Job)

			if job.Status.Succeeded > 0 || job.Status.Failed > 0 {
				return
			} else if event.Type == watch.Deleted {
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

func getKubernetesClientset() (*kubernetes.Clientset, error) {
	kubeconfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(kubeconfig)
	return clientset, err
}
