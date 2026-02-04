package nodes

import (
	"context"
	"fmt"

	goflow "goflow"
	"goflow/kv"
)

// KVReadNode reads a value from the provided KV store.
type KVReadNode struct {
	id        string
	store     kv.KVStore
	Key       string
	OutputKey string
}

func NewKVReadNode(id string, store kv.KVStore, key, outputKey string) *KVReadNode {
	return &KVReadNode{
		id:        id,
		store:     store,
		Key:       key,
		OutputKey: outputKey,
	}
}

func (n *KVReadNode) Name() string {
	return n.id
}

func (n *KVReadNode) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
	if n.store == nil {
		return nil, fmt.Errorf("kv store not configured for node %s", n.Name())
	}
	value, err := n.store.Get(n.Key)
	if err != nil {
		return nil, err
	}
	shared[n.OutputKey] = string(value)
	return goflow.ResultWithAction(goflow.ActionNext), nil
}

// KVWriteNode writes a shared value into the KV store.
type KVWriteNode struct {
	id       string
	store    kv.KVStore
	Key      string
	InputKey string
}

func NewKVWriteNode(id string, store kv.KVStore, key, inputKey string) *KVWriteNode {
	return &KVWriteNode{
		id:       id,
		store:    store,
		Key:      key,
		InputKey: inputKey,
	}
}

func (n *KVWriteNode) Name() string {
	return n.id
}

func (n *KVWriteNode) Run(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
	if n.store == nil {
		return nil, fmt.Errorf("kv store not configured for node %s", n.Name())
	}
	value, ok := shared[n.InputKey]
	if !ok {
		return nil, fmt.Errorf("input key %s missing for node %s", n.InputKey, n.Name())
	}
	str := fmt.Sprintf("%v", value)
	if err := n.store.Put(n.Key, []byte(str)); err != nil {
		return nil, err
	}
	return goflow.ResultWithAction(goflow.ActionNext), nil
}

func init() {
	RegisterNode(NodeDefinition{
		ID:          "kv_read",
		Description: "Pulls a string from a KV store and writes it back into shared state.",
		Example:     `nodes.NewKVReadNode("load", store, "key", "loaded")`,
	})
	RegisterNode(NodeDefinition{
		ID:          "kv_write",
		Description: "Persists the value under InputKey into the KV store key.",
		Example:     `nodes.NewKVWriteNode("persist", store, "key", "value")`,
	})
}
