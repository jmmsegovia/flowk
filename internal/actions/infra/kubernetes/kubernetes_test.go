package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/pointer"
)

func resetPortForwardSessions() {
	portForwardSessionsMu.Lock()
	portForwardSessions = make(map[int32]*portForwardSession)
	portForwardSessionsMu.Unlock()
}

func TestTaskConfigValidateStopPortForward(t *testing.T) {
	t.Run("allows empty context", func(t *testing.T) {
		cfg := taskConfig{
			Operation: OperationStopPortForward,
			LocalPort: 8080,
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	t.Run("rejects invalid local port", func(t *testing.T) {
		cfg := taskConfig{
			Operation: OperationStopPortForward,
			LocalPort: 0,
		}
		err := cfg.Validate()
		if err == nil {
			t.Fatal("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "local_port must be between 1 and 65535") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestTaskConfigValidateWaitForPodReadiness(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		cfg := taskConfig{
			Context:             "example",
			Namespace:           "apps",
			Operation:           OperationWaitForPodReadiness,
			Deployments:         []string{"demo"},
			MaxWaitSeconds:      30,
			PollIntervalSeconds: 5,
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	t.Run("missing namespace", func(t *testing.T) {
		cfg := taskConfig{
			Context:             "example",
			Operation:           OperationWaitForPodReadiness,
			Deployments:         []string{"demo"},
			MaxWaitSeconds:      30,
			PollIntervalSeconds: 5,
		}
		err := cfg.Validate()
		if err == nil {
			t.Fatal("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "namespace is required") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing deployments", func(t *testing.T) {
		cfg := taskConfig{
			Context:             "example",
			Namespace:           "apps",
			Operation:           OperationWaitForPodReadiness,
			MaxWaitSeconds:      30,
			PollIntervalSeconds: 5,
		}
		err := cfg.Validate()
		if err == nil {
			t.Fatal("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "at least one deployment") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid wait duration", func(t *testing.T) {
		cfg := taskConfig{
			Context:             "example",
			Namespace:           "apps",
			Operation:           OperationWaitForPodReadiness,
			Deployments:         []string{"demo"},
			MaxWaitSeconds:      0,
			PollIntervalSeconds: 5,
		}
		err := cfg.Validate()
		if err == nil {
			t.Fatal("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "max_wait_seconds") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid poll interval", func(t *testing.T) {
		cfg := taskConfig{
			Context:             "example",
			Namespace:           "apps",
			Operation:           OperationWaitForPodReadiness,
			Deployments:         []string{"demo"},
			MaxWaitSeconds:      30,
			PollIntervalSeconds: 0,
		}
		err := cfg.Validate()
		if err == nil {
			t.Fatal("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "poll_interval_seconds") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

type recordingLogger struct {
	mu       sync.Mutex
	messages []string
}

func (l *recordingLogger) Printf(format string, v ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, fmt.Sprintf(format, v...))
}

func TestStopPortForward_NoActiveSession(t *testing.T) {
	resetPortForwardSessions()

	if _, err := stopPortForward(context.Background(), 9000, nil); err == nil {
		t.Fatal("stopPortForward() error = nil, want error for missing session")
	}
}

func TestStopPortForward_SessionLifecycle(t *testing.T) {
	resetPortForwardSessions()

	localPort := int32(8081)
	stopCalled := make(chan struct{})
	session := &portForwardSession{
		done: make(chan struct{}),
	}
	var stopOnce sync.Once
	session.stop = func() {
		stopOnce.Do(func() {
			close(stopCalled)
		})
	}

	portForwardSessionsMu.Lock()
	portForwardSessions[localPort] = session
	portForwardSessionsMu.Unlock()

	go func() {
		<-stopCalled
		portForwardSessionsMu.Lock()
		delete(portForwardSessions, localPort)
		portForwardSessionsMu.Unlock()
		close(session.done)
	}()

	result, err := stopPortForward(context.Background(), localPort, nil)
	if err != nil {
		t.Fatalf("stopPortForward() error = %v", err)
	}
	if !result.Stopped {
		t.Fatalf("Stopped = false, want true")
	}
	if result.LocalPort != localPort {
		t.Fatalf("LocalPort = %d, want %d", result.LocalPort, localPort)
	}
	if result.Message != "port-forward stopped" {
		t.Fatalf("Message = %q, want port-forward stopped", result.Message)
	}

	portForwardSessionsMu.Lock()
	_, exists := portForwardSessions[localPort]
	portForwardSessionsMu.Unlock()
	if exists {
		t.Fatalf("session for port %d still registered, want removed", localPort)
	}
}

func TestFormatRelativeDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{30 * time.Second, "30s"},
		{2 * time.Minute, "2m"},
		{3 * time.Hour, "3h"},
		{48 * time.Hour, "2d"},
		{45 * 24 * time.Hour, "1mo"},
		{400 * 24 * time.Hour, "1y"},
	}

	for _, tt := range tests {
		if got := formatRelativeDuration(tt.duration); got != tt.want {
			t.Fatalf("formatRelativeDuration(%v) = %q, want %q", tt.duration, got, tt.want)
		}
	}
}

func TestBuildPodDetails(t *testing.T) {
	createdAt := time.Now().Add(-90 * time.Minute)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "example-pod",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(createdAt),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "example/app:1.0"},
				{Name: "sidecar", Image: "example/sidecar:latest"},
			},
			NodeName: "worker-1",
		},
		Status: corev1.PodStatus{
			Phase:  corev1.PodRunning,
			PodIP:  "10.0.0.10",
			HostIP: "192.168.1.20",
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "app", Ready: true, RestartCount: 1},
				{Name: "sidecar", Ready: false, RestartCount: 0},
			},
		},
	}

	details := buildPodDetails(pod)

	if details.Name != "example-pod" {
		t.Fatalf("Name = %q, want example-pod", details.Name)
	}
	if details.Namespace != "default" {
		t.Fatalf("Namespace = %q, want default", details.Namespace)
	}
	if details.Ready != "1/2" {
		t.Fatalf("Ready = %q, want 1/2", details.Ready)
	}
	if details.Status != "Running" {
		t.Fatalf("Status = %q, want Running", details.Status)
	}
	if details.Restarts != 1 {
		t.Fatalf("Restarts = %d, want 1", details.Restarts)
	}
	if details.Node != "worker-1" {
		t.Fatalf("Node = %q, want worker-1", details.Node)
	}
	if details.IP != "10.0.0.10" {
		t.Fatalf("IP = %q, want 10.0.0.10", details.IP)
	}
	if details.HostIP != "192.168.1.20" {
		t.Fatalf("HostIP = %q, want 192.168.1.20", details.HostIP)
	}
	if details.Age != "1h" {
		t.Fatalf("Age = %q, want 1h", details.Age)
	}
	if len(details.ContainerImages) != 2 {
		t.Fatalf("ContainerImages length = %d, want 2", len(details.ContainerImages))
	}
}

func TestScaleDeployment(t *testing.T) {
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32(2),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "demo"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "demo"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "demo:latest"}}},
			},
		},
	})

	result, err := scaleDeployment(context.Background(), client, "default", "example", 5)
	if err != nil {
		t.Fatalf("scaleDeployment() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("Changed = false, want true")
	}
	if result.PreviousReplicas != 2 {
		t.Fatalf("PreviousReplicas = %d, want 2", result.PreviousReplicas)
	}
	if result.DesiredReplicas != 5 {
		t.Fatalf("DesiredReplicas = %d, want 5", result.DesiredReplicas)
	}

	dep, err := client.AppsV1().Deployments("default").Get(context.Background(), "example", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get deployment error = %v", err)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 5 {
		t.Fatalf("deployment replicas = %v, want 5", dep.Spec.Replicas)
	}

	unchanged, err := scaleDeployment(context.Background(), client, "default", "example", 5)
	if err != nil {
		t.Fatalf("scaleDeployment() second call error = %v", err)
	}
	if unchanged.Changed {
		t.Fatalf("Changed = true, want false for unchanged scale")
	}
}

func TestWaitForPodReadiness_Success(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "apps"},
			Spec: appsv1.DeploymentSpec{
				Replicas: pointer.Int32(2),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "demo"}},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 2, AvailableReplicas: 2},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo-1",
				Namespace: "apps",
				Labels:    map[string]string{"app": "demo"},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo-2",
				Namespace: "apps",
				Labels:    map[string]string{"app": "demo"},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
	)

	logger := &recordingLogger{}
	cfg := Config{
		Deployments:  []string{"demo"},
		MaxWait:      5 * time.Second,
		PollInterval: 10 * time.Millisecond,
	}

	result, err := waitForPodReadiness(context.Background(), client, "apps", cfg, logger)
	if err != nil {
		t.Fatalf("waitForPodReadiness() error = %v", err)
	}
	if !result.Succeeded {
		t.Fatalf("Succeeded = false, want true")
	}
	if result.Checks != 1 {
		t.Fatalf("Checks = %d, want 1", result.Checks)
	}
	if len(result.Deployments) != 1 {
		t.Fatalf("Deployments length = %d, want 1", len(result.Deployments))
	}
	status := result.Deployments[0]
	if !status.Ready {
		t.Fatalf("Ready = false, want true")
	}

	logger.mu.Lock()
	messages := append([]string(nil), logger.messages...)
	logger.mu.Unlock()
	if len(messages) == 0 {
		t.Fatal("expected logger to record at least one message")
	}
	found := false
	for _, msg := range messages {
		if strings.Contains(msg, "deployment demo") && strings.Contains(msg, "2/2") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected log message to mention deployment readiness, got: %v", messages)
	}
}

func TestWaitForPodReadiness_Timeout(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "apps"},
			Spec: appsv1.DeploymentSpec{
				Replicas: pointer.Int32(2),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "demo"}},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 1, AvailableReplicas: 1},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo-1",
				Namespace: "apps",
				Labels:    map[string]string{"app": "demo"},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "demo-2",
				Namespace: "apps",
				Labels:    map[string]string{"app": "demo"},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodReady,
					Status: corev1.ConditionFalse,
				}},
			},
		},
	)

	cfg := Config{
		Deployments:  []string{"demo"},
		MaxWait:      30 * time.Millisecond,
		PollInterval: 10 * time.Millisecond,
	}

	_, err := waitForPodReadiness(context.Background(), client, "apps", cfg, nil)
	if err == nil {
		t.Fatal("waitForPodReadiness() error = nil, want timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSelectContainers(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers:     []corev1.Container{{Name: "app"}},
			InitContainers: []corev1.Container{{Name: "init"}},
		},
	}

	containers, err := selectContainers(pod, "")
	if err != nil {
		t.Fatalf("selectContainers() error = %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("selectContainers() length = %d, want 2", len(containers))
	}

	only, err := selectContainers(pod, "app")
	if err != nil {
		t.Fatalf("selectContainers() specific error = %v", err)
	}
	if len(only) != 1 || only[0] != "app" {
		t.Fatalf("selectContainers() specific = %v, want [app]", only)
	}

	if _, err := selectContainers(pod, "missing"); err == nil {
		t.Fatalf("selectContainers() missing container error = nil, want error")
	}

	emptyPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "empty"}}
	if _, err := selectContainers(emptyPod, ""); err == nil {
		t.Fatalf("selectContainers() empty pod error = nil, want error")
	}
}

func TestSelectServicePort(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "https", Port: 443, TargetPort: intstr.FromInt(8443)},
				{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
			},
		},
	}

	port, err := selectServicePort(svc, 443)
	if err != nil {
		t.Fatalf("selectServicePort() error = %v", err)
	}
	if port.Name != "https" {
		t.Fatalf("selectServicePort() Name = %q, want https", port.Name)
	}

	port, err = selectServicePort(svc, 80)
	if err != nil {
		t.Fatalf("selectServicePort() by number error = %v", err)
	}
	if port.Name != "http" {
		t.Fatalf("selectServicePort() by number Name = %q, want http", port.Name)
	}

	if _, err := selectServicePort(svc, 12345); err == nil {
		t.Fatalf("selectServicePort() missing port error = nil, want error")
	}
}

func TestSelectServicePod(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-a",
				Namespace: "default",
				Labels:    map[string]string{"app": "demo"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-b",
				Namespace: "default",
				Labels:    map[string]string{"app": "demo"},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			},
		},
	)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "demo"}},
	}

	pod, err := selectServicePod(context.Background(), client, "default", svc)
	if err != nil {
		t.Fatalf("selectServicePod() error = %v", err)
	}
	if pod.Name != "pod-b" {
		t.Fatalf("selectServicePod() = %s, want pod-b", pod.Name)
	}

	if _, err := selectServicePod(context.Background(), client, "default", &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"}}); err == nil {
		t.Fatalf("selectServicePod() missing selector error = nil, want error")
	}
}

func TestResolveTargetPort(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "app",
				Ports: []corev1.ContainerPort{{Name: "https", ContainerPort: 8443}},
			}},
		},
	}

	port := corev1.ServicePort{Name: "https", Port: 443, TargetPort: intstr.FromString("https")}
	resolved, err := resolveTargetPort(pod, port)
	if err != nil {
		t.Fatalf("resolveTargetPort() error = %v", err)
	}
	if resolved != "8443" {
		t.Fatalf("resolveTargetPort() = %q, want 8443", resolved)
	}

	port = corev1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)}
	resolved, err = resolveTargetPort(pod, port)
	if err != nil {
		t.Fatalf("resolveTargetPort() int error = %v", err)
	}
	if resolved != "8080" {
		t.Fatalf("resolveTargetPort() int = %q, want 8080", resolved)
	}

	port = corev1.ServicePort{Name: "fallback", Port: 9000}
	resolved, err = resolveTargetPort(pod, port)
	if err != nil {
		t.Fatalf("resolveTargetPort() fallback error = %v", err)
	}
	if resolved != "9000" {
		t.Fatalf("resolveTargetPort() fallback = %q, want 9000", resolved)
	}

	port = corev1.ServicePort{Name: "missing", TargetPort: intstr.FromString("unknown")}
	if _, err := resolveTargetPort(pod, port); err == nil {
		t.Fatalf("resolveTargetPort() missing port error = nil, want error")
	}
}

func TestIsPodReady(t *testing.T) {
	readyPod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}
	if !isPodReady(readyPod) {
		t.Fatalf("isPodReady() ready pod = false, want true")
	}

	notReady := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodPending}}
	if isPodReady(notReady) {
		t.Fatalf("isPodReady() pending pod = true, want false")
	}
}

func TestBuildPodLogOptions(t *testing.T) {
	now := time.Now().Add(-time.Hour)
	parsed := now.UTC().Truncate(time.Second)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(parsed.Add(-time.Minute))},
		Status:     corev1.PodStatus{StartTime: &metav1.Time{Time: parsed}},
	}

	cfg := Config{SinceTime: &parsed}
	opts := buildPodLogOptions(pod, "app", cfg)
	if opts.SinceTime == nil || !opts.SinceTime.Time.Equal(parsed) {
		t.Fatalf("buildPodLogOptions() sinceTime = %v, want %v", opts.SinceTime, parsed)
	}

	cfg = Config{SincePodStart: true}
	opts = buildPodLogOptions(pod, "app", cfg)
	if opts.SinceTime == nil || !opts.SinceTime.Time.Equal(parsed) {
		t.Fatalf("buildPodLogOptions() pod start = %v, want %v", opts.SinceTime, parsed)
	}

	pod.Status.StartTime = nil
	opts = buildPodLogOptions(pod, "app", cfg)
	if opts.SinceTime == nil || !opts.SinceTime.Time.Equal(pod.CreationTimestamp.Time) {
		t.Fatalf("buildPodLogOptions() fallback = %v, want %v", opts.SinceTime, pod.CreationTimestamp.Time)
	}

	cfg = Config{}
	opts = buildPodLogOptions(pod, "app", cfg)
	if opts.SinceTime != nil {
		t.Fatalf("buildPodLogOptions() default sinceTime = %v, want nil", opts.SinceTime)
	}
}
