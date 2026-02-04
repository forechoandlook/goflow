package nodes

import (
	"context"

	goflow "goflow"
)

// FunctionNode wraps a callback to satisfy the Node contract.
type FunctionNode struct {
	id string
	fn func(context.Context, map[string]any) (goflow.NodeResult, error)
}

// NewFunctionNode accepts a callback that returns full NodeResult payloads.
func NewFunctionNode(id string, fn func(context.Context, map[string]any) (goflow.NodeResult, error)) *FunctionNode {
	return &FunctionNode{id: id, fn: fn}
}

func (n *FunctionNode) Name() string {
	return n.id
}

func (n *FunctionNode) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
	if n.fn == nil {
		return goflow.ResultWithAction(goflow.ActionNext), nil
	}
	return n.fn(ctx, shared)
}

func init() {
	RegisterNode(NodeDefinition{
		ID:          "function",
		Description: "Wraps a Go callback so you can inline custom logic inside a flow.",
		Example:     `nodes.NewFunctionNode("clean", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) { shared["status"] = "cleaned"; return goflow.ResultWithAction(goflow.ActionNext), nil })`,
	})
}
