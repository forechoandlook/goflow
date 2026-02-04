package nodes

import (
	"context"
	"time"

	goflow "goflow"
)

// NodeAttributes describes optional metadata that Flow can act on before or
// after invoking a node implementation.
type NodeAttributes struct {
	// RetryAttempts is the number of additional times to rerun the node when
	// Run returns an error. Zero means do not retry.
	RetryAttempts int
	// RetryDelay is the pause between retry attempts.
	RetryDelay time.Duration
}

// AttributeAwareNode exposes attributes that flows can inspect at runtime.
type AttributeAwareNode interface {
	Node
	Attributes() NodeAttributes
}

// WrapNodeWithAttributes decorates any node with metadata so flows can honor
// retry/delay rules without altering the original implementation.
func WrapNodeWithAttributes(node Node, attrs NodeAttributes) Node {
	if node == nil {
		return nil
	}
	if _, ok := node.(AttributeAwareNode); ok {
		return node
	}
	return &attrNode{inner: node, attrs: attrs}
}

type attrNode struct {
	inner Node
	attrs NodeAttributes
}

func (a *attrNode) Name() string {
	return a.inner.Name()
}

func (a *attrNode) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
	return a.inner.Run(ctx, shared)
}

func (a *attrNode) Attributes() NodeAttributes {
	return a.attrs
}
