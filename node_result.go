package goflow

// ActionFromResult extracts the action label from a NodeResult.
func ActionFromResult(res NodeResult) string {
	if res == nil {
		return ActionNext
	}

	if action, ok := res["action"].(string); ok && action != "" {
		return action
	}

	return ActionNext
}

// SignalsFromResult returns zero or more signal names emitted by the node.
func SignalsFromResult(res NodeResult) []string {
	if res == nil {
		return nil
	}

	switch v := res["signals"].(type) {
	case string:
		return []string{v}
	case []string:
		return v
	case []any:
		signals := make([]string, 0, len(v))
		for _, raw := range v {
			if s, ok := raw.(string); ok {
				signals = append(signals, s)
			}
		}
		return signals
	}

	if signal, ok := res["signal"].(string); ok && signal != "" {
		return []string{signal}
	}

	return nil
}

// ResultWithAction builds a minimal NodeResult with an explicit action.
func ResultWithAction(action string) NodeResult {
	if action == "" {
		action = ActionNext
	}
	return NodeResult{"action": action}
}
