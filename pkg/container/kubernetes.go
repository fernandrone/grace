package container

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/fernandrone/grace/pkg/addr"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

// KubernetesPod represents a Kubernetes Pod.
type KubernetesPod struct {
	restConfig   *rest.Config
	coreV1Client corev1client.CoreV1Interface

	namespace string
	podName   string
}

const podDefaultStopTimeout = time.Second * time.Duration(30)

// requires shareProcessNamespace: true
func NewKubernetesPod(kubeconfig, namespace, podID string) (Containers, error) {
	// use the current context in kubeconfig
	if kubeconfig == "" {
		return nil, errors.New("kubeconfig is not set")
	}

	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	p := KubernetesPod{
		restConfig:   restConfig,
		coreV1Client: clientSet.CoreV1(),
		namespace:    namespace,
		podName:      podID,
	}

	return &p, nil
}

func (k KubernetesPod) Stop(ctx context.Context) ([]Response, error) {
	pod, err := k.coreV1Client.Pods(k.namespace).Get(ctx, k.podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error retrieving pod: %v", err)
	}

	podJS, err := json.Marshal(pod)
	if err != nil {
		return nil, fmt.Errorf("error creating JSON for pod: %v", err)
	}

	debugContainer := &corev1.EphemeralContainer{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{
			Name:                     fmt.Sprintf("grace-%s", pod.Name[:57]),
			Image:                    "gcr.io/distroless/static-debian11",
			Stdin:                    true,
			TerminationMessagePolicy: corev1.TerminationMessageReadFile,
			TTY:                      true,
		},
	}

	debugPod := pod.DeepCopy()
	debugPod.Spec.EphemeralContainers = append(debugPod.Spec.EphemeralContainers, *debugContainer)
	debugJS, err := json.Marshal(debugPod)
	if err != nil {
		return nil, fmt.Errorf("error creating JSON for debug container: %v", err)
	}

	patch, err := strategicpatch.CreateTwoWayMergePatch(podJS, debugJS, pod)
	if err != nil {
		return nil, fmt.Errorf("error creating patch to add debug container: %v", err)
	}

	_, err = k.coreV1Client.Pods(k.namespace).Patch(ctx, k.podName, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		// The apiserver will return a 404 when the EphemeralContainers feature
		// is disabled because the `/ephemeralcontainers` subresource is
		// missing. Unlike the 404 returned by a missing pod, the status details
		// will be empty.
		if serr, ok := err.(*apierrors.StatusError); ok && serr.Status().Reason == metav1.StatusReasonNotFound && serr.ErrStatus.Details.Name == "" {
			return nil, fmt.Errorf("ephemeral containers are disabled for this cluster (error from server: %q)", err)
		}

		// The Kind used for the /ephemeralcontainers subresource changed in 1.22. When presented with an unexpected
		// Kind the api server will respond with a not-registered error. When this happens we can optimistically try
		// using the old API.
		if runtime.IsNotRegisteredError(err) {

			if err = k.debugByEphemeralContainerLegacy(ctx, pod, debugContainer); err != nil {
				return nil, err
			}
		}
	}

	pod, err = k.coreV1Client.Pods(k.namespace).Get(ctx, k.podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error retrieving pod: %v", err)
	}

	var response []Response
	for i, container := range pod.Spec.Containers {

		if pod.Status.ContainerStatuses[i].Started == nil || pod.Status.ContainerStatuses[i].Started == addr.Bool(false) {
			return nil, errors.New("container %s did not start yet")
		}

		if pod.Status.ContainerStatuses[i].State.Running == nil {
			return nil, errors.New("container %s is not running")
		}

		start := time.Now()

		// https://stackoverflow.com/questions/43314689/example-of-exec-in-k8ss-pod-by-using-go-client/43431535
		if err = k.shutdownContainer(*debugContainer, container); err != nil {
			return nil, fmt.Errorf("error shutting down container %s pod: %v", container.Name, err)
		}

		// wait until container shuts down
		for {
			pod, err = k.coreV1Client.Pods(k.namespace).Get(ctx, k.podName, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("error retrieving pod: %v", err)
			}

			if pod.Status.ContainerStatuses[i].State.Terminated != nil {
				break
			}

			time.Sleep(1)
		}

		response = append(response, Response{
			Config: Config{
				ID:          fmt.Sprintf("%s/%s", pod.Name[:7], container.Name[:7]),
				Image:       container.Image,
				Command:     fmt.Sprintf("%s", container.Command),
				StopTimeout: time.Duration(*pod.DeletionGracePeriodSeconds),
			},
			ExitCode:  int(pod.Status.ContainerStatuses[i].State.Terminated.ExitCode),
			Duration:  time.Since(start),
			OOMKilled: pod.Status.ContainerStatuses[i].State.Terminated.Reason == "OOMKilled",
		})
	}

	return response, nil
}

// debugByEphemeralContainerLegacy adds debugContainer as an ephemeral container using the pre-1.22 /ephemeralcontainers API
func (k *KubernetesPod) debugByEphemeralContainerLegacy(ctx context.Context, pod *corev1.Pod, debugContainer *corev1.EphemeralContainer) error {
	// We no longer have the v1.EphemeralContainers Kind since it was removed in 1.22, but
	// we can present a JSON 6902 patch that the api server will apply.
	patch, err := json.Marshal([]map[string]interface{}{{
		"op":    "add",
		"path":  "/ephemeralContainers/-",
		"value": debugContainer,
	}})
	if err != nil {
		return fmt.Errorf("error creating JSON 6902 patch for old /ephemeralcontainers API: %s", err)
	}

	result := k.coreV1Client.RESTClient().Patch(types.JSONPatchType).
		Namespace(pod.Namespace).
		Resource("pods").
		Name(pod.Name).
		SubResource("ephemeralcontainers").
		Body(patch).
		Do(ctx)
	if err := result.Error(); err != nil {
		return err
	}

	_, err = k.coreV1Client.Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
	return err
}

// shutdownContainer execs the 'kill command' on specific container from a running
// ephemeral container and wait the command's output.
func (k *KubernetesPod) shutdownContainer(debug corev1.EphemeralContainer, target corev1.Container) error {
	req := k.coreV1Client.RESTClient().Post().Resource("pods").Name(k.podName).Namespace(k.namespace).SubResource("exec")
	option := &v1.PodExecOptions{
		Container: debug.Name,
		Command:   []string{"kill", target.Name},
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
	}
	req.VersionedParams(
		option,
		scheme.ParameterCodec,
	)
	exec, err := remotecommand.NewSPDYExecutor(k.restConfig, "POST", req.URL())
	if err != nil {
		return err
	}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: os.Stdout,
		Stderr: os.Stdout,
	})
	return err
}
