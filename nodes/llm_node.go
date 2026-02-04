package nodes

import (
	"context"
	"fmt"
	"strings"

	openai "github.com/sashabaranov/go-openai"
	goflow "goflow"
)

// LLMNodeConfig controls how the LLM is called.
type LLMNodeConfig struct {
	Name         string
	Model        string
	SystemPrompt string
	InputKey     string
	OutputKey    string
	Temperature  float32
	MaxTokens    int
	Stop         []string
}

// DefaultLLMNodeConfig returns a starter config for a prompt-based node.
func DefaultLLMNodeConfig(prompt string) LLMNodeConfig {
	return LLMNodeConfig{
		Name:         "llm:" + prompt,
		Model:        openai.GPT3Dot5Turbo,
		SystemPrompt: "You are a helpful assistant.",
		InputKey:     "input",
		OutputKey:    "llm_output",
		Temperature:  0.5,
		MaxTokens:    256,
		Stop:         []string{},
	}
}

// LLMNode calls OpenAI's chat completion API to produce text.
type LLMNode struct {
	client *openai.Client
	cfg    LLMNodeConfig
}

func NewLLMNode(client *openai.Client, cfg LLMNodeConfig) *LLMNode {
	if cfg.Name == "" {
		cfg.Name = "llm-node"
	}
	if cfg.Model == "" {
		cfg.Model = openai.GPT3Dot5Turbo
	}
	if cfg.OutputKey == "" {
		cfg.OutputKey = "llm_output"
	}
	if cfg.InputKey == "" {
		cfg.InputKey = "input"
	}
	return &LLMNode{client: client, cfg: cfg}
}

func (n *LLMNode) Name() string {
	return n.cfg.Name
}

func (n *LLMNode) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
	if n.client == nil {
		input := fmt.Sprintf("%v", shared[n.cfg.InputKey])
		mock := fmt.Sprintf("mock response for %s", input)
		shared[n.cfg.OutputKey] = mock
		shared["llm_response"] = mock
		return goflow.NodeResult{"action": goflow.ActionNext, "llm_output": mock}, nil
	}

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: n.cfg.SystemPrompt},
	}

	if raw, ok := shared[n.cfg.InputKey]; ok {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: fmt.Sprintf("%v", raw),
		})
	}

	resp, err := n.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       n.cfg.Model,
		Messages:    messages,
		Temperature: n.cfg.Temperature,
		MaxTokens:   n.cfg.MaxTokens,
		Stop:        n.cfg.Stop,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM node %s call failed: %w", n.Name(), err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("LLM node %s returned empty choice list", n.Name())
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	if content == "" {
		content = resp.Choices[0].Message.Content
	}

	result := goflow.NodeResult{
		"llm_output":   content,
		"llm_response": resp.Choices[0].Message.Content,
		"action":       goflow.ActionNext,
	}

	shared[n.cfg.OutputKey] = content
	shared["llm_response"] = resp.Choices[0].Message.Content
	return result, nil
}

func init() {
	RegisterNode(NodeDefinition{
		ID:          "llm",
		Description: "Calls OpenAI via go-openai; nil client produces a mock response.",
		Example:     `nodes.NewLLMNode(client, nodes.DefaultLLMNodeConfig("translate to french"))`,
	})
}
