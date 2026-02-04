package flows

import (
	"context"
	"fmt"
	goflow "goflow"
	"goflow/kv"
	"goflow/nodes"
	"sync"
	"time"
)

// FlowEventType enumerates observable lifecycle hooks emitted by a flow.
type FlowEventType string

const (
	FlowEventTypeFlowStart     FlowEventType = "flow_start"
	FlowEventTypeNodeStart     FlowEventType = "node_start"
	FlowEventTypeNodeEnd       FlowEventType = "node_end"
	FlowEventTypeNodeError     FlowEventType = "node_error"
	FlowEventTypeNodeRetry     FlowEventType = "node_retry"
	FlowEventTypeSignalEmitted FlowEventType = "signal_emitted"
	FlowEventTypeFlowComplete  FlowEventType = "flow_complete"
)

// FlowEvent carries metadata that observability hooks can use.
type FlowEvent struct {
	Type      FlowEventType
	Timestamp time.Time
	Node      string
	Action    string
	Result    goflow.NodeResult
	Err       error
	Attempt   int
	Signals   []string
	Shared    map[string]any
}

// FlowMonitor observes lifecycle events emitted by Flow.Run().
type FlowMonitor interface {
	Notify(ctx context.Context, event FlowEvent)
}

type Node = goflow.Node

// FlowOption represents configuration options for a flow
type FlowOption struct {
	EnableCheckpoint bool
	KVStore          kv.KVStore
	MaxSteps         int
	FlowID           string
	Timeout          time.Duration
	Monitors         []FlowMonitor
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
	monitors        []FlowMonitor
	monitorMux      sync.RWMutex

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
		monitors:          nil,
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

	if len(opts.Monitors) > 0 {
		builder.WithMonitors(opts.Monitors...)
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

// WithMonitor registers an observability hook for the flow.
func (fb *FlowBuilder) WithMonitor(monitor FlowMonitor) *FlowBuilder {
	if monitor == nil {
		return fb
	}
	fb.flow.AddMonitor(monitor)
	return fb
}

// WithMonitors registers multiple observability hooks for the flow.
func (fb *FlowBuilder) WithMonitors(monitors ...FlowMonitor) *FlowBuilder {
	for _, monitor := range monitors {
		fb.WithMonitor(monitor)
	}
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

	if len(opts.Monitors) > 0 {
		fb.WithMonitors(opts.Monitors...)
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

// Add attaches a node to the flow while preserving builder chaining.
func (fb *FlowBuilder) Add(node Node) *FlowBuilder {
	fb.flow.Add(node)
	return fb
}

// Add adds a node to the flow
func (f *Flow) Add(node Node) *Flow {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.nodes[node.Name()] = node
	return f // 返回自身，支持链式
}

// AddMonitor registers a FlowMonitor for the flow.
func (f *Flow) AddMonitor(monitor FlowMonitor) *Flow {
	if monitor == nil {
		return f
	}
	f.monitorMux.Lock()
	f.monitors = append(f.monitors, monitor)
	f.monitorMux.Unlock()
	return f
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
func (f *Flow) Run(ctx context.Context, shared map[string]any) (runErr error) {
	f.mutex.RLock()
	current := f.start
	steps := 0
	f.mutex.RUnlock()

	lastNodeName := current.Name()

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
			for k, v := range restoredState {
				if k == "current_node" || k == "step_count" {
					continue
				}
				shared[k] = v
			}

			if currentNodeName, ok := restoredState["current_node"].(string); ok {
				f.mutex.RLock()
				if restored, exists := f.nodes[currentNodeName]; exists {
					current = restored
				}
				f.mutex.RUnlock()
			}

			if stepCount, ok := restoredState["step_count"].(float64); ok {
				steps = int(stepCount)
			}
		}
	}

	lastNodeName = current.Name()

	f.emitEvent(ctx, FlowEvent{
		Type:   FlowEventTypeFlowStart,
		Node:   current.Name(),
		Shared: snapshotShared(shared),
	})

	defer func() {
		f.emitEvent(ctx, FlowEvent{
			Type:   FlowEventTypeFlowComplete,
			Node:   lastNodeName,
			Err:    runErr,
			Shared: snapshotShared(shared),
		})
	}()

	for current != nil {
		if f.maxSteps > 0 && steps >= f.maxSteps {
			return fmt.Errorf("max steps exceeded: %d", f.maxSteps)
		}

		var shouldContinue bool
		var err error
		var nextNode Node

		nextNode, shouldContinue, err = f.executeStep(ctx, current, shared, steps)
		if err != nil {
			return err
		}

		if !shouldContinue {
			break
		}

		current = nextNode
		steps++
		lastNodeName = current.Name()
	}

	return nil
}

func (f *Flow) executeStep(ctx context.Context, current Node, shared map[string]any, steps int) (Node, bool, error) {
	select {
	case <-ctx.Done():
		if f.checkpointEnabled && f.kvStore != nil {
			_ = f.saveCheckpoint(current.Name(), steps, shared)
		}
		return nil, false, ctx.Err()
	default:
	}

	f.emitEvent(ctx, FlowEvent{
		Type:   FlowEventTypeNodeStart,
		Node:   current.Name(),
		Shared: snapshotShared(shared),
	})

	result, attempts, err := f.runNodeWithAttributes(ctx, current, shared)
	if err != nil {
		if f.checkpointEnabled && f.kvStore != nil {
			_ = f.saveCheckpoint(current.Name(), steps, shared)
		}

		f.emitEvent(ctx, FlowEvent{
			Type:    FlowEventTypeNodeError,
			Node:    current.Name(),
			Err:     err,
			Attempt: attempts,
			Shared:  snapshotShared(shared),
		})

		return nil, false, err
	}

	action := goflow.ActionFromResult(result)
	signals := goflow.SignalsFromResult(result)

	f.emitEvent(ctx, FlowEvent{
		Type:    FlowEventTypeNodeEnd,
		Node:    current.Name(),
		Action:  action,
		Result:  result,
		Attempt: attempts,
		Signals: signals,
		Shared:  snapshotShared(shared),
	})

	f.emitSignals(ctx, current, signals, shared)

	if action == "" || action == goflow.ActionEnd {
		return nil, false, nil
	}

	if f.checkpointEnabled && f.kvStore != nil {
		_ = f.saveCheckpoint(current.Name(), steps, shared)
	}

	f.mutex.RLock()
	nextName, ok := f.transitions[current.Name()][action]
	f.mutex.RUnlock()

	if !ok || nextName == "" {
		return nil, false, nil
	}

	f.mutex.RLock()
	nextNode := f.nodes[nextName]
	f.mutex.RUnlock()

	return nextNode, true, nil
}

func (f *Flow) runNodeWithAttributes(ctx context.Context, node Node, shared map[string]any) (goflow.NodeResult, int, error) {
	attrs := nodes.NodeAttributes{}
	if aware, ok := node.(nodes.AttributeAwareNode); ok {
		attrs = aware.Attributes()
	}

	retries := 0
	for {
		attempt := retries + 1
		result, err := node.Run(ctx, shared)
		if err == nil {
			return result, attempt, nil
		}

		if retries >= attrs.RetryAttempts {
			return nil, attempt, err
		}

		f.emitEvent(ctx, FlowEvent{
			Type:    FlowEventTypeNodeRetry,
			Node:    node.Name(),
			Err:     err,
			Attempt: attempt,
			Shared:  snapshotShared(shared),
		})

		retries++
		if attrs.RetryDelay > 0 {
			timer := time.NewTimer(attrs.RetryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, attempt, ctx.Err()
			case <-timer.C:
			}
		}
	}
}


func snapshotShared(shared map[string]any) map[string]any {
	if shared == nil {
		return nil
	}

	copy := make(map[string]any, len(shared))
	for k, v := range shared {
		copy[k] = v
	}

	return copy
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
