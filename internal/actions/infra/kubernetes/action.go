package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"flowk/internal/actions/registry"
)

type taskConfig struct {
	Context             string   `json:"context"`
	Namespace           string   `json:"namespace"`
	Operation           string   `json:"operation"`
	Deployments         []string `json:"deployments,omitempty"`
	Replicas            *int32   `json:"replicas,omitempty"`
	Kubeconfig          string   `json:"kubeconfig,omitempty"`
	Pods                []string `json:"pod,omitempty"`
	Container           string   `json:"container,omitempty"`
	SinceTime           string   `json:"since_time,omitempty"`
	SincePodStart       bool     `json:"since_pod_start,omitempty"`
	Service             string   `json:"service,omitempty"`
	LocalPort           int32    `json:"local_port,omitempty"`
	ServicePort         int32    `json:"service_port,omitempty"`
	MaxWaitSeconds      float64  `json:"max_wait_seconds,omitempty"`
	PollIntervalSeconds float64  `json:"poll_interval_seconds,omitempty"`
}

func (c taskConfig) Validate() error {
	op := strings.ToUpper(strings.TrimSpace(c.Operation))
	if strings.TrimSpace(c.Context) == "" && op != OperationStopPortForward {
		return fmt.Errorf("kubernetes task: context is required")
	}

	deployments := normalizeStringList(c.Deployments)
	pods := normalizeStringList(c.Pods)
	switch op {
	case OperationGetPods:
		return nil
	case OperationGetDeployments:
		return nil
	case OperationGetLogs:
		if len(pods) == 0 && len(deployments) == 0 {
			return fmt.Errorf("kubernetes task: pod or deployments must be specified for GET_LOGS operations")
		}
		if len(pods) > 0 && len(deployments) > 0 {
			return fmt.Errorf("kubernetes task: specify either pod or deployments for GET_LOGS operations, not both")
		}
		if trimmed := strings.TrimSpace(c.SinceTime); trimmed != "" {
			if _, err := time.Parse(time.RFC3339, trimmed); err != nil {
				return fmt.Errorf("kubernetes task: since_time must be in RFC3339 format: %w", err)
			}
		}
		return nil
	case OperationScale:
		if strings.TrimSpace(c.Namespace) == "" {
			return fmt.Errorf("kubernetes task: namespace is required for SCALE operations")
		}
		if len(deployments) == 0 {
			return fmt.Errorf("kubernetes task: at least one deployment is required for SCALE operations")
		}
		if c.Replicas == nil {
			return fmt.Errorf("kubernetes task: replicas is required for SCALE operations")
		}
		if *c.Replicas < 0 {
			return fmt.Errorf("kubernetes task: replicas must be greater than or equal to zero")
		}
		return nil
	case OperationWaitForPodReadiness:
		if strings.TrimSpace(c.Namespace) == "" {
			return fmt.Errorf("kubernetes task: namespace is required for WAIT_FOR_POD_READINESS operations")
		}
		if len(deployments) == 0 {
			return fmt.Errorf("kubernetes task: at least one deployment is required for WAIT_FOR_POD_READINESS operations")
		}
		if c.MaxWaitSeconds <= 0 {
			return fmt.Errorf("kubernetes task: max_wait_seconds must be greater than zero for WAIT_FOR_POD_READINESS operations")
		}
		if c.PollIntervalSeconds <= 0 {
			return fmt.Errorf("kubernetes task: poll_interval_seconds must be greater than zero for WAIT_FOR_POD_READINESS operations")
		}
		return nil
	case OperationPortForward:
		if strings.TrimSpace(c.Service) == "" {
			return fmt.Errorf("kubernetes task: service is required for PORT_FORWARD operations")
		}
		if c.LocalPort <= 0 || c.LocalPort > 65535 {
			return fmt.Errorf("kubernetes task: local_port must be between 1 and 65535 for PORT_FORWARD operations")
		}
		if c.ServicePort <= 0 || c.ServicePort > 65535 {
			return fmt.Errorf("kubernetes task: service_port must be between 1 and 65535 for PORT_FORWARD operations")
		}
		return nil
	case OperationStopPortForward:
		if c.LocalPort <= 0 || c.LocalPort > 65535 {
			return fmt.Errorf("kubernetes task: local_port must be between 1 and 65535 for STOP_PORT_FORWARD operations")
		}
		return nil
	default:
		if strings.TrimSpace(c.Operation) == "" {
			return fmt.Errorf("kubernetes task: operation is required")
		}
		return fmt.Errorf("kubernetes task: unsupported operation %q", c.Operation)
	}
}

func decodeTask(data json.RawMessage) (Config, error) {
	var cfg taskConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decoding kubernetes task payload: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	var sinceTime *time.Time
	if trimmed := strings.TrimSpace(cfg.SinceTime); trimmed != "" {
		parsed, err := time.Parse(time.RFC3339, trimmed)
		if err != nil {
			return Config{}, fmt.Errorf("kubernetes task: parsing since_time: %w", err)
		}
		sinceTime = &parsed
	}

	deployments := normalizeStringList(cfg.Deployments)
	pods := normalizeStringList(cfg.Pods)

	return Config{
		Context:       strings.TrimSpace(cfg.Context),
		Namespace:     strings.TrimSpace(cfg.Namespace),
		Operation:     strings.TrimSpace(cfg.Operation),
		Deployments:   deployments,
		Replicas:      cfg.Replicas,
		Kubeconfig:    strings.TrimSpace(cfg.Kubeconfig),
		Pods:          pods,
		Container:     strings.TrimSpace(cfg.Container),
		SinceTime:     sinceTime,
		SincePodStart: cfg.SincePodStart,
		Service:       strings.TrimSpace(cfg.Service),
		LocalPort:     cfg.LocalPort,
		ServicePort:   cfg.ServicePort,
		MaxWait:       time.Duration(cfg.MaxWaitSeconds * float64(time.Second)),
		PollInterval:  time.Duration(cfg.PollIntervalSeconds * float64(time.Second)),
	}, nil
}

type action struct{}

func init() {
	registry.Register(action{})
}

func (action) Name() string {
	return ActionName
}

func (action) Execute(ctx context.Context, payload json.RawMessage, execCtx *registry.ExecutionContext) (registry.Result, error) {
	cfg, err := decodeTask(payload)
	if err != nil {
		return registry.Result{}, err
	}

	cfg.LogDir = execCtx.LogDir

	value, resultType, err := Execute(ctx, cfg, execCtx.Logger)
	if err != nil {
		return registry.Result{}, err
	}
	return registry.Result{Value: value, Type: resultType}, nil
}

func normalizeStringList(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}
	return normalized
}
