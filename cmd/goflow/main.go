package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/sashabaranov/go-openai"

	goflow "goflow"
	"goflow/flows"
	"goflow/nodes"
)

func main() {
	fmt.Println("Starting GoFlow Server on :8080...")
	startServer()
}

var aiClient = openAIClient()

func openAIClient() *openai.Client {
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return openai.NewClient(key)
	}
	return nil
}

// --- Web Monitor ---

type WebMonitor struct {
	events []flows.FlowEvent
	mutex  sync.RWMutex
}

func (m *WebMonitor) Notify(ctx context.Context, event flows.FlowEvent) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	// Store only relevant info to avoid huge payloads if Shared is large
	// For now, we store everything
	m.events = append(m.events, event)
}

func (m *WebMonitor) GetEvents() []flows.FlowEvent {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	events := make([]flows.FlowEvent, len(m.events))
	copy(events, m.events)
	return events
}

func (m *WebMonitor) Clear() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.events = []flows.FlowEvent{}
}

var globalMonitor = &WebMonitor{}

// --- Demo Flow Setup ---

func setupDemoFlow() *flows.Flow {
	// Define condition function
	conditionFunc := func(shared map[string]any) string {
		score, _ := shared["score"].(float64)
		if score > 80 {
			return "high"
		}
		return "low"
	}

	// Create sub-flows for different branches
	highScoreFlow := flows.NewFlowBuilder(
		nodes.NewFunctionNode("high_score_handler", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
			shared["category"] = "high"
			shared["priority"] = "urgent"
			time.Sleep(500 * time.Millisecond) // Simulate work
			return goflow.ResultWithAction("done"), nil
		}),
	).Build()

	lowScoreFlow := flows.NewFlowBuilder(
		nodes.NewFunctionNode("low_score_handler", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
			shared["category"] = "low"
			shared["priority"] = "normal"
			time.Sleep(500 * time.Millisecond) // Simulate work
			return goflow.ResultWithAction("done"), nil
		}),
	).Build()

	// Create conditional node with branches
	conditionalNode := nodes.NewConditionalNode("score_router", conditionFunc)
	conditionalNode.Branch("high", highScoreFlow)
	conditionalNode.Branch("low", lowScoreFlow)

	// Build main flow
	startNode := nodes.NewFunctionNode("start", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
		fmt.Printf("[StartNode] Starting with input: %v\n", shared["input"])
		time.Sleep(500 * time.Millisecond) // Simulate work
		return goflow.ResultWithAction("next"), nil
	})

	processNode := nodes.NewFunctionNode("process", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
		fmt.Println("[ProcessNode] Processing...")
		time.Sleep(500 * time.Millisecond) // Simulate work
		return goflow.ResultWithAction("next"), nil
	})

	finalizerNode := nodes.NewFunctionNode("finalizer", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
		fmt.Printf("[FinalizerNode] Final category: %v, priority: %v\n", shared["category"], shared["priority"])
		return goflow.ResultWithAction("end"), nil
	})

	flow := flows.NewFlowBuilder(startNode).
		Then(processNode).
		Then(conditionalNode).
		Then(finalizerNode).
		WithMonitor(globalMonitor).
		Build()

	return flow
}

// --- Server ---

func startServer() {
	flow := setupDemoFlow()

	// API Handlers
	http.HandleFunc("/api/graph", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(flow.Graph())
	})

	http.HandleFunc("/api/run", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var input map[string]any
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Reset monitor events
		globalMonitor.Clear()

		// Run in background
		go func() {
			err := flow.Run(context.Background(), input)
			if err != nil {
				log.Printf("Flow execution failed: %v", err)
			}
		}()

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "started"})
	})

	http.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(globalMonitor.GetEvents())
	})

	// Static files
	fs := http.FileServer(http.Dir("./frontend/dist"))
	http.Handle("/", fs)

	log.Fatal(http.ListenAndServe(":8080", nil))
}
