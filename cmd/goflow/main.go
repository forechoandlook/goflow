package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/sashabaranov/go-openai"

	goflow "goflow"
	"goflow/flows"
	"goflow/kv"
	"goflow/nodes"
	"goflow/utils"
)

func main() {
	fmt.Println("Starting GoFlow example...")

	// Example 1: Basic flow with new builder pattern
	fmt.Println("\n=== Basic Flow with Builder ===")
	basicFlowExample()

	// Example 2: Conditional routing with sub-flows
	fmt.Println("\n=== Conditional Routing with Sub-Flows ===")
	conditionalRoutingExample()

	// Example 3: Parallel execution
	fmt.Println("\n=== Parallel Execution ===")
	parallelExecutionExample()

	// Example 4: Checkpointing with options
	fmt.Println("\n=== Checkpointing with Options ===")
	checkpointingExample()

	// Example 5: Custom function nodes
	fmt.Println("\n=== Custom Function Nodes ===")
	customFunctionNodesExample()
}

var aiClient = openAIClient()

func openAIClient() *openai.Client {
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return openai.NewClient(key)
	}
	return nil
}

func llmNode(prompt string) *nodes.LLMNode {
	cfg := nodes.DefaultLLMNodeConfig(prompt)
	cfg.SystemPrompt = fmt.Sprintf("You are a workflow assistant. %s", prompt)
	return nodes.NewLLMNode(aiClient, cfg)
}

func basicFlowExample() {
	shared := map[string]any{
		"input": "translate this to french: hello world",
	}

	// Using the new FlowBuilder pattern
	flow := flows.NewFlowBuilder(llmNode("translate to french")).
		Then(llmNode("polish text")).
		Then(llmNode("summarize in one sentence")).
		Build()

	ctx := context.Background()
	err := flow.Run(ctx, shared)
	if err != nil {
		log.Printf("Flow execution failed: %v", err)
		return
	}

	fmt.Printf("Final shared state: %+v\n", shared)
}

func conditionalRoutingExample() {
	shared := map[string]any{
		"input": "analyze sentiment",
		"score": 95.0,
	}

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
			return goflow.ResultWithAction("done"), nil
		}),
	).Build()

	lowScoreFlow := flows.NewFlowBuilder(
		nodes.NewFunctionNode("low_score_handler", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
			shared["category"] = "low"
			shared["priority"] = "normal"
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
		return goflow.ResultWithAction("next"), nil
	})

	finalizerNode := nodes.NewFunctionNode("finalizer", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
		fmt.Printf("[FinalizerNode] Final category: %v, priority: %v\n", shared["category"], shared["priority"])
		return goflow.ResultWithAction("end"), nil
	})

	mainFlow := flows.NewFlowBuilder(startNode).
		Then(conditionalNode).
		Then(finalizerNode). // Connect directly after conditional node
		Build()

	ctx := context.Background()
	err := mainFlow.Run(ctx, shared)
	if err != nil {
		log.Printf("Flow execution failed: %v", err)
		return
	}

	fmt.Printf("Final shared state: %+v\n", shared)
}

func parallelExecutionExample() {
	shared := map[string]any{
		"input": "parallel processing test",
	}

	// Create sub-flows for parallel execution
	parallelFlow1 := flows.NewFlowBuilder(
		nodes.NewFunctionNode("parallel_task_1", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
			// Simulate some work
			time.Sleep(100 * time.Millisecond)
			shared["task1_result"] = "completed"
			shared["task1_data"] = "data_from_task1"
			return goflow.ResultWithAction("next"), nil
		}),
	).Then(nodes.NewFunctionNode("post_task_1", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
		shared["task1_post"] = "post_processed"
		return goflow.ResultWithAction("done"), nil
	})).Build()

	parallelFlow2 := flows.NewFlowBuilder(
		nodes.NewFunctionNode("parallel_task_2", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
			// Simulate some work
			time.Sleep(150 * time.Millisecond)
			shared["task2_result"] = "completed"
			shared["task2_data"] = "data_from_task2"
			return goflow.ResultWithAction("next"), nil
		}),
	).Then(nodes.NewFunctionNode("post_task_2", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
		shared["task2_post"] = "post_processed"
		return goflow.ResultWithAction("done"), nil
	})).Build()

	parallelFlow3 := flows.NewFlowBuilder(
		nodes.NewFunctionNode("parallel_task_3", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
			// Simulate some work
			time.Sleep(50 * time.Millisecond)
			shared["task3_result"] = "completed"
			shared["task3_data"] = "data_from_task3"
			return goflow.ResultWithAction("next"), nil
		}),
	).Build()

	// Create parallel node
	parallelNode := nodes.NewParallelNode("parallel_processing", parallelFlow1, parallelFlow2, parallelFlow3)

	// Build main flow
	mainFlow := flows.NewFlowBuilder(nodes.NewFunctionNode("init", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
		fmt.Println("Initializing parallel processing...")
		return goflow.ResultWithAction("start"), nil
	})).
		Then(parallelNode).
		Then(nodes.NewFunctionNode("finalize", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
			fmt.Printf("All parallel tasks completed. Results: task1=%v, task2=%v, task3=%v\n",
				shared["task1_result"], shared["task2_result"], shared["task3_result"])
			return goflow.ResultWithAction("end"), nil
		})).
		Build()

	ctx := context.Background()
	err := mainFlow.Run(ctx, shared)
	if err != nil {
		log.Printf("Flow execution failed: %v", err)
		return
	}

	fmt.Printf("Final shared state: %+v\n", shared)
}

func checkpointingExample() {
	threadID := "workflow-123"
	initialShared := map[string]any{
		"input": "checkpoint test",
		"step":  1,
		"data":  "initial data",
	}

	// Create a flow with checkpointing enabled
	startNode := nodes.NewFunctionNode("start", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
		fmt.Println("Starting workflow...")
		shared["start_time"] = time.Now().Unix()
		return goflow.ResultWithAction("continue"), nil
	})

	middleNode := nodes.NewFunctionNode("middle", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
		fmt.Println("Processing in middle node...")
		shared["processed"] = true
		shared["middle_step"] = 2
		return goflow.ResultWithAction("continue"), nil
	})

	endNode := nodes.NewFunctionNode("end", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
		fmt.Println("Finishing workflow...")
		shared["end_time"] = time.Now().Unix()
		shared["completed"] = true
		return goflow.ResultWithAction("end"), nil
	})

	// Configure flow with checkpointing options
	options := flows.FlowOption{
		EnableCheckpoint: true,
		KVStore:          kv.NewFileBasedKVStore("./checkpoints.json"),
		MaxSteps:         10,
		FlowID:           threadID,
		Timeout:          30 * time.Second,
	}

	flow := flows.NewFlowBuilder(startNode).
		Then(middleNode).
		Then(endNode).
		WithOptions(options).
		Build()

	ctx := context.Background()

	// Run the flow
	fmt.Println("Running flow with checkpointing...")
	err := flow.Run(ctx, initialShared)
	if err != nil {
		log.Printf("Flow execution failed: %v", err)
		return
	}

	fmt.Printf("Final shared state: %+v\n", initialShared)

	// Test timeout functionality
	fmt.Println("\n=== Timeout Example ===")
	timeoutExample()
}

func timeoutExample() {
	shared := map[string]any{
		"input": "timeout test",
	}

	// Create a slow node that exceeds timeout
	slowNode := utils.WithTimeoutOnNode(llmNode("slow operation"), 100*time.Millisecond)

	// Create flow with timeout
	options := flows.FlowOption{
		Timeout: 50 * time.Millisecond, // Shorter than the slow node's simulated delay
	}

	flow := flows.NewFlowWithOptions(slowNode, options)

	ctx := context.Background()
	err := flow.Run(ctx, shared)
	if err != nil {
		fmt.Printf("Expected timeout error: %v\n", err)
	} else {
		fmt.Printf("Unexpected success: %+v\n", shared)
	}
}

func customFunctionNodesExample() {
	shared := map[string]any{
		"user_id": 12345,
		"request": "get_profile",
	}

	// Create custom nodes using the FunctionNode
	authNode := nodes.NewFunctionNode("auth_check", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
		userID, ok := shared["user_id"].(int)
		if !ok || userID <= 0 {
			return goflow.ResultWithAction("unauthorized"), fmt.Errorf("invalid user ID")
		}

		// Simulate auth check
		shared["authenticated"] = true
		shared["user_role"] = "premium"
		return goflow.ResultWithAction("authorized"), nil
	})

	dataNode := nodes.NewFunctionNode("data_fetch", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
		if !shared["authenticated"].(bool) {
			return goflow.ResultWithAction("error"), fmt.Errorf("not authenticated")
		}

		// Simulate data fetching
		shared["user_data"] = map[string]interface{}{
			"id":    shared["user_id"],
			"name":  "John Doe",
			"email": "john@example.com",
		}
		return goflow.ResultWithAction("success"), nil
	})

	responseNode := nodes.NewFunctionNode("response_build", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
		userData := shared["user_data"].(map[string]interface{})
		shared["response"] = fmt.Sprintf("User profile: %s (%s)", userData["name"], userData["email"])
		return goflow.ResultWithAction("complete"), nil
	})

	// Build flow
	flow := flows.NewFlowBuilder(authNode).
		Connect(authNode, "authorized", dataNode).
		Connect(authNode, "unauthorized", nodes.NewFunctionNode("reject", func(ctx context.Context, shared map[string]any) (goflow.NodeResult, error) {
			shared["response"] = "Access denied"
			return goflow.ResultWithAction("rejected"), nil
		})).
		Connect(dataNode, "success", responseNode).
		Build()

	ctx := context.Background()
	err := flow.Run(ctx, shared)
	if err != nil {
		log.Printf("Flow execution failed: %v", err)
		return
	}

	fmt.Printf("Final shared state: %+v\n", shared)
}
