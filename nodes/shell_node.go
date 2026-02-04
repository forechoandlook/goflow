package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	goflow "goflow"
)

// ShellNodeConfig controls how the shell command is executed.
type ShellNodeConfig struct {
	ID        string
	Command   string
	Args      []string
	Dir       string
	Env       map[string]string
	Timeout   time.Duration
	InputKey  string
	OutputKey string
	ParseJSON bool
	StatusKey string
}

func DefaultShellNodeConfig(id string) ShellNodeConfig {
	return ShellNodeConfig{
		ID:        id,
		Timeout:   0,
		OutputKey: fmt.Sprintf("%s_output", id),
		StatusKey: fmt.Sprintf("%s_status", id),
	}
}

// ShellNode runs an external command and merges the output back into shared state.
type ShellNode struct {
	cfg ShellNodeConfig
}

func NewShellNode(cfg ShellNodeConfig) (*ShellNode, error) {
	if cfg.ID == "" {
		return nil, errors.New("shell node requires id")
	}
	if cfg.Command == "" {
		return nil, errors.New("shell node requires command")
	}
	return &ShellNode{cfg: cfg}, nil
}

func (n *ShellNode) Name() string {
	return n.cfg.ID
}

func (n *ShellNode) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
	var cancel context.CancelFunc
	if n.cfg.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, n.cfg.Timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	payload, err := json.Marshal(n.inputPayload(shared))
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, n.cfg.Command, n.cfg.Args...)
	if n.cfg.Dir != "" {
		cmd.Dir = n.cfg.Dir
	}
	if env := n.envList(); len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stdin = bytes.NewReader(payload)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("shell node %s failed: %w: %s", n.cfg.ID, err, stderr.String())
	}

	if n.cfg.StatusKey != "" && cmd.ProcessState != nil {
		shared[n.cfg.StatusKey] = cmd.ProcessState.ExitCode()
	}

	outputBytes := stdout.Bytes()
	stored := any(string(outputBytes))
	var parsedMap map[string]any
	if n.cfg.ParseJSON {
		var parsed any
		if err := json.Unmarshal(outputBytes, &parsed); err == nil {
			stored = parsed
			if pm, ok := parsed.(map[string]any); ok {
				parsedMap = pm
			}
		}
	}

	if n.cfg.OutputKey != "" {
		shared[n.cfg.OutputKey] = stored
	} else if parsedMap != nil {
		for key, value := range parsedMap {
			shared[key] = value
		}
	}

	return goflow.ResultWithAction(goflow.ActionNext), nil
}

func (n *ShellNode) inputPayload(shared map[string]any) any {
	if n.cfg.InputKey == "" {
		return shared
	}
	if value, ok := shared[n.cfg.InputKey]; ok {
		return value
	}
	return map[string]any{}
}

func (n *ShellNode) envList() []string {
	if len(n.cfg.Env) == 0 {
		return nil
	}
	result := make([]string, 0, len(n.cfg.Env))
	for key, value := range n.cfg.Env {
		result = append(result, fmt.Sprintf("%s=%s", key, value))
	}
	return result
}

func init() {
	RegisterNode(NodeDefinition{
		ID:          "shell",
		Description: "Runs a shell command with shared state serialized to stdin and returns its output.",
		Example:     `nodes.NewShellNode(nodes.ShellNodeConfig{ID: "git_status", Command: "git", Args: []string{"status", "-sb"}})`,
	})
}
