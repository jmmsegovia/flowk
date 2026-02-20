package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"flowk/internal/actions/registry"
)

const ActionName = "DOCKER"

const (
	OperationImagesList       = "IMAGES_LIST"
	OperationImagePull        = "IMAGE_PULL"
	OperationImageRemove      = "IMAGE_REMOVE"
	OperationImagePrune       = "IMAGE_PRUNE"
	OperationContainersList   = "CONTAINERS_LIST"
	OperationContainersAll    = "CONTAINERS_LIST_ALL"
	OperationContainerRun     = "CONTAINER_RUN"
	OperationContainerStart   = "CONTAINER_START"
	OperationContainerStop    = "CONTAINER_STOP"
	OperationContainerRestart = "CONTAINER_RESTART"
	OperationContainerRemove  = "CONTAINER_REMOVE"
	OperationContainerPrune   = "CONTAINER_PRUNE"
	OperationContainerLogs    = "CONTAINER_LOGS"
	OperationContainerExec    = "CONTAINER_EXEC"
	OperationVolumeList       = "VOLUME_LIST"
	OperationVolumeCreate     = "VOLUME_CREATE"
	OperationVolumeInspect    = "VOLUME_INSPECT"
	OperationVolumeRemove     = "VOLUME_REMOVE"
	OperationVolumePrune      = "VOLUME_PRUNE"
	OperationNetworkList      = "NETWORK_LIST"
	OperationNetworkCreate    = "NETWORK_CREATE"
	OperationNetworkInspect   = "NETWORK_INSPECT"
	OperationNetworkRemove    = "NETWORK_REMOVE"
)

type Payload struct {
	Operation   string   `json:"operation"`
	Image       string   `json:"image"`
	Container   string   `json:"container"`
	Name        string   `json:"name"`
	Volume      string   `json:"volume"`
	Network     string   `json:"network"`
	Command     []string `json:"command"`
	Env         []string `json:"env"`
	Ports       []string `json:"ports"`
	Interactive bool     `json:"interactive"`
	TTY         bool     `json:"tty"`
	Detach      bool     `json:"detach"`
	RemoveExisting bool  `json:"remove_existing"`
}

type ExecutionResult struct {
	Command         []string `json:"command"`
	ExitCode        int      `json:"exitCode"`
	Stdout          string   `json:"stdout"`
	Stderr          string   `json:"stderr"`
	DurationSeconds float64  `json:"durationSeconds"`
}

func (p *Payload) Validate() error {
	p.Operation = strings.ToUpper(strings.TrimSpace(p.Operation))
	p.Image = strings.TrimSpace(p.Image)
	p.Container = strings.TrimSpace(p.Container)
	p.Name = strings.TrimSpace(p.Name)
	p.Volume = strings.TrimSpace(p.Volume)
	p.Network = strings.TrimSpace(p.Network)

	for i := range p.Command {
		p.Command[i] = strings.TrimSpace(p.Command[i])
		if p.Command[i] == "" {
			return fmt.Errorf("docker task: command[%d] is required", i)
		}
	}

	for i := range p.Env {
		p.Env[i] = strings.TrimSpace(p.Env[i])
		if p.Env[i] == "" {
			return fmt.Errorf("docker task: env[%d] is required", i)
		}
	}

	for i := range p.Ports {
		p.Ports[i] = strings.TrimSpace(p.Ports[i])
		if p.Ports[i] == "" {
			return fmt.Errorf("docker task: ports[%d] is required", i)
		}
	}

	switch p.Operation {
	case OperationImagesList,
		OperationImagePrune,
		OperationContainersList,
		OperationContainersAll,
		OperationContainerPrune,
		OperationVolumeList,
		OperationVolumePrune,
		OperationNetworkList:
		return nil
	case OperationImagePull, OperationImageRemove:
		if p.Image == "" {
			return fmt.Errorf("docker task: image is required for %s", p.Operation)
		}
	case OperationContainerRun:
		if p.Image == "" {
			return fmt.Errorf("docker task: image is required for %s", p.Operation)
		}
	case OperationContainerStart, OperationContainerStop, OperationContainerRestart, OperationContainerRemove, OperationContainerLogs:
		if p.Container == "" {
			return fmt.Errorf("docker task: container is required for %s", p.Operation)
		}
	case OperationContainerExec:
		if p.Container == "" {
			return fmt.Errorf("docker task: container is required for %s", p.Operation)
		}
		if len(p.Command) == 0 {
			return fmt.Errorf("docker task: command is required for %s", p.Operation)
		}
	case OperationVolumeCreate, OperationVolumeInspect, OperationVolumeRemove:
		if p.Volume == "" {
			return fmt.Errorf("docker task: volume is required for %s", p.Operation)
		}
	case OperationNetworkCreate, OperationNetworkInspect, OperationNetworkRemove:
		if p.Network == "" {
			return fmt.Errorf("docker task: network is required for %s", p.Operation)
		}
	default:
		return fmt.Errorf("docker task: unsupported operation %q", p.Operation)
	}

	return nil
}

func Execute(ctx context.Context, spec Payload, execCtx *registry.ExecutionContext) (ExecutionResult, error) {
	if spec.Operation == OperationContainerRun && spec.RemoveExisting && strings.TrimSpace(spec.Name) != "" {
		if err := removeContainerIfExists(ctx, spec.Name, execCtx); err != nil {
			return ExecutionResult{}, err
		}
	}

	args, err := buildDockerArgs(spec)
	if err != nil {
		return ExecutionResult{}, err
	}

	command := exec.CommandContext(ctx, "docker", args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	command.Stdout = &stdoutBuf
	command.Stderr = &stderrBuf

	logCommand(execCtx.Logger, args)

	start := time.Now()
	runErr := command.Run()
	duration := time.Since(start)

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ExecutionResult{}, fmt.Errorf("docker: command interrupted: %w", ctxErr)
			}
			return ExecutionResult{}, fmt.Errorf("docker: executing command: %w", runErr)
		}
	}

	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	logCommandOutcome(execCtx.Logger, exitCode, stdout, stderr, duration)

	result := ExecutionResult{
		Command:         append([]string{"docker"}, args...),
		ExitCode:        exitCode,
		Stdout:          stdout,
		Stderr:          stderr,
		DurationSeconds: duration.Seconds(),
	}

	if runErr != nil && exitCode != 0 {
		return result, fmt.Errorf("docker: command exited with code %d", exitCode)
	}

	return result, nil
}

func removeContainerIfExists(ctx context.Context, name string, execCtx *registry.ExecutionContext) error {
	args := []string{"rm", "-f", name}
	command := exec.CommandContext(ctx, "docker", args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	command.Stdout = &stdoutBuf
	command.Stderr = &stderrBuf

	logCommand(execCtx.Logger, args)
	start := time.Now()
	runErr := command.Run()
	duration := time.Since(start)

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}

	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()
	logCommandOutcome(execCtx.Logger, exitCode, stdout, stderr, duration)

	if runErr != nil && exitCode != 0 {
		if strings.Contains(stderr, "No such container") || strings.Contains(stdout, "No such container") {
			return nil
		}
		return fmt.Errorf("docker: remove existing container %q failed with code %d", name, exitCode)
	}
	return nil
}

func buildDockerArgs(spec Payload) ([]string, error) {
	args := []string{}
	flags := dockerFlags(spec.Interactive, spec.TTY)

	switch spec.Operation {
	case OperationImagesList:
		args = append(args, "images")
	case OperationImagePull:
		args = append(args, "pull", spec.Image)
	case OperationImageRemove:
		args = append(args, "rmi", spec.Image)
	case OperationImagePrune:
		args = append(args, "image", "prune", "--force")
	case OperationContainersList:
		args = append(args, "ps")
	case OperationContainersAll:
		args = append(args, "ps", "-a")
	case OperationContainerRun:
		args = append(args, "run")
		args = append(args, flags...)
		if spec.Detach {
			args = append(args, "-d")
		}
		if spec.Name != "" {
			args = append(args, "--name", spec.Name)
		}
		for _, env := range spec.Env {
			args = append(args, "-e", env)
		}
		for _, port := range spec.Ports {
			args = append(args, "-p", port)
		}
		args = append(args, spec.Image)
		args = append(args, spec.Command...)
	case OperationContainerStart:
		args = append(args, "start", spec.Container)
	case OperationContainerStop:
		args = append(args, "stop", spec.Container)
	case OperationContainerRestart:
		args = append(args, "restart", spec.Container)
	case OperationContainerRemove:
		args = append(args, "rm", spec.Container)
	case OperationContainerPrune:
		args = append(args, "container", "prune", "--force")
	case OperationContainerLogs:
		args = append(args, "logs", spec.Container)
	case OperationContainerExec:
		args = append(args, "exec")
		args = append(args, flags...)
		args = append(args, spec.Container)
		args = append(args, spec.Command...)
	case OperationVolumeList:
		args = append(args, "volume", "ls")
	case OperationVolumeCreate:
		args = append(args, "volume", "create", spec.Volume)
	case OperationVolumeInspect:
		args = append(args, "volume", "inspect", spec.Volume)
	case OperationVolumeRemove:
		args = append(args, "volume", "rm", spec.Volume)
	case OperationVolumePrune:
		args = append(args, "volume", "prune", "--force")
	case OperationNetworkList:
		args = append(args, "network", "ls")
	case OperationNetworkCreate:
		args = append(args, "network", "create", spec.Network)
	case OperationNetworkInspect:
		args = append(args, "network", "inspect", spec.Network)
	case OperationNetworkRemove:
		args = append(args, "network", "rm", spec.Network)
	default:
		return nil, fmt.Errorf("docker task: unsupported operation %q", spec.Operation)
	}

	return args, nil
}

func dockerFlags(interactive, tty bool) []string {
	flags := []string{}
	if interactive {
		flags = append(flags, "-i")
	}
	if tty {
		flags = append(flags, "-t")
	}
	return flags
}

func logCommand(logger registry.Logger, args []string) {
	if logger == nil {
		return
	}
	logger.Printf("DOCKER: executing docker %s", strings.Join(args, " "))
}

func logCommandOutcome(logger registry.Logger, exitCode int, stdout, stderr string, duration time.Duration) {
	if logger == nil {
		return
	}

	logger.Printf("DOCKER: exit code %d (duration %s)", exitCode, duration.Round(time.Millisecond))
	if strings.TrimSpace(stdout) != "" {
		logger.Printf("DOCKER stdout:\n%s", stdout)
	}
	if strings.TrimSpace(stderr) != "" {
		logger.Printf("DOCKER stderr:\n%s", stderr)
	}
}
