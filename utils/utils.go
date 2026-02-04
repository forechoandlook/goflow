package utils

import (
	"context"
	"time"

	goflow "goflow"
)

type Node = goflow.Node

// WithTimeout wraps a function call with a timeout
func WithTimeout(parentCtx context.Context, timeout time.Duration, fn func(ctx context.Context) error) error {
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- fn(ctx)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// WithTimeoutOnNode wraps a node's Run method with a timeout
func WithTimeoutOnNode(node Node, timeout time.Duration) Node {
	return &TimeoutNode{
		Node:    node,
		Timeout: timeout,
	}
}

// TimeoutNode wraps another node with a timeout
type TimeoutNode struct {
	Node
	Timeout time.Duration
}

func (tn *TimeoutNode) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
	ctx, cancel := context.WithTimeout(ctx, tn.Timeout)
	defer cancel()

	result := make(chan struct {
		result goflow.NodeResult
		err    error
	}, 1)

	go func() {
		res, err := tn.Node.Run(ctx, shared)
		result <- struct {
			result goflow.NodeResult
			err    error
		}{res, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-result:
		return res.result, res.err
	}
}

// WithRetry executes a function with retry logic
func WithRetry(maxRetries int, backoff time.Duration, fn func() error) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		if i < maxRetries-1 {
			time.Sleep(backoff)
		}
	}
	return err
}

// MergeMaps merges multiple maps into one, with later maps overriding earlier ones
func MergeMaps(maps ...map[string]any) map[string]any {
	result := make(map[string]any)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}
