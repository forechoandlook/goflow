package checkpointer

import (
	"context"
	"encoding/json"
	"fmt"
	goflow "goflow"
	flows "goflow/flows"
	kvstore "goflow/kv"
	"sync"
	"time"
)

type Node = goflow.Node

// Checkpoint stores the execution state of a flow
type Checkpoint struct {
	ThreadID    string         `json:"thread_id"`
	CurrentNode string         `json:"current_node"`
	Shared      map[string]any `json:"shared"`
	StepCount   int            `json:"step_count"`
	Timestamp   time.Time      `json:"timestamp"`
	History     []string       `json:"history,omitempty"`
}

// Checkpointer interface defines methods for saving and loading checkpoints
type Checkpointer interface {
	Save(ctx context.Context, cp *Checkpoint) error
	Load(ctx context.Context, threadID string) (*Checkpoint, error)
	ListThreads() ([]string, error)
}

// MemoryCheckpointer implements checkpointing in memory
type MemoryCheckpointer struct {
	mu    sync.RWMutex
	store map[string]*Checkpoint
}

func NewMemoryCheckpointer() *MemoryCheckpointer {
	return &MemoryCheckpointer{
		store: make(map[string]*Checkpoint),
	}
}

func (m *MemoryCheckpointer) Save(_ context.Context, cp *Checkpoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[cp.ThreadID] = cp
	return nil
}

func (m *MemoryCheckpointer) Load(_ context.Context, threadID string) (*Checkpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp, ok := m.store[threadID]
	if !ok {
		return nil, fmt.Errorf("checkpoint not found for thread: %s", threadID)
	}
	return cp, nil
}

func (m *MemoryCheckpointer) ListThreads() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	threads := make([]string, 0, len(m.store))
	for threadID := range m.store {
		threads = append(threads, threadID)
	}
	return threads, nil
}

// KVCheckpointer implements checkpointing using a key-value store
type KVCheckpointer struct {
	store kvstore.KVStore
	mu    sync.RWMutex
}

func NewKVCheckpointer(store kvstore.KVStore) *KVCheckpointer {
	return &KVCheckpointer{
		store: store,
	}
}

func (kvc *KVCheckpointer) Save(_ context.Context, cp *Checkpoint) error {
	kvc.mu.Lock()
	defer kvc.mu.Unlock()

	data, err := json.Marshal(cp)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	key := "checkpoint:" + cp.ThreadID
	return kvc.store.Put(key, data)
}

func (kvc *KVCheckpointer) Load(_ context.Context, threadID string) (*Checkpoint, error) {
	kvc.mu.RLock()
	defer kvc.mu.RUnlock()

	key := "checkpoint:" + threadID
	data, err := kvc.store.Get(key)
	if err != nil {
		return nil, fmt.Errorf("checkpoint not found for thread: %s", threadID)
	}

	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &cp, nil
}

func (kvc *KVCheckpointer) ListThreads() ([]string, error) {
	// This is a simplified implementation - in a real system you might maintain
	// a separate index of thread IDs
	return nil, fmt.Errorf("ListThreads not implemented for KVCheckpointer")
}

// RunWithCheckpoint executes a flow with automatic checkpointing
func RunWithCheckpoint(ctx context.Context, flow *flows.Flow, threadID string, initialShared map[string]any, checkpointer Checkpointer) error {
	var shared map[string]any
	var current Node

	// Try to restore from checkpoint
	cp, err := checkpointer.Load(ctx, threadID)
	var steps int
	if err == nil && cp != nil {
		// Restore state
		shared = cp.Shared
		flow.MutexRLock()
		current = flow.GetNodeByName(cp.CurrentNode)
		flow.MutexRUnlock()
		fmt.Printf("从 checkpoint 恢复: node=%s, steps=%d\n", current.Name(), cp.StepCount)
		steps = cp.StepCount
	} else {
		// Start fresh
		shared = initialShared
		flow.MutexRLock()
		current = flow.GetStartNode()
		flow.MutexRUnlock()
		steps = 0
	}
	for current != nil {
		// Check context cancellation
		select {
		case <-ctx.Done():
			// Save current state before returning
			checkpoint := &Checkpoint{
				ThreadID:    threadID,
				CurrentNode: current.Name(),
				Shared:      shared,
				StepCount:   steps,
				Timestamp:   time.Now(),
			}
			checkpointer.Save(ctx, checkpoint)
			return ctx.Err()
		default:
		}

		// Execute current node
		result, err := current.Run(ctx, shared)
		if err != nil {
			return err
		}

		action := goflow.ActionFromResult(result)

		// Check for pause action
		if action == goflow.ActionPause {
			checkpoint := &Checkpoint{
				ThreadID:    threadID,
				CurrentNode: current.Name(),
				Shared:      shared,
				StepCount:   steps,
				Timestamp:   time.Now(),
			}
			checkpointer.Save(ctx, checkpoint)
			return fmt.Errorf("paused for human intervention at node: %s", current.Name())
		}

		// Save checkpoint after each successful step
		checkpoint := &Checkpoint{
			ThreadID:    threadID,
			CurrentNode: current.Name(),
			Shared:      shared,
			StepCount:   steps + 1,
			Timestamp:   time.Now(),
		}
		checkpointer.Save(ctx, checkpoint)

		// Find next node
		flow.MutexRLock()
		nextName, ok := flow.GetTransition(current.Name(), action)
		flow.MutexRUnlock()

		if !ok || nextName == "" {
			break
		}

		flow.MutexRLock()
		current = flow.GetNodeByName(nextName)
		flow.MutexRUnlock()

		steps++
	}

	return nil
}
