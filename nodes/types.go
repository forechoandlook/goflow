package nodes

import (
	"context"

	goflow "goflow"
)

// Node is an alias for the core workflow node interface.
type Node = goflow.Node

// FlowRunner can execute a flow-like sequence of nodes.
type FlowRunner interface {
	Run(ctx context.Context, shared map[string]any) error
}
