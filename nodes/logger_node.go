package nodes

import (
	"context"
	"fmt"
	"log"
	"strings"

	goflow "goflow"
)

// LoggerNode writes shared values to stdout for debugging.
type LoggerNode struct {
	id        string
	Message   string
	InputKeys []string
}

func NewLoggerNode(id, message string, inputKeys ...string) *LoggerNode {
	return &LoggerNode{id: id, Message: message, InputKeys: inputKeys}
}

func (ln *LoggerNode) Name() string {
	return ln.id
}

func (ln *LoggerNode) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
	parts := []string{ln.Message}
	for _, key := range ln.InputKeys {
		if val, ok := shared[key]; ok {
			parts = append(parts, fmt.Sprintf("%s=%v", key, val))
		}
	}
	log.Println(strings.Join(parts, " | "))
	return goflow.ResultWithAction(goflow.ActionNext), nil
}

func init() {
	RegisterNode(NodeDefinition{
		ID:          "logger",
		Description: "Emits selected shared keys and a custom message to stdout.",
		Example:     `nodes.NewLoggerNode("debug", "shared", "input", "result")`,
	})
}
