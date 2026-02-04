package flows

import (
	"context"
	"time"
)

// emitEvent emits a flow event to all registered monitors
func (f *Flow) emitEvent(ctx context.Context, event FlowEvent) {
	f.monitorMux.RLock()
	monitors := append([]FlowMonitor(nil), f.monitors...)
	f.monitorMux.RUnlock()

	if len(monitors) == 0 {
		return
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	for _, monitor := range monitors {
		monitor.Notify(ctx, event)
	}
}

// emitSignals emits signals to registered listeners
func (f *Flow) emitSignals(ctx context.Context, node Node, signals []string, shared map[string]any) {
	if len(signals) == 0 {
		return
	}

	f.emitEvent(ctx, FlowEvent{
		Type:    FlowEventTypeSignalEmitted,
		Node:    node.Name(),
		Signals: signals,
		Shared:  snapshotShared(shared),
	})

	for _, signal := range signals {
		f.mutex.RLock()
		listeners := append([]Node(nil), f.signalListeners[signal]...)
		f.mutex.RUnlock()

		for _, listener := range listeners {
			go f.runSignalListener(ctx, listener, shared)
		}
	}
}

// runSignalListener runs a signal listener node
func (f *Flow) runSignalListener(ctx context.Context, node Node, shared map[string]any) {
	_, _, _ = f.runNodeWithAttributes(ctx, node, shared)
}
