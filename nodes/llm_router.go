package nodes

import (
	"context"
	"fmt"
	"strings"

	openai "github.com/sashabaranov/go-openai"
	goflow "goflow"
)

// LLMRouterConfig configures how a router turns LLM outputs into actions.
type LLMRouterConfig struct {
	Name        string
	Model       string
	Prompt      string
	Actions     []string
	InputKey    string
	Default     string
	Temperature float32
	MaxTokens   int
}

// LLMRouter asks the LLM which branch to execute next.
type LLMRouter struct {
	client *openai.Client
	cfg    LLMRouterConfig
}

func NewLLMRouter(client *openai.Client, cfg LLMRouterConfig) *LLMRouter {
	if cfg.Name == "" {
		cfg.Name = "llm-router"
	}
	if cfg.Model == "" {
		cfg.Model = openai.GPT3Dot5Turbo
	}
	if cfg.InputKey == "" {
		cfg.InputKey = "input"
	}
	if cfg.Default == "" && len(cfg.Actions) > 0 {
		cfg.Default = cfg.Actions[0]
	}
	return &LLMRouter{client: client, cfg: cfg}
}

func (lr *LLMRouter) Name() string {
	return lr.cfg.Name
}

func (lr *LLMRouter) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
	if lr.client == nil {
		return goflow.NodeResult{"action": lr.cfg.Default}, nil
	}

	inText := fmt.Sprintf("%v", shared[lr.cfg.InputKey])
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: lr.cfg.Prompt},
		{Role: openai.ChatMessageRoleUser, Content: inText},
	}

	resp, err := lr.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       lr.cfg.Model,
		Messages:    messages,
		Temperature: lr.cfg.Temperature,
		MaxTokens:   lr.cfg.MaxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM router %s call failed: %w", lr.Name(), err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("LLM router %s returned no choices", lr.Name())
	}

	answer := strings.ToLower(strings.TrimSpace(resp.Choices[0].Message.Content))
	for _, action := range lr.cfg.Actions {
		if strings.Contains(answer, strings.ToLower(action)) {
			return goflow.NodeResult{"action": action}, nil
		}
	}
	return goflow.NodeResult{"action": lr.cfg.Default}, nil
}

func init() {
	RegisterNode(NodeDefinition{
		ID:          "llm_router",
		Description: "Prompts an LLM to pick a named action from the supplied list.",
		Example:     `nodes.NewLLMRouter(client, nodes.LLMRouterConfig{Actions: []string{"search","summarize"}, Prompt: "Pick one action" })`,
	})
}
