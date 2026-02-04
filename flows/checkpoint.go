package flows

import (
	"encoding/json"
	"fmt"
)

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
