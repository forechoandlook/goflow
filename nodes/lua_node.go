package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	goflow "goflow"
)

// LuaNodeConfig configures how a Lua script interacts with shared state.
type LuaNodeConfig struct {
	ID          string
	Script      string
	ScriptPath  string
	Args        []string
	Timeout     time.Duration
	Env         map[string]string
	WorkingDir  string
	ParseOutput bool
	StatusKey   string
}

func DefaultLuaNodeConfig(id string) LuaNodeConfig {
	return LuaNodeConfig{
		ID:          id,
		StatusKey:   fmt.Sprintf("%s_status", id),
		ParseOutput: true,
	}
}

// LuaNode executes a Lua script via the installed interpreter and merges text output back into shared state.
type LuaNode struct {
	cfg LuaNodeConfig
}

func NewLuaNode(cfg LuaNodeConfig) (*LuaNode, error) {
	if cfg.ID == "" {
		return nil, errors.New("lua node requires id")
	}
	if cfg.Script == "" && cfg.ScriptPath == "" {
		return nil, errors.New("lua node requires script or script_path")
	}
	if cfg.StatusKey == "" {
		cfg.StatusKey = fmt.Sprintf("%s_status", cfg.ID)
	}
	return &LuaNode{cfg: cfg}, nil
}

func (n *LuaNode) Name() string {
	return n.cfg.ID
}

func (n *LuaNode) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
	var cancel context.CancelFunc
	if n.cfg.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, n.cfg.Timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	scriptPath, cleanup, err := n.prepareScript()
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	cmd := exec.CommandContext(ctx, "lua", append([]string{scriptPath}, n.cfg.Args...)...)
	if n.cfg.WorkingDir != "" {
		cmd.Dir = n.cfg.WorkingDir
	}
	cmd.Env = append(os.Environ(), n.cfg.EnvList()...)
	cmd.Env = append(cmd.Env, n.sharedEnv(shared)...)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("lua node %s failed: %w: %s", n.cfg.ID, err, stderr.String())
	}

	if n.cfg.StatusKey != "" {
		shared[n.cfg.StatusKey] = 0
	}

	action := goflow.ActionNext
	if n.cfg.ParseOutput {
		lines := strings.Split(stdout.String(), "\n")
		for _, raw := range lines {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			if strings.HasPrefix(raw, "action=") {
				action = strings.TrimPrefix(raw, "action=")
				continue
			}
			parts := strings.SplitN(raw, "=", 2)
			if len(parts) != 2 {
				continue
			}
			shared[parts[0]] = n.decodeValue(parts[1])
		}
	}

	return goflow.ResultWithAction(action), nil
}

func (n *LuaNode) prepareScript() (string, func(), error) {
	if n.cfg.ScriptPath != "" {
		if _, err := os.Stat(n.cfg.ScriptPath); err != nil {
			return "", nil, err
		}
		return n.cfg.ScriptPath, nil, nil
	}

	tmp, err := os.CreateTemp("", fmt.Sprintf("lua-node-%s-*.lua", n.cfg.ID))
	if err != nil {
		return "", nil, err
	}
	if _, err := tmp.WriteString(n.cfg.Script); err != nil {
		tmp.Close()
		return "", nil, err
	}
	if err := tmp.Close(); err != nil {
		return "", nil, err
	}
	return tmp.Name(), func() { os.Remove(tmp.Name()) }, nil
}

func (n *LuaNode) sharedEnv(shared map[string]any) []string {
	if len(shared) == 0 {
		return nil
	}
	keys := make([]string, 0, len(shared))
	vars := make([]string, 0, len(shared)+1)
	for key, value := range shared {
		sanitized := strings.ReplaceAll(strings.ToUpper(key), "=", "_")
		envKey := fmt.Sprintf("GOFLOW_SHARED_%s", sanitized)
		serialized, _ := json.Marshal(value)
		vars = append(vars, fmt.Sprintf("%s=%s", envKey, string(serialized)))
		keys = append(keys, sanitized)
	}
	vars = append(vars, fmt.Sprintf("GOFLOW_SHARED_KEYS=%s", strings.Join(keys, ",")))
	return vars
}

func (n *LuaNode) decodeValue(raw string) any {
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return parsed
	}
	return raw
}

func (cfg LuaNodeConfig) EnvList() []string {
	if len(cfg.Env) == 0 {
		return nil
	}
	result := make([]string, 0, len(cfg.Env))
	for key, value := range cfg.Env {
		result = append(result, fmt.Sprintf("%s=%s", key, value))
	}
	return result
}

func init() {
	RegisterNode(NodeDefinition{
		ID:          "lua",
		Description: "Runs a Lua script via the system interpreter and accepts shared data via GOFLOW_SHARED_* env vars.",
		Example:     `nodes.NewLuaNode(nodes.LuaNodeConfig{ID: "sanitizer", Script: "print('action=next')", ParseOutput: true})`,
	})
}
