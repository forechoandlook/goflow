package nodes

import (
	"context"

	goflow "goflow"
)

// ConditionalNode routes to sub-flows based on a predicate.
type ConditionalNode struct {
	id        string
	condition func(map[string]any) string
	branches  map[string]FlowRunner
}

func NewConditionalNode(id string, condition func(map[string]any) string) *ConditionalNode {
	return &ConditionalNode{
		id:        id,
		condition: condition,
		branches:  make(map[string]FlowRunner),
	}
}

func (cn *ConditionalNode) Name() string {
	return cn.id
}

func (cn *ConditionalNode) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
	action := cn.condition(shared)
	if action == "" {
		return goflow.ResultWithAction(goflow.ActionNext), nil
	}
	if branch, exists := cn.branches[action]; exists {
		if err := branch.Run(ctx, shared); err != nil {
			return nil, err
		}
		return goflow.ResultWithAction(goflow.ActionNext), nil
	}
	return goflow.NodeResult{"action": action}, nil
}

func (cn *ConditionalNode) Branch(action string, flow FlowRunner) *ConditionalNode {
	cn.branches[action] = flow
	return cn
}

func init() {
	RegisterNode(NodeDefinition{
		ID:          "conditional",
		Description: "Routes execution to sub-flows based on a predicate result.",
		Example:     `nodes.NewConditionalNode("router", func(shared map[string]any) string { return shared["route"].(string) })`,
	})
}
