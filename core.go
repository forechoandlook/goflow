package goflow

import "context"

// NodeResult allows nodes to return an actionable payload that can include an
// explicit action, emitted signals, or any other metadata the flow might need.
type NodeResult map[string]any

// Node is a step that can run inside a flow.
type Node interface {
	Name() string
	Run(ctx context.Context, shared map[string]any) (NodeResult, error)
}

const (
	// Constants for common actions
	ActionContinue = "continue"
	ActionPause    = "pause_for_human"
	ActionEnd      = "end"
	ActionNext     = "next"
	ActionRetry    = "retry"
	ActionTimeout  = "timeout"
)
