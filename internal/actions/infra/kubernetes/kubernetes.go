package kubernetes

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"

	"flowk/internal/flow"
)

const (
	// ActionName identifies the Kubernetes action in the flow definition.
	ActionName = "KUBERNETES"

	// OperationGetPods lists pods within a namespace using the specified context.
	OperationGetPods = "GET_PODS"
	// OperationGetDeployments lists deployments within a namespace using the specified context.
	OperationGetDeployments = "GET_DEPLOYMENTS"
	// OperationGetLogs retrieves logs for pods or deployments.
	OperationGetLogs = "GET_LOGS"
	// OperationScale updates the replica count of the requested deployment.
	OperationScale = "SCALE"
	// OperationPortForward establishes a port-forward tunnel to a service-selected pod.
	OperationPortForward = "PORT_FORWARD"
	// OperationStopPortForward terminates an active port-forward tunnel.
	OperationStopPortForward = "STOP_PORT_FORWARD"
	// OperationWaitForPodReadiness waits until every pod belonging to the requested deployments is ready.
	OperationWaitForPodReadiness = "WAIT_FOR_POD_READINESS"
)

// Logger defines the minimal interface expected from loggers used by the action.
type Logger interface {
	Printf(format string, v ...interface{})
}

// Config contains the information required to execute a Kubernetes operation.
type Config struct {
	Context       string
	Namespace     string
	Operation     string
	Deployments   []string
	Replicas      *int32
	Kubeconfig    string
	Pods          []string
	Container     string
	SinceTime     *time.Time
	SincePodStart bool
	Service       string
	LocalPort     int32
	ServicePort   int32
	MaxWait       time.Duration
	PollInterval  time.Duration
	LogDir        string `json:"-"`
}

// DeploymentDetails captures high-level readiness details for a deployment.
type DeploymentDetails struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	DesiredReplicas   int32  `json:"desiredReplicas"`
	UpdatedReplicas   int32  `json:"updatedReplicas"`
	ReadyReplicas     int32  `json:"readyReplicas"`
	AvailableReplicas int32  `json:"availableReplicas"`
	Age               string `json:"age"`
}

// PodDetails mirrors the information produced by `kubectl get pods -o wide` in JSON form.
type PodDetails struct {
	Name            string   `json:"name"`
	Namespace       string   `json:"namespace"`
	Ready           string   `json:"ready"`
	Status          string   `json:"status"`
	Restarts        int32    `json:"restarts"`
	Age             string   `json:"age"`
	IP              string   `json:"ip,omitempty"`
	Node            string   `json:"node,omitempty"`
	NominatedNode   string   `json:"nominatedNode,omitempty"`
	ReadinessGates  []string `json:"readinessGates,omitempty"`
	HostIP          string   `json:"hostIP,omitempty"`
	ContainerImages []string `json:"containerImages,omitempty"`
}

// ScaleResult reports the outcome of a deployment scaling operation.
type ScaleResult struct {
	Namespace        string `json:"namespace"`
	Deployment       string `json:"deployment"`
	PreviousReplicas int32  `json:"previousReplicas"`
	DesiredReplicas  int32  `json:"desiredReplicas"`
	Changed          bool   `json:"changed"`
}

// DeploymentReadinessStatus captures the readiness information for a deployment.
type DeploymentReadinessStatus struct {
	Deployment        string `json:"deployment"`
	DesiredReplicas   int32  `json:"desiredReplicas"`
	ReadyReplicas     int32  `json:"readyReplicas"`
	AvailableReplicas int32  `json:"availableReplicas"`
	ReadyPods         int    `json:"readyPods"`
	TotalPods         int    `json:"totalPods"`
	Ready             bool   `json:"ready"`
}

// WaitForPodReadinessResult reports the outcome of a WAIT_FOR_POD_READINESS operation.
type WaitForPodReadinessResult struct {
	Namespace   string                      `json:"namespace"`
	Deployments []DeploymentReadinessStatus `json:"deployments"`
	Checks      int                         `json:"checks"`
	Elapsed     string                      `json:"elapsed"`
	Succeeded   bool                        `json:"succeeded"`
}

// Execute performs the requested Kubernetes operation and returns the outcome.
func Execute(ctx context.Context, cfg Config, logger Logger) (any, flow.ResultType, error) {
	client, restCfg, defaultNamespace, err := buildClient(cfg.Context, cfg.Kubeconfig)
	if err != nil {
		return nil, "", err
	}

	namespace := strings.TrimSpace(cfg.Namespace)
	if namespace == "" {
		namespace = defaultNamespace
	}
	if namespace == "" {
		namespace = "default"
	}

	operation := strings.ToUpper(strings.TrimSpace(cfg.Operation))
	switch operation {
	case OperationGetPods:
		if logger != nil {
			logger.Printf("Kubernetes: listing pods in namespace %s (context %s)", namespace, cfg.Context)
		}
		pods, err := listPods(ctx, client, namespace)
		if err != nil {
			return nil, "", err
		}
		return pods, flow.ResultTypeJSON, nil
	case OperationGetDeployments:
		if logger != nil {
			logger.Printf("Kubernetes: listing deployments in namespace %s (context %s)", namespace, cfg.Context)
		}
		deployments, err := listDeployments(ctx, client, namespace)
		if err != nil {
			return nil, "", err
		}
		return deployments, flow.ResultTypeJSON, nil
	case OperationGetLogs:
		if logger != nil {
			var targetType string
			var targets []string
			switch {
			case len(cfg.Pods) > 0:
				targetType = "pods"
				targets = cfg.Pods
			case len(cfg.Deployments) > 0:
				targetType = "deployments"
				targets = cfg.Deployments
			default:
				targetType = "targets"
			}
			targetList := strings.Join(targets, ", ")
			if targetList == "" {
				targetList = "(unspecified)"
			}
			logger.Printf("Kubernetes: fetching logs for %s %s in namespace %s (context %s)", targetType, targetList, namespace, cfg.Context)
		}
		logs, err := getLogs(ctx, client, namespace, cfg)
		if err != nil {
			return nil, "", err
		}
		return logs, flow.ResultTypeJSON, nil
	case OperationScale:
		if cfg.Replicas == nil {
			return nil, "", fmt.Errorf("kubernetes SCALE operation: replicas is required")
		}
		if len(cfg.Deployments) == 0 {
			return nil, "", fmt.Errorf("kubernetes SCALE operation: at least one deployment is required")
		}
		if logger != nil {
			logger.Printf("Kubernetes: scaling deployments %s in namespace %s to %d replicas (context %s)", strings.Join(cfg.Deployments, ", "), namespace, *cfg.Replicas, cfg.Context)
		}
		results := make([]ScaleResult, 0, len(cfg.Deployments))
		for _, deployment := range cfg.Deployments {
			result, err := scaleDeployment(ctx, client, namespace, deployment, *cfg.Replicas)
			if err != nil {
				return nil, "", err
			}
			results = append(results, result)
		}
		return results, flow.ResultTypeJSON, nil
	case OperationPortForward:
		if strings.TrimSpace(cfg.Service) == "" {
			return nil, "", fmt.Errorf("kubernetes PORT_FORWARD operation: service is required")
		}
		if cfg.LocalPort <= 0 || cfg.LocalPort > 65535 {
			return nil, "", fmt.Errorf("kubernetes PORT_FORWARD operation: local_port must be between 1 and 65535")
		}
		if cfg.ServicePort <= 0 || cfg.ServicePort > 65535 {
			return nil, "", fmt.Errorf("kubernetes PORT_FORWARD operation: service_port must be between 1 and 65535")
		}
		if logger != nil {
			logger.Printf("Kubernetes: port-forwarding service %s in namespace %s on localhost:%d (context %s)", cfg.Service, namespace, cfg.LocalPort, cfg.Context)
		}
		result, err := portForwardService(ctx, client, restCfg, namespace, cfg, logger)
		if err != nil {
			return nil, "", err
		}
		return result, flow.ResultTypeJSON, nil
	case OperationStopPortForward:
		if cfg.LocalPort <= 0 || cfg.LocalPort > 65535 {
			return nil, "", fmt.Errorf("kubernetes STOP_PORT_FORWARD operation: local_port must be between 1 and 65535")
		}
		if logger != nil {
			logger.Printf("Kubernetes: stopping port-forward on localhost:%d", cfg.LocalPort)
		}
		result, err := stopPortForward(ctx, cfg.LocalPort, logger)
		if err != nil {
			return nil, "", err
		}
		return result, flow.ResultTypeJSON, nil
	case OperationWaitForPodReadiness:
		if len(cfg.Deployments) == 0 {
			return nil, "", fmt.Errorf("kubernetes WAIT_FOR_POD_READINESS operation: at least one deployment is required")
		}
		if cfg.MaxWait <= 0 {
			return nil, "", fmt.Errorf("kubernetes WAIT_FOR_POD_READINESS operation: max_wait_seconds must be greater than zero")
		}
		if cfg.PollInterval <= 0 {
			return nil, "", fmt.Errorf("kubernetes WAIT_FOR_POD_READINESS operation: poll_interval_seconds must be greater than zero")
		}
		if logger != nil {
			logger.Printf("Kubernetes: waiting for deployments %s to become ready in namespace %s (context %s)", strings.Join(cfg.Deployments, ", "), namespace, cfg.Context)
		}
		result, err := waitForPodReadiness(ctx, client, namespace, cfg, logger)
		if err != nil {
			return nil, "", err
		}
		return result, flow.ResultTypeJSON, nil
	default:
		return nil, "", fmt.Errorf("unsupported Kubernetes operation %q", cfg.Operation)
	}
}

func waitForPodReadiness(ctx context.Context, client kubernetes.Interface, namespace string, cfg Config, logger Logger) (WaitForPodReadinessResult, error) {
	start := time.Now()
	deadline := start.Add(cfg.MaxWait)

	checks := 0
	for {
		statuses := make([]DeploymentReadinessStatus, 0, len(cfg.Deployments))
		allReady := true

		for _, name := range cfg.Deployments {
			status, err := collectDeploymentReadiness(ctx, client, namespace, name)
			if err != nil {
				return WaitForPodReadinessResult{}, err
			}
			statuses = append(statuses, status)
			if logger != nil {
				logger.Printf("Kubernetes: checking deployment %s - %d/%d pods ready (desired replicas: %d)", status.Deployment, status.ReadyPods, status.TotalPods, status.DesiredReplicas)
			}
			if !status.Ready {
				allReady = false
			}
		}

		checks++

		if allReady {
			return WaitForPodReadinessResult{
				Namespace:   namespace,
				Deployments: statuses,
				Checks:      checks,
				Elapsed:     formatRelativeDuration(time.Since(start)),
				Succeeded:   true,
			}, nil
		}

		if time.Now().After(deadline) {
			var notReady []string
			for _, status := range statuses {
				if !status.Ready {
					notReady = append(notReady, fmt.Sprintf("%s %d/%d ready", status.Deployment, status.ReadyPods, status.TotalPods))
				}
			}
			timeout := formatRelativeDuration(cfg.MaxWait)
			if len(notReady) == 0 {
				notReady = append(notReady, "no deployments reported readiness details")
			}
			return WaitForPodReadinessResult{}, fmt.Errorf("kubernetes WAIT_FOR_POD_READINESS operation: timeout after %s waiting for deployments: %s", timeout, strings.Join(notReady, ", "))
		}

		remaining := time.Until(deadline)
		wait := cfg.PollInterval
		if remaining < wait {
			wait = remaining
		}

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return WaitForPodReadinessResult{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func collectDeploymentReadiness(ctx context.Context, client kubernetes.Interface, namespace, deploymentName string) (DeploymentReadinessStatus, error) {
	deployment, err := client.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return DeploymentReadinessStatus{}, fmt.Errorf("kubernetes: retrieving deployment %s in namespace %s: %w", deploymentName, namespace, err)
	}

	if deployment.Spec.Selector == nil {
		return DeploymentReadinessStatus{}, fmt.Errorf("kubernetes: deployment %s in namespace %s does not define a selector", deploymentName, namespace)
	}

	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return DeploymentReadinessStatus{}, fmt.Errorf("kubernetes: building selector for deployment %s in namespace %s: %w", deploymentName, namespace, err)
	}

	podList, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return DeploymentReadinessStatus{}, fmt.Errorf("kubernetes: listing pods for deployment %s in namespace %s: %w", deploymentName, namespace, err)
	}

	readyPods := 0
	totalPods := 0
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.DeletionTimestamp != nil {
			continue
		}
		if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
			continue
		}
		totalPods++
		if isPodReady(pod) {
			readyPods++
		}
	}

	desiredReplicas := int32(1)
	if deployment.Spec.Replicas != nil {
		desiredReplicas = *deployment.Spec.Replicas
		if desiredReplicas < 0 {
			desiredReplicas = 0
		}
	}

	ready := false
	if desiredReplicas == 0 {
		ready = totalPods == 0
	} else {
		ready = readyPods >= int(desiredReplicas) && readyPods == totalPods && totalPods >= int(desiredReplicas)
	}

	return DeploymentReadinessStatus{
		Deployment:        deploymentName,
		DesiredReplicas:   desiredReplicas,
		ReadyReplicas:     deployment.Status.ReadyReplicas,
		AvailableReplicas: deployment.Status.AvailableReplicas,
		ReadyPods:         readyPods,
		TotalPods:         totalPods,
		Ready:             ready,
	}, nil
}

func buildClient(contextName, kubeconfigPath string) (kubernetes.Interface, *rest.Config, string, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if trimmed := strings.TrimSpace(kubeconfigPath); trimmed != "" {
		loadingRules.ExplicitPath = trimmed
	} else if envPath := strings.TrimSpace(os.Getenv("KUBECONFIG")); envPath != "" {
		loadingRules.ExplicitPath = envPath
	}

	overrides := &clientcmd.ConfigOverrides{}
	if trimmed := strings.TrimSpace(contextName); trimmed != "" {
		overrides.CurrentContext = trimmed
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	restCfg, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, nil, "", fmt.Errorf("kubernetes: building client configuration: %w", err)
	}

	namespace, _, err := clientConfig.Namespace()
	if err != nil {
		return nil, nil, "", fmt.Errorf("kubernetes: resolving namespace: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, nil, "", fmt.Errorf("kubernetes: creating clientset: %w", err)
	}

	return clientset, restCfg, namespace, nil
}

// PortForwardResult captures the details of an established port-forward tunnel.
type PortForwardResult struct {
	Namespace   string `json:"namespace"`
	Service     string `json:"service"`
	Pod         string `json:"pod"`
	LocalPort   int32  `json:"localPort"`
	ServicePort int32  `json:"servicePort"`
	TargetPort  string `json:"targetPort"`
}

// StopPortForwardResult reports the outcome of a STOP_PORT_FORWARD operation.
type StopPortForwardResult struct {
	LocalPort int32  `json:"localPort"`
	Stopped   bool   `json:"stopped"`
	Message   string `json:"message,omitempty"`
}

type portForwardSession struct {
	stop func()
	done chan struct{}
	err  error
}

var (
	portForwardSessionsMu sync.Mutex
	portForwardSessions   = make(map[int32]*portForwardSession)
)

func portForwardService(ctx context.Context, client kubernetes.Interface, restCfg *rest.Config, namespace string, cfg Config, logger Logger) (PortForwardResult, error) {
	serviceName := strings.TrimSpace(cfg.Service)
	svc, err := client.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return PortForwardResult{}, fmt.Errorf("kubernetes: retrieving service %s in namespace %s: %w", serviceName, namespace, err)
	}

	svcPort, err := selectServicePort(svc, cfg.ServicePort)
	if err != nil {
		return PortForwardResult{}, err
	}

	pod, err := selectServicePod(ctx, client, namespace, svc)
	if err != nil {
		return PortForwardResult{}, err
	}

	targetPort, err := resolveTargetPort(pod, svcPort)
	if err != nil {
		return PortForwardResult{}, err
	}

	transport, upgrader, err := spdy.RoundTripperFor(restCfg)
	if err != nil {
		return PortForwardResult{}, fmt.Errorf("kubernetes: preparing port-forward transport: %w", err)
	}

	req := client.CoreV1().RESTClient().Post().Resource("pods").Namespace(namespace).Name(pod.Name).SubResource("portforward")
	if tr, ok := transport.(*http.Transport); ok {
		// Port-forward connections must not enforce REST client timeouts.
		tr.ResponseHeaderTimeout = 0
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, req.URL())

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	errCh := make(chan error, 1)
	stopOnce := sync.Once{}
	stopFn := func() {
		stopOnce.Do(func() {
			close(stopCh)
		})
	}

	go func() {
		select {
		case <-ctx.Done():
			stopFn()
		case <-stopCh:
		}
	}()

	ports := []string{fmt.Sprintf("%d:%s", cfg.LocalPort, targetPort)}

	pf, err := portforward.NewOnAddresses(dialer, []string{"localhost"}, ports, stopCh, readyCh, io.Discard, io.Discard)
	if err != nil {
		stopFn()
		return PortForwardResult{}, fmt.Errorf("kubernetes: creating port-forward: %w", err)
	}

	go func() {
		err := pf.ForwardPorts()
		errCh <- err
		stopFn()
	}()

	select {
	case <-readyCh:
	case err := <-errCh:
		if err == nil {
			return PortForwardResult{}, fmt.Errorf("kubernetes: port-forward terminated before becoming ready")
		}
		return PortForwardResult{}, fmt.Errorf("kubernetes: running port-forward: %w", err)
	case <-ctx.Done():
		return PortForwardResult{}, ctx.Err()
	}

	session := &portForwardSession{
		stop: stopFn,
		done: make(chan struct{}),
	}

	portForwardSessionsMu.Lock()
	portForwardSessions[cfg.LocalPort] = session
	portForwardSessionsMu.Unlock()

	go func() {
		err := <-errCh
		session.err = err
		if err != nil && logger != nil {
			logger.Printf("Kubernetes: port-forward for service %s/%s terminated: %v", namespace, serviceName, err)
		}
		portForwardSessionsMu.Lock()
		if current, ok := portForwardSessions[cfg.LocalPort]; ok && current == session {
			delete(portForwardSessions, cfg.LocalPort)
		}
		portForwardSessionsMu.Unlock()
		close(session.done)
	}()

	return PortForwardResult{
		Namespace:   namespace,
		Service:     serviceName,
		Pod:         pod.Name,
		LocalPort:   cfg.LocalPort,
		ServicePort: cfg.ServicePort,
		TargetPort:  targetPort,
	}, nil
}

func selectServicePort(svc *corev1.Service, requested int32) (corev1.ServicePort, error) {
	if svc == nil {
		return corev1.ServicePort{}, fmt.Errorf("kubernetes: service not provided for PORT_FORWARD operation")
	}
	if len(svc.Spec.Ports) == 0 {
		return corev1.ServicePort{}, fmt.Errorf("kubernetes: service %s in namespace %s does not define any ports", svc.Name, svc.Namespace)
	}

	if requested <= 0 {
		return corev1.ServicePort{}, fmt.Errorf("kubernetes PORT_FORWARD operation: service_port must be between 1 and 65535")
	}

	for _, port := range svc.Spec.Ports {
		if port.Port == requested {
			return port, nil
		}
	}

	return corev1.ServicePort{}, fmt.Errorf("kubernetes: service %s in namespace %s does not expose port %d", svc.Name, svc.Namespace, requested)
}

func selectServicePod(ctx context.Context, client kubernetes.Interface, namespace string, svc *corev1.Service) (*corev1.Pod, error) {
	if svc == nil {
		return nil, fmt.Errorf("kubernetes: service not provided for PORT_FORWARD operation")
	}
	if len(svc.Spec.Selector) == 0 {
		return nil, fmt.Errorf("kubernetes: service %s in namespace %s does not define a selector", svc.Name, svc.Namespace)
	}

	selector := labels.Set(svc.Spec.Selector).AsSelector().String()
	podList, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: listing pods for service %s in namespace %s: %w", svc.Name, namespace, err)
	}
	if len(podList.Items) == 0 {
		return nil, fmt.Errorf("kubernetes: no pods found for service %s in namespace %s", svc.Name, namespace)
	}

	sort.Slice(podList.Items, func(i, j int) bool {
		return podList.Items[i].Name < podList.Items[j].Name
	})

	for i := range podList.Items {
		if isPodReady(&podList.Items[i]) {
			return &podList.Items[i], nil
		}
	}

	return &podList.Items[0], nil
}

func resolveTargetPort(pod *corev1.Pod, port corev1.ServicePort) (string, error) {
	if pod == nil {
		return "", fmt.Errorf("kubernetes: no pod available for service port forwarding")
	}

	target := port.TargetPort
	if target.Type == intstr.Int && target.IntValue() == 0 && port.Port > 0 {
		return strconv.Itoa(int(port.Port)), nil
	}

	switch target.Type {
	case intstr.Int:
		if target.IntValue() > 0 {
			return strconv.Itoa(int(target.IntValue())), nil
		}
	case intstr.String:
		name := strings.TrimSpace(target.StrVal)
		if name == "" {
			break
		}
		if portNumber, err := strconv.Atoi(name); err == nil {
			return strconv.Itoa(portNumber), nil
		}
		if resolved, ok := findContainerPortByName(pod, name); ok {
			return strconv.Itoa(int(resolved)), nil
		}
		return "", fmt.Errorf("kubernetes: pod %s in namespace %s does not expose a port named %q", pod.Name, pod.Namespace, name)
	}

	if port.Port > 0 {
		return strconv.Itoa(int(port.Port)), nil
	}

	return "", fmt.Errorf("kubernetes: could not resolve target port for service port %s", port.Name)
}

func stopPortForward(ctx context.Context, localPort int32, logger Logger) (StopPortForwardResult, error) {
	portForwardSessionsMu.Lock()
	session, ok := portForwardSessions[localPort]
	portForwardSessionsMu.Unlock()
	if !ok {
		return StopPortForwardResult{}, fmt.Errorf("kubernetes STOP_PORT_FORWARD operation: no active port-forward on local port %d", localPort)
	}

	session.stop()

	select {
	case <-session.done:
		message := "port-forward stopped"
		if session.err != nil {
			message = session.err.Error()
		}
		return StopPortForwardResult{LocalPort: localPort, Stopped: true, Message: message}, nil
	case <-ctx.Done():
		return StopPortForwardResult{}, ctx.Err()
	}
}

func findContainerPortByName(pod *corev1.Pod, name string) (int32, bool) {
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			if strings.EqualFold(port.Name, name) {
				return port.ContainerPort, true
			}
		}
	}
	for _, container := range pod.Spec.InitContainers {
		for _, port := range container.Ports {
			if strings.EqualFold(port.Name, name) {
				return port.ContainerPort, true
			}
		}
	}
	return 0, false
}

func isPodReady(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}

	if pod.Status.Phase != corev1.PodRunning {
		return false
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}

	return false
}

func listPods(ctx context.Context, client kubernetes.Interface, namespace string) ([]PodDetails, error) {
	podList, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: listing pods in namespace %s: %w", namespace, err)
	}

	pods := make([]PodDetails, 0, len(podList.Items))
	for i := range podList.Items {
		pods = append(pods, buildPodDetails(&podList.Items[i]))
	}

	sort.Slice(pods, func(i, j int) bool {
		return pods[i].Name < pods[j].Name
	})

	return pods, nil
}

func buildPodDetails(pod *corev1.Pod) PodDetails {
	if pod == nil {
		return PodDetails{}
	}

	readyContainers := 0
	for _, status := range pod.Status.ContainerStatuses {
		if status.Ready {
			readyContainers++
		}
	}

	totalContainers := len(pod.Status.ContainerStatuses)
	if totalContainers == 0 {
		totalContainers = len(pod.Spec.Containers)
	}

	restarts := int32(0)
	for _, status := range pod.Status.ContainerStatuses {
		restarts += status.RestartCount
	}
	for _, status := range pod.Status.InitContainerStatuses {
		restarts += status.RestartCount
	}

	readinessGates := make([]string, 0, len(pod.Spec.ReadinessGates))
	for _, gate := range pod.Spec.ReadinessGates {
		if gate.ConditionType != "" {
			readinessGates = append(readinessGates, string(gate.ConditionType))
		}
	}

	images := make([]string, 0, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		if strings.TrimSpace(container.Image) != "" {
			images = append(images, container.Image)
		}
	}

	status := strings.TrimSpace(pod.Status.Reason)
	if status == "" {
		status = string(pod.Status.Phase)
	}

	age := ""
	if !pod.CreationTimestamp.IsZero() {
		age = formatRelativeDuration(time.Since(pod.CreationTimestamp.Time))
	}

	nominatedNode := strings.TrimSpace(pod.Status.NominatedNodeName)

	return PodDetails{
		Name:            pod.Name,
		Namespace:       pod.Namespace,
		Ready:           fmt.Sprintf("%d/%d", readyContainers, totalContainers),
		Status:          status,
		Restarts:        restarts,
		Age:             age,
		IP:              strings.TrimSpace(pod.Status.PodIP),
		Node:            strings.TrimSpace(pod.Spec.NodeName),
		NominatedNode:   nominatedNode,
		ReadinessGates:  readinessGates,
		HostIP:          strings.TrimSpace(pod.Status.HostIP),
		ContainerImages: images,
	}
}

func formatRelativeDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}

	switch {
	case d < time.Minute:
		seconds := int(d.Seconds())
		if seconds <= 0 {
			seconds = 1
		}
		return fmt.Sprintf("%ds", seconds)
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}
}

func listDeployments(ctx context.Context, client kubernetes.Interface, namespace string) ([]DeploymentDetails, error) {
	deploymentList, err := client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: listing deployments in namespace %s: %w", namespace, err)
	}

	deployments := make([]DeploymentDetails, 0, len(deploymentList.Items))
	for i := range deploymentList.Items {
		deploy := &deploymentList.Items[i]
		desired := int32(1)
		if deploy.Spec.Replicas != nil {
			desired = *deploy.Spec.Replicas
			if desired < 0 {
				desired = 0
			}
		}

		age := ""
		if !deploy.CreationTimestamp.IsZero() {
			age = formatRelativeDuration(time.Since(deploy.CreationTimestamp.Time))
		}

		deployments = append(deployments, DeploymentDetails{
			Name:              deploy.Name,
			Namespace:         deploy.Namespace,
			DesiredReplicas:   desired,
			UpdatedReplicas:   deploy.Status.UpdatedReplicas,
			ReadyReplicas:     deploy.Status.ReadyReplicas,
			AvailableReplicas: deploy.Status.AvailableReplicas,
			Age:               age,
		})
	}

	sort.Slice(deployments, func(i, j int) bool {
		return deployments[i].Name < deployments[j].Name
	})

	return deployments, nil
}

func scaleDeployment(ctx context.Context, client kubernetes.Interface, namespace, name string, replicas int32) (ScaleResult, error) {
	deployments := client.AppsV1().Deployments(namespace)

	var (
		result  ScaleResult
		changed bool
	)

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		deployment, err := deployments.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		current := int32(0)
		if deployment.Spec.Replicas != nil {
			current = *deployment.Spec.Replicas
		}

		result.Namespace = namespace
		result.Deployment = name
		result.PreviousReplicas = current
		result.DesiredReplicas = replicas

		if current == replicas {
			changed = false
			return nil
		}

		deployment.Spec.Replicas = pointer.Int32(replicas)
		if _, err := deployments.Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
			return err
		}

		changed = true
		return nil
	})
	if err != nil {
		return ScaleResult{}, fmt.Errorf("kubernetes: scaling deployment %s in namespace %s: %w", name, namespace, err)
	}

	result.Changed = changed
	return result, nil
}

// ContainerLog represents the stored log output of a specific container within a pod.
type ContainerLog struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
	File      string `json:"file"`
}

func getLogs(ctx context.Context, client kubernetes.Interface, namespace string, cfg Config) ([]ContainerLog, error) {
	if len(cfg.Pods) == 0 && len(cfg.Deployments) == 0 {
		return nil, fmt.Errorf("kubernetes GET_LOGS operation: either pod or deployments must be specified")
	}
	if len(cfg.Pods) > 0 && len(cfg.Deployments) > 0 {
		return nil, fmt.Errorf("kubernetes GET_LOGS operation: pod and deployments cannot be specified together")
	}

	if len(cfg.Pods) > 0 {
		var logs []ContainerLog
		for _, podName := range cfg.Pods {
			pod, err := client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("kubernetes: retrieving pod %s in namespace %s: %w", podName, namespace, err)
			}
			podLogs, err := collectPodLogs(ctx, client, namespace, pod, cfg)
			if err != nil {
				return nil, err
			}
			logs = append(logs, podLogs...)
		}
		return logs, nil
	}

	var logs []ContainerLog
	for _, deploymentName := range cfg.Deployments {
		deployment, err := client.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("kubernetes: retrieving deployment %s in namespace %s: %w", deploymentName, namespace, err)
		}

		if deployment.Spec.Selector == nil {
			return nil, fmt.Errorf("kubernetes: deployment %s in namespace %s does not define a selector", deploymentName, namespace)
		}

		selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
		if err != nil {
			return nil, fmt.Errorf("kubernetes: parsing selector for deployment %s in namespace %s: %w", deploymentName, namespace, err)
		}

		podList, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
		if err != nil {
			return nil, fmt.Errorf("kubernetes: listing pods for deployment %s in namespace %s: %w", deploymentName, namespace, err)
		}

		if len(podList.Items) == 0 {
			return nil, fmt.Errorf("kubernetes: no pods found for deployment %s in namespace %s", deploymentName, namespace)
		}

		sort.Slice(podList.Items, func(i, j int) bool {
			return podList.Items[i].Name < podList.Items[j].Name
		})

		for i := range podList.Items {
			podLogs, err := collectPodLogs(ctx, client, namespace, &podList.Items[i], cfg)
			if err != nil {
				return nil, err
			}
			logs = append(logs, podLogs...)
		}
	}

	return logs, nil
}

func collectPodLogs(ctx context.Context, client kubernetes.Interface, namespace string, pod *corev1.Pod, cfg Config) ([]ContainerLog, error) {
	containers, err := selectContainers(pod, cfg.Container)
	if err != nil {
		return nil, err
	}

	logRoot := strings.TrimSpace(cfg.LogDir)
	if logRoot == "" {
		logRoot = "."
	}

	baseRelativeDir := filepath.Join("kubernetes_logs", sanitizePathSegment(pod.Namespace), sanitizePathSegment(pod.Name))
	baseDir := filepath.Join(logRoot, baseRelativeDir)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("kubernetes: ensuring log directory %s: %w", baseDir, err)
	}

	podLogs := make([]ContainerLog, 0, len(containers))

	for _, container := range containers {
		options := buildPodLogOptions(pod, container, cfg)
		stream, err := client.CoreV1().Pods(namespace).GetLogs(pod.Name, options).Stream(ctx)
		if err != nil {
			return nil, fmt.Errorf("kubernetes: retrieving logs for container %s in pod %s: %w", container, pod.Name, err)
		}

		data, err := io.ReadAll(stream)
		stream.Close()
		if err != nil {
			return nil, fmt.Errorf("kubernetes: reading logs for container %s in pod %s: %w", container, pod.Name, err)
		}

		fileName := fmt.Sprintf("%s.log", sanitizePathSegment(container))
		relativePath := filepath.Join(baseRelativeDir, fileName)
		filePath := filepath.Join(logRoot, relativePath)
		if err := os.WriteFile(filePath, data, 0o644); err != nil {
			return nil, fmt.Errorf("kubernetes: writing log file %s: %w", filePath, err)
		}

		podLogs = append(podLogs, ContainerLog{
			Namespace: pod.Namespace,
			Pod:       pod.Name,
			Container: container,
			File:      relativePath,
		})
	}

	return podLogs, nil
}

func sanitizePathSegment(value string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		"..", "_",
		":", "_",
		string(filepath.Separator), "_",
	)

	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		trimmed = "unnamed"
	}

	sanitized := replacer.Replace(trimmed)
	sanitized = strings.ReplaceAll(sanitized, " ", "_")
	return sanitized
}

func selectContainers(pod *corev1.Pod, specific string) ([]string, error) {
	if pod == nil {
		return nil, fmt.Errorf("kubernetes: pod reference is nil")
	}

	trimmed := strings.TrimSpace(specific)
	if trimmed != "" {
		if containerExists(pod, trimmed) {
			return []string{trimmed}, nil
		}
		return nil, fmt.Errorf("kubernetes: container %s not found in pod %s", trimmed, pod.Name)
	}

	containers := make([]string, 0, len(pod.Spec.Containers)+len(pod.Spec.InitContainers)+len(pod.Spec.EphemeralContainers))
	for _, c := range pod.Spec.Containers {
		if name := strings.TrimSpace(c.Name); name != "" {
			containers = append(containers, name)
		}
	}
	for _, c := range pod.Spec.InitContainers {
		if name := strings.TrimSpace(c.Name); name != "" {
			containers = append(containers, name)
		}
	}
	for _, c := range pod.Spec.EphemeralContainers {
		if name := strings.TrimSpace(c.Name); name != "" {
			containers = append(containers, name)
		}
	}

	if len(containers) == 0 {
		return nil, fmt.Errorf("kubernetes: pod %s has no containers", pod.Name)
	}

	return containers, nil
}

func containerExists(pod *corev1.Pod, name string) bool {
	for _, c := range pod.Spec.Containers {
		if strings.TrimSpace(c.Name) == name {
			return true
		}
	}
	for _, c := range pod.Spec.InitContainers {
		if strings.TrimSpace(c.Name) == name {
			return true
		}
	}
	for _, c := range pod.Spec.EphemeralContainers {
		if strings.TrimSpace(c.Name) == name {
			return true
		}
	}
	return false
}

func buildPodLogOptions(pod *corev1.Pod, container string, cfg Config) *corev1.PodLogOptions {
	opts := &corev1.PodLogOptions{Container: container}

	if cfg.SinceTime != nil {
		since := metav1.NewTime(*cfg.SinceTime)
		opts.SinceTime = &since
		return opts
	}

	if cfg.SincePodStart {
		if pod != nil && pod.Status.StartTime != nil {
			opts.SinceTime = pod.Status.StartTime
		} else if pod != nil && !pod.CreationTimestamp.IsZero() {
			ts := pod.CreationTimestamp
			opts.SinceTime = &ts
		}
	}

	return opts
}
