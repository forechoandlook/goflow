package flows

import (
	"context"
	"errors"
	"testing"

	goflow "goflow"
	"goflow/nodes"
)

type recordingMonitor struct {
	events []FlowEvent
}

func (m *recordingMonitor) Notify(_ context.Context, event FlowEvent) {
	m.events = append(m.events, event)
}

func TestFlowMonitorEvents(t *testing.T) {
	attempts := 0
	var unstable nodes.Node = nodes.NewFunctionNode("unstable", func(_ context.Context, shared map[string]any) (goflow.NodeResult, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("boom")
		}
		shared["recovered"] = true
		return goflow.ResultWithAction(goflow.ActionNext), nil
	})
	unstable = nodes.WrapNodeWithAttributes(unstable, nodes.NodeAttributes{RetryAttempts: 1})

	final := nodes.NewFunctionNode("final", func(_ context.Context, shared map[string]any) (goflow.NodeResult, error) {
		shared["done"] = true
		return goflow.ResultWithAction(goflow.ActionEnd), nil
	})

	monitor := &recordingMonitor{}
	flow := NewFlowBuilder(unstable).
		Then(final).
		WithMonitor(monitor).
		Build()

	shared := make(map[string]any)
	if err := flow.Run(context.Background(), shared); err != nil {
		t.Fatalf("flow run returned error: %v", err)
	}

	if done, ok := shared["done"].(bool); !ok || !done {
		t.Fatalf("expected final node to mark done, got %#v", shared["done"])
	}

	if len(monitor.events) == 0 {
		t.Fatal("expected monitor to receive events")
	}

	var sawRetry, sawFlowStart bool
	last := monitor.events[len(monitor.events)-1]
	if last.Type != FlowEventTypeFlowComplete {
		t.Fatalf("expected last event to be FlowComplete, got %s", last.Type)
	}

	for _, event := range monitor.events {
		switch event.Type {
		case FlowEventTypeFlowStart:
			if event.Node != "unstable" {
				t.Fatalf("flow start should report the entry node, got %q", event.Node)
			}
			sawFlowStart = true
		case FlowEventTypeNodeRetry:
			if event.Attempt != 1 {
				t.Fatalf("retry event should report first attempt, got %d", event.Attempt)
			}
			sawRetry = true
		}
	}

	if !sawFlowStart {
		t.Fatal("FlowStart event was not emitted")
	}
	if !sawRetry {
		t.Fatal("NodeRetry event was not emitted")
	}
}

func TestParseFlowDSL(t *testing.T) {
	script := `
node seed = set prompt "translate to spanish"
node translator = llm input=prompt output=translation system="You are a translator"
node logger = logger "translation" translation
start seed
connect seed -> translator
connect translator -> logger
`

	flow, err := ParseFlowDSL(script)
	if err != nil {
		t.Fatalf("ParseFlowDSL returned error: %v", err)
	}

	shared := make(map[string]any)
	if err := flow.Run(context.Background(), shared); err != nil {
		t.Fatalf("flow run failed: %v", err)
	}

	if translated, ok := shared["translation"].(string); !ok || translated == "" {
		t.Fatalf("expected translation output, got %#v", shared["translation"])
	}
}
