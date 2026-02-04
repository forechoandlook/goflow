package nodes

import (
	"context"
	"time"

	goflow "goflow"
)

// DelayNode waits for the configured duration before continuing.
type DelayNode struct {
	id       string
	Duration time.Duration
}

func NewDelayNode(id string, duration time.Duration) *DelayNode {
	return &DelayNode{id: id, Duration: duration}
}

func (dn *DelayNode) Name() string {
	return dn.id
}

func (dn *DelayNode) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(dn.Duration):
		return goflow.ResultWithAction(goflow.ActionNext), nil
	}
}

func init() {
	RegisterNode(NodeDefinition{
		ID:          "delay",
		Description: "Pauses execution for Duration then returns next.",
		Example:     `nodes.NewDelayNode("wait", 500*time.Millisecond)`,
	})
}
