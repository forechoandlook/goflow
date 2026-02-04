package nodes

import "sort"

// NodeDefinition captures metadata about a built-in node.
type NodeDefinition struct {
	ID          string
	Description string
	Example     string
}

var nodeCatalog = make(map[string]NodeDefinition)

// RegisterNode makes a node definition discoverable.
func RegisterNode(def NodeDefinition) {
	if def.ID == "" {
		return
	}
	nodeCatalog[def.ID] = def
}

// RegisteredNodes returns the known nodes sorted by ID.
func RegisteredNodes() []NodeDefinition {
	ids := make([]string, 0, len(nodeCatalog))
	for id := range nodeCatalog {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	result := make([]NodeDefinition, 0, len(ids))
	for _, id := range ids {
		result = append(result, nodeCatalog[id])
	}
	return result
}

// NodeDefinitionFor returns metadata for a registered node.
func NodeDefinitionFor(id string) (NodeDefinition, bool) {
	def, ok := nodeCatalog[id]
	return def, ok
}
