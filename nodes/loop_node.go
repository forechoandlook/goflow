package nodes

import (
	"context"

	goflow "goflow"
)

// LoopNode repeats until max iterations are reached.
type LoopNode struct {
	name          string
	maxIterations int
	counter       int
}

func NewLoopNode(name string, maxIterations int) *LoopNode {
	return &LoopNode{name: name, maxIterations: maxIterations}
}

func (l *LoopNode) Name() string {
	return l.name
}

func (l *LoopNode) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
	l.counter++
	if l.counter >= l.maxIterations {
		return goflow.ResultWithAction(goflow.ActionNext), nil
	}
	return goflow.ResultWithAction(goflow.ActionContinue), nil
}

func init() {
	RegisterNode(NodeDefinition{
		ID:          "loop",
		Description: "Repeats execution a fixed number of times, emitting continue until complete.",
		Example:     `nodes.NewLoopNode("retry", 3)`,
	})
}
