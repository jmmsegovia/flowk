package evaluate

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"flowk/internal/actions/core/sleep"
	"flowk/internal/actions/registry"
)

type branchConfig struct {
	Continue      json.RawMessage `json:"continue,omitempty"`
	SleepSeconds  *float64        `json:"sleep,omitempty"`
	GoToTaskID    string          `json:"gototask,omitempty"`
	GoToTaskIDAlt string          `json:"gototaskid,omitempty"`
	Exit          *string         `json:"exit,omitempty"`
	Break         *string         `json:"break,omitempty"`
}

type branchActions struct {
	Continue        bool
	ContinueMessage string
	SleepSeconds    float64
	GoToTaskID      string
	Exit            bool
	ExitMessage     string
	Break           bool
	BreakMessage    string
}

type taskConfig struct {
	IfConditions []Condition  `json:"if_conditions"`
	Then         branchConfig `json:"then"`
	Else         branchConfig `json:"else"`

	thenActions branchActions
	elseActions branchActions
}

func (c *taskConfig) Validate() error {
	if len(c.IfConditions) == 0 {
		return fmt.Errorf("evaluate task: at least one if_condition is required")
	}
	for i, condition := range c.IfConditions {
		if err := condition.Validate(); err != nil {
			return fmt.Errorf("evaluate task: if_conditions[%d]: %w", i, err)
		}
	}

	var err error
	c.thenActions, err = c.Then.toActions("then")
	if err != nil {
		return err
	}
	c.elseActions, err = c.Else.toActions("else")
	if err != nil {
		return err
	}

	return nil
}

func (b branchConfig) toActions(section string) (branchActions, error) {
	var actions branchActions
	var continueSet bool

	if len(b.Continue) > 0 {
		var continueValue *string
		if err := json.Unmarshal(b.Continue, &continueValue); err != nil {
			return actions, fmt.Errorf("evaluate task: decoding %s.continue: %w", section, err)
		}
		if continueValue != nil {
			actions.ContinueMessage = *continueValue
			if strings.TrimSpace(actions.ContinueMessage) != "" {
				continueSet = true
			}
		}
	}

	if b.Exit != nil {
		actions.Exit = true
		actions.ExitMessage = *b.Exit
	}

	if b.SleepSeconds != nil {
		seconds := *b.SleepSeconds
		if seconds <= 0 {
			return actions, fmt.Errorf("evaluate task: %s.sleep must be greater than zero", section)
		}
		actions.SleepSeconds = seconds
	}

	goTo := strings.TrimSpace(b.GoToTaskID)
	if goTo == "" {
		goTo = strings.TrimSpace(b.GoToTaskIDAlt)
	}
	actions.GoToTaskID = goTo

	if actions.Exit {
		if actions.SleepSeconds > 0 {
			return actions, fmt.Errorf("evaluate task: %s.exit cannot be combined with sleep", section)
		}
		if actions.GoToTaskID != "" {
			return actions, fmt.Errorf("evaluate task: %s.exit cannot be combined with goto", section)
		}
		if continueSet {
			return actions, fmt.Errorf("evaluate task: %s.exit cannot be combined with continue", section)
		}
	}

	if b.Break != nil {
		actions.Break = true
		actions.BreakMessage = *b.Break
	}
	if actions.Break {
		if actions.GoToTaskID != "" {
			return actions, fmt.Errorf("evaluate task: %s.break cannot be combined with goto", section)
		}
		if actions.Exit {
			return actions, fmt.Errorf("evaluate task: %s.break cannot be combined with exit", section)
		}
		if continueSet {
			return actions, fmt.Errorf("evaluate task: %s.break cannot be combined with continue", section)
		}
	}

	if !actions.Exit && actions.SleepSeconds == 0 && actions.GoToTaskID == "" {
		actions.Continue = true
	}

	return actions, nil
}

func decodeTask(data json.RawMessage) (taskConfig, error) {
	var cfg taskConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("decoding evaluate task payload: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
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

	var variableValues map[string]any
	if len(execCtx.Variables) > 0 {
		variableValues = make(map[string]any, len(execCtx.Variables))
		for name, variable := range execCtx.Variables {
			variableValues[name] = variable.Value
		}
	}

	matches, resultType, err := Execute(execCtx.Task, execCtx.Tasks, variableValues, cfg.IfConditions, execCtx.Logger)
	if err != nil {
		return registry.Result{}, err
	}

	result := registry.Result{Value: matches, Type: resultType}
	branch := cfg.thenActions
	if !matches {
		branch = cfg.elseActions
	}

	if branch.SleepSeconds > 0 {
		if _, _, err := sleep.Execute(ctx, branch.SleepSeconds, execCtx.Logger); err != nil {
			return registry.Result{}, fmt.Errorf("evaluate task: executing %s branch sleep: %w", branchName(matches), err)
		}
	}

	if trimmedMsg := strings.TrimSpace(branch.ContinueMessage); trimmedMsg != "" {
		execCtx.Logger.Printf("Evaluate %s branch continue: %s", branchName(matches), trimmedMsg)
	}

	if trimmedGoTo := strings.TrimSpace(branch.GoToTaskID); trimmedGoTo != "" {
		result.Control = &registry.Control{JumpToTaskID: trimmedGoTo}
	}

	if branch.Exit {
		if result.Control == nil {
			result.Control = &registry.Control{}
		}
		result.Control.Exit = true
		if trimmedMsg := strings.TrimSpace(branch.ExitMessage); trimmedMsg != "" {
			execCtx.Logger.Printf("Evaluate %s branch exit: %s", branchName(matches), trimmedMsg)
		} else {
			execCtx.Logger.Printf("Evaluate %s branch exit", branchName(matches))
		}
	}

	if branch.Break {
		if result.Control == nil {
			result.Control = &registry.Control{}
		}
		result.Control.BreakLoop = true
		if trimmedMsg := strings.TrimSpace(branch.BreakMessage); trimmedMsg != "" {
			execCtx.Logger.Printf("Evaluate %s branch break: %s", branchName(matches), trimmedMsg)
		} else {
			execCtx.Logger.Printf("Evaluate %s branch break", branchName(matches))
		}
	}

	return result, nil
}

func branchName(matches bool) string {
	if matches {
		return "then"
	}
	return "else"
}
