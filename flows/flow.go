package flows

import (
	"context"
	"encoding/json"
	"fmt"
	goflow "goflow"
	"goflow/kv"
	"goflow/nodes"
	"sync"
	"time"
)

type Node = goflow.Node

// FlowOption represents configuration options for a flow
type FlowOption struct {
	EnableCheckpoint bool
	KVStore          kv.KVStore
	MaxSteps         int
	FlowID           string
	Timeout          time.Duration
}

// Flow represents a workflow with nodes and transitions
type Flow struct {
	start           Node
	nodes           map[string]Node              // name -> Node
	transitions     map[string]map[string]string // fromNode -> action -> toNodeName
	maxSteps        int
	mutex           sync.RWMutex
	currentNode     Node // Track the current node for Then() method
	signalListeners map[string][]Node

	// Checkpoint and state management
	checkpointEnabled bool
	kvStore           kv.KVStore
	flowID            string
	timeout           time.Duration
}

// FlowBuilder provides a fluent interface for building flows
type FlowBuilder struct {
	flow *Flow
}

func NewFlowBuilder(start Node) *FlowBuilder {
	flow := &Flow{
		start:             start,
		nodes:             make(map[string]Node),
		transitions:       make(map[string]map[string]string),
		maxSteps:          0,
		currentNode:       start, // Initialize to start node
		checkpointEnabled: false,
		kvStore:           nil,
		flowID:            "",
		timeout:           0,
		signalListeners:   make(map[string][]Node),
	}
	flow.nodes[start.Name()] = start

	return &FlowBuilder{
		flow: flow,
	}
}

func NewFlow(start Node) *Flow {
	return NewFlowBuilder(start).Build()
}

func NewFlowWithOptions(start Node, opts FlowOption) *Flow {
	builder := NewFlowBuilder(start)

	if opts.EnableCheckpoint {
		builder.WithCheckpoint(opts.KVStore, opts.FlowID)
	}

	if opts.MaxSteps > 0 {
		builder.WithMaxSteps(opts.MaxSteps)
	}

	return builder.Build()
}

// Then creates a sequential connection (shortcut for connecting with "next" action)
func (fb *FlowBuilder) Then(next Node) *FlowBuilder {
	fb.flow.Then(next)
	return fb
}

// Connect defines a transition from one node to another based on action
func (fb *FlowBuilder) Connect(from Node, action string, to Node) *FlowBuilder {
	fb.flow.Connect(from, action, to)
	return fb
}

// Listen registers an asynchronous listener that will run when the named signal is emitted.
func (fb *FlowBuilder) Listen(signal string, listener Node) *FlowBuilder {
	fb.flow.Listen(signal, listener)
	return fb
}

// WithOptions applies flow options
func (fb *FlowBuilder) WithOptions(opts FlowOption) *FlowBuilder {
	if opts.EnableCheckpoint {
		fb.WithCheckpoint(opts.KVStore, opts.FlowID)
	}

	if opts.MaxSteps > 0 {
		fb.WithMaxSteps(opts.MaxSteps)
	}

	fb.flow.timeout = opts.Timeout

	return fb
}

// WithCheckpoint enables checkpointing for the flow
func (fb *FlowBuilder) WithCheckpoint(store kv.KVStore, flowID string) *FlowBuilder {
	fb.flow.checkpointEnabled = true
	fb.flow.kvStore = store
	fb.flow.flowID = flowID
	return fb
}

// WithMaxSteps sets the maximum number of steps for the flow
func (fb *FlowBuilder) WithMaxSteps(max int) *FlowBuilder {
	fb.flow.maxSteps = max
	return fb
}

// Build returns the constructed flow
func (fb *FlowBuilder) Build() *Flow {
	return fb.flow
}

// Add adds a node to the flow
func (f *Flow) Add(node Node) *Flow {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.nodes[node.Name()] = node
	return f // 返回自身，支持链式
}

// Listen registers a node that runs asynchronously when the given signal fires.
func (f *Flow) Listen(signal string, listener Node) *Flow {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.signalListeners[signal] = append(f.signalListeners[signal], listener)
	return f
}

// Connect defines a transition from one node to another based on action
func (f *Flow) Connect(from Node, action string, to Node) *Flow {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	fromName := from.Name()
	if _, ok := f.transitions[fromName]; !ok {
		f.transitions[fromName] = make(map[string]string)
	}

	if to != nil {
		f.transitions[fromName][action] = to.Name()
		f.nodes[to.Name()] = to
	} else {
		f.transitions[fromName][action] = ""
	}

	return f
}

// Then creates a sequential connection (shortcut for connecting with "next" action)
func (f *Flow) Then(next Node) *Flow {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	fromNodeName := f.currentNode.Name()

	if _, ok := f.transitions[fromNodeName]; !ok {
		f.transitions[fromNodeName] = make(map[string]string)
	}

	f.transitions[fromNodeName]["next"] = next.Name()
	f.nodes[next.Name()] = next

	// Update the current node to the newly added node
	f.currentNode = next

	return f
}

// WithMaxSteps sets the maximum number of steps for the flow
func (f *Flow) WithMaxSteps(max int) *Flow {
	f.maxSteps = max
	return f
}

// Run executes the flow
func (f *Flow) Run(ctx context.Context, shared map[string]any) error {
	f.mutex.RLock()
	current := f.start
	steps := 0
	f.mutex.RUnlock()

	// Apply timeout if configured
	if f.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, f.timeout)
		defer cancel()
	}

	// If checkpointing is enabled, try to restore from previous state
	if f.checkpointEnabled && f.kvStore != nil {
		restoredState, err := f.restoreFromCheckpoint()
		if err == nil && restoredState != nil {
			// Restore the shared state and potentially other execution state
			for k, v := range restoredState {
				if k != "current_node" && k != "step_count" {
					shared[k] = v
				}
			}

			// Restore current node and step count if available
			if currentNodeName, ok := restoredState["current_node"].(string); ok {
				f.mutex.RLock()
				current = f.nodes[currentNodeName]
				f.mutex.RUnlock()
			}

			if stepCount, ok := restoredState["step_count"].(float64); ok {
				steps = int(stepCount)
			}
		}
	}

	for current != nil {
		// Check max steps
		if f.maxSteps > 0 && steps >= f.maxSteps {
			return fmt.Errorf("max steps exceeded: %d", f.maxSteps)
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			// If checkpointing is enabled, save state before exiting
			if f.checkpointEnabled && f.kvStore != nil {
				f.saveCheckpoint(current.Name(), steps, shared)
			}
			return ctx.Err()
		default:
		}

		// Execute current node (respecting attribute-driven retries)
		result, err := f.runNodeWithAttributes(ctx, current, shared)
		if err != nil {
			// If checkpointing is enabled, save error state
			if f.checkpointEnabled && f.kvStore != nil {
				f.saveCheckpoint(current.Name(), steps, shared)
			}
			return err
		}

		f.emitSignals(ctx, result, shared)

		// Check for end conditions
		action := goflow.ActionFromResult(result)
		if action == "" || action == goflow.ActionEnd {
			break
		}

		// Save checkpoint if enabled
		if f.checkpointEnabled && f.kvStore != nil {
			f.saveCheckpoint(current.Name(), steps, shared)
		}

		// Find next node
		f.mutex.RLock()
		nextName, ok := f.transitions[current.Name()][action]
		f.mutex.RUnlock()

		if !ok || nextName == "" {
			break
		}

		f.mutex.RLock()
		current = f.nodes[nextName]
		f.mutex.RUnlock()

		steps++
	}

	return nil
}

func (f *Flow) runNodeWithAttributes(ctx context.Context, node Node, shared map[string]any) (goflow.NodeResult, error) {
	attrs := nodes.NodeAttributes{}
	if aware, ok := node.(nodes.AttributeAwareNode); ok {
		attrs = aware.Attributes()
	}

	attempts := 0
	for {
		result, err := node.Run(ctx, shared)
		if err == nil {
			return result, nil
		}

		if attempts >= attrs.RetryAttempts {
			return nil, err
		}

		attempts++
		if attrs.RetryDelay > 0 {
			timer := time.NewTimer(attrs.RetryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
}

func (f *Flow) emitSignals(ctx context.Context, result goflow.NodeResult, shared map[string]any) {
	signals := goflow.SignalsFromResult(result)
	if len(signals) == 0 {
		return
	}

	for _, signal := range signals {
		f.mutex.RLock()
		listeners := append([]Node(nil), f.signalListeners[signal]...)
		f.mutex.RUnlock()

		for _, listener := range listeners {
			go f.runSignalListener(ctx, listener, shared)
		}
	}
}

func (f *Flow) runSignalListener(ctx context.Context, node Node, shared map[string]any) {
	_, _ = f.runNodeWithAttributes(ctx, node, shared)
}

// restoreFromCheckpoint attempts to restore the flow state from checkpoint
func (f *Flow) restoreFromCheckpoint() (map[string]any, error) {
	if f.kvStore == nil || f.flowID == "" {
		return nil, fmt.Errorf("KV store not initialized or flow ID not set")
	}

	key := fmt.Sprintf("flow_%s_state", f.flowID)
	value, err := f.kvStore.Get(key)
	if err != nil {
		return nil, err
	}

	var state map[string]any
	if err := json.Unmarshal(value, &state); err != nil {
		return nil, err
	}

	return state, nil
}

// saveCheckpoint saves the current flow state
func (f *Flow) saveCheckpoint(currentNodeName string, stepCount int, sharedState map[string]any) error {
	if f.kvStore == nil || f.flowID == "" {
		return fmt.Errorf("KV store not initialized or flow ID not set")
	}

	// Create a copy of the shared state and add execution metadata
	checkpointData := make(map[string]any)
	for k, v := range sharedState {
		checkpointData[k] = v
	}
	checkpointData["current_node"] = currentNodeName
	checkpointData["step_count"] = stepCount

	// Serialize the state
	jsonData, err := json.Marshal(checkpointData)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("flow_%s_state", f.flowID)
	return f.kvStore.Put(key, jsonData)
}

// MutexRLock acquires a read lock on the flow's mutex
func (f *Flow) MutexRLock() {
	f.mutex.RLock()
}

// MutexRUnlock releases a read lock on the flow's mutex
func (f *Flow) MutexRUnlock() {
	f.mutex.RUnlock()
}

// GetStartNode returns the start node of the flow
func (f *Flow) GetStartNode() Node {
	return f.start
}

// GetNodeByName returns a node by its name
func (f *Flow) GetNodeByName(name string) Node {
	return f.nodes[name]
}

// GetTransition returns the next node name for a given node and action
func (f *Flow) GetTransition(nodeName, action string) (string, bool) {
	nextName, ok := f.transitions[nodeName][action]
	return nextName, ok
}
