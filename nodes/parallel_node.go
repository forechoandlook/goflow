package nodes

import (
	"context"
	"sync"

	goflow "goflow"
)

// ParallelNode executes multiple subflows concurrently.
type ParallelNode struct {
	id       string
	subFlows []FlowRunner
}

func NewParallelNode(id string, flows ...FlowRunner) *ParallelNode {
	return &ParallelNode{id: id, subFlows: flows}
}

func (pn *ParallelNode) Name() string {
	return pn.id
}

func (pn *ParallelNode) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for _, flow := range pn.subFlows {
		wg.Add(1)
		go func(subFlow FlowRunner) {
			defer wg.Done()
			copied := copyShared(shared)
			if err := subFlow.Run(ctx, copied); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			mu.Lock()
			for k, v := range copied {
				shared[k] = v
			}
			mu.Unlock()
		}(flow)
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	return goflow.ResultWithAction(goflow.ActionNext), nil
}

func copyShared(shared map[string]any) map[string]any {
	result := make(map[string]any, len(shared))
	for k, v := range shared {
		result[k] = v
	}
	return result
}

func init() {
	RegisterNode(NodeDefinition{
		ID:          "parallel",
		Description: "Runs sub-flows concurrently and merges their shared state.",
		Example:     `nodes.NewParallelNode("fans", flowA, flowB)`,
	})
}
